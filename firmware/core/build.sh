#!/bin/sh
# Builds (and with "upload <port>" uploads) the core firmware for the Nano.
# - The larger serial receive buffer keeps a whole ADD line safe while the display redraws.
# - The device parameter block is placed at flash address 0x7000, where trustctl patches it.
# - The autonomous wipe (wipe without a host, by a long button gesture) is built in unless
#   AUTONOMOUS_WIPE=0 is set; it also needs its config bit switched on on the device.
set -e
cd "$(dirname "$0")"
fqbn=arduino:avr:nano:cpu=atmega328old
wipe="-DAUTONOMOUS_WIPE=1"
[ "$AUTONOMOUS_WIPE" = 0 ] && wipe=""
arduino-cli compile -b "$fqbn" --library ../trustcore \
  --build-property "compiler.cpp.extra_flags=-DSERIAL_RX_BUFFER_SIZE=128 $wipe" \
  --build-property "compiler.c.elf.extra_flags=-Wl,--section-start=.paramblock=0x7000" \
  --output-dir "$PWD/build" .
if [ "$1" = upload ]; then
  arduino-cli upload -p "$2" -b "$fqbn" .
fi
