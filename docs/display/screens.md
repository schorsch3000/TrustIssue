# Screens of the device

Every screen the device can show, as the real firmware draws it (see {doc}`../development/display-images`
for how these pictures are made). The display is 128x32 pixels: four rows of sixteen characters.

(idle-screen)=
## The idle screen

```{list-table}
:widths: 50 50
* - ![Idle screen with no keys](/_static/display/idle-empty.png)

    A finished device with no keys.
  - ![Idle screen with three keys](/_static/display/idle-keys.png)

    With keys: how many of the 20 slots are used.
* - ![Idle screen of a development device](/_static/display/idle-development.png)

    A device that is not finished: inverted `UNLOCKED` (the lock bits are not set, so the keys can be
    read over the programming header) and `NO PIN` (no update PIN is set).
  - ![Idle screen with the version](/_static/display/idle-version.png)

    With `trustctl config set show-version=on`: the firmware version on the second row.
```

The warnings are a reminder for the owner of the device, not a security boundary (a malicious
firmware could hide them, but flashing one needs the button gesture and the PIN). `trustctl hello` and
`trustctl add` print the same warnings on the computer. See {doc}`../security/physical-access`.

### Idle logo and sleep

After 10 seconds the display goes to sleep. With `trustctl config set logo=on` (and a logo in the
device's parameter block, see {doc}`../setup/calibration`) it first shows the logo, and shows it at
power-up too:

```{image} /_static/display/idle-logo.gif
:alt: Power-up: the logo for 5 seconds, the idle screen, then the logo at full contrast, dimmed, and finally the display turns off.
```

Without the logo, the display simply turns off after the idle time. A button press wakes it:

```{image} /_static/display/idle-sleep.gif
:alt: The idle screen turns off after 10 seconds, a button press wakes it.
```

## Confirm screens

All of these wait for the button; hold 1 to 2 seconds (wipe: 5 to 6) and let go while the bar is a
checkerboard, see {doc}`../concepts`.

```{list-table}
:widths: 50 50
* - ![Add key](/_static/display/add-confirm.png)

    `trustctl add`: the name and the parameters.
  - ![Delete key](/_static/display/del-confirm.png)

    `trustctl del`.
* - ![Token request](/_static/display/token-confirm.png)

    `trustctl token`: the name, the timezone offset in the corner, and the local date and time the
    code is for.
  - ![Token request with a long name](/_static/display/token-longname.png)

    Names of up to 27 characters wrap over two lines; the whole name is always visible.
* - ![Set timezone](/_static/display/tz-confirm.png)

    `trustctl tz set` or `tz sync`: the one place where you must know the correct offset.
  - ![Wipe](/_static/display/wipe-confirm.png)

    `trustctl wipe`: hold 5 to 6 seconds.
* - ![Settings](/_static/display/config-flags.png)

    `trustctl config set`: all five settings, those this request changes in inverse.
  -
```

## Results

```{list-table}
:widths: 25 25 25 25
* - ![Done](/_static/display/result-done.png)
  - ![Denied](/_static/display/result-denied.png)
  - ![Too long](/_static/display/result-too-long.png)
  - ![Timeout](/_static/display/result-timeout.png)
* - Accepted.
  - Let go too early (before 1 s, or a tap).
  - Held past the end of the window: denied at once.
  - Nobody answered for 30 s.
```

After an accepted code request, `Done` is followed by the code in large digits for five seconds:

```{image} /_static/display/token-code.png
:alt: The result screen of a code request: Done and the six digit code.
```

## Wiping without a computer

With `trustctl config set autowipe=on` the keys can be destroyed with the button alone (a power bank is
enough). Hold the button at idle: after 2 seconds a bar appears, and you let go between 10 and 11
seconds. Then a four second countdown (a press cancels), then the same gesture as the bootloader's:
tap, tap, press a third time and hold for 2 seconds. Any mistake starts the gesture over, and
after 20 seconds it is cancelled.

```{list-table}
:widths: 33 33 33
* - ![Hold 10 s](/_static/display/autowipe-hold.png)
  - ![Countdown](/_static/display/autowipe-countdown.png)
  - ![Gesture](/_static/display/autowipe-gesture.png)
* - The bar to the end of the 10 to 11 s window.
  - A draining bar; any press cancels.
  - A third of the bar per tap; the hold fills the rest.
```

```{image} /_static/display/autowipe.gif
:alt: The whole autonomous wipe: hold 10 seconds, countdown, tap, tap, hold, wiping.
```

## Bootloader screens

The bootloader draws its own screens when the button is held while the device resets. They are
described with the flashing procedure in {ref}`the-gate`.
