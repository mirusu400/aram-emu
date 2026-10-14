#!/usr/bin/env python3
"""Verify native ABI coverage and ELF alignment in an Android AAR, APK, or AAB."""

import argparse
from pathlib import Path
import struct
import zipfile


DEFAULT_ABIS = ("armeabi-v7a", "arm64-v8a", "x86_64")
# ELF class, e_machine, and minimum PT_LOAD alignment. Android's 16 KB page
# requirement applies to 64-bit devices; ARMv7 libraries use 4 KB pages.
ABI_ELF = {
    "armeabi-v7a": (1, 40, 4096),
    "arm64-v8a": (2, 183, 16384),
    "x86_64": (2, 62, 16384),
}


def verify_elf(data, abi, name):
    elf_class, machine, minimum_alignment = ABI_ELF[abi]
    header_size, segment_size = (52, 32) if elf_class == 1 else (64, 56)
    if (
        len(data) < header_size
        or data[:7] != b"\x7fELF" + bytes((elf_class, 1, 1))
        or struct.unpack_from("<HH", data, 16) != (3, machine)
    ):
        raise ValueError(f"{name}: invalid ELF shared library for {abi}")
    if elf_class == 1:
        offset = struct.unpack_from("<I", data, 28)[0]
        entry_size, count = struct.unpack_from("<HH", data, 42)
    else:
        offset = struct.unpack_from("<Q", data, 32)[0]
        entry_size, count = struct.unpack_from("<HH", data, 54)
    if (
        count == 0
        or offset < header_size
        or entry_size < segment_size
        or offset + count * entry_size > len(data)
    ):
        raise ValueError(f"{name}: invalid ELF program header table")
    loads = 0
    for index in range(count):
        start = offset + index * entry_size
        if struct.unpack_from("<I", data, start)[0] != 1:  # PT_LOAD
            continue
        loads += 1
        if elf_class == 1:
            _, file_offset, address, _, file_size, _, _, alignment = struct.unpack_from(
                "<IIIIIIII", data, start
            )
        else:
            _, _, file_offset, address, _, file_size, _, alignment = struct.unpack_from(
                "<IIQQQQQQ", data, start
            )
        if (
            alignment < minimum_alignment
            or alignment & (alignment - 1)
            or (address - file_offset) % alignment
        ):
            raise ValueError(f"{name}: LOAD alignment must be at least {minimum_alignment}")
        if file_size and file_offset + file_size > len(data):
            raise ValueError(f"{name}: truncated ELF LOAD segment")
    if not loads:
        raise ValueError(f"{name}: ELF has no LOAD segments")


def verify_archive(path, expected_abis=DEFAULT_ABIS):
    path = Path(path)
    expected_abis = tuple(expected_abis)
    if not expected_abis or len(set(expected_abis)) != len(expected_abis):
        raise ValueError("expected ABIs must be non-empty and unique")
    if set(expected_abis) - ABI_ELF.keys():
        raise ValueError(f"unsupported expected ABIs: {expected_abis}")
    try:
        prefix = {".aar": "jni/", ".apk": "lib/", ".aab": "base/lib/"}[path.suffix]
    except KeyError:
        raise ValueError("expected an .aar, .apk, or .aab archive") from None
    bindings = set()
    with zipfile.ZipFile(path) as archive:
        names = [entry.filename for entry in archive.infolist()]
        if len(names) != len(set(names)):
            raise ValueError("archive has duplicate entries")
        for entry in archive.infolist():
            if not entry.filename.startswith(prefix) or not entry.filename.endswith(".so"):
                continue
            relative = entry.filename[len(prefix):].split("/")
            if len(relative) != 2 or relative[0] not in expected_abis:
                raise ValueError(f"unexpected native library: {entry.filename}")
            abi, filename = relative
            verify_elf(archive.read(entry), abi, entry.filename)
            if filename == "libgojni.so":
                bindings.add(abi)
    missing = set(expected_abis) - bindings
    if missing:
        raise ValueError(f"missing libgojni.so for: {', '.join(sorted(missing))}")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("archive", type=Path)
    parser.add_argument("--abis", default=",".join(DEFAULT_ABIS))
    arguments = parser.parse_args()
    abis = tuple(arguments.abis.split(","))
    try:
        verify_archive(arguments.archive, abis)
    except (OSError, ValueError, zipfile.BadZipFile) as error:
        parser.exit(1, f"{error}\n")
    print(f"Verified {arguments.archive.name}: {', '.join(abis)}")


if __name__ == "__main__":
    main()
