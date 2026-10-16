# Trust Issues: project notes

Hardware TOTP token vault for people who don't trust their computers. Components: Core is the device firmware, `trustctl` the command line client, Trust Desk the planned GUI.

This file records the design decisions and the state of the project. The firmware, the bootloader and `trustctl` exist and are tested on the dev rig (see the status sections). Items marked **OPEN** are undecided.

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
- Lookalikes inside the set (`l`/`1`/`I`, `O`/`0`) remain possible; the display font should draw them distinctly (slashed or dotted zero, different `l`/`1`/`I`). Done: 12 fonts were compared on the real display and `victoriamedium8` was chosen.
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

- The SETCFG confirm screen shows all four flags (`Rot:off Inv:on` / `Logo:on Wp:off`) and shows the ones this request changes inverted (white block, dark text), so the user sees at a glance what is being switched. Mechanism: bit 7 of a character in the shadow grid means "draw inverted" (`drawTile`, set by `settingsRow`); the progress bar still XORs over it. The autowipe label is `Wp:` because `Logo:off Wipe:off` is 17 characters and the old text lost its last letter.
- After an accepted `TOKEN` the result screen shows "Done" and the code in large letters (two lines, each centered, tile rows 0-1 and 2-3, up to 8 characters per line) for 5 s (`TOKEN_SHOW_MS`) instead of the usual 1.2 s "Done"; a button press ends it early. The host gets the code anyway, so this reveals nothing new; it lets the user read it off the device. `showResult(text, second, ms)`; the large glyphs are computed per tile by `bigTile`.
- Unlocked indicator (done): at boot the core firmware reads the lock byte and the high fuse with `boot_lock_fuse_bits_get()` (`checkLock`). The device counts as finished only with lock byte 0xCC (what `tools/user/final_lock.sh` sets) and high fuse bits 3..0 = 1010 (BOOTRST set, 2 KB boot section, EESAVE unprogrammed); otherwise the idle screen shows an inverted `NOT LOCKED` on its last row and `HELLO` and the banner end with ` unlocked`. `trustctl hello` and `trustctl add` print a warning when they see that word (`warnIfUnlocked`). It is a warning for the owner, not a security boundary: a malicious firmware could hide it, but flashing needs the button gesture. The dev device always shows it, which is correct. Reading the bits with lock 0xCC set is untested (the dev device stays open); the datasheet says the SPMCSR read is not restricted by the lock modes. The logo screens do not show it.
- ISP scripts for the dev setup (`tools/user/`): `flash_bootloader.sh PORT --yes [--keep-eeprom] [--hex FILE]` checks the signature, builds `firmware/bootloader` (or takes `--hex`), does the chip erase + flash + fuses (lfuse 0xFF, hfuse 0xDA, efuse 0xFD) + lock 0xCF and verifies them; `--keep-eeprom` programs EESAVE (hfuse 0xD2) for the erase and clears it afterwards, refused on a locked chip (fuses locked). `unlock.sh PORT --yes` is a chip erase (the only way back from lock 0xCC; destroys flash, bootloader, EEPROM; fuses stay) and checks the lock byte reads 0xFF afterwards. `final_lock.sh` stays the way forward. All use `-c stk500v1 -b 19200 -P PORT` (the Uno with ArduinoISP); `ISP_ARGS="-c usbasp"` selects another programmer. Tested: `flash_bootloader.sh --keep-eeprom` on the real dev device (EEPROM byte-identical afterwards, fuses read back right); `unlock.sh` only against a fake `avrdude`, since erasing the dev device was not wanted.
- Rotation and inversion are applied by hardware commands, so no RAM or redraw is needed. `SETCFG` for these should preview the new setting (apply it, then ask for the confirm) and revert on deny or timeout.
- The idle logo lives in flash (PROGMEM) as a generated header from an image, at most 512 bytes (128x32); a narrower centered logo is also fine. The application has plenty of flash; the bootloader does not show it. An OLED does not get stuck pixels from a static image, but each lit pixel loses brightness with on-time and current, so a permanent static logo leaves a visible ghost. Mitigations, in order of value: low contrast (SSD1306 command 0x81) for the idle logo, shifting the logo by a pixel or two every few minutes (redrawing 512 bytes takes about 12 ms), and turning the display off after a timeout. Alternately inverting the logo is not recommended: it lights most of the screen half of the time, which ages the display faster overall. Rough figures for small OLED panels (from general knowledge, not measured here): thousands to tens of thousands of hours to half brightness at full contrast, much longer when dimmed.

### Device parameter block (done)

A fixed 1300 byte block at flash address **0x7000** of the core firmware (section `.paramblock`, placed by `-Wl,--section-start=.paramblock=0x7000` in `firmware/core/build.sh`; generated defaults in `firmware/core/params_default.h` by `tools/build/gen_params.py`, which takes the font from `firmware/fonts/victoriamedium8.bin`). The application code must end below 0x7000 (it is at about 20 KB). Layout: `0x000` magic `TIPB`, `0x004` version 1, `0x005` flags (bit0 font valid, bit1 logo valid), `0x006` contrast full (default 0x20), `0x007` contrast dim (default 0x00), `0x010` font (772 bytes, U8x8 format), `0x314` logo (512 bytes, 128x32, SSD1306 page layout). `trustctl flash` rewrites bytes in the hex file; the firmware reads the block with `pgm_read_byte`.

### Idle screen

Timings are compile-time options (defaults in parentheses, to be tuned on the real device):

