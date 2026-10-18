# Tests

## Without hardware

- `firmware/test/run.sh`: host tests of the portable core (SHA-1, HMAC and TOTP vectors, Base32, storage,
  protocol, time).
- `cd trustctl && go test ./...`: the host tool against a simulated device.
- `tools/build/smoke_dummy.py`: a built `trustctl` against its simulated device.
- `tools/build/test_trustctl_menu.sh`, `tools/build/test_onboard_usbtiny.sh`, `tools/user/trustctl-tray/test.sh`.
- `tools/docs/gen_display_images.py --check`: the display pictures of this documentation are up to date.

CI (`.github/workflows`) runs all of this on every push to `master`, and checks that a firmware build is
byte-identical to the images committed in `trustctl/internal/firmware`.

## With the rig

A second Nano running `firmware/rigisp/rigisp.ino` is both the ISP programmer and the finger that presses
the device's button: its `D4` goes through 1 kOhm to the device's `A3`, `D10` to `RST`, `D11` to `D13` to
`D11` to `D13`, the grounds are shared, nothing on 5 V. `tools/build/rig.py` finds the rig and the device
and provisions the device over ISP. The suites, all of which pass on the dev rig:

| Script                                  | What it tests                                                                |
| --------------------------------------- | ---------------------------------------------------------------------------- |
| `tools/build/core_test.py [--slow]`     | the core firmware: protocol, windows, results                                |
| `tools/build/autowipe_test.py`          | the autonomous wipe, including wrong gestures                                |
| `tools/build/boot_test.py [--pin]`      | the bootloader gate, the PIN, refused writes                                 |
| `tools/build/trustctl_e2e.py`           | `trustctl` against the real device                                           |
| `tools/build/trustctl_flash_e2e.py`     | `trustctl flash` and `calibrate`                                             |
| `tools/build/locked_test.py`            | the finished-device state (final lock bits)                                  |

`tools/build/make_release.py` runs all of them. In the project's own setup the rig sits on a Raspberry Pi
that tests every `master` commit by itself (`tools/build/rig_ci.py`, `tools/rig/README.md`) and posts the
commit status `device-tests`.

Some timings are worth knowing when you write tests: the measured hold times are 2 to 16 ms above the
commanded ones, so keep at least 100 ms of margin to the window edges, and test timestamps must not sit near
a 30 second boundary because the device adds elapsed time.
