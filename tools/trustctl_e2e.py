#!/usr/bin/env python3
"""End-to-end test of trustctl against the real device, with the Uno rig as the finger.

  tools/trustctl_e2e.py [--bin trustctl/bin/trustctl] [--device /dev/ttyUSB0] [--rig /dev/ttyUSB1]

The device must run firmware/core (with the bootloader) and hold no keys.
"""
import argparse
import hashlib
import hmac
import os
import queue
import struct
import subprocess
import sys
import threading
import time

import serial

parser = argparse.ArgumentParser()
parser.add_argument("--bin", default="trustctl/bin/trustctl")
parser.add_argument("--device", default="/dev/ttyUSB0")
parser.add_argument("--rig", default="/dev/ttyUSB1")
args = parser.parse_args()

SECRET_B32 = "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"
SECRET = b"12345678901234567890"
failures = 0

rig = serial.Serial(args.rig, 115200, timeout=30)
time.sleep(2.5)
rig.reset_input_buffer()


def press(ms):
    rig.write(f"SEQ {ms}\n".encode())
    rig.readline()


def trustctl(*cmd, stdin=None, holds=()):
    """Runs trustctl; presses the button (hold times from holds, default 1500 ms) at each confirmation request."""
    proc = subprocess.Popen([args.bin, "--device", args.device, *cmd], stdin=subprocess.PIPE,
                            stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
    pending = list(holds)
    errors = []
    prompts = 0

    def watch():
        nonlocal prompts
        for line in proc.stderr:
            errors.append(line.rstrip())
            if line.startswith("Confirm on the device"):
                prompts += 1
                time.sleep(0.3)  # let the display come up
                press(pending.pop(0) if pending else 1500)

    # stderr is read by the watcher thread alone (communicate() would race it)
    outputs = []
    watcher = threading.Thread(target=watch)
    reader = threading.Thread(target=lambda: outputs.append(proc.stdout.read()))
    watcher.start()
    reader.start()
    if stdin:
        proc.stdin.write(stdin)
    proc.stdin.close()
    proc.wait(timeout=120)
    watcher.join()
    reader.join()
    return proc.returncode, outputs[0].strip(), errors, prompts


def check(ok, what, detail=""):
    global failures
    failures += not ok
    print(f"{'PASS' if ok else 'FAIL'}  {what}" + (f"   [{detail}]" if detail and not ok else ""), flush=True)


def totp(unix, digits=6, period=30):
    mac = hmac.new(SECRET, struct.pack(">Q", unix // period), hashlib.sha1).digest()
    offset = mac[19] & 15
    return str((struct.unpack(">I", mac[offset:offset + 4])[0] & 0x7FFFFFFF) % 10 ** digits).zfill(digits)


trustctl("daemon", "stop")  # start from a clean state: the first command starts a new daemon
code, out, err, prompts = trustctl("list")
check(code == 0 and out == "", "list on an empty device", f"{code} {out!r} {err}")

code, out, err, prompts = trustctl("add", "E2E Test", stdin=SECRET_B32 + "\n")
check(code == 0 and "slot 0" in out and prompts == 1, "add confirms once and reports slot 0", f"{code} {out!r} {err}")

code, out, err, prompts = trustctl("add", "e2e test", stdin=SECRET_B32 + "\n")
check(code == 1 and "already exists" in "\n".join(err) and prompts == 0, "a duplicate name is refused without a prompt", f"{code} {err}")

code, out, err, prompts = trustctl("add", "Bad_Name", stdin=SECRET_B32 + "\n")
check(code == 1 and prompts == 0, "a bad name is refused before the device is bothered", f"{code} {err}")

code, out, err, prompts = trustctl("list")
check(out.split() == ["0", "E2E", "Test"], "list shows the key", out)

code, out, err, prompts = trustctl("tz")
check(code == 0 and "device: UTC+0" in out, "tz shows the device offset", out)

t0 = int(time.time())
code, out, err, prompts = trustctl("token", "e2e")
t1 = int(time.time())
expected = {totp(t) for t in range(t0 - 1, t1 + 10)}
check(code == 0 and out in expected and prompts == 2, "token: offset sync confirmed, then the code (matches the host's TOTP)", f"{code} {out!r} {sorted(expected)} prompts={prompts} {err}")

code, out, err, prompts = trustctl("tz")
check("device:" in out and out.split("\n")[0].split()[1] == out.split("\n")[1].split()[1], "device and host offsets now agree", out)

t0 = int(time.time())
code, out, err, prompts = trustctl("token", "E2E Test")
t1 = int(time.time())
check(code == 0 and out in {totp(t) for t in range(t0 - 1, t1 + 10)} and prompts == 1, "token again: one confirmation only", f"{code} {out!r} prompts={prompts}")

code, out, err, prompts = trustctl("token", "e2e", holds=[100])
check(code == 2 and out == "" and prompts == 1, "a tap denies: exit code 2, no code printed", f"{code} {out!r} {err}")

code, out, err, prompts = trustctl("token", "nope")
check(code == 1 and prompts == 0, "an unknown key", f"{code} {err}")

code, out, err, prompts = trustctl("config", "set", "invert=on")
code, out, err, prompts = trustctl("config")
check("invert    on" in out and "rotate    off" in out, "config set and get", out)
trustctl("config", "set", "invert=off")

code, out, err, prompts = trustctl("del", "e2e")
check(code == 0 and prompts == 1, "del confirms once", f"{code} {err}")
code, out, err, prompts = trustctl("list")
check(out == "", "list is empty again", out)

trustctl("add", "Wipe Me", stdin=SECRET_B32 + "\n")
code, out, err, prompts = trustctl("wipe", holds=[1500])
check(code == 2, "wipe with a normal hold is denied", f"{code} {err}")
code, out, err, prompts = trustctl("wipe", holds=[5500])
check(code == 0, "wipe with a 5.5 s hold works", f"{code} {err}")
code, out, err, prompts = trustctl("list")
check(out == "", "no keys after the wipe", out)

trustctl("tz", "set", "+0")
code, out, err, prompts = trustctl("daemon", "status")
check(out.startswith("OK device=" + args.device), "the daemon is running and holds the device", out)
trustctl("daemon", "stop")
code, out, err, prompts = trustctl("daemon", "status")
check("no daemon" in out, "daemon stop works", out)
print("ALL PASSED" if not failures else f"{failures} FAILED")
sys.exit(1 if failures else 0)
