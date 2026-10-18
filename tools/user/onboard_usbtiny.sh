#!/bin/sh
# Onboards a fresh (or old) Arduino Nano in one go with a USBtinyISP programmer on the ICSP
# header: chip erase, the Trust Issue bootloader AND the core firmware, the fuses and the
# development lock bits. Afterwards the device works; it needs no button gesture and no second
# Arduino. Look and feel (font, logo, contrast) are patched in later with `trustctl flash`.
#
#   tools/user/onboard_usbtiny.sh --yes [--core FILE.hex] [--bootloader FILE.hex] [--final-lock]
#                                 [--erase-locked]
#
# What it writes: flash (bootloader at 0x7800 and the core firmware, verified), lfuse 0xFF,
# hfuse 0xDA (BOOTRST, 2 KB boot section, EESAVE unprogrammed), efuse 0xFD, lock byte 0xCF.
# The chip erase empties the flash AND the EEPROM (all keys). A device that already carries
# the final lock (0xCC) is refused, because erasing it destroys its keys, unless --erase-locked.
# Without --core the firmware built into trustctl is used (trustctl/internal/firmware/core.hex),
# without --bootloader the bootloader is built from firmware/bootloader (needs the Arduino AVR
# toolchain).
#
# --final-lock sets lock byte 0xCC at the very end (no ISP read-out or programming any more,
# the bootloader section is frozen; undo only by a chip erase, see unlock.sh). Do it only for a
# finished device: it takes the "NOT LOCKED" warning away, and a PIN, font or logo can still be
# changed afterwards through the bootloader gate, but the chip can no longer be read over ISP.
#
# Wiring: USBtiny 6-pin ICSP cable to the Nano's ICSP header (MISO, VCC, SCK, MOSI, RESET,
# GND); the Nano may be powered by the programmer or by its own USB cable, not both.
# Another programmer or options: ISP_ARGS="-c usbtiny -B 4" (default "-c usbtiny").
set -e
usage() { sed -n '2,27p' "$0"; exit 64; }
yes=0 final=0 erase_locked=0 core="" boot=""
while [ $# -gt 0 ]; do
  case "$1" in
    --yes) yes=1 ;;
    --final-lock) final=1 ;;
    --erase-locked) erase_locked=1 ;;
    --core) [ $# -ge 2 ] || { echo "--core needs a file" >&2; usage; }; core="$2"; shift ;;
    --bootloader) [ $# -ge 2 ] || { echo "--bootloader needs a file" >&2; usage; }; boot="$2"; shift ;;
    *) echo "unknown option $1" >&2; usage ;;
  esac
  shift
done
[ "$yes" = 1 ] || usage
root=$(cd "$(dirname "$0")/../.." && pwd)
: "${ISP_ARGS:=-c usbtiny}"
# shellcheck disable=SC2086
avr() { avrdude $ISP_ARGS -p m328p "$@"; }
read_byte() { avr -U "$1:r:-:h" 2>/dev/null | tr -d '[:space:]'; }

[ -n "$core" ] || core="$root/trustctl/internal/firmware/core.hex"
if [ -z "$boot" ]; then
  make -C "$root/firmware/bootloader" >&2
  boot="$root/firmware/bootloader/boot.hex"
fi
[ -f "$core" ] || { echo "no firmware image $core"; exit 1; }
[ -f "$boot" ] || { echo "no bootloader image $boot"; exit 1; }

signature=$(read_byte signature) || true
case "$signature" in
  0x1e,0x95,0xf|0x1e,0x95,0x0f|0x1e950f) ;;
  *) echo "no ATmega328P found (signature: '$signature'): is the USBtiny plugged in and the ICSP cable the right way round?"; exit 1 ;;
esac
lock=$(read_byte lock)
echo "chip found, lock byte $lock"
if [ "$lock" = 0xcc ] && [ "$erase_locked" != 1 ]; then
  echo "this device carries the final lock: a chip erase destroys its keys."
  echo "If that is what you want, run again with --erase-locked."
  exit 1
fi

echo "erasing, writing the bootloader and the firmware (verified)..."
avr -e -U "flash:w:$boot:i" -U "flash:w:$core:i" -U lfuse:w:0xFF:m -U efuse:w:0xFD:m 2>&1 | tail -6
# the high fuse comes last: BOOTRST then points the reset vector at the bootloader
avr -U hfuse:w:0xDA:m -U lock:w:0xCF:m 2>&1 | tail -2

lfuse=$(read_byte lfuse) hfuse=$(read_byte hfuse) efuse=$(read_byte efuse) lock=$(read_byte lock)
echo "fuses: low $lfuse high $hfuse extended $efuse, lock $lock"
if [ "$lfuse" != 0xff ] || [ "$hfuse" != 0xda ] || [ "$efuse" != 0xfd ] || [ "$lock" != 0xcf ]; then
  echo "the fuses are not what they should be, check the wiring and try again"
  exit 1
fi

if [ "$final" = 1 ]; then
  echo "setting the final lock bits (0xCC)..."
  avr -U lock:w:0xCC:m 2>&1 | tail -1
  lock=$(read_byte lock)
  [ "$lock" = 0xcc ] || { echo "the lock byte reads $lock, expected 0xcc"; exit 1; }
  echo "locked: flash and EEPROM can no longer be read over ISP."
else
  echo "the device is onboarded (development lock bits: the idle screen says NOT LOCKED)."
  echo "Set the final lock when the device is finished: ISP_ARGS="-c usbtiny" tools/user/final_lock.sh - --yes"
fi
echo "Next: trustctl --device /dev/ttyUSBx hello   (and add keys; trustctl flash to patch font, logo and contrast)"
