"""Check Android package ABI coverage and ELF requirements with synthetic libraries."""

from pathlib import Path
import struct
import tempfile
import unittest
import zipfile

import verify_android_native as verifier


def library(abi, alignment=None):
    elf_class, machine = {
        "armeabi-v7a": (1, 40),
        "arm64-v8a": (2, 183),
        "x86_64": (2, 62),
    }[abi]
    alignment = alignment or (4096 if elf_class == 1 else 16384)
    ident = b"\x7fELF" + bytes([elf_class, 1, 1]) + bytes(9)
    if elf_class == 1:
        header = struct.pack("<HHIIIIIHHHHHH", 3, machine, 1, 0, 52, 0, 0, 52, 32, 1, 0, 0, 0)
        segment = struct.pack("<IIIIIIII", 1, 0, 0, 0, 84, 84, 5, alignment)
    else:
        header = struct.pack("<HHIQQQIHHHHHH", 3, machine, 1, 0, 64, 0, 0, 64, 56, 1, 0, 0, 0)
        segment = struct.pack("<IIQQQQQQ", 1, 5, 0, 0, 0, 120, 120, alignment)
    return ident + header + segment


class AndroidNativePackageTest(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)

    def archive(self, suffix=".apk", entries=None):
        prefix = {".apk": "lib", ".aar": "jni", ".aab": "base/lib"}[suffix]
        if entries is None:
            entries = {
                f"{prefix}/{abi}/libgojni.so": library(abi)
                for abi in verifier.DEFAULT_ABIS
            }
        path = Path(self.temporary.name) / ("synthetic" + suffix)
        with zipfile.ZipFile(path, "w") as package:
            for name, data in entries.items():
                package.writestr(name, data)
        return path

    def test_armv7_4k_and_64_bit_16k_libraries_pass_in_all_package_formats(self):
        for suffix in (".aar", ".apk", ".aab"):
            with self.subTest(suffix=suffix):
                verifier.verify_archive(self.archive(suffix))

    def test_missing_armv7_binding_is_rejected(self):
        entries = {
            f"lib/{abi}/libgojni.so": library(abi)
            for abi in ("arm64-v8a", "x86_64")
        }
        with self.assertRaisesRegex(ValueError, "armeabi-v7a"):
            verifier.verify_archive(self.archive(entries=entries))

    def test_64_bit_library_with_4k_alignment_is_rejected(self):
        for abi in ("arm64-v8a", "x86_64"):
            with self.subTest(abi=abi):
                entries = {f"lib/{abi}/libgojni.so": library(abi, 4096)}
                with self.assertRaisesRegex(ValueError, "alignment"):
                    verifier.verify_archive(self.archive(entries=entries), (abi,))

    def test_library_for_wrong_machine_is_rejected(self):
        entries = {"lib/armeabi-v7a/libgojni.so": library("arm64-v8a")}
        with self.assertRaisesRegex(ValueError, "ELF"):
            verifier.verify_archive(self.archive(entries=entries), ("armeabi-v7a",))

    def test_additional_native_dependency_must_also_have_correct_alignment(self):
        entries = {f"lib/{abi}/libgojni.so": library(abi) for abi in verifier.DEFAULT_ABIS}
        entries["lib/arm64-v8a/libdependency.so"] = library("arm64-v8a", 4096)
        with self.assertRaisesRegex(ValueError, "libdependency.so.*alignment"):
            verifier.verify_archive(self.archive(entries=entries))

    def test_unexpected_abi_is_rejected(self):
        entries = {f"lib/{abi}/libgojni.so": library(abi) for abi in verifier.DEFAULT_ABIS}
        entries["lib/x86/libgojni.so"] = library("armeabi-v7a")
        with self.assertRaisesRegex(ValueError, "x86"):
            verifier.verify_archive(self.archive(entries=entries))

    def test_truncated_elf_is_rejected(self):
        entries = {"lib/armeabi-v7a/libgojni.so": library("armeabi-v7a")[:60]}
        with self.assertRaisesRegex(ValueError, "ELF"):
            verifier.verify_archive(self.archive(entries=entries), ("armeabi-v7a",))

    def test_local_single_abi_build_can_be_checked_explicitly(self):
        entries = {"lib/arm64-v8a/libgojni.so": library("arm64-v8a")}
        verifier.verify_archive(self.archive(entries=entries), ("arm64-v8a",))


if __name__ == "__main__":
    unittest.main()
