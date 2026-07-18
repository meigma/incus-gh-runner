---
title: systemd deployment
description: Run incus-gh-runner as a hardened foreground systemd service.
---

# systemd deployment

The checked-in unit runs one controller as a foreground `Type=simple` service.
It uses a dynamic service identity, a root-loaded systemd credential for the
GitHub App private key, a private persistent diagnostics directory, bounded
shutdown, and `Restart=on-failure` recovery.

`deploy/systemd/verify.sh` validates the unit on Linux with
`systemd-analyze verify` and rejects a systemd security exposure score above
5.0. The reference validation uses Ubuntu 24.04 and currently scores 2.9.

## Host boundary

The v1 controller connects through the local Incus Unix socket. The unit joins
the `incus-admin` group because the standard daemon socket requires it. Incus
documents that this socket grants full daemon control and should be treated as
root-equivalent host access. The controller's exact ownership marker still
prevents accidental mutation outside its configured runner set, but it is not a
security boundary against compromise of the controller process.

Use a dedicated runner host. Do not install this service on a host where the
controller must be isolated from unrelated Incus workloads. Project-restricted
remote TLS authorization is the intended future path for reducing this host
privilege; v1 does not implement that transport.

## Install

Install the binary, unit, configuration, and GitHub App private key as root:

```sh
sudo install -D -m 0755 incus-gh-runner /usr/bin/incus-gh-runner
sudo install -D -m 0644 deploy/systemd/incus-gh-runner.service \
  /usr/lib/systemd/system/incus-gh-runner.service
sudo install -d -m 0755 /etc/incus-gh-runner
sudo install -m 0644 deploy/systemd/config.example.yaml \
  /etc/incus-gh-runner/config.yaml
sudo install -m 0600 /path/to/github-app-private-key.pem \
  /etc/incus-gh-runner/github-app-private-key.pem
```

Edit `config.yaml` for the prepared Incus project, image, profiles, exact owner
marker, capacity, and GitHub App installation. The private-key path is supplied
by the unit through `LoadCredential=` and must not be added to the YAML file.

The unit uses `DynamicUser=yes` and creates `/var/log/incus-gh-runner` with mode
`0700`. Configure `incus.diagnostics_dir` below that path so terminal runner
evidence remains writable without granting broader filesystem access.

## Start and inspect

Validate and start the installed unit:

```sh
sudo systemd-analyze verify /usr/lib/systemd/system/incus-gh-runner.service
sudo systemctl daemon-reload
sudo systemctl enable --now incus-gh-runner.service
sudo systemctl status incus-gh-runner.service
sudo journalctl -u incus-gh-runner.service --follow
```

Startup fails before the service reports active work if configuration, GitHub
authentication, the initial message session, or Incus preflight is invalid.
After successful startup, transient GitHub listener failures recover inside the
process; fatal or wedged failures exit non-zero and are restarted by systemd.

## Stop behavior

`systemctl stop incus-gh-runner` sends SIGTERM. The controller stops polling and
scheduling, gives active Incus operations their bounded shutdown windows, and
does not delete busy runner VMs merely because the service is stopping. A later
start reconstructs owned capacity from Incus metadata.

The default application budget is twice `timeouts.shutdown` (60 seconds in the
example). `TimeoutStopSec=70s` leaves a supervisor margin before systemd resorts
to SIGKILL. Keep the unit timeout greater than twice the configured shutdown
value when overriding either setting.
