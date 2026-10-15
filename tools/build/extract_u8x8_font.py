#!/usr/bin/env python3
"""Extracts a U8x8 font array from U8g2's u8x8_fonts.c and writes it as raw bytes
or as a C header (PROGMEM array).

  tools/build/extract_u8x8_font.py <u8x8_fonts.c> <font name> bin  <out.bin>
  tools/build/extract_u8x8_font.py <u8x8_fonts.c> <font name> hdr  <out.h>
"""
import re
import sys

ESCAPES = {"n": 10, "t": 9, "r": 13, "\\": 92, '"': 34, "'": 39, "a": 7, "b": 8, "f": 12, "v": 11}


def decode_c_string(body):
    data = bytearray()
    for literal in re.findall(r'"((?:[^"\\]|\\.)*)"', body, re.S):
        i = 0
        while i < len(literal):
            c = literal[i]
            if c != "\\":
                data.append(ord(c))
                i += 1
            elif literal[i + 1] == "x":
                m = re.match(r"[0-9a-fA-F]+", literal[i + 2:])
                data.append(int(m.group(0), 16) & 0xFF)
                i += 2 + len(m.group(0))
            elif literal[i + 1] in "01234567":
                m = re.match(r"[0-7]{1,3}", literal[i + 1:])
                data.append(int(m.group(0), 8) & 0xFF)
                i += 1 + len(m.group(0))
            else:
                data.append(ESCAPES[literal[i + 1]])
                i += 2
    return bytes(data)


def extract(path, name):
    source = open(path, errors="replace").read()
    m = re.search(r"const uint8_t %s\[(\d+)\][^=]*=\s*(.*?);" % re.escape(name), source, re.S)
    if not m:
        raise SystemExit(f"{name} not found in {path}")
    data = decode_c_string(m.group(2))
    size = int(m.group(1))
    if len(data) == size - 1:  # the array holds the string's terminating NUL
        data += b"\0"
    if len(data) != size:
        raise SystemExit(f"{name}: decoded {len(data)} bytes, expected {m.group(1)}")
    return data


def main():
    if len(sys.argv) != 5:
        raise SystemExit(__doc__)
    _, path, name, kind, out = sys.argv
    data = extract(path, name)
    if kind == "bin":
        open(out, "wb").write(data)
    else:
        lines = []
        for i in range(0, len(data), 12):
            lines.append("    " + ", ".join(f"0x{b:02X}" for b in data[i:i + 12]) + ",")
        open(out, "w").write(
            f"// {name}: U8x8 font, {len(data)} bytes = 4 byte header + 8 bytes per glyph.\n"
            "// Extracted from U8g2 (Open Game Art group of fonts, CC0 / public domain).\n"
            "#include <avr/pgmspace.h>\n"
            f"static const uint8_t FONT_DEFAULT[{len(data)}] PROGMEM = {{\n" + "\n".join(lines) + "\n};\n")
    print(f"{name}: {len(data)} bytes, header {list(data[:4])}")


if __name__ == "__main__":
    main()
