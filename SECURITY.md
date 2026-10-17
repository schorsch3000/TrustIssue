# Security policy

Trust Issues exists to keep TOTP secrets away from a computer you don't trust, so security reports are welcome and taken seriously.

## Reporting a vulnerability

Please do not open a public issue for a vulnerability.

Use GitHub's private reporting instead: go to the [Security tab](https://github.com/schorsch3000/TrustIssue/security) of this repository and choose "Report a vulnerability". Only the maintainer sees the report.

Helpful in a report: the firmware and `trustctl` version (`trustctl version`), whether the device had the final lock bits and an update PIN, what an attacker needs (host malware, brief physical access, or more), and steps to reproduce. A proof of concept against the simulator (`trustctl --device DUMMY`) is fine where the issue does not depend on hardware.

This is a hobby project run by one person. Expect an acknowledgement within about a week and a fix or an explanation after that; there is no bounty. Reporters are credited in the release notes unless they prefer not to be.

## Supported versions

Only the latest release (firmware and `trustctl` are released together and are tightly coupled). Fixes are not backported.

## What counts as a vulnerability

The threat model is in [README.md](README.md) (sections "Security" and "Physical access") and in more detail in [CLAUDE.md](CLAUDE.md). In short, these are in scope:

- Anything that lets a compromised host read, export or infer a stored secret.
- Anything that lets a compromised host add, delete, wipe or obtain a token without the user's button confirmation on the device, or with a confirmation screen that shows something other than what is executed (name, date, time, timezone offset).
- Bypassing the button-gated bootloader from the host, or flashing firmware without the button gesture or without the update PIN.
- An oracle that lets a host or an attacker guess the update PIN faster than one guess per full gate (about 15 s).
- Leaking the PIN, secrets or tokens through `trustctl`, its daemon socket, its config or profile files, or the tray and menu helpers.
- Flaws in the final lock bit setup that make the EEPROM readable without destroying it.

## Known limits (not vulnerabilities)

These are documented design limits:

- Attacks on the chip itself: decapping, voltage or clock glitching, probing, cold boot.
- A patient attacker with physical access guessing a short update PIN. USB and the button are reachable by design, so a laptop plus a servo on the button can try PINs unattended. PIN length against the time the device is out of your sight is the only defence.
- A user tricked into performing the update gesture and entering the PIN on an attacker's prompt.
- TOTP itself stays phishable: the token passes through the host.
- A device without the final lock bits (`NOT LOCKED` on the idle screen) is a development device; ISP can read its EEPROM.
- There is no key export by design. Keep offline backups of your secrets.

Reports that improve on these limits (for example a cheap way to make the PIN guessing detectable) are still welcome as ideas, just not as vulnerabilities.
