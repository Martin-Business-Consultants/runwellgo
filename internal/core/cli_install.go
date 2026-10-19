package core

import (
	"net/http"
	"strings"

	"github.com/Martin-Business-Consultants/runwellgo/v3/internal/update"
)

// The runwell CLI, served by the install so it matches it: /install/cli is
// a shell installer for the runwell binary (the same one that runs an
// install: it is the CLI too), of this install's version when it's a
// release, checked against the release's checksums, then signed in here.
//
//	curl -fsSL https://runwell.example.com/install/cli | sh

func init() { routeSets = append(routeSets, (*App).cliInstallRoutes) }

func (a *App) cliInstallRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /install/cli", a.cliInstall)
}

func (a *App) cliInstall(w http.ResponseWriter, r *http.Request) {
	version := a.cfg.Version
	if version == "" || version == "dev" || strings.Contains(version, "-") {
		version = "latest"
	} else if !strings.HasPrefix(version, "v") {
		version = "v" + version
	}
	script := strings.NewReplacer("__RUNWELL_URL__", a.BaseURL(), "__REPO__", update.Repo, "__VERSION__", version).Replace(cliInstallScript)
	w.Header().Set("Content-Type", "text/x-shellscript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(script))
}

//nolint:dupword // a shell script's nested fi
const cliInstallScript = `#!/bin/sh
# Installs the runwell CLI for __RUNWELL_URL__
set -eu

repo="__REPO__"
version="${RUNWELL_VERSION:-__VERSION__}"

case "$(uname -s)" in
  Linux) os=linux ;;
  Darwin) os=darwin ;;
  *) echo "runwell: no build for $(uname -s); download runwell-windows-amd64.exe from https://github.com/$repo/releases" >&2; exit 1 ;;
esac
case "$(uname -m)" in
  x86_64 | amd64) arch=amd64 ;;
  arm64 | aarch64) arch=arm64 ;;
  *) echo "runwell: no build for $(uname -m)" >&2; exit 1 ;;
esac
command -v curl >/dev/null 2>&1 || { echo "runwell: needs curl" >&2; exit 1; }

file="runwell-$os-$arch"
if [ "$version" = latest ]; then
  version="$(curl -fsSL "https://api.github.com/repos/$repo/releases?per_page=100" |
    tr ',' '\n' | grep -E '"(tag_name|browser_download_url)"' |
    awk -v f="/$file\"" '/tag_name/ { tag = $0 } index($0, f) { print tag; exit }' |
    sed 's/.*"tag_name": *"\([^"]*\)".*/\1/')"
  if [ -z "$version" ]; then
    echo "runwell: no release has a $file build yet." >&2
    exit 1
  fi
fi
base="https://github.com/$repo/releases/download/$version"

dir="${RUNWELL_BIN:-$HOME/.local/bin}"
mkdir -p "$dir"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

echo "Downloading $file ($version)…"
curl -fsSL "$base/$file" -o "$tmp/$file"
curl -fsSL "$base/checksums.txt" -o "$tmp/checksums.txt"
expected="$(grep " $file\$" "$tmp/checksums.txt" | cut -d' ' -f1)"
if command -v sha256sum >/dev/null 2>&1; then
  actual="$(sha256sum "$tmp/$file" | cut -d' ' -f1)"
else
  actual="$(shasum -a 256 "$tmp/$file" | cut -d' ' -f1)"
fi
if [ -z "$expected" ] || [ "$expected" != "$actual" ]; then
  echo "runwell: the download doesn't match the release's checksum; not installing it." >&2
  exit 1
fi

chmod +x "$tmp/$file"
mv "$tmp/$file" "$dir/runwell"
echo "Installed runwell to $dir/runwell."
case ":$PATH:" in *":$dir:"*) ;; *) echo "Add $dir to your PATH to run it as runwell." ;; esac
echo "Next: runwell login __RUNWELL_URL__   (then runwell mcp setup to add it to an AI app)"
`
