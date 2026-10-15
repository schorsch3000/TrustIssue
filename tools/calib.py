#!/usr/bin/env python3
"""Throwaway driver for the calibration firmware (the real tool will be trustctl).

Examples:
  tools/calib.py /dev/ttyUSB0 demo
  tools/calib.py /dev/ttyUSB0 contrast 40
  tools/calib.py /dev/ttyUSB0 flip 1
  tools/calib.py /dev/ttyUSB0 pattern
  tools/calib.py /dev/ttyUSB0 box
  tools/calib.py /dev/ttyUSB0 logo logo.png [--invert]
  tools/calib.py /dev/ttyUSB0 raw A7
"""
import sys
import time

import serial
from PIL import Image

WIDTH, HEIGHT = 128, 32


def open_port(path):
    port = serial.Serial(path, 9600, timeout=2)
    time.sleep(2.0)  # opening the port resets the Nano
    port.reset_input_buffer()
    return port


def send(port, command):
    port.write((command + "\n").encode("ascii"))
    answer = port.readline().decode("ascii", "replace").strip()
    if answer != "OK" and not answer.startswith("TrustIssue"):
        raise SystemExit(f"{command!r} -> {answer!r}")
    return answer


def test_pattern(frame_only=False):
    """A frame, a diagonal from top-left to bottom-right, and a 'T' block."""
    pixels = set()
    for x in range(WIDTH):
        pixels.add((x, 0))
        pixels.add((x, HEIGHT - 1))
        if not frame_only:
            pixels.add((x, x * (HEIGHT - 1) // (WIDTH - 1)))
    for y in range(HEIGHT):
        pixels.add((0, y))
        pixels.add((WIDTH - 1, y))
    if frame_only:
        return pack(pixels)
    for x in range(8, 24):
        pixels.add((x, 6))
    for y in range(6, 20):
        pixels.add((15, y))
    return pack(pixels)


def pack(pixels):
    data = bytearray(WIDTH * HEIGHT // 8)
    for x, y in pixels:
        data[(y // 8) * WIDTH + x] |= 1 << (y % 8)
    return bytes(data)


def load_logo(path, invert=False):
    """PNG to pixel set, following the rules in CLAUDE.md ("Logo images")."""
    image = Image.open(path)
    width, height = image.size
    if width > WIDTH or height > HEIGHT:
        raise SystemExit(f"{path}: {width}x{height} is larger than {WIDTH}x{HEIGHT}")
    if image.mode == "P":
        palette = image.getpalette() or []
        colors = {}
        for index in set(image.getdata()):
            colors[index] = tuple(palette[3 * index:3 * index + 3])
        # darkest by luminance; ties go to the first palette entry
        off_color = min(
            ((299 * r + 587 * g + 114 * b, index, (r, g, b))
             for index, (r, g, b) in colors.items()))[2]
        is_on = lambda value: colors[value] != off_color
        values = image.getdata()
    elif image.mode in ("I;16", "I", "F"):
        is_on = lambda value: value != 0
        values = image.getdata()
    else:
        black = Image.new("RGBA", image.size, (0, 0, 0, 255))
        flat = Image.alpha_composite(black, image.convert("RGBA"))
        is_on = lambda value: value[:3] != (0, 0, 0)
        values = flat.getdata()
    left, top = (WIDTH - width) // 2, (HEIGHT - height) // 2
    pixels = set()
    for i, value in enumerate(values):
        if is_on(value) != invert:
            pixels.add((left + i % width, top + i // width))
    if invert:  # inverting also lights the margin around a smaller image
        for y in range(HEIGHT):
            for x in range(WIDTH):
                if not (left <= x < left + width and top <= y < top + height):
                    pixels.add((x, y))
    return pack(pixels)


def send_bitmap(port, data):
    for offset in range(0, len(data), 16):
        send(port, f"LOGO {offset} {data[offset:offset + 16].hex()}")


def main():
    if len(sys.argv) < 3:
        raise SystemExit(__doc__)
    port = open_port(sys.argv[1])
    command, args = sys.argv[2], sys.argv[3:]
    print(send(port, "HELLO"))
    if command == "demo":
        send(port, "CLEAR")
        send(port, "TEXT 0 Trust Issues")
        send(port, "TEXT 1 0123456789ABCDEF")
        send(port, "BIG 2 09:41")
    elif command in ("contrast", "flip", "invert", "power"):
        send(port, f"{command.upper()} {args[0]}")
    elif command == "pattern":
        send(port, "CLEAR")
        send_bitmap(port, test_pattern())
    elif command == "box":
        send(port, "CLEAR")
        send_bitmap(port, test_pattern(frame_only=True))
    elif command == "logo":
        send(port, "CLEAR")
        send_bitmap(port, load_logo(args[0], "--invert" in args[1:]))
    elif command == "raw":
        send(port, f"RAW {args[0]}")
    elif command == "clear":
        send(port, "CLEAR")
    else:
        raise SystemExit(__doc__)
    print("OK")


if __name__ == "__main__":
    main()
