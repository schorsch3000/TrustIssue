# The rig machine (Raspberry Pi)

The rig (a Nano running `firmware/rigisp`, wired to the device under test, see `CLAUDE.md`) sits on a machine that is on all the
time. That machine tests every new `master` commit by itself: `tools/build/rig_ci.py` polls GitHub, no GitHub runner is installed
and no code from pull requests ever reaches it. Only what CI built for a commit on `master` is run.

## Set up once

As an admin:

```sh
sudo apt install python3 python3-serial avrdude git
sudo useradd -m -s /bin/bash rig            # no sudo for this user
sudo usermod -aG dialout rig                # access to the serial ports
sudo loginctl enable-linger rig             # lets its timer run without a login
```

As the user `rig`:

```sh
git clone https://github.com/schorsch3000/TrustIssue ~/TrustIssue
mkdir -p ~/.config/trustissue-rig ~/.local/state/trustissue-rig ~/.config/systemd/user
# a fine-grained GitHub token for this repository with ONLY "Commit statuses: Read and write":
install -m 600 /dev/stdin ~/.config/trustissue-rig/token     # paste the token, then Ctrl-D
cp ~/TrustIssue/tools/rig/rig-ci.service ~/TrustIssue/tools/rig/rig-ci.timer ~/.config/systemd/user/
systemctl --user daemon-reload
(cd ~/TrustIssue && python3 tools/build/rig.py find)    # must print the rig and the device
systemctl --user enable --now rig-ci.timer
```

## Power

If `/usr/local/bin/rig-power` exists (or the program named in `TRUSTISSUE_RIG_POWER`), the rig is switched on with `rig-power on` only when
a commit is about to be tested (after the downloads), the run waits up to 90 s for the two USB serial ports, and the rig is switched off
with `rig-power off` when the run is over, whatever happens: failed tests, exceptions, SIGTERM from systemd. A run that was killed
hard (power cut, `kill -9`) leaves a marker `~/.local/state/trustissue-rig/powered`, and the next poll switches the rig off first.
The user that runs the timer must be allowed to call `rig-power`. Only one run at a time: `rig_ci.py` holds a lock, so a manual run
and the timer cannot meet at the rig. After updating, copy the changed service file again (`cp ~/TrustIssue/tools/rig/rig-ci.service
~/.config/systemd/user/ && systemctl --user daemon-reload`).

## What it does

Before anything is touched, `rig_ci.py` and `make_release.py` run a preflight (`rig.require_tools`) that names every missing tool at once, with the Debian package. On the Pi that is `avrdude`, `git`, `sed`, `tr`, `tail` and the python module `serial`; no compiler is needed, because the bootloader and firmware come from the download.

Every 5 minutes (`OnCalendar=*:0/5`; a run that is still going makes the next one exit at once, see the lock): asks GitHub which commit the prerelease `nightly` was built from. For a new one it downloads that release's `trustctl`
(for this machine) and firmware archive, checks them against `SHA256SUMS`, checks out the scripts at exactly that commit (it must be on `master`),
provisions the device over ISP with the downloaded bootloader and firmware and runs the rig suites (`make_release.py --rig-only`).
The result is posted on the commit as the status `device-tests`. The report and the log stay on the machine in `~/reports/<commit>.md` and `.log` (`latest.md` and `latest.log` point at the newest run: the files sort by hash, not by time). At most the 100 newest are kept; older ones are deleted at the start of the next poll.
A missing rig gives the status `error` and the commit is tried again at the next poll.

Look at it: `systemctl --user list-timers`, `journalctl --user -u rig-ci`, `tools/build/rig_ci.py --dry-run` (everything except the status; it also re-tests the newest commit).

## Update

`cd ~/TrustIssue && git pull`: the script itself is whatever is in that clone. The tests run from a fresh checkout of the tested commit.
