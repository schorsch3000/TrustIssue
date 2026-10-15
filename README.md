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
| `firmware/rig`, `firmware/button_test` | the Uno test rig that presses the button and resets the Nano |
| `trustctl` | the host tool in Go: `list`, `token`, `add`, `del`, `tz`, `config`, `wipe`, `flash`, `calibrate`, `font extract`, `logo extract`, `daemon` |
| `tools` | end-to-end tests, the Python prototypes, `dummy-keys.html` (fake keys to try the device with) |

## Hardware

Arduino Nano (ATmega328P, 16 MHz), SSD1306 128x32 OLED on I2C (A4 SDA, A5 SCL, address 0x3C), a
push button from D2 to GND. For development a second Arduino (an Uno) acts as ISP programmer and as
the "finger": its D4 goes through 1 kOhm to the Nano's D2, its D10 to the Nano's RST, and D11 to D13
to the Nano's D11 to D13 (full table in `CLAUDE.md`).

## Setting up a device

1. **Bootloader (once, over ISP).** Put the `ArduinoISP` example sketch on the Uno, then
   `make -C firmware/bootloader` and
   `avrdude -c stk500v1 -p m328p -P <uno> -b 19200 -e -U flash:w:firmware/bootloader/boot.hex:i -U lock:w:0xCF:m`.
   The fuses of a stock Nano (`hfuse 0xDA`) are right already.
2. **Host tool.** `cd trustctl && go build -o bin/trustctl ./cmd/trustctl` (a static binary; Go 1.25+, which
   the module fetches by itself).
3. **Firmware.** `firmware/core/build.sh`, then
   `trustctl --device /dev/ttyUSB0 flash firmware/core/build/core.ino.hex`. It tells you what to do on
   the device: hold the button while it resets, release when the display says so, wait for the bar,
   then tap, tap, press a third time and hold until the bar is full.
4. **Look and feel (optional).** Build `firmware/calibration/build.sh` and flash it the same way
   (`trustctl flash firmware/calibration/build/calibration.ino.hex --no-patch`), then
   `trustctl calibrate contrast full 0x20`, `calibrate contrast dim 0`, `calibrate logo logo.png`,
   `calibrate font firmware/fonts/victoriamedium8.bin` (the options like `--device` may stand before or after
   the command), and flash the core firmware again; the settings are
   patched in from `~/.config/trustctl/device.json`.
5. `trustctl config set logo=on` for the idle logo.

Put `device = /dev/ttyUSB0` into `~/.config/trustctl/config` to leave out `--device`. `trustctl help-options` lists every
option with its `--argument`, environment variable and config file syntax, and the defaults.

To start from what the device uses now, `trustctl font extract font.bin` writes the current font (the profile's, else the
built-in one) and `trustctl logo extract logo.png` writes the current logo as a 128x32 PNG (white = lit). The device itself
cannot be read back, so "current" means what `flash` would patch in; `--from firmware.hex` takes it from the parameter block
of a firmware file instead.

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

To try it with fake keys, open `tools/dummy-keys.html` in a browser.

### A menu for Linux and BSD

`tools/trustctl-menu` is a small POSIX shell script: it lists the keys in a dmenu-like launcher, asks the
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
- When a device is finished, set the read-out lock bits with `tools/final_lock.sh`.

## Tests

- `firmware/test/run.sh`: host tests of the portable core.
- `cd trustctl && go test ./...`: the host tool against a simulated device.
- With the rig: `tools/core_test.py`, `tools/boot_test.py`, `tools/autowipe_test.py`,
  `tools/trustctl_e2e.py`, `tools/trustctl_flash_e2e.py` (the Uno does the button gestures).
