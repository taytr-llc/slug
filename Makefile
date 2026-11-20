PKG := github.com/taytr-llc/slug

# Temporary candidate engineering profile; its qualification is separate evidence.
# Explicit compatibility mode preserves the minimum-Go CI job. Plain go test is
# unchanged and does not implicitly use this profile.
GO_PROFILE ?= candidate
ifeq ($(GO_PROFILE),candidate)
PROFILE_GO := sh "$(abspath scripts/candidate-go.sh)"
GO_ENV := env GOENV=off GOTOOLCHAIN=local GOFLAGS= GOEXPERIMENT= go env
VERIFY_BINARY := $(PROFILE_GO) verify-binary
else ifeq ($(GO_PROFILE),compatibility)
ifneq ($(filter install release-install,$(MAKECMDGOALS)),)
$(error Compatibility mode cannot install shipping binaries)
endif
PROFILE_GO := go
GO_ENV := go env
VERIFY_BINARY := true
else
$(error GO_PROFILE must be candidate or compatibility)
endif

LINT_PKG := github.com/golangci/golangci-lint/v2/cmd/golangci-lint
GOIMPORTS_PKG := golang.org/x/tools/cmd/goimports

GO_BIN := $(if $(GOBIN),$(GOBIN),$(shell $(GO_ENV) GOBIN))
GO_EXE := $(shell GOOS="$(GOOS)" $(GO_ENV) GOEXE)

# If GOBIN is not set, default to GOPATH/bin.
ifeq ($(GO_BIN),)
GO_BIN := $(shell $(GO_ENV) GOPATH)/bin
endif

LINT_BIN := $(GO_BIN)/golangci-lint
GOIMPORTS_BIN := $(GO_BIN)/goimports

LINT_COMMIT := v2.14.0
GOIMPORTS_COMMIT := v0.51.0

GOBUILD := $(PROFILE_GO) build -v
GOINSTALL := $(PROFILE_GO) install -v
DEV_TAGS := rpctest
GOTEST_DEV = $(PROFILE_GO) test -v -tags=$(DEV_TAGS)
GOTEST := $(PROFILE_GO) test -v
# CI runs the complete chain/integration suites separately with test-only PoW.
# Local targets retain their historical default unless explicitly selected.
ROOT_UNIT_PACKAGES ?= ./...
CHAIN_TEST_FLAGS ?=
COVER_FLAGS = -coverprofile=coverage.txt -covermode=atomic -coverpkg=$(PKG)/...

# Linting uses a lot of memory, so keep it under control by limiting the number
# of workers if requested.
ifneq ($(workers),)
LINT_WORKERS = --concurrency=$(workers)
endif
LINT_TIMEOUT := 5m

LINT = $(LINT_BIN) run -v $(LINT_WORKERS) --timeout=$(LINT_TIMEOUT)

GREEN := "\\033[0;32m"
NC := "\\033[0m"
define print
	echo $(GREEN)$1$(NC)
endef

#? default: Run `make build`
default: build

#? all: Run `make build` and `make check`
all: build check

# ============
# DEPENDENCIES
# ============

$(LINT_BIN):
	@$(call print, "Fetching linter")
	$(GOINSTALL) $(LINT_PKG)@$(LINT_COMMIT)

#? goimports: Install goimports
goimports:
	@$(call print, "Installing goimports.")
	$(GOINSTALL) $(GOIMPORTS_PKG)@$(GOIMPORTS_COMMIT)

# ============
# INSTALLATION
# ============

#? build: Build all binaries, place them in project directory
build:
	@$(call print, "Building all binaries")
	$(GOBUILD) -o slugd$(GO_EXE) $(PKG)
	$(GOBUILD) $(PKG)/cmd/slugctl
	$(GOBUILD) $(PKG)/cmd/slugcheckpoint
	$(GOBUILD) $(PKG)/cmd/gencerts
	$(GOBUILD) $(PKG)/cmd/findcheckpoint
	$(GOBUILD) $(PKG)/cmd/addblock
	$(VERIFY_BINARY) slugd$(GO_EXE) slugctl$(GO_EXE) slugcheckpoint$(GO_EXE) gencerts$(GO_EXE) findcheckpoint$(GO_EXE) addblock$(GO_EXE)

