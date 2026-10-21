#!/bin/sh
# Builds trustctl-menubar.app next to the sources with the Swift compiler of the Command Line Tools
# (xcode-select --install; no Xcode project). A universal binary (arm64 and x86_64), ad-hoc signed.
# --arch ARCH builds for one architecture only (quicker, e.g. for a test).
set -e
cd "$(dirname "$0")"
min=12.0
archs="arm64 x86_64"
if [ "$1" = "--arch" ]; then archs=$2; fi
app=trustctl-menubar.app
out=$(mktemp -d)
trap 'rm -rf "$out"' EXIT
bins=""
for arch in $archs; do
  swiftc -O -swift-version 5 -target "$arch-apple-macos$min" -o "$out/$arch" TrustctlMenubar.swift Logic.swift
  bins="$bins $out/$arch"
done
rm -rf "$app"
mkdir -p "$app/Contents/MacOS"
# shellcheck disable=SC2086
lipo -create -output "$app/Contents/MacOS/trustctl-menubar" $bins
cat > "$app/Contents/Info.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>CFBundleIdentifier</key><string>de.trustissue.trustctl-menubar</string>
  <key>CFBundleName</key><string>trustctl-menubar</string>
  <key>CFBundleExecutable</key><string>trustctl-menubar</string>
  <key>CFBundlePackageType</key><string>APPL</string>
  <key>CFBundleShortVersionString</key><string>1.0</string>
  <key>CFBundleVersion</key><string>1</string>
  <key>LSMinimumSystemVersion</key><string>$min</string>
  <key>LSUIElement</key><true/>
  <key>NSHighResolutionCapable</key><true/>
</dict>
</plist>
PLIST
# ad-hoc: enough to run on this machine; macOS ties the Accessibility permission to the signature, so after
# a rebuild it may have to be granted again
codesign --force --sign - "$app"
echo "built $(pwd)/$app"
