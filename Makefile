# The one file, for each platform it runs on.
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null | sed 's/^v//' || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)
PLATFORMS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64
# The core's packages (this module, Runwell as a library), named so a stray
# directory in the tree never breaks a target.
PKGS := ./cmd/runwell-runner/... ./internal/... ./plugin/... ./plugintest/... ./runwell/...
# The official binary: a module of its own (cmd/runwell/go.mod), the core
# (this checkout) with the official plugins at the versions it names. go.work
# puts the two together, and any plugin checkout you add to it.
BIN := cmd/runwell
# The linters, pinned and run without adding them to go.mod.
LINT := go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.15.0
VULNCHECK := go run golang.org/x/vuln/cmd/govulncheck@v1.9.0

.PHONY: build run test lint vuln cover fmt tidy dist plugin-ui ui ui-dev clean

# The server, and the runner (cmd/runwell-runner) people run on their own machines.
build:
	cd $(BIN) && CGO_ENABLED=0 go build -trimpath -ldflags '$(LDFLAGS)' -o $(CURDIR)/dist/runwell .
	CGO_ENABLED=0 go build -trimpath -ldflags '$(LDFLAGS)' -o dist/runwell-runner ./cmd/runwell-runner

run:
	cd $(BIN) && go run . serve --data $(CURDIR)/data

# vet, then the tests, race-checked and in random order: the core's, then
# the binary's (which vets and builds it with the plugins).
test:
	go vet $(PKGS)
	cd $(BIN) && go vet ./...
	go test -race -shuffle=on $(PKGS)
	cd $(BIN) && go test -race -shuffle=on ./...

# golangci-lint, as .golangci.yml configures it, on both modules.
lint:
	$(LINT) run $(PKGS)
	cd $(BIN) && $(LINT) run ./...

# Known vulnerabilities in code the binaries actually call (the plugins' too).
vuln:
	$(VULNCHECK) $(PKGS)
	cd $(BIN) && $(VULNCHECK) ./...

# The tests' total coverage, with the per-function detail in coverage.out.
cover:
	go test -race -covermode=atomic -coverprofile=coverage.out $(PKGS)
	go tool cover -func=coverage.out | tail -1

# gofumpt and goimports, rewriting files in place.
fmt:
	$(LINT) fmt $(PKGS)
	cd $(BIN) && $(LINT) fmt ./...

# Both modules' go.mod and go.sum, as the code needs them.
tidy:
	GOWORK=off go mod tidy
	cd $(BIN) && GOWORK=off go mod tidy

# dist/runwell-<os>-<arch>[.exe], dist/runwell-runner-<os>-<arch>[.exe] and
# dist/checksums.txt: what a release attaches, and what install.sh downloads.
dist:
	@rm -rf dist && mkdir -p dist
	@for p in $(PLATFORMS); do \
	  os=$${p%/*}; arch=$${p#*/}; ext=; [ $$os = windows ] && ext=.exe; \
	  echo "runwell-$$os-$$arch$$ext ($(VERSION))"; \
	  (cd $(BIN) && CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build -trimpath -ldflags '$(LDFLAGS)' -o $(CURDIR)/dist/runwell-$$os-$$arch$$ext .) || exit 1; \
	  CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build -trimpath -ldflags '$(LDFLAGS)' -o dist/runwell-runner-$$os-$$arch$$ext ./cmd/runwell-runner || exit 1; \
	done
	cd dist && sha256sum runwell-* > checksums.txt

# The official plugins' own screens (each plugin module's ui/, found through
# cmd/runwell's go.mod) copied into ui/src/plugins/<key>/, where the app
# picks them up. The copies are gitignored; ui and ui-dev run this first.
plugin-ui:
	node ui/scripts/plugin-ui.js

# The app (ui/, Svelte + Tailwind + shadcn-svelte) into internal/admin/dist, which the
# binary embeds. Committed, so building the binary needs no Node.
ui: plugin-ui
	cd ui && npm ci && npm run check && npm run build

# The app with hot reload on :5173, against `make run` on :8080.
ui-dev: plugin-ui
	cd ui && npm run dev

clean:
	rm -rf dist coverage.out
