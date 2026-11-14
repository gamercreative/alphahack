import pytest

from cornilia_guard import GatewayClient, GatewayError


class _FakeResponse:
    def __init__(self, body: bytes):
        self._body = body

    def __enter__(self):
        return self

    def __exit__(self, *exc):
        return False

    def read(self):
        return self._body


class _FakeOpener:
    def __init__(self, body: bytes):
        self._body = body
        self.requests = []

    def open(self, req, timeout=None):
        self.requests.append((req, timeout))
        return _FakeResponse(self._body)


def _client_with_body(body: bytes) -> GatewayClient:
    client = GatewayClient("http://gw.test", b"k" * 32, "tok")
    client._opener = _FakeOpener(body)
    return client


def test_non_json_2xx_body_raises_gateway_error():
    client = _client_with_body(b"<html>oops</html>")
    with pytest.raises(GatewayError) as excinfo:
        client.call("catalogue.search", {"q": "x"})
    e = excinfo.value
    assert e.status == 200
    assert e.code == "bad_response"


def test_json_list_2xx_body_raises_gateway_error():
    client = _client_with_body(b"[1,2]")
    with pytest.raises(GatewayError) as excinfo:
        client.call("catalogue.search", {"q": "x"})
    e = excinfo.value
    assert e.status == 200
    assert e.code == "bad_response"


def test_ok_false_still_bad_response():
    client = _client_with_body(b'{"ok":false}')
    with pytest.raises(GatewayError) as excinfo:
        client.call("catalogue.search", {"q": "x"})
    e = excinfo.value
    assert e.status == 200
    assert e.code == "bad_response"


def test_ok_true_returns_data():
    client = _client_with_body(b'{"ok":true,"data":{"x":1}}')
    assert client.call("catalogue.search", {"q": "x"}) == {"x": 1}
