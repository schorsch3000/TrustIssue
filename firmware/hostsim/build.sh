#!/bin/sh
# Builds the host simulation of the core firmware: build.sh [OUTPUT]  (default build/hostsim).
# AUTONOMOUS_WIPE=0 leaves the autonomous wipe out, like core/build.sh.
set -e
cd "$(dirname "$0")"
out="${1:-build/hostsim}"
mkdir -p "$(dirname "$out")"
wipe="-DAUTONOMOUS_WIPE=1"
[ "$AUTONOMOUS_WIPE" = 0 ] && wipe=""
g++ -std=gnu++11 -O1 -Wall -Wno-unused-function $wipe -Imock -I. -I../core -I../trustcore/src \
  -x c++ -o "$out" hostsim.cpp ../trustcore/src/*.cpp
