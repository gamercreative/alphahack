import json
import re
import os
import sys
from typing import Annotated, Literal

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))  # lets this file import _http when Cornilia loads it

import _http  # noqa: E402
from cornilia.tools import tool  # noqa: E402

MAX_MATCHES = 5
MAX_POLICY_CHARS = 1900
IMPORTER_RETURN_CAP = 4
RARE_STOCK_THRESHOLD = 3
NEAR_EXPIRY_DAYS = 180

POLICY_FLAGS = (
    "RARE_PROTECT",
    "CHRONIC_BUFFER",
    "EXCHANGE_ONLY",
    "NEAR_EXPIRY_PUSH",
)

FRANCO_FILLER_WORDS = {
    "badde", "bade", "baddi", "3ande", "3andi", "3elab", "3elbe", "3elbet",
    "علبة", "علب", "بدي", "عندي", "حبة", "حبوب",
}

FRANCO_FORM_ALIASES = {
    "habb": "tablet",
    "habbe": "tablet",
    "tabs": "tablet",
    "tab": "tablet",
    "kabsoule": "capsule",
    "caps": "capsule",
    "sharab": "syrup",
    "bakhakh": "inhaler",
    "ebre": "injection",
    "marham": "cream",
    "no2at": "drops",
}


def normalize_intake_query(query: str, form: str | None = None) -> tuple[list[str], str | None]:
    """Extracts clean search tokens and optional dosage form from Franco-Arabic/shorthand queries without guessing clinical substitutes."""
    inferred_form = form
    tokens: list[str] = []
    for raw_token in re.findall(r"[^\W_]+", str(query or "")):
        low = raw_token.lower()
        if low in FRANCO_FORM_ALIASES and inferred_form is None:
            inferred_form = FRANCO_FORM_ALIASES[low]
            continue
        if low in FRANCO_FILLER_WORDS:
            continue
        cleaned_alpha = re.sub(r"\d+", "", raw_token)
        if len(cleaned_alpha) >= 4 and cleaned_alpha.lower() not in FRANCO_FILLER_WORDS:
            tokens.append(cleaned_alpha)
    return tokens[:6], inferred_form


def evaluate_inventory_shield(
    batches: list[dict],
    active_refills: int = 0,
    requested_qty: int = 1,
    chronic_eligible: bool = False,
) -> dict:
    """Computes FEFO batch order and deterministic B2B stock-shield & barter policy flags (< 2000 chars)."""
    valid_batches = []
    near_expiry_batches = []
    total_available = 0

    for b in batches or []:
        if not isinstance(b, dict) or b.get("is_recalled"):
            continue
        expiry = str(b.get("expiry_date") or "")
        if expiry and expiry < "2026-11-15":
            continue
        on_hand = max(0, int(b.get("quantity_on_hand") or 0))
        reserved = max(0, int(b.get("quantity_reserved") or 0))
        avail = max(0, on_hand - reserved)
        total_available += avail
        entry = {
            "batch_id": b.get("id") or b.get("batch_id"),
            "batch_number": b.get("batch_number"),
            "expiry_date": expiry[:10] if expiry else "",
            "available_qty": avail,
        }
        valid_batches.append(entry)
        if avail > IMPORTER_RETURN_CAP or (expiry and expiry[:7] <= "2027-05"):
            near_expiry_batches.append(entry["batch_number"] or entry["batch_id"])

    valid_batches.sort(key=lambda x: x.get("expiry_date") or "9999-12-31")

    chronic_reserved = max(0, int(active_refills or 0)) if (chronic_eligible or active_refills > 0) else 0
    surplus_for_peers = max(0, total_available - chronic_reserved)

    flags: list[str] = []
    if total_available <= RARE_STOCK_THRESHOLD or surplus_for_peers < max(1, int(requested_qty or 1)):
        flags.append("RARE_PROTECT")
    if chronic_reserved > 0:
        flags.append("CHRONIC_BUFFER")
    if "RARE_PROTECT" in flags or (0 < surplus_for_peers <= RARE_STOCK_THRESHOLD):
        flags.append("EXCHANGE_ONLY")
    if near_expiry_batches:
        flags.append("NEAR_EXPIRY_PUSH")

    return {
        "total_available": total_available,
        "chronic_reserved": chronic_reserved,
        "surplus_for_peers": surplus_for_peers,
        "policy_flags": flags,
        "fefo_batches": valid_batches[:5],
    }


@tool
def search_medications(
    query: Annotated[str, "The medicine name as the requester wrote it, e.g. 'augmentin' or '1g tabs'"],
    form: Annotated[
        Literal["tablet", "capsule", "syrup", "inhaler", "injection", "cream", "drops"] | None,
        "Dosage form to keep, if the request states one",
    ] = None,
) -> dict:
    """Finds medicines in the catalogue by name. Use it first, before any pharmacy search, to turn the request into a catalogue medicine_id."""
    out = _http.call("GET", "/api/v1/medications", query={"q": query, "limit": 20})
    error = _http.tool_error(out)
    if error:
        return error
    items = list(out.get("data", []) or [])
    if not items:
        # The catalogue matches substrings, so a phrase like "augmentin 1g tabs" finds nothing.
        # Retry word by word (letters only) and merge, so the model never has to guess the right query.
        seen = set()
        for word in re.findall(r"[^\W\d_]{4,}", query)[:6]:
            more = _http.call("GET", "/api/v1/medications", query={"q": word, "limit": 20})
            for item in (more.get("data", []) or []) if not _http.tool_error(more) else []:
                if item.get("id") not in seen:
                    seen.add(item.get("id"))
                    items.append(item)
    wanted = _strengths_mg(query)

    matches = []
    for item in sorted(items, key=lambda i: 0 if _strengths_mg(i.get("strength", "")) & wanted else 1):
        if form and item.get("dosage_form") != form:
            continue
        matches.append({
            "medication_id": _cut_text(item.get("id")),
            "brand_name": _cut_text(item.get("brand_name")),
            "active_ingredient": _cut_text(item.get("active_ingredient")),
            "strength": _cut_text(item.get("strength")),
            "dosage_form": _cut_text(item.get("dosage_form")),
        })
        if len(matches) == MAX_MATCHES:
            break

    if not matches:
        return {"matches": [], "hint": "No catalogue match. Ask the requester with ask_requester; never invent a medicine."}
    result = {"matches": matches, "count": len(matches)}
    exact = [m for m in matches if _strengths_mg(m.get("strength", "")) & wanted]
    if wanted and len(exact) == 1:
        result["strength_match"] = exact[0]["medication_id"]
        result["hint"] = "Exactly one match has the requested strength: set_medication with strength_match."
    while len(result["matches"]) > 1 and len(json.dumps(result)) >= MAX_POLICY_CHARS:
        result["matches"].pop()
        result["count"] = len(result["matches"])
    return result


def _cut_text(value, limit: int = 120) -> str:
    text = str(value or "")
    return text if len(text) <= limit else text[: limit - 3] + "..."


def _strengths_mg(text: str) -> set:
    """Strengths in mg found in text: '1g' -> 1000, '625 mg' -> 625, '1000mg' -> 1000."""
    out = set()
    for num, unit in re.findall(r"(\d+(?:[.,]\d+)?)\s*(mg|g|mcg)\b", str(text or "").lower()):
        v = float(num.replace(",", "."))
        out.add(v * 1000 if unit == "g" else v / 1000 if unit == "mcg" else v)
    return out
