"""Deterministic Wassit policy, no model. The fallback when the model is slow or broken.

It calls the same agent tools as the model (their python functions, via `.function`), so every
action goes through the same backend API and the same tool-call trace.

decide(state, call) reads the raw enquiry detail (GET /api/v1/enquiries/{id}) and calls
call(name, **kwargs) for each step. It returns the tool names it called, in order.
Each decide() run ends with wait(), close_enquiry() or ask_requester().
"""

import re
import sys
from pathlib import Path

TOOLS_DIR = Path(__file__).resolve().parent / "tools"
if str(TOOLS_DIR) not in sys.path:
    sys.path.append(str(TOOLS_DIR))

import catalogue  # noqa: E402
import enquiry  # noqa: E402

TOOL_FUNCTIONS = {
    "search_medications": catalogue.search_medications.function,
    "set_medication": enquiry.set_medication.function,
    "ask_requester": enquiry.ask_requester.function,
    "find_candidates": enquiry.find_candidates.function,
    "propose_targets": enquiry.propose_targets.function,
    "send_enquiries": enquiry.send_enquiries.function,
    "follow_up": enquiry.follow_up.function,
    "mark_no_response": enquiry.mark_no_response.function,
    "notify_requester": enquiry.notify_requester.function,
    "wait": enquiry.wait.function,
    "close_enquiry": enquiry.close_enquiry.function,
}

CLOSED_STATUSES = {"closed", "cancelled"}
WAIT_MINUTES = 5
CANDIDATE_LIMIT = 5
PROPOSE_LIMIT = 3
QUANTITY_WORDS = {"x", "×", "box", "boxes", "pack", "packs", "pcs", "pc", "piece", "pieces",
                  "unit", "units", "qty", "quantity"}

# quantity phrases that must not be read as strengths: "x2", "2 tabs", "2 boxes"
_QUANTITY_PATTERNS = [
    re.compile(r"(?<![A-Za-z])[x×]\s*\d+\b", re.IGNORECASE),
    re.compile(r"\b\d+\s*(?:tabs?|tablets?|caps?|capsules?|boxes|box|packs?|pcs|pieces|units?)\b",
               re.IGNORECASE),
]
_STRENGTH = re.compile(r"(\d+(?:[.,]\d+)?)\s*(mcg|mg|g|ml|iu)?", re.IGNORECASE)


def run_tool(name: str, **kwargs) -> dict:
    """Runs one agent tool by name through its python function."""
    return TOOL_FUNCTIONS[name](**kwargs)


def _strengths(text: str) -> set[float]:
    """Strength numbers in mg (1g is 1000). Quantities are removed first."""
    cleaned = str(text or "")
    for pattern in _QUANTITY_PATTERNS:
        cleaned = pattern.sub(" ", cleaned)
    values: set[float] = set()
    for number, unit in _STRENGTH.findall(cleaned):
        value = float(number.replace(",", "."))
        if unit.lower() == "g":
            value *= 1000
        values.add(value)
    return values


def _search_query(raw_request: str) -> str:
    """The request without numbers and quantity words. The strength is chosen by code afterwards."""
    words = []
    for word in str(raw_request or "").split():
        cleaned = word.strip(".,;:!?()[]\"'")
        if not cleaned or any(ch.isdigit() for ch in cleaned):
            continue
        if cleaned.lower() in QUANTITY_WORDS:
            continue
        words.append(cleaned)
    return " ".join(words) or str(raw_request or "").strip()


def _strength_question(matches: list[dict]) -> str:
    strengths = []
    for m in matches:
        strength = str(m.get("strength") or "").strip()
        if strength and strength not in strengths:
            strengths.append(strength)
    if not strengths:
        return "Which medicine exactly? Please send the name, strength and form."
    return f"Which strength do you need? Options: {', '.join(strengths)}. Reply with the one you want."


def _choose_medication(enq: dict, eid, do) -> str | None:
    """Sets the medication when code finds exactly one match. Otherwise asks the requester. Returns the medication_id or None."""
    raw = str(enq.get("raw_request") or "")
    question = str(enq.get("clarification_question") or "")
    answer = str(enq.get("clarification_answer") or "").strip()
    if question and not answer:
        return None  # asked already, the requester has not answered yet

    result = do("search_medications", query=_search_query(raw))
    if "error" in result:
        do("wait", enquiry_id=eid, minutes=WAIT_MINUTES)
        return None
    matches = [m for m in result.get("matches", []) if isinstance(m, dict) and m.get("medication_id")]

    wanted = _strengths(answer if answer else raw)
    picks = [m for m in matches if _strengths(m.get("strength")) & wanted]
    if len(picks) == 1:
        medication_id = picks[0]["medication_id"]
        do("set_medication", enquiry_id=eid, medication_id=medication_id)
        return medication_id

    do("ask_requester", enquiry_id=eid, question=_strength_question(matches))
    return None


