#!/usr/bin/env python3
"""Minimal VictoriaLogs mock: GET /select/logsql/query -> NDJSON lines.
LINES ta qator, har biri 1ms kechikish bilan (export ~3s davom etadi,
cancel/restart testlari uchun oyna beradi)."""
import json, sys, time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

PORT = int(sys.argv[1]) if len(sys.argv) > 1 else 19428
LINES = int(sys.argv[2]) if len(sys.argv) > 2 else 3000


class H(BaseHTTPRequestHandler):
    def log_message(self, *a):
        pass

    def do_GET(self):
        if not self.path.startswith("/select/logsql/query"):
            self.send_response(404)
            self.end_headers()
            return
        self.send_response(200)
        self.send_header("Content-Type", "application/stream+json")
        self.end_headers()
        try:
            for i in range(LINES):
                line = {"_time": "2026-10-21T10:00:%02d.000Z" % (i % 60), "_msg": "line %d" % i,
                        "level": "info" if i % 7 else "error", "service": "mock"}
                self.wfile.write((json.dumps(line) + "\n").encode())
                if i % 50 == 0:
                    self.wfile.flush()
                time.sleep(0.001)
            self.wfile.flush()
        except (BrokenPipeError, ConnectionResetError):
            pass


ThreadingHTTPServer(("127.0.0.1", PORT), H).serve_forever()
