"""Layout smoke tests: entry points exist, scripts compile, docs point at real files.

Stdlib + pytest only. No network, no backend, no model.
"""

import py_compile
import re
import subprocess
import tempfile
import sys
from pathlib import Path

import pytest

ROOT = Path(__file__).resolve().parents[1]

# Markdown links whose target does not resolve today. Left in place on purpose:
# docs are not fixed by this test suite. Entries are (source file, link target).
KNOWN_BROKEN = {
    # Submission files written by a parallel job; drop these once they are committed.
    ("README.md", "documents/DOSYN_Wassit_Submission.pdf"),
    ("README.md", "documents/jury/"),
}


def _rel(path):
    return path.relative_to(ROOT).as_posix()


# ---------------------------------------------------------------- 1


ENTRY_POINTS = [
    "agent/runner.py",
    "agent/scripted.py",
    "agent/usage.py",
    "agent/tools/catalogue.py",
    "agent/tools/enquiry.py",
    "agent/prompts/system.md",
    "agent/cases/cases.jsonl",
    "scripts/acceptance.py",
    "scripts/demo_flow.py",
    "scripts/run_cases.py",
    "scripts/show_results.py",
    "scripts/seed_demo.sh",
    "backend/cmd/server/main.go",
    "backend/go.mod",
    "backend/Makefile",
    "cornilia-guard/go/go.mod",
    "cornilia-guard/python/cornilia_guard/__init__.py",
    "AGENTS.md",
    "CLAUDE.md",
    "CONTRACT.md",
]


def test_entry_points_exist():
    missing = [p for p in ENTRY_POINTS if not (ROOT / p).exists()]
    assert missing == [], f"missing entry points: {missing}"


# ---------------------------------------------------------------- 2


def test_scripts_compile():
    scripts = sorted((ROOT / "scripts").glob("*.py"))
    assert scripts, "no scripts/*.py found"
    with tempfile.TemporaryDirectory() as tmp:
        for script in scripts:
            # doraise=True raises PyCompileError on a syntax error; bytecode goes to a temp dir
            py_compile.compile(
                str(script),
                cfile=str(Path(tmp) / (script.stem + ".pyc")),
                doraise=True,
            )


# ---------------------------------------------------------------- 3
# acceptance.py, demo_flow.py and run_cases.py build their argparse parser first
# thing in main() and do no network I/O at import time, so --help exits 0 offline.
# show_results.py has no argparse and calls the backend at import, so it is not listed.


@pytest.mark.parametrize("name", ["acceptance.py", "demo_flow.py", "run_cases.py"])
def test_script_help(name):
    result = subprocess.run(
        [sys.executable, str(ROOT / "scripts" / name), "--help"],
        cwd=str(ROOT),
        timeout=20,
        capture_output=True,
        text=True,
    )
    assert result.returncode == 0, f"{name} --help exited {result.returncode}: {result.stderr}"
    assert "usage" in result.stdout, f"{name} --help printed no usage: {result.stdout!r}"


# ---------------------------------------------------------------- 4


def test_claude_md_references_exist():
    text = (ROOT / "CLAUDE.md").read_text(encoding="utf-8")

    # Plain *.md references (glob patterns cannot match this class, so they are skipped here).
    md_tokens = sorted(set(re.findall(r"[\w./-]+\.md", text)))
    md_tokens = [t.lstrip("@") for t in md_tokens if "*" not in t]
    assert md_tokens, "CLAUDE.md references no markdown files"
    missing = [t for t in md_tokens if not (ROOT / t).is_file()]
    assert missing == [], f"CLAUDE.md references missing files: {missing}"

    # Glob references such as LEAD_PROMPT_*.md must match at least one real file.
    globs = sorted(set(re.findall(r"[\w./-]*\*[\w./*-]*\.md", text)))
    assert any(g.endswith("LEAD_PROMPT_*.md") for g in globs), "expected a LEAD_PROMPT_*.md glob in CLAUDE.md"
    for pattern in globs:
        matches = [p for p in ROOT.glob(pattern) if ".git" not in p.relative_to(ROOT).parts]
        assert matches, f"CLAUDE.md glob {pattern!r} matches no file under ROOT"

    # Directory reference.
    assert ".claude/agents/" in text, "expected .claude/agents/ reference in CLAUDE.md"
    assert (ROOT / ".claude" / "agents").is_dir(), "CLAUDE.md references .claude/agents/ which is missing"


# ---------------------------------------------------------------- 5

LINK_RE = re.compile(r"\]\(([^)#\s]+)")
EXTERNAL_PREFIXES = ("http://", "https://", "mailto:")


def _tracked_markdown():
    out = subprocess.run(
        ["git", "ls-files", "-z", "*.md"],
        cwd=str(ROOT),
        capture_output=True,
        text=True,
        check=True,
    ).stdout
    return [ROOT / name for name in out.split("\0") if name]


def test_markdown_relative_links_resolve():
    files = _tracked_markdown()
    assert files, "git ls-files returned no markdown files"

    failures = []
    for md in files:
        text = md.read_text(encoding="utf-8")
        for raw_target in LINK_RE.findall(text):
            target = raw_target.strip("<>")  # <https://...> autolink form
            if target.startswith(EXTERNAL_PREFIXES) or target.startswith("#"):
                continue
            key = (_rel(md), target)
            if key in KNOWN_BROKEN:
                continue
            resolved = (md.parent / target).resolve()
            if not resolved.exists():
                failures.append(f"{_rel(md)} -> {target}")

    assert failures == [], "broken relative markdown links:\n  " + "\n  ".join(failures)
