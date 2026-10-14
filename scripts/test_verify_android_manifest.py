"""Keep the standalone app's required GLES version in source and shipped manifests."""

import os
from pathlib import Path
import unittest
from unittest.mock import patch

import verify_android_manifest as verifier


def manifest(features):
    return (
        '<manifest xmlns:android="http://schemas.android.com/apk/res/android">'
        + features
        + "</manifest>"
    ).encode()


REQUIRED_GLES3 = '<uses-feature android:glEsVersion="0x00030000" android:required="true" />'


class AndroidManifestTest(unittest.TestCase):
    def test_required_gles3_accepts_hex_and_decimal_tool_output(self):
        for version in ("0x00030000", "196608"):
            with self.subTest(version=version):
                verifier.verify_manifest(manifest(REQUIRED_GLES3.replace("0x00030000", version)))

    def test_android_default_for_required_is_true(self):
        verifier.verify_manifest(manifest('<uses-feature android:glEsVersion="0x00030000" />'))

    def test_missing_optional_or_wrong_version_is_rejected(self):
        for feature in (
            "",
            REQUIRED_GLES3.replace('required="true"', 'required="false"'),
            REQUIRED_GLES3.replace("0x00030000", "0x00020000"),
            REQUIRED_GLES3.replace("0x00030000", "0x00030001"),
            REQUIRED_GLES3.replace("0x00030000", "invalid"),
            REQUIRED_GLES3.replace("android:glEsVersion", "glEsVersion"),
        ):
            with self.subTest(feature=feature), self.assertRaisesRegex(ValueError, "OpenGL ES 3.0"):
                verifier.verify_manifest(manifest(feature))

    def test_duplicate_gles_declarations_are_rejected(self):
        with self.assertRaisesRegex(ValueError, "OpenGL ES 3.0"):
            verifier.verify_manifest(manifest(REQUIRED_GLES3 + REQUIRED_GLES3))

    def test_nested_feature_cannot_replace_a_manifest_feature(self):
        with self.assertRaisesRegex(ValueError, "OpenGL ES 3.0"):
            verifier.verify_manifest(manifest("<application>" + REQUIRED_GLES3 + "</application>"))

    def test_named_feature_cannot_replace_the_gles_requirement(self):
        feature = REQUIRED_GLES3.replace("<uses-feature", '<uses-feature android:name="unrelated"')
        with self.assertRaisesRegex(ValueError, "OpenGL ES 3.0"):
            verifier.verify_manifest(manifest(feature))

    def test_product_source_requires_gles3(self):
        source = Path(__file__).resolve().parents[1] / "android/app/src/main/AndroidManifest.xml"
        verifier.verify_manifest(source.read_bytes())

    @patch.dict(os.environ, {"ANDROID_HOME": "/sdk"})
    @patch("verify_android_manifest.tool_xml", return_value=manifest(REQUIRED_GLES3))
    def test_apk_uses_the_sdk_manifest_decoder(self, tool_xml):
        verifier.verify_package(Path("path with spaces/app.apk"))
        executable = "apkanalyzer.bat" if os.name == "nt" else "apkanalyzer"
        tool_xml.assert_called_once_with([
            str(Path("/sdk/cmdline-tools/latest/bin") / executable),
            "manifest", "print", str(Path("path with spaces/app.apk")),
        ])

    @patch.dict(os.environ, {"JAVA_HOME": "/jdk"})
    @patch("verify_android_manifest.tool_xml", return_value=manifest(REQUIRED_GLES3))
    def test_aab_decodes_the_actual_base_module(self, tool_xml):
        verifier.verify_package(Path("app.aab"), bundletool=Path("bundletool.jar"))
        executable = "java.exe" if os.name == "nt" else "java"
        tool_xml.assert_called_once_with([
            str(Path("/jdk/bin") / executable), "-jar", "bundletool.jar", "dump", "manifest",
            "--bundle=app.aab", "--module=base",
        ])

    def test_aab_requires_an_explicit_bundletool(self):
        with self.assertRaisesRegex(ValueError, "bundletool"):
            verifier.verify_package(Path("app.aab"))


if __name__ == "__main__":
    unittest.main()
