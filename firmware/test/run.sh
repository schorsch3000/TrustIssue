#!/bin/sh
# Builds and runs the host tests.
set -e
cd "$(dirname "$0")"
out="${TMPDIR:-/tmp}/trustcore_test"
for test in test_core test_storage test_protocol test_time; do
  g++ -std=c++11 -Wall -Wextra -O1 -o "$out.$test" "$test.cpp" ../trustcore/src/*.cpp
  "$out.$test"
done
