import json
import os
import sys
from typing import Annotated, Literal
from urllib.parse import quote

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))  # lets this file import _http when Cornilia loads it

import _http  # noqa: E402
from cornilia.tools import tool  # noqa: E402

TEXT_LIMIT = 300  # long free text is cut so one tool result stays small


def _enq(enquiry_id: str) -> str:
    return f"/api/v1/enquiries/{quote(str(enquiry_id), safe='')}"


def _target(target_id: str) -> str:
    return f"/api/v1/targets/{quote(str(target_id), safe='')}"


def _cut(value) -> str:
    text = "" if value is None else str(value)
    return text if len(text) <= TEXT_LIMIT else text[:TEXT_LIMIT] + "..."


def _medication_label(detail: dict) -> str | None:
    med = detail.get("medication")
    if not isinstance(med, dict) or not med.get("brand_name"):
        return None
    parts = [med.get("brand_name"), med.get("strength"), med.get("dosage_form")]
    return " ".join(str(p) for p in parts if p)


def _latest_enquiry_message_status(messages: list, target_id: str) -> str | None:
    mine = [m for m in messages if m.get("target_id") == target_id and m.get("kind") == "enquiry"]
    if not mine:
        return None
    mine.sort(key=lambda m: str(m.get("created_at", "")))
    return mine[-1].get("status")


MAX_RESULT_CHARS = 1900  # the tool result must stay under 2000 characters of JSON
RARE_STOCK_THRESHOLD = 3
IMPORTER_RETURN_CAP = 4


def _compute_candidate_flags(reported_qty: int | None, requested_qty: int = 1) -> list[str]:
    """Computes deterministic B2B inventory shield & trade mode flags for a candidate branch."""
    qty = int(reported_qty or 0)
    req = max(1, int(requested_qty or 1))
    flags: list[str] = []
    if qty <= RARE_STOCK_THRESHOLD or qty < req:
        flags.append("RARE_PROTECT")
    if 0 < qty <= RARE_STOCK_THRESHOLD:
        flags.append("EXCHANGE_ONLY")
    if qty > IMPORTER_RETURN_CAP:
        flags.append("NEAR_EXPIRY_PUSH")
    return flags


def _compute_enquiry_policy_flags(detail: dict, targets: list[dict]) -> list[str]:
    """Computes deterministic B2B policy flags for the enquiry state (< 2000 chars budget)."""
    req_qty = max(1, int(detail.get("quantity") or 1))
    med = detail.get("medication") if isinstance(detail.get("medication"), dict) else {}
    flags: list[str] = []
    if med.get("chronic_eligible") or any(
        w in str(detail.get("raw_request") or "").lower()
        for w in ("concor", "glucophage", "lipitor", "ventolin", "chronic", "refill")
    ):
        flags.append("CHRONIC_BUFFER")
    avail_Common = [
        int(t.get("reply_quantity") or 0)
        for t in targets
        if t.get("status") == "available" and t.get("reply_quantity") is not None
    ]
    if any(0 < q <= RARE_STOCK_THRESHOLD for q in avail_Common) or req_qty >= IMPORTER_RETURN_CAP:
        flags.append("RARE_PROTECT")
        flags.append("EXCHANGE_ONLY")
    if any(q > IMPORTER_RETURN_CAP for q in avail_Common):
        flags.append("NEAR_EXPIRY_PUSH")
    return flags


def _next_step(out: dict, messages: list) -> str:
    """Computes the next allowed step from the state (code decides what is allowed; the model decides)."""
    status = out.get("status")
    targets = [t for t in out.get("targets", []) if t.get("status") != "cancelled"]
    if status in ("closed", "cancelled"):
        return "Nothing to do: the enquiry is closed."
    if status == "awaiting_clarification":
        return "Waiting for the requester's answer: call wait(5)."
    if not out.get("medication"):
        return "Identify the medicine: search_medications, then set_medication if exactly one strength/form matches; otherwise ask_requester."
    if not targets:
        return "find_candidates, then propose_targets with the 3 best candidates, then wait(5)."
    avail = [t for t in targets if t["status"] == "available"]
    if avail:
        return f"A pharmacy replied available: close_enquiry outcome 'available' with branch_id of {avail[0]['pharmacy']} (branch_id {avail[0].get('branch_id')})."
    if any(t["status"] == "approved" for t in targets):
        return "The requester approved targets: call send_enquiries now, then wait(5)."
    if any(t["status"] == "proposed" for t in targets):
        return "Waiting for the requester to approve the proposed pharmacies: call wait(5)."
    notified = " ".join(m.get("body", "") for m in messages if m.get("kind") == "notification")
    alt = [t for t in targets if t["status"] == "alternative_offered" and (t.get("pharmacy") or "") not in notified]
    if alt:
        return f"{alt[0]['pharmacy']} offered an alternative ({alt[0].get('reply_text')}). It is NOT a match: notify_requester naming {alt[0]['pharmacy']}, then continue."
    pending = [t for t in targets if t["status"] == "sent"]
    if pending:
        undelivered = [t for t in pending if t.get("message") != "sent"]
        if undelivered:
            return "Messages are still in the undo window: call wait(5)."
        no_fu = [t for t in pending if t.get("follow_ups", 0) == 0]
        if no_fu:
            return "Still silent: follow_up " + ", ".join(t["target_id"] for t in no_fu) + " (once each), then wait(5)."
        return ("Followed up already. If you have not waited since the follow-up, call wait(5) now. If this run was woken by "
                "the timer after that wait, mark_no_response " + ", ".join(t["target_id"] for t in pending) + ".")
    if all(t["status"] == "unavailable" for t in targets):
        return "Every pharmacy replied unavailable: close_enquiry outcome 'unavailable'."
    return "All pharmacies answered or are no_response and none is available: close_enquiry outcome 'unable_to_confirm'."


