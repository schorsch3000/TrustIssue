#!/usr/bin/env python3
"""End-to-end test of `trustctl flash` and `trustctl calibrate` on the real device.

  tools/build/trustctl_flash_e2e.py [--bin trustctl/bin/trustctl] [--device /dev/ttyUSB0] [--rig /dev/ttyUSB1]

It flashes the calibration firmware, tries contrast, logo and font on it, then
flashes the core firmware with those settings patched in. The Uno rig performs
the button gesture of the bootloader. Needs firmware/calibration/build and
firmware/core/build (see build.sh) and no running trustctl daemon (it stops it).
"""
import argparse
import os
import subprocess
import sys
import tempfile
import threading
import time

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import flash  # noqa: E402

parser = argparse.ArgumentParser()
parser.add_argument("--bin", default="trustctl/bin/trustctl")
parser.add_argument("--device", default="/dev/ttyUSB0")
parser.add_argument("--rig", default="/dev/ttyUSB1")
parser.add_argument("--calibration-hex", default="calibration")
parser.add_argument("--core-hex", default="core")
parser.add_argument("--font", help="with --flash-only: flash this font too (and let the profile remember it)")
parser.add_argument("--flash-only", action="store_true", help="only flash the core firmware, patched with ~/.config/trustctl/device.json")
args = parser.parse_args()
failures = 0
rig = flash.Rig(args.rig)
profile = os.path.join(tempfile.mkdtemp(), "device.json")


def check(ok, what, detail=""):
    global failures
    failures += not ok
    print(f"{'PASS' if ok else 'FAIL'}  {what}" + (f"   [{detail}]" if detail and not ok else ""), flush=True)


def run(*cmd, gate=False, stdin=None):
    """Runs trustctl. With gate=True the rig does the bootloader gesture when trustctl asks for it."""
    proc = subprocess.Popen([args.bin, "--device", args.device, *cmd], stdin=subprocess.PIPE,
                            stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
    lines = []

    def watch():
        pending = None
        for line in proc.stderr:
            lines.append(line.rstrip())
            if gate and line.startswith("HOLD THE BUTTON"):
                rig.command("PRESS")
            elif gate and line.startswith("Resetting the device now"):
                time.sleep(0.6)
                rig.command("RELEASE")
                time.sleep(5.9)  # the bar drains for about 4.3 s
                pending = rig.command_in_background("SEQ 150 200 150 200 2300")
        if pending:
            pending.join()

    outputs = []
    watcher = threading.Thread(target=watch)
    reader = threading.Thread(target=lambda: outputs.append(proc.stdout.read()))
    watcher.start()
    reader.start()
    if stdin:
        proc.stdin.write(stdin)
    proc.stdin.close()
    proc.wait(timeout=240)
    watcher.join()
    reader.join()
    return proc.returncode, outputs[0].strip(), lines


run("daemon", "stop")

if "--flash-only" in sys.argv:  # just flash the core firmware with the real profile
    code, out, err = run("flash", args.core_hex, "--countdown", "1", *(["--font", args.font] if args.font else []), gate=True)
    print(out or err[-3:])
    sys.exit(code)

print("-- flash the calibration firmware")
code, out, err = run("flash", args.calibration_hex, "--no-patch", "--countdown", "1", gate=True)
check(code == 0 and "Done" in out, "flash calibration firmware through the gate", f"{code} {out} {err[-3:]}")

print("-- calibrate (look at the display)")
code, out, err = run("calibrate", "contrast", "full", "0x20", "--profile", profile)
check(code == 0, "contrast full", f"{code} {err}")
code, out, err = run("calibrate", "contrast", "dim", "0", "--profile", profile)
check(code == 0, "contrast dim", f"{code} {err}")
code, out, err = run("calibrate", "contrast", "full", "300", "--profile", profile)
check(code != 0, "a contrast above 255 is refused", f"{code} {err}")
code, out, err = run("calibrate", "logo", "Demo-Logos/trust_issue_01.png", "--profile", profile)
check(code == 0, "logo preview", f"{code} {err}")
time.sleep(3)
code, out, err = run("calibrate", "font", "firmware/fonts/victoriamedium8.bin", "--profile", profile)
check(code == 0, "font preview", f"{code} {err}")
time.sleep(3)
code, out, err = run("font", "list")
names = [l.split()[0] for l in out.splitlines()]
check(len(names) >= 7 and "victoriamedium8" in names, "font list", out)
for name in names:
    code, out, err = run("calibrate", "font", name, "--profile", profile)
    check(code == 0, f"built-in font {name} on the display", f"{code} {err}")
    time.sleep(1.5)
code, out, err = run("calibrate", "font", "nonsense", "--profile", profile)
check(code != 0, "an unknown font name is refused", f"{code}")
code, out, err = run("calibrate", "font", "README.md", "--profile", profile)
check(code != 0, "a file that is no font is refused", f"{code}")
code, out, err = run("calibrate", "show", "--profile", profile)
check("contrast full 32" in out and "dim 0" in out and "logo " in out and "font builtin:" in out, "the profile remembers it all", out)

print("-- flash the core firmware with the profile patched in")
code, out, err = run("flash", args.core_hex, "--profile", profile, "--font", "5x8", "--countdown", "1", gate=True)
check(code == 0 and "Done" in out and any("Patched from" in l for l in err), "flash core firmware with the patch", f"{code} {out} {err[-4:]}")
time.sleep(1.5)
code, out, err = run("hello")
check(code == 0 and out.startswith("TrustIssue 0."), "the application runs after flashing", f"{code} {out} {err}")
code, out, err = run("daemon", "stop")

print("ALL PASSED" if not failures else f"{failures} FAILED")
sys.exit(1 if failures else 0)
