#!/usr/bin/env python3
"""Smoke test of a built trustctl binary against its simulated device (--device DUMMY).

  tools/build/smoke_dummy.py [path/to/trustctl]

Needs no hardware: add two keys, list, get a token and compare it with a TOTP computed
here, delete, stop the daemon. Runs the daemon in a private XDG_RUNTIME_DIR.
"""
import base64
import hashlib
import hmac
import os
import struct
import subprocess
import sys
import tempfile
import time

BIN = os.path.abspath(sys.argv[1] if len(sys.argv) > 1 else "trustctl/bin/trustctl")
SECRET = base64.b32encode(b"12345678901234567890").decode()
failures = 0


def totp(t):
    mac = hmac.new(b"12345678901234567890", struct.pack(">Q", t // 30), hashlib.sha1).digest()
    o = mac[-1] & 15
    return "%06d" % ((struct.unpack(">I", mac[o:o + 4])[0] & 0x7FFFFFFF) % 10**6)


def check(ok, what, detail=""):
    global failures
    failures += not ok
    print(f"{'PASS' if ok else 'FAIL'}  {what}" + (f"   [{detail}]" if detail and not ok else ""), flush=True)


with tempfile.TemporaryDirectory() as tmp:
    env = dict(os.environ, XDG_RUNTIME_DIR=tmp, HOME=tmp, TRUSTCTL_NO_DAEMON="")
    os.chmod(tmp, 0o700)

    def run(*a, stdin=None):
        p = subprocess.run([BIN, "--device", "DUMMY", *a], input=stdin, capture_output=True, text=True, env=env, timeout=60)
        return p.returncode, p.stdout, p.stderr

    try:
        rc, out, err = run("add", "GitHub", stdin=SECRET + "\n")
        check(rc == 0, "add GitHub", err)
        check("simulat" in err.lower(), "the DUMMY notice is printed once", err)
        rc, out, err = run("add", stdin="otpauth://totp/Example:bob?secret=" + SECRET + "&issuer=Example\n")
        check(rc == 0, "add from an otpauth URI", err)
        rc, out, err = run("list")
        check(out.split() == ["0", "GitHub", "1", "Example:bob"], "list shows both keys", out)
        check("simulat" not in err.lower(), "no notice on later commands", err)
        before = int(time.time())
        rc, out, err = run("token", "github")
        after = int(time.time())
        check(rc == 0 and out.strip() in {totp(before), totp(after)}, "token matches an independent TOTP", out + err)
        rc, out, err = run("del", "GitHub")
        check(rc == 0, "del", err)
        rc, out, err = run("list")
        check(out.split() == ["1", "Example:bob"], "only the second key is left", out)
        rc, out, err = run("flash")
        check(rc != 0 and "DUMMY" in err, "flash refuses the dummy device", err)
        rc, out, err = run("version")
        check(rc == 0 and "built-in firmware" in out, "version", out + err)
    finally:
        subprocess.run([BIN, "daemon", "stop"], env=env, capture_output=True, timeout=30)

print("ALL PASSED" if not failures else f"{failures} FAILED")
sys.exit(1 if failures else 0)
