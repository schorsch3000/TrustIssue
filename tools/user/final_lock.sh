#!/bin/sh
# Sets the final lock bits of a finished device. This is NOT reversible except by a
# chip erase over ISP (which also wipes the flash and the EEPROM, so the keys).
#
#   tools/user/final_lock.sh <uno-with-ArduinoISP-port> --yes
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
  sed -n '2,15p' "$0"
  exit 64
fi
read_fuse() { avrdude -c stk500v1 -p m328p -P "$port" -b 19200 -U "$1:r:-:h" 2>/dev/null | tr -d '[:space:]'; }
hfuse=$(read_fuse hfuse)
echo "hfuse is $hfuse (expected 0xda: BOOTRST set, 2 KB boot section, EESAVE unprogrammed)"
[ "$hfuse" = "0xda" ] || { echo "unexpected hfuse, not locking"; exit 1; }
avrdude -c stk500v1 -p m328p -P "$port" -b 19200 -U lock:w:0xCC:m -U lock:r:-:h
echo "locked: flash and EEPROM can no longer be read over ISP"
