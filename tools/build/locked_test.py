#!/usr/bin/env python3
"""Tests the finished-device state on the rig: the lock bits, and that the firmware reports them right.

  tools/build/locked_test.py [--rig PORT] [--device PORT]      (both found by themselves without arguments)

Needs a device that is NOT locked and holds no keys (rig.py provision). It then
  - checks that the firmware calls the unlocked device unlocked (HELLO ends in "unlocked"),
  - sets the final lock bits (tools/user/final_lock.sh, 0xCC) and checks that the lock byte reads 0xCC,
    that the firmware no longer says "unlocked" and that trustctl prints no warning,
  - checks that ISP can no longer read the flash,
  - flashes the firmware through the bootloader gate on the locked chip (the real way to update a finished device)
    and checks that it still reports itself locked,
  - unlocks it again (a chip erase) and provisions it again, so the device is left as it was.
"""
import argparse
import os
import subprocess
import sys
import tempfile
import time

import serial

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import rig as rig_module  # noqa: E402

ROOT = rig_module.ROOT
parser = argparse.ArgumentParser()
parser.add_argument("--rig")
parser.add_argument("--device")
parser.add_argument("--core", help="provision again with this firmware hex (default: the embedded core.hex)")
parser.add_argument("--bootloader", help="provision again with this bootloader hex (default: build one)")
parser.add_argument("--bin", default=os.path.join(ROOT, "trustctl", "bin", "trustctl"))
args = parser.parse_args()
failures = 0


def check(ok, what, detail=""):
    global failures
    failures += not ok
    print(f"{'PASS' if ok else 'FAIL'}  {what}" + (f"   [{detail}]" if detail and not ok else ""), flush=True)


rig_module.require_tools(rig=True, build_bootloader=not args.bootloader)  # before anything is touched
rig = rig_module.Rig(args.rig) if args.rig else rig_module.find_rig()
device = args.device or rig_module.find_device(rig)


def hello():
    """The device's HELLO line after a reset by the rig."""
    port = serial.Serial(device, 9600, timeout=1)
    time.sleep(0.3)
    rig.command("RESET 50")
    time.sleep(2.5)
    port.reset_input_buffer()
    port.write(b"HELLO\n")
    line = port.readline().decode(errors="replace").strip()
    port.close()
    return line


def isp(*command):
    """Runs a script or avrdude through the rig's ISP; the rig's port must be free for it."""
    global rig
    rig.close()
    time.sleep(0.5)
    try:
        return subprocess.run(command, capture_output=True, text=True, timeout=300)
    finally:
        rig = rig_module.Rig(rig.path)


def lock_byte():
    r = isp("avrdude", "-c", "stk500v1", "-b", "19200", "-P", rig.path, "-p", "m328p", "-U", "lock:r:-:h")
    return r.stdout.strip().lower() if r.returncode == 0 else f"avrdude failed: {r.stderr[-200:]}"


try:
    line = hello()
    check(line.startswith("TrustIssue") and line.endswith("unlocked"), "an open device says unlocked in HELLO", line)
    check(lock_byte() == "0xcf", "the lock byte reads 0xcf before")

    r = isp(os.path.join(ROOT, "tools", "user", "final_lock.sh"), rig.path, "--yes")
    check(r.returncode == 0, "final_lock.sh sets the lock bits", r.stdout[-200:] + r.stderr[-200:])
    check(lock_byte() == "0xcc", "the lock byte reads 0xcc after")
    line = hello()
    check(line.startswith("TrustIssue") and "unlocked" not in line, "a finished device no longer says unlocked", line)
    t = subprocess.run([args.bin, "--no-daemon", "--device", device, "hello"], capture_output=True, text=True, timeout=60)
    check(t.returncode == 0 and "not locked" not in t.stdout + t.stderr and "unlocked" not in t.stdout, "trustctl hello prints no unlocked warning", t.stdout + t.stderr)

    with tempfile.TemporaryDirectory() as tmp:
        dump = os.path.join(tmp, "flash.bin")
        isp("avrdude", "-c", "stk500v1", "-b", "19200", "-P", rig.path, "-p", "m328p", "-U", f"flash:r:{dump}:r")
        content = open(dump, "rb").read() if os.path.exists(dump) else b""
        check(set(content) <= {0x00, 0xFF}, "ISP reads no flash content from a locked chip", f"{len(content)} bytes, {len(set(content))} different values")

    rig.close()
    time.sleep(0.5)
    try:
        u = subprocess.run([sys.executable, os.path.join(ROOT, "tools", "build", "flash.py"),
                            os.path.join(ROOT, "trustctl", "internal", "firmware", "core.hex"),
                            "--port", device, "--rig", rig.path], capture_output=True, text=True, timeout=300)
    finally:
        rig = rig_module.Rig(rig.path)
    check(u.returncode == 0 and "done" in u.stdout, "the bootloader gate still opens on a locked chip and flashes the firmware", u.stdout[-200:] + u.stderr[-200:])
    time.sleep(1)
    line = hello()
    check(line.startswith("TrustIssue") and "unlocked" not in line, "after a gate update the device is still locked and answers", line)
finally:
    r = isp(os.path.join(ROOT, "tools", "user", "unlock.sh"), rig.path, "--yes")
    check(r.returncode == 0 and "0xff" in r.stdout.lower(), "unlock.sh erases the chip and the lock reads 0xff again", r.stdout[-300:] + r.stderr[-200:])
    rig = rig_module.provision(rig, os.path.abspath(args.core) if args.core else None, os.path.abspath(args.bootloader) if args.bootloader else None)
    line = hello()
    check(line.startswith("TrustIssue") and line.endswith("unlocked"), "provisioned again: the device is open and answers", line)

print("ALL PASSED" if not failures else f"{failures} FAILED")
sys.exit(1 if failures else 0)
