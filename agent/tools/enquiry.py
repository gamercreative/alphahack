import json
import os
import re
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


REPLIED_STATUSES = ("available", "unavailable", "alternative_offered")
NOT_A_MATCH = ("escalated", "rejected")
CLIENT_DESK_BRANCH = "branch-client-desk"  # the client reserves the offer; the agent never closes these as available
_STRENGTH_RE = re.compile(r"(\d+(?:[.,]\d+)?)\s*(mcg|mg|g|ml)\b", re.IGNORECASE)
_TO_MG = {"mcg": 0.001, "mg": 1.0, "g": 1000.0}


def _strengths(text) -> list[tuple[str, float, str]]:
    """Finds strengths in free text as (as written, value, unit), with g and mcg turned into mg."""
    found = []
    for number, unit in _STRENGTH_RE.findall(str(text or "")):
        unit = unit.lower()
        value = float(number.replace(",", "."))
        if unit in _TO_MG:
            value, unit = value * _TO_MG[unit], "mg"
        found.append((f"{number}{unit if unit != 'mg' else 'mg'}", value, unit))
    return found


def _other_strength(medication, reply_text) -> str | None:
    """Returns the first strength named in the reply that differs from the requested one, or None."""
    wanted = {(v, u) for _, v, u in _strengths(medication)}
    if not wanted:
        return None
    for written, value, unit in _strengths(reply_text):
        if (value, unit) not in wanted:
            return written
    return None


def _needs_offer(target: dict) -> bool:
    return (target.get("status") in REPLIED_STATUSES and bool(str(target.get("reply_text") or "").strip())
            and not target.get("offer"))


def _next_step(out: dict, messages: list) -> str:
    """Computes the next allowed step from the state (code decides what is allowed; the model decides)."""
    status = out.get("status")
    targets = [t for t in out.get("targets", []) if t.get("status") != "cancelled"]
    if status in ("closed", "cancelled"):
        return "Nothing to do: the enquiry is closed."
    if status == "awaiting_clarification":
        return "Waiting for the requester's answer: call wait(5)."
    if not out.get("medication") and out.get("client_desk"):
        return ("Identify the medicine: search_medications, then set_medication if exactly one strength/form matches; otherwise "
                "ask_requester about the medicine only (strength, form, quantity). Never ask who the client is.")
    if not out.get("medication"):
        return "Identify the medicine: search_medications, then set_medication if exactly one strength/form matches; otherwise ask_requester."
    if not targets:
        return "find_candidates, then propose_targets with the 3 best candidates, then stop: the requester must approve them (do not call wait)."
    photos = out.get("photos_to_read") or []
    if photos:
        return f"read_package_photo(photo_id) for photo {photos[0]}: the rules check the typed code and expiry; a photo alone proves nothing."
    unread = [t for t in targets if _needs_offer(t)]
    if unread:
        t = unread[0]
        step = (f"record_offer for target {t['target_id']} ({t.get('pharmacy')}): read the reply into fields with exact quotes; "
                "the rules decide.")
        other = _other_strength(out.get("medication"), t.get("reply_text"))
        if other:
            step += (f" The reply names {other}, not the requested strength: call check_reference(ingredient, '{other}', form) "
                     "first (information only).")
        return step
    if status == "awaiting_approval":
        return "Stop now: the requester must approve the proposed pharmacies. Do not call wait(); end this run with a one-line summary."
    avail = [t for t in targets if t["status"] == "available" and t.get("offer") not in NOT_A_MATCH]
    notified = " ".join(m.get("body", "") for m in messages if m.get("kind") == "notification")
    if out.get("client_desk"):
        # Contract rule 6/11: a client ticket closes 'available' only at a branch with a verified offer; Go enforces it.
        unverified = [t for t in avail if t.get("offer") != "matches"]
        avail = [t for t in avail if t.get("offer") == "matches"]
        if avail:
            return (f"{avail[0]['pharmacy']} has a rules-accepted offer: close_enquiry outcome 'available' with branch_id "
                    f"{avail[0].get('branch_id')}. Go accepts it only if the offer is verified; if it refuses, "
                    "notify_requester in plain words (no advice, no other medicine) and wait(5).")
        fresh = [t for t in unverified if (t.get("pharmacy") or "") not in notified]
        if fresh:
            return (f"{fresh[0]['pharmacy']} says it has it but there is no verified offer yet: notify_requester on the client "
                    f"ticket naming {fresh[0]['pharmacy']} in plain words (no advice, no other medicine), then wait(5). "
                    "Do NOT close as available.")
    if avail:
        return f"A pharmacy replied available: close_enquiry outcome 'available' with branch_id of {avail[0]['pharmacy']} (branch_id {avail[0].get('branch_id')})."
    if any(t["status"] == "approved" for t in targets):
        return "The requester approved targets: call send_enquiries now, then wait(5)."
    if any(t["status"] == "proposed" for t in targets):
        return "Stop now: the requester must approve the proposed pharmacies. Do not call wait(); end this run with a one-line summary."
    after = _after_replies(targets)
    alt = [t for t in targets if t["status"] == "alternative_offered" and (t.get("pharmacy") or "") not in notified]
    if alt and alt[0].get("offer") == "matches":
        return (f"The rules checked the offer from {alt[0]['pharmacy']} ({alt[0].get('reply_text')}) as a dose equivalent: "
                f"notify_requester naming {alt[0]['pharmacy']} and what they offer. Do NOT close as available "
                "on an alternative. Then, in the same run: " + after)
    if alt:
        return (f"{alt[0]['pharmacy']} offered an alternative ({alt[0].get('reply_text')}). It is NOT a match: "
                f"notify_requester naming {alt[0]['pharmacy']}. Then, in the same run: " + after)
    return after


