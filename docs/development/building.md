# Building from source

## Layout

| Path                   | What                                                                                       |
| ---------------------- | ------------------------------------------------------------------------------------------ |
| `firmware/core`        | the application: serial protocol, confirm screens, idle logo, autonomous wipe              |
| `firmware/trustcore`   | portable C++ core (SHA-1, HMAC, TOTP, Base32, EEPROM storage, protocol parser, time)        |
| `firmware/bootloader`  | the button-gated bootloader (2 KB, from scratch)                                           |
| `firmware/calibration` | the calibration firmware                                                                   |
| `firmware/hostsim`     | host builds of the core firmware and the bootloader (for the display pictures)             |
| `firmware/rigisp`      | the test rig: ArduinoISP plus a button finger                                              |
| `trustctl`             | the host tool in Go, and `trustctl-import`                                                 |
| `tools/user`           | tools for people who use the device                                                        |
| `tools/build`          | building, testing and releasing                                                            |
| `tools/docs`           | the documentation's display pictures                                                       |
| `docs`                 | this documentation                                                                         |

## Firmware

You need `arduino-cli` with the `arduino:avr` core (1.8.8) and the U8g2 library (2.36.19, for the calibration
firmware).

```sh
firmware/core/build.sh               # build/core.ino.hex; "upload PORT" uploads through a stock bootloader
firmware/calibration/build.sh
make -C firmware/bootloader          # boot.hex, fails if it does not fit in 2048 bytes
tools/build/embed_firmware.sh        # builds both and copies them into trustctl/internal/firmware
```

`core/build.sh` enlarges the serial receive buffer to 128 bytes (a whole `ADD` line must survive a display
redraw), places the parameter block at `0x7000`, builds the autonomous wipe in (`AUTONOMOUS_WIPE=0`
leaves it out) and passes `--clean` so that a cached core built with other flags is never reused. Board:
`arduino:avr:nano:cpu=atmega328old` (the choice only affects compile settings, the bootloader is ours).
Bump `VERSION` in `core.ino` for every released firmware change; the embedded images are committed and a
test fails if the embedded version differs from the source.

## trustctl

```sh
cd trustctl
go build -o bin/trustctl ./cmd/trustctl
go test ./...
```

`tools/build/release.sh [--quick]` cross-compiles every platform into `dist/` with `SHA256SUMS`,
`trustctl-import` into `dist/import/`; `tools/build/package.py VERSION` makes the archives.

## This documentation

```sh
python3 -m venv .venv && . .venv/bin/activate
pip install -r docs/requirements.txt
sphinx-build -W -b html docs docs/_build/html
```

Read the Docs builds it from `.readthedocs.yaml`; the display pictures are committed, so the build needs
no compiler. See {doc}`display-images` for regenerating them.
