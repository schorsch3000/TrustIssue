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
tools_check=$(cd ~/TrustIssue && python3 tools/build/rig.py find) && echo "$tools_check"    # rig and device found?
systemctl --user enable --now rig-ci.timer
```

## What it does

Every 5 minutes: asks GitHub which commit the prerelease `nightly` was built from. For a new one it downloads that release's `trustctl`
(for this machine) and firmware archive, checks them against `SHA256SUMS`, checks out the scripts at exactly that commit (it must be on `master`),
provisions the device over ISP with the downloaded bootloader and firmware and runs the rig suites (`make_release.py --rig-only`).
The result is posted on the commit as the status `device-tests`. The report and the log stay on the machine in `~/reports/<commit>.md` and `.log`.
A missing rig gives the status `error` and the commit is tried again at the next poll.

Look at it: `systemctl --user list-timers`, `journalctl --user -u rig-ci`, `tools/build/rig_ci.py --dry-run` (everything except the status; it also re-tests the newest commit).

## Update

`cd ~/TrustIssue && git pull`: the script itself is whatever is in that clone. The tests run from a fresh checkout of the tested commit.
