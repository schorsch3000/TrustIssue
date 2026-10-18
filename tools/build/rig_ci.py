#!/usr/bin/env python3
"""The rig's CI: runs on the machine the rig is attached to (a Raspberry Pi), polls GitHub and tests every new master commit.

  tools/build/rig_ci.py [--once] [--dry-run]

No GitHub runner and no code from pull requests: the machine pulls. Every run (a systemd timer starts it, see
tools/rig/README.md) does this:
  1. asks GitHub which commit the rolling prerelease "nightly" was built from (only CI on a push to master can move it);
  2. does nothing if that commit was tested already;
  3. downloads that release's trustctl for this machine and its firmware archive (core.hex, boot.hex), checks them against
     SHA256SUMS, and checks out the test scripts at exactly that commit (it must be on master) in a separate worktree;
  4. switches the rig's power on (`/usr/local/bin/rig-power on`, if that exists), waits for its USB ports, runs
     `make_release.py --rig-only`: provisions the device over ISP with those files and runs the rig suites, and switches the
     power off again (`rig-power off`) NO MATTER HOW IT ENDS: a failing test, an exception, a SIGTERM (systemd's timeout), and
     a stale "powered" marker of a run that was killed hard is cleaned up by the next poll;
  5. posts a commit status "device-tests" (success, failure, or error when the rig could not be found).

Only one run at a time (a lock in the state directory, so a manual run and the timer cannot meet at the rig).

Needs: python3 with pyserial, avrdude, git, and a fine-grained token with only "Commit statuses: read and write" on the
repository, in ~/.config/trustissue-rig/token (mode 600). Reports and logs: ~/reports/<commit>.md and .log.
Only the Python standard library is used besides pyserial (through rig.py).
"""
import argparse
import fcntl
import hashlib
import json
import os
import platform
import shutil
import signal
import subprocess
import sys
import tarfile
import tempfile
import time
import urllib.request
import zipfile

