"""A model that answers Chat Completions with fixed replies, one per call, for retaking the landing page's screens.

The replies are the words a model gave in a real demo run, so the screens show real answers without spending anything.
A reply is chosen by the question it answers, and in order when none matches.
"""
import json
import sys
import time
from http.server import BaseHTTPRequestHandler, HTTPServer

REPLIES = json.load(open(sys.argv[1]))
calls = {"n": 0}


class Replay(BaseHTTPRequestHandler):
    def do_POST(self):
        asked = self.rfile.read(int(self.headers.get("content-length", 0))).decode("utf-8", "replace")
        # a reply is found by the words it answers, so a call the app makes on its own does not shift the rest
        # (a conversation's earlier words come with the latest, so the question that comes last is the one asked)
        found = [(asked.rfind(r["say"][:40]), r) for r in REPLIES if r.get("say") and r["say"][:40] in asked]
        reply = max(found, key=lambda each: each[0])[1] if found else REPLIES[min(calls["n"], len(REPLIES) - 1)]
        time.sleep(reply.get("wait", 0))
        calls["n"] += 1
        body = json.dumps({
            "id": "replay", "object": "chat.completion", "model": "gemma-demo",
            "choices": [{"index": 0, "message": {"role": "assistant", "content": reply["text"]}, "finish_reason": "stop"}],
            "usage": {"prompt_tokens": reply["in"], "completion_tokens": reply["out"]},
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
    HTTPServer(("127.0.0.1", 9100), Replay).serve_forever()
