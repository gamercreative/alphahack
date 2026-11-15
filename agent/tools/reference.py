import os
import sys
from typing import Annotated
from urllib.parse import urlencode

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))  # lets this file import _http when Cornilia loads it

import _http  # noqa: E402
from cornilia.tools import tool  # noqa: E402

MAX_ROWS = 5
NOTE = "Information only; the Go rules decide equivalence."


@tool
def check_reference(
    ingredient: Annotated[str, "Active ingredient as written in the catalogue, e.g. Amoxicillin/Clavulanate"],
    strength: Annotated[str | None, "Strength with its unit, e.g. 625mg; leave out to see every strength"] = None,
    form: Annotated[str | None, "Dosage form, e.g. tablet, syrup, inhaler; leave out for every form"] = None,
) -> dict:
    """Looks up the government reference list (registered products and official prices) for one ingredient. Read only: it never decides whether an offer is equivalent and never changes the enquiry."""
    ingredient = str(ingredient or "").strip()
    if not ingredient:
        return {"error": "ingredient is required, e.g. Amoxicillin/Clavulanate; take it from search_medications", "code": "validation_failed"}
    params = {"ingredient": ingredient, "limit": MAX_ROWS}
    if strength and str(strength).strip():
        params["strength"] = str(strength).strip()
    if form and str(form).strip():
        params["form"] = str(form).strip()
    out = _http.call("GET", "/api/v1/reference/products?" + urlencode(params))
    err = _http.tool_error(out)
    if err:
        return err
    rows = [r for r in (out.get("data") or []) if isinstance(r, dict)][:MAX_ROWS]
    products = [
        {
            "code": r.get("code"),
            "brand_name": r.get("brand_name"),
            "strength": r.get("strength"),
            "dosage_form": r.get("dosage_form"),
            "official_price_cents": r.get("official_price_cents"),
        }
        for r in rows
    ]
    result = {"products": products, "count": len(products), "note": NOTE}
    if not products:
        result["hint"] = "No registered product matches; try without strength or form, or check the ingredient spelling."
    return result
