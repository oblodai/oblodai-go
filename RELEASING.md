# Releasing

This package (`github.com/oblodai/oblodai-go`) is published to **Go modules** by CI when a `v*` tag is pushed.

## Setup (one-time)
**No secret needed.** Go modules are published by tag; consumers fetch via `go get`. (The repo is currently **private** — consumers must set `GOPRIVATE=github.com/oblodai/*` and have access.)

## Cut a release
1. `git tag vX.Y.Z && git push origin vX.Y.Z`.
2. The **Release** workflow runs tests and creates a GitHub Release. The version is immediately resolvable via `go get github.com/oblodai/oblodai-go@vX.Y.Z`.

CI (build + tests) runs on every push and pull request via `.github/workflows/ci.yml`.