#? install: Install all binaries, place them in $GOPATH/bin
install:
	@$(call print, "Installing all binaries")
	$(GOBUILD) -o "$(GO_BIN)/slugd$(GO_EXE)" $(PKG)
	$(GOINSTALL) $(PKG)/cmd/slugctl
	$(GOINSTALL) $(PKG)/cmd/slugcheckpoint
	$(GOINSTALL) $(PKG)/cmd/gencerts
	$(GOINSTALL) $(PKG)/cmd/findcheckpoint
	$(GOINSTALL) $(PKG)/cmd/addblock
	$(VERIFY_BINARY) "$(GO_BIN)/slugd$(GO_EXE)" "$(GO_BIN)/slugctl$(GO_EXE)" "$(GO_BIN)/slugcheckpoint$(GO_EXE)" "$(GO_BIN)/gencerts$(GO_EXE)" "$(GO_BIN)/findcheckpoint$(GO_EXE)" "$(GO_BIN)/addblock$(GO_EXE)"

#? release-install: Install slugd and slugctl release binaries, place them in $GOBIN
release-install:
	@$(call print, "Installing slugd and slugctl release binaries")
	env CGO_ENABLED=0 $(GOBUILD) -trimpath -ldflags="-s -w -buildid=" -o "$(GO_BIN)/slugd$(GO_EXE)" $(PKG)
	env CGO_ENABLED=0 $(GOINSTALL) -trimpath -ldflags="-s -w -buildid=" $(PKG)/cmd/slugctl
	$(VERIFY_BINARY) "$(GO_BIN)/slugd$(GO_EXE)" "$(GO_BIN)/slugctl$(GO_EXE)"

# =======
# TESTING
# =======

#? check: Run `make unit`
check: unit

#? test-chain: Run complete blockchain and integration suites with test-only PoW
test-chain:
	$(PROFILE_GO) test $(CHAIN_TEST_FLAGS) -tags=slugtestpow -count=1 -timeout=30m ./blockchain/...
	$(PROFILE_GO) test $(CHAIN_TEST_FLAGS) -tags=slugtestpow,rpctest -count=1 -timeout=30m ./integration/...

#? unit: Run unit tests
unit:
	@$(call print, "Running unit tests.")
	$(GOTEST_DEV) $(ROOT_UNIT_PACKAGES) -test.timeout=20m
	cd address && $(GOTEST_DEV) ./... -test.timeout=20m
	cd btcec && $(GOTEST_DEV) ./... -test.timeout=20m
	cd btcutil && $(GOTEST_DEV) ./... -test.timeout=20m
	cd chaincfg && $(GOTEST_DEV) ./... -test.timeout=20m
	cd chainhash && $(GOTEST_DEV) ./... -test.timeout=20m
	cd txscript && $(GOTEST_DEV) ./... -test.timeout=20m
	cd psbt && $(GOTEST_DEV) ./... -test.timeout=20m
	cd wire && $(GOTEST_DEV) ./... -test.timeout=20m
	cd v2transport && $(GOTEST_DEV) ./... -test.timeout=20m

