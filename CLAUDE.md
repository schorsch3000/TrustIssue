# Trust Issues: project notes

Hardware TOTP token vault for people who don't trust their computers. See `branding.md` for naming and tone (Core = device firmware, `trustctl` = CLI client, Trust Desk = GUI).

This file records the design decisions made so far in brainstorming. Nothing beyond the I2C scanner sketch is implemented yet. Items marked **OPEN** are undecided.

## Goal

Keep many TOTP secrets on a small dedicated device instead of the phone (manual typing) or the laptop (too exposed).
The host talks to the device over the USB serial console and can:

- add a key (name + secret + parameters)
- delete a key
- list key names (so a GUI can show them; names are not secret)
- request the current token for a key
- read and set the device's timezone offset

Every sensitive operation needs physical confirmation on the device. The display shows the key name; the user must press the button to approve.

## Threat model

- **In scope:** a compromised host (malware on the laptop). It controls the serial port, DTR/reset, and timing.
- **Out of scope:** physical access to the device. Anyone holding the device (ISP, decapping, glitching) wins; this is accepted and not to be discussed further.
- TOTP stays phishable: the token passes through the host. The gain is that secrets can never be exported and each token needs a human press.

## Hardware

- Arduino Nano (ATmega328P, 32 KB flash, 2 KB RAM, 1 KB EEPROM, 16 MHz). No RTC.
- 0.91" OLED, I2C, 128x32 (usually SSD1306 at 0x3C; controller not yet confirmed). Text grid: 4 rows of 8x8 characters, 16 columns.
  - The 128x32 init differs from 128x64 (multiplex ratio 31, COM pins config 0x02); the driver must handle this.
  - A 0.96" 128x64 display is on hand for development; keep display code resolution-agnostic where possible.
- One push button.

Wiring:

| Nano        | Connects to                                  |
| ----------- | -------------------------------------------- |
| 5V (or 3V3) | OLED VCC                                     |
| GND         | OLED GND, button leg 2                       |
| A4 (SDA)    | OLED SDA                                     |
| A5 (SCL)    | OLED SCL                                     |
| D2          | button leg 1 (`INPUT_PULLUP`, pressed = LOW) |

- No external resistors: I2C pull-ups are on the display module, button uses the internal pull-up.
- Check the module's silkscreen for pin order (often GND-VCC-SCL-SDA, not always).
- Keep D10-D13 free (ICSP: reset, MOSI, MISO, SCK). D13 is also the onboard LED.
- A 4-pin tactile switch: wire diagonally opposite legs.

## Storage layout (EEPROM, 1 KB)

- 20 fixed slots, about 51 bytes each, plus a 4 byte header: byte 0 magic (4 bit) + layout version (4 bit), byte 1 timezone offset, byte 2 configuration flags (see Configuration), byte 3 CRC over bytes 0-2. A header with a bad CRC falls back to compile-time defaults; it never touches the key slots (each slot has its own CRC).
- Timezone offset: one signed byte in 15-minute units (covers UTC-12 to UTC+14 including 45-minute zones). Plain fixed offset only; no DST rule on the device (see Time and timezone).
- Slot (51 bytes, implemented in `tc_storage`): +0 flags (bit0 used, bits1-2 algorithm, bits3-4 digits-6, bit5 period 60 s), +1 secret (20 bytes, zero padded), +21 name (28 bytes, NUL padded), +49 CRC-16/CCITT over bytes 0..48. Header: byte 0 magic 0xA + layout version 1, byte 1 timezone (int8, 15-minute units), byte 2 config flags, byte 3 CRC-8.
- No secret length field: a shorter HMAC key padded with zeros to the block size gives the same MAC, so a 16-byte secret stored zero padded in 20 bytes yields identical codes. Secrets longer than 20 bytes are rejected for now (`ERR BADARG`); SHA256/512 secrets are a later extension.
- Crash safety: the CRC is written last and a slot is used only if the used bit is set and the CRC matches, so an interrupted add leaves an unused slot, never a half-written key. Delete clears the flags first, then zeroes secret, name and CRC. Blank (0xFF) or garbled EEPROM simply reads as "no keys".
- Names: 1 to 27 characters from `A-Z a-z 0-9 . : -` and space (66 symbols). Mixed case is kept for display ("GitHub"), but uniqueness is case-insensitive: `ADD` of a name that equals an existing one apart from case is rejected with `ERR DUPLICATE`. No leading or trailing space and no double spaces (otherwise two names would look identical on screen). The device validates all of this itself (`ERR BADARG`); `trustctl` checks first and gives a readable message. Longer names or other characters (umlauts, emoji) are rejected, never truncated or silently changed, because truncation could create duplicates.
- Lookalikes inside the set (`l`/`1`/`I`, `O`/`0`) remain possible; the display font should draw them distinctly (slashed or dotted zero, different `l`/`1`/`I`). **OPEN:** compare a few fonts on the real display.
- Fixed slots chosen over variable-length records for simplicity and safe deletes. The slot index is the key id.
- 16-byte secrets free 4 more bytes for the name; secrets above 20 bytes (SHA256/512 setups) eat into it. SHA1 is the main target; SHA512 is tight on 2 KB RAM.
- EEPROM wear (about 100k cycles) is irrelevant: writes happen only on add/delete.
- A bigger MCU (ESP32-S3/C3 with flash encryption and secure boot, or external I2C EEPROM) was considered; the Nano is the chosen target. The classic ESP8266 D1 mini is not suitable (no flash encryption, host-triggerable ROM bootloader, flash dumpable with esptool).

