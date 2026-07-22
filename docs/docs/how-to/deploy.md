# How to deploy incus-gh-runner to production

Deploy the `incus-gh-runner` controller as a hardened systemd unit and connect it to a live GitHub scale set using either a GitHub App or a personal access token (PAT).

## Prerequisites

- Incus 7.0 or newer, initialized with QEMU VM support, on a host reachable at the target Incus socket. Incus 6 is not supported.
- The host `br_netfilter` kernel module loaded at boot. Incus requires it when
  starting bridged NICs with `security.ipv4_filtering` or
  `security.ipv6_filtering`; filtered bridged NICs cannot start without it.
- The `incus-admin` group exists on the host (`getent group incus-admin`). Membership in this group grants root-equivalent access to the Incus socket.

    !!! warning "Root-equivalent socket access"
        `SupplementaryGroups=incus-admin` in the unit file gives the controller the same host access as a user in `incus-admin`. Run the controller on a host dedicated to this workload, not one shared with unrelated services.

- A systemd version supporting `LoadCredential=` and the `%d` credentials-directory specifier the unit relies on, along with `DynamicUser=` and the unit's other sandboxing directives. Ubuntu 24.04 is the validated reference host. TPM-bound proof keys additionally require systemd 250 or newer, an enrolled TPM 2.0 device, and the distribution's TPM2 userspace runtime libraries.
- Administrative access to the target GitHub organization or repository.
- The `incus-gh-runner` package or binary for your platform, and a checkout of this repository: the steps below use the desired-state files from `deploy/incus/`.

## 1. Prepare and validate Incus

Load bridge netfilter now and persist it across host reboots:

```sh
sudo modprobe br_netfilter
printf 'br_netfilter\n' | sudo tee /etc/modules-load.d/incus-gh-runner.conf >/dev/null
test -d /sys/module/br_netfilter
```

Treat a failed check as a host-preparation error. The API drift validator
cannot prove kernel-module state, so verify the module after provisioning and
after every host reboot.

Start from the fail-closed desired-state example instead of creating an
unrestricted project and attaching the project's `default` profile:

```sh
cp deploy/incus/baseline.example.json incus-baseline.json
```

The repository also ships the dependency-free CUE policy prototype under
`deploy/incus/cue/`. Its default input renders exactly the JSON above, derives
aggregate project ceilings from host and runner capacity, and rejects attempts
to weaken fixed isolation controls. It also renders a partial controller
configuration that keeps `incus.project`, the sole Incus profile, and
`capacity.max_runners` aligned with those ceilings. The module is not yet
registry-published, so the rendered files remain local deployment artifacts for
this increment.

Edit the copy for the target host. In particular, replace every documentation
address, bridge subnet, resource name, ZFS source, and capacity limit. Managed
bridge names must be 2 to 15 characters, start with a lowercase letter, and
otherwise contain only lowercase letters, digits, or hyphens. The example proxy
and DNS addresses are non-routable and intentionally provide no useful egress
until replaced. Configure a controlled proxy to allow only GitHub or GHES and
the dependency destinations approved for this builder. Do not replace the proxy
boundary with unrestricted TCP/443 and call it a GitHub allowlist.

`baseline.example.json` is reviewable desired state, not an input Incus can
apply directly. Materialize the exact project, network, ACL, profile, and
storage state through your Incus CLI or infrastructure-management workflow;
the controller does not create or modify that infrastructure. Keep the bridge
and ACL host-owned in the Incus `default` project, while the restricted runner
project inherits the allowlisted bridge and owns only its runner profile. The
baseline requires a dedicated ZFS pool, default-deny ACLs at the bridge and
NIC, anti-spoofing and port isolation, and both per-VM and aggregate project
ceilings. Keep the current controller on a dedicated, single-purpose host: its
Unix-socket `incus-admin` identity remains root-equivalent.

Set the project VM limit at or above the controller's
`capacity.max_runners`, then size aggregate CPU, memory, and disk for that many
profile-limited VMs while reserving explicit headroom for Incus, the
controller, and the host. A project limit below `capacity.max_runners` makes
requested capacity impossible; limits at physical capacity do not protect the
host control plane from exhaustion. Incus project CPU and memory ceilings are
admission budgets calculated from the declared per-VM limits; they are not
aggregate runtime throttles shared dynamically by running VMs.

Validate the effective API state before importing an image or starting the
controller:

```sh
incus-gh-runner validate incus-baseline.json
```

