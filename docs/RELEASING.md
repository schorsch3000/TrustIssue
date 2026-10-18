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

The rig sits on the rig machine (a Raspberry Pi, `tools/rig/README.md`), which tests every `master` commit by itself and posts the
status `device-tests` on it. A release tags a commit it has already tested, so the tagged commit is exactly what was tested.

1. Bump `VERSION` in `firmware/core/core.ino` (and the expectation in `tools/build/core_test.py`) if the firmware version
   changes, run `tools/build/embed_firmware.sh`, commit and push. The tag `vX.Y.Z` must equal the firmware version.
2. Wait until the commit shows green CI and `device-tests: success` on GitHub (about 15 minutes).
3. `export TRUSTISSUE_RIG_HOST=trustissue-test@homedisplay` (once, e.g. in your shell profile), then `tools/build/make_release.py X.Y.Z`
   checks both, copies that commit's report from the rig machine with `scp`, makes the signed tag on `HEAD`, verifies it and asks
   before it pushes the tag. Then it commits the report as `docs/test-reports/vX.Y.Z.md` (after the tag, so the tag stays on the
   tested commit) and asks before it pushes `master`.
4. The tag starts `.github/workflows/release.yml`: it checks the tag (signature, on master, version, `device-tests: success`
   for the tagged commit), runs the tests again, builds, attests and publishes the release with the commit list and a link to
   the report.

With a rig attached to your own machine instead, `tools/build/make_release.py X.Y.Z --local` (or without the variable) runs everything here (the tests of CI, the
rig tests) and commits the report before the tag (`--dry-run`: no commit, tag or push; `--host-only`: no rig). The workflow
accepts that too, if the tagged commit only adds the report to a tested commit.

## CI

- `ci.yml`: every push to master and pull request: `build.yml` (tests, firmware build check, trustctl for all platforms, the
  Windows tray tool, archives). On master it then replaces the prerelease `nightly`.
- `release.yml`: tags `vX.Y.Z`.
- Third-party actions are pinned by commit SHA. Settings worth enabling on GitHub: a tag ruleset for `v*` (nobody may
  delete or move them), branch protection on master, "Private vulnerability reporting".
