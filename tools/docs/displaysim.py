"""Runs the real core firmware on the host (firmware/hostsim) and draws what its display shows.

Used by gen_display_images.py to make the pictures of the documentation. The firmware is not
re-implemented here: hostsim builds core.ino unchanged against a mock display, and this module
only turns the recorded display contents into PNG and GIF files.
"""

import os
import subprocess
import sys
from dataclasses import dataclass, field

from PIL import Image, ImageChops, ImageDraw, ImageFont

ROOT = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
HOSTSIM_DIR = os.path.join(ROOT, "firmware", "hostsim")
HOSTSIM = os.path.join(HOSTSIM_DIR, "build", "hostsim")  # the core firmware
BOOTSIM = os.path.join(HOSTSIM_DIR, "build", "bootsim")  # the bootloader

SCALE = 6  # screen pixels per display pixel
BEZEL = 24
STRIP = 70  # height of the caption strip under the display
FRAME_MS = 40  # a GIF frame every 40 ms (25 per second) while something changes
BACKGROUND = (232, 232, 228)  # off-gray page behind the display
FRAME = (22, 24, 28)  # the dark edge of the display module
PIXEL_OFF = (6, 8, 12)
PIXEL_ON = (214, 236, 255)  # the usual blue-white of a small OLED
TEXT = (38, 40, 46)
MUTED = (104, 108, 116)
ACCENT = (41, 128, 185)  # the blue of the documentation theme: captions
BUTTON_RED = (214, 36, 30)  # the red button of the device
FULL_CONTRAST = 0x20  # the contrast the project uses for "full"; anything above looks the same


def build():
    subprocess.run([os.path.join(HOSTSIM_DIR, "build.sh")], check=True)
    subprocess.run([os.path.join(HOSTSIM_DIR, "build_boot.sh")], check=True)


def run_script(lines, extra_env=None, binary=HOSTSIM):
    """Runs a hostsim (or, with binary=BOOTSIM, bootsim) script; returns the (ms, kind, payload) events."""
    if not os.path.exists(binary):
        build()
    env = dict(os.environ, **(extra_env or {}))
    proc = subprocess.run([binary], input="\n".join(lines) + "\n", capture_output=True, text=True, env=env)
    if proc.returncode != 0:
        sys.exit("hostsim failed: " + proc.stderr)
    events = []
    for line in proc.stdout.splitlines():
        ms, kind, payload = (line + " ").split(" ", 2)
        events.append((int(ms), kind, payload.rstrip()))
    return events


@dataclass
class State:
    """Everything the picture depends on at one moment."""

    frame: str = "00" * 512
    contrast: int = FULL_CONTRAST
    button: bool = False
    pressed_at: int = 0
    reply: str = ""
    caption: str = ""


def decode_frame(hexdata):
    """512 bytes of SSD1306 page layout to a 128x32 image (L mode, 255 = lit)."""
    data = bytes.fromhex(hexdata)
    image = Image.new("L", (128, 32), 0)
    pixels = image.load()
    for page in range(4):
        for x in range(128):
            byte = data[page * 128 + x]
            for bit in range(8):
                if byte >> bit & 1:
                    pixels[x, page * 8 + bit] = 255
    return image


def clip_events(events):
    """Events from the last 'record' mark on."""
    start = 0
    for i, (ms, kind, payload) in enumerate(events):
        if kind == "M" and payload == "record":
            start = i
    return events[start:]