0. At power-up or reset, with the logo option on and a logo in the parameter block, the logo is shown at full contrast for 5 s (`BOOT_LOGO_MS`) before the idle screen appears. A button press or a command that needs a confirmation cuts it short; read-only commands do not.
1. After the last activity (a serial command finishing, a button press) the display stays as it is for the idle wait (10 s).
2. With the logo bit set (and a logo in the parameter block): show the logo at full contrast for the showcase time (30 s), then at low contrast for the dim time (2 min; range 1-5 min), then turn the display off. Implemented in the core firmware; without the logo bit or without a logo the display turns off after the idle wait.
3. With the logo bit clear: turn the display off after the idle wait.
4. Any button press wakes the display, and so does any serial command that needs a confirm (`ADD`, `DEL`, `TOKEN`, `SETTZ`, `SETCFG`, `WIPE`). Read-only commands (`HELLO`, `GETTZ`, `GETCFG`, `LIST`) neither wake the display nor reset the idle timer, so a polling daemon cannot keep it lit. Names are not secret, so a silent `LIST` is accepted. A press that only wakes the display is not an accept.

- Pixel shifting is not needed: the lit time per idle cycle is capped at a few minutes, and the display is off the rest of the time. It stays an optional later addition.
- The display-off state also turns off the charge pump (SSD1306 command 0x8D). Power is not a concern (the device runs from a laptop or AC-powered host), so this is only a free extra.
- The autonomous-wipe hold still works while the display is off (timed from the first press, the display wakes at once).
- Contrast for full and dim is device specific. It is not stored in EEPROM (that would cost name length); it lives in a small "device parameters" block in flash (contrast values, the logo, display controller variant and offsets) with a magic marker, which `trustctl flash` patches into the hex image before flashing. No recompile per device.

### Calibration firmware

A tiny separate firmware (replaces the I2C scanner as the first thing to flash) that talks to `trustctl` and lets you try out the display: demo text, the logo (uploaded over serial), contrast levels, rotation, inversion, and the controller variant (SSD1306 vs SH1106 column offset, 128x32 COM pins config). The chosen values are written to a local device profile (a file under `~/.config/trustctl`, one per device) and patched into the real firmware by `trustctl flash`.

- Implemented as `firmware/calibration/calibration.ino` (U8x8 text plus raw I2C, line protocol at 9600 baud: `HELLO`, `CLEAR`, `TEXT`, `BIG`, `CONTRAST`, `FLIP`, `INVERT`, `POWER`, `LOGO <offset> <hex>`, `RAW`), driven by `trustctl calibrate`. 31% flash, 35% RAM.
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
- Logo workflow: flash the calibration firmware, then watch the file, e.g. `echo my-logo.png | entr -c trustctl calibrate logo my-logo.png`; every save shows up on the display immediately. (The exact `trustctl` command line is not fixed.)
- The bootloader also reads the configuration byte (one EEPROM read, about 40 bytes of code) so its security prompts use the same rotation and inversion. Its "black on white" emphasis is relative to the inversion setting.
- `HELLO` reports compile-time capabilities (e.g. `autowipe`), so `trustctl` knows which options exist.

## Display

