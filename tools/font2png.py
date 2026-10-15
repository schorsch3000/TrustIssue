#!/usr/bin/env python3
"""Turn a Trust Issues font file (772 bytes, U8x8 format) into a PNG sheet.

  tools/font2png.py font.bin sheet.png [--scale N]

The sheet has 16 glyphs per row and 6 rows (characters 32 to 127, row by row),
8x8 pixels each: 128x48 pixels at scale 1. White is lit. --scale N enlarges
every pixel to N x N for easier editing; png2font.py takes the same option.
"""
import argparse
import sys

from PIL import Image

COLUMNS, ROWS, GLYPH, FIRST, LAST = 16, 6, 8, 32, 127
FONT_SIZE = 4 + (LAST - FIRST + 1) * GLYPH
HEADER = bytes([FIRST, LAST, 1, 1])


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("font")
    ap.add_argument("png")
    ap.add_argument("--scale", type=int, default=1, help="pixel size in the PNG (default 1)")
    args = ap.parse_args()
    if args.scale < 1:
        sys.exit("--scale must be at least 1")
    with open(args.font, "rb") as f:
        data = f.read()
    if len(data) != FONT_SIZE or data[:4] != HEADER:
        sys.exit(f"{args.font}: not a font file ({FONT_SIZE} bytes, header {list(HEADER)} expected)")

    img = Image.new("L", (COLUMNS * GLYPH, ROWS * GLYPH), 0)
    for index in range(LAST - FIRST + 1):
        gx, gy = index % COLUMNS * GLYPH, index // COLUMNS * GLYPH
        for x in range(GLYPH):
            column = data[4 + index * GLYPH + x]
            for y in range(GLYPH):
                if column >> y & 1:  # bit 0 is the top pixel
                    img.putpixel((gx + x, gy + y), 255)
    if args.scale > 1:
        img = img.resize((img.width * args.scale, img.height * args.scale), Image.NEAREST)
    img.save(args.png)


if __name__ == "__main__":
    main()
