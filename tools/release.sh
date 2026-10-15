#!/bin/sh
# Builds the release binaries of trustctl into dist/. Each one carries the core
# and the calibration firmware built from this very checkout, so "trustctl flash"
# works without any other file.
#
#   tools/release.sh           rebuild the firmware, then build all platforms
#   tools/release.sh --quick   use the firmware images that are already embedded
#
# Commit the changed trustctl/internal/firmware/ files together with the code.
set -e
root=$(cd "$(dirname "$0")/.." && pwd)
[ "$1" = --quick ] || "$root/tools/embed_firmware.sh"
export GOPATH="${GOPATH_RELEASE:-$HOME/go}" CGO_ENABLED=0
firmware=$(cat "$root/trustctl/internal/firmware/VERSION")
describe=$(git -C "$root" describe --tags --always --dirty 2>/dev/null || echo unknown)
version="fw$firmware-$describe"
dist="$root/dist"
mkdir -p "$dist"
cd "$root/trustctl"
for target in linux/amd64 linux/arm64 linux/arm linux/386 windows/amd64 windows/arm64 \
              darwin/amd64 darwin/arm64 freebsd/amd64 openbsd/amd64; do
  os=${target%/*}
  arch=${target#*/}
  out="$dist/trustctl-$os-$arch"
  [ "$os" = windows ] && out="$out.exe"
  GOOS=$os GOARCH=$arch go build -trimpath -ldflags "-s -w -X main.version=$version" -o "$out" ./cmd/trustctl
  echo "built $out"
done
(cd "$dist" && sha256sum trustctl-* > SHA256SUMS)
echo "release $version in $dist"
