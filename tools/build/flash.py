#!/usr/bin/env python3
"""Flashes an Intel HEX file through the Trust Issues bootloader (STK500v1, flash only).

  tools/build/flash.py firmware.hex [--port /dev/ttyUSB0] [--rig /dev/ttyUSB1] [--baud 115200]

The bootloader only opens after a physical gate: hold the button while the device
resets, release when told to, wait for the bar to drain, then tap, tap, and press
again and hold for 2 seconds. Without --rig you do that yourself: hold the button BEFORE the
tool resets the device (it counts down first). With --rig the Uno test rig presses
the button for you (the rig wiring in CLAUDE.md).

This is the prototype of `trustctl flash`.
"""
import argparse
import sys
import threading
import time

import serial

PAGE = 128
FLASH_END = 0x7800  # the bootloader's own section starts here

INSYNC, OK = 0x14, 0x10


def read_hex(path):
    data = {}
    base = 0
    for line in open(path):
        line = line.strip()
        if not line.startswith(":"):
            continue
        raw = bytes.fromhex(line[1:])
        length, address, kind = raw[0], (raw[1] << 8) | raw[2], raw[3]
        payload = raw[4:4 + length]
        if sum(raw) & 0xFF:
            raise SystemExit(f"{path}: bad checksum in {line}")
        if kind == 0:
            for i, byte in enumerate(payload):
                data[base + address + i] = byte
        elif kind == 2:
            base = ((payload[0] << 8) | payload[1]) << 4
        elif kind == 4:
            base = ((payload[0] << 8) | payload[1]) << 16
        elif kind == 1:
            break
    if not data:
        raise SystemExit(f"{path}: no data")
    if max(data) >= FLASH_END:
        raise SystemExit(f"{path}: data beyond {FLASH_END:#x}, that is the bootloader's section")
    return data


def pages_of(data):
    pages = {}
    for address, byte in data.items():
        page = address - address % PAGE
        pages.setdefault(page, bytearray(b"\xff" * PAGE))[address % PAGE] = byte
    return sorted(pages.items())


class Rig:
    """The Uno test rig: presses the button with exact timings."""

    def __init__(self, path):
        self.port = serial.Serial(path, 115200, timeout=30)
        time.sleep(2.5)
        self.port.reset_input_buffer()

    def command(self, text):
        self.port.write((text + "\n").encode())
        if self.port.readline().decode().strip() != "OK":
            raise SystemExit(f"rig: {text} failed")

    def command_in_background(self, text):
        thread = threading.Thread(target=self.command, args=(text,))
        thread.start()
        return thread


class Programmer:
    def __init__(self, port):
        self.port = port

    def exchange(self, request, reply_length):
        self.port.write(bytes(request))
        reply = self.port.read(reply_length)
        if len(reply) != reply_length or reply[0] != INSYNC or reply[-1] != OK:
            raise SystemExit(f"unexpected reply {reply.hex()} to {bytes(request)[:1].hex()}")
        return reply[1:-1]

    def load_address(self, byte_address):
        word = byte_address >> 1
        self.exchange([0x55, word & 0xFF, word >> 8, 0x20], 2)

    def write_page(self, byte_address, data):
        self.load_address(byte_address)
        self.exchange([0x64, len(data) >> 8, len(data) & 0xFF, ord("F")] + list(data) + [0x20], 2)

    def read_page(self, byte_address, length):
        self.load_address(byte_address)
        return self.exchange([0x74, length >> 8, length & 0xFF, ord("F"), 0x20], 2 + length)


def open_gate_and_sync(args, rig=None):
    rig = rig or (Rig(args.rig) if args.rig else None)
    if rig:
        rig.command("PRESS")
    else:
        print("Hold the button now. The device is reset in 5 seconds; keep holding until the display says RELEASE BUTTON.")
        for n in range(5, 0, -1):
            print(f"  {n}...", flush=True)
            time.sleep(1)
    port = serial.Serial(args.port, args.baud, timeout=0.2)
    port.dtr = False  # a DTR pulse resets the device (opening alone may not: DTR can stay up between runs)
    time.sleep(0.1)
    port.dtr = True
    pressing = None
    if rig:
        time.sleep(0.6)
        rig.command("RELEASE")
        time.sleep(5.3)  # the draining bar (4 s) plus the gesture prompt
        pressing = rig.command_in_background("SEQ 150 200 150 200 2300")  # tap, tap, press and hold
    else:
        print("Follow the display: release the button, wait for the bar to drain, then tap, tap, and press again and hold for 2 s.")
    deadline = time.time() + args.wait
    while time.time() < deadline:
        port.reset_input_buffer()
        port.write(b"\x30\x20")
        if port.read(2) == bytes([INSYNC, OK]):
            if pressing:
                pressing.join()
            return port
        time.sleep(0.2)
    raise SystemExit("the bootloader did not open (gate not passed?)")


def main():
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("hexfile")
    parser.add_argument("--port", default="/dev/ttyUSB0")
    parser.add_argument("--rig")
    parser.add_argument("--baud", type=int, default=115200)
    parser.add_argument("--wait", type=float, default=60, help="seconds to wait for the gate")
    args = parser.parse_args()

    pages = pages_of(read_hex(args.hexfile))
    print(f"{len(pages)} pages, {sum(len(p) for _, p in pages)} bytes")
    port = open_gate_and_sync(args)
    port.timeout = 2
    programmer = Programmer(port)
    programmer.exchange([0x42] + [0] * 20 + [0x20], 2)
    programmer.exchange([0x45] + [0] * 5 + [0x20], 2)
    programmer.exchange([0x50, 0x20], 2)
    for address, data in pages:
        programmer.write_page(address, data)
    print("written, verifying")
    for address, data in pages:
        if programmer.read_page(address, PAGE) != bytes(data):
            raise SystemExit(f"verify failed at {address:#06x}")
    programmer.exchange([0x51, 0x20], 2)
    print("done, the device restarts into the new firmware")


if __name__ == "__main__":
    main()
