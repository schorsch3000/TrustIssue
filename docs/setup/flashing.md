# Flashing over USB

After the first programming ({doc}`onboarding`) the firmware is updated over the USB cable with
`trustctl flash`. The bootloader only opens after a gesture on the button, so a program on the
computer can never reflash the device, whatever it does with the serial port.

```console
$ trustctl --device /dev/ttyUSB0 flash
Flashing the built-in core firmware 0.2.1.
Flashing 192 pages (24576 bytes) through the bootloader of /dev/ttyUSB0.
HOLD THE BUTTON on the device and keep holding it.
Resetting the device in 4...
```

`flash` (or `flash core`) writes the core firmware that is built into `trustctl`, `flash calibration`
the calibration firmware ({doc}`calibration`), and `flash FILE.hex` a firmware file of your own. Keys,
the timezone and the settings are kept: flashing only rewrites flash pages and never erases the chip.

(the-gate)=
## The gesture, step by step

1. **Hold the button** and keep holding it. `trustctl` counts down and resets the device.
2. The display says `RELEASE BUTTON`, inverted. **Let go.** (Holding through a reset never goes
   further: that is what stops malware from interrupting your real code request.)
3. `FIRMWARE UPDATE` and `WAIT`: a bar drains over about four seconds. Do not touch the button; a press
   aborts the gate.
4. `TAP TAP HOLD`: **tap, tap, then press a third time and hold** until the bar is full. The bottom row
   lights up one third per tap, and fills for the rest while you hold. Taps must be shorter than 0.6
   seconds and follow each other within 0.8 seconds. Any mistake clears the bar and you start the
   gesture over.
5. `RELEASE BUTTON` again. **Let go.** The update mode opens (`UPDATE MODE`) and `trustctl` writes the
   pages. Every wait in the gate has a timeout of 15 seconds that ends in a reset into the application.

```{image} /_static/display/flash-gate.gif
:alt: The bootloader gate: release the button, wait for the bar, tap, tap, hold, update mode.
```

`trustctl` waits up to 90 seconds (`--wait`) for the bootloader to open, so there is time to do the gesture
without hurry. It prints what to do. When the gesture is done it writes the pages, each with a
checksum byte; the bootloader verifies every page in place and `trustctl` resends a page up to three
times if it is refused.

## If something goes wrong

The mistakes are meant to be harmless. A press while the bar drains stops the gate:

```{image} /_static/display/flash-aborted.gif
:alt: The button is pressed while the WAIT bar drains and the gate gives up.
```

A hold that is too short clears the bar and the gesture starts over:

```{image} /_static/display/flash-mistake.gif
:alt: The third press is released too early, the bar goes back to empty and the gesture starts over.
```

And you can always do nothing: the device times out into the application. **Never complete the update
gesture unless you started an update.** An unexpected update screen means: do nothing.

(update-pin)=
## The update PIN

Without a PIN the gesture is all it takes to flash. An attacker who has the device unnoticed for a few
minutes (two wires to the button pads and a microcontroller are enough) could flash a firmware that
reads the keys, restore the original, and leave no trace. An **update PIN** (4 to 15 printable
characters) makes that slow: see {doc}`../security/physical-access`.

```console
$ trustctl flash --set-pin              # sets a PIN with this flash (asked for twice)
$ trustctl flash pin --set-pin          # changes only the PIN, the firmware stays
$ trustctl flash --clear-pin            # removes it (the current PIN is asked for first)
$ trustctl flash --pin                  # asks for the current PIN before the gesture
```

With a PIN set, the device asks for it after the gesture (`FIRMWARE UPDATE`, `PIN`). `trustctl` asks
for the PIN without echo and sends it. **The device waits only 8 seconds for it**, so type it quickly or
use `--pin`, which asks before the gesture.

Whether the PIN is right or wrong, the bar on the PIN screen drains over about seven seconds, and only
then does the device either open the update mode or simply start the application. Nothing in between
tells a guess apart, so the attacker's guesses cost the whole gate plus seven seconds each.

```{image} /_static/display/flash-gate-pin.gif
:alt: The gate with an update PIN: after the gesture the PIN screen shows a bar that runs for 7 seconds, then the update mode opens.
```

A wrong PIN looks the same until the very end:

```{image} /_static/display/flash-wrong-pin.gif
:alt: A wrong PIN: the same screen and bar, and then the application starts instead of the update mode.
```

`trustctl` tells the two apart by waiting for the update mode; with a wrong PIN it reports that after
about 12 seconds. A forgotten PIN is not a disaster: the device keeps working (keys can be added and
deleted), but it can no longer be flashed. Add every key again somewhere else, then erase and reprogram
the chip over ISP (`tools/user/unlock.sh`, which erases everything, and {doc}`onboarding`).

The PIN is stored in the last page of the application area (`0x7780`), which `trustctl flash` refuses
to touch except through `--set-pin` and `--clear-pin`, and it is never readable from the host: the update
mode has no read-back and no compare command.

## Other flash options

`--font NAME|FILE.bin`
: patch a font in for this flash (see {doc}`calibration`); remembered in the profile afterwards.

`--no-patch`
: flash the firmware as it is, without the font, logo and contrast from the profile.

`--profile PATH`
: use another profile file.

`--wait DURATION`
: how long to wait for the bootloader (default `90s`).

`--countdown N`
: seconds between "hold the button" and the reset (default `4`).

All options with their environment variables are in {doc}`../reference/trustctl`.