def _after_replies(targets: list) -> str:
    """The step once every reply is read and reported: chase silent pharmacies, then close."""
    pending = [t for t in targets if t["status"] == "sent"]
    if pending:
        undelivered = [t for t in pending if t.get("message") != "sent"]
        if undelivered:
            return "Messages are still in the undo window: call wait(5)."
        no_fu = [t for t in pending if t.get("follow_ups", 0) == 0]
        if no_fu:
            return "Still silent: follow_up " + ", ".join(t["target_id"] for t in no_fu) + " (once each), then wait(5)."
        ids = ", ".join(t["target_id"] for t in pending)
        return ("Followed up already. If you have not waited since the follow-up, call wait(5) now. If this run was woken by "
                f"the timer after that wait, mark_no_response {ids}, then get_enquiry and follow next_step in the same run.")
    if targets and all(t["status"] == "unavailable" for t in targets):
        return "Every pharmacy replied unavailable: close_enquiry outcome 'unavailable'."
    return "All pharmacies answered or are no_response and none is available: close_enquiry outcome 'unable_to_confirm'."


def _offer_decisions(enquiry_id, targets_raw: list) -> dict:
    """Maps target_id to the rules decision of its latest recorded offer. Asks the backend only when a reply has text."""
    if not any(isinstance(t, dict) and t.get("status") in REPLIED_STATUSES and str(t.get("reply_text") or "").strip()
               for t in targets_raw):
        return {}
    listed = _http.call("GET", _enq(enquiry_id) + "/offers")
    rows = listed.get("data") if isinstance(listed, dict) else None
    decisions = {}
    for row in sorted((r for r in rows or [] if isinstance(r, dict)), key=lambda r: str(r.get("created_at", ""))):
        decisions[str(row.get("target_id", ""))] = row.get("decision") or "recorded"
    return decisions


PHOTO_DECIDED = ("verified", "flagged", "mismatch", "rejected")


def _photos_to_read(detail: dict, targets_raw: list) -> list[str]:
    """Ids of package photos nobody has read yet (from the detail's photos list, a target's photos, or the photo wake)."""
    rows = list(detail.get("photos") or [])
    for t in targets_raw:
        if isinstance(t, dict):
            rows += list(t.get("photos") or [])
    ids = [str(p["id"]) for p in rows if isinstance(p, dict) and p.get("id") and p.get("status") not in PHOTO_DECIDED]
    wake_photo = detail.get("wake_photo_id")
    if not ids and wake_photo and detail.get("wake_trigger") == "photo":
        ids = [str(wake_photo)]
    return ids[:3]


def _compact_enquiry(detail: dict, offers: dict | None = None) -> dict:
    offers = offers or {}
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
        if offers.get(target_id):
            targets[-1]["offer"] = offers[target_id]  # the rules decision: matches, escalated or rejected
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
    if detail.get("requester_branch_id") == CLIENT_DESK_BRANCH:
        out["client_desk"] = True  # a client ticket: notify texts go to the client, never ask who they are
    to_read = _photos_to_read(detail, targets_raw)
    if to_read:
        out["photos_to_read"] = to_read
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
    enquiry = out.get("enquiry") if isinstance(out.get("enquiry"), dict) else out
    return _compact_enquiry(out, _offer_decisions(enquiry.get("id") or enquiry_id, out.get("targets", []) or []))


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
        "note": "Stop now; the requester (or the rules for client-desk tickets) approves the pharmacies. Do not call wait().",
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
