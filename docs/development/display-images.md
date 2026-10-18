# The display pictures

The pictures and animations in this documentation are not mock-ups. They are made by running the **real
firmware** on a computer:

```{list-table}
:widths: 28 72
* - `firmware/hostsim/hostsim.cpp`
  - Compiles `firmware/core/core.ino` **unchanged** against small mock versions of the Arduino API, the
    EEPROM, `Wire` and U8x8 (`firmware/hostsim/mock/`). The mock display is an SSD1306 framebuffer; the clock
    is virtual; a script on standard input says what the host sends (`serial ADD ...`), when the button is
    pressed (`hold 1500`, `tap`) and how long to wait. Slow things take their real time: an EEPROM write
    advances the clock by 3 ms, which is what makes the wipe progress bar visible.
* - `firmware/hostsim/bootsim.cpp`
  - Does the same for the **bootloader** (`firmware/bootloader/boot.c`; only its two assembler lines are
    replaced by `build_boot.sh`): fake ATmega registers, a TWI-level SSD1306 emulation and a timeline of
    button presses and bytes from the host, so the gate, the PIN screen and the aborts are the code that
    runs on the chip.
* - `tools/docs/displaysim.py`
  - Turns the recorded display contents into pictures: the 128x32 frame as a grid of pixels (contrast
    shown as brightness, a display that is off as black), and for GIFs a strip with the command, the
    device's replies and the button with the time it has been held.
* - `tools/docs/gen_display_images.py`
  - The scenarios: which keys exist, what the host sends, how long the button is held. It writes
    `docs/_static/display/*.png` and `*.gif`.
```

The calibration pictures (`calibrate-*.png`) are drawn directly from the font and logo data, since
the calibration firmware only draws text with a font and raw bitmaps.

## Regenerating

```sh
tools/docs/gen_display_images.py                  # everything
tools/docs/gen_display_images.py --only token add # some scenarios (names are in the script)
tools/docs/gen_display_images.py --check          # fail if a committed picture is out of date
```

It needs `g++` and Python with Pillow. The output is deterministic (the virtual clock has no
jitter), so `--check` can run in CI: when a firmware change alters a screen, the check fails until the
pictures are regenerated and committed.

## Adding a picture

Add an entry to `SCENARIOS` (core firmware) or `BOOT_SCENARIOS` (bootloader):

```python
"del": dict(setup=keys(3),                         # the device as it is before: three keys
            cmd="trustctl del Gitea",              # shown in the caption of the GIF
            clip=["serial DEL 1", "wait 400", "hold 1500", "wait 1800"],
            shot_at={"del-confirm": 300}),         # a still PNG 300 ms into the clip
```

Script commands for the core simulation: `serial LINE`, `wait MS`, `hold MS`, `tap [PAUSE]`, `press`,
`release`, `mark shot NAME` and `mark cap TEXT` (a caption), `record on|off`, `pin set|none`,
`unlocked on|off`, `logo FILE`, `font FILE`, `contrast FULL DIM`, `eeprom_save|eeprom_load FILE`, `boot`. For
the bootloader: `at MS press|release`, `at MS uart TEXT`, `at MS cap TEXT`, `pin set|none`,
`firmware present|absent`, `config HEX`, `limit MS`. The scripts print every display change, button
change and serial line with its time, which also makes the simulation handy for debugging a screen:
`firmware/hostsim/build.sh && printf 'boot\nwait 100\n' | firmware/hostsim/build/hostsim`.
