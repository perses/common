GO      ?= go
GOFMT   ?= $(GO)fmt
pkgs     = $$($(GO) list ./...)

.PHONY: checkformat
checkformat:
	@echo ">> checking code format"
	! $(GOFMT) -d $$(find . -name '*.go' -print) | grep '^' ;\

fmt:
	@echo ">> format code"
	$(GO) fmt $(pkgs)

test:
	@echo ">> running all tests"
	$(GO) test -race -count=1 -v $(pkgs)
	@echo ">> running the echo tests in FIPS 140-3 mode"
	GODEBUG=fips140=on $(GO) test -race -count=1 ./echo/...
