#!/bin/sh
# Sets the final lock bits of a finished device. This is NOT reversible except by a
# chip erase over ISP (which also wipes the flash and the EEPROM, so the keys).
#
#   tools/user/final_lock.sh <uno-with-ArduinoISP-port> --yes
#   ISP_ARGS="-c usbtiny" tools/user/final_lock.sh - --yes     (another programmer; the port is unused)
#
# Lock byte 0xCC:
#   LB   = 00  mode 3: no reading or programming of flash and EEPROM over ISP
#   BLB1 = 00  mode 3: nothing may write into the boot section (not even the bootloader),
#              and the application cannot read the bootloader with LPM
#   BLB0 = 11  unrestricted: the bootloader must be able to write the application section
# Checks first that the fuses are as the design wants them (BOOTRST, 2 KB boot section,
# EESAVE unprogrammed, so a chip erase also empties the EEPROM).
set -e
port="$1"
if [ -z "$port" ] || [ "$2" != "--yes" ]; then
  sed -n '2,16p' "$0"
  exit 64
fi
: "${ISP_ARGS:=-c stk500v1 -b 19200 -P $port}"
# shellcheck disable=SC2086
avr() { avrdude $ISP_ARGS -p m328p "$@"; }
read_fuse() { avr -U "$1:r:-:h" 2>/dev/null | tr -d '[:space:]'; }
hfuse=$(read_fuse hfuse)
echo "hfuse is $hfuse (expected 0xda: BOOTRST set, 2 KB boot section, EESAVE unprogrammed)"
[ "$hfuse" = "0xda" ] || { echo "unexpected hfuse, not locking"; exit 1; }
avr -U lock:w:0xCC:m -U lock:r:-:h
echo "locked: flash and EEPROM can no longer be read over ISP"
