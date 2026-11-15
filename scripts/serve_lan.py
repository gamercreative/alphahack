#!/usr/bin/env python3.13
"""Serve the DOSYN web app and the Wassit API from ONE address on the local network.

    python3.13 scripts/serve_lan.py                 # http://<this-mac>:5600, API -> http://localhost:8113
    LAN_PORT=5600 BACKEND=http://localhost:8113 python3.13 scripts/serve_lan.py

Static files come from web/. /api/*, /v1/* and /healthz are forwarded to BACKEND, including
server-sent events (streamed) and multipart uploads. Other devices on the same Wi-Fi open
http://<this-mac-ip>:5600. LAN only: the backend has no auth, never expose this to the internet.
"""
from __future__ import annotations

import http.client
import os
import socket
import urllib.parse
from http.server import SimpleHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
WEB = ROOT / "web"
PORT = int(os.environ.get("LAN_PORT", "5600"))
BACKEND = urllib.parse.urlparse(os.environ.get("BACKEND", "http://localhost:8113"))
HOP = {"connection", "keep-alive", "proxy-authenticate", "proxy-authorization", "te", "trailers",
       "transfer-encoding", "upgrade", "content-length", "host"}


class Handler(SimpleHTTPRequestHandler):
    def __init__(self, *args, **kwargs):
        super().__init__(*args, directory=str(WEB), **kwargs)

    def _is_api(self) -> bool:
        return self.path.startswith(("/api/", "/v1/", "/healthz"))

    def _proxy(self):
        length = int(self.headers.get("Content-Length") or 0)
        body = self.rfile.read(length) if length else None
        headers = {k: v for k, v in self.headers.items() if k.lower() not in HOP}
        conn = http.client.HTTPConnection(BACKEND.hostname, BACKEND.port or 80, timeout=600)
        try:
            conn.request(self.command, self.path, body=body, headers=headers)
            resp = conn.getresponse()
        except OSError as err:
            self.send_error(502, f"backend unreachable: {err}")
            return
        self.send_response(resp.status)
        streaming = "text/event-stream" in (resp.getheader("Content-Type") or "")
        for k, v in resp.getheaders():
            if k.lower() not in HOP:
                self.send_header(k, v)
        if streaming:
            self.send_header("Cache-Control", "no-cache")
            self.end_headers()
            try:
                while True:
                    chunk = resp.read1(4096)
                    if not chunk:
                        break
                    self.wfile.write(chunk)
                    self.wfile.flush()
            except (BrokenPipeError, ConnectionResetError, socket.timeout):
                pass
        else:
            data = resp.read()
            self.send_header("Content-Length", str(len(data)))
            self.end_headers()
            self.wfile.write(data)
        conn.close()

    def do_GET(self):
        if self._is_api():
            return self._proxy()
        return super().do_GET()

    def do_HEAD(self):
        if self._is_api():
            return self._proxy()
        return super().do_HEAD()

    def do_POST(self):
        return self._proxy() if self._is_api() else self.send_error(405)

    def do_PUT(self):
        return self._proxy() if self._is_api() else self.send_error(405)

    def do_PATCH(self):
        return self._proxy() if self._is_api() else self.send_error(405)

    def do_DELETE(self):
        return self._proxy() if self._is_api() else self.send_error(405)

    def do_OPTIONS(self):
        return self._proxy() if self._is_api() else self.send_error(405)

    def end_headers(self):
        if not self._is_api():
            self.send_header("Cache-Control", "no-store")
        super().end_headers()

    def log_message(self, fmt, *args):
        pass


def lan_ip() -> str:
    s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
    try:
        s.connect(("10.255.255.255", 1))
        return s.getsockname()[0]
    except OSError:
        return "127.0.0.1"
    finally:
        s.close()


if __name__ == "__main__":
    print(f"DOSYN on the local network: http://{lan_ip()}:{PORT}   (this Mac: http://localhost:{PORT})")
    print(f"API forwarded to {BACKEND.geturl()}. Ctrl-C to stop.")
    ThreadingHTTPServer(("0.0.0.0", PORT), Handler).serve_forever()