REPO = "schorsch3000/TrustIssue"
CONTEXT = "device-tests"
HOME = os.path.expanduser("~")
STATE = os.path.join(HOME, ".local", "state", "trustissue-rig")
REPORTS = os.path.join(HOME, "reports")
TOKEN_FILE = os.path.join(HOME, ".config", "trustissue-rig", "token")
POWER = os.environ.get("TRUSTISSUE_RIG_POWER", "/usr/local/bin/rig-power")  # `rig-power on|off` switches the rig's supply
POWER_MARKER = os.path.join(STATE, "powered")  # exists while the rig was switched on by a run that has not switched it off
ARCH = {"x86_64": "amd64", "aarch64": "arm64", "armv7l": "arm", "armv6l": "arm", "i686": "386"}
ROOT = os.path.normpath(os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", ".."))


def api(path, data=None, token=None):
    request = urllib.request.Request(f"https://api.github.com/repos/{REPO}/{path}", headers={
        "User-Agent": "trustissue-rig-ci", "Accept": "application/vnd.github+json"})
    if token:
        request.add_header("Authorization", f"Bearer {token}")
    if data is not None:
        request.data = json.dumps(data).encode()
        request.add_header("Content-Type", "application/json")
    with urllib.request.urlopen(request, timeout=60) as response:
        return json.load(response)


def download(url, path):
    request = urllib.request.Request(url, headers={"User-Agent": "trustissue-rig-ci"})
    with urllib.request.urlopen(request, timeout=300) as response, open(path, "wb") as out:
        shutil.copyfileobj(response, out)


def git(*args, cwd=ROOT):
    return subprocess.run(["git", *args], cwd=cwd, capture_output=True, text=True, check=True).stdout.strip()


def post_status(token, sha, state, description, dry_run):
    print(f"status {state}: {description}", flush=True)
    if dry_run:
        return
    api(f"statuses/{sha}", {"state": state, "context": CONTEXT, "description": description[:140]}, token)


def sha256(path):
    h = hashlib.sha256()
    with open(path, "rb") as f:
        for block in iter(lambda: f.read(1 << 20), b""):
            h.update(block)
    return h.hexdigest()


def power(state):
    """Switches the rig on or off with rig-power, if there is one. The marker records that it is on, so that a run that was
    killed without a chance to switch off is noticed by the next poll."""
    if not os.path.exists(POWER):
        return
    print(f"rig_ci: rig-power {state}", flush=True)
    result = subprocess.run([POWER, state], capture_output=True, text=True, timeout=60)
    if result.returncode:
        raise RuntimeError(f"{POWER} {state} failed: {(result.stderr or result.stdout).strip()}")
    if state == "on":
        open(POWER_MARKER, "w").close()
    elif os.path.exists(POWER_MARKER):
        os.remove(POWER_MARKER)


def wait_for_rig_ports(timeout=90):
    """After switching on: the two Nanos (the rig and the device) show up as serial ports, then they need a moment to boot."""
    import rig
    deadline = time.time() + timeout
    while time.time() < deadline and len(rig.serial_ports()) < 2:
        time.sleep(1)
    time.sleep(4)


def prune_reports(keep=100):
    """Keeps the reports and logs of the `keep` newest commits (by modification time) and deletes the rest. latest.* stays."""
    groups = {}
    for name in os.listdir(REPORTS):
        stem, dot, ext = name.partition(".")
        if dot and len(stem) == 40 and ext in ("md", "log") and all(c in "0123456789abcdef" for c in stem):
            groups.setdefault(stem, []).append(os.path.join(REPORTS, name))
    newest_first = sorted(groups, key=lambda stem: max(os.path.getmtime(p) for p in groups[stem]), reverse=True)
    for stem in newest_first[keep:]:
        for path in groups[stem]:
            os.remove(path)
    return max(0, len(newest_first) - keep)


def main():
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--once", action="store_true", help="accepted for the service file; every run is one poll anyway")
    parser.add_argument("--dry-run", action="store_true", help="do everything except posting the status, and test the commit even if it was tested")
    args = parser.parse_args()
    os.makedirs(STATE, exist_ok=True)
    os.makedirs(REPORTS, exist_ok=True)
    lock = open(os.path.join(STATE, "run.lock"), "w")  # held until the process ends (not "lock": an old service file wraps the script in flock on that file)
    try:
        fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
    except BlockingIOError:
        print("rig_ci: another run is in progress")
        return
    signal.signal(signal.SIGTERM, lambda *_: sys.exit(143))  # systemd's stop: leave through the finally blocks, which switch the power off
    if os.path.exists(POWER_MARKER):  # a run was killed hard while the rig was on: nobody else is running (we hold the lock)
        print("rig_ci: the rig was left switched on: switching it off", flush=True)
        try:
            power("off")
        except Exception as e:
            print(f"rig_ci: {e}", file=sys.stderr)
    removed = prune_reports()
    if removed:
        print(f"rig_ci: removed the reports of {removed} old commits (the newest 100 stay)", flush=True)
    try:  # leftovers of a killed run
        git("worktree", "prune")
    except subprocess.CalledProcessError:
        pass

    sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
    import rig  # noqa: E402
    rig.require_tools(rig=True, git=True)  # the bootloader comes from the download: no compiler is needed
    arch = ARCH.get(platform.machine())
    if not arch:
        sys.exit(f"rig_ci: no trustctl build for {platform.machine()}")
    token = None
    if not args.dry_run:
        try:
            token = open(TOKEN_FILE).read().strip()
        except OSError:
            sys.exit(f"rig_ci: no token in {TOKEN_FILE}")

    # 1. which commit is the nightly built from?
    release = api("releases/tags/nightly")
    sha = api("git/ref/tags/nightly")["object"]["sha"]
    short = sha[:7]
    last = open(os.path.join(STATE, "last")).read().strip() if os.path.exists(os.path.join(STATE, "last")) else ""
    if sha == last and not args.dry_run:
        return
    assets = {a["name"]: a["browser_download_url"] for a in release["assets"]}
    wanted = {
        "sums": "SHA256SUMS",
        "trustctl": next((n for n in assets if n.endswith(f"-{short}-linux-{arch}.tar.gz")), None),
        "firmware": next((n for n in assets if n.startswith("trustctl-firmware-") and n.endswith(f"-{short}.zip")), None),
    }
    if not wanted["trustctl"] or not wanted["firmware"] or "SHA256SUMS" not in assets:
        print(f"rig_ci: the nightly for {short} is not complete yet (assets: {len(assets)}); trying again next time")
        return

    # 2. the commit must be on master
    git("fetch", "--quiet", "origin", "master")
    try:
        git("merge-base", "--is-ancestor", sha, "origin/master")
    except subprocess.CalledProcessError:
        print(f"rig_ci: {short} is not on master: not testing it")
        if not args.dry_run:
            open(os.path.join(STATE, "last"), "w").write(sha)
        return

    work = tempfile.mkdtemp(prefix="rig-ci-")
    powered = False
    log_path = os.path.join(REPORTS, f"{sha}.log")
    report_path = os.path.join(REPORTS, f"{sha}.md")
    try:
        for kind, path in (("log", log_path), ("md", report_path)):  # ~/reports/latest.log: the newest run, whatever its hash
            link = os.path.join(REPORTS, f"latest.{kind}")
            if os.path.lexists(link):
                os.remove(link)
            os.symlink(os.path.basename(path), link)
        post_status(token, sha, "pending", "device tests are running on the rig", args.dry_run)
        # 3. download and check
        for key in ("sums", "trustctl", "firmware"):
            download(assets[wanted[key]], os.path.join(work, wanted[key]))
        sums = dict(line.split("  ", 1)[::-1] for line in open(os.path.join(work, "SHA256SUMS")).read().splitlines() if "  " in line)
        for key in ("trustctl", "firmware"):
            name = wanted[key]
            if sums.get(name) != sha256(os.path.join(work, name)):
                raise RuntimeError(f"{name} does not match SHA256SUMS")
        tree = os.path.join(work, "tree")
        git("worktree", "add", "--detach", "--quiet", tree, sha)
        with tarfile.open(os.path.join(work, wanted["trustctl"])) as tar:
            member = next(m for m in tar.getmembers() if m.name.endswith("/trustctl"))
            member.name = "trustctl"
            os.makedirs(os.path.join(tree, "trustctl", "bin"), exist_ok=True)
            tar.extract(member, os.path.join(tree, "trustctl", "bin"), filter="data")
        with zipfile.ZipFile(os.path.join(work, wanted["firmware"])) as z:
            z.extractall(os.path.join(work, "firmware"))
        fw = next(os.path.join(work, "firmware", d) for d in os.listdir(os.path.join(work, "firmware")))
        version = open(os.path.join(fw, "VERSION")).read().strip()

        # 4. the tests: the rig gets power only now, and loses it again whatever happens
        powered = True  # set first: a failing rig-power on may have switched it on
        power("on")
        wait_for_rig_ports()
        command = [sys.executable, "tools/build/make_release.py", version, "--rig-only", "--yes",
                   "--trustctl", os.path.join(tree, "trustctl", "bin", "trustctl"),
                   "--core", os.path.join(fw, "core.hex"), "--bootloader", os.path.join(fw, "boot.hex"),
                   "--report", report_path]
        started = time.time()
        with open(log_path, "w") as log:
            result = subprocess.run(command, cwd=tree, stdout=log, stderr=subprocess.STDOUT, timeout=3 * 3600)
        minutes = (time.time() - started) / 60
        text = open(log_path, errors="replace").read()
        passed = sum(1 for l in text.splitlines() if l.startswith("PASS"))
        if result.returncode == 0:
            post_status(token, sha, "success", f"all rig suites passed: {passed} checks in {minutes:.0f} min", args.dry_run)
        elif "make_release: no rig among" in text or "RigNotFound" in text or "no rig" in text:
            post_status(token, sha, "error", "the rig was not found: not tested", args.dry_run)
            return  # try this commit again at the next poll
        else:
            failed = [l for l in text.splitlines() if l.startswith("FAIL")][:2]
            lines = [l.strip() for l in text.splitlines() if l.strip()]
            # without a FAIL line the run broke before or between the suites: the last line of the log is the reason
            reason = "; ".join(failed) if failed else (lines[-1] if lines else "no output")
            post_status(token, sha, "failure", f"rig tests failed: {reason}"[:140], args.dry_run)
        if not args.dry_run:  # a dry run does not count as tested
            open(os.path.join(STATE, "last"), "w").write(sha)
    except Exception as e:
        post_status(token, sha, "error", f"rig CI error: {e}"[:140], args.dry_run)
        raise
    finally:
        if powered:
            try:
                power("off")
            except Exception as e:  # the marker stays: the next poll tries again
                print(f"rig_ci: {e}", file=sys.stderr)
        try:
            git("worktree", "remove", "--force", os.path.join(work, "tree"))
        except Exception:
            pass
        shutil.rmtree(work, ignore_errors=True)


if __name__ == "__main__":
    main()