- U8x8 text mode (no framebuffer, 16x4 characters at 8x8) or SSD1306Ascii. A full 128x32 framebuffer would take 0.5 KB of the 2 KB RAM; U8g2 page mode is the fallback.
- U8g2 2.36.19 is installed (the calibration firmware uses its U8x8 part).
- Font: compared 12 8x8 candidates on the real display for lookalikes (`Il1`, `O0o`, `rn`/`m`, `vv`/`w`). `victoriamedium8_r` is the favorite (default), `victoriabold8_r` the runner-up; the common `chroma48medium8_r` and the others were less distinct.
- The font is a flash option like contrast and logo: a fixed-size block in the device parameters (U8x8 font format, 772 bytes = 4 byte header + 96 glyphs of 8 bytes each, characters 32-127; each glyph is 8 column bytes, LSB at the top, the same layout as the logo bitmap), patched in by `trustctl flash --font <file>`. The block is always part of the firmware's hex file with the default font in it, so there is no separate fallback. The core firmware draws text with its own tile routine from that block (it does not need U8x8). Only 8x8 single-tile fonts qualify (not the 2x2 big fonts).
  - The bootloader keeps its own tiny font (only the few letters its prompts use): it must not depend on anything in the application area.
  - Licensing (checked): U8g2 (https://github.com/olikraus/u8g2, by Oliver Kraus) has its code under the two-clause BSD license, but its `LICENSE` says "fonts are licensed under different conditions", listed per font group at https://github.com/olikraus/u8g2/wiki/fntgrp. The Victoria fonts are in the "Open Game Art" group (`fntgrpopengameart`): downloaded from opengameart.org ("A package of 8-bit fonts for Grafx2 and Linux") and marked **CC0 / public domain** ("Zero License", https://creativecommons.org/publicdomain/zero/1.0/). So the Victoria font data can be shipped. Other candidates (e.g. `chroma48medium8`) belong to other groups with their own terms; check the group page before using one. The BSD notice still applies to any U8g2 code that is copied.
  - Live font preview (done): `trustctl calibrate font file.bin` uploads a font (772 bytes) into the calibration firmware (RAM use is now 77%, 461 bytes left), which redraws a sample with it.
- Keep a 64-byte shadow of the text grid in RAM (cheap); it lets the progress bar redraw tiles in inverse.
- Names are wrapped over two lines (2 x 16 = 32 chars) rather than scrolled; the full name must be visible when the user confirms.
- Confirm screens must show the name and the **date and time** (local, computed on the device), so a host cannot obtain tokens valid for another time. The date matters: a token for tomorrow at the same time of day would otherwise look identical.
- Time shown is `host timestamp + milliseconds since the request arrived`, so it stays correct while the user takes seconds to press, and it updates live.
- The whole display is the progress bar while the button is held: the full width is the END of the accept window (2 s, or 6 s for wipe), and the window's start is marked on the bar: up to the window start the bar is solid (inverted), from there on it is a checkerboard of 2x2 pixel squares (each pixel column XORed with 0x33 or 0xCC, alternating every two columns, so the text stays visible; a 1x1 checkerboard looked too fine). So the user sees where the window begins (at half width for 1-2 s, at 5/6 for 5-6 s) and releases while the bar is checkered. The wipe's own progress, after confirming, is a plain solid bar. Feasible with 4 text rows: the shadow buffer lets the fill invert the text tile by tile (16 steps trivially, finer steps with partial-column tiles), and each step only rewrites one column of 4 tiles. Run I2C at 400 kHz.
- Idea (**OPEN**, to confirm): short alert screens (bootloader prompts, wipe, countdown digits) use a large 24 px text line plus one 8 px line; token/add/delete screens use the 4 small rows. In the bootloader the large text should be the 8x8 font scaled in software (2x or 3x), not a separate 16x24 font, to save flash.

## Application security rules

- No command may ever return a stored secret (no read-back, no export).
- Add key, delete key, wipe, set timezone, and token requests all need a button confirm with the details shown. Without it, malware could silently delete or overwrite keys. Listing names and reading the timezone need no confirm.
- No UPDATE key command: the host must delete and re-add to change a key. This avoids accidental or malicious overwrites.
- One request at a time: while a confirmation is pending, other commands get `ERR BUSY`. A pending request times out after a compile-time configurable duration (about 20-30 s).
- Overshoot (implemented): the moment the hold passes the end of the window (2 s; 6 s for wipe; 11 s for the autonomous wipe hold) the request is denied at once: the big "Too long" replaces the progress bar directly (every tile of the display goes from the bar to the word in one pass of about 20 ms, no blank screen and no blinking in between; this holds for all result screens) and the host gets `ERR DENIED`, without waiting for the release. A press that is still going does not count for a request that arrives meanwhile (`pressIgnored`: the user has to let go and press again). For the autonomous wipe hold the same stops the hold bar without a flow.
- Button gesture for accepting: release the button inside a window of 1 s to 2 s of holding. Releasing before 1 s or after 2 s is not an accept. This avoids accidental presses and malware guessing the timing. The exact window may be tuned (**OPEN**).
  - A release before 1 s or after 2 s denies immediately (the host gets `ERR DENIED` and simply requests again).
  - The button is debounced in software (a state change counts only after the level is stable for about 20-30 ms), so a flaky contact edge cannot cause a false release/deny. Hold time is measured from the first stable press.
- "Wipe all keys" is an application feature, a serial command with the same gesture but a much longer window: release between 5 s and 6 s of holding (normal accept: 1-2 s). The long hold gives time to notice the difference and reconsider; wiping is rare, so the cost is irrelevant. Same deny rules apply. It replaces any wipe-on-flash.
- Autonomous wipe (implemented; compiled in unless `AUTONOMOUS_WIPE=0` is set for `firmware/core/build.sh`, enabled by config bit 3, default off): wipe without any host, e.g. when the device must be destroyed with only a power bank or phone charger at hand. At idle (no pending request) hold the button: after 2 s a bar with the checkerboard accept window (the end of the bar is 11 s, the window 10 to 11 s) appears under "WIPE ALL KEYS? / HOLD 10 S"; releasing inside the window starts the flow, releasing outside it just restores the idle screen. The flow: "WIPE ALL KEYS? / WAIT" with a bar that drains over 4 s (a press aborts), then the bootloader's gesture on "TAP TAP HOLD / 2 SECONDS": tap (released within 0.6 s), a second tap within 0.8 s (the bar runs to a third and two thirds in quick steps of 4 columns), then a third press held for 2 s while the bar fills; any mistake starts the gesture over, and 20 s after the start the flow is cancelled. Then the wipe runs with the display as progress bar. It only arms when keys exist and the config bit is set; during the flow every serial command gets `ERR BUSY`. `HELLO` and the banner report `caps=autowipe`; a firmware built without it rejects `SETCFG` with bit 3 (`ERR BADARG`). It only destroys data, so a bystander with physical access can at worst cause a denial of service (keys come back from the offline backups); that is why it defaults to off. Test: `tools/build/autowipe_test.py` (rig; 15 checks incl. 9 s, 12 s and 1.5 s holds do nothing, a press in the countdown cancels, wrong gestures wipe nothing, the right one wipes and keeps the configuration).
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
- Every token screen shows the offset ("UTC+2", right aligned on the first row next to "TOKEN") and the date and time with a 4 digit year on the last row ("15.10.2026 09:41"), so a wrongly set offset is visible, and the `SETTZ` confirm shows it prominently. The `SETTZ` confirm is the one place where the user must know the correct offset.

## Bootloader design (the key security decision)

Problem: with the stock Nano bootloader, host malware can reflash the firmware over serial (DTR resets the chip) and flash an app that dumps EEPROM. EEPROM is not erased by flashing.

Decision: a custom, button-gated bootloader. Host software cannot press the button, so it cannot open the bootloader.

- Installed once via ISP (a second Arduino as ISP or a USBasp).
- Fuses/lock bits: `BOOTRST` set (chip always starts in the bootloader, so the check cannot be bypassed); boot section locked against self-programming from the app (BLB1) so a flashed app cannot rewrite the bootloader; lock bits blocking ISP read-out. `EESAVE` must stay **unprogrammed**: otherwise a chip erase would clear the lock bits but keep the EEPROM.
- Boot section 1-2 KB (the ATmega328P allows up to 2 KB).
- Strip everything unneeded from optiboot, especially all EEPROM read/write handling. This is for size, not security: malicious _firmware_ reads EEPROM by itself, so the real protection is the gate. Flash read-back used to be offered for verification; the update PIN (below) lives in flash, so it is gone.
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

### The evil-maid problem and the update PIN

The worst remaining attack is not theft (a stolen device is noticed, the owner rotates passwords and resets MFA) but a **temporary physical access that goes unnoticed**: the attacker does the button gesture (trivially automated: two wires on the button pads and a microcontroller, like the Uno rig), flashes a firmware that dumps the EEPROM over serial, copies the keys, restores the original firmware and the original EEPROM, and puts the device back. Nothing on the chip can detect this afterwards: anything stored in rewritable memory (a counter, a flag) can be dumped and restored by the same firmware. So the only levers are what an attacker needs to know and how long each guess takes.

Decision: an optional **update PIN** that the bootloader asks for after the button gesture.

- The PIN (4 to 15 printable ASCII characters, 4 minimum; digits are fine) is stored in flash, the last application page `0x7780` (`PIN_ADDRESS`), characters followed by 0xFF; a blank page (first byte 0xFF) means no PIN and the gate is as before. It is set and changed by flashing that page, which needs the current PIN: `trustctl flash --set-pin` / `--clear-pin`, `trustctl flash pin --set-pin` for the PIN page alone. A forgotten PIN is not a disaster: the device keeps working (keys can be added and deleted), the owner re-adds every key, then rewrites the chip over ISP (`unlock.sh`, which erases everything).
- Protocol: after the gesture the bootloader shows `FIRMWARE UPDATE / PIN` and sends `?` every 100 ms. The host answers `0x02`, the characters and `\n` (junk before the `0x02`, such as sync polls, is ignored). The bootloader compares the characters as they arrive with the stored PIN without storing them or exiting early, then shows the same `FIRMWARE UPDATE / WAIT` screen with a bar draining over about 7 s (`CHECK_TICKS`) whatever the result. Only then does the update mode open (right PIN) or the application start (wrong PIN or a press during the bar). The verdict is not observable earlier, so resetting the chip during the wait gains the attacker nothing, and no counter in EEPROM (no room in 2 KB) is needed.
- Cost of a guess = the whole gate (about 8 s when done quickly) + about 7 s. A 4 digit PIN averages 5000 guesses × 15 s = 21 h, worst case 42 h; a 5 digit PIN 10 times that. The wait can be raised cheaply (it only costs the legitimate user at an update). People remember 4 digit PINs, a longer password invites a password manager, which malware can reach; a longer PIN stays an option.
- The PIN must never be readable from the host, which is our primary threat: the update mode has **no flash read-back and no compare command** (a good/bad answer to "does this page match" would be an oracle for guessing the PIN page at full speed). `READ_PAGE`, `SET_DEVICE`, `GET_PARAMETER` are gone from the bootloader (these also paid for the PIN code).
- Verification instead of read-back, both without an oracle: every `PROG_PAGE` carries one more byte, the sum of the page (catches line errors; STK500 has no checksum and 115200 baud at 16 MHz is 3.5 % off), and after writing a page the bootloader compares the flash with its RAM buffer and answers `STK_FAILED` on a mismatch. `trustctl` and `flash.py` resend a page up to 3 times. In-place verify alone would not catch a byte damaged on the line (it would be written and verified as damaged), the sum alone would not catch a flash write error; together they do.
- Where the PIN is typed matters: `trustctl flash` asks for it without echo, only at updates. A keylogger on the host sees it then; that is the narrow case where a compromised host and an evil maid combine, and it is accepted. Changing the PIN sends the new one over the same host.
- It only counts on a finished device: without the final lock bits (`final_lock.sh`, 0xCC) ISP reads the EEPROM directly. With them, ISP can only chip-erase, which destroys the keys (EESAVE unprogrammed). A tamper-evident seal on the case is the only detection available for the rest.
- Cuts that paid for it (the bootloader went from 2038 to 2040 of 2048 bytes): the `NO FIRMWARE / PRESS BUTTON` screen (a blank chip now starts the normal gate), the `WRITING` line, the unknown-character fallback of the font lookup, the double debounce of the button at power-up, and `run_app` no longer resets TWI, UART and the pull-up (the Arduino startup does).

### Bootloader implementation (done, on the dev Nano)

Source in `firmware/bootloader` (`boot.c`, `Makefile` using the Arduino avr-gcc, `gen_font.py` which generates `boot_font.h` with only the glyphs the messages need, from the CC0 Victoria font in `victoriamedium8.bin`). Written from scratch, not derived from optiboot. **2040 of 2048 bytes** (almost no margin; `make` checks the limit, any new feature needs something else to go). Linked at 0x7800 with `-nostartfiles`; the reset vector is a naked `start` that clears r1 and jumps to `main`.

- Normal start: button not held and a firmware present: `jmp 0` straight away, no display, no serial. Held at reset (or no firmware, which then goes through the normal gate): watchdog 2 s, display init (SSD1306 128x32, page addressing, rotation/invert from the application's EEPROM header byte 2, contrast 0x20 as a constant), then the gate. A dead display with a firmware present boots the application; without a firmware the chip halts.
- Gate: "RELEASE BUTTON" (inverted, 15 s timeout), then "FIRMWARE UPDATE / WAIT" with a bar in the bottom row that drains smoothly, one pixel column at a time, over about 4 s (a press of more than 30 ms while it drains makes the gate give up and reset, so a still-held finger gets a fresh gate), then "TAP TAP HOLD" (the "2 SECONDS" line had to go to make room; the filling bar shows how long to hold). The gesture is **tap, tap, then press a third time and hold 2 s** (taps released within 0.6 s, the next press within 0.8 s). The bottom row lights up one third per tap (two thirds after the second; the bar runs there in quick steps of 3 columns instead of jumping, and also runs back to 0 after a mistake), and fills smoothly from two thirds to the full width while the third press is held (about 1.9 s) so the user can see how far the gesture got; any mistake clears the bar and starts the gesture over; success shows "RELEASE BUTTON" again and the update mode opens after the release. Every wait has a 15 s timeout that ends in a reset. Button samples are every 10 ms and need 3 in a row.
- Update PIN: see "The evil-maid problem and the update PIN" above (`pin_gate()`, between the gesture and the update mode).
- Update mode: `UART0` 115200 baud, a minimal STK500v1 subset (sync and enter programming mode, which are harmless and share the default case, load address, program page with the sum byte and the in-place verify, read signature, leave progmode). Flash only: a program page for EEPROM is refused with `STK_FAILED`, writes must be page aligned and below 0x7800, a bad sum gets `STK_FAILED` without writing. No read page, no set device, no parameter queries: anything else is taken for a harmless one-byte command, and when its next byte is not the end-of-packet byte the chip resets. The watchdog (8 s) ends update mode when the host goes quiet; leaving progmode reboots. Flash cannot be read back (the PIN is in it).
- Deviations from the design above: the countdown digits became a draining bar and are not restarted by a press (it aborts, see above); the config byte is read without checking the header CRC (the check did not fit into 2 KB; a damaged byte only rotates or inverts the prompts); the update mode baud rate is 115200 (so the Arduino board variant for it is `atmega328`, not `atmega328old`).
- Installed via ISP: `avrdude -c stk500v1 -b 19200 -p m328p -P <uno> -e -U flash:w:boot.hex:i` (the `-e` chip erase also empties the EEPROM; to keep the EEPROM, e.g. for keys on the dev device, first write `hfuse 0xD2` (EESAVE programmed), then burn, then write `hfuse 0xDA` back), fuses unchanged (`lfuse 0xFF`, `hfuse 0xDA`, `efuse 0xFD`, so `BOOTRST` set, 2 KB boot section, `EESAVE` unprogrammed), lock byte `0xCF` (boot section protected against SPM; ISP read-out still open for development). **The final read-out lock bits are not set yet:** `tools/user/final_lock.sh <uno port> --yes` sets lock byte `0xCC` (LB mode 3 = no ISP read or write, BLB1 mode 3, BLB0 unrestricted because the bootloader has to write the application section) after checking the fuses; it is irreversible except by a chip erase, and has not been run.
- Flashing: `tools/build/flash.py firmware/core/build/core.ino.hex --port /dev/ttyUSB0 [--rig /dev/ttyUSB1]` (Python, Intel HEX parser, STK500v1 client with the sum byte per page; `--pin`, `--set-pin`, `--clear-pin`; the prototype of `trustctl flash`). Without `--rig` the user does the gate by hand; with the Uno rig the button is pressed automatically. `avrdude -c arduino -b 115200` has not been tried; it only retries about 10 times, far less than the gate takes, so a wrapper is needed anyway.
- Tests (`tools/build/boot_test.py`, all pass): a host that resets the chip and speaks STK500 gets no answer; a reset with the button held through it never opens the update mode (22 s observed) and times out into the application; pressing in the countdown, tap-tap without the hold, tap-tap and a 0.9 s hold, a tap and a hold (one tap too few), and a long first press all fail to open it; the correct gesture opens it; EEPROM write, a write into the bootloader's own section and a page with a wrong sum are refused, READ_PAGE returns no flash data; the update mode times out and the application still runs afterwards. With `--pin` the update PIN is tested too (it sets 4711, tries a wrong PIN, which starts the application and takes at least 6 s to be noticed, then the right one, which also takes 6 s, and removes it again; on an abort it tries to remove it).

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
- Test rig (done): `firmware/rig/rig.ino` on the Uno (115200 baud; `PRESS`, `RELEASE`, `SEQ <ms> ...`, `RESET <ms>`, `RESETHOLD <pre> <post>`) presses the Nano's button and resets it for the end-to-end tests. Measured hold times are 2-16 ms above the commanded ones (Uno `delay` plus serial latency), so keep at least 100 ms of margin to the window edges in tests. The Uno currently runs the rig sketch: to use ISP again, re-upload `ArduinoISP` (from `arduino/arduino-examples`, `examples/11.ArduinoISP`) to the Uno.
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
- Commands: `list`, `token NAME`, `add [NAME]` (secret or `otpauth://` URI from stdin, as a masked star field on a terminal (`cmd/trustctl/maskedinput.go`, unit-tested and tried on a real pty), never from the command line; without a NAME a URI supplies it: `Issuer:account`, or just the label when it already starts with the issuer, e.g. `GitHub-schorsch3000`; characters the device cannot show, like the @ of an e-mail address, become `-` and are reported; a name that is too long or empty is an error, nothing is cut off; a given NAME wins over the URI's label), `del NAME`, `tz [get|sync|set +H[:MM]]`, `config [get|set FLAG=on|off]`, `wipe`, `hello`. `NAME` is a name (any case), a slot number, or a unique prefix; a name beats a slot number. `token` first syncs the device's timezone offset to the host's (with a confirmation on the device if it differs). `--simulate` uses the simulator. Exit codes: 0 ok, 1 failure, 2 denied or timed out on the device, 64 usage, 69 no device.
- Resets on open: opening a serial port raises DTR, which resets the Nano (its display lights up, and the 1-2 s start-up delay follows), so even `list` or `tz get` woke the display. On Linux, macOS, FreeBSD and OpenBSD `serialdev` now clears HUPCL on the port before opening it, so DTR stays up between runs and only the first open after plugging in resets the device; later runs take about 0.15 s and do not touch the display (the firmware itself never wakes the display for the read-only commands). The connection asks `HELLO` instead of waiting for the banner, and accepts either. Windows and other systems still reset on every open until the daemon (which holds the port open) exists.
- `trustctl help-options` (`cmd/trustctl/helpoptions.go`) prints every setting with its `--argument`, environment variable, config file line and default (global options, per-command options, the profile keys of `device.json`, the settings stored on the device, the other environment variables read), wrapped to 78 columns, with the real config and profile paths. The config keys and environment variable names are shared constants (`configKey...`, `env...`), and tests check that the text mentions each of them, that the documented flags are accepted, that the documented config keys are parsed, and the order flag > environment > config file. `trustctl help` prints the short usage on stdout.
- The global options (`--device`, `--autoprobe`, `--simulate`, `--no-daemon`) may stand before or after the command (`takeGlobalOptions`), e.g. `trustctl calibrate --device /dev/ttyUSB0 logo x.png`; before this, the Go flag parser stopped at the command and the option was taken for an argument of `calibrate`. `calibrate` needs the calibration firmware on the device (`firmware/calibration/build.sh`, then `flash ... --no-patch`) and says so when it finds something else.
- Device selection: `--device PATH`, `TRUSTCTL_DEVICE`, or `device =` in `~/.config/trustctl/config` (also `autoprobe = true`); no implicit probing.
- Tests: `go test ./...` in `trustctl/` (protocol against the simulator, name resolution), and `tools/build/trustctl_e2e.py` against the real device with the Uno rig pressing the button (17 checks: add, duplicate, bad name, list, tz sync, token matching the host's TOTP, tap denies, config, del, wipe window); all pass.
- Build: `cd trustctl && GOPATH=$HOME/go go build -o bin/trustctl ./cmd/trustctl`. Note: this machine's shell sets `GOPATH` to a read-only system directory, so override it; the `go.mod` pulls a newer Go toolchain automatically (it needs network the first time).
- Portability: `CGO_ENABLED=0 go build` works for linux (amd64, arm64, arm, 386), windows (amd64, arm64), darwin (amd64, arm64), freebsd and openbsd. NetBSD does not build because `go.bug.st/serial` has no NetBSD support (only checked that it compiles; only Linux amd64 has run against the device).
- Daemon (`internal/daemon`, done): the first command starts it (`trustctl --idle 5m0s --device PATH __daemon`, detached with its own session, log in `$XDG_RUNTIME_DIR/trustctl/daemon.log`), later commands connect to the Unix socket `$XDG_RUNTIME_DIR/trustctl/trustctl.sock` (mode 0600 in a 0700 directory; fallback `<tmp>/trustctl-<uid>`). The socket speaks the device's own line protocol (the daemon sends a banner first), so the same client works over it; replies are streamed (`PENDING` reaches the user at once). It caches `LIST`, `GETTZ` and `GETCFG`, and drops the cache before and after `ADD`, `DEL`, `WIPE`, `SETTZ` and `SETCFG`. One request on the device at a time: a second client gets `ERR BUSY`. It exits when it is told to (`trustctl daemon stop`), after the idle timeout (default 5 min, `idle_timeout = 10m` in the config file), when the device disconnects, or when it notices the machine was suspended (a wall-clock jump of more than 5 s between two one-second ticks). Control lines: `!STATUS`, `!STOP`. `--no-daemon`, `TRUSTCTL_NO_DAEMON=1` or `daemon = false` talk to the device directly. A running daemon holds one device: asking for another `--device` is an error until `daemon stop`. `flash` and `calibrate` stop the daemon first. Unit tests use the simulator (caching, invalidation, busy, stop, idle, disconnect, a second daemon refused).
- `flash FILE.hex` (`internal/flasher`, `internal/hexfile`, `internal/params`, `internal/profile`, done): parses the hex, patches the device parameter block from the profile, stops the daemon, opens the port at 115200, prints `HOLD THE BUTTON ...`, counts down, resets the device with a DTR pulse, prints what to do on the display, polls `GET_SYNC` for up to 90 s (if the device answers with `?` it has an update PIN: asked for without echo, sent, then polling pauses 5 s while the device decides; a wrong PIN shows up as the application's banner and gives `ErrWrongPIN`), writes the pages (each with its sum byte, resent up to 3 times when the bootloader refuses it; the bootloader verifies in place, there is no read-back), and leaves programming mode. It refuses anything that touches the bootloader's section or the PIN page (`0x7780` and up) and needs `--device`. Options: `--no-patch`, `--profile PATH`, `--font`, `--set-pin`, `--clear-pin`, `--wait`, `--countdown`; `flash pin --set-pin|--clear-pin` writes only the PIN page. The PIN prompt reads a pipe too (one line per prompt). Opening the port alone no longer resets the device (HUPCL is cleared), so every tool that needs a reset (`tools/build/flash.py` too) pulses DTR explicitly. Test: `tools/build/trustctl_flash_e2e.py` (the Uno rig does the gesture when trustctl prints its prompts) flashes the calibration firmware, runs the calibrate commands, flashes the core firmware with a patch and checks the application answers; all pass.
- `calibrate` (`internal/logo`, done): `contrast full|dim VALUE`, `logo FILE.png [--invert]`, `font FILE.bin`, `show`, `forget logo|font|contrast`. They talk to the calibration firmware (version 0.2 adds `FONTDATA`/`FONTTEXT`), apply the setting live and remember it in `~/.config/trustctl/device.json` (`--profile PATH`), which `flash` patches into the core firmware. PNG conversion follows the "Logo images" rules above; unit tests cover black/near-black/transparent truecolor, 16-bit gray, palettes (darkest off, equal colors off, ties, white-first 1-bit palette), centering with odd margins, invert with margin, size limits and the bit layout.
- `font extract FILE.bin` / `logo extract FILE.png` (`cmd/trustctl/extract.go`, done): write the current blob to a file to work on. The device cannot be read back (lock bits), so "current" is what `flash` would patch in: the profile's font or logo, or with `--from FILE.hex` the one in a firmware's parameter block (`params.Extract`); without a profile font the built-in Victoria font is written (embedded copy in `internal/params`, a test keeps it equal to `firmware/fonts/victoriamedium8.bin`); without any logo it is an error. The logo is written as a 128x32 grayscale PNG (`logo.ToImage`, white = lit, already inverted if the profile says so, so converting it again without `--invert` gives the same bitmap).
- `tools/user/font2png.py` and `tools/user/png2font.py` (standalone, Python with Pillow): font file <-> PNG sheet of 16 x 6 glyphs (characters 32 to 127 row by row, 8x8 pixels each, 128x48 at scale 1, white = lit, exactly black = off), `--scale N` for easier editing (blocks must be uniform when converting back). Round trip of the Victoria font is byte exact at scale 1 and 5.
- Built-in firmware (done): `trustctl/internal/firmware` embeds `core.hex`, `calibration.hex` and `VERSION` (go:embed), produced by `tools/build/embed_firmware.sh` (builds both, copies, reads the version from `core.ino`) and committed so a plain `go build` works. trustctl is tightly coupled to the firmware version, so a new trustctl is released with every firmware version. `trustctl flash` (= `flash core`), `flash calibration` flash the built-in images (calibration is never patched); `flash FILE.hex` stays for development; the names win over files, use `./core` for a file of that name. `trustctl version` prints both versions; `tools/build/release.sh [--quick]` cross-compiles into `dist/` with `-X main.version` and `SHA256SUMS`. A test compares the embedded version with `core.ino`; the e2e flash test uses the built-ins by default.
- Built-in fonts (done): `trustctl/internal/fonts` embeds seven 772 byte fonts (`victoriamedium8` default, `victoriabold8`, `chroma48medium8`, `artossans8`, `artosserif8`, `torussansbold8`, `5x8`), generated by `tools/build/embed_fonts.py` from U8g2 (95 glyph fonts get a blank character 127). Only redistributable ones: the first six are in U8g2's "Open Game Art" group (CC0), 5x8 is X11 misc fixed (public domain). Left out because their terms need care: Press Start 2P (OFL, reserved name), PC Senior (codeman38's license), PxPlus IBM CGA / CGA thin (VileR), Amstrad CPC extended (ruboku); see `internal/fonts/README.md`. `trustctl font list`, `font extract FILE --name NAME`, `calibrate font NAME|FILE` (profile stores `builtin:NAME`), `flash --font NAME|FILE` (overrides the profile's font for that flash). A bare name is built in unless it has a directory part or an extension; `builtin:NAME` is explicit. A test keeps the default equal to `firmware/fonts/victoriamedium8.bin`. After a successful flash, `flash --font` also stores the font in the profile (`builtin:NAME` or the absolute path), so the profile always says what is on the device. `flash --font` replaces the profile's font before anything is read, so a profile that names a deleted font file does not block flashing another font (`flashPatch`); without `--font` a missing font file says to use `--font` or `calibrate forget font`.
- `tools/user/c64rom2font.py` (standalone, no dependencies): C64 character ROM to font files. ROM rows are one byte per pixel row (MSB left), 8 bytes per character in screen code order; transposed to the font's column bytes (bit 0 on top). 4096 bytes: set 1 (capitals and graphics) becomes `BASE_upper.EXT`, set 2 `BASE_lower.EXT`; 2048 bytes: `BASE_font.EXT` (`--set` says which set, default upper); never overwrites the ROM; `--outdir`. ASCII 32-63 map to screen codes 32-63, `@` 0, `[` 27, `]` 29, `^` the up arrow 30, `|` the vertical line 93, `_` the bottom line 100, `\\` the diagonal 77 (upper set only; blank in the lower, where 77 is an M), capitals 1-26 (upper) or 65-90 (lower), lower case 1-26 (in the upper set it shows capitals), the rest blank. Tried on `c64-chars.bin` (identical to VICE's chargen) and the PXL C64sys ROM in the project root; those two files are not committed (the original is Commodore's, the PXL one's license was not checked).
- Shell completion (`cmd/trustctl/completion.go`): `trustctl autocomplete` (also `autocomplete-bash`, `autocomplete-sh`) prints a bash script, `autocomplete-zsh` and `autocomplete-fish` the others; they are generated from one command table (a test checks that every command of the usage text is in it) and complete commands, global options, key names (through the hidden `trustctl __keys`, which prints the names one per line and is silent on any error; with the daemon it is quick), `tz`/`config`/`daemon`/`calibrate` sub-commands and `flash`/`calibrate` file names by extension. The bash script is tested by running real bash on 23 command lines with a fake `trustctl` in the PATH; the zsh and fish scripts were written to the shells' documented conventions but **not run** (neither shell is installed on the dev machine).
- Not done yet: signing or checksums of the profile, a `calibrate` subcommand for rotation and inversion (use `trustctl config set` on the core firmware), NetBSD, a Windows run.

- `tools/user/dummy-keys.html`: a single-file page (no network, no dependencies; its own SHA-1/HMAC, checked against the RFC vectors in node and driven once in headless Chromium) to make up dummy TOTP keys, stored unencrypted in localStorage, so that real keys never go onto the open dev device. It shows the live codes with a countdown, applies the device's name rules, offers "Copy add command" (`echo SECRET | trustctl add NAME --digits N --period S`), the secret and an otpauth URI, and checks a code typed from the device (current, previous or next window).

- `tools/user/trustctl-menu` (POSIX sh, Linux/BSD): a launcher front end for tokens. Parameters `chooser=PROGRAM` (a name or path; default the first of rofi, dmenu, wofi, bemenu, fuzzel; rofi gets `-dmenu`, wofi and fuzzel `--dmenu`), `target=stdout|type|copy`, for `type` `delay=MS` and `enter` (or `enter=1`), for `copy` `clipboard=clipboard|primary|secondary`; everything after `--` goes to the chooser. It takes the names from `trustctl list` (slot numbers cut off), runs `trustctl token NAME`, notifies via `notify-send` that the device waits for the button (the notice is posted with `-p` to get its id and a 30 s expiry, and closed as soon as `trustctl token` returns, also on failure and on Ctrl-C: through `gdbus` `CloseNotification`, else `busctl`, else by replacing it with a notice that expires after 1 ms; an old `notify-send` without `-p` simply lets it expire), and delivers the code through stdin only (`xdotool type --file -`, `wtype -`, `xclip`/`xsel`/`wl-copy`). Exit status 0, 1 nothing chosen, 2 error (also shown as a notification). `tools/build/test_trustctl_menu.sh` tests all of it with fake `trustctl`, launchers and typing/clipboard tools in a private PATH (45 checks; `shellcheck` is clean for the script); it has not been run against a real rofi, xdotool or clipboard.

## Tooling and environment

- `tools/user/` holds the tools for people who use the device (`trustctl-menu`, `dummy-keys.html`, `font2png.py`, `png2font.py`, `c64rom2font.py`, `final_lock.sh`); `tools/build/` holds what builds, releases and tests the project (end-to-end tests, `release.sh`, `embed_firmware.sh`, `embed_fonts.py`, `gen_params.py`, `extract_u8x8_font.py`, `flash.py`, `test_trustctl_menu.sh`). Scripts are run from the repository root, e.g. `tools/build/release.sh`.
- `arduino-cli` (with `arduino:avr`), `avrdude`, and PlatformIO are installed; the user is in the `dialout` group.
- Board: `arduino:avr:nano:cpu=atmega328old` is used for building (the bootloader is our own, so the choice only affects the compile settings). Serial ports of the dev rig: Nano `/dev/ttyUSB0`, Uno `/dev/ttyUSB1`; check them before every upload, they can re-enumerate.

## Status

Everything in this file marked "done" exists and was tested on the dev rig: the portable core library (`firmware/trustcore`, host tests in `firmware/test/run.sh`: SHA-1/HMAC/TOTP vectors, Base32, storage, protocol, time), the core firmware (`firmware/core`, 73% flash; one TOTP takes about 19 ms on the ATmega328P), the bootloader, the calibration firmware and `trustctl` with the built-in firmware and fonts.

- Tests: `firmware/test/run.sh` and `go test ./...` in `trustctl/` need no hardware; `tools/build/core_test.py [--slow]`, `autowipe_test.py`, `boot_test.py`, `trustctl_e2e.py`, `trustctl_flash_e2e.py` need the rig (the first, second and fourth want an empty device with default settings). Because the device adds elapsed time, test timestamps must not sit near a 30 s boundary.
- `firmware/core_selftest` runs the RFC vectors on the Nano itself (17% flash, 15% RAM); `firmware/rig` runs on the Uno and presses the button for the tests.
- Still open: the update PIN is written and unit tested against a fake bootloader but **not yet run on the device** (the new bootloader has to be burned first: `tools/user/flash_bootloader.sh PORT --yes --keep-eeprom`, then `tools/build/boot_test.py --pin`, `trustctl_flash_e2e.py`); the final lock bits on a finished device (`tools/user/final_lock.sh`, never run on the dev device), more large-font alert screens, the exact timings (marked **OPEN** above), and runs of `trustctl` on Windows, macOS and the BSDs and of the zsh and fish completions.
