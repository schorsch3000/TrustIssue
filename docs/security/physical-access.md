# Physical access

Earlier versions of the design treated anyone holding the device as an attacker who wins. That is no
longer the whole story: whoever has the device for a while can do less than before, and what remains is
spelled out here.

## What stops an attacker with the finished device

**No reading over ISP.** With the final lock bits ({ref}`final-lock`, lock byte `0xCC`) the programming
header cannot read flash or EEPROM. It still allows a chip erase, which destroys the keys too (`EESAVE` is
unprogrammed, so the EEPROM does not survive an erase).

**No reflashing without the button gesture.** The bootloader opens only after holding the button through a
reset, waiting for a bar, and a tap, tap, hold gesture ({ref}`the-gate`). A press that the host did not ask
for, a held button through a reset and a user mashing the button do not open it.

**The update PIN.** Without a PIN the gesture is all an attacker needs: two wires on the button pads and a
microcontroller can do it, flash a firmware that dumps the EEPROM over serial, restore the original firmware
and EEPROM, and nothing would show afterwards. With a PIN ({ref}`update-pin`, 4 to 15 characters), the
bootloader asks for it after the gesture. The device gives no sign whether a guess was right until a fixed
wait of about 7 seconds has passed, and nothing can read the PIN back, so **every guess costs the whole
gate (about 8 s) plus that wait**.

Each guess takes about 15 seconds (8 s gate plus 7 s wait), and on average half of all possibilities
have to be tried. The tables assume a PIN that was chosen **at random**; a PIN a person made up (a date,
a word) is found far sooner by trying likely candidates first. The PIN may be 4 to 15 printable ASCII
characters (the 95 characters from space to `~`).

**Numeric PIN** (10 possibilities per character):

| Length    | Possibilities | Average time to guess | Worst case |
| --------- | ------------- | --------------------- | ---------- |
| 4 digits  | 10^4          | about 21 hours        | 1.7 days   |
| 5 digits  | 10^5          | about 9 days          | 17 days    |
| 6 digits  | 10^6          | about 3 months        | 6 months   |
| 8 digits  | 10^8          | about 24 years        | 48 years   |
| 10 digits | 10^10         | about 2,400 years     | 4,800 years |

**Random password** from all 95 printable characters (upper and lower case, digits, symbols):

| Length        | Possibilities | Average time to guess        | Worst case                |
| ------------- | ------------- | ---------------------------- | ------------------------- |
| 4 characters  | 8.1 x 10^7    | about 19 years               | 39 years                  |
| 5 characters  | 7.7 x 10^9    | about 1,800 years            | 3,700 years               |
| 6 characters  | 7.4 x 10^11   | about 175,000 years          | 350,000 years             |
| 8 characters  | 6.6 x 10^15   | about 1.6 billion years      | 3.2 billion years         |
| 10 characters | 6.0 x 10^19   | about 10^13 years            | 3 x 10^13 years           |

So a four character random password already beats a seven digit PIN by a wide margin, and from eight
characters on the guessing attack is out of the question. The price is typing it: the PIN is entered on the
host at every update, within the 8 seconds the device waits (`trustctl flash --pin` asks before the gesture),
and a password you keep in a password manager is a password malware on that host can reach. A random 6 to 8
digit PIN or a short random password you can remember is a sensible middle.

**Warnings.** A device without the final lock bits shows `UNLOCKED` on its idle screen, one without a
PIN shows `NO PIN`, and `trustctl hello` and `add` warn in words.

**Wiping.** `trustctl wipe`, and the autonomous wipe if you enable it (a long hold at idle, no host
needed), destroy the keys, so a device that must not be taken can be emptied. The keys come back from
your offline backups.

## What is not covered

- **Long access.** Time is all the PIN buys. The USB port and the button are reachable by design, so a
  laptop on the USB port and a servo or solenoid on the button (or a wire to its pads) can try PINs
  unattended. Nothing on the device can notice or log it (no counter fits in the bootloader, and
  anything rewritable could be restored), and a tamper seal does not help while the port and the button
  are outside the case. What helps is a PIN length that matches how long the device could be out of your
  sight, and not leaving it unattended for that long.
- **Attacks on the chip itself:** decapping, voltage or clock glitching, probing, cold boot. Anyone with
  the equipment wins against an ATmega328P; this is accepted.
- **A user tricked into the gesture** on a screen the attacker induced (with a PIN the attacker still
  needs it). The rule: never complete the update gesture unless you started an update; an unexpected
  update screen means do nothing, it times out into the application.
- **A compromised host and an evil maid together:** the PIN is typed on the host at an update, so a
  keylogger there sees it. That is the narrow case that is accepted.
- **A stolen device** is not prevented, only made useless to read: you notice it is gone, rotate the
  passwords and reset the MFA of every account.

All of this only holds for a finished device: until the lock bits are set the programming header can
read everything.

## Why the bootloader is built the way it is

- It has **no flash read-back and no compare command**. The PIN lives in flash; a "does this page match"
  answer would be an oracle for guessing it at full speed. Verification is done instead by a checksum
  byte per page and a comparison of the written flash with the bootloader's own RAM buffer.
- The EEPROM is neither readable nor writable in update mode, and the bootloader's own section is
  protected.
- The EEPROM is **not** wiped on an update: wiping every time would push people to keep their key backups
  on the device. The gate has to hold on its own.
- A missing or dead display fails closed: the update mode never opens (recovery needs ISP).
- It is 2 KB, written from scratch, with its own tiny text-only display driver.

Signed firmware verified in the bootloader would remove the remaining risk of a tricked user, but costs
key management and flash space; it was deliberately skipped.
