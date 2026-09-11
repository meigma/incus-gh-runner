# How to deploy incus-gh-runner to production

Deploy the `incus-gh-runner` controller as a hardened systemd unit and connect it to a live GitHub scale set using either a GitHub App or a personal access token (PAT).

## Prerequisites

- A dedicated, single-purpose Incus 7.0 or newer compute host with QEMU VM
  support. Incus 6 is not supported.
- A Linux controller machine. For Unix-socket mode, this is the compute host
  and the `incus-admin` group must exist. For HTTPS mode, the controller can be
  a separate machine and does not need `incus-admin` membership.
- An existing trusted Incus administration path with authority to configure
  the compute host, projects, networks, ACLs, profiles, storage, and trust
  entries. Use a trusted administration workstation for HTTPS enrollment.
  IncusOS has no general-purpose host shell, so an IncusOS compute host
  requires HTTPS mode and all Incus CLI commands run from the workstation.
- The compute host's `br_netfilter` kernel module loaded at boot. Incus
  requires it when starting bridged NICs with `security.ipv4_filtering` or
  `security.ipv6_filtering`.
- A systemd version on the controller machine supporting `LoadCredential=`,
  the `%d` credentials-directory specifier, `DynamicUser=`, and the unit's
  other sandboxing directives. TPM-bound proof keys additionally require
  systemd 250 or newer, an enrolled TPM 2.0 device, and the distribution's
  TPM2 userspace runtime libraries.
- Administrative access to the target GitHub organization or repository.
- `curl` and GnuPG (`gpg`) on the controller machine to verify and add the
  package repository.
- A checkout of this repository on an administration machine. The steps below
  use the desired-state files from `deploy/incus/`.

The production baseline remains one standalone, dedicated Incus compute host
in both connection modes. HTTPS separates the controller process from that
host; it does not add cluster placement or failover behavior.

The controller supports Incus 7.0 and newer. The bundled isolation baseline
deliberately retains a 7.0-7.2 compatibility control and rejects servers that
advertise the newer project-level VM-nesting extension until the baseline is
updated. A newer server can run the controller even when this baseline
validator rejects it; do not bypass the rejection for a production deployment.

## 1. Prepare and validate Incus

### Prepare the compute host

On a conventional Linux compute host, load bridge netfilter now and persist it
across reboots:

```sh
sudo modprobe br_netfilter
printf 'br_netfilter\n' | sudo tee /etc/modules-load.d/incus-gh-runner.conf >/dev/null
test -d /sys/module/br_netfilter
```

Treat a failed check as a host-preparation error. The API drift validator
cannot prove kernel-module state, so verify the module after provisioning and
after every host reboot. IncusOS does not expose a host shell; establish and
verify this prerequisite through the IncusOS administration surface instead
of attempting these commands on the appliance.

Start from the fail-closed desired state instead of creating an unrestricted
project and attaching the project's `default` profile. For Unix-socket mode,
the portable fixtures are ready to adapt:

```sh
# ZFS (the backward-compatible default)
cp deploy/incus/baseline.example.json incus-baseline.json

# Or LVM thin pool
cp deploy/incus/baseline.lvm.example.json incus-baseline.json
```

The repository also ships the dependency-free CUE policy under
`deploy/incus/cue/`. Its default ZFS and LVM inputs render the corresponding
JSON fixtures, derive aggregate project ceilings from host and runner
capacity, and reject attempts to weaken fixed isolation controls. It also
renders a partial controller configuration that keeps `incus.project`, the
sole Incus profile, and `capacity.max_runners` aligned with those ceilings.

For Unix-socket mode, leave `inputs.server.coreHTTPSAddress` empty. CUE then
renders `dedicated-host-unix-socket` authority and requires an empty Incus
HTTPS listener.

For HTTPS mode, do not use an unchanged portable JSON fixture. Add this
unification to the selected CUE example and replace the documentation address:

```cue
_deployment: runner.#Deployment & {
	inputs: server: coreHTTPSAddress: "192.0.2.20:8443"
}
```

