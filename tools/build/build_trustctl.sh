#!/bin/sh
# Builds trustctl for this machine from the current checkout, for testing:
#
#   tools/build/build_trustctl.sh                 -> trustctl/bin/trustctl
#   tools/build/build_trustctl.sh OUTPUT          -> OUTPUT
#
# It always rebuilds the firmware from firmware/ first and embeds it in
# trustctl/internal/firmware (the same files release.sh and CI use), so the binary carries
# the current firmware. Version like "fw0.1.5-8064620-dirty".
# trustctl/bin/ is ignored by git. For all platforms use tools/build/release.sh.
set -e
root=$(cd "$(dirname "$0")/../.." && pwd)
"$root/tools/build/embed_firmware.sh"
out="${1:-$root/trustctl/bin/trustctl}"
export GOPATH="${GOPATH_RELEASE:-$HOME/go}" CGO_ENABLED=0
firmware=$(cat "$root/trustctl/internal/firmware/VERSION")
describe=$(git -C "$root" describe --tags --always --dirty 2>/dev/null || echo unknown)
mkdir -p "$(dirname "$out")"
cd "$root/trustctl"
go build -trimpath -ldflags "-X main.version=fw$firmware-$describe" -o "$out" ./cmd/trustctl
echo "built $out"
"$out" version
