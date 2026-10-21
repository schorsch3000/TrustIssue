#!/bin/sh
# Compiles Logic.swift with the tests of its pure helpers and runs them. The menu bar app itself needs a
# window system: build.sh, then open trustctl-menubar.app.
set -e
cd "$(dirname "$0")"
out=$(mktemp -d)
trap 'rm -rf "$out"' EXIT
swiftc -swift-version 5 -o "$out/tests" Logic.swift tests/main.swift
"$out/tests"
