#!/bin/sh
# Installs Runwell: one file, for this machine, from the latest
# release (or RUNWELL_VERSION=v2.0.0), checked against the release's
# checksums.
#
#   curl -fsSL https://raw.githubusercontent.com/Martin-Business-Consultants/runwellgo/main/install.sh | sh
#
# It goes to /usr/local/bin when that's writable, else ~/.local/bin
# (RUNWELL_DIR=… to choose). RUNWELL_BINARY=runwell-runner installs the
# runner instead (docs/runners.md), the program that does work Runwell hands
# to a machine of yours.
set -eu

repo="Martin-Business-Consultants/runwellgo"
version="${RUNWELL_VERSION:-latest}"
binary="${RUNWELL_BINARY:-runwell}"
case "$binary" in
  runwell | runwell-runner) ;;
  *) echo "runwell: RUNWELL_BINARY is runwell or runwell-runner, not $binary" >&2; exit 1 ;;
esac

case "$(uname -s)" in
  Linux) os=linux ;;
  Darwin) os=darwin ;;
  *) echo "runwell: no build for $(uname -s); on Windows, download runwell-windows-amd64.exe from https://github.com/$repo/releases" >&2; exit 1 ;;
esac
case "$(uname -m)" in
  x86_64 | amd64) arch=amd64 ;;
  arm64 | aarch64) arch=arm64 ;;
  *) echo "runwell: no build for $(uname -m)" >&2; exit 1 ;;
esac

file="$binary-$os-$arch"

# The repo's releases include the Rails core's (from main), which carry no
# binary, so "latest" is the newest release that has this file.
if [ -n "${RUNWELL_BASE_URL:-}" ]; then
  base="$RUNWELL_BASE_URL" # a mirror, or a local dist/ to test with
elif [ "$version" = latest ]; then
  version="$(curl -fsSL "https://api.github.com/repos/$repo/releases?per_page=100" |
    tr ',' '\n' | grep -E '"(tag_name|browser_download_url)"' |
    awk -v f="/$file\"" '/tag_name/ { tag = $0 } index($0, f) { print tag; exit }' |
    sed 's/.*"tag_name": *"\([^"]*\)".*/\1/')"
  if [ -z "$version" ]; then
    echo "$binary: no release has a $file build yet." >&2
    echo "From a checkout with Go installed, run it directly instead: go run ./cmd/runwell serve" >&2
    exit 1
  fi
  base="https://github.com/$repo/releases/download/$version"
else
  base="https://github.com/$repo/releases/download/$version"
fi
if [ -n "${RUNWELL_DIR:-}" ]; then
  dir="$RUNWELL_DIR"
elif [ -w /usr/local/bin ]; then
  dir=/usr/local/bin
else
  dir="$HOME/.local/bin"
fi
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
mv "$tmp/$file" "$dir/$binary"
echo "Installed $("$dir/$binary" version) at $dir/$binary"
case ":$PATH:" in *":$dir:"*) ;; *) echo "Add $dir to your PATH to run it as \`$binary\`." ;; esac
echo
if [ "$binary" = runwell-runner ]; then
  echo "Set it up with the command Settings › Runners gave you (runwell-runner init …),"
  echo "then check it with runwell-runner doctor and start it with runwell-runner run."
  exit 0
fi
echo "Start it:  runwell serve --data ./data"
echo "Then open http://localhost:8080/ and set up the first owner."
