#!/usr/bin/env python3
"""Publish a verified Android App Bundle through the Google Play Edits API."""

import argparse
import json
import os
from pathlib import Path
import re
import sys
from urllib.error import HTTPError, URLError
from urllib.parse import quote, urlencode
from urllib.request import Request, urlopen


API_ROOT = "https://androidpublisher.googleapis.com/androidpublisher/v3/applications"
UPLOAD_ROOT = "https://androidpublisher.googleapis.com/upload/androidpublisher/v3/applications"
MAX_RELEASE_NOTES_CHARS = 500
BLOCKING_RELEASE_STATUSES = frozenset({"draft", "inProgress", "halted"})
LANGUAGE_RE = re.compile(r"^[A-Za-z]{2,3}(?:-[A-Za-z0-9]{2,8})+$")


class GooglePlayError(RuntimeError):
    """A Google Play API request or publishing invariant failed."""


def load_release_notes(entries):
    """Load LANGUAGE=PATH arguments into Google Play LocalizedText objects."""
    notes = []
    seen = set()
    for entry in entries:
        if "=" not in entry:
            raise ValueError(f"release note must use LANGUAGE=PATH: {entry}")
        language, raw_path = entry.split("=", 1)
        if not LANGUAGE_RE.fullmatch(language):
            raise ValueError(f"invalid release-note language: {language}")
        if language in seen:
            raise ValueError(f"duplicate release-note language: {language}")
        path = Path(raw_path)
        text = path.read_text(encoding="utf-8").strip()
        if not text:
            raise ValueError(f"release note is empty: {path}")
        if len(text) > MAX_RELEASE_NOTES_CHARS:
            raise ValueError(
                f"release note for {language} is {len(text)} characters; "
                f"Google Play allows at most {MAX_RELEASE_NOTES_CHARS}"
            )
        notes.append({"language": language, "text": text})
        seen.add(language)
    if not notes:
        raise ValueError("at least one localized release note is required")
    return notes


def build_track_payload(track, version_code, release_name, status, release_notes):
    if status not in {"draft", "completed"}:
        raise ValueError("release status must be draft or completed")
    if not release_name.strip():
        raise ValueError("release name must not be empty")
    return {
        "track": track,
        "releases": [{
            "name": release_name,
            "versionCodes": [str(version_code)],
            "status": status,
            "releaseNotes": release_notes,
        }],
    }


def ensure_track_has_no_pending_release(track_state):
    blockers = [
        release.get("status", "statusUnspecified")
        for release in track_state.get("releases", [])
        if release.get("status") in BLOCKING_RELEASE_STATUSES
    ]
    if blockers:
        statuses = ", ".join(sorted(set(blockers)))
        raise GooglePlayError(
            "refusing to replace a track with an unfinished release "
            f"({statuses}); finish or remove it in Play Console first"
        )


