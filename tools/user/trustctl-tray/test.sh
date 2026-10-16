#!/bin/sh
# Compiles TrustctlTray.cs with Mono's mcs (C# 5, the language level of the compiler that ships with Windows) as a
# check, and runs the tests of its pure helpers. The GUI itself needs Windows.
set -e
cd "$(dirname "$0")"
out=$(mktemp -d)
trap 'rm -rf "$out"' EXIT
mcs -langversion:5 -nowarn:1591 -target:exe -main:LogicTests -out:"$out/tests.exe" \
  -r:System.Windows.Forms -r:System.Drawing TrustctlTray.cs tests/LogicTests.cs
mono "$out/tests.exe"
