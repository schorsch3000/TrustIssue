# Trust Issues

A hardware TOTP vault for people who don't trust their computers.

The secrets live on a small device (an Arduino Nano with a 128x32 OLED and one button) and never
leave it. The host can add keys, list their names, and ask for a code, but every one of those
needs somebody to hold the button on the device while its display shows what is being done.
Malware on the laptop can neither export the secrets nor silently use them, and it cannot reflash
the device either: the bootloader only opens after a gesture on the button.

`CLAUDE.md` holds the design decisions and the detailed state of the project.

## What is in here

| Path | What |
| --- | --- |
| `firmware/core` | the application: serial protocol, confirm screens, idle logo, autonomous wipe |
| `firmware/trustcore` | portable C++ core (SHA-1, HMAC, TOTP, Base32, EEPROM storage, protocol parser, time) with host tests in `firmware/test` |
| `firmware/bootloader` | the button-gated bootloader (2 KB, from scratch) |
| `firmware/calibration` | tiny firmware to try out contrast, logo and font on the display |
| `firmware/rig` | the Uno test rig that presses the button and resets the Nano |
| `trustctl` | the host tool in Go: `list`, `token`, `add`, `del`, `tz`, `config`, `wipe`, `flash`, `calibrate`, `font extract`, `logo extract`, `daemon` |
| `tools/user` | tools for people who use the device: `trustctl-menu`, `dummy-keys.html` (fake keys), `font2png.py`, `png2font.py`, `c64rom2font.py`, `final_lock.sh` |
| `tools/build` | for building and testing the project: end-to-end tests (need the rig), `release.sh`, `embed_firmware.sh`, `embed_fonts.py`, `gen_params.py`, `flash.py` |

## Hardware

Arduino Nano (ATmega328P, 16 MHz), SSD1306 128x32 OLED on I2C (A4 SDA, A5 SCL, address 0x3C), a
push button from D2 to GND. For development a second Arduino (an Uno) acts as ISP programmer and as
the "finger": its D4 goes through 1 kOhm to the Nano's D2, its D10 to the Nano's RST, and D11 to D13
to the Nano's D11 to D13 (full table in `CLAUDE.md`).

## Setting up a device

1. **Bootloader (once, over ISP).** Put the `ArduinoISP` example sketch on the Uno, then
   `tools/user/flash_bootloader.sh <uno port> --yes`. It builds the bootloader, sets the fuses and the
   development lock bits and verifies them. The burn starts with a chip erase, so the keys are gone, unless you add
   `--keep-eeprom`. `tools/user/unlock.sh <uno port> --yes` is the way back from the final lock bits
   (`final_lock.sh`): a chip erase that destroys flash and EEPROM, followed by `flash_bootloader.sh`.
2. **Host tool.** `cd trustctl && go build -o bin/trustctl ./cmd/trustctl` (a static binary; Go 1.25+, which
   the module fetches by itself).
3. **Firmware.** `trustctl --device /dev/ttyUSB0 flash` (the core firmware is built into the `trustctl` binary;
   for development `flash FILE.hex` takes your own build, e.g. from `firmware/core/build.sh`). It tells you what to do on
   the device: hold the button while it resets, release when the display says so, wait for the bar,
   then tap, tap, press a third time and hold until the bar is full.
4. **Look and feel (optional).** Flash the built-in calibration firmware the same way
   (`trustctl flash calibration`), then
   `trustctl calibrate contrast full 0x20`, `calibrate contrast dim 0`, `calibrate logo Demo-Logos/trust_issue_01.png`,
   `calibrate font victoriabold8` (the options like `--device` may stand before or after
   the command), and flash the core firmware again; the settings are
   patched in from `~/.config/trustctl/device.json`.
5. `trustctl config set logo=on` for the idle logo.
6. Optional: `trustctl flash --set-pin` adds an update PIN (see Security model). From then on every `flash` asks for it after the gesture; `flash --clear-pin` removes it, `flash pin --set-pin` changes only the PIN.

Put `device = /dev/ttyUSB0` into `~/.config/trustctl/config` to leave out `--device`. `trustctl help-options` lists every
option with its `--argument`, environment variable and config file syntax, and the defaults.

Seven fonts are built into `trustctl` (`trustctl font list`; sources and licenses in `trustctl/internal/fonts/README.md`).
`trustctl calibrate font NAME` previews one on the calibration firmware and remembers it, `trustctl flash --font NAME`
patches one in and, once flashed, remembers it in the profile, and `trustctl font extract FILE.bin --name NAME` writes one out as a starting point.
A font file of your own works wherever a name does.

`tools/user/c64rom2font.py ROM [--set upper|lower] [--outdir DIR]` makes fonts from a C64 character ROM: a 4096 byte ROM gives
`NAME_upper.EXT` (capitals and graphics) and `NAME_lower.EXT` (lower case), a 2048 byte one gives `NAME_font.EXT`. Only the
ASCII range is taken (screen codes mapped to ASCII, a few characters like `\\` or `_` are approximated or blank). Then
`trustctl calibrate font NAME_lower.bin`. Mind the license of the ROM you convert; the original Commodore ROM is not ours to ship.

