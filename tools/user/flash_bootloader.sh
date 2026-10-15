#!/bin/sh
# Puts the Trust Issues bootloader on a chip over ISP and sets the fuses and the development
# lock bits the design wants (low fuse 0xFF, high fuse 0xDA: BOOTRST, 2 KB boot section, EESAVE
# unprogrammed; extended fuse 0xFD; lock byte 0xCF: the boot section cannot be overwritten from
# the application, ISP read-out still open). Works on a blank chip, on a development device and,
# since the burn starts with a chip erase, on a locked one.
#
#   tools/user/flash_bootloader.sh <port of the Uno running ArduinoISP> --yes [--keep-eeprom] [--hex FILE]
#
# The chip erase empties the flash AND the EEPROM (all keys), unless --keep-eeprom is given:
# then EESAVE is programmed for the erase and cleared again afterwards. That does not work on a
# locked device (the fuses cannot be changed): use unlock.sh first, and the keys are gone.
# Without --hex the bootloader is built from firmware/bootloader (needs the Arduino AVR toolchain).
# Afterwards flash the firmware with: trustctl flash   (final lock bits: tools/user/final_lock.sh)
#
# Another ISP programmer: set ISP_ARGS, e.g. ISP_ARGS="-c usbasp" tools/user/flash_bootloader.sh - --yes
set -e
usage() { sed -n '2,15p' "$0"; exit 64; }
[ $# -ge 1 ] || usage
port="$1"
shift
yes=0 keep=0 hex=""
while [ $# -gt 0 ]; do
  case "$1" in
    --yes) yes=1 ;;
    --keep-eeprom) keep=1 ;;
    --hex) [ $# -ge 2 ] || { echo "--hex needs a file" >&2; usage; }; hex="$2"; shift ;;
    *) echo "unknown option $1" >&2; usage ;;
  esac
  shift
done
if [ -z "$port" ] || [ "$yes" != 1 ]; then usage; fi
root=$(cd "$(dirname "$0")/../.." && pwd)
: "${ISP_ARGS:=-c stk500v1 -b 19200 -P $port}"
# shellcheck disable=SC2086
avr() { avrdude $ISP_ARGS -p m328p "$@"; }
read_byte() { avr -U "$1:r:-:h" 2>/dev/null | tr -d '[:space:]'; }

if [ -z "$hex" ]; then
  make -C "$root/firmware/bootloader" >&2
  hex="$root/firmware/bootloader/boot.hex"
fi
[ -f "$hex" ] || { echo "no bootloader image $hex"; exit 1; }

signature=$(read_byte signature) || true
case "$signature" in 0x1e,0x95,0xf|0x1e,0x95,0x0f|0x1e950f) ;; *) echo "no ATmega328P found over ISP (signature: '$signature')"; exit 1 ;; esac
lock=$(read_byte lock)
echo "chip found, lock byte $lock"

if [ "$keep" = 1 ]; then
  if [ "$lock" != "0xcf" ] && [ "$lock" != "0xff" ]; then
    echo "this device is locked, its fuses cannot be changed, so the EEPROM cannot be kept."
    echo "Unlock it first (tools/user/unlock.sh): that erases the keys."
    exit 1
  fi
  echo "keeping the EEPROM: programming EESAVE for the erase"
  avr -U hfuse:w:0xD2:m 2>&1 | tail -1
fi

echo "burning the bootloader (chip erase first)..."
avr -e -U "flash:w:$hex:i" -U lfuse:w:0xFF:m -U efuse:w:0xFD:m 2>&1 | tail -4
# the final high fuse is written last: EESAVE unprogrammed, as the design wants
avr -U hfuse:w:0xDA:m -U lock:w:0xCF:m 2>&1 | tail -2

lfuse=$(read_byte lfuse) hfuse=$(read_byte hfuse) efuse=$(read_byte efuse) lock=$(read_byte lock)
echo "fuses: low $lfuse high $hfuse extended $efuse, lock $lock"
if [ "$lfuse" = 0xff ] && [ "$hfuse" = 0xda ] && [ "$efuse" = 0xfd ] && [ "$lock" = 0xcf ]; then
  echo "bootloader installed. Next: trustctl flash"
else
  echo "the fuses are not what they should be, check the wiring and try again"
  exit 1
fi
