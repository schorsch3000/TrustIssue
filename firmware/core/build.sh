#!/bin/sh
# Builds (and with "upload <port>" uploads) the core firmware for the Nano.
# - The larger serial receive buffer keeps a whole ADD line safe while the display redraws.
# - The device parameter block is placed at flash address 0x7000, where trustctl patches it.
set -e
cd "$(dirname "$0")"
fqbn=arduino:avr:nano:cpu=atmega328old
arduino-cli compile -b "$fqbn" --library ../trustcore \
  --build-property "compiler.cpp.extra_flags=-DSERIAL_RX_BUFFER_SIZE=128" \
  --build-property "compiler.c.elf.extra_flags=-Wl,--section-start=.paramblock=0x7000" \
  --output-dir "$PWD/build" .
if [ "$1" = upload ]; then
  arduino-cli upload -p "$2" -b "$fqbn" .
fi
