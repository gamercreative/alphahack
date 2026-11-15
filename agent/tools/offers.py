import os
import re
import sys
from typing import Annotated, Literal
from urllib.parse import quote

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))  # lets this file import _http when Cornilia loads it

import _http  # noqa: E402
import enquiry as _enquiry  # noqa: E402  (module import only: its tools are not re-exported here)
from cornilia.tools import tool  # noqa: E402

EVIDENCE_KEYS = ("in_stock", "strength", "quantity", "price", "pickup_by", "payment", "needs_prescription")
# Names small models use for evidence keys -> the backend's key. Keys for things that are not fields are dropped.
EVIDENCE_ALIASES = {
    "price_cents": "price", "unit_price": "price", "price_per_box": "price", "cost": "price",
    "qty": "quantity", "boxes": "quantity", "count": "quantity",
    "stock": "in_stock", "available": "in_stock", "availability": "in_stock",
    "prescription": "needs_prescription", "rx": "needs_prescription",
    "pickup": "pickup_by", "pickup_time": "pickup_by",
}
NOT_FIELDS = {"currency", "expiry", "exp", "expiry_date", "expiration", "brand", "medicine", "form", "dosage_form"}
_ARABIC_DIGITS = str.maketrans("٠١٢٣٤٥٦٧٨٩۰۱۲۳۴۵۶۷۸۹", "01234567890123456789")
_NUMBER = re.compile(r"\d+(?:[.,]\d+)?")


def _numbers(text: str) -> set[float]:
    out = set()
    for tok in _NUMBER.findall(str(text or "").translate(_ARABIC_DIGITS)):
        out.add(float(tok.replace(",", ".")))
    return out


def _currency_from(quote: str) -> str:
    """USD or LBP when the price quote writes it ($, dollar, دولار / LBP, lira, ليرة, ل.ل), else ""."""
    q = str(quote or "").lower()
    if any(m in q for m in ("$", "usd", "dollar", "dolar", "دولار")):
        return "USD"
    if any(m in q for m in ("lbp", "l.l", "lira", "leira", "ليرة", "ل.ل")):
        return "LBP"
    return ""


def _field_values(in_stock, strength, quantity, price_cents, pickup_by, payment, needs_prescription) -> dict:
    """The fields the model set, keyed like the evidence."""
    values = {"in_stock": in_stock, "strength": strength or None, "quantity": quantity, "price": price_cents,
              "pickup_by": pickup_by or None, "payment": payment or None, "needs_prescription": needs_prescription}
    return {k: v for k, v in values.items() if v is not None}


def _check_reading(fields: dict, evidence: dict) -> str | None:
    """Teaching message when the reading cannot be right, before anything is stored."""
    if not fields:
        if evidence:
            keys = ", ".join(sorted(evidence))
            return (f"You quoted {keys} but set no field. Set the field values too, next to the quotes, e.g. "
                    "in_stock=true, quantity=2, evidence={\"in_stock\": \"3ande\", \"quantity\": \"2 3elab\"}.")
        return "Set at least one field read from the reply (in_stock, quantity, strength, price_cents, ...) with its quote in evidence."
    missing = [k for k in fields if k not in evidence]
    if missing:
        return (f"{', '.join(missing)} set without a quote. Put the exact words of the reply that say it in evidence "
                f"(evidence keys: {', '.join(EVIDENCE_KEYS)}), or leave the field out if the reply does not say it. "
                "Never copy a strength or quantity from the request.")
    if "quantity" in fields and float(fields["quantity"]) not in _numbers(evidence["quantity"]):
        return (f"quantity={fields['quantity']} but the quote {evidence['quantity']!r} does not write that number. Quote the "
                "words with the number, or leave quantity out when the reply gives no number.")
    if "price" in fields:
        cents = float(fields["price"])
        if not ({cents, cents / 100} & _numbers(evidence["price"])):
            return (f"price_cents={fields['price']} but the quote {evidence['price']!r} does not write that price. Use the "
                    "price exactly as written (12$ -> price_cents=1200, currency USD); do not multiply it.")
    return None


@tool
def record_offer(
    target_id: Annotated[str, "The target_id of a pharmacy that replied with text"],
    evidence: Annotated[dict[str, str], "One exact quote from the reply per field you set. Keys: in_stock, strength, quantity, price, pickup_by, payment, needs_prescription"],
    confidence: Annotated[float, "How sure you are of the whole reading, 0 to 1"],
    in_stock: Annotated[bool | None, "True if the reply says they have it, False if it says they do not"] = None,
    strength: Annotated[str | None, "Only if the reply names a strength, with its unit, e.g. 500mg; never the requested one"] = None,
    quantity: Annotated[int | None, "Only if the reply writes a number of boxes/units"] = None,
    price_cents: Annotated[int | None, "Price as written in the reply, in cents: 12$ -> 1200 (USD), 9 dollars -> 900"] = None,
    currency: Annotated[str | None, "LBP or USD, required with price_cents (no evidence needed)"] = None,
    pickup_by: Annotated[str | None, "Pickup window in plain words, e.g. before 18:00 (an expiry date is NOT a pickup time)"] = None,
    payment: Annotated[Literal["cash", "card", "any"] | None, "Payment the pharmacy asks for"] = None,
    needs_prescription: Annotated[bool | None, "True if the pharmacy asks for a prescription"] = None,
) -> dict:
    """Reads one pharmacist reply into a structured offer: set each field the reply states AND put the exact words that state it in evidence.
Example: reply "eh 3ande 2 3elab, 12$ el 3elbe, exp 08/2027" -> in_stock=true, quantity=2, price_cents=1200, currency="USD",
evidence={"in_stock": "eh 3ande", "quantity": "2 3elab", "price": "12$"}. Reply "la2 ma fi" -> in_stock=false, evidence={"in_stock": "la2 ma fi"}.
Copy quotes exactly (no translation). Leave out what the reply does not say; an expiry date is not a field. The backend rules decide matches, escalated or rejected, not you."""
    if evidence is not None and not isinstance(evidence, dict):
        return {"error": "evidence must be an object like {\"in_stock\": \"3ande\"}: one exact quote per field you set", "code": "validation_failed"}
    quotes, ignored = {}, []
    for k, v in (evidence or {}).items():
        key = EVIDENCE_ALIASES.get(str(k).strip().lower(), str(k).strip().lower())
        if v is None or not str(v).strip():
            continue  # empty quotes are dropped
        if key in NOT_FIELDS or key not in EVIDENCE_KEYS:
            ignored.append(str(k))
            continue
        quotes.setdefault(key, str(v))  # quotes are text: a number the model passes (2) is sent as "2"
    if price_cents is not None and not (currency or "").strip():
        currency = _currency_from(quotes.get("price", ""))
        if not currency:
            return {"error": "Not recorded: currency is required with price_cents: USD or LBP, as the reply writes it.",
                    "code": "validation_failed"}
    fields = _field_values(in_stock, strength, quantity, price_cents, pickup_by, payment, needs_prescription)
    problem = _check_reading(fields, quotes)
    if problem:
        return {"error": "Not recorded: " + problem, "code": "validation_failed"}
    body = {
        "evidence": quotes,
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
    result = {
        "offer_id": out.get("id"),
        "target_id": target_id,
        "decision": out.get("decision"),
        "reasons": [str(r)[:200] for r in (out.get("reasons") or [])][:6],
        "note": "The rules decided. Report escalated offers to the requester with notify_requester; never count them as a match.",
    }
    if ignored:
        result["ignored_evidence"] = ignored[:6]
    return _enquiry._with_next(result, out.get("enquiry_id"))
