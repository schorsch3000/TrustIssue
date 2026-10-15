#!/bin/sh
# Unlocks a device again: a chip erase over ISP. This is the only way back from the lock bits
# of final_lock.sh, and it DESTROYS EVERYTHING on the chip: the flash (the bootloader and the
# firmware) and the EEPROM (all keys, the timezone and the settings; EESAVE is unprogrammed
# by design, so the erase takes the EEPROM with it). The fuses are not changed.
# Afterwards the device has no bootloader: run tools/user/flash_bootloader.sh next.
#
#   tools/user/unlock.sh <port of the Uno running ArduinoISP> --yes
#
# Another ISP programmer: set ISP_ARGS, e.g. ISP_ARGS="-c usbasp" tools/user/unlock.sh - --yes
set -e
port="$1"
if [ -z "$port" ] || [ "$2" != "--yes" ]; then
  sed -n '2,12p' "$0"
  exit 64
fi
: "${ISP_ARGS:=-c stk500v1 -b 19200 -P $port}"
# shellcheck disable=SC2086
avr() { avrdude $ISP_ARGS -p m328p "$@"; }

lock=$(avr -U lock:r:-:h 2>/dev/null | tr -d '[:space:]') || { echo "cannot talk to the chip over ISP"; exit 1; }
echo "lock byte before: $lock"
echo "erasing the chip (flash, EEPROM, lock bits)..."
avr -e 2>&1 | tail -3
lock=$(avr -U lock:r:-:h 2>/dev/null | tr -d '[:space:]')
echo "lock byte after: $lock"
case "$lock" in
  0xff) echo "unlocked and empty: now run tools/user/flash_bootloader.sh" ;;
  *) echo "the lock byte is not 0xff, the erase did not work"; exit 1 ;;
esac
