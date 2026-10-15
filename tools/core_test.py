#!/usr/bin/env python3
"""End-to-end test of the core firmware: protocol over serial, button via the Uno rig.

  tools/core_test.py [nano_port] [uno_port] [--slow]

The Nano must run firmware/core and hold no keys (the test adds and removes its own).
--slow also runs the 30 s request timeout test.
"""
import sys
import time

import serial

ARGS = [a for a in sys.argv[1:] if not a.startswith("--")]
SLOW = "--slow" in sys.argv
NANO = ARGS[0] if len(ARGS) > 0 else "/dev/ttyUSB0"
UNO = ARGS[1] if len(ARGS) > 1 else "/dev/ttyUSB1"

# The RFC 6238 secret "12345678901234567890" in Base32. The device adds the time
# elapsed since the request (here 1-4 s) to the timestamp, so the test timestamps
# are chosen away from a 30 s boundary: t=35 is in counter 1 (code 287082) and
# t=1111111090 in counter 37037036 (code 081804).
SECRET = "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"

failures = 0


def check(ok, what, detail=""):
    global failures
    if not ok:
        failures += 1
    print(f"{'PASS' if ok else 'FAIL'}  {what}" + (f"   [{detail}]" if detail and not ok else ""))


class Device:
    def __init__(self):
        self.nano = serial.Serial(NANO, 9600, timeout=3)
        self.rig = serial.Serial(UNO, 115200, timeout=30)
        time.sleep(3.5)  # both boards reset when the ports open; the bootloader waits
        self.nano.reset_input_buffer()
        self.rig.reset_input_buffer()

    def line(self):
        return self.nano.readline().decode("ascii", "replace").strip()

    def send(self, text):
        self.nano.write((text + "\n").encode())

    def ask(self, text):
        self.send(text)
        return self.line()

    def press(self, sequence):
        self.rig.write(f"SEQ {sequence}\n".encode())
        assert self.rig.readline().decode().strip() == "OK"

    def confirmed(self, text, hold="1500"):
        """Sends a command that needs the button; returns the final reply."""
        self.send(text)
        first = self.line()
        if first != "PENDING":
            return first
        self.press(hold)
        return self.line()

    def lines_until_end(self):
        lines = []
        while True:
            l = self.line()
            if l == "END" or not l:
                return lines
            lines.append(l)


d = Device()
d.send("HELLO")
check(d.line().startswith("TrustIssue 0.1.0 proto 1"), "HELLO")
d.send("LIST")
keys = d.lines_until_end()
if keys:
    raise SystemExit(f"the device already holds keys {keys}; this test wants an empty device")

check(d.confirmed(f"ADD {SECRET} SHA1 6 30 RFC Test") == "OK 0", "ADD accepted gets slot 0")
d.send("LIST")
check(d.lines_until_end() == ["KEY 0 RFC Test"], "LIST shows the key")
check(d.ask(f"ADD {SECRET} SHA1 6 30 rfc test") == "ERR DUPLICATE", "case-insensitive duplicate, no prompt")
check(d.ask(f"ADD {SECRET} SHA1 6 30 bad_name") == "ERR BADARG", "bad name, no prompt")
check(d.ask("DEL 7") == "ERR NOTFOUND", "DEL unknown slot, no prompt")

check(d.confirmed("TOKEN 0 35") == "OK 287082", "TOKEN accepted: counter 1 (t=35)")
check(d.confirmed("TOKEN 0 1111111090") == "OK 081804", "TOKEN accepted: leading zero in the code")
check(d.confirmed("TOKEN 0 35", hold="100") == "ERR DENIED", "TOKEN tap is denied")
check(d.confirmed("TOKEN 0 35", hold="900") == "ERR DENIED", "TOKEN 0.9 s is denied")
check(d.confirmed("TOKEN 0 35", hold="2300") == "ERR DENIED", "TOKEN 2.3 s is denied")
check(d.confirmed("TOKEN 0 35", hold="1100") == "OK 287082", "TOKEN 1.1 s is accepted")
check(d.confirmed("TOKEN 0 35", hold="1900") == "OK 287082", "TOKEN 1.9 s is accepted")

# elapsed time is added: requested at t=29 (counter 0, code 755224), confirmed
# about 3.5 s later, which is t=32 and counter 1 (code 287082)
d.send("TOKEN 0 29")
d.line()
time.sleep(2.0)  # 29 + 2 s crosses into the next 30 s period before the press
d.press("1500")
reply = d.line()
check(reply == "OK 287082", "token uses request time plus elapsed time", reply + " (755224 means no elapsed time was added)")

d.send("TOKEN 0 35")
check(d.line() == "PENDING", "second request is pending")
d.send("HELLO")
check(d.line() == "ERR BUSY", "other commands get ERR BUSY while pending")
d.press("1500")
check(d.line() == "OK 287082", "the pending request still completes")

check(d.ask("GETTZ") == "OK 0", "GETTZ default")
check(d.confirmed("SETTZ 8") == "OK", "SETTZ accepted")
check(d.ask("GETTZ") == "OK 8", "GETTZ after SETTZ")
check(d.confirmed("SETTZ -4", hold="300") == "ERR DENIED", "SETTZ denied keeps the value")
check(d.ask("GETTZ") == "OK 8", "GETTZ unchanged")
check(d.ask("GETCFG") == "OK 00", "GETCFG default")
check(d.confirmed("SETCFG 04") == "OK", "SETCFG accepted")
check(d.ask("GETCFG") == "OK 04", "GETCFG after SETCFG")
check(d.confirmed("SETCFG 00") == "OK", "SETCFG back to default")
check(d.ask("SETCFG 10") == "ERR BADARG", "reserved config bit")
check(d.ask("FROB") == "ERR UNKNOWN", "unknown command")
check(d.ask("TOKEN 0") == "ERR BADARG", "missing argument")

check(d.confirmed("DEL 0") == "OK", "DEL accepted")
d.send("LIST")
check(d.lines_until_end() == [], "LIST empty after DEL")
check(d.ask("TOKEN 0 35") == "ERR NOTFOUND", "TOKEN on a deleted key")

check(d.confirmed(f"ADD {SECRET} SHA1 8 60 Wipe me") == "OK 0", "ADD for the wipe test")
check(d.confirmed("WIPE", hold="1500") == "ERR DENIED", "WIPE needs the long hold (1.5 s denied)")
check(d.confirmed("WIPE", hold="5500") == "OK", "WIPE accepted at 5.5 s")
d.send("LIST")
check(d.lines_until_end() == [], "no keys after WIPE")
check(d.ask("GETTZ") == "OK 8", "timezone survives WIPE")
check(d.ask("WIPE") == "OK", "WIPE on an empty device needs no confirmation")
check(d.confirmed("SETTZ 0") == "OK", "restore timezone 0")

if SLOW:
    d.confirmed(f"ADD {SECRET} SHA1 6 30 Timeout")
    d.send("TOKEN 0 35")
    check(d.line() == "PENDING", "request for the timeout test")
    d.nano.timeout = 40
    check(d.line() == "ERR TIMEOUT", "request times out after 30 s")
    d.confirmed("DEL 0")

print(f"{'ALL PASSED' if not failures else str(failures) + ' FAILED'}")
sys.exit(1 if failures else 0)
