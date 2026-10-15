#!/usr/bin/env python3
"""Extracts the fonts that trustctl carries from U8g2 into trustctl/internal/fonts.

  tools/embed_fonts.py [path/to/u8x8_fonts.c]

Only fonts whose license allows redistribution are listed (see FONTS and
trustctl/internal/fonts/README.md). Fonts with 95 glyphs (characters 32 to 126)
get a blank character 127, so every file is a 772 byte font in the format
trustctl expects. Run it after changing FONTS and commit the result.
"""
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from extract_u8x8_font import extract  # noqa: E402

DEFAULT_SOURCE = os.path.expanduser("~/Arduino/libraries/U8g2/src/clib/u8x8_fonts.c")
# trustctl name -> U8g2 name. The first six come from the "Open Game Art" group (CC0), 5x8 is the
# X11 "misc fixed" font ("Public domain font. Share and enjoy.").
FONTS = {
    "victoriamedium8": "victoriamedium8_r",
    "victoriabold8": "victoriabold8_r",
    "chroma48medium8": "chroma48medium8_r",
    "artossans8": "artossans8_r",
    "artosserif8": "artosserif8_r",
    "torussansbold8": "torussansbold8_r",
    "5x8": "5x8_r",
}
FIRST, LAST = 32, 127


def normalize(name, data):
    first, last = data[0], data[1]
    if (first, data[2], data[3]) != (FIRST, 1, 1) or last > LAST:
        raise SystemExit(f"{name}: unexpected header {list(data[:4])}")
    glyphs = data[4:4 + (last - first + 1) * 8]
    if len(glyphs) != (last - first + 1) * 8:
        raise SystemExit(f"{name}: too short")
    glyphs += bytes(8 * (LAST - last))
    return bytes([FIRST, LAST, 1, 1]) + glyphs


def main():
    source = sys.argv[1] if len(sys.argv) > 1 else DEFAULT_SOURCE
    out = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "trustctl", "internal", "fonts")
    for name, u8name in FONTS.items():
        data = normalize(name, extract(source, "u8x8_font_" + u8name))
        assert len(data) == 772
        with open(os.path.join(out, name + ".bin"), "wb") as f:
            f.write(data)
        print("wrote", name)


if __name__ == "__main__":
    main()