To start from what the device uses now, `trustctl font extract font.bin` writes the current font (the profile's, else the
built-in one) and `trustctl logo extract logo.png` writes the current logo as a 128x32 PNG (white = lit). The device itself
cannot be read back, so "current" means what `flash` would patch in; `--from firmware.hex` takes it from the parameter block
of a firmware file instead.

To edit a font as a picture, `tools/user/font2png.py font.bin sheet.png [--scale 8]` writes a sheet of 16 x 6 glyphs
(characters 32 to 127, 8x8 pixels each, white = lit) and `tools/user/png2font.py sheet.png font.bin [--scale 8]` turns the
edited sheet back into a font file (both need Pillow; the scale must match). Then `trustctl calibrate font font.bin`.

Tab completion (commands, options, key names asked from the device, file names for `flash` and `calibrate`):

```
source <(trustctl autocomplete)             # bash and sh-compatible shells; put it into ~/.bashrc
source <(trustctl autocomplete-zsh)         # zsh
trustctl autocomplete-fish | source         # fish
```

## Using it

```
trustctl list
echo SECRET | trustctl add "GitHub"        # confirm on the device
echo 'otpauth://totp/...' | trustctl add   # a URI carries its own name (issuer:account)
trustctl token git                         # prints the code once you hold the button 1-2 s
trustctl del GitHub
trustctl wipe                              # hold 5-6 s
```

The first command starts a small daemon that keeps the serial port open (so the device is not reset
by every command). `trustctl daemon stop` ends it. The device knows no clock: `token` sends the host's
time, and the display shows the resulting local date and time so you can see it is not for another
moment. Without a host, the keys can be wiped by a long hold at idle (`trustctl config set autowipe=on`
first).

To try it with fake keys, open `tools/user/dummy-keys.html` in a browser.

### A menu for Linux and BSD

`tools/user/trustctl-menu` is a small POSIX shell script: it lists the keys in a dmenu-like launcher, asks the
device for the code of the one you pick (hold the button) and hands it over.

```
trustctl-menu                                        # first launcher found: rofi, dmenu, wofi, bemenu, fuzzel
trustctl-menu chooser=rofi target=copy               # into the clipboard
trustctl-menu chooser=/bin/rofi target=copy clipboard=primary
trustctl-menu target=type delay=300 enter            # type it into the focused window (xdotool, or wtype on Wayland)
trustctl-menu chooser=rofi -- -p token -theme-str 'window {width: 20%;}'   # after -- the launcher's own arguments
```
It needs `trustctl` in the `PATH` (with the device known to it, see above) and shows a notification if
`notify-send` exists, so you know the device is waiting for the button; the notice is closed again as soon as you have
confirmed, refused or the device timed out. Bind it to a key in your window manager.
The code never appears on a command line: it goes to `xdotool`, `wtype`, `xclip`, `xsel` or `wl-copy` through stdin.

## Security model in short

- A compromised host is the threat; physical access to the device is out of scope.
- Secrets can't be read back over serial. Adding, deleting, wiping, changing settings and every
  token need the button, with the name or value on the display.
- The bootloader only opens after a physical gesture and refuses to touch the EEPROM or itself.
  A held button at reset (the bait and switch) never opens it.
- **Evil maid:** whoever holds the device briefly can do the button gesture and flash a firmware that copies the keys, then restore everything, and nothing would show. Set an update PIN (`trustctl flash --set-pin`, 4 to 15 characters) and the bootloader asks for it after the gesture; a wrong one starts the application after a wait of about 7 s, so each guess costs the whole gate plus that wait (a 4 digit PIN takes about 21 hours on average). The PIN is only needed to flash; the device works without it. Details in `CLAUDE.md`. It only counts together with the final lock bits.
- When a device is finished, set the read-out lock bits with `tools/user/final_lock.sh` (undo: `tools/user/unlock.sh`, which erases everything). Until then the idle screen shows an inverted `NOT LOCKED` and `trustctl hello` and `add` warn.

## Tests

- `firmware/test/run.sh`: host tests of the portable core.
- `cd trustctl && go test ./...`: the host tool against a simulated device.
- With the rig: `tools/build/core_test.py`, `tools/build/boot_test.py`, `tools/build/autowipe_test.py`,
  `tools/build/trustctl_e2e.py`, `tools/build/trustctl_flash_e2e.py` (the Uno does the button gestures).

## Releases

`trustctl` and the firmware are released together: every `trustctl` binary carries the core and the calibration firmware
(`trustctl/internal/firmware/`, embedded with `go:embed`), so users flash with `trustctl flash` (core, the default) or
`trustctl flash calibration` and need no other file. `trustctl version` shows both versions. `tools/build/release.sh` runs
`tools/build/embed_firmware.sh` (builds both firmwares, copies the images, takes the version from `firmware/core/core.ino`) and
cross-compiles all platforms into `dist/` with `SHA256SUMS`. Commit the embedded images with the code; a test fails if the
embedded version differs from the source. Bump `VERSION` in `core.ino` for every firmware change that is released.
