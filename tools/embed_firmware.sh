#!/bin/sh
# Builds the core and the calibration firmware and copies the images into
# trustctl/internal/firmware, where trustctl embeds them (go:embed), together
# with the firmware version from firmware/core/core.ino. Run it whenever the
# firmware changes, and commit the result: trustctl and the firmware are
# released together.
set -e
root=$(cd "$(dirname "$0")/.." && pwd)
target="$root/trustctl/internal/firmware"
"$root/firmware/core/build.sh"
"$root/firmware/calibration/build.sh"
cp "$root/firmware/core/build/core.ino.hex" "$target/core.hex"
cp "$root/firmware/calibration/build/calibration.ino.hex" "$target/calibration.hex"
sed -n 's/^static const char VERSION\[\] PROGMEM = "\(.*\)";/\1/p' "$root/firmware/core/core.ino" > "$target/VERSION"
echo "firmware $(cat "$target/VERSION") embedded in $target"
