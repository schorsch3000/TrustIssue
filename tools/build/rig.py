#!/usr/bin/env python3
"""The test rig: a Nano running firmware/rigisp (ArduinoISP plus the button finger) wired to the device under test.

As a module (used by the tests and by make_release.py):

    import rig
    r = rig.find_rig()                  # looks at every serial port, returns a Rig or raises RigNotFound
    r.command("SEQ 1500")               # press the device's button for 1.5 s
    port = rig.find_device(r)           # the serial port of the device wired to this rig
    rig.provision(r, core_hex)          # chip erase, bootloader, firmware, fuses over ISP

On the command line:

    tools/build/rig.py find                       which port is the rig (read-only) and which is the device
    tools/build/rig.py provision [--core HEX]     put the bootloader and the firmware on the device over ISP
    tools/build/rig.py press MS | reset | hello

How the ports are found: the rig answers "!HELLO" at 19200 baud with "TrustIssue-RigISP". The device is the port
that prints a boot banner when the rig pulls its reset line: a device that is not wired to the rig (a production
device on the same machine, say) is never picked, and nothing but its serial port is opened.
"""
import argparse
import glob
import shutil
import os
import subprocess
import sys
import tempfile
import termios
import threading
import time

import serial

ROOT = os.path.normpath(os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", ".."))
BAUD = 19200
BANNER = "TrustIssue-RigISP"
WANTED_VERSION = "0.2"


class RigNotFound(Exception):
    pass


# what each tool is called on Debian, for the hint
APT = {"avrdude": "avrdude", "make": "make", "git": "git", "go": "golang-go", "gofmt": "golang-go", "shellcheck": "shellcheck",
       "g++": "g++", "arduino-cli": "(arduino-cli, see https://arduino.github.io/arduino-cli)", "sed": "sed", "tr": "coreutils",
       "tail": "coreutils", "sh": "dash", "scp": "openssh-client", "gh": "gh"}


def find_gh():
    """The gh to use: TRUSTISSUE_GH, else ~/.local/bin/gh (where a newer one goes when the distribution's is too old), else the PATH."""
    local = os.path.expanduser("~/.local/bin/gh")
    return os.environ.get("TRUSTISSUE_GH") or (local if os.access(local, os.X_OK) else None) or shutil.which("gh")


def find_avr_gcc():
    """avr-gcc from the PATH or from the Arduino AVR core (what the bootloader Makefile uses)."""
    return shutil.which("avr-gcc") or next(iter(glob.glob(os.path.expanduser("~/.arduino15/packages/arduino/tools/avr-gcc/*/bin/avr-gcc"))), None)


def missing_tools(rig=True, build_bootloader=False, host=False, git=False, scp=False, gh=False):
    """Everything the chosen jobs call that is not there, as readable lines. Empty when all is in place."""
    tools = []
    if rig:
        tools += ["avrdude", "sh", "sed", "tr", "tail"]
    if git or host:
        tools += ["git"]
    if scp:
        tools += ["scp"]
    if build_bootloader or host:
        tools += ["make"]
    if host:
        tools += ["go", "gofmt", "shellcheck", "g++", "arduino-cli"]
    problems = [f"{t} (Debian: sudo apt install {APT[t]})" for t in dict.fromkeys(tools) if not shutil.which(t)]
    if gh:
        path = find_gh()
        if not path:
            problems.append("gh (Debian: sudo apt install gh; it must be 2.49 or newer for `gh attestation`)")
        elif subprocess.run([path, "attestation", "verify", "--help"], capture_output=True).returncode != 0:
            problems.append(f"{path} is too old: it has no `gh attestation` (needs 2.49 or newer). A current one for the runner user without sudo: "
                            "download the linux_arm64 .tar.gz from https://github.com/cli/cli/releases/latest and put bin/gh into ~/.local/bin")
    if rig and not os.path.exists(os.path.join(ROOT, "tools", "user", "flash_bootloader.sh")):
        problems.append("tools/user/flash_bootloader.sh (run this from a complete checkout)")
    if (build_bootloader or host) and not find_avr_gcc():
        problems.append("avr-gcc (the Arduino AVR core: arduino-cli core install arduino:avr, or Debian: sudo apt install gcc-avr avr-libc)")
    if rig or host:
        try:
            import serial  # noqa: F401
        except ImportError:
            problems.append("the python module serial (Debian: sudo apt install python3-serial)")
    return problems


def require_tools(**what):
    """The preflight: stops before anything is touched, naming everything that is missing at once."""
    problems = missing_tools(**what)
    if problems:
        sys.exit("preflight: this is missing:\n  - " + "\n  - ".join(problems))


def serial_ports():
    return sorted(glob.glob("/dev/ttyUSB*") + glob.glob("/dev/ttyACM*"))


class Rig:
    """The rig Nano. Commands are the lines of firmware/rigisp without the '!', e.g. "SEQ 150 200 150 200 2300"."""

    def __init__(self, path, patience=15):
        self.path = path
        self.port = serial.Serial(path, BAUD, timeout=1)
        # closing the port must not drop DTR: the next open (avrdude's, say) would reset the Nano again, and an
        # ISP sync that arrives while its bootloader is still waiting is lost
        attrs = termios.tcgetattr(self.port.fileno())
        attrs[2] &= ~termios.HUPCL
        termios.tcsetattr(self.port.fileno(), termios.TCSANOW, attrs)
        # opening the port resets the Nano; its start-up takes 1 to 4 s: ask until it answers
        deadline = time.time() + patience
        self.version = None
        while time.time() < deadline:
            self.port.reset_input_buffer()
            self.port.write(b"!HELLO\n")
            reply = self.port.readline().decode(errors="replace").strip()
            if reply.startswith(BANNER):
                self.version = reply.split()[-1]
                self.port.readline()  # the OK
                break
        if self.version is None:
            self.port.close()
            raise RigNotFound(f"{path} does not answer like the rig")
        self.port.timeout = 60
        self.command("RELEASE")  # a test that was killed may have left the button pressed

    def command(self, text):
        self.port.write(("!" + text + "\n").encode())
        reply = self.port.readline().decode(errors="replace").strip()
        if reply != "OK":
            raise SystemExit(f"rig: {text} failed ({reply or 'no answer'})")

    def command_in_background(self, text):
        thread = threading.Thread(target=self.command, args=(text,))
        thread.start()
        return thread

    def close(self):
        self.port.close()


def find_rig(ports=None, patience=8):
    """Opens every serial port in parallel, asks "!HELLO" and returns the Rig that answers (exactly one may)."""
    ports = ports or serial_ports()
    answers = {}

    def probe(path):
        try:
            answers[path] = Rig(path, patience)
        except (RigNotFound, serial.SerialException, OSError):
            pass

    threads = [threading.Thread(target=probe, args=(p,)) for p in ports]
    for t in threads:
        t.start()
    for t in threads:
        t.join()
    if not answers:
        raise RigNotFound(f"no rig among {', '.join(ports) or 'no serial ports'}: is firmware/rigisp on the rig Nano?")
    if len(answers) > 1:
        for r in answers.values():
            r.close()
        raise RigNotFound(f"more than one rig: {', '.join(answers)}")
    rig = next(iter(answers.values()))
    if rig.version != WANTED_VERSION:
        print(f"warning: the rig sketch is version {rig.version}, this tool expects {WANTED_VERSION}", file=sys.stderr)
    return rig


def find_device(rig, wait=4):
    """The serial port of the device wired to the rig: the one that prints its banner when the rig resets it."""
    candidates = [p for p in serial_ports() if p != rig.path]
    found = []
    ports = {}
    for path in candidates:
        try:
            ports[path] = serial.Serial(path, 9600, timeout=0.2)
        except (serial.SerialException, OSError):
            pass
    time.sleep(2.5)  # opening a port resets some devices: let them finish booting
    for p in ports.values():
        p.reset_input_buffer()
    rig.command("RESET 50")
    deadline = time.time() + wait
    seen = {path: b"" for path in ports}
    while time.time() < deadline:
        for path, p in ports.items():
            seen[path] += p.read(200)
        if any(b"TrustIssue" in s for s in seen.values()):
            time.sleep(0.5)
            for path, p in ports.items():
                seen[path] += p.read(200)
            break
    for path, s in seen.items():
        if b"TrustIssue" in s:
            found.append(path)
    for p in ports.values():
        p.close()
    if len(found) == 1:
        return found[0]
    if not found:
        raise RigNotFound("no device prints a banner when the rig resets it (blank chip or wiring? try: rig.py provision)")
    raise RigNotFound(f"more than one device reacts to the rig's reset: {', '.join(found)}")


def merged_hex(bootloader_hex, core_hex, out):
    """Bootloader and firmware in one Intel HEX file (their addresses do not overlap)."""
    with open(out, "w") as f:
        for path in (bootloader_hex, core_hex):
            for line in open(path):
                if line.strip() and not line.startswith(":00000001FF"):
                    f.write(line if line.endswith("\n") else line + "\n")
        f.write(":00000001FF\n")


def provision(rig, core_hex=None, bootloader_hex=None):
    """Chip erase, bootloader, firmware and fuses over ISP (tools/user/flash_bootloader.sh). Wipes the EEPROM too."""
    core_hex = core_hex or os.path.join(ROOT, "trustctl", "internal", "firmware", "core.hex")
    require_tools(rig=True, build_bootloader=bootloader_hex is None)
    if bootloader_hex is None:
        subprocess.run(["make", "-B", "-C", os.path.join(ROOT, "firmware", "bootloader")], check=True, capture_output=True)
        bootloader_hex = os.path.join(ROOT, "firmware", "bootloader", "boot.hex")
    path = rig.path
    rig.close()  # avrdude needs the port
    time.sleep(0.5)
    with tempfile.TemporaryDirectory() as tmp:
        both = os.path.join(tmp, "device.hex")
        merged_hex(bootloader_hex, core_hex, both)
        subprocess.run([os.path.join(ROOT, "tools", "user", "flash_bootloader.sh"), path, "--yes", "--hex", both], check=True)
    return Rig(path)


def main():
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    sub = parser.add_subparsers(dest="action", required=True)
    sub.add_parser("find")
    sub.add_parser("hello")
    p = sub.add_parser("provision")
    p.add_argument("--core")
    p.add_argument("--bootloader")
    p = sub.add_parser("press")
    p.add_argument("ms", type=int)
    sub.add_parser("reset")
    args = parser.parse_args()
    try:
        r = find_rig()
    except RigNotFound as e:
        sys.exit(f"rig.py: {e}")
    if args.action == "find":
        print(f"rig:    {r.path} (sketch {r.version})")
        try:
            print(f"device: {find_device(r)}")
        except RigNotFound as e:
            print(f"device: not found ({e})")
    elif args.action == "hello":
        print(f"rig {r.path} sketch {r.version}")
    elif args.action == "provision":
        r = provision(r, args.core, args.bootloader)
        print(f"device: {find_device(r)}")
    elif args.action == "press":
        r.command(f"SEQ {args.ms}")
    elif args.action == "reset":
        r.command("RESET 50")


if __name__ == "__main__":
    main()
