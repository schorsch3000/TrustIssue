#!/usr/bin/env python3
"""Drive the Uno test rig and check what the Nano's button_test firmware reports.

  tools/rig.py [nano_port] [uno_port]      (defaults: /dev/ttyUSB0 /dev/ttyUSB1)
"""
import sys
import time

import serial

# (name, rig command, expected classes of the RELEASE lines, expected glitches >= )
SCENARIOS = [
    ("tap 100 ms", "SEQ 100", ["DENY-SHORT"], 0),
    ("hold 900 ms", "SEQ 900", ["DENY-SHORT"], 0),
    ("hold 1100 ms", "SEQ 1100", ["ACCEPT"], 0),
    ("hold 1900 ms", "SEQ 1900", ["ACCEPT"], 0),
    ("hold 2100 ms", "SEQ 2100", ["DENY-LONG"], 0),
    ("bouncy press, then hold 1500 ms", "SEQ 2 1 1 2 1500", ["ACCEPT"], 1),
    ("hold 1500 ms, bouncy release", "SEQ 1500 2 1 2 3", ["ACCEPT"], 1),
    ("wipe hold 5500 ms", "SEQ 5500", ["WIPE"], 0),
    ("double tap and hold", "SEQ 80 120 80 120 2000", ["DENY-SHORT", "DENY-SHORT", "DENY-LONG"], 0),
]


def read_lines(port, seconds):
    end = time.time() + seconds
    lines = []
    while time.time() < end:
        raw = port.readline()
        if raw:
            lines.append(raw.decode("ascii", "replace").strip())
    return lines


def rig_command(rig, command):
    rig.write((command + "\n").encode())
    answer = rig.readline().decode().strip()
    if answer != "OK":
        raise SystemExit(f"rig: {command!r} -> {answer!r}")


def main():
    nano_path = sys.argv[1] if len(sys.argv) > 1 else "/dev/ttyUSB0"
    uno_path = sys.argv[2] if len(sys.argv) > 2 else "/dev/ttyUSB1"
    nano = serial.Serial(nano_path, 9600, timeout=0.2)
    rig = serial.Serial(uno_path, 115200, timeout=30)
    time.sleep(2.5)  # opening both ports resets both boards
    nano.reset_input_buffer()
    rig.reset_input_buffer()
    rig.write(b"HELLO\n")
    print("rig:", rig.readline().decode().strip())
    failures = 0
    for name, command, expected, min_glitches in SCENARIOS:
        nano.reset_input_buffer()
        rig_command(rig, command)
        lines = read_lines(nano, 0.5)
        releases = [l for l in lines if l.startswith("RELEASE")]
        classes = [l.split()[2] for l in releases]
        glitches = sum(int(l.split("glitches=")[1]) for l in releases)
        ok = classes == expected and glitches >= min_glitches
        failures += not ok
        print(f"{'PASS' if ok else 'FAIL'}  {name}: {releases or lines}")
    print(f"{len(SCENARIOS) - failures}/{len(SCENARIOS)} passed")
    raise SystemExit(1 if failures else 0)


if __name__ == "__main__":
    main()
