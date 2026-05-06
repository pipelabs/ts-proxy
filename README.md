# ts-proxy

A tiny, self-contained TCP proxy that embeds Tailscale's `tsnet`. It joins a Tailnet as its own Tailscale node, listens on one or more Tailnet ports plus matching local ports, and forwards each connection to a configured service inside the Tailnet.

## How It Works

`ts-proxy` creates a Tailscale node using `tsnet`, then starts one Tailscale TCP listener and one local TCP listener for each configured mapping.

```text
[Client in Tailnet] -> [ts-proxy:5432] -> [postgres.tailnet.ts.net:5432]
[Local machine]     -> [localhost:5432] -> [postgres.tailnet.ts.net:5432]
[Client in Tailnet] -> [ts-proxy:8080] -> [100.64.0.10:80]
```

The proxy dials targets through `tsnet`, so target hosts can be Tailnet DNS names, MagicDNS names, or Tailscale IPs.

## Configuration

Configuration is via environment variables:

- `TS_PROXY_MAPPINGS` is required. Use `listenPort=targetHost:targetPort` entries separated by commas, semicolons, or newlines.
- `TS_PROXY_HOSTNAME` controls the Tailscale node name. Defaults to `ts-proxy`.
- `TS_PROXY_STATE_DIR` controls where tsnet stores its node identity. Defaults to `./tsnet-state`.
- `TS_PROXY_LOCAL_ADDR` controls the local bind address for matching local listeners. Defaults to `127.0.0.1`; use `0.0.0.0` to expose on all local network interfaces.
- `TS_PROXY_ACCEPT_ROUTES` controls whether the proxy accepts subnet routes advertised by other Tailnet nodes. Defaults to `true`.
- `TS_PROXY_AUTH_KEY` or `TS_AUTHKEY` is optional. This can be a Tailscale auth key or OAuth client secret. When omitted, first run prints a Tailscale login URL.
- `TS_PROXY_ADVERTISE_TAGS` is optional. Use comma, semicolon, or newline-separated tags such as `tag:ci,tag:proxy`.
- `TS_PROXY_AUTH_EPHEMERAL` is optional. When set, appends the OAuth auth-key parameter `ephemeral=true|false`.
- `TS_PROXY_AUTH_PREAUTHORIZED` is optional. When set, appends the OAuth auth-key parameter `preauthorized=true|false`.
- `TS_PROXY_AUTH_BASE_URL` is optional. When set, appends the OAuth auth-key parameter `baseURL=...`.

Example:

```bash
export TS_PROXY_HOSTNAME=client-services-proxy
export TS_PROXY_MAPPINGS="5432=postgres.tailnet.ts.net:5432,8080=100.64.0.10:80"
go run .
```

Then connect from another Tailnet machine:

```bash
psql "postgres://user:pass@client-services-proxy:5432/db"
curl http://client-services-proxy:8080
```

Or connect locally on the machine running the proxy:

```bash
psql "postgres://user:pass@localhost:5432/db"
curl http://localhost:8080
```

For unattended deployments, create a reusable or ephemeral auth key in Tailscale and set:

```bash
export TS_PROXY_AUTH_KEY=tskey-auth-...
```

OAuth client secrets are also supported. The OAuth client must have the `auth_keys` scope and must be allowed to advertise the tag or tags passed in `TS_PROXY_ADVERTISE_TAGS`:

```bash
export TS_PROXY_AUTH_KEY=tskey-client-...
export TS_PROXY_ADVERTISE_TAGS=tag:ci
go run .
```

You can pass OAuth auth-key parameters directly:

```bash
export TS_PROXY_AUTH_KEY="tskey-client-...?ephemeral=false&preauthorized=true"
export TS_PROXY_ADVERTISE_TAGS=tag:ci
go run .
```

Or let `ts-proxy` append them from env vars:

```bash
export TS_PROXY_AUTH_KEY=tskey-client-...
export TS_PROXY_ADVERTISE_TAGS=tag:ci
export TS_PROXY_AUTH_EPHEMERAL=false
export TS_PROXY_AUTH_PREAUTHORIZED=true
go run .
```

Route acceptance is enabled by default so targets behind approved subnet routers are reachable. To disable it:

```bash
export TS_PROXY_ACCEPT_ROUTES=false
```

## Installation

### Download Pre-built Binaries

Pre-built binaries are created for each release:

```bash
# Linux
wget https://github.com/pipelabs/ts-proxy/releases/latest/download/ts-proxy-VERSION-linux-amd64.tar.gz
tar -xzf ts-proxy-VERSION-linux-amd64.tar.gz

# macOS Apple Silicon
wget https://github.com/pipelabs/ts-proxy/releases/latest/download/ts-proxy-VERSION-mac-arm64.tar.gz
tar -xzf ts-proxy-VERSION-mac-arm64.tar.gz

# macOS Intel
wget https://github.com/pipelabs/ts-proxy/releases/latest/download/ts-proxy-VERSION-mac-amd64.tar.gz
tar -xzf ts-proxy-VERSION-mac-amd64.tar.gz
```

### Build From Source

```bash
# Current platform
go build -o ts-proxy .

# Release-style binaries in dist/
./build.sh
```

## Deployment

1. Copy the binary to the machine that will run the proxy.
2. Set `TS_PROXY_HOSTNAME` and `TS_PROXY_MAPPINGS`.
3. Optionally set `TS_AUTHKEY` for non-interactive authentication.
4. Run `./ts-proxy`.

On first run without `TS_AUTHKEY`, the process prints a Tailscale authentication URL.

The `tsnet-state` directory should be persisted across restarts so the proxy keeps the same Tailscale identity.

## Mapping Examples

PostgreSQL:

```bash
export TS_PROXY_MAPPINGS="5432=postgres.tailnet.ts.net:5432"
```

Multiple services:

```bash
export TS_PROXY_MAPPINGS="5432=postgres.tailnet.ts.net:5432,6379=redis.tailnet.ts.net:6379,8080=100.64.0.10:80"
```

Newline-separated mappings:

```bash
export TS_PROXY_MAPPINGS="
5432=postgres.tailnet.ts.net:5432
6379=redis.tailnet.ts.net:6379
"
```

## Build Artifacts

`./build.sh` creates:

- `dist/ts-proxy-{version}-linux-amd64`
- `dist/ts-proxy-{version}-mac-arm64`
- `dist/ts-proxy-{version}-mac-amd64`
- `dist/ts-proxy-{version}-windows-amd64.exe`

Release builds embed version information that is printed on startup:

```text
ts-proxy
Version: v1.2.3
Build Time: 2026-05-06_01:23:45
Git Commit: abc1234
Module: github.com/pipelabs/ts-proxy
```

## Development

```bash
go test ./...
go build ./...
```

## Troubleshooting

### Authentication Issues

- Check the login URL printed by the process.
- Verify `TS_AUTHKEY` is valid if running unattended.
- Make sure the Tailscale account has room for another node.

### Connection Refused

- Confirm the mapping listen port is present in `TS_PROXY_MAPPINGS`.
- Confirm the target service is reachable from inside the Tailnet.
- Check Tailscale ACLs allow the client to reach the proxy port.

### Hostname Changed

Persist `TS_PROXY_STATE_DIR` between restarts. Deleting the state directory creates a new tsnet identity.

## Release Process

Releases are automated via GitHub Actions on pushes to `main`. See `RELEASING.md` for details.
