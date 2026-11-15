"""CLI tests for scripts/: exit codes, missing-backend behaviour, cases file shape.

Part A needs no server: each script is pointed at a dead port (nothing listens there).
Part B uses the `live` fixture and is skipped unless WASSIT_LIVE_URL is set.
Stdlib + pytest only. Runs the scripts as subprocesses from the repo root.
"""

import json
import os
import subprocess
import sys
from pathlib import Path

import pytest

from conftest import KnownBugFailure, assert_known_bug

ROOT = Path(__file__).resolve().parents[1]
SCRIPTS = ROOT / "scripts"
CASES = ROOT / "agent" / "cases" / "cases.jsonl"
RESULTS = ROOT / "agent" / "cases" / "results.md"

DEAD = "http://127.0.0.1:9"  # nothing listens on port 9 locally
VALID_OUTCOMES = {"available", "unavailable", "unable_to_confirm"}
CASE_KEYS = ("id", "raw_request", "quantity", "replies", "expected_outcome")


def _env(backend):
    env = dict(os.environ)
    env["BACKEND_URL"] = backend
    return env


def _run_py(script, args, backend, timeout=60):
    return subprocess.run(
        [sys.executable, str(SCRIPTS / script), *args],
        cwd=str(ROOT),
        env=_env(backend),
        capture_output=True,
        text=True,
        timeout=timeout,
    )


def _run_sh(script, backend, timeout=60):
    return subprocess.run(
        ["bash", str(SCRIPTS / script)],
        cwd=str(ROOT),
        env=_env(backend),
        capture_output=True,
        text=True,
        timeout=timeout,
    )


# ---------------------------------------------------------------- Part A (no server)


def test_seed_demo_backend_down_exits_1():
    result = _run_sh("seed_demo.sh", DEAD)
    assert result.returncode == 1, f"expected exit 1, got {result.returncode}: {result.stdout} {result.stderr}"
    combined = result.stdout + result.stderr
    assert "make run" in combined, f"start commands (make run) not printed: {combined!r}"


def test_seed_demo_help_text_uses_documented_price():
    text = (SCRIPTS / "seed_demo.sh").read_text(encoding="utf-8")
    assert "AGENT_PRICE_IN_PER_MTOK" in text, "seed_demo.sh does not document AGENT_PRICE_IN_PER_MTOK"


def test_acceptance_backend_down_exits_1():
    result = subprocess.run(
        [sys.executable, str(SCRIPTS / "acceptance.py")],
        cwd=str(ROOT),
        env=_env(DEAD),
        capture_output=True,
        text=True,
        timeout=120,
    )
    assert result.returncode == 1, f"expected exit 1, got {result.returncode}: {result.stdout[-500:]} {result.stderr[-500:]}"


@pytest.mark.xfail(strict=True, raises=KnownBugFailure,
                   reason='BUG-13: scripts/show_results.py prints a raw Python traceback when the backend is down (the 30-second demo script)')
def test_show_results_backend_down_exits_nonzero():
    result = _run_py("show_results.py", [], DEAD)
    assert result.returncode != 0, f"expected non-zero exit, got {result.returncode}"
    assert_known_bug("Traceback" not in result.stderr, f"raw traceback on stderr: {result.stderr[-800:]}")


def test_demo_flow_backend_down_exits_1():
    result = _run_py("demo_flow.py", ["--no-pause"], DEAD)
    assert result.returncode == 1, f"expected exit 1, got {result.returncode}: {result.stdout[-500:]} {result.stderr[-500:]}"
    assert "Traceback" not in result.stderr, f"raw traceback on stderr: {result.stderr[-800:]}"


def test_run_cases_missing_cases_file_exits_1():
    result = _run_py("run_cases.py", ["--cases", "/nonexistent.jsonl"], DEAD)
    assert result.returncode == 1, f"expected exit 1, got {result.returncode}: {result.stderr[-500:]}"


def test_run_cases_unknown_case_id_exits_1():
    before = RESULTS.read_bytes()
    try:
        result = _run_py("run_cases.py", ["--only", "no-such-case"], DEAD)
        assert result.returncode == 1, f"expected exit 1, got {result.returncode}: {result.stderr[-500:]}"
        after = RESULTS.read_bytes()
        assert after == before, "agent/cases/results.md was rewritten by a run with an unknown case id"
    finally:
        RESULTS.write_bytes(before)


def test_cases_file_has_20_valid_cases():
    lines = [line for line in CASES.read_text(encoding="utf-8").splitlines() if line.strip()]
    assert len(lines) == 20, f"expected 20 JSON lines, found {len(lines)}"
    cases = [json.loads(line) for line in lines]
    for case in cases:
        missing = [k for k in CASE_KEYS if k not in case]
        assert missing == [], f"case {case.get('id')!r} missing keys {missing}"
    ids = [c["id"] for c in cases]
    assert len(set(ids)) == len(ids), f"duplicate case ids: {sorted({i for i in ids if ids.count(i) > 1})}"
    bad = [(c["id"], c["expected_outcome"]) for c in cases if c["expected_outcome"] not in VALID_OUTCOMES]
    assert bad == [], f"invalid expected_outcome values: {bad}"


# ---------------------------------------------------------------- Part B (live backend)


def test_seed_demo_against_live_backend_exits_0(live):
    result = _run_sh("seed_demo.sh", live.base)
    assert result.returncode == 0, f"expected exit 0, got {result.returncode}: {result.stdout[-500:]} {result.stderr[-500:]}"
    assert "seed check passed" in result.stdout, f"no success line in stdout: {result.stdout[-500:]!r}"


def test_show_results_against_live_backend_exits_0(live):
    result = _run_py("show_results.py", [], live.base)
    assert result.returncode == 0, f"expected exit 0, got {result.returncode}: {result.stderr[-800:]}"


@pytest.mark.xfail(strict=True, raises=KnownBugFailure,
                   reason='BUG-14: scripts/run_cases.py rewrites the tracked agent/cases/results.md on every run; there is no flag to write elsewhere')
def test_run_cases_does_not_modify_tracked_results_file(live):
    original = RESULTS.read_bytes()
    first_id = json.loads(CASES.read_text(encoding="utf-8").splitlines()[0])["id"]
    try:
        result = _run_py("run_cases.py", ["--only", first_id, "--policy", "scripted"], live.base, timeout=180)
        assert result.returncode == 0, f"expected exit 0, got {result.returncode}: {result.stderr[-800:]}"
        after = RESULTS.read_bytes()
        assert_known_bug(after == original, "a test run rewrote the tracked file agent/cases/results.md")
    finally:
        RESULTS.write_bytes(original)
