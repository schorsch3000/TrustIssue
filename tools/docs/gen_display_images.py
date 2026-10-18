#!/usr/bin/env python3
"""Makes the display pictures of the documentation: docs/_static/display/*.png and *.gif.

Every picture comes from the real core firmware, built for the host (firmware/hostsim) and driven
through a scenario: a few commands as the host would send them, and button presses with exact
times. Run from anywhere:

    tools/docs/gen_display_images.py [--only NAME ...] [--out DIR] [--check]

--check regenerates into a temporary directory and fails if a committed picture differs, which
tells that the firmware's screens changed and the pictures are out of date.
"""

import argparse
import calendar
import os
import sys
import tempfile

from PIL import Image

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import displaysim  # noqa: E402

ROOT = displaysim.ROOT
OUT = os.path.join(ROOT, "docs", "_static", "display")
DEMO_LOGOS = sorted(os.path.join(ROOT, "Demo-Logos", f) for f in os.listdir(os.path.join(ROOT, "Demo-Logos")) if f.endswith(".png"))
LOGO = os.path.join(ROOT, "Demo-Logos", "trust_issue_03.png")

# The time all token screens show: 18 Oct 2026, 09:41:00 UTC (a time away from a 30 s boundary).
NOW = calendar.timegm((2026, 10, 18, 9, 41, 0))

SECRET = "JBSWY3DPEHPK3PXP"


def add_key(name, digits=6, period=30):
    """Script lines that store a key the way a person does: request, hold 1.5 s, wait for Done to go."""
    return ["serial ADD %s SHA1 %d %d %s" % (SECRET, digits, period, name), "wait 100", "hold 1500", "wait 1500"]


def set_config(flags):
    return ["serial SETCFG %02x" % flags, "wait 100", "hold 1500", "wait 1500"]


def set_tz(quarter_hours):
    return ["serial SETTZ %d" % quarter_hours, "wait 100", "hold 1500", "wait 1500"]


KEYS = ["GitHub", "Mastodon", "Fastmail", "Gitea"]
CFG_ROTATE, CFG_INVERT, CFG_LOGO, CFG_AUTOWIPE, CFG_VERSION = 1, 2, 4, 8, 16


def keys(count=3):
    lines = []
    for name in KEYS[:count]:
        lines += add_key(name)
    return lines


