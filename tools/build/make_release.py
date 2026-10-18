#!/usr/bin/env python3
"""Makes a release: runs every test (the rig tests too), writes the test report, tags.

  tools/build/make_release.py X.Y.Z [--rig PORT] [--yes]
  tools/build/make_release.py X.Y.Z --dry-run [--report FILE]       everything, the rig too, but nothing is committed or tagged
  tools/build/make_release.py X.Y.Z --host-only [--report FILE]     dry run: only the tests that need no device,
                                                                     report to a file, nothing is committed or tagged

What it does, in order:
  1. checks that you are on master with a clean tree, that X.Y.Z equals the firmware version (core.ino and the
     embedded copy), that the tag does not exist yet and that tag signing is set up;
  2. rebuilds the firmware and trustctl, runs the tests that need no device (the same as CI), then, on the rig
     (a Nano with firmware/rigisp wired to the device under test, see tools/build/rig.py): finds the rig by itself,
     provisions the device over ISP (chip erase, bootloader, the firmware under test: so it starts empty, on UTC+0
     with the default settings) and runs core_test, autowipe_test, trustctl_e2e, boot_test --pin,
     trustctl_flash_e2e and locked_test (the lock bits, which also leaves the device provisioned again). A device
     that is not wired to the rig, a production device on the same machine for one, is never touched: the device
     is the port that reacts to the rig's reset line;
  3. writes docs/test-reports/vX.Y.Z.md, commits it, creates the signed tag vX.Y.Z and verifies it;
  4. asks before it pushes master and the tag. The tag push starts the release workflow.

Any failed suite stops the release before anything is written to git.
"""
import argparse
import datetime
import os
import platform
import re
import shutil
import signal
import subprocess
import threading
import sys
import time

