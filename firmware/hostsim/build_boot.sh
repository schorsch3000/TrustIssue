#!/bin/sh
# Builds the host simulation of the bootloader: build_boot.sh [OUTPUT]  (default build/bootsim).
# The bootloader source is used as it is, except that its two assembler lines (the reset vector and
# the jump to the application) become a no-op and a call into the simulation.
set -e
cd "$(dirname "$0")"
out="${1:-build/bootsim}"
mkdir -p "$(dirname "$out")" build
sed -e 's|__asm__ volatile("clr r1\\n\\trjmp main");||' -e 's|__asm__ volatile("jmp 0");|hs_run_app();|' \
  ../bootloader/boot.c > build/boot_patched.c
grep -q hs_run_app build/boot_patched.c || { echo "boot.c changed: the jmp 0 line was not found" >&2; exit 1; }
! grep -q '__asm__' build/boot_patched.c || { echo "boot.c has an assembler line the host build does not know" >&2; exit 1; }
g++ -std=gnu++11 -O1 -Wall -Wno-unused-function -Wno-narrowing -fpermissive -Wno-write-strings -x c++ \
  -Ibootmock -I. -Ibuild -I../bootloader -o "$out" bootsim.cpp
