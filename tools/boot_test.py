#!/usr/bin/env python3
"""Security tests of the button-gated bootloader, with the Uno rig as the finger.

  tools/boot_test.py [nano_port] [uno_port]

Needs the bootloader on the Nano (firmware/bootloader) with the core firmware flashed.
"""
import argparse
import os
import sys
import time

import serial

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import flash  # noqa: E402

NANO = sys.argv[1] if len(sys.argv) > 1 else "/dev/ttyUSB0"
UNO = sys.argv[2] if len(sys.argv) > 2 else "/dev/ttyUSB1"
failures = 0


def check(ok, what):
    global failures
    failures += not ok
    print(f"{'PASS' if ok else 'FAIL'}  {what}", flush=True)


def bootloader_answers(port, seconds):
    """True if something answers STK500 GET_SYNC at 115200 baud within the time."""
    port.baudrate = 115200
    end = time.time() + seconds
    while time.time() < end:
        port.reset_input_buffer()
        port.write(b"\x30\x20")
        if port.read(2) == bytes([flash.INSYNC, flash.OK]):
            return True
        time.sleep(0.2)
    return False


def app_runs(port):
    port.baudrate = 9600
    for _ in range(3):
        port.reset_input_buffer()
        port.write(b"HELLO\n")
        if port.readline().decode(errors="replace").startswith("TrustIssue"):
            return True
    return False


def app_runs_within(port, seconds):
    end = time.time() + seconds
    while time.time() < end:
        if app_runs(port):
            return True
    return False


rig = flash.Rig(UNO)
nano = serial.Serial(NANO, 9600, timeout=0.3)  # opening resets the Nano
time.sleep(3)

print("-- normal operation")
check(app_runs(nano), "the application runs after a reset without the button")

rig.command("RESET 50")
time.sleep(0.3)
check(not bootloader_answers(nano, 5), "a host that resets the chip and speaks STK500 gets no answer")
check(app_runs(nano), "the application is still running afterwards")

print("-- bait and switch: the button is held while the chip is reset")
rig.command("RESETHOLD 0 3000")  # press, reset, keep holding 3 s, release
check(not bootloader_answers(nano, 22), "no update mode after a held reset, even after the release (gate times out)")
time.sleep(1)
check(app_runs(nano), "the gate timed out into the application")

print("-- gate mistakes")
rig.command("RESETHOLD 0 500")
time.sleep(2.0)
rig.command("SEQ 300")  # a press during the countdown aborts
time.sleep(1.0)
check(not bootloader_answers(nano, 3), "pressing during the countdown does not open the update mode")
# the press makes the gate return and the chip reset; if the finger is still down
# the new start is a fresh gate (RELEASE BUTTON, countdown, ...), which times out
check(app_runs_within(nano, 40), "after a press in the countdown the device ends up in the application")

rig.command("RESETHOLD 0 500")
time.sleep(5.3)
rig.command("SEQ 150 200 150")  # tap, tap: no hold
check(not bootloader_answers(nano, 3), "tap, tap without the hold does not open it")
rig.command("SEQ 150 200 150 200 900")  # tap, tap and a too short hold
check(not bootloader_answers(nano, 3), "tap, tap and a hold of 0.9 s does not open it")
rig.command("SEQ 150 200 2300")  # only one tap before the hold
check(not bootloader_answers(nano, 3), "tap and a hold (one tap too few) does not open it")
rig.command("SEQ 1500")  # a long first press is no tap
check(not bootloader_answers(nano, 3), "a long first press does not open it")

print("-- the correct gesture, then what the update mode refuses")
nano.close()
args = argparse.Namespace(port=NANO, baud=115200, wait=30, rig=None)
port = flash.open_gate_and_sync(args, rig)
port.timeout = 2
pg = flash.Programmer(port)
pg.exchange([0x50, 0x20], 2)

port.reset_input_buffer()
check(len(pg.read_page(0, 16)) == 16, "flash can be read (for verification)")

port.write(bytes([0x55, 0x00, 0x00, 0x20]))
port.read(2)
port.write(bytes([0x74, 0x00, 0x10, ord("E"), 0x20]))
reply = port.read(2)
check(reply == bytes([flash.INSYNC, 0x11]), f"EEPROM read is refused (reply {reply.hex()})")

port.write(bytes([0x55, 0x00, 0x00, 0x20]))
port.read(2)
port.write(bytes([0x64, 0x00, 0x04, ord("E"), 1, 2, 3, 4, 0x20]))
reply = port.read(2)
check(reply == bytes([flash.INSYNC, 0x11]), f"EEPROM write is refused (reply {reply.hex()})")

boot_word_address = 0x7800 >> 1
port.write(bytes([0x55, boot_word_address & 0xFF, boot_word_address >> 8, 0x20]))
port.read(2)
port.write(bytes([0x64, 0x00, 0x80, ord("F")] + [0] * 128 + [0x20]))
reply = port.read(2)
check(reply == bytes([flash.INSYNC, 0x11]), f"writing the bootloader's own section is refused (reply {reply.hex()})")

time.sleep(9)  # no traffic: the update mode ends after 8 s and the chip restarts

nano = serial.Serial(NANO, 9600, timeout=0.3)
time.sleep(3)
print("-- after all that")
check(app_runs(nano), "the device restarted into its application (the firmware was not damaged)")
print("ALL PASSED" if not failures else f"{failures} FAILED")
sys.exit(1 if failures else 0)
