# Sub2API

Sub2API is an independently maintained AI API gateway for routing, managing, and operating multiple AI upstream accounts behind OpenAI-compatible and Anthropic-compatible APIs.

This branch keeps the deployable name `sub2api` for compatibility, but its releases are owned by this repository and start from `v1.0.0`.

## Independent Maintenance

This repository is maintained independently from upstream `Wei-Shaw/sub2api`.

- We do not automatically rebase or merge upstream `main`.
- Upstream pull requests are reviewed individually before being accepted.
- Local releases use independent SemVer tags starting at `v1.0.0`.
- Existing binary, service, and image basenames remain `sub2api` so current deployments do not need a rename.

## Relationship to Upstream

Sub2API originated from `Wei-Shaw/sub2api`, but this branch is no longer an upstream-tracking branch. Upstream changes are treated as review candidates. A change is merged only when it matches this branch's architecture, operating assumptions, and user-facing behavior.

## Core Capabilities

- Multi-upstream AI API gateway for subscription and API-key based accounts.
- OpenAI-compatible, Anthropic-compatible, and Claude Code oriented request handling.
- Account selection, failover, concurrency control, and rate-limit cooldown handling.
- Web administration UI for accounts, groups, keys, models, and operational settings.
- Docker Compose and binary/systemd deployment paths.
- Release artifacts built under the compatible `sub2api` name.

## Installation

### Docker Compose

The recommended container image is the GHCR image for this repository:

```bash
docker pull ghcr.io/is7qin/sub2api:latest
```

For a prepared Docker Compose deployment:

```bash
curl -sSL https://raw.githubusercontent.com/is7Qin/sub2api/main/deploy/docker-deploy.sh | bash
docker compose -f docker-compose.local.yml up -d
```

### Binary Install on Linux

```bash
curl -sSL https://raw.githubusercontent.com/is7Qin/sub2api/main/deploy/install.sh | sudo bash
```

The installer downloads release assets from `is7Qin/sub2api` and installs the `sub2api` binary and systemd service.

### Manual Download

Download archives from:

```text
https://github.com/is7Qin/sub2api/releases
```

Choose the archive matching your OS and CPU architecture, then follow the deployment notes in `deploy/README.md`.

## Configuration

Start from the deployment examples under `deploy/`:

- `deploy/config.example.yaml` for binary/systemd configuration.
- `deploy/docker-compose.local.yml` for Docker Compose with local data directories.
- `deploy/docker-compose.yml` for Docker Compose with named volumes.
- `deploy/docker-compose.standalone.yml` when PostgreSQL and Redis are provided separately.

For Docker deployments, set secrets and database credentials in `.env` before starting the service.

## Upgrade and Release Policy

- First independent release: `v1.0.0`.
- Patch releases (`v1.0.x`) contain small fixes, documentation updates, and reviewed upstream PR cherry-picks.
- Minor releases (`v1.x.0`) contain new features or notable behavior changes.
- Major releases (`v2.0.0+`) are reserved for breaking configuration, deployment, API, or data-model changes.

The runtime `VERSION` value is generated from the tag by the release workflow. For example, tag `v1.0.0` produces runtime version `1.0.0`.

## Upstream PR Review Policy

When an upstream PR is considered, review includes:

1. Intent and user impact.
2. Implementation risk and architectural fit.
3. Tests and operational behavior.
4. Interaction with local changes.
5. Known edge cases and accepted limitations.

PRs are merged or cherry-picked only after explicit approval for that PR.

## Disclaimer

This is an independently maintained branch. It is not an official upstream release and is not endorsed by upstream maintainers. Use it according to your own operational and compliance requirements.