def timeline(events):
    """Resamples the events into (ms, State) steps of FRAME_MS, with the still marks.

    Returns (states, stills) where stills maps a name to the index of a state.
    """
    events = clip_events(events)
    t0 = events[0][0]
    end = events[-1][0]
    state = State()
    frames = {}  # ms -> decoded image cache by hex
    states = []
    stills = {}
    index = 0
    for t in range(t0, end + FRAME_MS, FRAME_MS):
        while index < len(events) and events[index][0] <= t:
            ms, kind, payload = events[index]
            if kind == "F":
                contrast, hexdata = payload.split(" ")
                state.contrast = int(contrast)
                state.frame = hexdata
            elif kind == "B":
                state.button = payload == "1"
                state.pressed_at = ms
            elif kind == ">":
                state.reply = payload
            elif kind == "<":
                state.reply = ""
            elif kind == "M" and payload.startswith("cap "):
                state.caption = payload[4:]
            elif kind == "M" and payload.startswith("shot "):
                stills[payload[5:]] = len(states)
            index += 1
        states.append((t - t0, State(state.frame, state.contrast, state.button, state.pressed_at - t0, state.reply, state.caption)))
    return states, stills


def brightness(contrast):
    """Dim phase: contrast 0 is not dark but clearly dimmer than the full 0x20."""
    if contrast > 0xFF:  # display off
        return 0.0
    return 0.3 + 0.7 * min(contrast, FULL_CONTRAST) / FULL_CONTRAST


def _grid_mask():
    cell = Image.new("L", (SCALE, SCALE), 0)
    ImageDraw.Draw(cell).rectangle((0, 0, SCALE - 2, SCALE - 2), fill=255)
    mask = Image.new("L", (128 * SCALE, 32 * SCALE))
    for y in range(32):
        for x in range(128):
            mask.paste(cell, (x * SCALE, y * SCALE))
    return mask


_MASK = None


def draw_display(frame_hex, contrast, with_bezel=True):
    global _MASK
    if _MASK is None:
        _MASK = _grid_mask()
    lit = decode_frame(frame_hex).resize((128 * SCALE, 32 * SCALE), Image.NEAREST)
    mask = ImageChops.multiply(lit, _MASK)
    level = brightness(contrast)
    on = tuple(round(PIXEL_OFF[i] + (PIXEL_ON[i] - PIXEL_OFF[i]) * level) for i in range(3))
    screen = Image.composite(Image.new("RGB", lit.size, on), Image.new("RGB", lit.size, PIXEL_OFF), mask)
    if not with_bezel:
        return screen
    image = Image.new("RGB", (screen.width + 2 * BEZEL, screen.height + 2 * BEZEL), BACKGROUND)
    ImageDraw.Draw(image).rounded_rectangle((BEZEL - 12, BEZEL - 12, BEZEL + screen.width + 11, BEZEL + screen.height + 11),
                                            radius=10, fill=FRAME)
    image.paste(screen, (BEZEL, BEZEL))
    return image


def _font(size):
    for path in ("/usr/share/fonts/truetype/dejavu/DejaVuSansMono.ttf",
                 "/usr/share/fonts/dejavu/DejaVuSansMono.ttf", "DejaVuSansMono.ttf"):
        try:
            return ImageFont.truetype(path, size)
        except OSError:
            pass
    return ImageFont.load_default()


def draw_picture(state, t, command=None):
    """The display with a caption strip: the command on the host, the reply, and the button."""
    image = draw_display(state.frame, state.contrast)
    width = image.width
    picture = Image.new("RGB", (width, image.height + STRIP), BACKGROUND)
    picture.paste(image, (0, 0))
    d = ImageDraw.Draw(picture)
    y = image.height + 4
    if command:
        d.text((BEZEL, y), "$ " + command, font=_font(17), fill=TEXT)
    if state.caption:
        d.text((BEZEL, y + 28), state.caption, font=_font(17), fill=ACCENT)
    elif command and state.reply:
        d.text((BEZEL, y + 28), "< " + state.reply, font=_font(17), fill=MUTED)
    # the button: a ring, filled while pressed, with the hold time next to it
    cx, cy, r = width - BEZEL - 150, y + 11, 11
    d.ellipse((cx - r, cy - r, cx + r, cy + r), outline=MUTED, width=2, fill=BUTTON_RED if state.button else BACKGROUND)
    label = "button"
    if state.button:
        label = "held %.1f s" % ((t - state.pressed_at) / 1000)
    d.text((cx + r + 12, cy - 10), label, font=_font(17), fill=BUTTON_RED if state.button else MUTED)
    return picture


