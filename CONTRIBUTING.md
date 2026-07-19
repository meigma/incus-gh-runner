# Contributing

Thank you for contributing to `incus-gh-runner`. Keep changes small and
focused, and be willing to revise the design as you learn more.

For private vulnerability reporting, use [SECURITY.md](SECURITY.md) instead of
public channels.

## Pull requests

Contributors should:

1. Keep each pull request focused on a single change.
2. Add or update tests when observable behavior changes.
3. Update documentation when user-facing behavior changes.
4. Use Conventional Commit subjects, such as `feat(controller): reconcile demand`.
5. Run `moon run root:check` before requesting review.

## Local setup

```sh
mise install
moon run root:check
```

Useful focused commands are:

```sh
moon run root:format
moon run root:lint
moon run root:build
moon run root:test
go run ./cmd/incus-gh-runner --version
```

Release Please uses Conventional Commit subjects to prepare release notes and
version updates. Integration happens through squash-merged GitHub pull requests.
