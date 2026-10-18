#!/usr/bin/env python3
"""Tests rig_ci.py without GitHub, a rig or a Pi: the rig must be switched on only for the tests and switched off again
in every case, and a rig that was left on by a killed run must be switched off by the next poll.

  tools/build/test_rig_ci.py
"""
import hashlib
import importlib.util
import io
import os
import stat
import subprocess
import sys
import tarfile
import tempfile
import types
import zipfile

here = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, here)
spec = importlib.util.spec_from_file_location("rig_ci", os.path.join(here, "rig_ci.py"))
rig_ci = importlib.util.module_from_spec(spec)
spec.loader.exec_module(rig_ci)
import rig  # noqa: E402

SHA = "a" * 40
failures = 0


def check(ok, what, detail=""):
    global failures
    failures += not ok
    print(f"{'PASS' if ok else 'FAIL'}  {what}" + (f"   [{detail}]" if detail and not ok else ""), flush=True)


def scenario(name, make_release=0, raise_in_make_release=None, power_on_fails=False, marker_before=False, no_power_script=False):
    """Runs main() with everything outside mocked; returns (power calls, statuses, marker present)."""
    tmp = tempfile.mkdtemp()
    state, reports = os.path.join(tmp, "state"), os.path.join(tmp, "reports")
    os.makedirs(state)
    token = os.path.join(tmp, "token")
    open(token, "w").write("t")
    calls = os.path.join(tmp, "calls")
    script = os.path.join(tmp, "rig-power")
    open(script, "w").write(f'#!/bin/sh\necho "$1" >> {calls}\n' + ('[ "$1" = on ] && exit 1\n' if power_on_fails else ""))
    os.chmod(script, os.stat(script).st_mode | stat.S_IEXEC)
    rig_ci.STATE, rig_ci.REPORTS, rig_ci.TOKEN_FILE = state, reports, token
    rig_ci.POWER = os.path.join(tmp, "missing") if no_power_script else script
    rig_ci.POWER_MARKER = os.path.join(state, "powered")
    if marker_before:
        open(rig_ci.POWER_MARKER, "w").close()
    statuses = []
    short = SHA[:7]
    names = {"SHA256SUMS": None, f"trustctl-nightly-fw1-{short}-linux-{rig_ci.ARCH.get(os.uname().machine, 'amd64')}.tar.gz": None,
             f"trustctl-firmware-nightly-fw1-{short}.zip": None}
    files = {}
    buf = io.BytesIO()
    with tarfile.open(fileobj=buf, mode="w:gz") as tar:
        data = b"binary"
        info = tarfile.TarInfo("x/trustctl")
        info.size = len(data)
        tar.addfile(info, io.BytesIO(data))
    files[[n for n in names if n.endswith(".tar.gz")][0]] = buf.getvalue()
    buf = io.BytesIO()
    with zipfile.ZipFile(buf, "w") as z:
        for n in ("VERSION", "core.hex", "boot.hex"):
            z.writestr("fw/" + n, "1\n")
    files[[n for n in names if n.endswith(".zip")][0]] = buf.getvalue()
    files["SHA256SUMS"] = ("".join(f"{hashlib.sha256(v).hexdigest()}  {k}\n" for k, v in files.items())).encode()

    def api(path, data=None, token=None):
        if path == "releases/tags/nightly":
            return {"assets": [{"name": n, "browser_download_url": "http://x/" + n} for n in files]}
        if path == "git/ref/tags/nightly":
            return {"object": {"sha": SHA}}
        raise AssertionError(path)

    def download(url, path):
        open(path, "wb").write(files[url.rsplit("/", 1)[1]])

    def git(*args, cwd=None):
        if args[:2] == ("worktree", "add"):
            os.makedirs(args[-2], exist_ok=True)
        return ""

    real_run = subprocess.run

    def run(cmd, *a, **kw):
        if cmd[0] == rig_ci.POWER:
            return real_run(cmd, *a, **kw)
        if "make_release.py" in " ".join(cmd):
            if raise_in_make_release:
                raise raise_in_make_release
            kw["stdout"].write(("PASS  a check\n" if make_release == 0 else "FAIL  a check\n"))
            return subprocess.CompletedProcess(cmd, make_release)
        raise AssertionError(cmd)

    rig_ci.api, rig_ci.download, rig_ci.git = api, download, git
    rig_ci.post_status = lambda token, sha, state_, description, dry: statuses.append(state_)
    fake = types.SimpleNamespace(**{k: getattr(subprocess, k) for k in dir(subprocess) if not k.startswith("_")})
    fake.run = run
    rig_ci.subprocess = fake
    rig.serial_ports = lambda: ["/dev/ttyUSB0", "/dev/ttyUSB1"]
    rig_ci.time.sleep = lambda s: None
    rig.require_tools = lambda **kw: None
    sys.argv = ["rig_ci.py"]
    error = None
    try:
        rig_ci.main()
    except BaseException as e:  # noqa: BLE001
        error = e
    on_off = open(calls).read().split() if os.path.exists(calls) else []
    return on_off, statuses, os.path.exists(rig_ci.POWER_MARKER), error


on_off, statuses, marker, error = scenario("passing tests")
check(on_off == ["on", "off"] and statuses[-1] == "success" and not marker, "passing tests: power on, then off", f"{on_off} {statuses} {marker}")
on_off, statuses, marker, error = scenario("failing tests", make_release=1)
check(on_off == ["on", "off"] and statuses[-1] == "failure" and not marker, "failing tests: power off all the same", f"{on_off} {statuses}")
on_off, statuses, marker, error = scenario("exception", raise_in_make_release=RuntimeError("boom"))
check(on_off == ["on", "off"] and isinstance(error, RuntimeError) and not marker, "an exception during the tests: power off all the same", f"{on_off} {error!r}")
on_off, statuses, marker, error = scenario("timeout", raise_in_make_release=subprocess.TimeoutExpired("x", 1))
check(on_off == ["on", "off"] and not marker, "a timeout of the tests: power off all the same", f"{on_off} {error!r}")
on_off, statuses, marker, error = scenario("sigterm", raise_in_make_release=SystemExit(143))
check(on_off == ["on", "off"] and isinstance(error, SystemExit) and not marker, "SIGTERM (SystemExit 143): power off all the same", f"{on_off} {error!r}")
on_off, statuses, marker, error = scenario("power on fails", power_on_fails=True)
check(on_off.count("off") == 1 and "error" in statuses, "a failing rig-power on: error status, and off is still tried", f"{on_off} {statuses}")
on_off, statuses, marker, error = scenario("stale marker", marker_before=True)
check(on_off[:2] == ["off", "on"] and on_off[-1] == "off" and not marker, "a rig left on by a killed run is switched off first", f"{on_off}")
on_off, statuses, marker, error = scenario("no power script", no_power_script=True)
check(on_off == [] and statuses[-1] == "success", "without rig-power the run works as before", f"{on_off} {statuses}")

print("ALL PASSED" if not failures else f"{failures} FAILED")
sys.exit(1 if failures else 0)
