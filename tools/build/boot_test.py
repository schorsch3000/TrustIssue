#!/usr/bin/env python3
"""Security tests of the button-gated bootloader, with the Uno rig as the finger.

  tools/build/boot_test.py [nano_port] [uno_port] [--pin]

--pin adds the update PIN tests (about 2 minutes): it writes the PIN 4711 to the device, tries a wrong and
the right one, and removes it again. Without it the device must have no PIN set.

Needs the bootloader on the Nano (firmware/bootloader) with the core firmware flashed.
"""
import argparse
import os
import sys
import time

import serial

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import flash  # noqa: E402

PIN_TESTS = "--pin" in sys.argv  # also test the update PIN: sets it on the device and clears it again
positional = [a for a in sys.argv[1:] if not a.startswith("--")]
NANO = positional[0] if len(positional) > 0 else "/dev/ttyUSB0"
UNO = positional[1] if len(positional) > 1 else "/dev/ttyUSB1"
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

port.write(bytes([0x55, 0x00, 0x00, 0x20]))
port.read(2)
eeprom_write = bytes([0x64, 0x00, 0x04, ord("E"), 1, 2, 3, 4, 10, 0x20])  # with the sum byte
port.write(eeprom_write)
reply = port.read(2)
check(reply == bytes([flash.INSYNC, 0x11]), f"EEPROM write is refused (reply {reply.hex()})")

boot_word_address = 0x7800 >> 1
port.write(bytes([0x55, boot_word_address & 0xFF, boot_word_address >> 8, 0x20]))
port.read(2)
port.write(bytes([0x64, 0x00, 0x80, ord("F")] + [0] * 128 + [0, 0x20]))
reply = port.read(2)
check(reply == bytes([flash.INSYNC, 0x11]), f"writing the bootloader's own section is refused (reply {reply.hex()})")

port.write(bytes([0x55, 0x00, 0x00, 0x20]))
port.read(2)
bad_sum = bytes([0x64, 0x00, 0x80, ord("F")] + [0x11] * 128 + [0x99, 0x20])
port.write(bad_sum)
reply = port.read(2)
check(reply == bytes([flash.INSYNC, 0x11]), f"a page with a wrong checksum is refused (reply {reply.hex()})")

# there is no read-back at all (the PIN lives in flash): the old READ_PAGE gets no flash data
port.reset_input_buffer()
port.write(bytes([0x74, 0x00, 0x80, ord("F"), 0x20]))
time.sleep(1)
leak = port.read(port.in_waiting or 1)
# (what comes back, if anything, is the restarted application's banner heard at the wrong baud rate)
check(not (len(leak) >= 130 and leak[0] == flash.INSYNC and leak[129] == flash.OK),
      f"READ_PAGE returns no page of flash data ({len(leak)} bytes came back)")

time.sleep(9)  # no traffic (the unknown command probably reset the chip already): the update mode ends after 8 s

nano = serial.Serial(NANO, 9600, timeout=0.3)
time.sleep(3)
print("-- after all that")
check(app_runs(nano), "the device restarted into its application (the firmware was not damaged)")
LAST = []


def application_starts_within(port, seconds):
    """Watches a port (switched to the application's 9600 baud) for the first HELLO answer."""
    port.baudrate = 9600
    end = time.time() + seconds
    while time.time() < end:
        port.reset_input_buffer()
        port.write(b"HELLO\n")
        if port.readline().startswith(b"TrustIssue"):
            return True
        time.sleep(0.2)
    return False


def gate(pin=None, watch=None):
    """Opens the update mode through the gesture (and the PIN); returns (args, port) or raises WrongPin."""
    a = argparse.Namespace(port=NANO, baud=115200, wait=60, rig=None, pin=pin, watch=watch)
    LAST.append(a)  # the timing is read from it even when the gate raises
    return a, flash.open_gate_and_sync(a, rig)


def write_pin_page(port, text):
    page = bytearray([0xFF] * flash.PAGE)
    page[: len(text)] = text.encode()
    port.timeout = 2
    pg = flash.Programmer(port)
    pg.exchange([0x50, 0x20], 2)
    pg.write_page(flash.PIN_PAGE, page)
    pg.exchange([0x51, 0x20], 2)


if PIN_TESTS:
    print("-- the update PIN")
    nano.close()
    try:
        _, port = gate()
        write_pin_page(port, "4711")
        port.close()
        time.sleep(3)
        check(True, "a PIN was written through the gate")

        try:
            a, port = gate(pin="0000", watch=lambda p: application_starts_within(p, 15))
            check(False, "a wrong PIN must not open the update mode")
            port.close()
        except flash.WrongPin:
            check(True, "a wrong PIN starts the application instead")
        a = LAST[-1]
        decided = a.verdict - a.pin_sent
        check(decided >= 6, f"the wrong PIN is only noticed after the bar ({decided:.1f} s after sending it)")
        nano = serial.Serial(NANO, 9600, timeout=0.3)
        check(app_runs_within(nano, 10), "the application runs after a wrong PIN")
        nano.close()

        a, port = gate(pin="4711")
        decided = a.verdict - a.pin_sent
        check(decided >= 6, f"the right PIN opens the update mode, also only after the bar ({decided:.1f} s)")
        write_pin_page(port, "")  # remove the PIN again (needs the open update mode)
        port.close()
        time.sleep(3)
        _, port = gate()
        check(True, "after removing it the gesture alone opens the update mode again")
        port.timeout = 2
        flash.Programmer(port).exchange([0x51, 0x20], 2)
        port.close()
        time.sleep(3)
    except BaseException:
        print("PIN test aborted: trying to remove the PIN again (4711) so the device is left as it was", flush=True)
        try:
            a, port = gate(pin="4711")
            write_pin_page(port, "")
            port.close()
        except BaseException as error:  # noqa: BLE001
            print(f"could not remove it: {error!r}; flash a blank PIN page by hand: flash.py FILE --pin 4711 --clear-pin")
        failures += 1
        raise


print("ALL PASSED" if not failures else f"{failures} FAILED")
sys.exit(1 if failures else 0)