The validator defaults to `/var/lib/incus/unix.socket`. Pass another local
socket explicitly when needed:

```sh
incus-gh-runner validate --socket /run/incus/unix.socket incus-baseline.json
```

This command is read-only and fails on drift. It validates the baseline against
the embedded CUE policy in process and reads effective state from the local
Incus socket; it does not invoke external `cue`, `incus`, or `jq` executables.
It does not load controller configuration or require GitHub credentials. The
socket remains root-equivalent, so run the command only from a trusted host
administration context.

The validator confirms the effective resource ceilings, but it cannot re-prove
the physical-host capacity and reserved headroom used when CUE generated them.
Re-render and review the baseline after changing host capacity or reservations.
Resolve every failure; do not weaken or bypass it to continue deployment. See
[`deploy/incus/README.md`](https://github.com/meigma/incus-gh-runner/tree/master/deploy/incus)
for the manifest contract, controlled-egress model, compatibility residuals,
and the official Incus references behind each setting.

Import a runner image into the validated project. Any image that implements
the [guest contract](../reference/guest-contract.md) works; see [Build a
hardened runner image](./build-runner-images.md) for building and boot-testing
one. Configure only the validated
`github-runner` profile; adding the `default` or a second profile can add
devices or relax limits outside the checked baseline. The controller pins and
materializes the validated profile snapshot into each VM, so later profile
edits do not alter an approved runner environment.

## 2. Choose the GitHub scope and credential

Private-repository scope is the recommended hardened starting point. Set
`github.config_url` to the exact HTTPS destination for the scale set:

- One repository: `https://github.com/OWNER/REPOSITORY`
- An organization: `https://github.com/ORGANIZATION`

A private repository URL restricts the scale set to that repository and must
use the `default` runner group. Do not expose this self-hosted runner to an
untrusted public-repository fork workflow; public repositories require a
separate threat review and workflow/approval policy. Enterprise URLs are
outside the supported controller contract. Before using organization scope:

1. Create a dedicated non-default runner group, such as `incus-gh-runner-prod`.
2. Set its repository access to **Selected repositories** and disable access to
   public repositories.
3. Enable selected-workflow access and allow only fully qualified workflow
   paths. For a SLSA builder, pin each allowed workflow as
   `OWNER/REPOSITORY/.github/workflows/build.yml@<full-commit-SHA>`.
4. Confirm the group contains only the intended scale set, then use the exact
   group name as `github.runner_group`.

GitHub documents these controls in
[Managing access to self-hosted runners using groups](https://docs.github.com/en/actions/how-tos/manage-runners/self-hosted-runners/manage-access).
The [runner-group REST fields](https://docs.github.com/en/rest/actions/self-hosted-runner-groups?apiVersion=2022-11-28)
expose the same repository, public-repository, and selected-workflow policy for
automation and verification.
The controller rejects the broad `default` group at organization scope. If the
selected-workflow restriction is unavailable, use repository scope; the
hardened organization profile depends on that control. GitHub App installation
scope controls API authority, but it is not a substitute for the runner group's
scheduling policy.

The time-limited token shown on GitHub's **New self-hosted runner** page cannot operate this controller: it registers one runner once, while `incus-gh-runner` must continuously create fresh JIT configurations for replacement VMs.

Choose one of the following renewable credential methods. GitHub Apps are preferred for independent lifecycle and rotation; a repository-scoped fine-grained PAT is the simpler option when installing an App is undesirable. GitHub maintains the current permission requirements in [Authenticating Actions Runner Controller to the GitHub API](https://docs.github.com/en/actions/how-tos/manage-runners/use-actions-runner-controller/authenticate-to-the-api).

### Option A: GitHub App

1. Create a GitHub App owned by the target organization.
2. Grant only the permissions for the selected scope:
   - Repository scope: repository **Administration: read and write** and **Metadata: read-only**.
   - Organization scope: repository **Metadata: read-only** and organization **Self-hosted runners: read and write**.
3. Install the App for the target organization or selected repository.
4. Record the App's client ID and installation ID.
5. Generate and download a private key PEM file.

### Option B: personal access token

For a single repository, create a fine-grained PAT that can access only that
repository and grant repository **Administration: read and write**. For an
organization-scoped scale set, grant organization **Administration: read** and
**Self-hosted runners: read and write**. Do not add unrelated permissions.

A classic PAT also works, but requires the broader `repo` scope for repository runners or `admin:org` for organization runners. Prefer a fine-grained PAT when GitHub makes it available for the target.

## 3. Install the controller

Prefer the native package from the GitHub release. It installs the binary, base
unit, tmpfiles policy, editable example configuration, license files, and
credential drop-in examples without enabling or starting the service:

```sh
sudo apt-get install ./incus-gh-runner_<version>_amd64.deb
# or
sudo dnf install ./incus-gh-runner-<version>-1.x86_64.rpm
```

Use `arm64.deb` or `aarch64.rpm` on ARM64 hosts. Packaged credential examples
are under `/usr/share/doc/incus-gh-runner/systemd/`; select and install exactly
one GitHub credential method later in this guide.

For a raw-binary installation, install the same files manually:

```sh
sudo install -m 0755 incus-gh-runner /usr/bin/incus-gh-runner
sudo install -m 0644 deploy/systemd/incus-gh-runner.service /etc/systemd/system/incus-gh-runner.service
sudo install -m 0644 deploy/systemd/incus-gh-runner.tmpfiles.conf /usr/lib/tmpfiles.d/incus-gh-runner.conf
sudo install -d -m 0755 /etc/incus-gh-runner
sudo install -m 0644 deploy/systemd/config.example.yaml /etc/incus-gh-runner/config.yaml
```

The unit runs under `DynamicUser=yes`, so `config.yaml` must remain readable by the dynamically allocated service user. Credential files remain root-only and are exposed to the service through systemd's protected credential directory. The tmpfiles policy does not enable diagnostics persistence; it expires files from the recommended diagnostics directory if you opt in later.

## 4. Write the configuration

Edit the installed `/etc/incus-gh-runner/config.yaml`. The shipped example
already matches this walkthrough's Incus and capacity values; set
`github.config_url` to the exact destination chosen in step 2 and change
`github.scale_set` from the example's `incus-linux-x64` to the label your
workflows will target — this guide uses `incus-gh-runner-prod` throughout. The
example's remaining keys ship at the built-in defaults and can stay unchanged.
The result works with either credential method:

```yaml
github:
  config_url: https://github.com/OWNER/REPOSITORY
  scale_set: incus-gh-runner-prod
  runner_group: default
incus:
  project: github-runners
  image: incus-gh-runner-v0.1.0
  profiles: [github-runner]
  owner: incus-gh-runner-production
capacity:
  min_runners: 1
  max_runners: 4
```

For organization scope, replace the three GitHub scheduling fields with a
dedicated group whose selected-repository and selected-workflow policy was
configured in step 2:

```yaml
github:
  config_url: https://github.com/ORGANIZATION
  scale_set: incus-gh-runner-prod
  runner_group: incus-gh-runner-prod
```

When using a GitHub App, add its non-secret identifiers beneath the existing
`github` mapping. Do not add a second `github` key; exact configuration decoding
rejects duplicate keys.

```yaml
github:
  app:
    client_id: Iv1.xxxxxxxxxxxxxxxx
    installation_id: 12345678
```

When using a PAT, do not add the `app` block. The selected systemd drop-in supplies the remaining credential path.

- `github.scale_set` names the runner scale set; the controller creates it automatically on first start if it does not already exist.
- `incus.image` is the alias or fingerprint of the image you imported in step 1.
- `incus.owner` is an arbitrary cleanup selector exclusive to this deployment — do not reuse it across independent controller instances pointed at the same Incus project. Another project writer can forge it, so it is not authorization.

See [Configuration reference](../reference/configuration.md) for every key, default, environment variable, and credential validation rule.

## 5. Install one GitHub credential drop-in

Install exactly one credential file and its matching drop-in as `credentials.conf`.

For a GitHub App:

```sh
sudo install -m 0600 github-app-private-key.pem /etc/incus-gh-runner/github-app-private-key.pem
sudo install -d -m 0755 /etc/systemd/system/incus-gh-runner.service.d
sudo install -m 0644 deploy/systemd/credentials-github-app.conf \
  /etc/systemd/system/incus-gh-runner.service.d/credentials.conf
```

For a PAT stored in a local file named `github-token`:

```sh
sudo install -m 0600 github-token /etc/incus-gh-runner/github-token
sudo install -d -m 0755 /etc/systemd/system/incus-gh-runner.service.d
sudo install -m 0644 deploy/systemd/credentials-personal-access-token.conf \
  /etc/systemd/system/incus-gh-runner.service.d/credentials.conf
```

Do not place the App private key or PAT value in `config.yaml`. The drop-ins load the root-owned source file and point the controller at the protected runtime copy. To change methods, replace `credentials.conf`, add or remove the `github.app` identifiers in `config.yaml`, then reload and restart the service.

## 6. Enable job proofs (optional)

Job proofs bind each GitHub Actions job to the Incus VM that ran it; the
[job proofs reference](../reference/job-proofs.md) documents the proof
envelope, payload schema, and key-ID rule. Generate and enroll the host's
Ed25519 proof key, then choose one proof-key storage mode. Both modes expose
the same runtime credential to the unchanged controller and compose with
either GitHub credential drop-in.

### Generate and enroll the proof key

Generate the Ed25519 signing key and its SubjectPublicKeyInfo public key
without loosening the process umask:

```sh
umask 077
openssl genpkey -algorithm Ed25519 -out machine-provenance-key.pem
openssl pkey \
  -in machine-provenance-key.pem \
  -pubout \
  -out machine-provenance-key.pub.pem
```

Derive the enrolled key ID with OpenSSL:

```sh
key_hex="$(
  openssl pkey -pubin -in machine-provenance-key.pub.pem -outform DER |
    openssl dgst -sha256 -r |
    awk '{print $1}'
)"
printf 'sha256:%s\n' "$key_hex"
```

Enroll three values with each proof consumer: the stable `job_proof.host_id`,
`machine-provenance-key.pub.pem`, and the derived `sha256:<hex>` key ID. See
the [key-ID rule](../reference/job-proofs.md#key-id) for what the key ID does
and does not identify.

### Option A: file-backed proof key

Install the private source key as `root:root` mode `0600`, then install the
file-backed proof credential drop-in:

```sh
sudo install -o root -g root -m 0600 machine-provenance-key.pem \
  /etc/incus-gh-runner/machine-provenance-key.pem
sudo install -m 0644 deploy/systemd/credentials-job-proof-file.conf \
  /etc/systemd/system/incus-gh-runner.service.d/job-proof.conf
sudo stat -c '%U:%G %a' /etc/incus-gh-runner/machine-provenance-key.pem
```

The final command must print `root:root 600`. Add the enrolled host identity to
`config.yaml`; the drop-in supplies `job_proof.signing_key_file` through the
protected systemd runtime credential:

```yaml
job_proof:
  host_id: builder-host-01
```

The proof drop-in does not replace `credentials.conf`. It composes with either
GitHub credential method and leaves proofs disabled when it is absent.

### Option B: TPM-bound proof key

Use systemd 250 or newer to encrypt the same PKCS#8 Ed25519 key to the target
host's TPM. The encryption attempt is the capability check; do not gate this
procedure on `systemd-creds has-tpm2`, which is unavailable on older supported
systemd versions.

Install the distribution's TPM2 userspace stack first. On minimal Ubuntu
systems, `systemd` can report `+TPM2` while some dynamically loaded TSS2
libraries are absent; the `tpm2-tools` package supplies them:

```sh
sudo apt-get update
sudo apt-get install tpm2-tools
```

Treat a failed encryption attempt as authoritative even when `systemd
--version` reports `+TPM2`. If systemd reports that AES-128-CFB may be missing,
rerun with `SYSTEMD_LOG_LEVEL=debug`; a failed TSS2 library load must be fixed
before treating that message as a TPM firmware limitation.

Create the encrypted credential directory explicitly and stage the plaintext
key only on the root-owned `/run` temporary filesystem:

```sh
sudo install -d -o root -g root -m 0700 /etc/credstore.encrypted
sudo install -o root -g root -m 0600 machine-provenance-key.pem \
  /run/incus-gh-runner-machine-provenance-key.pem
sudo systemd-creds encrypt \
  --name=machine-provenance-key \
  --with-key=tpm2 \
  --tpm2-device=auto \
  --tpm2-pcrs= \
  /run/incus-gh-runner-machine-provenance-key.pem \
  /etc/credstore.encrypted/incus-gh-runner-machine-provenance-key.cred
sudo chmod 0600 \
  /etc/credstore.encrypted/incus-gh-runner-machine-provenance-key.cred
```

Before removing the plaintext, decrypt once on the origin host and confirm that
it derives the public key already enrolled for this `job_proof.host_id`:

```sh
sudo systemd-creds decrypt \
  --name=machine-provenance-key \
  /etc/credstore.encrypted/incus-gh-runner-machine-provenance-key.cred \
  /run/incus-gh-runner-machine-provenance-key.check.pem
sudo openssl pkey \
  -in /run/incus-gh-runner-machine-provenance-key.check.pem \
  -pubout | cmp - machine-provenance-key.pub.pem && \
  sudo rm -f \
    /run/incus-gh-runner-machine-provenance-key.pem \
    /run/incus-gh-runner-machine-provenance-key.check.pem
```

If the public-key comparison fails, the plaintext files remain under the
root-only `/run` paths for diagnosis and must not be treated as successfully
enrolled.

Install the TPM-bound drop-in:

```sh
sudo install -m 0644 deploy/systemd/credentials-job-proof-tpm.conf \
  /etc/systemd/system/incus-gh-runner.service.d/job-proof.conf
```

The empty PCR set is deliberate: normal firmware, kernel, and bootloader
updates must not lock out the service. `PrivateDevices=yes` remains enabled;
systemd decrypts the credential during service activation, before the
controller enters its device namespace. The controller never opens a TPM
device.

Delete any remaining plaintext transfer copy after successful encryption, or
move it into an explicit offline recovery escrow. Escrow permits the same key
to be sealed again after TPM or motherboard replacement, but weakens the
assurance that the encrypted credential is the only recoverable private-key
copy. Without escrow, replacement requires a new key and consumer enrollment.

If a second TPM host is available, copy only the encrypted credential there
and run this same name-aware check; it must fail after origin-host decryption
succeeded:

```sh
sudo systemd-creds decrypt \
  --name=machine-provenance-key \
  incus-gh-runner-machine-provenance-key.cred \
  /run/incus-gh-runner-machine-provenance-key.cross-host.pem
```

Remove the temporary output if the command unexpectedly creates it. Without a
second host, record cross-host binding as an untested evidence gap rather than
claiming it.

### Rotate and recover proof keys

Rotate with overlap: distribute and trust the new public key first, replace
the file-backed source or encrypt and install a new TPM-bound credential,
then restart the controller. Retain the old public key for as long as
existing proofs must remain verifiable.

TPM clearing or motherboard replacement makes the encrypted credential
unusable. With an offline escrow, seal the same private key to the
replacement TPM, following the escrow guidance in
[Option B](#option-b-tpm-bound-proof-key); without one, generate a new key
and enroll its public key and key ID before restarting.

## 7. Start and enable the service

```sh
sudo systemctl daemon-reload
sudo systemctl enable --now incus-gh-runner.service
```

## Verification

Confirm the controller started and connected successfully:

```sh
sudo journalctl -u incus-gh-runner -n 50 --no-pager
```

Look for a JSON log line with `msg="incus-gh-runner started"`, carrying `scale_set`, `scale_set_id`, and `incus_project` fields. Its absence, or a repeating restart loop, means startup failed — check the preceding log lines for the specific error before continuing.

For TPM-bound job proofs, reboot the host normally and confirm the service
starts again before accepting the deployment. Retrieve a proof in a real job,
verify it externally with the enrolled public key, and compare it with a
file-backed proof. The storage modes must produce the same schema, key-ID rule,
verifier behavior, and workflow experience; the receipt cannot attest which
storage mode was used.

Validate a repository-scoped deployment end to end by running a real workflow
job in the configured repository:

```yaml
jobs:
  example:
    runs-on: incus-gh-runner-prod
    steps:
      - run: echo "hello from incus-gh-runner"
```

Dispatch the job and confirm a VM is created, the job completes, and the VM is deleted afterward.

At organization scope, select both the approved group and the scale-set label:

```yaml
jobs:
  example:
    runs-on:
      group: incus-gh-runner-prod
      labels: incus-gh-runner-prod
    steps:
      - run: echo "hello from incus-gh-runner"
```

Then inspect the runner group and
confirm that it is non-default, selected-repository only, public repository
access is disabled, and its selected workflows are commit-pinned as intended.
Before production use, dispatch the scale-set label from a disposable
unauthorized repository and from an unauthorized workflow. Neither dispatch
may create controller demand or a runner VM. If selected-workflow restriction
cannot be configured and verified, deploy at repository scope instead.

## Related

- [Operate and troubleshoot incus-gh-runner](./operate.md) — running the deployed controller, log fields, and recovering from failures.
- [Build a hardened runner image](./build-runner-images.md) — building and boot-testing a contract-conforming guest image.
- [Configuration reference](../reference/configuration.md) — every config key, environment variable, CLI flag, and credential rule.
- [How incus-gh-runner works](../explanation/how-it-works.md) — capacity model, runner lifecycle, and the security model behind the credential and cleanup rules used above.
