#!/bin/sh
# Builds the calibration firmware into firmware/calibration/build/calibration.ino.hex.
# Flash it with: trustctl --device PORT flash firmware/calibration/build/calibration.ino.hex --no-patch
set -e
cd "$(dirname "$0")"
arduino-cli compile -b arduino:avr:nano:cpu=atmega328old --output-dir "$PWD/build" .
