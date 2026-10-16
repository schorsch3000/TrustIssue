# Wiring a Trust Issues device

A Trust Issues device is an Arduino Nano, a small OLED display and one push button. Everything is
wired on one side of the Nano, the side with the analog pins (labels `A0` to `A7`), so the wires
stay tidy and short. There are only five connections to the Nano plus the button.

## What you need

| Part                                                  | Notes                                                                                          |
| ----------------------------------------------------- | ---------------------------------------------------------------------------------------------- |
| Arduino Nano                                          | the ATmega328P version (not the 168, not the newer Nano Every); the chip print says ATmega328P |
| 0.91" OLED display, 128x32 pixels, I2C                | usually an SSD1306, address 0x3C; four pins: GND, VCC, SCL, SDA                                |
| Push button                                           | any momentary switch; a 4-pin tactile switch works, see below                                  |
| Four jumper wires for the display, two for the button |                                                                                                |
| USB cable                                             | Mini-USB or Micro-USB, whichever your Nano has; it powers the device and carries the data      |

No resistors and no capacitors are needed: the display module has its own I2C pull-up resistors,
and the button uses the Nano's internal pull-up.

## Connections

| From       | To                    | Wire                                                                             |
| ---------- | --------------------- | -------------------------------------------------------------------------------- |
| Nano `5V`  | display `VCC`         | power (if your display module says 3.3 V only, use the Nano's `3V3` pin instead) |
| Nano `GND` | display `GND`         | ground                                                                           |
| Nano `A4`  | display `SDA`         | I2C data                                                                         |
| Nano `A5`  | display `SCL`         | I2C clock                                                                        |
| Nano `A3`  | button, one leg       | the button signal                                                                |
| Nano `GND` | button, the other leg | ground (the same `GND` pin as the display can be shared)                         |

That is all. The button has no polarity: it does not matter which leg goes where, only that one
leg goes to `A3` and the other to `GND`.

## The picture

The Nano's analog-side header row, as the silkscreen labels read from the end that has `VIN`
(which side of the board this is depends on how you hold it):

```
 VIN  GND  RST  5V  A7  A6  A5  A4  A3  A2  A1  A0  REF  3V3  D13
       |         |           |   |   |
       |         |           |   |   +------------------------ button (one leg)
       |         |           |   +---------------------------- display SDA
       |         |           +-------------------------------- display SCL
       |         +-------------------------------------------- display VCC
       +------------------------------------------------------ display GND and the button's other leg
```

The same as a connection diagram:

```
      Nano                           OLED 128x32
  +-----------+                    +-------------+
  |        5V |--------------------| VCC         |
  |       GND |---+----------------| GND         |
  |        A4 |--------------------| SDA         |
  |        A5 |--------------------| SCL         |
  |           |   |                +-------------+
  |        A3 |---|---+
  +-----------+   |   |
                  |   |   push button (either way round)
                  |   +----o   o----+
                  +-----------------+
```

The three pins `A3`, `A4`, `A5` sit next to each
other, and `5V` and `GND` are on the same row, so the whole device is wired along one edge.
`A6` and `A7` look like handy neighbors but cannot be used for the button: they are analog-only
inputs without a pull-up. The button must be on `A3` (or `A0` to `A2` if you change the
firmware); a different pin does nothing.

## Check the display's pin order

Display modules differ. The pins are not always in the order shown on a drawing: common orders
are `GND VCC SCL SDA` and `VCC GND SCL SDA`. Read the tiny silkscreen letters on the module and
connect by name, not by position. Swapping `VCC` and `GND` can destroy the display. If the
display stays dark after the firmware is flashed, check `SDA` and `SCL` first (they are easy to
swap; swapping them does no harm).

## The button

A 4-pin tactile switch has two pairs of legs that are connected inside. Use two legs that are
diagonally opposite each other: those two are always connected only while the button is pressed.
Two legs on the same side can be connected permanently, which makes the device think the button
is held down (it then never starts normally and shows the update screens).

How the button is used: pressing and releasing inside a window of 1 to 2 seconds confirms a
request that the device shows on its display; a wipe needs 5 to 6 seconds. See `README.md`.

## Power and USB

Plug the Nano into the computer with its own USB cable. Do not connect any other 5 V source to the
Nano's `5V` pin at the same time as the USB cable, and do not power the display from anywhere
else than the Nano.

## Programming header (once, for a new Nano)

A new Nano needs the Trust Issues bootloader and firmware once, through the six-pin ICSP header
on the board (the small 2x3 pin block near the end of the board opposite the USB connector). A
USBtinyISP programmer does it with `tools/user/onboard_usbtiny.sh`. The six signals are:

| ICSP pin | Signal    | Same signal on the Nano's header row |
| -------- | --------- | ------------------------------------ |
| 1        | MISO      | `D12`                                |
| 2        | VCC (5 V) | `5V`                                 |
| 3        | SCK       | `D13`                                |
| 4        | MOSI      | `D11`                                |
| 5        | RESET     | `RST`                                |
| 6        | GND       | `GND`                                |

Pin 1 is the one marked on the board (a dot, a square pad or a `1`; check the silkscreen); the
programmer's cable has its red stripe on pin 1. Power the Nano either from the programmer (its target-power jumper on) or from its own USB
cable, not from both at once. The ICSP header is only needed for this one-time step; afterwards the Nano is updated
over USB with `trustctl flash` (hold the button while it resets, see `README.md`).

## For developers: the test rig

The development setup (an Arduino Uno as ISP programmer and as the "finger" that presses the
button for the automatic tests) is described in `CLAUDE.md`, section "Development setup". Its
button wire is the Uno's `D4`, through a 1 kOhm resistor to the Nano's `A3`.
