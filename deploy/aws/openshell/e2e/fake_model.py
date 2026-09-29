"""A model that answers every call the same way, so the run does not need one."""
import json
from http.server import BaseHTTPRequestHandler, HTTPServer


class Model(BaseHTTPRequestHandler):
    def do_GET(self):
        body = json.dumps({"object": "list", "data": [{"id": "deepseek-flash", "object": "model"}]}).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_POST(self):
        asked = json.loads(self.rfile.read(int(self.headers.get("Content-Length", 0))) or b"{}")
        if asked.get("stream"):
            self.send_response(200)
            self.send_header("Content-Type", "text/event-stream")
            self.end_headers()
            chunk = lambda delta, finish=None, extra=None: "data: " + json.dumps(dict({
                "id": "fake", "object": "chat.completion.chunk", "model": "fake",
                "choices": [{"index": 0, "delta": delta, "finish_reason": finish}]}, **(extra or {}))) + "\n\n"
            self.wfile.write(chunk({"role": "assistant", "content": "pong"}).encode())
            self.wfile.write(chunk({}, "stop", {"usage": {"prompt_tokens": 3, "completion_tokens": 2, "total_tokens": 5}}).encode())
            self.wfile.write(b"data: [DONE]\n\n")
            return
        body = json.dumps({
            "id": "fake", "object": "chat.completion", "model": "fake",
            "choices": [{"index": 0, "message": {"role": "assistant", "content": "pong"}, "finish_reason": "stop"}],
            "usage": {"prompt_tokens": 3, "completion_tokens": 2, "total_tokens": 5},
        }).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def log_message(self, *args):
        pass


HTTPServer(("127.0.0.1", 9000), Model).serve_forever()
