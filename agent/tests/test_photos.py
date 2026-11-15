import json

import pytest

import _http
import photos


class FakeCall:
    def __init__(self, *responses):
        self.responses = list(responses)
        self.calls = []

    def __call__(self, method, path, body=None, query=None):
        self.calls.append((method, path, body, query))
        return self.responses.pop(0) if self.responses else {}


@pytest.fixture
def setup(monkeypatch):
    """Mocks the photo download, the vision model and the backend call. `vision` is text or an exception."""
    def install(vision, backend=None, photo=(b"\x89PNG-bytes", None)):
        seen = {"fetched": [], "images": []}

        def fake_fetch(photo_id):
            seen["fetched"].append(photo_id)
            return photo

        def fake_vision(image):
            seen["images"].append(image)
            if isinstance(vision, Exception):
                raise vision
            return vision

        f = FakeCall(backend if backend is not None else {"id": "p1", "status": "needs_review", "decided_by": "rules"})
        monkeypatch.setattr(photos, "_fetch_photo", fake_fetch)
        monkeypatch.setattr(photos, "_vision", fake_vision)
        monkeypatch.setattr(_http, "call", f)
        seen["call"] = f
        return seen
    return install


def test_happy_path_posts_reading_and_returns_backend_decision(setup):
    seen = setup('{"code": "6280000000001", "expiry_month": "2027-08", "confidence": 0.93}',
                 {"id": "P 1", "status": "verified", "decided_by": "rules", "note": "code and expiry agree with the typed values",
                  "file_path": "/data/x.png", "typed_code": "6280000000001"})
    out = photos.read_package_photo.function(photo_id="P 1")
    assert seen["fetched"] == ["P 1"] and seen["images"] == [b"\x89PNG-bytes"]
    method, path, body, query = seen["call"].calls[0]
    assert (method, path, query) == ("POST", "/api/v1/photos/P%201/agent-reading", None)
    assert body == {"code": "6280000000001", "expiry_month": "2027-08", "confidence": 0.93}
    assert out == {"photo_id": "P 1", "read": body, "decision": "verified", "decided_by": "rules",
                   "note": "code and expiry agree with the typed values"}
    assert len(json.dumps(out)) < 2000


def test_fenced_json_with_chatter_is_parsed(setup):
    seen = setup('Sure!\n```json\n{"code": "628-0000 000001", "expiry_month": "2027-08", "confidence": 1.0}\n```\nDone.')
    out = photos.read_package_photo.function(photo_id="p1")
    assert seen["call"].calls[0][2] == {"code": "6280000000001", "expiry_month": "2027-08", "confidence": 1.0}
    assert out["decision"] == "needs_review" and "warning" not in out


@pytest.mark.parametrize("text", ["", "I cannot see any image.", "{not json}", "[1, 2]", '{"code": "62800'])
def test_garbage_output_posts_confidence_zero(setup, text):
    seen = setup(text)
    out = photos.read_package_photo.function(photo_id="p1")
    assert seen["call"].calls[0][2] == {"code": "", "expiry_month": "", "confidence": 0.0}
    assert "human reviewer" in out["warning"] and "not readable" in out["warning"]
    assert out["decision"] == "needs_review"


def test_bad_fields_are_cleaned_not_trusted(setup):
    seen = setup('{"code": "N/A", "expiry_month": "08/2027", "confidence": 7}')
    photos.read_package_photo.function(photo_id="p1")
    assert seen["call"].calls[0][2] == {"code": "", "expiry_month": "", "confidence": 0.0}  # nothing read -> 0

    seen = setup('{"code": 6280000000001, "expiry_month": "2027-13", "confidence": "high"}')
    photos.read_package_photo.function(photo_id="p1")
    assert seen["call"].calls[0][2] == {"code": "6280000000001", "expiry_month": "", "confidence": 0.0}

    seen = setup('{"code": "123", "expiry_month": "2027-01", "confidence": 1.7}')
    photos.read_package_photo.function(photo_id="p1")
    assert seen["call"].calls[0][2]["confidence"] == 1.0


@pytest.mark.parametrize("exc", [TimeoutError("timed out"), OSError("connection refused"), ValueError("bad body")])
def test_model_failure_posts_confidence_zero(setup, exc):
    seen = setup(exc)
    out = photos.read_package_photo.function(photo_id="p1")
    assert seen["call"].calls[0][2] == {"code": "", "expiry_month": "", "confidence": 0.0}
    assert "could not be reached" in out["warning"] and "human reviewer" in out["warning"]


def test_backend_conflict_is_passed_back_as_teaching_error(setup):
    setup('{"code": "1", "expiry_month": "2027-08", "confidence": 0.9}',
          {"error": {"code": "conflict", "message": "this photo was already reviewed by a human; do not read it again"}})
    out = photos.read_package_photo.function(photo_id="p1")
    assert out == {"error": "this photo was already reviewed by a human; do not read it again", "code": "conflict"}


def test_never_sends_anything_but_code_expiry_confidence(setup):
    seen = setup('{"code": "6280000000001", "expiry_month": "2027-08", "confidence": 0.9, "status": "verified", '
                 '"genuine": true, "decision": "verified", "decided_by": "agent", "note": "looks real"}')
    out = photos.read_package_photo.function(photo_id="p1")
    assert len(seen["call"].calls) == 1
    assert set(seen["call"].calls[0][2]) == {"code", "expiry_month", "confidence"}
    assert out["decision"] == "needs_review" and out["decided_by"] == "rules"  # the backend's words, not the model's


def test_photo_download_error_stops_before_model_and_backend(setup):
    seen = setup("{}", photo=(b"", {"error": "photo not found", "code": "not_found"}))
    out = photos.read_package_photo.function(photo_id="nope")
    assert out == {"error": "photo not found", "code": "not_found"}
    assert seen["images"] == [] and seen["call"].calls == []


def test_empty_photo_id_is_a_validation_error(setup):
    seen = setup("{}")
    out = photos.read_package_photo.function(photo_id="  ")
    assert out["code"] == "validation_failed" and seen["fetched"] == []


def test_result_stays_small_and_reads_nested_photo(setup):
    setup('{"code": "1", "expiry_month": "2027-08", "confidence": 0.9}',
          {"photo": {"status": "needs_review", "decided_by": "rules", "reasons": ["x" * 5000, "y"]}})
    out = photos.read_package_photo.function(photo_id="p1")
    assert out["decision"] == "needs_review" and len(json.dumps(out)) < 2000


def test_vision_url_and_model_defaults(monkeypatch):
    for name in ("WASSIT_VISION_URL", "WASSIT_VISION_MODEL", "CORNILIA_BASE_URL"):
        monkeypatch.delenv(name, raising=False)
    assert photos._vision_url() == "http://10.31.216.24:11434" and photos._vision_model() == "gemma4:31b"
    monkeypatch.setenv("CORNILIA_BASE_URL", "http://localhost:11434/v1/")
    assert photos._vision_url() == "http://localhost:11434"
    monkeypatch.setenv("WASSIT_VISION_URL", "http://vision:1/")
    monkeypatch.setenv("WASSIT_VISION_MODEL", "other:1b")
    assert photos._vision_url() == "http://vision:1" and photos._vision_model() == "other:1b"


def test_schema_has_one_required_parameter():
    params = photos.read_package_photo.get_schema()["function"]["parameters"]
    assert params["required"] == ["photo_id"] and list(params["properties"]) == ["photo_id"]
