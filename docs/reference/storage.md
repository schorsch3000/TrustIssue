# Limits and storage

## Keys

- **20 slots.** The slot index is the key's id (`trustctl list`). A deleted slot is free again.
- **Algorithm:** TOTP with HMAC-SHA1 only. SHA-256 and SHA-512 secrets, and HOTP, are not supported
  (the 2 KB of RAM of the ATmega328P is tight for SHA-512).
- **Secret:** at most 20 bytes (160 bits). Shorter secrets work: a short HMAC key padded with zeros gives
  the same result, so a 16 byte secret gives identical codes.
- **Digits:** 6, 7 or 8. **Period:** 30 or 60 seconds.

## Names

1 to 27 printable ASCII characters (`0x20` to `0x7E`: 95 symbols, exactly what the display font
draws). Mixed case is kept for display (`GitHub`), but names are unique ignoring case. No leading or
trailing space, no double spaces (two names would look identical on screen). Longer names, umlauts, emoji
and control characters are rejected, never truncated or silently changed, because truncation could create
two identical-looking keys.

Some characters look alike on a small display: `l` `1` `I` `|` `!`, `O` `0`, `'` `` ` ``, `,` `.`, `;` `:`,
`-` `_` `~`. The default font draws them distinctly, and {doc}`../setup/calibration` shows how to compare
fonts.

## What is stored where

| Where                         | What                                                                                  |
| ----------------------------- | ------------------------------------------------------------------------------------- |
| EEPROM (1 KB)                 | the keys (51 bytes per slot, own CRC-16), a 4 byte header: layout version, timezone offset, settings flags, CRC |
| flash, parameter block `0x7000` | the font, the logo and the two contrast values (patched in by `trustctl flash`)     |
| flash, page `0x7780`          | the update PIN (blank = none)                                                         |
| flash, `0x7800` and up        | the bootloader (2 KB)                                                                 |

A slot is used only if its used bit is set and its CRC matches, and the CRC is written last, so an
interrupted add leaves an unused slot, never a half written key. A damaged header falls back to defaults
and never touches the keys. EEPROM wear (about 100,000 writes per cell) is irrelevant: writes only happen
on add and delete.

Flashing the firmware does not touch the EEPROM: keys, timezone and settings survive an update. The
EEPROM layout version in the header guards against incompatible firmware versions.
