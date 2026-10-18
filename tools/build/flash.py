#!/usr/bin/env python3
"""Flashes an Intel HEX file through the Trust Issue bootloader (STK500v1, flash only).

  tools/build/flash.py firmware.hex [--port /dev/ttyUSB0] [--rig /dev/ttyUSB1] [--baud 115200]
                       [--pin PIN] [--set-pin NEWPIN | --clear-pin]

--pin answers the device's update PIN question, --set-pin and --clear-pin add the PIN page to the flash.

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

from rig import Rig  # noqa: E402  (the button finger: firmware/rigisp)

PAGE = 128
FLASH_END = 0x7800  # the bootloader's own section starts here

INSYNC, OK = 0x14, 0x10
PIN_PAGE = 0x7780  # the update PIN: up to 15 characters, then 0xFF


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
    if max(data) >= PIN_PAGE:
        raise SystemExit(f"{path}: data beyond {PIN_PAGE:#x}: the update PIN page and the bootloader section are not for firmware")
    return data


def pages_of(data):
    pages = {}
    for address, byte in data.items():
        page = address - address % PAGE
        pages.setdefault(page, bytearray(b"\xff" * PAGE))[address % PAGE] = byte
    return sorted(pages.items())


class WrongPin(Exception):
    """The device took the PIN and started the application instead of the update mode."""


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
        """The bootloader wants the sum of the page after it, and verifies what it wrote."""
        self.load_address(byte_address)
        self.exchange([0x64, len(data) >> 8, len(data) & 0xFF, ord("F")] + list(data) + [sum(data) & 0xFF, 0x20], 2)


class GateSilent(SystemExit):
    """The device never answered at all: the gesture probably missed its moment."""


def open_gate_and_sync(args, rig=None, attempts=3):
    """Opens the update mode. With the rig the whole sequence is tried again (up to `attempts` times, 25 s each, the
    last one for args.wait) when the device does not answer at all, which happens on a slow machine; a retry is
    announced, so that flakes do not go unnoticed. A device that asks for its PIN is never retried."""
    rig = rig or (Rig(args.rig) if args.rig else None)
    if not rig:
        return _open_gate_once(args, rig, args.wait)
    for attempt in range(1, attempts + 1):
        try:
            return _open_gate_once(args, rig, args.wait if attempt == attempts else min(args.wait, 25))
        except GateSilent:
            if attempt == attempts:
                raise
            print(f"NOTE: the gate did not answer, trying again (attempt {attempt + 1} of {attempts})", flush=True)
            time.sleep(3)


def _open_gate_once(args, rig, wait):
    if rig:
        rig.command("PRESS")
    else:
        print("Hold the button now. The device is reset in 5 seconds; keep holding until the display says RELEASE BUTTON.")
        for n in range(5, 0, -1):
            print(f"  {n}...", flush=True)
            time.sleep(1)
    port = serial.Serial(args.port, args.baud, timeout=0.2)
    # a DTR pulse resets the device (opening alone may not: DTR can stay up between runs); RTS with it, because on
    # some CH340 boards the reset capacitor hangs on RTS and DTR alone does nothing
    port.dtr = False
    port.rts = False
    time.sleep(0.1)
    port.rts = True
    port.dtr = True
    pressing = None
    if rig:
        time.sleep(0.6)
        rig.command("RELEASE")
        time.sleep(5.3)  # the draining bar (4 s) plus the gesture prompt
        pressing = rig.command_in_background("SEQ 150 200 150 200 2300")  # tap, tap, press and hold
    else:
        print("Follow the display: release the button, wait for the bar to drain, then tap, tap, and press again and hold for 2 s.")
    deadline = time.time() + wait
    try:
        answered = 0
        heard = False
        while time.time() < deadline:
            # a wrong PIN starts the application, which says nothing at this baud rate: the update mode
            # opens about 7 s after the PIN, so silence for 12 s means it did not
            if answered and time.time() > answered + 12:
                args.verdict = time.time()
                raise WrongPin()
            port.reset_input_buffer()
            port.write(b"\x30\x20")
            reply = port.read(2)
            heard = heard or bool(reply)
            if reply == bytes([INSYNC, OK]):
                if pressing:
                    pressing.join()
                args.verdict = time.time()
                return port
            if b"?" in reply and getattr(args, "pin", None) is not None:  # the device asks for its PIN
                port.write(b"\x02")
                time.sleep(0.2)  # the device only listens between its prompts: the UART holds just 3 bytes
                port.write(args.pin.encode() + b"\n")
                port.flush()
                args.pin_sent = answered = time.time()  # the tests want to know how long the device takes to decide
                time.sleep(5)  # it decides after about 7 s; polls would only pile up
                args.pin = None
                continue
            time.sleep(0.2)
        if pressing:
            pressing.join()
        if not heard:
            raise GateSilent("the bootloader did not open (gate not passed?)")
        raise SystemExit("the bootloader did not open (gate not passed?)")
    except BaseException:
        port.close()  # a leaked handle on the port misleads the tests that follow
        raise


def main():
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("hexfile")
    parser.add_argument("--port", default="/dev/ttyUSB0")
    parser.add_argument("--rig")
    parser.add_argument("--baud", type=int, default=115200)
    parser.add_argument("--wait", type=float, default=60, help="seconds to wait for the gate")
    parser.add_argument("--pin", help="the device's update PIN, if one is set")
    parser.add_argument("--set-pin", help="write this update PIN with the flash")
    parser.add_argument("--clear-pin", action="store_true", help="remove the update PIN with the flash")
    args = parser.parse_args()

    pages = pages_of(read_hex(args.hexfile))
    if args.set_pin or args.clear_pin:
        page = bytearray([0xFF] * PAGE)
        page[: len(args.set_pin or "")] = (args.set_pin or "").encode()
        pages.append((PIN_PAGE, page))
    print(f"{len(pages)} pages, {sum(len(p) for _, p in pages)} bytes")
    port = open_gate_and_sync(args)
    port.timeout = 2
    programmer = Programmer(port)
    programmer.exchange([0x50, 0x20], 2)
    for address, data in pages:
        programmer.write_page(address, data)
    programmer.exchange([0x51, 0x20], 2)
    print("done, the device restarts into the new firmware")


if __name__ == "__main__":
    main()
