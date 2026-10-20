#!/usr/bin/env python3
"""Packs the files in dist/ (from tools/build/release.sh) into the release archives.

  tools/build/package.py VERSION [--tray PATH.exe] [--bootloader PATH.hex] [--out DIR]

One archive per platform (.zip for Windows, .tar.gz for the others) holding trustctl, trustctl-import
(from dist/import/), the user tools (tools/user), README.md, LICENSE and THIRD_PARTY_LICENSES.md, plus a firmware
archive with core.hex, calibration.hex and boot.hex, and SHA256SUMS over everything in the
output directory. The archives are reproducible: sorted entries, fixed timestamps (the
commit time), fixed permissions.
"""
import argparse
import gzip
import hashlib
import io
import os
import subprocess
import sys
import tarfile
import zipfile

ROOT = os.path.normpath(os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", ".."))

parser = argparse.ArgumentParser()
parser.add_argument("version")
parser.add_argument("--tray", help="the built trustctl-tray.exe (goes into the Windows archives)")
parser.add_argument("--bootloader", help="boot.hex (goes into the firmware archive)")
parser.add_argument("--out", default=os.path.join(ROOT, "dist", "archives"))
args = parser.parse_args()

try:
    epoch = int(subprocess.check_output(["git", "-C", ROOT, "log", "-1", "--format=%ct"], text=True).strip())
except Exception:
    epoch = 0
os.makedirs(args.out, exist_ok=True)

DOCS = ["README.md", "LICENSE", "THIRD_PARTY_LICENSES.md"]
SKIP_DIRS = {"__pycache__", "tests"}


def read(path):
    with open(path, "rb") as f:
        return f.read()


def user_tools(osname):
    """(archive path, mode, content) for everything in tools/user."""
    base = os.path.join(ROOT, "tools", "user")
    for dirpath, dirnames, filenames in os.walk(base):
        dirnames[:] = sorted(d for d in dirnames if d not in SKIP_DIRS)
        rel_dir = os.path.relpath(dirpath, base)
        if rel_dir.split(os.sep)[0] == "trustctl-tray" and osname != "windows":
            continue  # the tray tool is for Windows
        if rel_dir.split(os.sep)[0] == "trustctl-menubar" and osname != "darwin":
            continue  # the menu bar tool is for macOS (sources only: there is no macOS runner to build it)
        for name in sorted(filenames):
            if name.endswith((".pyc", ".exe", ".gitignore")) or name == "test.sh":
                continue
            full = os.path.join(dirpath, name)
            yield os.path.join("tools", "user", os.path.relpath(full, base)), os.stat(full).st_mode & 0o111 and 0o755 or 0o644, read(full)


def write_tar(path, top, entries):
    raw = io.BytesIO()
    with gzip.GzipFile(fileobj=raw, mode="wb", mtime=0, compresslevel=9) as gz:
        with tarfile.open(fileobj=gz, mode="w", format=tarfile.PAX_FORMAT) as tar:
            for name, mode, data in sorted(entries):
                info = tarfile.TarInfo(f"{top}/{name}")
                info.size, info.mode, info.mtime = len(data), mode, epoch
                info.uid = info.gid = 0
                info.uname = info.gname = ""
                tar.addfile(info, io.BytesIO(data))
    with open(path, "wb") as f:
        f.write(raw.getvalue())


def write_zip(path, top, entries):
    with zipfile.ZipFile(path, "w", zipfile.ZIP_DEFLATED, compresslevel=9) as z:
        for name, mode, data in sorted(entries):
            info = zipfile.ZipInfo(f"{top}/{name}", date_time=max((1980, 1, 1, 0, 0, 0), __import__("time").gmtime(epoch)[:6]))
            info.external_attr = (0o100000 | mode) << 16
            info.compress_type = zipfile.ZIP_DEFLATED
            z.writestr(info, data)


dist = os.path.join(ROOT, "dist")
found = False
for fname in sorted(os.listdir(dist)):
    if not fname.startswith("trustctl-") or fname == "SHA256SUMS":
        continue
    stem = fname[len("trustctl-"):]
    if stem.endswith(".exe"):
        stem = stem[:-4]
    osname, arch = stem.split("-", 1)
    windows = osname == "windows"
    found = True
    entries = [("trustctl.exe" if windows else "trustctl", 0o755, read(os.path.join(dist, fname)))]
    importer = os.path.join(dist, "import", f"trustctl-import-{osname}-{arch}" + (".exe" if windows else ""))
    if not os.path.isfile(importer):
        sys.exit(f"{importer} is missing: run tools/build/release.sh first")
    entries.append(("trustctl-import.exe" if windows else "trustctl-import", 0o755, read(importer)))
    entries += [(d, 0o644, read(os.path.join(ROOT, d))) for d in DOCS]
    entries += list(user_tools(osname))
    if windows:
        if args.tray:
            entries.append(("tools/user/trustctl-tray/trustctl-tray.exe", 0o755, read(args.tray)))
    top = f"trustctl-{args.version}-{osname}-{arch}"
    if windows:
        write_zip(os.path.join(args.out, top + ".zip"), top, entries)
    else:
        write_tar(os.path.join(args.out, top + ".tar.gz"), top, entries)
    print("packed", top)
if not found:
    sys.exit("no trustctl-* binaries in dist/: run tools/build/release.sh first")

fw = os.path.join(ROOT, "trustctl", "internal", "firmware")
entries = [("core.hex", 0o644, read(os.path.join(fw, "core.hex"))),
           ("calibration.hex", 0o644, read(os.path.join(fw, "calibration.hex"))),
           ("VERSION", 0o644, read(os.path.join(fw, "VERSION")))]
if args.bootloader:
    entries.append(("boot.hex", 0o644, read(args.bootloader)))
entries += [(d, 0o644, read(os.path.join(ROOT, d))) for d in DOCS[1:]]
top = f"trustctl-firmware-{args.version}"
write_zip(os.path.join(args.out, top + ".zip"), top, entries)
print("packed", top)

sums = []
for fname in sorted(os.listdir(args.out)):
    if fname == "SHA256SUMS":
        continue
    sums.append(f"{hashlib.sha256(read(os.path.join(args.out, fname))).hexdigest()}  {fname}\n")
with open(os.path.join(args.out, "SHA256SUMS"), "w") as f:
    f.writelines(sums)
print("checksums in", os.path.join(args.out, "SHA256SUMS"))