## Configuration

One flags byte in the EEPROM header, readable and writable over serial (`GETCFG` / `SETCFG`, the latter needs a button confirm like every mutating command). Defaults are compile-time constants; the physical case layout (button-display-USB vs display-button-USB) decides the sensible default rotation. Anything that cannot be a runtime bit is a compile-time option (or patched into the hex at flash time).

| Bit | Meaning | Runtime cost |
| --- | --- | --- |
| 0 | rotate display 180 degrees | none: SSD1306 segment remap (0xA0/0xA1) plus COM scan direction (0xC0/0xC8) |
| 1 | invert display (swap lit/dark pixels) | none: SSD1306 command 0xA6/0xA7 |
| 2 | show idle logo (yes/no); see Idle screen below | small code; logo is compile-time data |
| 3 | autonomous wipe enabled (only effective if compiled in; default off) | one bit |
| 4-7 | reserved | |

- Rotation and inversion are applied by hardware commands, so no RAM or redraw is needed. `SETCFG` for these should preview the new setting (apply it, then ask for the confirm) and revert on deny or timeout.
- The idle logo lives in flash (PROGMEM) as a generated header from an image, at most 512 bytes (128x32); a narrower centered logo is also fine. The application has plenty of flash; the bootloader does not show it. An OLED does not get stuck pixels from a static image, but each lit pixel loses brightness with on-time and current, so a permanent static logo leaves a visible ghost. Mitigations, in order of value: low contrast (SSD1306 command 0x81) for the idle logo, shifting the logo by a pixel or two every few minutes (redrawing 512 bytes takes about 12 ms), and turning the display off after a timeout. Alternately inverting the logo is not recommended: it lights most of the screen half of the time, which ages the display faster overall. Rough figures for small OLED panels (from general knowledge, not measured here): thousands to tens of thousands of hours to half brightness at full contrast, much longer when dimmed.

### Idle screen

Timings are compile-time options (defaults in parentheses, to be tuned on the real device):

1. After the last activity (a serial command finishing, a button press) the display stays as it is for the idle wait (10 s).
2. With the logo bit set: show the logo at full contrast for the showcase time (30 s), then at low contrast for the dim time (2 min; range 1-5 min), then turn the display off.
3. With the logo bit clear: turn the display off after the idle wait.
4. Any button press wakes the display, and so does any serial command that needs a confirm (`ADD`, `DEL`, `TOKEN`, `SETTZ`, `SETCFG`, `WIPE`). Read-only commands (`HELLO`, `GETTZ`, `GETCFG`, `LIST`) neither wake the display nor reset the idle timer, so a polling daemon cannot keep it lit. Names are not secret, so a silent `LIST` is accepted. A press that only wakes the display is not an accept.

- Pixel shifting is not needed: the lit time per idle cycle is capped at a few minutes, and the display is off the rest of the time. It stays an optional later addition.
- The display-off state also turns off the charge pump (SSD1306 command 0x8D). Power is not a concern (the device runs from a laptop or AC-powered host), so this is only a free extra.
- The autonomous-wipe hold still works while the display is off (timed from the first press, the display wakes at once).
- Contrast for full and dim is device specific. It is not stored in EEPROM (that would cost name length); it lives in a small "device parameters" block in flash (contrast values, the logo, display controller variant and offsets) with a magic marker, which `trustctl flash` patches into the hex image before flashing. No recompile per device.

### Calibration firmware

A tiny separate firmware (replaces the I2C scanner as the first thing to flash) that talks to `trustctl` and lets you try out the display: demo text, the logo (uploaded over serial), contrast levels, rotation, inversion, and the controller variant (SSD1306 vs SH1106 column offset, 128x32 COM pins config). The chosen values are written to a local device profile (a file under `~/.config/trustctl`, one per device) and patched into the real firmware by `trustctl flash`.

- Implemented as `firmware/calibration/calibration.ino` (U8x8 text plus raw I2C, line protocol at 9600 baud: `HELLO`, `CLEAR`, `TEXT`, `BIG`, `CONTRAST`, `FLIP`, `INVERT`, `POWER`, `LOGO <offset> <hex>`, `RAW`), driven by the throwaway `tools/calib.py` (Python with Pillow, which also holds the reference PNG conversion for the later Go `trustctl`). 31% flash, 35% RAM.
- First dev display (128x32, SSD1306-compatible at 0x3C, text/logo/frame/rotation checked by eye): contrast **0x20 = full** and **0x00 = dim** are comfortable; 0x00 is not fully dark on an SSD1306, which is what the dim phase wants. These values go into the device profile for this display.
- It never reads or writes the EEPROM, and flashing through the bootloader only rewrites flash pages (never a chip erase), so it is safe to flash with keys present: keys, timezone and config flags survive the trip calibration firmware -> core firmware. The EEPROM layout version in the header guards against incompatible firmware versions.
- Rotation and inversion can only be tried out volatilely there (they are EEPROM config bits, set later with `SETCFG`). Contrast, logo and controller variant live in the flash parameter block, so they are lost when the core is flashed without patching; the profile on the host is the source of truth. Flash cannot be read back from the device (lock bits), so keep the profile.
- Logo try-out is instant and needs no reflash: `trustctl calibrate logo file.png` converts the image and streams it over serial (chunked hex lines, about 1 s for 512 bytes at 9600 baud) straight into the display memory, so the calibration firmware needs no RAM buffer for it.

