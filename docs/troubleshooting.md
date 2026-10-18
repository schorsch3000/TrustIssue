# Troubleshooting

## The display stays dark

- Check `SDA` and `SCL` first: they are easy to swap (swapping them does no harm). Then the power pins.
  Display modules differ in pin order (`GND VCC SCL SDA` or `VCC GND SCL SDA`); read the silkscreen. Swapping
  `VCC` and `GND` can destroy the module.
- The display must answer at I2C address `0x3C` (most 0.91" 128x32 SSD1306 modules do).
- The idle display turns off after 10 seconds (or after the logo phase): press the button.
- A dead or absent display makes the **bootloader fail closed**: with a firmware present the application
  starts, but the update gate never opens.

## The device shows the update screens at power-up, or never starts normally

The button reads as pressed. Check its wiring: on a 4-pin tactile switch use two legs that are diagonally
opposite; two legs on the same side can be permanently connected ({doc}`hardware/wiring`).

## `trustctl: no device` (exit status 69)

Give the port: `--device /dev/ttyUSB0` or `device =` in `~/.config/trustctl/config`. You must be allowed to
open the port (`dialout` group on Debian and Ubuntu). Cheap Nano clones change their port number after a replug.

## `trustctl: device: the device is waiting for a confirmation of another request` (`BUSY`)

A request is still pending on the device (for up to 30 seconds), or the autonomous wipe is running. Finish
or cancel it on the device, or wait for the timeout. Only one request at a time; two programs using the
same device collide this way.

## It says "refused on the device" or "Denied" though I pressed

The hold must be released between 1 and 2 seconds (wipe: 5 to 6). Watch the bar: let go while it is a
checkerboard. A tap, or letting go before the checkerboard, is a denial; holding past the end of the
bar is "Too long". If the button was already down when the request arrived, that press is ignored: let go and press
again.

## The code is wrong

- Compare the date and time on the device with your clock. A wrong timezone offset shows up there;
  `trustctl tz sync` fixes it. The device has no clock: the time comes from the host, so a wrong host
  clock gives wrong codes.
- The key was added with the wrong parameters (digits 6 to 8, period 30 or 60 seconds): delete and add it again.
- A secret over 20 bytes, SHA-256/512 or HOTP cannot be stored.

## Flashing does not start or times out

- `trustctl flash` waits 90 seconds (`--wait`). Do the gesture in {ref}`the-gate`: release at
  `RELEASE BUTTON`, do not touch the button while the bar drains, then tap, tap, hold.
- Some clones reset only on RTS, some only on DTR; `trustctl` pulses both.
- With an update PIN the device waits only **8 seconds** for it: use `trustctl flash --pin` so that it is
  asked before the gesture.
- "No update mode 12 seconds after the PIN" means the PIN was wrong.
- Use a good USB cable and check the contacts: a loose connection is a likely cause of a failed upload.

## `unlocked` or `nopin` in `trustctl hello`

The device is not finished: see {doc}`setup/first-use`.

## I forgot the update PIN

The device keeps working but cannot be flashed. Add every key again somewhere else, then erase and
reprogram the chip over ISP ({doc}`setup/onboarding`, `unlock.sh`). This destroys the keys on the device.

## The device was reset when I opened the port

On Linux, macOS, FreeBSD and OpenBSD `trustctl` opens the port without toggling DTR, so only the first open after
plugging in resets the device. On Windows every open resets it until the daemon holds the port.
