# Releasing

This package (`github.com/oblodai/oblodai-go/v2`) is published to **Go modules** by a `v*` tag.

## Before a release

1. Regenerate from the backend (`make sdk` there) and commit `zz_generated_*.go` and `names.lock`.
2. `OBLODAI_BACKEND=/path/to/oblodai-backend make ci` — every gate green, locally.
3. Bump `Version` in `oblodai.go` and add the `CHANGELOG.md` entry (`make package` checks both).

## Cut a release

`git tag vX.Y.Z && git push origin vX.Y.Z`. The version is immediately resolvable via
`go get github.com/oblodai/oblodai-go/v2@vX.Y.Z`. No secret is needed.