#? unit-cover: Run unit coverage tests
unit-cover:
	@$(call print, "Running unit coverage tests.")
	$(GOTEST) $(COVER_FLAGS) $(ROOT_UNIT_PACKAGES)

	# We need to remove the /v2 pathing from the module to have it work
	# nicely with the CI tool we use to render live code coverage.
	cd address && $(GOTEST) $(COVER_FLAGS) ./... && sed -i.bak 's/v2\///g' coverage.txt
	cd btcec && $(GOTEST) $(COVER_FLAGS) ./... && sed -i.bak 's/v2\///g' coverage.txt
	cd btcutil && $(GOTEST) $(COVER_FLAGS) ./... && sed -i.bak 's/v2\///g' coverage.txt
	cd chaincfg && $(GOTEST) $(COVER_FLAGS) ./... && sed -i.bak 's/v2\///g' coverage.txt
	cd chainhash && $(GOTEST) $(COVER_FLAGS) ./... && sed -i.bak 's/v2\///g' coverage.txt
	cd txscript && $(GOTEST) $(COVER_FLAGS) ./... && sed -i.bak 's/v2\///g' coverage.txt
	cd psbt && $(GOTEST) $(COVER_FLAGS) ./... && sed -i.bak 's/v2\///g' coverage.txt
	cd wire && $(GOTEST) $(COVER_FLAGS) ./... && sed -i.bak 's/v2\///g' coverage.txt
	cd v2transport && $(GOTEST) $(COVER_FLAGS) ./... && sed -i.bak 's/v2\///g' coverage.txt

#? unit-race: Run unit race tests
unit-race:
	@$(call print, "Running unit race tests.")
	env CGO_ENABLED=1 GORACE="history_size=7 halt_on_errors=1" $(GOTEST) -race -test.timeout=20m $(ROOT_UNIT_PACKAGES)
	cd address && env CGO_ENABLED=1 GORACE="history_size=7 halt_on_errors=1" $(GOTEST) -race -test.timeout=20m ./...
	cd btcec && env CGO_ENABLED=1 GORACE="history_size=7 halt_on_errors=1" $(GOTEST) -race -test.timeout=20m ./...
	cd btcutil && env CGO_ENABLED=1 GORACE="history_size=7 halt_on_errors=1" $(GOTEST) -race -test.timeout=20m ./...
	cd chaincfg && env CGO_ENABLED=1 GORACE="history_size=7 halt_on_errors=1" $(GOTEST) -race -test.timeout=20m ./...
	cd chainhash && env CGO_ENABLED=1 GORACE="history_size=7 halt_on_errors=1" $(GOTEST) -race -test.timeout=20m ./...
	cd txscript && env CGO_ENABLED=1 GORACE="history_size=7 halt_on_errors=1" $(GOTEST) -race -test.timeout=20m ./...
	cd psbt && env CGO_ENABLED=1 GORACE="history_size=7 halt_on_errors=1" $(GOTEST) -race -test.timeout=20m ./...
	cd wire && env CGO_ENABLED=1 GORACE="history_size=7 halt_on_errors=1" $(GOTEST) -race -test.timeout=20m ./...
	cd v2transport && env CGO_ENABLED=1 GORACE="history_size=7 halt_on_errors=1" $(GOTEST) -race -test.timeout=20m ./...

# =========
# UTILITIES
# =========

#? fmt: Fix imports and formatting source
fmt: goimports
	@$(call print, "Fixing imports.")
	$(GOIMPORTS_BIN) -w .
	@$(call print, "Formatting source.")
	gofmt -l -w -s .

#? lint: Lint source
lint: $(LINT_BIN)
	@$(call print, "Linting source.")
	$(LINT)

#? clean: Clean source
clean:
	@$(call print, "Cleaning source.$(NC)")
	find . -name coverage.txt | xargs $(RM)
	find . -name coverage.txt.bak | xargs $(RM)

#? tidy-module: Run 'go mod tidy' for all modules
tidy-module:
	echo "Running 'go mod tidy' for all modules"
	scripts/tidy_modules.sh

#? check-go-versions: Check that Go versions follow the policy in README.md
check-go-versions:
	@$(call print, "Checking Go versions.")
	scripts/check_go_versions.sh

.PHONY: all \
	default \
	build \
	check \
	unit \
	unit-cover \
	unit-race \
	fmt \
	lint \
	clean \
	tidy-module \
	check-go-versions

#? help: Get more info on make commands
help: Makefile
	@echo " Choose a command run in btcd:"
	@sed -n 's/^#?//p' $< | column -t -s ':' |  sort | sed -e 's/^/ /'

.PHONY: help
