#!/usr/bin/env python3
"""Draws the favicon of the documentation: the device seen from above, a dark case with the red button on
the left and the display on the right. Writes docs/_static/favicon.ico and favicon.png."""

import os

from PIL import Image, ImageDraw

OUT = os.path.join(os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__)))), "docs", "_static")
N = 512


def icon():
    image = Image.new("RGBA", (N, N), (0, 0, 0, 0))
    d = ImageDraw.Draw(image)
    d.rounded_rectangle((16, 120, 496, 392), radius=44, fill=(38, 40, 46, 255))   # the case
    d.ellipse((50, 160, 210, 320), fill=(205, 45, 40, 255))                           # the red button
    d.ellipse((74, 178, 186, 270), fill=(228, 80, 70, 255))                           # its highlight
    d.rounded_rectangle((246, 178, 470, 334), radius=14, fill=(6, 10, 16, 255))        # the display
    for row, (x0, x1) in enumerate(((268, 440), (268, 380), (268, 410))):             # three lines of text
        y = 200 + row * 40
        d.rectangle((x0, y, x1, y + 20), fill=(190, 225, 255, 255))
    return image


def main():
    big = icon()
    big.resize((64, 64), Image.LANCZOS).save(os.path.join(OUT, "favicon.png"))
    big.save(os.path.join(OUT, "favicon.ico"), sizes=[(16, 16), (32, 32), (48, 48), (64, 64)])


if __name__ == "__main__":
    main()
