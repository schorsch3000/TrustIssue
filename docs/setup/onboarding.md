# First programming (ISP)

A new Nano needs the Trust Issues **bootloader** and the **firmware** once, through the six pin ICSP
header. This is the one step that cannot be done over USB, because the stock Nano bootloader is
replaced by one that only opens after a gesture on the button. Afterwards every update goes over USB
with `trustctl flash`, see {doc}`flashing`.

:::{warning}
The first burn starts with a **chip erase**: everything on the chip is gone, including the keys. Do it
on a fresh device, or one whose keys you have backed up (there is no export, you need the original
secrets or QR codes).
:::

## What you need

- The wired device, see {doc}`../hardware/wiring`.
- An ISP programmer. Two ways are supported:
  - a **USBtinyISP** (or a compatible programmer, `ISP_ARGS` selects another with `avrdude`): the
    easy way, one script does everything;
  - a second **Arduino running the `ArduinoISP` sketch** (an Uno works): the way the project was
    developed.
- `avrdude` (on Debian and Ubuntu: `sudo apt install avrdude`).
- A copy of the repository (or the `tools/user` directory of a release archive). The scripts below are
  in `tools/user/`.
- Without `--bootloader FILE`, the scripts build the bootloader from `firmware/bootloader`, which needs
  the Arduino AVR toolchain. The release archives contain a ready `boot.hex` (in the firmware zip).

The ICSP header is the small 2x3 pin block on the Nano. The signals are on the Nano's header row, too:

| ICSP pin | Signal    | Same signal on the header row |
| -------- | --------- | ----------------------------- |
| 1        | MISO      | `D12`                         |
| 2        | VCC (5 V) | `5V`                          |
| 3        | SCK       | `D13`                         |
| 4        | MOSI      | `D11`                         |
| 5        | RESET     | `RST`                         |
| 6        | GND       | `GND`                         |

Pin 1 is the one marked on the board (a dot, a square pad or a `1`); the programmer's cable has its red
stripe there. Power the Nano either from the programmer (its target power jumper) or from its own USB
cable, **not from both**.

## With a USBtinyISP

```sh
tools/user/onboard_usbtiny.sh --yes
```

It checks the chip's signature, erases it, writes the bootloader and the core firmware (the one built
into `trustctl`) in one verified `avrdude` call, sets the fuses (low `0xFF`, high `0xDA`, extended
`0xFD`) and the development lock byte `0xCF`. Then the device works: no gesture and no second Arduino
needed. Options:

`--core FILE.hex`
: use your own firmware instead of the built-in one.

`--bootloader FILE.hex`
: use a ready bootloader image instead of building it.

`--final-lock`
: set the final lock bits at the very end, see {ref}`final-lock`. Only for a finished device.

`--erase-locked`
: allow erasing a device that already carries the final lock (this destroys its keys; without it the
  script refuses).

`ISP_ARGS="-c usbtiny -B 4"`
: another programmer or options for `avrdude` (default `-c usbtiny`).

## With an Arduino as ISP

1. Put the `ArduinoISP` example sketch (Arduino IDE, `File > Examples > 11.ArduinoISP`) on the Uno.
2. Wire the Uno to the Nano: Uno `D10` to Nano `RST`, `D11` to `D11`, `D12` to `D12`, `D13` to `D13`,
   and the grounds together. Nothing on `5V`: each board has its own USB cable. If `avrdude` fails to
   sync, a 10 uF capacitor between the Uno's `RESET` and `GND` stops it from resetting while `avrdude`
   talks to it (remove it again before uploading another sketch to the Uno).
3. Burn the bootloader:

   ```sh
   tools/user/flash_bootloader.sh /dev/ttyUSB1 --yes
   ```

   `/dev/ttyUSB1` is the Uno's port. This builds the bootloader, erases the chip, writes the bootloader,
   sets the fuses and the lock byte `0xCF` and verifies them. `--keep-eeprom` keeps the EEPROM
   (programs `EESAVE` for the erase and clears it afterwards; not possible on a locked chip), `--hex FILE`
   uses a ready bootloader image.
4. Put the firmware on it through the bootloader, see {doc}`flashing`. The bootloader gate needs
   the button gesture; a chip without a firmware starts it automatically.

## What the fuses and lock bits mean

| Setting                | Value  | Why                                                                                      |
| ---------------------- | ------ | ---------------------------------------------------------------------------------------- |
| low fuse               | `0xFF` | the stock value                                                                          |
| high fuse              | `0xDA` | `BOOTRST` set (the chip always starts in the bootloader), 2 KB boot section, **`EESAVE` unprogrammed** |
| extended fuse          | `0xFD` | the stock value                                                                          |
| lock byte (development) | `0xCF` | the bootloader cannot be overwritten from the application; ISP can still read the chip   |
| lock byte (final)      | `0xCC` | no reading or programming over ISP except a chip erase; see below                        |

`EESAVE` must stay unprogrammed: that is what makes a chip erase destroy the EEPROM (the keys) too,
so a locked chip cannot be "unlocked" while keeping them.

(final-lock)=
## The final lock

While the lock bits are only `0xCF`, anyone with a programmer can read the whole chip, EEPROM and keys
included. That is fine for a development device and the idle screen says so (`UNLOCKED`). When the
device is finished and has its real keys:

```sh
tools/user/final_lock.sh /dev/ttyUSB1 --yes        # the Uno's port
ISP_ARGS="-c usbtiny" tools/user/final_lock.sh - --yes   # with a USBtinyISP
```

It checks the fuses first, then sets the lock byte `0xCC`. **This is irreversible** except by a chip
erase, which destroys the flash, the bootloader and the EEPROM: `tools/user/unlock.sh PORT --yes`
(the fuses stay), followed by a new bootloader burn. After the final lock the `UNLOCKED` warning is
gone from the idle screen and from `trustctl hello`. A PIN, font or logo can still be changed through
the bootloader gate afterwards.

Set an update PIN (see {ref}`update-pin`) before or right after: without it the physical gesture is all
an attacker with the device in their hands needs.
