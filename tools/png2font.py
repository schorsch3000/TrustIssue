#!/usr/bin/env python3
"""Turn a PNG sheet (as written by font2png.py) into a Trust Issues font file.

  tools/png2font.py sheet.png font.bin [--scale N]

The sheet must be 128x48 pixels (times N): 16 glyphs per row, 6 rows,
characters 32 to 127 row by row, 8x8 pixels per glyph. Exactly black is off,
any other color is lit (transparent pixels count as black). With --scale N
every pixel of the sheet is an N x N block, which must be uniform. The result
is 772 bytes: header 32 127 1 1, then 8 column bytes per glyph, bit 0 on top.
"""
import argparse
import sys

from PIL import Image

COLUMNS, ROWS, GLYPH, FIRST, LAST = 16, 6, 8, 32, 127


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("png")
    ap.add_argument("font")
    ap.add_argument("--scale", type=int, default=1, help="pixel size in the PNG (default 1)")
    args = ap.parse_args()
    if args.scale < 1:
        sys.exit("--scale must be at least 1")

    img = Image.open(args.png).convert("RGBA")
    s = args.scale
    want = (COLUMNS * GLYPH * s, ROWS * GLYPH * s)
    if img.size != want:
        sys.exit(f"{args.png}: the sheet must be {want[0]}x{want[1]} pixels at scale {s}, this one is {img.width}x{img.height}")
    bg = Image.new("RGBA", img.size, (0, 0, 0, 255))
    pixels = Image.alpha_composite(bg, img).convert("RGB").load()

    def lit(x, y):
        values = {pixels[x * s + i, y * s + j] != (0, 0, 0) for i in range(s) for j in range(s)}
        if len(values) > 1:
            sys.exit(f"{args.png}: the {s}x{s} block of pixel {x},{y} is not uniform")
        return values.pop()

    out = bytearray([FIRST, LAST, 1, 1])
    for index in range(LAST - FIRST + 1):
        gx, gy = index % COLUMNS * GLYPH, index // COLUMNS * GLYPH
        for x in range(GLYPH):
            out.append(sum(lit(gx + x, gy + y) << y for y in range(GLYPH)))
    with open(args.font, "wb") as f:
        f.write(out)


if __name__ == "__main__":
    main()