class GooglePlayPublisher:
    def __init__(self, package_name, access_token, *, opener=urlopen):
        if not access_token:
            raise ValueError("Google Play access token is empty")
        package = quote(package_name, safe="")
        self.base_url = f"{API_ROOT}/{package}"
        self.upload_url = f"{UPLOAD_ROOT}/{package}"
        self.access_token = access_token
        self.opener = opener

    def request(self, method, url, *, payload=None, media_path=None, timeout=60):
        if payload is not None and media_path is not None:
            raise ValueError("a request cannot contain both JSON and media")
        headers = {
            "Accept": "application/json",
            "Authorization": f"Bearer {self.access_token}",
        }
        data = None
        if payload is not None:
            data = json.dumps(payload, ensure_ascii=False).encode("utf-8")
            headers["Content-Type"] = "application/json; charset=utf-8"
        elif media_path is not None:
            data = Path(media_path).read_bytes()
            headers["Content-Type"] = "application/octet-stream"
        request = Request(url, data=data, headers=headers, method=method)
        try:
            with self.opener(request, timeout=timeout) as response:
                raw = response.read()
        except HTTPError as error:
            detail = error.read().decode("utf-8", errors="replace").strip()
            raise GooglePlayError(
                f"Google Play API {method} failed with HTTP {error.code}: {detail}"
            ) from None
        except URLError as error:
            raise GooglePlayError(
                f"Google Play API {method} request failed: {error.reason}"
            ) from None
        if not raw:
            return {}
        try:
            return json.loads(raw)
        except json.JSONDecodeError as error:
            raise GooglePlayError(
                f"Google Play API {method} returned invalid JSON: {error}"
            ) from None

    def insert_edit(self):
        response = self.request("POST", f"{self.base_url}/edits")
        edit_id = response.get("id")
        if not edit_id:
            raise GooglePlayError("Google Play did not return an edit ID")
        return str(edit_id)

    def get_track(self, edit_id, track):
        path = (
            f"{self.base_url}/edits/{quote(edit_id, safe='')}/tracks/"
            f"{quote(track, safe='')}"
        )
        return self.request("GET", path)

    def upload_bundle(self, edit_id, aab_path):
        path = f"{self.upload_url}/edits/{quote(edit_id, safe='')}/bundles"
        response = self.request(
            "POST",
            f"{path}?{urlencode({'uploadType': 'media'})}",
            media_path=aab_path,
            timeout=180,
        )
        try:
            version_code = int(response["versionCode"])
        except (KeyError, TypeError, ValueError):
            raise GooglePlayError(
                "Google Play bundle upload did not return a versionCode"
            ) from None
        if version_code <= 0:
            raise GooglePlayError("Google Play returned an invalid versionCode")
        return version_code

    def upload_mapping(self, edit_id, version_code, mapping_path):
        path = (
            f"{self.upload_url}/edits/{quote(edit_id, safe='')}/apks/"
            f"{version_code}/deobfuscationFiles/proguard"
        )
        return self.request(
            "POST",
            f"{path}?{urlencode({'uploadType': 'media'})}",
            media_path=mapping_path,
            timeout=180,
        )

    def update_track(self, edit_id, track, payload):
        path = (
            f"{self.base_url}/edits/{quote(edit_id, safe='')}/tracks/"
            f"{quote(track, safe='')}"
        )
        return self.request("PUT", path, payload=payload)

    def commit_edit(self, edit_id):
        query = urlencode({
            "changesInReviewBehavior": "ERROR_IF_IN_REVIEW",
        })
        path = f"{self.base_url}/edits/{quote(edit_id, safe='')}:commit?{query}"
        return self.request("POST", path)

    def delete_edit(self, edit_id):
        path = f"{self.base_url}/edits/{quote(edit_id, safe='')}"
        return self.request("DELETE", path)


def publish_release(
    publisher,
    *,
    aab_path,
    mapping_path,
    track,
    status,
    release_name,
    release_notes,
):
    edit_id = None
    try:
        edit_id = publisher.insert_edit()
        ensure_track_has_no_pending_release(publisher.get_track(edit_id, track))
        version_code = publisher.upload_bundle(edit_id, aab_path)
        publisher.upload_mapping(edit_id, version_code, mapping_path)
        payload = build_track_payload(
            track, version_code, release_name, status, release_notes
        )
        publisher.update_track(edit_id, track, payload)
        publisher.commit_edit(edit_id)
        return version_code
    except Exception:
        if edit_id is not None:
            try:
                publisher.delete_edit(edit_id)
            except Exception as cleanup_error:
                print(
                    f"warning: could not delete failed Google Play edit: {cleanup_error}",
                    file=sys.stderr,
                )
        raise


def parse_args(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--package-name", required=True)
    parser.add_argument("--aab", type=Path, required=True)
    parser.add_argument("--mapping", type=Path, required=True)
    parser.add_argument("--track", default="production")
    parser.add_argument("--status", choices=("draft", "completed"), default="completed")
    parser.add_argument("--release-name", required=True)
    parser.add_argument(
        "--release-note",
        action="append",
        default=[],
        metavar="LANGUAGE=PATH",
    )
    parser.add_argument(
        "--access-token-env",
        default="GOOGLE_PLAY_ACCESS_TOKEN",
        help="environment variable containing the OAuth access token",
    )
    return parser.parse_args(argv)


def main(argv=None):
    args = parse_args(argv)
    for path in (args.aab, args.mapping):
        if not path.is_file() or path.stat().st_size == 0:
            raise SystemExit(f"required publishing artifact is missing or empty: {path}")
    token = os.environ.get(args.access_token_env, "")
    if not token:
        raise SystemExit(f"{args.access_token_env} is not set")
    try:
        release_notes = load_release_notes(args.release_note)
        publisher = GooglePlayPublisher(args.package_name, token)
        version_code = publish_release(
            publisher,
            aab_path=args.aab,
            mapping_path=args.mapping,
            track=args.track,
            status=args.status,
            release_name=args.release_name,
            release_notes=release_notes,
        )
    except (GooglePlayError, OSError, ValueError) as error:
        raise SystemExit(str(error)) from None
    print(
        f"Published {args.release_name} (versionCode {version_code}) "
        f"to Google Play track {args.track} with status {args.status}."
    )


if __name__ == "__main__":
    main()
