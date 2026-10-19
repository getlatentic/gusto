#!/usr/bin/env python3
"""A customer system as an MCP server over Streamable HTTP, for the integration end-to-end run.

It has one tool, find_customer, and answers only a call that carries the key gusto's gateway holds, so a harness that reached it
directly, without the gateway, would be refused. Every request is written to its log, with whether the key was right.
"""
import json
import sys
from http.server import BaseHTTPRequestHandler, HTTPServer

KEY = "e2e-customer-system-key"
LOG = sys.argv[1] if len(sys.argv) > 1 else "customers.log"
CUSTOMERS = {"acme": "CUST-7391-ZETA", "globex": "CUST-2208-OMEGA"}


class Handler(BaseHTTPRequestHandler):
    def log_message(self, *args):
        pass

    def reply(self, status, body=None, headers=None):
        data = b"" if body is None else json.dumps(body).encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        for name, value in (headers or {}).items():
            self.send_header(name, value)
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)

    def do_POST(self):
        message = json.loads(self.rfile.read(int(self.headers.get("Content-Length", 0))) or b"{}")
        keyed = self.headers.get("X-Api-Key") == KEY
        with open(LOG, "a") as log:
            log.write(json.dumps({"method": message.get("method"), "keyed": keyed, "from": self.client_address[0]}) + "\n")
        if not keyed:
            return self.reply(401, {"error": "no key"})
        if "id" not in message:
            return self.reply(202)
        method, result = message.get("method"), {}
        headers = {}
        if method == "initialize":
            result = {"protocolVersion": "2025-06-18", "capabilities": {"tools": {}}, "serverInfo": {"name": "customers", "version": "1"}}
            headers["Mcp-Session-Id"] = "e2e-session"
        elif method == "tools/list":
            result = {"tools": [{
                "name": "find_customer",
                "description": "Finds a customer of the company by name and returns their account id. Use it whenever the person asks for a customer's account id.",
                "inputSchema": {"type": "object", "properties": {"name": {"type": "string", "description": "The customer's name"}}, "required": ["name"]},
            }]}
        elif method == "tools/call":
            asked = (message.get("params", {}).get("arguments") or {}).get("name", "").strip().lower()
            found = CUSTOMERS.get(asked)
            text = "account id " + found if found else "no customer called " + asked
            result = {"content": [{"type": "text", "text": text}], "isError": found is None}
        return self.reply(200, {"jsonrpc": "2.0", "id": message["id"], "result": result}, headers)

    def do_DELETE(self):
        self.reply(200)

    def do_GET(self):
        self.reply(405)


HTTPServer(("127.0.0.1", 9100), Handler).serve_forever()
