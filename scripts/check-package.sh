#!/usr/bin/env bash
# The packaging gate: what `go get github.com/oblodai/oblodai-go/v2` would download builds on its
# own. Copies the files the module ships (tracked and new, not ignored) into a clean directory,
# builds and vets it offline, and checks go.mod is tidy, carries no replace directive and that the
# release files are there.
set -euo pipefail
root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"

version="$(sed -n 's/^const Version = "\(.*\)"$/\1/p' oblodai.go)"
if ! grep -q "^## \[$version\]" CHANGELOG.md; then
  echo "check-package: CHANGELOG.md has no [$version] entry" >&2
  exit 1
fi
for f in go.mod LICENSE README.md README.ru.md AGENTS.md CHANGELOG.md MIGRATION-2.0.md names.lock; do
  [ -f "$f" ] || { echo "check-package: $f is missing" >&2; exit 1; }
done
if grep -q '^replace' go.mod; then
  echo "check-package: go.mod carries a replace directive" >&2
  exit 1
fi
go mod tidy -diff

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
git ls-files -co --exclude-standard | while IFS= read -r f; do
  if [ -f "$f" ]; then
    mkdir -p "$tmp/$(dirname "$f")"
    cp "$f" "$tmp/$f"
  fi
done
(cd "$tmp" && GOFLAGS=-mod=mod GOPROXY=off GOWORK=off go build ./... && GOFLAGS=-mod=mod GOPROXY=off GOWORK=off go vet ./...)
echo "package: github.com/oblodai/oblodai-go/v2 $version builds from its own files"
