import os
import sys
from typing import Annotated, Literal
from urllib.parse import quote

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))  # lets this file import _http when Cornilia loads it

import _http  # noqa: E402
from cornilia.tools import tool  # noqa: E402


@tool
def record_offer(
    target_id: Annotated[str, "The target_id of a pharmacy that replied with text"],
    evidence: Annotated[dict[str, str], "One entry per field you set, keyed in_stock, strength, quantity, price, pickup_by, payment or needs_prescription; the value is the exact words copied from the pharmacist's reply"],
    confidence: Annotated[float, "How sure you are of the whole reading, 0 to 1"],
    in_stock: Annotated[bool | None, "True if the reply says they have it, False if it says they do not"] = None,
    strength: Annotated[str | None, "Strength offered with its unit, e.g. 500mg, 1g, 500mcg"] = None,
    quantity: Annotated[int | None, "How many units the pharmacy offers"] = None,
    price_cents: Annotated[int | None, "Total price in cents (or LBP units) if the reply names one"] = None,
    currency: Annotated[str | None, "LBP or USD, required with price_cents"] = None,
    pickup_by: Annotated[str | None, "Pickup window in plain words, e.g. before 18:00"] = None,
    payment: Annotated[Literal["cash", "card", "any"] | None, "Payment the pharmacy asks for"] = None,
    needs_prescription: Annotated[bool | None, "True if the pharmacy asks for a prescription"] = None,
) -> dict:
    """Reads one pharmacist reply into a structured offer. Copy every quote exactly from the reply; do not translate or fix spelling. Leave a field out when the reply does not say it. The backend rules decide matches, escalated or rejected, not you: an escalated offer is never a match."""
    if evidence is not None and not isinstance(evidence, dict):
        return {"error": "evidence must be an object like {\"in_stock\": \"3ande\"}: one exact quote per field you set", "code": "validation_failed"}
    body = {
        # Quotes are text: a number the model passes (2) is sent as "2"; empty quotes are dropped.
        "evidence": {str(k): str(v) for k, v in (evidence or {}).items() if v is not None and str(v).strip()},
        "confidence": confidence,
        "in_stock": in_stock,
        "strength": strength or "",
        "quantity": quantity,
        "price_cents": price_cents,
        "currency": currency or "",
        "pickup_by": pickup_by or "",
        "payment": payment or "",
        "needs_prescription": needs_prescription,
    }
    out = _http.call("POST", f"/api/v1/targets/{quote(str(target_id), safe='')}/offers", body=body)
    error = _http.tool_error(out)
    if error:
        return error
    return {
        "offer_id": out.get("id"),
        "target_id": target_id,
        "decision": out.get("decision"),
        "reasons": [str(r)[:200] for r in (out.get("reasons") or [])][:6],
        "note": "The rules decided. Report escalated offers to the requester with notify_requester; never count them as a match.",
    }
