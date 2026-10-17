#!/bin/sh
# Builds the calibration firmware into firmware/calibration/build/calibration.ino.hex.
# Flash it with: trustctl --device PORT flash firmware/calibration/build/calibration.ino.hex --no-patch
set -e
cd "$(dirname "$0")"
# --clean: see firmware/core/build.sh (a cached core built with other flags must not be reused).
arduino-cli compile --clean -b arduino:avr:nano:cpu=atmega328old --output-dir "$PWD/build" .
