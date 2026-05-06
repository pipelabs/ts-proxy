# Release Process

This project uses automated releases via GitHub Actions. Every push to the `main` branch will automatically:

1. Determine the next semantic version.
2. Run tests.
3. Build binaries for multiple platforms.
4. Create a git tag.
5. Create a GitHub release with artifacts.

## Semantic Versioning

The workflow determines the next version from [Conventional Commits](https://www.conventionalcommits.org/).

### Major Version

Commit messages starting with:

- `BREAKING CHANGE:` in the body
- `feat!:` or `fix!:`

Example:

```bash
git commit -m "feat!: change mapping syntax

BREAKING CHANGE: TS_PROXY_MAPPINGS now uses a different format"
```

### Minor Version

Commit messages starting with `feat:` or `feat(scope):`.

Example:

```bash
git commit -m "feat: add UDP forwarding support"
git commit -m "feat(config): support JSON mapping files"
```

### Patch Version

All other commits, including:

- `fix:` bug fixes
- `docs:` documentation changes
- `chore:` maintenance tasks
- `refactor:` code refactoring

Example:

```bash
git commit -m "fix: close target connection after client disconnect"
git commit -m "docs: update deployment examples"
```

## Build Artifacts

Each release includes pre-built binaries for:

- Linux AMD64: `ts-proxy-vX.Y.Z-linux-amd64.tar.gz`
- macOS ARM64: `ts-proxy-vX.Y.Z-mac-arm64.tar.gz`
- macOS AMD64: `ts-proxy-vX.Y.Z-mac-amd64.tar.gz`
- Windows AMD64: `ts-proxy-vX.Y.Z-windows-amd64.zip`
- Checksums: `checksums-vX.Y.Z.txt`

## Version Information

Release binaries are built with embedded version information:

- Version: semantic version tag, for example `v1.2.3`
- Build Time: UTC timestamp
- Git Commit: short commit hash
- Module Name: Go module name

The binary prints this information on startup:

```text
ts-proxy
Version: v1.2.3
Build Time: 2026-05-06_01:23:45
Git Commit: abc1234
Module: github.com/pipelabs/ts-proxy
```

## Manual Release

To create a manual release:

```bash
git tag -a v1.2.3 -m "Release v1.2.3"
git push origin v1.2.3
```

To build locally using the same ldflags pattern:

```bash
./build.sh
```

This creates binaries in the `dist/` directory with version information from git.

## First Release

If no tags exist yet, the workflow starts from `v0.0.0` and increments to `v0.0.1` or higher based on commit messages.

## Viewing Releases

Releases are available at:

```text
https://github.com/pipelabs/ts-proxy/releases
```

## Troubleshooting

### Workflow Not Running

- Ensure you're pushing to the `main` branch.
- Check GitHub Actions are enabled in repository settings.
- Verify the workflow file is at `.github/workflows/release.yml`.

### Version Not Incrementing

- The workflow reads commits between the current HEAD and the latest tag.
- If there are no new commits since the latest tag, push at least one new commit.
- Check that commit messages follow Conventional Commit format.

### Build Failures

- Run `go test ./...` locally.
- Run `./build.sh` locally.
- Check for platform-specific compile errors in GitHub Actions logs.
