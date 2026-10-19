# The one file, for each platform it runs on.
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null | sed 's/^v//' || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)
PLATFORMS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64
# The core's packages, named so a stray directory in the tree never breaks a target.
PKGS := ./cmd/... ./internal/... ./plugins/...
# The linters, pinned and run without adding them to go.mod.
LINT := go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.15.0
VULNCHECK := go run golang.org/x/vuln/cmd/govulncheck@v1.9.0

.PHONY: build run test lint vuln cover fmt dist ui ui-dev clean

# The server, and the runner (cmd/runwell-runner) people run on their own machines.
build:
	CGO_ENABLED=0 go build -trimpath -ldflags '$(LDFLAGS)' -o dist/runwell ./cmd/runwell
	CGO_ENABLED=0 go build -trimpath -ldflags '$(LDFLAGS)' -o dist/runwell-runner ./cmd/runwell-runner

run:
	go run ./cmd/runwell serve --data ./data

# vet, then the tests, race-checked and in random order.
test:
	go vet $(PKGS)
	go test -race -shuffle=on $(PKGS)

# golangci-lint, as .golangci.yml configures it.
lint:
	$(LINT) run $(PKGS)

# Known vulnerabilities in code the binary actually calls.
vuln:
	$(VULNCHECK) $(PKGS)

# The tests' total coverage, with the per-function detail in coverage.out.
cover:
	go test -race -covermode=atomic -coverprofile=coverage.out $(PKGS)
	go tool cover -func=coverage.out | tail -1

# gofumpt and goimports, rewriting files in place.
fmt:
	$(LINT) fmt $(PKGS)

# The app (ui/, Svelte + Tailwind + shadcn-svelte) into internal/admin/dist, which the
# binary embeds. Committed, so building the binary needs no Node.
ui:
	cd ui && npm ci && npm run check && npm run build

# The app with hot reload on :5173, against `make run` on :8080.
ui-dev:
	cd ui && npm run dev


# dist/runwell-<os>-<arch>[.exe], dist/runwell-runner-<os>-<arch>[.exe] and
# dist/checksums.txt: what a release attaches, and what install.sh downloads.
dist:
	@rm -rf dist && mkdir -p dist
	@for p in $(PLATFORMS); do \
	  os=$${p%/*}; arch=$${p#*/}; ext=; [ $$os = windows ] && ext=.exe; \
	  echo "runwell-$$os-$$arch$$ext ($(VERSION))"; \
	  CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build -trimpath -ldflags '$(LDFLAGS)' -o dist/runwell-$$os-$$arch$$ext ./cmd/runwell || exit 1; \
	  CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build -trimpath -ldflags '$(LDFLAGS)' -o dist/runwell-runner-$$os-$$arch$$ext ./cmd/runwell-runner || exit 1; \
	done
	cd dist && sha256sum runwell-* > checksums.txt

clean:
	rm -rf dist coverage.out
