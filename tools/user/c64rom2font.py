#!/usr/bin/env python3
"""Convert a C64 character ROM into Trust Issue font files (772 bytes, U8x8 format).

  tools/user/c64rom2font.py ROMFILE [--set upper|lower] [--outdir DIR]

A character ROM holds 8 bytes per character (one byte per pixel row, the most
significant bit is the left pixel), 256 characters per character set, in
screen code order:

  4096 bytes: two sets. Two fonts are written, next to the ROM file (or in
              --outdir): BASENAME_upper.EXT (the set with capitals and graphics)
              and BASENAME_lower.EXT (the set with lower case and capitals).
  2048 bytes: one set, one font, BASENAME_font.EXT. --set says which set the
              ROM holds (default upper).

Only the characters ASCII 32 to 127 are needed, so only the first 128 screen
codes (the non-reversed ones) are read. Where the C64 has no glyph for an ASCII
character it is left blank (` { } ~ DEL) or replaced by the closest one:
\\ is the diagonal line graphic (upper set only), | the vertical line, _ the bottom line, ^ the
up arrow, and in the upper set the lower case letters show the capitals.
"""
import argparse
import os
import sys

FIRST, LAST = 32, 127

# ASCII -> screen code, the same in both sets
COMMON = {64: 0, 91: 27, 93: 29, 94: 30, 124: 93, 95: 100}
COMMON.update({c: c for c in range(32, 64)})


def screen_code(ascii_code, lower_set):
    if ascii_code in COMMON:
        return COMMON[ascii_code]
    if ascii_code == 92:  # the diagonal line graphic exists only in the upper set (77 is an M in the other)
        return None if lower_set else 77
    if 65 <= ascii_code <= 90:  # capitals
        return ascii_code if lower_set else ascii_code - 64
    if 97 <= ascii_code <= 122:  # lower case: its own glyph, or the capital in the upper set
        return ascii_code - 96
    return None  # blank


def convert(rom, lower_set):
    out = bytearray([FIRST, LAST, 1, 1])
    for ascii_code in range(FIRST, LAST + 1):
        code = screen_code(ascii_code, lower_set)
        rows = rom[code * 8:code * 8 + 8] if code is not None else bytes(8)
        for x in range(8):  # column x, bit 0 on top
            out.append(sum(((rows[y] >> (7 - x)) & 1) << y for y in range(8)))
    assert len(out) == 772
    return bytes(out)


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("rom")
    ap.add_argument("--set", choices=("upper", "lower"), default="upper", help="which set a 2048 byte ROM holds")
    ap.add_argument("--outdir", help="where to write (default: next to the ROM file)")
    args = ap.parse_args()
    with open(args.rom, "rb") as f:
        rom = f.read()
    base, ext = os.path.splitext(os.path.basename(args.rom))
    outdir = args.outdir or os.path.dirname(os.path.abspath(args.rom))

    def write(name, data):
        path = os.path.join(outdir, name)
        with open(path, "wb") as f:
            f.write(data)
        print("wrote", path)

    if len(rom) == 4096:
        write(f"{base}_upper{ext}", convert(rom[:2048], False))
        write(f"{base}_lower{ext}", convert(rom[2048:], True))
    elif len(rom) == 2048:
        write(f"{base}_font{ext}", convert(rom, args.set == "lower"))
    else:
        sys.exit(f"{args.rom}: a character ROM has 2048 or 4096 bytes, this one has {len(rom)}")


if __name__ == "__main__":
    main()
