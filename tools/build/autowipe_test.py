#!/usr/bin/env python3
"""Tests the autonomous wipe (no host involved) with the Uno rig as the finger.

  tools/build/autowipe_test.py [nano_port] [uno_port]

The Nano must run firmware/core built with the autonomous wipe (the default of build.sh)
and hold no keys. The test adds keys over serial, and wipes them with the button only.
"""
import os
import sys
import time

import serial

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import flash  # noqa: E402

NANO = sys.argv[1] if len(sys.argv) > 1 else "/dev/ttyUSB0"
UNO = sys.argv[2] if len(sys.argv) > 2 else "/dev/ttyUSB1"
SECRET = "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"
failures = 0


def check(ok, what, detail=""):
    global failures
    failures += not ok
    print(f"{'PASS' if ok else 'FAIL'}  {what}" + (f"   [{detail}]" if detail and not ok else ""), flush=True)


rig = flash.Rig(UNO)
nano = serial.Serial(NANO, 9600, timeout=3)
time.sleep(1.0)
nano.reset_input_buffer()


def line():
    return nano.readline().decode(errors="replace").strip()


def ask(text):
    nano.write((text + "\n").encode())
    return line()


def confirmed(text, hold=1500):
    nano.write((text + "\n").encode())
    first = line()
    if first != "PENDING":
        return first
    rig.command(f"SEQ {hold}")
    return line()


def keys():
    nano.write(b"LIST\n")
    out = []
    while True:
        l = line()
        if l == "END" or not l:
            return out
        out.append(l)


check("caps=autowipe" in ask("HELLO"), "the firmware says it has the autonomous wipe")
check(keys() == [], "start with no keys")
check(confirmed(f"ADD {SECRET} SHA1 6 30 Alpha") == "OK 0", "add a key")
check(confirmed(f"ADD {SECRET} SHA1 6 30 Beta") == "OK 1", "add another key")

rig.command("SEQ 10500")
time.sleep(0.5)
check(len(keys()) == 2, "switched off (the default): a 10.5 s hold does nothing")

check(confirmed("SETCFG 08") == "OK", "switch the autonomous wipe on")
check(ask("GETCFG") == "OK 08", "the flag is set")

rig.command("SEQ 9000")
rig.command("SEQ 12000")
rig.command("SEQ 1500")
time.sleep(0.5)
check(len(keys()) == 2, "holds of 9 s, 12 s and 1.5 s do nothing")

rig.command("SEQ 10500")  # right: the flow starts when the button is released
time.sleep(0.3)  # the release needs its debounce time before the device reacts
check(ask("LIST") == "ERR BUSY", "during the flow the device is busy", ask("LIST"))
time.sleep(1.0)
rig.command("SEQ 300")  # a press in the countdown aborts
time.sleep(2.0)
check(len(keys()) == 2, "a press during the countdown cancels", str(keys()))

rig.command("SEQ 10500")
time.sleep(4.6)  # the countdown (4 s) is over
rig.command("SEQ 150 200 150")  # tap, tap and stop
time.sleep(1.2)  # the gap of 0.8 s is over: the gesture starts again
rig.command("SEQ 150 200 150 200 900")  # a hold that is too short
time.sleep(1.2)
check(ask("LIST") == "ERR BUSY", "wrong gestures wipe nothing: the flow still waits for the right one")
rig.command("SEQ 150 200 150 200 2400")  # the right gesture
time.sleep(3.0)
check(keys() == [], "the right gesture wipes all keys", str(keys()))
check(ask("GETCFG") == "OK 08", "the configuration survives the wipe")

rig.command("SEQ 10500")
time.sleep(0.5)
check(ask("HELLO").startswith("TrustIssue"), "with no keys, the long hold starts nothing (the device answers)")
check(confirmed("SETCFG 00") == "OK", "switch it off again")
print("ALL PASSED" if not failures else f"{failures} FAILED")
sys.exit(1 if failures else 0)
