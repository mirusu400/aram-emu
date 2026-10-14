#!/usr/bin/env python3
"""Verify that the standalone app requires exactly OpenGL ES 3.0."""

import argparse
import os
from pathlib import Path
import subprocess
import xml.etree.ElementTree as ET


ANDROID = "{http://schemas.android.com/apk/res/android}"


def verify_manifest(xml):
    root = ET.fromstring(xml)
    features = [
        feature for feature in root.findall("uses-feature")
        if ANDROID + "glEsVersion" in feature.attrib
    ]
    error = "manifest must declare one required OpenGL ES 3.0 uses-feature"
    if root.tag != "manifest" or len(features) != 1:
        raise ValueError(error)
    feature = features[0]
    try:
        version = int(feature.get(ANDROID + "glEsVersion"), 0)
    except ValueError:
        raise ValueError(error) from None
    if (
        version != 0x00030000
        or feature.get(ANDROID + "required", "true") != "true"
        or ANDROID + "name" in feature.attrib
    ):
        raise ValueError(error)


def tool_xml(command):
    # SDK .bat launchers and Java must stay hidden on Windows. Capture bytes
    # directly so PowerShell's redirection encoding cannot alter the XML.
    return subprocess.run(
        command, check=True, capture_output=True,
        creationflags=subprocess.CREATE_NO_WINDOW if os.name == "nt" else 0,
    ).stdout


def verify_package(path, *, apkanalyzer=None, bundletool=None):
    path = Path(path)
    if path.suffix == ".xml":
        xml = path.read_bytes()
    elif path.suffix == ".apk":
        if apkanalyzer is None:
            sdk = os.environ.get("ANDROID_HOME") or os.environ.get("ANDROID_SDK_ROOT")
            if not sdk:
                raise ValueError("set ANDROID_HOME/ANDROID_SDK_ROOT or pass --apkanalyzer")
            executable = "apkanalyzer.bat" if os.name == "nt" else "apkanalyzer"
            apkanalyzer = Path(sdk) / "cmdline-tools/latest/bin" / executable
        xml = tool_xml([str(apkanalyzer), "manifest", "print", str(path)])
    elif path.suffix == ".aab":
        if bundletool is None:
            raise ValueError("AAB verification requires --bundletool pointing to bundletool-all.jar")
        executable = "java.exe" if os.name == "nt" else "java"
        java_home = os.environ.get("JAVA_HOME")
        java = str(Path(java_home) / "bin" / executable) if java_home else executable
        xml = tool_xml([
            java, "-jar", str(bundletool), "dump", "manifest",
            f"--bundle={path}", "--module=base",
        ])
    else:
        raise ValueError("expected an .apk, .aab, or .xml manifest")
    verify_manifest(xml)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("package", type=Path)
    parser.add_argument("--apkanalyzer", type=Path)
    parser.add_argument("--bundletool", type=Path)
    arguments = parser.parse_args()
    try:
        verify_package(
            arguments.package, apkanalyzer=arguments.apkanalyzer, bundletool=arguments.bundletool,
        )
    except (OSError, ValueError, ET.ParseError, subprocess.CalledProcessError) as error:
        detail = str(error)
        if isinstance(error, subprocess.CalledProcessError):
            detail = error.stderr.decode(errors="replace")
        parser.exit(1, f"{arguments.package.name}: {detail}\n")
    print(f"Verified {arguments.package.name}: required OpenGL ES 3.0")


if __name__ == "__main__":
    main()
