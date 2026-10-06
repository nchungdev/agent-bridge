# Versioning and releases

A build is a **release tag pushed on purpose**. Pushing code to `main` runs the tests and builds nothing.

| Tag | Means |
|---|---|
| `v1.0.1` | version 1.0.1, build 01 |
| `v1.0.1-b02` | a hotfix build of 1.0.1, build 02 |
| `v1.0.1-b99` | the last build of 1.0.1 |
| `v1.0.2` | the next patch version, build 01 again |

Build numbers run from 02 to 99 after the plain tag. After build 99 the next patch version must be tagged. Other
tag shapes (`v1.0.1-rc1`, `-b00`, `-b100`) are rejected by the release workflow.

The app shows the **newest release reachable from the code it was built from**, for example
`v1.0.1 (build 02)`, in **Settings → Updates**. A release counts as newer by version first, then by build.

## Cutting a release

```sh
git tag -a v1.0.1 -m v1.0.1 && git push origin v1.0.1
```

The `Release` workflow then builds the binaries (Linux and macOS, amd64 and arm64, with the version embedded),
publishes them with their `.sha256` files on the Releases page, and pushes the multi-arch image to
`ghcr.io/nchungdev/agent-bridge` (tags `1.0.1`, `1.0` for plain releases, and `latest`).

`Publish image` is a manual workflow that rebuilds only `:latest` from `main`.

## How installed copies update

A release build knows its own version. `agent-bridge update` (and Settings → Updates) asks GitHub Releases for the
newest release with a build for this platform, downloads the archive and its checksum, verifies the SHA-256,
replaces the binary and restarts the service. The previous binary is kept next to it, so
`agent-bridge update --rollback` can undo the change. A copy run from a git checkout (a development build)
updates with git instead.
