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

    matches = []
    for item in out.get("data", []):
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
    return {"matches": matches, "count": len(matches)}