# name: setup (script lines before the clip, run on a fresh device), pre (options before boot),
# clip (what is recorded), cmd (the host command shown in the caption), from_boot (record from power-up),
# gif (False: stills only). "mark shot NAME" in a clip writes the still NAME.png.
SCENARIOS = {
    "idle-empty": dict(clip=["wait 200", "mark shot idle-empty"], gif=False, pre=["pin set"]),
    "idle-keys": dict(setup=keys(3), clip=["wait 200", "mark shot idle-keys"], gif=False),
    "idle-development": dict(setup=keys(3), pre=["unlocked on", "pin none"],
                             clip=["wait 200", "mark shot idle-development"], gif=False),
    "idle-version": dict(setup=keys(3) + set_config(CFG_VERSION), clip=["wait 200", "mark shot idle-version"],
                         gif=False),
    "idle-sleep": dict(setup=keys(3), clip=["wait 11000", "tap 1500", "wait 100"], max_gap=2000,
                       note="The display turns off after 10 s; a press wakes it."),
    "idle-logo": dict(setup=keys(3) + set_config(CFG_LOGO), pre=["logo " + "{logo}", "contrast 0x20 0x00"],
                      from_boot=True, max_gap=2500, all_logos=True,
                      clip=["boot", "wait 5200", "wait 10500", "wait 30500", "wait 120500", "wait 500"],
                      note="Power-up: logo for 5 s, the idle screen, then the logo at full and dimmed contrast, then off."),
    "add": dict(cmd="echo SECRET | trustctl add GitHub",
                clip=["serial ADD %s SHA1 6 30 GitHub" % SECRET, "wait 400", "hold 1500", "wait 1800"],
                shot_at={"add-confirm": 300, "result-done": 2300}),
    "del": dict(setup=keys(3), cmd="trustctl del Gitea",
                clip=["serial DEL 1", "wait 400", "hold 1500", "wait 1800"], shot_at={"del-confirm": 300}),
    "token": dict(setup=keys(3) + set_tz(8), cmd="trustctl token GitHub",
                  clip=["serial TOKEN 0 %d" % NOW, "wait 400", "hold 1500", "wait 5800"],
                  target_zone={"target-zone": 1850}, shot_at={"token-confirm": 300, "token-code": 1500 + 400 + 600 + 300}),
    "token-longname": dict(setup=add_key("Contoso: alice@example.com") + set_tz(8),
                           clip=["serial TOKEN 0 %d" % NOW, "wait 400", "mark shot token-longname"], gif=False),
    "tz-set": dict(setup=keys(1), cmd="trustctl tz set +2", clip=["serial SETTZ 8", "wait 400", "hold 1500", "wait 1800"], shot_at={"tz-confirm": 300}),
    "config-rotate": dict(setup=keys(3), cmd="trustctl config set rotate=on",
                          clip=["serial SETCFG 01", "wait 400", "hold 1500", "wait 1800", "wait 500"]),
    "config-invert": dict(setup=keys(3), cmd="trustctl config set invert=on",
                          clip=["serial SETCFG 02", "wait 400", "hold 1500", "wait 1800", "wait 500"]),
    "config-flags": dict(setup=keys(3) + set_config(CFG_ROTATE | CFG_LOGO), cmd="trustctl config set show-version=on rotate=off",
                         clip=["serial SETCFG 14", "wait 400", "mark shot config-flags"], gif=False),
    "result-denied": dict(setup=keys(3), cmd="trustctl token GitHub",
                          clip=["serial TOKEN 0 %d" % NOW, "wait 400", "hold 600", "wait 1500"], shot_at={"result-denied": 1300}),
    "result-too-long": dict(setup=keys(3), cmd="trustctl token GitHub",
                            clip=["serial TOKEN 0 %d" % NOW, "wait 400", "hold 2600", "wait 1500"], shot_at={"result-too-long": 2900}),
    "result-timeout": dict(setup=keys(3), cmd="trustctl token GitHub", max_gap=1200,
                           clip=["serial TOKEN 0 %d" % NOW, "wait 30500", "wait 1500"], shot_at={"result-timeout": 30800}),
    "wipe": dict(setup=keys(4), cmd="trustctl wipe",
                 clip=["serial WIPE", "wait 400", "hold 5500", "wait 2500"], shot_at={"wipe-confirm": 300}),
    "wipe-denied": dict(setup=keys(4), cmd="trustctl wipe",
                        clip=["serial WIPE", "wait 400", "hold 1500", "wait 1500"]),
    "autowipe": dict(setup=keys(3) + set_config(CFG_AUTOWIPE),
                     max_gap=1000,
                     clip=["hold 10500", "wait 4300", "tap 300", "tap 300", "hold 2200", "wait 3000"],
                     shot_at={"autowipe-hold": 9000, "autowipe-countdown": 12500, "autowipe-gesture": 15500},
                     note="Without a host: hold 10 to 11 s, wait for the countdown, tap, tap, hold 2 s."),
}


def gate(pin=None, hold_until=9800):
    """The bootloader timeline up to the end of the gesture; pin: None (no PIN set), or the PIN the host sends."""
    lines = ["at 0 cap Button held while the device resets", "at 0 press",
             "at 1500 cap Let go: the bar drains (about 4 s), do not touch the button", "at 1500 release",
             "at 5950 cap Now: tap, tap, then press again and hold until the bar is full",
             "at 6500 press", "at 6650 release", "at 6900 press", "at 7050 release", "at 7350 press",
             "at %d release" % hold_until]
    return lines


BOOT_SCENARIOS = {
    "flash-gate": dict(cmd="trustctl flash", limit=12000, max_gap=1500,
                       script=gate() + ["at 9800 cap The update mode is open: trustctl now writes the firmware",
                                        "limit 11500"]),
    "flash-gate-pin": dict(cmd="trustctl flash --pin", pre=["pin set"], max_gap=1500,
                           script=gate() + ["at 9850 cap The device asks for the update PIN (it waits 8 s)",
                                            "at 10200 uart \\x02", "at 10400 uart 4711\\n",
                                            "at 10450 cap A bar runs for 7 s whether the PIN is right or not",
                                            "at 17500 cap Right PIN: the update mode opens", "limit 19000"]),
    "flash-wrong-pin": dict(cmd="trustctl flash --pin", pre=["pin set"], max_gap=1500,
                            script=gate() + ["at 9850 cap Wrong PIN: the same screen, the same 7 s bar",
                                             "at 10200 uart \\x02", "at 10400 uart 0000\\n",
                                             "at 17500 cap ... and then the application simply starts", "limit 19000"]),
    "flash-aborted": dict(cmd="trustctl flash", max_gap=1500,
                          script=["at 0 cap Button held while the device resets", "at 0 press", "at 1500 release",
                                  "at 3000 cap A press while the bar drains stops the gate", "at 3000 press",
                                  "at 3200 release", "limit 6000"]),
    "flash-mistake": dict(cmd="trustctl flash", max_gap=1500,
                          script=gate(hold_until=8350)[:-1] + ["at 8350 release",
                                                               "at 8350 cap Released too early: the bar goes back, start over",
                                                               "limit 10500"]),
}