def _compact_enquiry(detail: dict) -> dict:
    messages = [m for m in detail.get("messages", []) or [] if isinstance(m, dict)]
    targets_raw = detail.get("targets", []) or []
    if isinstance(detail.get("enquiry"), dict):  # backend shape: {"enquiry": {...}, "targets": [...], ...}
        detail = {**detail, **detail["enquiry"]}
    question = detail.get("clarification_question") or ""
    answer = detail.get("clarification_answer") or ""
    targets = []
    for t in targets_raw:
        if not isinstance(t, dict):
            continue
        target_id = str(t.get("id", ""))
        targets.append({
            "target_id": target_id,
            "branch_id": t.get("branch_id"),
            "pharmacy": t.get("branch_name"),
            "status": t.get("status"),
            "reply_quantity": t.get("reply_quantity"),
            "reply_text": _cut(t.get("reply_text")),
            "follow_ups": t.get("follow_ups", 0),
            "message": _latest_enquiry_message_status(messages, target_id),
        })
    out = {
        "enquiry_id": detail.get("id"),
        "status": detail.get("status"),
        "raw_request": _cut(detail.get("raw_request")),
        "quantity": detail.get("quantity"),
        "medication": _medication_label(detail),
        "clarification": {"question": _cut(question), "answer": _cut(answer)} if question else None,
        "targets": targets,
        "notified_requester": sum(1 for m in messages if m.get("kind") == "notification"),
        "outcome": detail.get("outcome") or "",
        "policy_flags": _compute_enquiry_policy_flags(detail, targets),
    }
    out["next_step"] = _next_step(out, messages)
    # hard guard: drop trailing targets (and say so) until the result fits the size budget
    omitted = 0
    while len(json.dumps(out)) > MAX_RESULT_CHARS and out["targets"]:
        out["targets"].pop()
        omitted += 1
    if omitted:
        out["targets_omitted"] = omitted
    return out


@tool
def get_enquiry(enquiry_id: Annotated[str, "The enquiry id from the trigger"]) -> dict:
    """Reads one enquiry with its targets and their latest status. Use it at the start of every run and after each wake-up to see the current state."""
    out = _http.call("GET", _enq(enquiry_id))
    error = _http.tool_error(out)
    if error:
        return error
    return _compact_enquiry(out)


@tool
def set_medication(
    enquiry_id: Annotated[str, "The enquiry id"],
    medication_id: Annotated[str, "The medication_id from search_medications"],
) -> dict:
    """Links the enquiry to one catalogue medicine. Use it after search_medications found exactly one clear match."""
    out = _http.call("POST", _enq(enquiry_id) + "/medication", body={"medication_id": medication_id})
    error = _http.tool_error(out)
    if error:
        return error
    return {"ok": True, "enquiry_id": enquiry_id, "medication_id": medication_id}


@tool
def ask_requester(
    enquiry_id: Annotated[str, "The enquiry id"],
    question: Annotated[str, "One short question for the requester, in the language they wrote in"],
) -> dict:
    """Asks the requester a question when the request is ambiguous. Use it instead of guessing a medicine, strength or quantity; the run ends after this."""
    out = _http.call("POST", _enq(enquiry_id) + "/clarification-request", body={"question": question})
    error = _http.tool_error(out)
    if error:
        return error
    return {
        "ok": True,
        "status": out.get("status"),
        "note": "STOP: the question is sent. Do not call any more tools; end this run now with a one-line summary. You will be woken when the requester answers.",
    }


