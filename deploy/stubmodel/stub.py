"""A model that answers Chat Completions the same way every time: one short message and a fixed usage.

The install check points a credential at it, so that what the proxy counts can be compared with numbers that are known.
"""
import json
from http.server import BaseHTTPRequestHandler, HTTPServer

PROMPT_TOKENS = 1000
COMPLETION_TOKENS = 100


class Stub(BaseHTTPRequestHandler):
    def do_POST(self):
        self.rfile.read(int(self.headers.get("content-length", 0)))
        if not self.path.endswith("/chat/completions"):
            self.send_error(404, "this model answers Chat Completions only")
            return
        body = json.dumps({
            "id": "stub-1",
            "object": "chat.completion",
            "model": "stub-model",
            "choices": [{"index": 0, "message": {"role": "assistant", "content": "stub says hi"}, "finish_reason": "stop"}],
            "usage": {"prompt_tokens": PROMPT_TOKENS, "completion_tokens": COMPLETION_TOKENS},
        }).encode()
        self.send_response(200)
        self.send_header("content-type", "application/json")
        self.send_header("content-length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_GET(self):
        self.send_response(200)
        self.end_headers()
        self.wfile.write(b"ok")


if __name__ == "__main__":
    HTTPServer(("0.0.0.0", 9000), Stub).serve_forever()
