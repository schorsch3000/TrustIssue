# Releasing

## One time: a signing key

Release tags are signed with an SSH key, and the release workflow refuses a tag that is not signed by a key listed in
`.github/allowed_signers`.

```sh
ssh-keygen -t ed25519 -C "trustissue release signing" -f ~/.ssh/id_ed25519_trustissue_release   # use a passphrase
git config user.signingkey ~/.ssh/id_ed25519_trustissue_release.pub
git config gpg.format ssh
```

Add the public key to `.github/allowed_signers` (`EMAIL ssh-ed25519 AAAA...`, one line per key, EMAIL is the address
of your git commits) and, so GitHub shows "Verified", under Settings, SSH and GPG keys, New SSH key, type "Signing key".

## Every release

1. Bump `VERSION` in `firmware/core/core.ino` (and the expectation in `tools/build/core_test.py`) if the firmware version
   changes, run `tools/build/embed_firmware.sh`, commit. The tag `vX.Y.Z` must equal the firmware version.
2. Connect the **empty dev device** (Nano with the button) and the rig (Uno). Never a production device: the script refuses a
   device that holds keys, is not on UTC+0 or does not have the default settings.
3. `tools/build/make_release.py X.Y.Z --nano /dev/ttyUSB0 --uno /dev/ttyUSB1`
   runs the tests of CI, then the rig tests, writes `docs/test-reports/vX.Y.Z.md`, commits it, creates and verifies the signed
   tag and asks before it pushes master and the tag together.
4. The tag starts `.github/workflows/release.yml`: it checks the tag (signature, on master, version, report present), runs
   the tests again, builds, attests and publishes the release with the commit list and the report.

`tools/build/make_release.py X.Y.Z --host-only` is a dry run without the rig; it commits and tags nothing.

## CI

- `ci.yml`: every push to master and pull request: `build.yml` (tests, firmware build check, trustctl for all platforms, the
  Windows tray tool, archives). On master it then replaces the prerelease `nightly`.
- `release.yml`: tags `vX.Y.Z`.
- Third-party actions are pinned by commit SHA. Settings worth enabling on GitHub: a tag ruleset for `v*` (nobody may
  delete or move them), branch protection on master, "Private vulnerability reporting".