@tool
def find_candidates(
    enquiry_id: Annotated[str, "The enquiry id"],
    limit: Annotated[int, "How many pharmacies to return, 1 to 10"] = 5,
) -> dict:
    """Ranks the participating pharmacies that could hold the medicine. Use it after the medicine is set, before propose_targets."""
    limit = max(1, min(int(limit), 10))
    out = _http.call("GET", _enq(enquiry_id) + "/candidates", query={"limit": limit})
    error = _http.tool_error(out)
    if error:
        return error
    candidates = []
    policy_summary = {}
    for c in out.get("data", out.get("candidates", [])) or []:
        if not isinstance(c, dict):
            continue
        reported = c.get("reported_quantity", c.get("available_quantity"))
        bid = c.get("branch_id")
        candidates.append({
            "branch_id": _cut(bid),
            "name": _cut(c.get("name")),
            "distance_km": c.get("distance_km"),
            "has_reported_stock": bool(c.get("has_reported_stock", reported)),
            "reported_quantity": reported,
        })
        if bid:
            policy_summary[_cut(bid)] = _compute_candidate_flags(reported)
    res = {"candidates": candidates, "policy_flags_by_branch": policy_summary}
    while len(res["candidates"]) > 1 and len(json.dumps(res)) > MAX_RESULT_CHARS:
        removed = res["candidates"].pop()
        res["policy_flags_by_branch"].pop(removed["branch_id"], None)
    return res


@tool
def propose_targets(
    enquiry_id: Annotated[str, "The enquiry id"],
    branch_ids: Annotated[list[str], "branch_ids chosen from find_candidates"],
) -> dict:
    """Proposes pharmacies to ask. Use it with branch_ids from find_candidates; the requester must approve before anything is sent."""
    out = _http.call("POST", _enq(enquiry_id) + "/targets", body={"branch_ids": branch_ids})
    error = _http.tool_error(out)
    if error:
        return error
    proposed = []
    for t in out.get("data", out.get("proposed", out.get("targets", []))) or []:
        if not isinstance(t, dict):
            continue
        proposed.append({
            "target_id": t.get("target_id", t.get("id")),
            "branch_id": t.get("branch_id"),
            "status": t.get("status"),
        })
    return {
        "proposed": proposed,
        "note": "The requester must approve before sending. Call wait().",
    }


@tool
def send_enquiries(enquiry_id: Annotated[str, "The enquiry id"]) -> dict:
    """Sends the enquiry to the approved pharmacies. Use it only after the requester approved the targets."""
    out = _http.call("POST", _enq(enquiry_id) + "/send")
    error = _http.tool_error(out)
    if error:
        return error
    queued = out.get("queued")
    if not isinstance(queued, int):
        queued = len(out.get("data", out.get("messages", [])) or [])
    return {"queued": queued, "note": "Messages leave after the undo window."}


@tool
def follow_up(target_id: Annotated[str, "The target_id of a pharmacy that has not replied"]) -> dict:
    """Sends one follow-up to a pharmacy that has not replied. Use it once per target, before mark_no_response."""
    out = _http.call("POST", _target(target_id) + "/follow-up")
    error = _http.tool_error(out)
    if error:
        return error
    return {"ok": True, "target_id": target_id, "status": out.get("status")}


@tool
def mark_no_response(target_id: Annotated[str, "The target_id of a pharmacy that stayed silent after a follow-up"]) -> dict:
    """Marks a pharmacy as no_response after its follow-up got no reply. Never use it for a pharmacy that replied."""
    out = _http.call("POST", _target(target_id) + "/no-response")
    error = _http.tool_error(out)
    if error:
        return error
    return {"ok": True, "target_id": target_id, "status": out.get("status")}


@tool
def notify_requester(
    enquiry_id: Annotated[str, "The enquiry id"],
    message: Annotated[str, "A short factual update for the requester, e.g. an alternative a pharmacy offered"],
) -> dict:
    """Tells the requester something they must know, such as a pharmacy offering a different strength or form. An alternative is reported here and never counted as a match."""
    out = _http.call("POST", _enq(enquiry_id) + "/notify", body={"body": message})
    error = _http.tool_error(out)
    if error:
        return error
    return {"ok": True}


@tool
def wait(
    enquiry_id: Annotated[str, "The enquiry id"],
    minutes: Annotated[Literal[5, 15, 30], "How long to wait before checking again: 5, 15 or 30"],
) -> dict:
    """Ends this run and schedules the next check. Use it whenever pharmacies are still pending and nothing else can be done now."""
    out = _http.call("POST", _enq(enquiry_id) + "/wait", body={"minutes": minutes})
    error = _http.tool_error(out)
    if error:
        return error
    return {"ok": True, "status": out.get("status"), "next_wake_at": out.get("next_wake_at"),
            "note": "STOP: waiting is set. Do not call any more tools; end this run now with a one-line summary."}


@tool
def close_enquiry(
    enquiry_id: Annotated[str, "The enquiry id"],
    outcome: Annotated[Literal["available", "unavailable", "unable_to_confirm"], "The final outcome"],
    branch_id: Annotated[str | None, "For outcome available: the branch_id that has the medicine"] = None,
) -> dict:
    """Closes the enquiry with a final outcome. Use it when every pharmacy has answered or been marked no_response; the server refuses a close that contradicts the replies."""
    out = _http.call("POST", _enq(enquiry_id) + "/close", body={"outcome": outcome, "branch_id": branch_id})
    error = _http.tool_error(out)
    if error:
        return error
    return {"ok": True, "outcome": outcome, "status": out.get("status"), "note": "STOP: the enquiry is closed. End this run now."}
