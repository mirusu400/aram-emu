"""Promote candidate pins only onto the exact product revision that was tested."""

import argparse
import json
import os
from pathlib import Path
import re
import subprocess


def git(repository: Path, *arguments: str) -> str:
    result = subprocess.run(
        ["git", "-C", str(repository), *arguments],
        check=True,
        capture_output=True,
        text=True,
        creationflags=subprocess.CREATE_NO_WINDOW if os.name == "nt" else 0,
    )
    return result.stdout.strip()


def component_main_heads(repository: Path) -> dict[str, str]:
    heads = {}
    for component in ("core", "frontend"):
        result = git(
            repository, "ls-remote",
            f"https://github.com/mirusu400/aram-{component}.git", "refs/heads/main",
        )
        fields = result.split()
        if len(fields) != 2 or re.fullmatch(r"[0-9a-f]{40}", fields[0]) is None:
            raise ValueError(f"cannot resolve current {component} main")
        heads[component] = fields[0]
    return heads


def promote(repository: Path, baseline: str, core: str, frontend: str) -> str | None:
    for revision in (baseline, core, frontend):
        if re.fullmatch(r"[0-9a-f]{40}", revision) is None:
            raise ValueError("promotion requires exact commit SHAs")
    if git(repository, "rev-parse", "HEAD") != baseline:
        raise ValueError("product checkout differs from the verified revision")
    if git(repository, "status", "--porcelain", "--untracked-files=no"):
        raise ValueError("product checkout contains unverified changes")

    git(repository, "fetch", "origin", "main")
    if git(repository, "rev-parse", "refs/remotes/origin/main") != baseline:
        print("main changed during validation; the next sync must validate it again")
        return None

    if component_main_heads(repository) != {"core": core, "frontend": frontend}:
        print("component main changed during validation; the next sync must retry")
        return None

    manifest_path = repository / "product-components.json"
    manifest = json.loads(manifest_path.read_text(encoding="utf-8"))
    manifest.update(core=core, frontend=frontend)
    manifest_path.write_text(json.dumps(manifest, indent=2) + "\n", encoding="utf-8")
    git(repository, "add", "product-components.json")
    git(
        repository,
        "commit",
        "-m", "chore: promote verified product components",
        "-m", f"aram-core: {core}\naram-frontend: {frontend}\nValidated product: {baseline}",
    )
    # A normal push also protects the race between the fetch and the push.
    # Never rebase or force-push a candidate onto product code that was not tested.
    git(repository, "push", "origin", "HEAD:main")
    return git(repository, "rev-parse", "HEAD")


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--baseline-sha", required=True)
    parser.add_argument("--core-sha", required=True)
    parser.add_argument("--frontend-sha", required=True)
    args = parser.parse_args()
    revision = promote(Path.cwd(), args.baseline_sha, args.core_sha, args.frontend_sha)
    output_path = os.environ.get("GITHUB_OUTPUT")
    if output_path:
        with open(output_path, "a", encoding="utf-8") as output:
            output.write(f"promoted={'true' if revision else 'false'}\n")
            output.write(f"product_sha={revision or ''}\n")


if __name__ == "__main__":
    main()