def _pharmacy(target: dict) -> str:
    return str(target.get("branch_name") or target.get("pharmacy") or target.get("branch_id") or "A pharmacy")


def _has_message(messages: list[dict], kind: str, status: str | None = None, target_id=None, body_has: str | None = None) -> bool:
    for m in messages:
        if m.get("kind") != kind:
            continue
        if status is not None and m.get("status") != status:
            continue
        if target_id is not None and m.get("target_id") != target_id:
            continue
        if body_has is not None and body_has not in str(m.get("body") or ""):
            continue
        return True
    return False


def decide(state: dict, call) -> list[str]:
    """One deterministic run for one enquiry. call(name, **kwargs) runs a tool and returns its result dict."""
    names: list[str] = []

    def do(name: str, **kwargs):
        names.append(name)
        return call(name, **kwargs)

    enq = state.get("enquiry") if isinstance(state.get("enquiry"), dict) else state
    eid = enq.get("id")
    if enq.get("status") in CLOSED_STATUSES:
        return names

    if not state.get("medication"):
        medication_id = _choose_medication(enq, eid, do)
        if medication_id is None:
            return names
        state = {**state, "medication": {"id": medication_id}}

    targets = [t for t in (state.get("targets") or []) if isinstance(t, dict)]
    messages = [m for m in (state.get("messages") or []) if isinstance(m, dict)]
    active = [t for t in targets if t.get("status") != "cancelled"]

    if not active:
        found = do("find_candidates", enquiry_id=eid, limit=CANDIDATE_LIMIT)
        if "error" in found:
            do("wait", enquiry_id=eid, minutes=WAIT_MINUTES)
            return names
        candidates = found.get("candidates", [])
        branch_ids = [c["branch_id"] for c in candidates if isinstance(c, dict) and c.get("branch_id")]
        if not branch_ids:
            do("close_enquiry", enquiry_id=eid, outcome="unable_to_confirm")
            return names
        do("propose_targets", enquiry_id=eid, branch_ids=branch_ids[:PROPOSE_LIMIT])
        do("wait", enquiry_id=eid, minutes=WAIT_MINUTES)
        return names

    available = [t for t in active if t.get("status") == "available"]
    if available:
        do("close_enquiry", enquiry_id=eid, outcome="available", branch_id=available[0].get("branch_id"))
        return names

    pending_alternatives = [
        t for t in active
        if t.get("status") == "alternative_offered"
        and not _has_message(messages, "notification", body_has=_pharmacy(t))
    ]
    if pending_alternatives:
        for t in pending_alternatives:
            reply = str(t.get("reply_text") or "").strip() or "(no details)"
            do(
                "notify_requester",
                enquiry_id=eid,
                message=f"{_pharmacy(t)} offers an alternative: {reply}. Not a match; tell us if acceptable.",
            )
        do("wait", enquiry_id=eid, minutes=WAIT_MINUTES)
        return names

    # approved before proposed, so an approved target is sent even while another one is still proposed
    if any(t.get("status") == "approved" for t in active):
        do("send_enquiries", enquiry_id=eid)
        do("wait", enquiry_id=eid, minutes=WAIT_MINUTES)
        return names

    if any(t.get("status") == "proposed" for t in active):
        do("wait", enquiry_id=eid, minutes=WAIT_MINUTES)
        return names

    acted = False
    for t in active:
        if t.get("status") != "sent":
            continue
        target_id = t.get("id")
        if not _has_message(messages, "enquiry", status="sent", target_id=target_id):
            continue
        if int(t.get("follow_ups") or 0) == 0:
            do("follow_up", target_id=target_id)
            acted = True
        elif _has_message(messages, "follow_up", status="sent", target_id=target_id):
            do("mark_no_response", target_id=target_id)
            acted = True
    if acted:
        do("wait", enquiry_id=eid, minutes=WAIT_MINUTES)
        return names

    statuses = {t.get("status") for t in active}
    if statuses == {"unavailable"}:
        do("close_enquiry", enquiry_id=eid, outcome="unavailable")
    elif statuses <= {"unavailable", "alternative_offered", "no_response"}:
        do("close_enquiry", enquiry_id=eid, outcome="unable_to_confirm")
    else:
        do("wait", enquiry_id=eid, minutes=WAIT_MINUTES)
    return names
