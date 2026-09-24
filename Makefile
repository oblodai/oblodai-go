# Every gate CI runs, in the order that fails fastest: `make ci`.
#
# The drift check and the shared conformance suite need the backend checkout: OBLODAI_BACKEND,
# else ../oblodai-backend next to this repository. The SDK builds with the local Go (≥ 1.25); the
# linter runs its analysis against LINT_GOTOOLCHAIN, because its staticcheck cannot read a newer
# standard library yet.
export GOTOOLCHAIN ?= local
export OBLODAI_BACKEND ?= $(abspath $(CURDIR)/../oblodai-backend)
LINT_GOTOOLCHAIN ?= go1.26.6
GO ?= go

.PHONY: ci fmt vet lint build drift test conformance package live

ci: fmt vet lint build drift test conformance package
	@echo "all gates green"

fmt:           ## gofmt must have nothing to say
	@out="$$(gofmt -l .)"; if [ -n "$$out" ]; then echo "gofmt needed:"; echo "$$out"; exit 1; fi

vet:
	$(GO) vet ./...

lint:
	@command -v golangci-lint >/dev/null 2>&1 || { echo "missing golangci-lint: https://golangci-lint.run" >&2; exit 1; }
	GOTOOLCHAIN=$(LINT_GOTOOLCHAIN) golangci-lint run ./...

build:
	$(GO) build ./...

drift:         ## zz_generated_*.go must be what the backend's generator makes of its contract
	./scripts/check-generated.sh --require

test:          ## unit, contract, examples, README code — hermetic, with the race detector
	$(GO) test -race -count=1 ./...

conformance:   ## the backend's shared scenarios; a missing suite fails
	$(GO) test -count=1 -run Conformance . ./webhooks

package:       ## what `go get` downloads builds on its own
	./scripts/check-package.sh

live:          ## the live tier against a real core (needs OBLODAI_LIVE_URL)
	$(GO) test -count=1 -run TestLive -v .
