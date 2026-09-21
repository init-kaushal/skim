#!/bin/sh
# Build the per-platform binaries published with a release.
#
# Pure Go with no cgo, so every target cross-compiles from one machine. Run by
# .github/workflows/release.yml on a tag, and runnable locally to check that a
# target still builds before tagging.
#
#   scripts/build-release.sh <version> [outdir]
#
# Produces, in outdir (default dist/):
#   skim-<version>-<os>-<arch>      one binary per target
#   SHA256SUMS                      checksums the launcher verifies before exec
set -eu

VERSION="${1:?usage: build-release.sh <version> [outdir]}"
OUT="${2:-dist}"

# Targets. Windows is absent on purpose: the launcher the hooks invoke is a
# POSIX shell script, so a windows/amd64 binary would ship with nothing able to
# start it. Claiming support we have not tested would be worse than the gap.
TARGETS="darwin/arm64 darwin/amd64 linux/amd64 linux/arm64"

rm -rf "$OUT"
mkdir -p "$OUT"

for t in $TARGETS; do
    os=${t%/*}
    arch=${t#*/}
    name="skim-${VERSION}-${os}-${arch}"
    echo "building $name"
    # CGO off keeps the binaries static and cross-compilable; trimpath and the
    # stripped build keep them small and reproducible.
    CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" \
        go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" \
        -o "$OUT/$name" ./cmd/skim
done

# One checksum file covering every artifact. The launcher refuses to exec a
# download whose hash is not listed here.
( cd "$OUT" && \
    if command -v sha256sum >/dev/null 2>&1; then
        sha256sum skim-* > SHA256SUMS
    else
        shasum -a 256 skim-* > SHA256SUMS
    fi )

echo
echo "artifacts in $OUT:"
ls -la "$OUT"
echo
cat "$OUT/SHA256SUMS"
