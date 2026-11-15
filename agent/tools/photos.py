"""Package photo reading (CONTRACT rule 10): the agent READS the photo, the Go backend DECIDES.

The vision model only extracts the printed product code and expiry. The backend compares that reading
with what the pharmacist typed and the government reference; any disagreement goes to a human.
A photo alone never proves a box is genuine.
"""

import base64
import json
import os
import re
import sys
import urllib.error
import urllib.request
from typing import Annotated
from urllib.parse import quote

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))  # lets this file import _http when Cornilia loads it

import _http  # noqa: E402
from cornilia.tools import tool  # noqa: E402

DEFAULT_VISION_MODEL = "gemma4:31b"  # probe 2026-11-15: reads the test box; qwen3.5:35b-a3b fails on images
DEFAULT_VISION_URL = "http://10.31.216.24:11434"
VISION_TIMEOUT_SECONDS = 60
MAX_PHOTO_BYTES = 10 * 1024 * 1024

VISION_PROMPT = (
    "Read this medicine package photo. Reply with ONLY JSON, no other text: "
    '{"code": "<the product code or barcode digits printed on the box>", '
    '"expiry_month": "YYYY-MM", "confidence": <0 to 1>}. '
    'Use "" for anything you cannot read clearly. Do not guess.'
)


def _vision_url() -> str:
    explicit = os.environ.get("WASSIT_VISION_URL", "").strip()
    if explicit:
        return explicit.rstrip("/")
    base = os.environ.get("CORNILIA_BASE_URL", "").strip().rstrip("/")
    if base.endswith("/v1"):
        base = base[: -len("/v1")]
    return base or DEFAULT_VISION_URL


def _vision_model() -> str:
    return os.environ.get("WASSIT_VISION_MODEL", "").strip() or DEFAULT_VISION_MODEL


def _fetch_photo(photo_id: str) -> tuple[bytes, dict | None]:
    """Downloads the photo file with the Agent credential. Returns (bytes, None) or (b"", tool error)."""
    url = _http._base_url() + f"/api/v1/photos/{quote(photo_id, safe='')}/file"
    req = urllib.request.Request(url, method="GET", headers={"Authorization": f"Agent {_http._token()}"})
    try:
        with urllib.request.urlopen(req, timeout=_http.TIMEOUT_SECONDS) as resp:
            data = resp.read(MAX_PHOTO_BYTES + 1)
    except urllib.error.HTTPError as e:
        try:
            parsed = json.loads(e.read() or b"{}")
        except Exception:
            parsed = {}
        err = _http.tool_error(parsed) if isinstance(parsed, dict) else None
        return b"", err or {"error": f"The backend answered HTTP {e.code} for this photo file.", "code": f"http_{e.code}"}
    except (urllib.error.URLError, TimeoutError, ConnectionError, OSError) as e:
        reason = getattr(e, "reason", None) or str(e)
        return b"", {"error": f"Could not reach the backend: {reason}"[:300], "code": "backend_unreachable"}
    if not data:
        return b"", {"error": "The photo file is empty; ask the pharmacist to upload it again.", "code": "empty_photo"}
    if len(data) > MAX_PHOTO_BYTES:
        return b"", {"error": "The photo file is larger than 10 MB and was not read.", "code": "photo_too_large"}
    return data, None


def _vision(image: bytes) -> str:
    """Asks the vision model for the reading. Returns its raw text; raises on any transport failure."""
    body = {
        "model": _vision_model(),
        "stream": False,
        "think": False,
        "options": {"temperature": 0},
        "messages": [{"role": "user", "content": VISION_PROMPT, "images": [base64.b64encode(image).decode("ascii")]}],
    }
    req = urllib.request.Request(_vision_url() + "/api/chat", data=json.dumps(body).encode("utf-8"),
                                 method="POST", headers={"Content-Type": "application/json"})
    with urllib.request.urlopen(req, timeout=VISION_TIMEOUT_SECONDS) as resp:
        out = json.loads(resp.read())
    return str((out.get("message") or {}).get("content") or "")


def _parse_reading(text: str) -> dict | None:
    """Turns the model's text into {code, expiry_month, confidence}, or None when it is not usable JSON."""
    match = re.search(r"\{.*\}", text or "", re.DOTALL)  # ignores code fences and chatter around the object
    if not match:
        return None
    try:
        raw = json.loads(match.group(0))
    except ValueError:
        return None
    if not isinstance(raw, dict):
        return None
    code = re.sub(r"\D", "", str(raw.get("code") or ""))[:32]
    expiry = str(raw.get("expiry_month") or "").strip()
    if not re.fullmatch(r"(19|20)\d\d-(0[1-9]|1[0-2])", expiry):
        expiry = ""
    try:
        confidence = float(raw.get("confidence"))
    except (TypeError, ValueError):
        confidence = 0.0
    if confidence != confidence:  # NaN
        confidence = 0.0
    confidence = round(min(1.0, max(0.0, confidence)), 2)
    if not code and not expiry:
        confidence = 0.0  # nothing was read, whatever the model claims
    return {"code": code, "expiry_month": expiry, "confidence": confidence}


def _note(out: dict) -> str:
    for key in ("note", "review_note", "reason", "message"):
        if isinstance(out.get(key), str) and out[key]:
            return out[key][:400]
    reasons = out.get("reasons")
    if isinstance(reasons, list):
        return "; ".join(str(r) for r in reasons[:4])[:400]
    return ""


@tool
def read_package_photo(photo_id: Annotated[str, "Package photo id from the enquiry state"]) -> dict:
    """Reads the product code and expiry printed on one package photo and hands the reading to the backend. The backend rules compare it with what the pharmacist typed and the government reference and decide; a mismatch or an unreadable photo goes to a human reviewer. A photo alone never proves a box is genuine, so never tell anyone it does."""
    photo_id = str(photo_id or "").strip()
    if not photo_id:
        return {"error": "photo_id is required: use a package photo id from the enquiry state", "code": "validation_failed"}
    image, err = _fetch_photo(photo_id)
    if err is not None:
        return err

    problem = ""
    reading = None
    try:
        reading = _parse_reading(_vision(image))
        if reading is None:
            problem = "the vision model's answer was not readable"
    except Exception as e:  # unreachable, timeout, bad body: the photo still goes to a human
        problem = f"the vision model could not be reached ({type(e).__name__})"
    if reading is None:
        reading = {"code": "", "expiry_month": "", "confidence": 0.0}

    # Only the reading is sent: the agent never sends a verdict.
    out = _http.call("POST", f"/api/v1/photos/{quote(photo_id, safe='')}/agent-reading", reading)
    err = _http.tool_error(out)
    if err is not None:
        return err
    photo = out.get("photo") if isinstance(out.get("photo"), dict) else out
    result = {
        "photo_id": photo_id,
        "read": reading,
        "decision": str(photo.get("status") or photo.get("decision") or "")[:40],
        "decided_by": str(photo.get("decided_by") or "")[:40],
        "note": _note(photo),
    }
    if problem:
        result["warning"] = (f"Nothing was read from the photo: {problem}. An empty reading with confidence 0 was sent "
                             "so a human reviewer checks it. Do not retry and do not describe the box as checked.")
    return result
