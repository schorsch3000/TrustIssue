# Other tools

All of these are in `tools/user/` and in the release archives.

## dummy-keys.html: fake keys for a development device

A single HTML file (no network, no dependencies, its own SHA-1 and HMAC, checked against the RFC test
vectors) that makes up dummy TOTP keys, so real keys never have to go onto an open development device. The
keys are kept unencrypted in your browser's `localStorage`. It shows the live codes with a countdown,
applies the device's name rules, offers *Copy add command* (`echo SECRET | trustctl add NAME --digits N
--period S`; fine for fake keys, never do that with a real secret), the secret and an `otpauth://` URI, and checks a code you typed from the device (the
current, previous or next window).

## final_lock.sh, unlock.sh, flash_bootloader.sh, onboard_usbtiny.sh

The ISP scripts: {doc}`../setup/onboarding`.

## font2png.py and png2font.py

Edit a font as a picture. `font2png.py font.bin sheet.png [--scale 8]` writes a sheet of 16 x 6 glyphs
(characters 32 to 127, row by row, 8x8 pixels each, white = lit, exactly black = off); `png2font.py
sheet.png font.bin [--scale 8]` turns the edited sheet back into a font file. The scale must match and
the blocks must be uniform. Both need Pillow; the round trip is byte exact. Then try it with
`trustctl calibrate font font.bin` ({doc}`../setup/calibration`).

## c64rom2font.py

Makes fonts from a C64 character ROM: `c64rom2font.py ROM [--set upper|lower] [--outdir DIR]`. A 4096 byte
ROM gives `NAME_upper.EXT` (capitals and graphics) and `NAME_lower.EXT` (lower case), a 2048 byte one
gives `NAME_font.EXT`. Only the ASCII range is taken (screen codes mapped to ASCII; a few characters such
as `\` or `_` are approximated or blank) and it never overwrites the ROM. Mind the license of the ROM you
convert: the original Commodore ROM is not ours to ship.

## trustctl-menu, trustctl-tray, trustctl-menubar, trustctl-import

{doc}`trustctl-menu`, {doc}`trustctl-tray`, {doc}`trustctl-menubar`, {doc}`trustctl-import`.