Render that example from `deploy/incus/cue/`, replacing `default` with `lvm`
when applicable:

```sh
mise exec -- cue export ./examples/default -e baseline --out json \
  > ../../../incus-baseline.json
```

CUE renders `dedicated-host-https` authority and requires
`server.core_https_address` to equal that host and port. Both modes require a
standalone server, an empty cluster listener, a dedicated host, and the same
restricted workload baseline. See the
[CUE module reference](https://github.com/meigma/incus-gh-runner/tree/master/deploy/incus/cue)
for the full render contract.

When runners need a service that cannot traverse the HTTPS proxy, add a named
item to `network.additionalEgress` with its IPv4 address, `tcp` or `udp`
protocol, and one destination port. Each item adds one exact `/32` permit after
DNS and proxy. CIDR ranges, port ranges, actions, and rule state are not
configurable, and the list accepts no more than 16 endpoints. Apply host
firewall policy independently when an endpoint terminates on the managed
bridge host because the Incus ACL does not constrain host-originated traffic.

Edit the copy for the target host. Replace every documentation address, bridge
subnet, resource name, storage source, and capacity limit. For LVM, also
replace the thin-pool name and default volume size. Managed bridge names must
be 2 to 15 characters, start with a lowercase letter, and otherwise contain
only lowercase letters, digits, or hyphens. The example proxy and DNS
addresses are non-routable and intentionally provide no useful egress until
replaced. Configure a controlled proxy to allow only GitHub or GHES and the
dependency destinations approved for this builder. Do not replace the proxy
boundary with unrestricted TCP/443 and call it a GitHub allowlist.

The selected baseline fixture is reviewable desired state, not an input Incus
can apply directly. Materialize the exact project, network, ACL, profile,
storage, and server-listener state through the existing trusted Incus
administration path. The controller does not create or modify that
infrastructure. Keep the bridge and ACL host-owned in the Incus `default`
project, while the restricted runner project inherits the allowlisted bridge
and owns only its runner profile. The baseline requires a dedicated ZFS or LVM
thin pool, default-deny ACLs at the bridge and NIC, anti-spoofing and port
isolation, and both per-VM and aggregate project ceilings.

Set the project VM limit at or above the controller's
`capacity.max_runners`, then size aggregate CPU, memory, and disk for that many
profile-limited VMs while reserving explicit headroom for Incus and the compute
host. A project limit below `capacity.max_runners` makes requested capacity
impossible; limits at physical capacity do not protect the host control plane
from exhaustion. Incus project CPU and memory ceilings are admission budgets
calculated from the declared per-VM limits; they are not aggregate runtime
throttles shared dynamically by running VMs.

### Enroll the HTTPS controller identity

Skip this subsection for Unix-socket mode.

Configure the exact `core.https_address` rendered into the HTTPS baseline
through the trusted administration path. On IncusOS, use the appliance
administration surface and run all Incus CLI commands below from the existing
trusted administration workstation; there is no host shell.

Generate the controller's key and certificate on the controller machine:

```sh
umask 077
openssl req -x509 -newkey rsa:4096 -sha256 -nodes -days 365 \
  -subj '/CN=incus-gh-runner-controller' \
  -keyout client.key \
  -out client.crt
```

Transfer only `client.crt` to the trusted administration workstation. After
the `github-runners` project exists, enroll this certificate as
project-restricted from the outset:

```sh
incus config trust add-certificate <remote>: client.crt --restricted --projects github-runners
```

Replace `<remote>` with the workstation's already trusted remote for the
compute host. This follows Incus's
[direct certificate enrollment](https://linuxcontainers.org/incus/docs/main/authentication/#adding-trusted-certificates-to-the-server)
workflow. Do not first enroll the certificate without `--restricted`. An
unrestricted TLS certificate has administrative authority over Incus, while
the controller needs only its named runner project. Project restriction does
not replace the runner project's network, storage, profile, and workload
isolation controls.

Obtain the Incus server certificate through an authenticated out-of-band
channel, such as the existing trusted administration path or an authenticated
appliance console. Save it as `server.crt` and compare its SHA-256 fingerprint
with a value confirmed independently by the compute-host administrator:

```sh
openssl x509 -in server.crt -noout -sha256 -fingerprint
```

Stop on any mismatch. Do not fetch and trust the certificate with an insecure
request, disable TLS verification, or accept a first-seen certificate without
an independently trusted fingerprint. The controller performs normal TLS
verification and exact leaf-certificate pinning; the pin is not an insecure
fallback.

### Validate the effective state

Import a runner image only after the baseline validates. Any image that
implements the [guest contract](../reference/guest-contract.md) works; see
[Build a hardened runner image](./build-runner-images.md) for building and
boot-testing one. Configure only the validated `github-runner` profile; adding
the `default` or a second profile can add devices or relax limits outside the
checked baseline. The controller pins and materializes the validated profile
snapshot into each VM, so later profile edits do not alter an approved runner
environment.

For Unix-socket mode, run the validator in a trusted compute-host
administration context and name the socket explicitly:

```sh
incus-gh-runner validate \
  --socket /var/lib/incus/unix.socket \
  incus-baseline.json
```

For HTTPS mode, run it from the trusted administration workstation with a
separate operator credential:

```sh
incus-gh-runner validate \
  --url https://incus.example.com:8443 \
  --client-cert operator.crt \
  --client-key operator.key \
  --server-cert server.crt \
  incus-baseline.json
```

The validator's separate operator credential must have an administrative view
that exposes sensitive server configuration, the named runner project, the
network and ACL in the `default` project, the runner-project profile, and the
global storage pool. Hidden listener or global-resource values fail
validation. The project-restricted controller certificate cannot read every
required value. Keep the credentials separate; do not broaden the controller
certificate for validation.

The command is read-only and fails on drift. It validates the baseline against
the embedded CUE policy in process and uses only Incus GET operations; it does
not invoke external `cue`, `incus`, or `jq` executables. It does not load
controller configuration, controller environment variables, or GitHub
credentials. HTTPS validation applies the same normal TLS checks and exact
server-certificate pin as controller mode.

The validator confirms effective resource ceilings, but it cannot re-prove the
physical-host capacity and reserved headroom used when CUE generated them.
Re-render and review the baseline after changing host capacity or
reservations. Resolve every failure; do not weaken or bypass it to continue
deployment. The baseline intentionally retains its Incus 7.0-7.2 VM-nesting
compatibility gate; see
[`deploy/incus/README.md`](https://github.com/meigma/incus-gh-runner/tree/master/deploy/incus)
for the transport modes, manifest contract, controlled-egress model, and
version semantics.

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

Download the Meigma repository key and verify its full primary fingerprint:

```sh
key_file="$(mktemp)"
curl -fsSL https://pkgs.meigma.dev/meigma.asc -o "$key_file"
fingerprint="$(gpg --show-keys --with-colons "$key_file" \
  | awk -F: '$1 == "fpr" { print $10; exit }')"
test "$fingerprint" = 9C74476A669465EEB8D46AD8B0E68773B6E259F6
```

Stop if the final command fails. On Debian or Ubuntu, install the verified key,
add the APT source, and install the controller:

```sh
sudo install -d -m 0755 /etc/apt/keyrings
sudo install -m 0644 "$key_file" /etc/apt/keyrings/meigma.asc
sudo tee /etc/apt/sources.list.d/meigma.sources >/dev/null <<'EOF'
Types: deb
URIs: https://pkgs.meigma.dev/apt
Suites: stable
Components: incus-gh-runner
Signed-By: /etc/apt/keyrings/meigma.asc
EOF
sudo apt update
sudo apt install incus-gh-runner
```

On Fedora, install the published repository definition and the controller:

```sh
sudo curl -fsSL \
  https://pkgs.meigma.dev/rpm/incus-gh-runner/meigma.repo \
  -o /etc/yum.repos.d/meigma.repo
sudo dnf --refresh install incus-gh-runner
```

Remove the temporary key after either path:

```sh
rm "$key_file"
```

The package installs the binary, base unit, tmpfiles policy, editable example
configuration, license files, and credential drop-in examples without enabling
or starting the service. Packaged credential examples are under
`/usr/share/doc/incus-gh-runner/systemd/`. Later steps install exactly one
GitHub credential drop-in, the Incus HTTPS drop-in when HTTPS mode is selected,
and optionally one proof-key drop-in.

Versioned DEB and RPM files remain available from the
[GitHub releases page](https://github.com/meigma/incus-gh-runner/releases) for
direct or offline installation.

For a raw-binary installation, install the same files manually:

```sh
sudo install -m 0755 incus-gh-runner /usr/bin/incus-gh-runner
sudo install -m 0644 deploy/systemd/incus-gh-runner.service /etc/systemd/system/incus-gh-runner.service
sudo install -m 0644 deploy/systemd/incus-gh-runner.tmpfiles.conf /usr/lib/tmpfiles.d/incus-gh-runner.conf
sudo install -d -m 0755 /etc/incus-gh-runner
sudo install -m 0644 deploy/systemd/config.example.yaml /etc/incus-gh-runner/config.yaml
sudo install -d -m 0755 /usr/share/doc/incus-gh-runner/systemd
sudo install -m 0644 deploy/systemd/credentials-*.conf \
  /usr/share/doc/incus-gh-runner/systemd/
```

Run these commands on the controller machine, not the remote compute host.
The unit uses `DynamicUser=yes`, so `config.yaml` and public certificate files
must be readable by the dynamically allocated service user. Private keys and
tokens remain root-only sources exposed through systemd's protected credential
directory. The tmpfiles policy does not enable diagnostics persistence; it
expires files from the recommended diagnostics directory if you opt in later.

## 4. Write the configuration

Edit the installed `/etc/incus-gh-runner/config.yaml`. Set
`github.config_url` to the exact destination chosen in step 2 and change
`github.scale_set` from the example's `incus-linux-x64` to the label your
workflows will target. This guide uses `incus-gh-runner-prod`.

For a controller running on the compute host through the Unix socket, use an
explicit socket:

```yaml
github:
  config_url: https://github.com/OWNER/REPOSITORY
  scale_set: incus-gh-runner-prod
  runner_group: default
incus:
  socket: /var/lib/incus/unix.socket
  project: github-runners
  image: incus-gh-runner-v0.1.0
  profiles: [github-runner]
  owner: incus-gh-runner-production
capacity:
  min_runners: 1
  max_runners: 4
```

For a controller using HTTPS, replace the complete `incus` mapping with:

```yaml
incus:
  url: https://incus.example.com:8443
  client_cert_file: /etc/incus-gh-runner/client.crt
  server_cert_file: /etc/incus-gh-runner/server.crt
  project: github-runners
  image: incus-gh-runner-v0.1.0
  profiles: [github-runner]
  owner: incus-gh-runner-production
```

The HTTPS mapping is completed by the step 5 systemd drop-in, which supplies
`incus.client_key_file` through
`INCUS_GH_RUNNER_INCUS_CLIENT_KEY_FILE` without putting the private-key path in
`config.yaml`. `incus.url` must be the HTTPS API root endpoint whose host name
matches the server certificate. All three certificate files are mandatory,
and the server must present the exact pinned leaf certificate.

Exactly one of `incus.socket` and `incus.url` is required. Existing
configurations that relied on the former implicit local socket must add
`socket: /var/lib/incus/unix.socket` or their intended path before upgrading.
Configuring both modes, neither mode, partial HTTPS credentials, or HTTPS
credential fields with a socket fails startup.

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
`github` mapping. Do not add a second `github` key; exact configuration
decoding rejects duplicate keys.

```yaml
github:
  app:
    client_id: Iv1.xxxxxxxxxxxxxxxx
    installation_id: 12345678
```

When using a PAT, do not add the `app` block. The selected systemd drop-in
supplies the remaining credential path.

- `github.scale_set` names the runner scale set; the controller creates it
  automatically on first start if it does not already exist.
- `incus.image` is the alias or fingerprint of the image imported in step 1.
- `incus.owner` is an arbitrary cleanup selector exclusive to this deployment.
  Do not reuse it across independent controller instances pointed at the same
  Incus project. Another project writer can forge it, so it is not
  authorization.

See [Configuration reference](../reference/configuration.md) for every key,
default, environment variable, and credential validation rule.

## 5. Install the credential drop-ins

### Install one GitHub credential

Install exactly one GitHub credential file and its matching drop-in as
`credentials.conf`.

For a GitHub App:

```sh
sudo install -o root -g root -m 0600 github-app-private-key.pem \
  /etc/incus-gh-runner/github-app-private-key.pem
sudo install -d -m 0755 /etc/systemd/system/incus-gh-runner.service.d
sudo install -m 0644 /usr/share/doc/incus-gh-runner/systemd/credentials-github-app.conf \
  /etc/systemd/system/incus-gh-runner.service.d/credentials.conf
```

For a PAT stored in a local file named `github-token`:

```sh
sudo install -o root -g root -m 0600 github-token \
  /etc/incus-gh-runner/github-token
sudo install -d -m 0755 /etc/systemd/system/incus-gh-runner.service.d
sudo install -m 0644 /usr/share/doc/incus-gh-runner/systemd/credentials-personal-access-token.conf \
  /etc/systemd/system/incus-gh-runner.service.d/credentials.conf
```

Do not place the App private key or PAT value in `config.yaml`. The drop-ins
load the root-owned source file and point the controller at the protected
runtime copy. To change methods, replace `credentials.conf`, add or remove the
`github.app` identifiers in `config.yaml`, then reload and restart the service.

### Install the Incus HTTPS credential

Skip this subsection for Unix-socket mode.

Transfer `client.key`, `client.crt`, and the independently verified
`server.crt` to the controller machine over an authenticated channel. Install
the private-key source root-only, install the public certificates so the
DynamicUser can read them, and install the independent HTTPS drop-in:

```sh
sudo install -o root -g root -m 0600 client.key \
  /etc/incus-gh-runner/client.key
sudo install -o root -g root -m 0644 client.crt \
  /etc/incus-gh-runner/client.crt
sudo install -o root -g root -m 0644 server.crt \
  /etc/incus-gh-runner/server.crt
sudo install -d -m 0755 /etc/systemd/system/incus-gh-runner.service.d
sudo install -m 0644 /usr/share/doc/incus-gh-runner/systemd/credentials-incus-https.conf \
  /etc/systemd/system/incus-gh-runner.service.d/incus-https.conf
```

The drop-in clears the base unit's `SupplementaryGroups=incus-admin`, loads
the root-only client key with `LoadCredential=`, and sets
`INCUS_GH_RUNNER_INCUS_CLIENT_KEY_FILE` to the protected runtime file. The
HTTPS controller therefore needs no local Incus CLI, socket, or `incus-admin`
group. This drop-in composes with either GitHub credential and either optional
job-proof drop-in.

The controller reads and parses the client key and both certificates once
during its bounded startup connection. Replace the source files atomically and
restart the service after any certificate or key rotation; a running process
does not reload them.

## 6. Enable job proofs (optional)

Job proofs bind each GitHub Actions job to the Incus VM that ran it; the
[job proofs reference](../reference/job-proofs.md) documents the proof
envelope, payload schema, and key-ID rule. Generate and enroll the host's
Ed25519 proof key, then choose one proof-key storage mode. Both modes expose
the same runtime credential to the unchanged controller and compose with
either GitHub credential drop-in and either Incus connection mode.

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
sudo install -m 0644 /usr/share/doc/incus-gh-runner/systemd/credentials-job-proof-file.conf \
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
sudo install -m 0644 /usr/share/doc/incus-gh-runner/systemd/credentials-job-proof-tpm.conf \
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

For TPM-bound job proofs, reboot the controller machine normally and confirm
the service starts again before accepting the deployment. Retrieve a proof in
a real job,
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