FONT_DIR = os.path.join(ROOT, "trustctl", "internal", "fonts")
FONTS = ["victoriamedium8", "victoriabold8", "chroma48medium8", "artossans8", "artosserif8", "torussansbold8", "5x8"]
FONT_SAMPLE = ["Il1 O0oQ 5S 8B", "GitHub paypa1", "rn m vv cl .:-", "AaBbCcDdEeFfGgHh"]  # what calibrate font shows


def text_frame(font, rows):
    """The display with rows of text in a 772 byte font: the calibration firmware's FONTTEXT, as a frame."""
    frame = bytearray(512)
    for row, text in enumerate(rows):
        for column in range(16):
            c = ord(text[column]) if column < len(text) else 32
            glyph = c - 32 if 32 <= c < 128 else ord("?") - 32
            frame[row * 128 + column * 8:row * 128 + column * 8 + 8] = font[4 + glyph * 8:4 + glyph * 8 + 8]
    return frame


def transformed(frame, flip=False, invert=False):
    """What the controller shows for the rotate and invert settings (180 degrees, swapped pixels)."""
    out = bytearray(512)
    for y in range(32):
        for x in range(128):
            sx, sy = (127 - x, 31 - y) if flip else (x, y)
            lit = (frame[sy // 8 * 128 + sx] >> (sy % 8) & 1) != invert
            out[y // 8 * 128 + x] |= lit << (y % 8)
    return out


def calibration_stills(outdir):
    """The pictures of `trustctl calibrate`: they come from the calibration firmware, which only draws
    text with a font and raw bitmaps, so they are drawn here from the same font and logo data."""
    made = []

    def save(name, frame, contrast=displaysim.FULL_CONTRAST):
        displaysim.write_png(os.path.join(outdir, name + ".png"), displaysim.State(frame.hex(), contrast))
        made.append(name + ".png")

    chroma = open(os.path.join(FONT_DIR, "chroma48medium8.bin"), "rb").read()
    home = text_frame(chroma, ["Trust Issue", "calibration"])
    logo = logo_bytes()
    save("calibrate-home", home)
    save("calibrate-rotate", transformed(home, flip=True))
    save("calibrate-invert", transformed(home, invert=True))
    save("calibrate-logo", logo)
    save("calibrate-logo-invert", transformed(logo, invert=True))
    save("calibrate-contrast-full", logo, 0x20)
    save("calibrate-contrast-dim", logo, 0x00)
    for name in FONTS:
        font = open(os.path.join(FONT_DIR, name + ".bin"), "rb").read()
        save("calibrate-font-" + name, text_frame(font, FONT_SAMPLE))
    return made


def logo_bytes(path=LOGO):
    """A logo as the 512 bytes of the display (the rule of trustctl: black is off, anything else on)."""
    image = Image.open(path).convert("RGBA")
    data = bytearray(512)
    for y in range(32):
        for x in range(128):
            r, g, b, a = image.getpixel((x, y))
            if (r or g or b) and a:
                data[(y // 8) * 128 + x] |= 1 << (y % 8)
    return bytes(data)


def run_boot_scenario(name, spec, outdir):
    script = list(spec.get("pre", [])) + ["limit %d" % spec.get("limit", 60000)] + spec["script"] + ["run"]
    events = displaysim.run_script(script, binary=displaysim.BOOTSIM)
    states, _ = displaysim.timeline(events)
    frames = displaysim.write_gif(os.path.join(outdir, name + ".gif"), states, spec.get("cmd"),
                                  max_gap=spec.get("max_gap", 1500))
    return ["%s.gif (%d frames)" % (name, frames)]


def run_scenario(name, spec, outdir, workdir):
    pre = [line.replace("{logo}", os.path.join(workdir, "logo.bin")) for line in spec.get("pre", [])]
    eeprom = os.path.join(workdir, name + ".eeprom")
    setup = spec.get("setup", [])
    if setup:
        displaysim.run_script(pre + ["boot", "wait 100"] + setup + ["wait 200", "eeprom_save " + eeprom])
        load = ["eeprom_load " + eeprom]
    else:
        load = []
    if spec.get("from_boot"):
        script = pre + load + ["record on"] + spec["clip"]
    else:
        script = pre + load + ["boot", "wait 100", "record on"] + spec["clip"]
    if spec.get("all_logos"):  # the same clip once per demo logo, one after the other
        states = []
        for number, path in enumerate(DEMO_LOGOS, 1):
            with open(os.path.join(workdir, "logo.bin"), "wb") as f:
                f.write(logo_bytes(path))
            caption = "mark cap Demo logo %d of %d (%s)" % (number, len(DEMO_LOGOS), os.path.basename(path))
            clip = displaysim.timeline(displaysim.run_script(pre + load + ["record on", caption] + spec["clip"]))[0]
            offset = states[-1][0] + displaysim.FRAME_MS if states else 0
            states += [(t + offset, state) for t, state in clip]
        stills = {}
    else:
        events = displaysim.run_script(script)
        states, stills = displaysim.timeline(events)
    made = []
    for shot, index in stills.items():
        if shot in spec.get("shot_at", {}):
            continue
        displaysim.write_png(os.path.join(outdir, shot + ".png"), states[index][1])
        made.append(shot + ".png")
    for shot, ms in spec.get("shot_at", {}).items():
        index = min(len(states) - 1, ms // displaysim.FRAME_MS)
        displaysim.write_png(os.path.join(outdir, shot + ".png"), states[index][1])
        made.append(shot + ".png")
    for name_, ms in spec.get("target_zone", {}).items():
        displaysim.write_target_zone(os.path.join(outdir, name_ + ".png"), states[min(len(states) - 1, ms // displaysim.FRAME_MS)][1])
        made.append(name_ + ".png")
    if spec.get("gif", True):
        frames = displaysim.write_gif(os.path.join(outdir, name + ".gif"), states, spec.get("cmd"),
                                      max_gap=spec.get("max_gap", 1500))
        made.append("%s.gif (%d frames)" % (name, frames))
    return made


def generate(outdir, only):
    os.makedirs(outdir, exist_ok=True)
    displaysim.build()
    with tempfile.TemporaryDirectory() as workdir:
        with open(os.path.join(workdir, "logo.bin"), "wb") as f:
            f.write(logo_bytes())
        for name, spec in SCENARIOS.items():
            if only and name not in only:
                continue
            print(name, ", ".join(run_scenario(name, spec, outdir, workdir)))
        if not only or "calibrate" in only:
            print("calibrate", ", ".join(calibration_stills(outdir)))
        for name, spec in BOOT_SCENARIOS.items():
            if only and name not in only:
                continue
            print(name, ", ".join(run_boot_scenario(name, spec, outdir)))


def same_pictures(a, b):
    """Equal in every frame and duration; the encoded bytes may differ between Pillow versions."""
    def content(path):
        image = Image.open(path)
        out = []
        for i in range(getattr(image, "n_frames", 1)):
            image.seek(i)
            out.append((image.convert("RGB").tobytes(), image.info.get("duration")))
        return out
    return content(a) == content(b)


def main():
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--only", nargs="*", default=[], help="scenario names")
    parser.add_argument("--out", default=OUT)
    parser.add_argument("--check", action="store_true", help="fail if the committed pictures are out of date")
    args = parser.parse_args()
    if not args.check:
        generate(args.out, args.only)
        return
    with tempfile.TemporaryDirectory() as tmp:
        generate(tmp, args.only)
        stale = [f for f in sorted(os.listdir(tmp))
                 if not os.path.exists(os.path.join(args.out, f)) or not same_pictures(os.path.join(tmp, f), os.path.join(args.out, f))]
        if stale:
            sys.exit("out of date: " + ", ".join(stale))
        print("display pictures are up to date")


if __name__ == "__main__":
    main()
