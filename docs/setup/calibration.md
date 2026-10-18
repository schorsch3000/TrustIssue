# Look and feel: font, logo, contrast

Everything about how the display looks is optional. The font, the idle logo and the two contrast
values are stored in a *parameter block* in the firmware's flash, which `trustctl flash` patches in from
a small profile on your computer. The **calibration firmware** lets you try them out live, without
reflashing for every attempt.

The profile is `~/.config/trustctl/device.json` (`--profile PATH` uses another one; JSON, you can
edit it by hand). **The device cannot be read back** (the lock bits forbid it), so the profile is the
source of truth: keep it. Flashing the core firmware without the profile (`--no-patch`) leaves the
defaults.

## Switch to the calibration firmware

```sh
trustctl flash calibration
```

It shows `Trust Issues` and `calibration`. Keys, timezone and settings survive: the calibration
firmware never touches the EEPROM, and flashing never erases the chip. Go back with
`trustctl flash` when you are done, so the new values get patched into the core firmware.

```{image} /_static/display/calibrate-home.png
:alt: The calibration firmware's start screen.
```

## Contrast

```console
$ trustctl calibrate contrast full 0x20
$ trustctl calibrate contrast dim 0
```

`full` is the brightness of the display while you use it and of the logo at first, `dim` the brightness
of the logo in its last two minutes before the display turns off. Values are 0 to 255 (or `0x..`).
On the first development display 0x20 and 0x00 were comfortable: 0x00 is not dark on an SSD1306, which
is what the dimmed phase wants. OLEDs lose brightness with use, which is why the idle logo is dimmed
and the display turns off.

```{list-table}
:widths: 50 50
* - ![Logo at full contrast](/_static/display/calibrate-contrast-full.png)

    `contrast full 0x20`
  - ![Logo dimmed](/_static/display/calibrate-contrast-dim.png)

    `contrast dim 0`
```

## Logo

```console
$ trustctl calibrate logo my-logo.png
$ trustctl calibrate logo my-logo.png --invert
```

The picture is streamed to the display in about a second and remembered for `flash`. Rules, which
`trustctl` keeps simple on purpose (thresholding, scaling and dithering are done in your image editor):

- The image must be 128x32 pixels or smaller; larger is an error. A smaller image is centered.
- Every pixel is on or off, with no dithering. For truecolor and grayscale images, exactly black is off
  and any other value, however dark (`#000001`), is on. Transparency counts as black.
- For indexed (palette) images the darkest palette color is off (the first one if several are equally
  dark); all other entries are on.
- `--invert` swaps lit and dark, including the margin around a smaller image.

```{list-table}
:widths: 50 50
* - ![A logo](/_static/display/calibrate-logo.png)

    `calibrate logo trust_issue_03.png`
  - ![The same inverted](/_static/display/calibrate-logo-invert.png)

    `calibrate logo trust_issue_03.png --invert`
```

A fast way to design one: watch the file and let every save show up on the display,

```sh
echo my-logo.png | entr -c trustctl calibrate logo my-logo.png
```

`trustctl logo extract logo.png` writes the current logo (the profile's, or with `--from FIRMWARE.hex`
the one in a firmware file) as a 128x32 PNG to start from; `calibrate forget logo` drops it. The demo
logos in the repository's `Demo-Logos/` directory are a starting point.

To show the logo on the finished device, turn it on there: `trustctl config set logo=on`, see
{doc}`../reference/trustctl`.

## Font

```console
$ trustctl font list
5x8
artossans8
artosserif8
chroma48medium8
torussansbold8
victoriabold8
victoriamedium8  (the firmware's default)
$ trustctl calibrate font victoriabold8
```

The font shows up on a test text that is built to expose lookalikes: `Il1`, `O0oQ`, `rn` against `m`,
`vv` against `w`, `GitHub` against `paypa1`. A confirm screen is only worth something if you can tell
`I`, `l` and `1` apart. Twelve fonts were compared on the real display and `victoriamedium8` is the
default. All built-in fonts:

```{list-table}
:widths: 50 50
* - ![victoriamedium8](/_static/display/calibrate-font-victoriamedium8.png)

    `victoriamedium8` (default)
  - ![victoriabold8](/_static/display/calibrate-font-victoriabold8.png)

    `victoriabold8`
* - ![chroma48medium8](/_static/display/calibrate-font-chroma48medium8.png)

    `chroma48medium8`
  - ![artossans8](/_static/display/calibrate-font-artossans8.png)

    `artossans8`
* - ![artosserif8](/_static/display/calibrate-font-artosserif8.png)

    `artosserif8`
  - ![torussansbold8](/_static/display/calibrate-font-torussansbold8.png)

    `torussansbold8`
* - ![5x8](/_static/display/calibrate-font-5x8.png)

    `5x8`
  -
```

Only the redistributable ones are built in (CC0 / public domain; sources and licenses are in
`trustctl/internal/fonts/README.md`). A font file of your own works wherever a name does: an 8x8 font
in the U8x8 format, 772 bytes (characters 32 to 127, each glyph eight column bytes with the least
significant bit on top). Tools to make one: {doc}`../tools/misc` (`font2png.py`, `png2font.py`,
`c64rom2font.py`), and `trustctl font extract FILE.bin [--name NAME | --from FILE.hex]` writes the
current or a built-in font as a starting point.

## Rotation and inversion

Whether the display is rotated by 180 degrees (it depends on how the case holds it) or inverted
(lit and dark swapped) is a **device setting**, stored in the EEPROM, not part of the profile. Try it live
on the calibration firmware and then set it on the core firmware:

```console
$ trustctl calibrate rotate on
$ trustctl calibrate invert on
$ trustctl config set rotate=on         # later, on the core firmware, with a button confirm
```

```{list-table}
:widths: 50 50
* - ![Rotated](/_static/display/calibrate-rotate.png)

    `calibrate rotate on`
  - ![Inverted](/_static/display/calibrate-invert.png)

    `calibrate invert on`
```

The bootloader reads the same setting, so its prompts follow the rotation and inversion too.

## Putting it on the device

```console
$ trustctl calibrate show           # what flash will patch in
$ trustctl flash                    # core firmware, patched
```

`flash --font NAME|FILE` overrides the profile's font for that flash and, once it succeeded, stores it
in the profile, so the profile always says what is on the device.
