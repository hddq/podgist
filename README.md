# Podgist

Podgist is a lightweight, self-hosted backend that implements the [gpodder.net](https://gpodder.net/) API. It allows you to sync your podcast subscriptions, episode playback progress, and devices across multiple clients (like [AntennaPod](https://github.com/AntennaPod/AntennaPod)) without relying on the official servers.

> [!WARNING]
> This project is currently **Work In Progress**. The core sync API should work fine, however, the web dashboard is still early and lacks features

## Docker Images

Docker images are automatically built via GitHub Actions and published to the GitHub Container Registry (GHCR).

### Tags

**Stable**: `ghcr.io/hddq/podgist:latest`, `:vX.Y.Z` (e.g., `v1.0.0`), `:vX.Y`, `:vX` - Recommended for normal use.

**Edge**: `ghcr.io/hddq/podgist:edge` - The bleeding-edge version, built automatically from the latest commit on the `master` branch.

## How to run it

The easiest way to get Podgist up and running is by using Docker Compose.

1. **Download config and compose:**
```bash
curl -O https://raw.githubusercontent.com/hddq/podgist/master/compose.yaml
curl -O https://raw.githubusercontent.com/hddq/podgist/master/config.example.yaml
cp config.example.yaml config.yaml
```

2. **Start containers:**
```bash
docker compose up -d
```

3. **Register first user:**
Open http://localhost:8080/app in your web browser and create account

The API should now be accessible at `http://localhost:8080`. Point your podcast client's sync settings to this URL!

## Development

If you use Nix/NixOS, a `flake.nix` is included. Just run `nix develop` to get a ready-to-use Go development environment with all necessary dependencies.

For local non-Docker builds, the embedded web UI assets must be built first:

```bash
./scripts/build-web.sh
go build ./cmd/api
```