def _palette():
    """One fixed palette for every GIF frame, so colors cannot flicker between frames."""
    colors = [BACKGROUND, PIXEL_OFF, TEXT, MUTED, ACCENT, BUTTON_RED]
    for level in range(0, 0x21, 4):
        b = brightness(level)
        colors.append(tuple(round(PIXEL_OFF[i] + (PIXEL_ON[i] - PIXEL_OFF[i]) * b) for i in range(3)))
    sheet = Image.new("RGB", (len(colors) * 8, 8))
    for i, color in enumerate(colors):
        sheet.paste(color, (i * 8, 0, i * 8 + 8, 8))
    # antialiased text needs the shades in between
    inks = (TEXT, BUTTON_RED, ACCENT)
    both = Image.new("RGB", (sheet.width + 256 * len(inks), 8))
    both.paste(sheet, (0, 0))
    for k, ink in enumerate(inks):
        for x in range(256):
            shade = tuple(round(BACKGROUND[i] + (ink[i] - BACKGROUND[i]) * x / 255) for i in range(3))
            both.paste(shade, (sheet.width + 256 * k + x, 0, sheet.width + 256 * k + x + 1, 8))
    return both.quantize(colors=128, dither=Image.NONE)


def write_gif(path, states, command=None, max_gap=1500, tail=1500, speed=1.0):
    """Writes the clip as an animated GIF. A picture that stays the same is shown once, at most
    max_gap ms long (so waiting for a timeout does not make a huge file); the last one stays tail ms."""
    palette = _palette()
    frames, durations, keys = [], [], []
    for t, state in states:
        label = int((t - state.pressed_at) / 100) if state.button else None
        key = (state.frame, state.contrast, state.button, label, state.reply, state.caption)
        if keys and keys[-1] == key:
            durations[-1] += FRAME_MS
            continue
        keys.append(key)
        frames.append(draw_picture(state, t, command).quantize(palette=palette, dither=Image.NONE))
        durations.append(FRAME_MS)
    durations = [max(20, round(min(d, max_gap) / speed / 10) * 10) for d in durations]
    durations[-1] = tail
    frames[0].save(path, save_all=True, append_images=frames[1:], duration=durations, loop=0, optimize=True,
                   disposal=1)
    return len(frames)


def write_png(path, state):
    """A still: only the display, no caption."""
    draw_display(state.frame, state.contrast).save(path, optimize=True)


GREEN = (28, 135, 68)
RED = (192, 58, 48)


def write_target_zone(path, state, window_start=0.5):
    """A still of the progress bar with the accept window marked under the display: left of the window
    is too early, the checkerboard is the target, and the end of the bar is the end of the window."""
    image = draw_display(state.frame, state.contrast)
    width, height = image.size
    labels = 78
    picture = Image.new("RGB", (width, height + labels), BACKGROUND)
    picture.paste(image, (0, 0))
    d = ImageDraw.Draw(picture)
    left, right = BEZEL, width - BEZEL
    split = left + round((right - left) * window_start)
    y = height + 6
    for x0, x1, color, text in ((left, split, RED, "let go here: too early, denied"),
                                (split, right, GREEN, "let go here: accepted")):
        d.line((x0 + 2, y, x1 - 2, y), fill=color, width=4)
        d.line((x0 + 2, y, x0 + 2, y - 8), fill=color, width=4)
        d.line((x1 - 2, y, x1 - 2, y - 8), fill=color, width=4)
        box = d.textbbox((0, 0), text, font=_font(17))
        d.text(((x0 + x1) // 2 - (box[2] - box[0]) // 2, y + 12), text, font=_font(17), fill=color)
    d.text((left, y + 40), "0 s", font=_font(15), fill=MUTED)
    d.text((split - 14, y + 40), "1 s", font=_font(15), fill=MUTED)
    d.text((right - 24, y + 40), "2 s", font=_font(15), fill=MUTED)
    picture.save(path, optimize=True)