### Logo images

`trustctl` converts PNG files to the display's native bitmap (page-oriented SSD1306 bytes, 128x32 = 512 bytes):

- The image must be the display size or smaller (the size comes from the device profile or `HELLO` capabilities); larger is an error. A smaller image is centered (odd margins round toward the top-left).
- Every pixel is on or off with no dithering or scaling:
  - 1-bit and truecolor/grayscale images: exactly black (all channels 0) is off; any non-zero channel, however dark (`#000001`, `#010000`, `#000100`), is on. 16-bit samples are tested on the raw value, not after conversion to 8 bit.
  - Indexed (palette) images, including 1-bit palettes: the darkest palette color (by luminance) is the off color. If several different colors share the darkest luminance, the first one in the palette is chosen. Every palette entry with exactly that color is off too (identical colors are treated identically); every other entry is on.
  - Transparency: composited over black (Go's `image/png` behavior), so transparent pixels are off.
- The only conversion flag is `--invert`. `trustctl` stays small on purpose: thresholding, scaling, dithering and anti-aliasing cleanup are done in the user's image editor. The calibration firmware makes the feedback loop fast enough for that.
- Logo workflow: flash the calibration firmware, then watch the file, e.g. `echo logo.png | entr -c trustctl calibrate logo logo.png`; every save shows up on the display immediately. (The exact `trustctl` command line is not fixed.)
- The bootloader also reads the configuration byte (one EEPROM read, about 40 bytes of code) so its security prompts use the same rotation and inversion. Its "black on white" emphasis is relative to the inversion setting.
- `HELLO` reports compile-time capabilities (e.g. `autowipe`), so `trustctl` knows which options exist.

## Display

- U8x8 text mode (no framebuffer, 16x4 characters at 8x8) or SSD1306Ascii. A full 128x32 framebuffer would take 0.5 KB of the 2 KB RAM; U8g2 page mode is the fallback.
- U8g2 2.36.19 is installed (the calibration firmware uses its U8x8 part).
- Font: compared 12 8x8 candidates on the real display for lookalikes (`Il1`, `O0o`, `rn`/`m`, `vv`/`w`). `victoriamedium8_r` is the favorite (default), `victoriabold8_r` the runner-up; the common `chroma48medium8_r` and the others were less distinct.
- The font is a flash option like contrast and logo: a fixed-size block in the device parameters (U8x8 font format, 772 bytes = 4 byte header + 96 glyphs of 8 bytes each, characters 32-127; each glyph is 8 column bytes, LSB at the top, the same layout as the logo bitmap), patched in by `trustctl flash --font <file>`. The firmware compiles in the default font and uses it when the block has no valid marker. The core firmware draws text with its own tile routine from that block (it does not need U8x8). Only 8x8 single-tile fonts qualify (not the 2x2 big fonts).
  - The bootloader keeps its own tiny font (only the few letters its prompts use): it must not depend on anything in the application area.
  - Licensing (checked): U8g2 (https://github.com/olikraus/u8g2, by Oliver Kraus) has its code under the two-clause BSD license, but its `LICENSE` says "fonts are licensed under different conditions", listed per font group at https://github.com/olikraus/u8g2/wiki/fntgrp. The Victoria fonts are in the "Open Game Art" group (`fntgrpopengameart`): downloaded from opengameart.org ("A package of 8-bit fonts for Grafx2 and Linux") and marked **CC0 / public domain** ("Zero License", https://creativecommons.org/publicdomain/zero/1.0/). So the Victoria font data can be shipped. Other candidates (e.g. `chroma48medium8`) belong to other groups with their own terms; check the group page before using one. The BSD notice still applies to any U8g2 code that is copied.
  - **TASK:** live font preview. `trustctl calibrate font file.bin` uploads a font (772 bytes) into the calibration firmware, which then redraws a sample with it. Needs about 770 bytes of RAM in the calibration firmware (it has room, currently 39% used). Same idea as the live logo preview.
- Keep a 64-byte shadow of the text grid in RAM (cheap); it lets the progress bar redraw tiles in inverse.
- Names are wrapped over two lines (2 x 16 = 32 chars) rather than scrolled; the full name must be visible when the user confirms.
- Confirm screens must show the name and the **date and time** (local, computed on the device), so a host cannot obtain tokens valid for another time. The date matters: a token for tomorrow at the same time of day would otherwise look identical.
- Time shown is `host timestamp + milliseconds since the request arrived`, so it stays correct while the user takes seconds to press, and it updates live.
- The whole display is the progress bar while the button is held: it fills across the screen, and the accept window (1-2 s) must be visible. Design of the bar is still to be discussed (**OPEN**). Feasible with 4 text rows: the shadow buffer lets the fill invert the text tile by tile (16 steps trivially, finer steps with partial-column tiles), and each step only rewrites one column of 4 tiles. Run I2C at 400 kHz.
- Idea (**OPEN**, to confirm): short alert screens (bootloader prompts, wipe, countdown digits) use a large 24 px text line plus one 8 px line; token/add/delete screens use the 4 small rows. In the bootloader the large text should be the 8x8 font scaled in software (2x or 3x), not a separate 16x24 font, to save flash.

## Application security rules

- No command may ever return a stored secret (no read-back, no export).
- Add key, delete key, wipe, set timezone, and token requests all need a button confirm with the details shown. Without it, malware could silently delete or overwrite keys. Listing names and reading the timezone need no confirm.
- No UPDATE key command: the host must delete and re-add to change a key. This avoids accidental or malicious overwrites.
- One request at a time: while a confirmation is pending, other commands get `ERR BUSY`. A pending request times out after a compile-time configurable duration (about 20-30 s).
- Button gesture for accepting: release the button inside a window of 1 s to 2 s of holding. Releasing before 1 s or after 2 s is not an accept. This avoids accidental presses and malware guessing the timing. The exact window may be tuned (**OPEN**).
  - A release before 1 s or after 2 s denies immediately (the host gets `ERR DENIED` and simply requests again).
  - The button is debounced in software (a state change counts only after the level is stable for about 20-30 ms), so a flaky contact edge cannot cause a false release/deny. Hold time is measured from the first stable press.
- "Wipe all keys" is an application feature, a serial command with the same gesture but a much longer window: release between 5 s and 6 s of holding (normal accept: 1-2 s). The long hold gives time to notice the difference and reconsider; wiping is rare, so the cost is irrelevant. Same deny rules apply. It replaces any wipe-on-flash.
- Autonomous wipe (optional feature, compiled in with a build flag and enabled by a config bit, default off): wipe without any host, e.g. when the device must be destroyed with only a power bank or phone charger at hand. At idle (no pending request, button not held during power-up) release the button between 10 s and 11 s of holding, then a prompt with a countdown, then the double-tap-and-hold gesture, then the wipe runs with the display as progress bar. A release outside the window is silently ignored. It only destroys data, so a bystander with physical access can at worst cause a denial of service (keys come back from the offline backups); that is why it defaults to off.
  - Wipe keeps the timezone offset and the configuration flags; it only destroys key slots.
  - The secrets are overwritten interleaved across all used slots: byte 0 of every key, then byte 1 of every key, and so on. An interrupted wipe (power pulled) therefore leaves every key equally damaged instead of leaving the last keys intact. Missing bytes must be brute-forced against a known token: 1-3 missing bytes is trivial, about 5 is feasible, 8 or more (2^64) is not. With 20 keys, 8 bytes per key takes about 0.5 s (8 x 20 x 3.3 ms); with fewer keys it is proportionally faster. Only used slots are processed.
  - After the secrets: names, CRCs, then the slot flags last. A half-wiped slot fails its CRC and is treated as corrupt, never as a valid key.
  - EEPROM writes take about 3.3 ms per byte, so wiping all secrets takes about 1.3 s; the display shows the progress bar.
- Delete also zeroes the slot's secret (not just the flag), so unused slots never hold key material and wipe can skip them.
- Users must keep offline backups of the original secrets/QR codes: there is no export, so a lost device loses the keys.

### Time and timezone

- No clock on the device: the host sends the unix time (uint32, good until 2106) with each token request, and the user checks the displayed local date and time against their own clock.
- The timezone offset is stored on the device and changed only via a confirmed command. It must never come from the same request as the timestamp: otherwise malware could send T+2h with offset -2h and the display would look correct.
- The device cannot handle daylight saving. `trustctl` reads the offset (`GETTZ`) before token requests, compares it with the host's current offset, and sets it (`SETTZ`, which needs a button confirm) if it differs (DST change, travel).
- Every token screen shows the offset next to the time (e.g. "15.10 09:41 +2") so a wrongly set offset is visible, and the `SETTZ` confirm shows it prominently. The `SETTZ` confirm is the one place where the user must know the correct offset.

## Bootloader design (the key security decision)

Problem: with the stock Nano bootloader, host malware can reflash the firmware over serial (DTR resets the chip) and flash an app that dumps EEPROM. EEPROM is not erased by flashing.

Decision: a custom, button-gated bootloader. Host software cannot press the button, so it cannot open the bootloader.

- Installed once via ISP (a second Arduino as ISP or a USBasp).
- Fuses/lock bits: `BOOTRST` set (chip always starts in the bootloader, so the check cannot be bypassed); boot section locked against self-programming from the app (BLB1) so a flashed app cannot rewrite the bootloader; lock bits blocking ISP read-out. `EESAVE` must stay **unprogrammed**: otherwise a chip erase would clear the lock bits but keep the EEPROM.
- Boot section 1-2 KB (the ATmega328P allows up to 2 KB).
- Strip everything unneeded from optiboot, especially all EEPROM read/write handling. This is for size, not security: malicious _firmware_ reads EEPROM by itself, so the real protection is the gate. Flash read-back is optional (flash is not secret).
- The EEPROM is NOT wiped on update, because wiping every time would push users to keep key backups on the device. The anti-bait-and-switch gate below must therefore hold on its own.

Normal boot: button not held at reset, jump straight to the app. The bootloader does not listen on serial.

Button held at reset (anti bait-and-switch; malware can reset the chip while the user is holding the button for a real token request):

1. Display, black on white: "RELEASE BUTTON". Holding through the reset must not advance the state.
2. After release: a short prompt (about "FLASH NEW FIRMWARE?"; the long sentence does not fit) with a countdown from 3 to 0, or a moving progress bar (cannot be skipped by reflex).
3. Only then a deliberate gesture opens flash mode: double-tap-and-hold (about 2 s). Triple-tap was rejected: a user mashing the button on an unexpected screen could complete it by accident. This gesture must differ from the app's confirm so it is never done by habit.
4. Timeout at any step boots the app.

Rule for the user: never complete the update gesture unless you started an update. An unexpected update screen means do nothing; it times out into the app. Malware can spam resets to show the prompt, which is annoying but not exploitable.

Flash flow caveat: opening the serial port toggles DTR and resets the chip, and the button is no longer held at that moment. The user holds the button, then the tool resets the chip; the gate (countdown plus gesture) takes longer than avrdude's roughly 10 sync attempts. So a wrapper (e.g. `trustctl flash`) must reset once, then retry syncing for 30 s or more without resetting again. Needs testing on real hardware.

Size estimate: trimmed optiboot ~400-500 B, plus a hand-written text-only SSD1306 driver (TWI ~150 B, init ~30 B, 5x7 font limited to used characters ~100-150 B). Total around 1-1.5 KB, which should fit. U8g2/U8x8 are too big, so the driver must be custom, and it depends on the controller (SSD1306 vs SH1106 column offset).

- If the display is dead or absent the bootloader fails closed (never opens); recovery needs ISP.
- Residual risk: a user tricked into deliberately performing the gesture on an induced screen can get a malicious app flashed with the keys still in EEPROM. Signed firmware verified in the bootloader would remove this, but costs key management and flash; deliberately skipped.

### Bootloader implementation (done, on the dev Nano)

Source in `firmware/bootloader` (`boot.c`, `Makefile` using the Arduino avr-gcc, `gen_font.py` which generates `boot_font.h` with only the glyphs the messages need, from the CC0 Victoria font in `victoriamedium8.bin`). Written from scratch, not derived from optiboot. **2016 of 2048 bytes** (little margin; `make` checks the limit, any new feature needs something else to go). Linked at 0x7800 with `-nostartfiles`; the reset vector is a naked `start` that clears r1 and jumps to `main`.

- Normal start: button not held and a firmware present: `jmp 0` straight away, no display, no serial. Held at reset (or no firmware): watchdog 2 s, display init (SSD1306 128x32, page addressing, rotation/invert from the application's EEPROM header byte 2, contrast 0x20 as a constant), then the gate. A dead display with a firmware present boots the application; without a firmware the chip halts.
- Gate: "RELEASE BUTTON" (inverted, 15 s timeout), then "FIRMWARE UPDATE / WAIT" with a bar in the bottom row that drains smoothly, one pixel column at a time, over about 4 s (a press of more than 30 ms while it drains makes the gate give up and reset, so a still-held finger gets a fresh gate), then "TAP TAP HOLD / 2 SECONDS". The gesture is **tap, tap, then press a third time and hold 2 s** (taps released within 0.6 s, the next press within 0.8 s). The bottom row lights up one third per tap (two thirds after the second), and fills smoothly from two thirds to the full width while the third press is held (about 1.9 s) so the user can see how far the gesture got; any mistake clears the bar and starts the gesture over; success shows "RELEASE BUTTON" again and the update mode opens after the release. Every wait has a 15 s timeout that ends in a reset. Button samples are every 10 ms and need 3 in a row.
- Update mode: `UART0` 115200 baud, a minimal STK500v1 subset (sync, parameters, set device, load address, program page, read page, read signature, leave progmode). Flash only: EEPROM read and write get `STK_FAILED`, writes must be page aligned and below 0x7800. The STK500 "universal" (ISP passthrough) command is not implemented (it did not fit), so an unknown command desynchronizes and ends in a reset. The watchdog (8 s) ends update mode when the host goes quiet; leaving progmode reboots. Flash can be read back (for verification; it holds no secrets).
- Deviations from the design above: the countdown digits became a draining bar and are not restarted by a press (it aborts, see above); the config byte is read without checking the header CRC (the check did not fit into 2 KB; a damaged byte only rotates or inverts the prompts); the update mode baud rate is 115200 (so the Arduino board variant for it is `atmega328`, not `atmega328old`).
- Installed via ISP: `avrdude -c stk500v1 -b 19200 -p m328p -P <uno> -e -U flash:w:boot.hex:i` (the `-e` chip erase also empties the EEPROM), fuses unchanged (`lfuse 0xFF`, `hfuse 0xDA`, `efuse 0xFD`, so `BOOTRST` set, 2 KB boot section, `EESAVE` unprogrammed), lock byte `0xCF` (boot section protected against SPM; ISP read-out still open for development). **The final read-out lock bits are not set yet.**
- Flashing: `tools/flash.py firmware/core/build/core.ino.hex --port /dev/ttyUSB0 [--rig /dev/ttyUSB1]` (Python, Intel HEX parser, STK500v1 client with read-back verification; the prototype of `trustctl flash`). Without `--rig` the user does the gate by hand; with the Uno rig the button is pressed automatically. `avrdude -c arduino -b 115200` has not been tried; it only retries about 10 times, far less than the gate takes, so a wrapper is needed anyway.
- Tests (`tools/boot_test.py`, all pass): a host that resets the chip and speaks STK500 gets no answer; a reset with the button held through it never opens the update mode (22 s observed) and times out into the application; pressing in the countdown, tap-tap without the hold, tap-tap and a 0.9 s hold, a tap and a hold (one tap too few), and a long first press all fail to open it; the correct gesture opens it; EEPROM read, EEPROM write and a write into the bootloader's own section are refused; the update mode times out and the application still runs afterwards.

## Serial protocol

Line-based ASCII, 9600 baud, strictly request/response (the host sends the next command only after the final response). Fixed fields come first and the free-text name last, so spaces in names need no escaping.

```
HELLO                                  -> TrustIssue 1.2.3 proto 1   (also sent unprompted at boot)
LIST                                   -> KEY <id> <name> ... END
ADD <base32> <algo> <digits> <period> <name...>
                                       -> PENDING ... OK <id> | ERR <code>
DEL <id>                               -> PENDING ... OK | ERR <code>
TOKEN <id> <unixtime>                  -> PENDING ... OK 012345 | ERR <code>
GETTZ                                  -> OK <quarter-hours>
SETTZ <quarter-hours>                  -> PENDING ... OK | ERR <code>
GETCFG                                 -> OK <flags-hex>
SETCFG <flags-hex>                     -> PENDING ... OK | ERR <code>
WIPE                                   -> PENDING ... OK | ERR <code>
```

`HELLO` also lists build capabilities, e.g. `TrustIssue 1.2.3 proto 1 caps=autowipe,logo`. `trustctl` maps the flag bits to names.

- Error codes: `DENIED`, `TIMEOUT`, `BUSY`, `FULL`, `DUPLICATE`, `BADARG`, `NOTFOUND`, `UNKNOWN`. No `-1` convention.
- Commands that need the button are answered with `PENDING` first (validation errors such as `DUPLICATE`, `BADARG`, `NOTFOUND` come at once, without a prompt), then `OK ...` or `ERR ...` when the user decided. `WIPE` on an empty device is answered `OK` immediately. While a request is pending every other line, even `HELLO`, gets `ERR BUSY`.
- Token is a string (leading zeros). The device addresses keys by id; `trustctl` maps names to ids.
- Arduino's serial RX buffer is a 64-byte ring buffer filled by an interrupt; bytes arriving while it is full are **dropped**, there is no hardware flow control on the Nano. An `ADD` line can be about 75 bytes, so raise `SERIAL_RX_BUFFER_SIZE` to 128 (costs 64 B RAM) and keep the main loop draining it. At 9600 baud one byte takes about 1 ms, so a 64-byte buffer tolerates about 67 ms of blocking. A full `LIST` (about 700 bytes) takes about 70 ms. The TX side blocks when full but loses nothing.
- Display updates other than the progress bar may block briefly; no commands are expected while the button is being held.
- The USB-serial chip resets the board on DTR when a port is opened, so the host must tolerate a 1-2 s start delay. The boot banner makes the device identifiable.

## trustctl

- Language: Go. A single static binary (`CGO_ENABLED=0`) cross-compiles to Linux, Windows, macOS, the BSDs, and many CPU architectures.
- CLI first, GUI (Trust Desk) later; the GUI can talk to the same daemon.
- Daemon: the first invocation starts a daemon (Go cannot `fork()` safely, so it re-executes itself with a hidden flag and detaches), later invocations talk to it over a Unix socket in `XDG_RUNTIME_DIR` (mode 0600). Its main benefit is keeping the port open, which avoids the 1-2 s DTR reset on every call.
- The daemon caches names and ids. The cache is cleared on any change: add, delete, and disconnect (the order cannot change while the port is open unless we do it).
- The daemon exits on disconnect, on suspend, and after a configurable idle timeout; restarting takes a few seconds, which is accepted for guaranteed clean state. Suspend can be detected without extra dependencies by noticing a jump in a periodic ticker.
- Non-invasive on the host: only the socket in `XDG_RUNTIME_DIR` and an optional config file in `~/.config`.
- Finding the device: a configured device path by default. VID/PID `1a86:7523` matches every CH340 board and probing resets whatever board is on the port, so autoprobe is an **option in the config file, off by default**. When probing, wait for the boot banner or send `HELLO`.
- `trustctl flash`: speaks the STK500v1 protocol of the stripped optiboot itself (a few hundred lines of Go), so no `avrdude` is needed. It resets the chip once, retries syncing for 30 s or more while the user goes through the bootloader gate, patches the device parameters block into the hex, and uploads. `trustctl calibrate` drives the calibration firmware.
- On every token request it runs `GETTZ`, compares with the host offset, and sets it if needed.
- `trustctl` accepts a name or an id and maps to the id itself.
- Compile-time options: pending-request timeout and similar firmware constants.

## Development setup

Old Arduinos are being collected as the development setup (the original Nano has no responding bootloader, and a second board is needed as ISP programmer anyway).

- Dev hardware found: one old Nano (chip print to be verified: it must be an ATmega328**P**; a plain 328 has another signature and a 168 has only 16 KB flash, 1 KB RAM and 512 B EEPROM, which would not fit this design), one Arduino Uno, and a display that looks like 128x32 (controller unknown).
- The Uno is both the ISP programmer (running the `ArduinoISP` example sketch) and later the end-to-end test rig (a sketch that presses the button and resets the Nano on serial commands from the PC). Use `avrdude -c stk500v1 -b 19200 -P <uno port>` for ISP.
- Bring-up results (2026-10-15): ISP works with the Uno (`ArduinoISP` from `arduino/arduino-examples`, `examples/11.ArduinoISP`; the capacitor was not needed on this Uno, port `/dev/ttyUSB1`). The Nano is an ATmega328P (signature `1e950f`). Fuses `lfuse 0xFF`, `hfuse 0xDA` (2 KB boot section, `BOOTRST` set, `EESAVE` unprogrammed), `efuse 0xFD`, lock `0xCF` (stock Arduino values). The old 2 KB Arduino bootloader sits at 0x7800 and the application area was blank; the serial upload failed earlier on the first, loosely connected setup (cause unknown, probably a flaky cable or contact), but works now: `arduino-cli upload -p /dev/ttyUSB0 -b arduino:avr:nano:cpu=atmega328old` (old bootloader, 57600 baud) wrote the scanner in 1.2 s, with the Uno's D10 still attached to the Nano's RST. The display answers at I2C address **0x3C**. The scanner sketch was written with `avrdude -D` (no chip erase) so the bootloader was kept.
- Test rig (done): `firmware/rig/rig.ino` on the Uno (115200 baud; `PRESS`, `RELEASE`, `SEQ <ms> ...`, `RESET <ms>`, `RESETHOLD <pre> <post>`), `firmware/button_test/button_test.ino` on the Nano (25 ms debounce measured from the last raw edge, classification into DENY-SHORT / ACCEPT / DENY-LONG / WIPE), and `tools/rig.py` which runs 9 scenarios end to end (taps, 0.9/1.1/1.9/2.1 s holds, bouncy press and release, wipe window, double-tap-and-hold). All 9 pass; measured hold times are 2-16 ms above the commanded ones (Uno `delay` plus serial latency), so keep at least 100 ms of margin to the window edges in tests. The Uno currently runs the rig sketch: to use ISP again, re-upload `ArduinoISP` (from `arduino/arduino-examples`, `examples/11.ArduinoISP`) to the Uno.
- The Nano keeps its own USB cable and powers itself from it; do not connect the Uno's 5V to the Nano (two 5V sources joined). Only GND is shared.
- During development leave the lock bits **unset** so the board can be read and reprogrammed freely. Set `BOOTRST`/boot-section size fuses as needed, but burn the final lock bits (BLB1, read-out protection) and verify `EESAVE` is unprogrammed only for the finished device.
- Breadboard wiring (Nano pins are the edge header pins; the ICSP header carries the same signals but is easy to mount rotated):

| From | To | Purpose |
| --- | --- | --- |
| Uno GND | Nano GND | common ground (the only shared supply line) |
| Uno D10 | Nano RST | ISP reset; the test rig also pulses it to reset the Nano |
| Uno D11 | Nano D11 | MOSI |
| Uno D12 | Nano D12 | MISO |
| Uno D13 | Nano D13 | SCK |
| Uno D4 | 1 kΩ resistor, then Nano D2 | simulated button press |
| Nano D2 | push button, other leg to Nano GND | physical button |
| Nano A4 | display SDA | I2C data |
| Nano A5 | display SCL | I2C clock |
| Nano 5V (or 3V3, see the module's label) | display VCC | display power |
| Nano GND | display GND | display ground |
| 10 µF capacitor, + leg on Uno RESET, - leg on Uno GND | | stops the Uno from auto-resetting while `avrdude` talks to ArduinoISP; make it removable (jumper) because it also blocks uploading new sketches to the Uno |

- The Uno's button pin is only ever driven low or released (`OUTPUT LOW` = pressed, `INPUT` = released), never driven high. Do not use Uno D7, D8 or D9 for it: `ArduinoISP` blinks LEDs on them.
- Each board has its own USB cable to the PC.
- Test-rig scenarios this enables: press and release the button with exact timings (accept window 1-2 s, deny, wipe 5-6 s), hold the button while resetting the Nano (bootloader gate), and the double-tap-and-hold gesture. The display itself still has to be checked by eye (or a camera).
- Steps to bring up a board: (1) ISP-flash the calibration firmware plus the patched bootloader, (2) identify the display controller and calibrate, (3) flash the core firmware through the bootloader gate with `trustctl flash`, (4) final lock bits.
- Everything in `trustctl` and the protocol can be developed first against a simulated device (a fake serial endpoint implementing the protocol), which needs no hardware.

### trustctl status (Go, in `trustctl/`)

Implemented and tested: the CLI without the daemon.

- Layout: `internal/protocol` (line protocol client with `PENDING` handling, name/secret/otpauth validation, errors you can test with `errors.Is`), `internal/sim` (a simulated device that mirrors the firmware's validation and replies, the button is a callback), `internal/serialdev` (`go.bug.st/serial`, banner handshake plus a settle time because opening the port can reset the device twice, autoprobe), `cmd/trustctl` (commands, name resolution, timezone sync, config file).
- Commands: `list`, `token NAME`, `add NAME` (secret or `otpauth://` URI from stdin, hidden on a terminal, never from the command line), `del NAME`, `tz [get|sync|set +H[:MM]]`, `config [get|set FLAG=on|off]`, `wipe`, `hello`. `NAME` is a name (any case), a slot number, or a unique prefix; a name beats a slot number. `token` first syncs the device's timezone offset to the host's (with a confirmation on the device if it differs). `--simulate` uses the simulator. Exit codes: 0 ok, 1 failure, 2 denied or timed out on the device, 64 usage, 69 no device.
- Device selection: `--device PATH`, `TRUSTCTL_DEVICE`, or `device =` in `~/.config/trustctl/config` (also `autoprobe = true`); no implicit probing.
- Tests: `go test ./...` in `trustctl/` (protocol against the simulator, name resolution), and `tools/trustctl_e2e.py` against the real device with the Uno rig pressing the button (17 checks: add, duplicate, bad name, list, tz sync, token matching the host's TOTP, tap denies, config, del, wipe window); all pass.
- Build: `cd trustctl && GOPATH=$HOME/go go build -o bin/trustctl ./cmd/trustctl`. Note: this machine's shell sets `GOPATH` to a read-only system directory, so override it; the `go.mod` pulls a newer Go toolchain automatically (it needs network the first time).
- Portability: `CGO_ENABLED=0 go build` works for linux (amd64, arm64, arm, 386), windows (amd64, arm64), darwin (amd64, arm64), freebsd and openbsd. NetBSD does not build because `go.bug.st/serial` has no NetBSD support (only checked that it compiles; only Linux amd64 has run against the device).
- Not done yet: the daemon (first invocation forks, Unix socket in `XDG_RUNTIME_DIR`, it can simply proxy the line protocol and cache `LIST`), `flash` (port of `tools/flash.py`, with hex patching of the device parameters), `calibrate` (logo/font/contrast preview, PNG conversion), packaging and cross-compilation checks for Windows, macOS and the BSDs.

## Tooling and environment

- `arduino-cli` (with `arduino:avr`), `avrdude`, and PlatformIO are installed; the user is in the `dialout` group.
- Board: `arduino:avr:nano:cpu=atmega328` (new bootloader) or `cpu=atmega328old` (old bootloader, 57600 baud). Neither could upload to the current board.
- Serial port: `/dev/ttyUSB0`, a CH340 (1a86:7523). It re-enumerated as `/dev/ttyUSB1` during testing, which suggests a flaky cable or contact; check the current port before every upload.
- Auto-reset via DTR and the bootloader response are unconfirmed on the current board; it may have no bootloader.

## Status and next steps

- Done: concept, threat model, hardware choice and wiring, storage layout, bootloader gate design, protocol draft, `trustctl` design; dev rig built and verified (ISP, display at 0x3C, button, reset line, serial upload); calibration firmware and display tuning (contrast, flip, invert, logo, font choice); button debounce and hold-window tests; TOTP core.
- No longer blocked: the dev boards work (see Development setup). The bootloader work needs `ArduinoISP` back on the Uno.
- Core firmware progress: `firmware/trustcore` (Arduino-style library, plain C++ without Arduino dependencies): SHA-1, HMAC-SHA1, HOTP/TOTP, strict Base32 decoder. Host tests in `firmware/test/run.sh` (SHA-1 FIPS vectors, HMAC RFC 2202 cases 1, 2 and 6, all RFC 6238 SHA-1 vectors, Base32 RFC 4648 and rejection cases) pass. `firmware/core_selftest` runs the RFC vectors on the Nano: all pass; one TOTP takes about 19 ms on the ATmega328P, the self-test sketch uses 17% flash and 15% RAM. Compile with `--library firmware/trustcore`.
- Also done in `firmware/trustcore` with host tests (`firmware/test/run.sh`, three suites): the EEPROM storage layer (`tc_storage`: name rules, case-insensitive duplicates, add/delete, interleaved wipe verified with simulated power loss, header with timezone and config) and the protocol parser (`tc_protocol`: all commands, strict argument checking). The whole library compiles for the Nano (12% flash, 13% RAM in a combined check).
- Core app firmware (`firmware/core`, build with `firmware/core/build.sh [upload <port>]`, which enlarges the serial RX buffer to 128): serial protocol, pending-request state machine (30 s timeout, `ERR BUSY`), confirm screens for token/add/delete/timezone/settings/wipe on the 128x32 display (4 text rows from the CC0 Victoria font, extracted with `tools/extract_u8x8_font.py` into `font_default.h`), the full-display progress bar (inverting tiles while the button is held, `setFill`), token computed from `timestamp + elapsed`, local date/time with offset (`tc_time`), idle screen then display off after 10 s, wake-only first press, config preview with revert on deny. Uses 67% flash and 50% RAM (1 KB left for the stack). End-to-end test `tools/core_test.py [--slow]` (protocol over serial, button from the Uno rig): all scenarios pass. Because the device adds elapsed time, test timestamps must not sit near a 30 s boundary.
- Not yet in the core firmware: idle logo, autonomous wipe, flash-patched device parameters (contrast, font, logo are compile-time constants for now), the final progress bar design with the visible accept window, large-font alert screens, the gate bootloader.
- Next: the remaining firmware features above, set the final lock bits at the end, `trustctl` (including `flash` based on `tools/flash.py`).
- (older plan) the app firmware that ties these together (serial loop, pending-request state machine with timeout, confirm flow with the progress bar and button, token computation with `timestamp + elapsed`), the minimal display driver and the patched bootloader (via ISP), the app firmware.
- Next (no hardware needed): `trustctl` in Go (protocol client, simulated device, daemon, PNG-to-bitmap conversion, STK500v1 flasher with hex patching), and a host-side test suite for the protocol. The `sketches/i2c_scan` sketch is superseded by the calibration firmware.
- Open items are marked **OPEN** in this file: progress bar design, the large-font alert screens, and the exact timings.