ROOT = os.path.normpath(os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", ".."))
os.chdir(ROOT)
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import rig as rig_module  # noqa: E402

parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
parser.add_argument("version", help="X.Y.Z, without the v")
parser.add_argument("--rig", help="serial port of the rig Nano (default: look at every serial port)")
parser.add_argument("--dry-run", action="store_true", help="with the rig, but nothing is committed, tagged or pushed; the report goes to a file")
parser.add_argument("--rig-only", action="store_true", help="only the rig: provision and run the rig suites (what the Pi does for CI); nothing is committed or tagged, the report goes to --report")
parser.add_argument("--trustctl", help="use this trustctl binary for the tests instead of trustctl/bin/trustctl (CI builds)")
parser.add_argument("--core", help="provision with this firmware hex instead of the embedded core.hex")
parser.add_argument("--bootloader", help="provision with this bootloader hex instead of building one")
parser.add_argument("--host-only", action="store_true", help="dry run without the rig; never commits, tags or pushes")
parser.add_argument("--report", help="with --host-only: where to write the report")
parser.add_argument("--suite-timeout", type=int, default=None, help="seconds one suite may take (default: 900 for host suites, 1800 for rig suites)")
parser.add_argument("--timeout", type=int, default=7200, help="seconds the whole run may take, rig included; what is left is not run and counts as failed")
parser.add_argument("--yes", action="store_true", help="do not ask before starting the rig tests (still asks before pushing)")
args = parser.parse_args()
no_git = args.host_only or args.dry_run or args.rig_only

version = args.version
tag = f"v{version}"
if not re.fullmatch(r"\d+\.\d+\.\d+", version):
    sys.exit("version must look like 1.2.3")

env = dict(os.environ)
gopath = env.get("GOPATH_RELEASE", os.path.expanduser("~/go"))
env["GOPATH"] = gopath


def out(*cmd, check=True, **kw):
    r = subprocess.run(cmd, capture_output=True, text=True, env=env, **kw)
    if check and r.returncode:
        sys.exit(f"{' '.join(cmd)} failed:\n{r.stdout}{r.stderr}")
    return r.stdout.strip()


def die(msg):
    sys.exit(f"make_release: {msg}")


# ---- 1. preconditions ------------------------------------------------------------------------------------------
if out("git", "rev-parse", "--abbrev-ref", "HEAD") != "master" and not no_git:
    die("not on master")
if out("git", "status", "--porcelain", "--untracked-files=no") and not no_git:
    die("the working tree has uncommitted changes")
core_version = re.search(r'VERSION\[\] PROGMEM = "(.*)";', open("firmware/core/core.ino").read()).group(1)
embedded_version = open("trustctl/internal/firmware/VERSION").read().strip()
if not (core_version == embedded_version == version):
    die(f"version mismatch: argument {version}, core.ino {core_version}, embedded {embedded_version}. "
        "Change VERSION in firmware/core/core.ino (and the expectation in tools/build/core_test.py), run "
        "tools/build/embed_firmware.sh and commit, then run this again.")
if not no_git:
    if out("git", "tag", "-l", tag):
        die(f"tag {tag} exists already")
    if out("git", "config", "--get", "user.signingkey", check=False) == "":
        die("no signing key: git config user.signingkey, gpg.format ssh (see docs/RELEASING.md)")
    if not os.path.exists(".github/allowed_signers"):
        die(".github/allowed_signers is missing (the release workflow verifies the tag against it)")
    out("git", "fetch", "origin", "master")
    if out("git", "rev-list", "--count", "HEAD..origin/master") != "0":
        die("origin/master has commits you do not have")

tested_commit = out("git", "rev-parse", "HEAD")


# ---- 2. the tests ----------------------------------------------------------------------------------------------
class Suite:
    def __init__(self, name, where, cmd, cwd=None, needs=None, shell=False):
        self.name, self.where, self.cmd, self.cwd, self.needs, self.shell = name, where, cmd, cwd, needs, shell
        self.timeout = None  # seconds; set from where it runs
        self.status = "not run"
        self.passed = self.failed = 0
        self.seconds = 0.0
        self.lines = []


def sh(script):
    return ["sh", "-ec", script]


trustctl_bin = os.path.abspath(args.trustctl) if args.trustctl else os.path.join(ROOT, "trustctl", "bin", "trustctl")
host = [
    Suite("gofmt and go vet", "host", sh('cd trustctl; test -z "$(gofmt -l .)" || { gofmt -l .; exit 1; }; go vet ./... && echo "PASS  gofmt, go vet"')),
    Suite("trustctl unit tests (go test)", "host", sh("cd trustctl && go test -count=1 ./...")),
    Suite("firmware host tests (SHA-1, HMAC, TOTP, Base32, storage, protocol, time)", "host", ["firmware/test/run.sh"]),
    Suite("shellcheck", "host", sh("shellcheck -S warning tools/user/trustctl-menu tools/user/*.sh tools/build/*.sh firmware/*/build.sh && echo 'PASS  shellcheck'")),
    Suite("trustctl-menu tests (fake launchers)", "host", ["tools/build/test_trustctl_menu.sh"]),
    Suite("onboard_usbtiny.sh tests (fake avrdude)", "host", ["tools/build/test_onboard_usbtiny.sh"]),
    Suite("tray tool helper tests (Mono)", "host", ["tools/user/trustctl-tray/test.sh"], needs="mcs"),
    Suite("firmware builds and equals the embedded images", "host", sh(
        "tools/build/embed_firmware.sh && make -B -C firmware/bootloader && "
        "git diff --exit-code -- trustctl/internal/firmware && echo 'PASS  core, calibration and bootloader build; embedded images are what the sources build'")),
    Suite("trustctl build and simulated-device smoke test (--device DUMMY)", "host", sh(
        "cd trustctl && CGO_ENABLED=0 go build -o bin/trustctl ./cmd/trustctl && cd .. && tools/build/smoke_dummy.py trustctl/bin/trustctl")),
]
if args.rig_only:
    host = []  # CI ran these already
rig = []


def rig_suites(rig_port, device):
    return [
        Suite("core firmware: protocol, button gesture, storage, time (core_test)", "rig", ["tools/build/core_test.py", device, rig_port]),
        Suite("autonomous wipe (autowipe_test)", "rig", ["tools/build/autowipe_test.py", device, rig_port]),
        Suite("trustctl against the device (trustctl_e2e)", "rig", ["tools/build/trustctl_e2e.py", "--bin", trustctl_bin, "--device", device, "--rig", rig_port]),
        Suite("bootloader gate and update PIN (boot_test --pin)", "rig", ["tools/build/boot_test.py", device, rig_port, "--pin"]),
        Suite("trustctl flash and calibrate (trustctl_flash_e2e)", "rig", ["tools/build/trustctl_flash_e2e.py", "--bin", trustctl_bin, "--device", device, "--rig", rig_port]),
        Suite("lock bits and the unlocked indicator (locked_test)", "rig", ["tools/build/locked_test.py", "--rig", rig_port, "--device", device, "--bin", trustctl_bin]),
    ]


deadline = time.time() + args.timeout
ANSI = re.compile(r"\x1b\[[0-9;]*m")


def run_suite(s):
    if args.suite_timeout:
        s.timeout = args.suite_timeout
    if s.needs and not shutil.which(s.needs):
        s.status = f"skipped ({s.needs} not installed)"
        print(f"\n=== {s.name}: {s.status}", flush=True)
        return True
    print(f"\n=== {s.name}", flush=True)
    start = time.time()
    left = deadline - start
    if left <= 0:
        s.status = "FAILED"
        s.lines = ["FAIL  not run: the time limit of the whole run (--timeout) was used up"]
        print(s.lines[0], flush=True)
        return False
    limit = min(s.timeout or (1800 if s.where == "rig" else 900), left)
    p = subprocess.Popen(s.cmd, cwd=s.cwd or ROOT, env=env, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True,
                         errors="replace", start_new_session=True)
    timed_out = []

    def kill():  # a hung test must not hold the rig (or the CI runner) for ever: kill the whole process group
        timed_out.append(True)
        try:
            os.killpg(p.pid, signal.SIGKILL)
        except ProcessLookupError:
            pass

    timer = threading.Timer(limit, kill)
    timer.start()
    for line in p.stdout:
        print(line, end="", flush=True)
        s.lines.append(ANSI.sub("", line.rstrip("\n")))
    p.wait()
    timer.cancel()
    if timed_out:
        s.lines.append(f"FAIL  timed out after {limit:.0f} s and was killed")
        print(s.lines[-1], flush=True)
    s.seconds = time.time() - start
    s.passed = sum(1 for l in s.lines if l.startswith(("PASS", "ok ")))
    s.failed = sum(1 for l in s.lines if l.startswith(("FAIL", "--- FAIL")))
    ok = p.returncode == 0 and s.failed == 0 and not timed_out
    s.status = "passed" if ok else "FAILED"
    return ok


def verify_device(port):
    """After provisioning the device must be empty, on UTC+0 and have the default settings. Read-only commands only."""
    base = [trustctl_bin, "--no-daemon", "--device", port]

    def ask(*a):
        r = subprocess.run(base + list(a), capture_output=True, text=True, timeout=60)
        if r.returncode:
            die(f"trustctl {' '.join(a)} on {port} failed: {r.stderr.strip()}")
        return r.stdout

    keys = [l for l in ask("list").splitlines() if re.match(r"\s*\d+\s", l)]
    if keys:
        die(f"{port} holds {len(keys)} key(s) after provisioning: refusing to run the rig tests on it.")
    tz = ask("tz", "get")
    if not re.search(r"device:\s+UTC\+0(:00)?\s", tz + " "):
        die(f"the device is not on UTC+0 after provisioning ({tz.strip().splitlines()[0]})")
    cfg = dict(l.split() for l in ask("config", "get").splitlines() if l.strip())
    if cfg != {"rotate": "off", "invert": "off", "logo": "off", "autowipe": "off"}:
        die(f"the device settings are not the defaults: {cfg}")
    print(f"dev device on {port}: no keys, UTC+0, default settings", flush=True)


rig_port = None
if not args.host_only:
    try:
        found = rig_module.find_rig([args.rig] if args.rig else None)
    except rig_module.RigNotFound as e:
        die(str(e))
    rig_port = found.path
    found.close()
    print(f"rig: {rig_port} (sketch {found.version})", flush=True)
    if not args.yes:
        print("The rig will erase the device wired to it (chip erase over ISP), flash the firmware under test and press its button.")
        if input("Start? [y/N] ").strip().lower() != "y":
            die("cancelled")

started = datetime.datetime.now(datetime.timezone.utc)
# the binary the rig tests use is built by the suite "trustctl build ..." above, so run the host suites first
for s in host:
    if not run_suite(s):
        break
if all(s.status != "FAILED" for s in host) and rig_port:
    provision = Suite("provision the device over ISP: chip erase, bootloader, firmware under test (rig.py provision)", "rig",
                      ["tools/build/rig.py", "provision"]
                      + (["--core", os.path.abspath(args.core)] if args.core else [])
                      + (["--bootloader", os.path.abspath(args.bootloader)] if args.bootloader else []))
    rig = [provision]
    if run_suite(provision):
        device = next((l.split()[-1] for l in provision.lines if l.startswith("device:")), None)
        if not device:
            die("rig.py provision did not say which port the device is on")
        verify_device(device)
        rig_tests = rig_suites(rig_port, device)
        rig += rig_tests
        for s in rig_tests:
            if not run_suite(s):
                break

# ---- 3. the report ---------------------------------------------------------------------------------------------
allsuites = host + rig


def tool_version(*cmd):
    try:
        return " ".join(out(*cmd, check=False).split()[:4]) if cmd[0] == "arduino-cli" else out(*cmd, check=False).splitlines()[0]
    except Exception:
        return "unknown"


report = [f"# Test report {tag}", "",
          f"- Commit tested: `{tested_commit}`",
          f"- Firmware version: {core_version}, `trustctl`: {tool_version(trustctl_bin, 'version')}",
          f"- Run: {started:%Y-%m-%d %H:%M} UTC on {platform.system()} {platform.machine()} ({platform.release()})",
          f"- Tools: {tool_version('go', 'version')}; {tool_version('arduino-cli', 'version')}; "
          f"{tool_version('avr-gcc', '--version') if shutil.which('avr-gcc') else 'avr-gcc of the Arduino core'}; Python {platform.python_version()}",
          "- Where: *host* suites need no device and also run in CI on every push; *rig* suites ran on the real dev device "
          "(Arduino Nano, SSD1306 display, button pressed and chip programmed by the rig Nano, tools/build/rig.py) and cannot run in CI.", "",
          "| Suite | Where | Result | Checks passed | Checks failed | Time |", "| --- | --- | --- | --- | --- | --- |"]
for s in allsuites:
    report.append(f"| {s.name} | {s.where} | {s.status} | {s.passed or ''} | {s.failed or ''} | {s.seconds:.0f} s |")
if args.host_only:
    report += ["", "**Dry run: the rig suites were not run.**"]
if args.rig_only:
    report += ["", "**Only the rig suites were run here; the host suites ran in CI.**"]
report += ["", "## Not covered by any automated test", "",
           "- The display, checked by eye on the dev device: text, the large result screens, the progress bar, rotation, inversion and the logo.",
           "- `trustctl` on macOS and the BSDs (cross-compiled only), the zsh and fish completions, and the Windows tray tool's window (its helpers are tested).",
           "- `onboard_usbtiny.sh` against a real USBtiny programmer (tested with a fake avrdude here). The lock bits and the unlocked indicator are tested on the rig (locked_test).",
           "", "## Output", ""]
for s in allsuites:
    report += [f"<details><summary>{s.name}: {s.status}</summary>", "", "```"]
    report += s.lines or [s.status]
    report += ["```", "", "</details>", ""]
text = "\n".join(report) + "\n"

failed = [s for s in allsuites if s.status == "FAILED"]
if no_git:
    path = args.report or os.path.join(ROOT, "dist", f"test-report-{tag}-dryrun.md")
    os.makedirs(os.path.dirname(path), exist_ok=True)
    open(path, "w").write(text)
    print(f"\nreport (dry run): {path}")
    sys.exit(1 if failed else 0)
if failed:
    path = os.path.join(ROOT, "dist", f"test-report-{tag}-FAILED.md")
    os.makedirs(os.path.dirname(path), exist_ok=True)
    open(path, "w").write(text)
    die(f"FAILED: {', '.join(s.name for s in failed)}. Nothing was committed or tagged. Report: {path}")
# the build suite rewrote nothing that is tracked (checked above), but be sure before the commit
if out("git", "status", "--porcelain", "--untracked-files=no"):
    die("the tests changed tracked files:\n" + out("git", "status", "--short", "--untracked-files=no"))

# ---- 4. commit, sign, verify, push -----------------------------------------------------------------------------
report_path = f"docs/test-reports/{tag}.md"
os.makedirs("docs/test-reports", exist_ok=True)
open(report_path, "w").write(text)
out("git", "add", report_path)
out("git", "commit", "-m", f"Release {version}: test report")
out("git", "tag", "-s", tag, "-m", f"trustctl and firmware {version}")
print(out("git", "-c", "gpg.ssh.allowedSignersFile=.github/allowed_signers", "tag", "-v", tag, check=False) or "tag created")
print(f"\nAll suites passed. Commit and signed tag {tag} are local.")
print(f"Pushing starts the release workflow: git push --atomic origin master {tag}")
if input("Push now? [y/N] ").strip().lower() == "y":
    print(out("git", "push", "--atomic", "origin", "master", tag, check=True))
else:
    print("Not pushed. To undo: git tag -d", tag, "&& git reset --hard HEAD~1")
