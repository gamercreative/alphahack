import re
import os
import sys
from typing import Annotated, Literal

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))  # lets this file import _http when Cornilia loads it

import _http  # noqa: E402
from cornilia.tools import tool  # noqa: E402

MAX_MATCHES = 5


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
            "medication_id": item.get("id"),
            "brand_name": item.get("brand_name"),
            "active_ingredient": item.get("active_ingredient"),
            "strength": item.get("strength"),
            "dosage_form": item.get("dosage_form"),
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
    return result


def _strengths_mg(text: str) -> set:
    """Strengths in mg found in text: '1g' -> 1000, '625 mg' -> 625, '1000mg' -> 1000."""
    out = set()
    for num, unit in re.findall(r"(\d+(?:[.,]\d+)?)\s*(mg|g|mcg)\b", str(text or "").lower()):
        v = float(num.replace(",", "."))
        out.add(v * 1000 if unit == "g" else v / 1000 if unit == "mcg" else v)
    return out
