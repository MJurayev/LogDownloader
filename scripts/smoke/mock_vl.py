#!/usr/bin/env python3
"""Minimal VictoriaLogs mock (smoke test uchun).

    mock_vl.py [port] [lines] [delay_per_line_sec]

Endpointlar: /select/logsql/{query,hits,field_names,field_values,streams,tail}.
Har `query` so'rovi LINES ta NDJSON qator qaytaradi; `_time` 2026-10-21 10:00:00Z
dan boshlab har qator +1 s (3000 qator → 10:00–10:49). `start`/`end` (RFC3339)
va `limit` parametrlari hurmat qilinadi — vaqt oynalari testi uchun.
Maxsus belgilar query ichida:
  FAIL_ONCE — har bir alohida `start` qiymati uchun birinchi so'rov 503 (retry testi)
  SLOW      — har qatorda qo'shimcha 2 ms (cancel/restart testlari uchun)
  RANDOM    — _msg ga 96 hex belgi qo'shiladi (siqilmaydi; arxiv volume testi)
/__last_headers — oxirgi so'rovning header'lari (tenant header testi)."""
import json, os, sys, time, threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.parse import urlparse, parse_qs

PORT = int(sys.argv[1]) if len(sys.argv) > 1 else 19428
LINES = int(sys.argv[2]) if len(sys.argv) > 2 else 3000
DELAY = float(sys.argv[3]) if len(sys.argv) > 3 else 0.001

state = {"last_headers": {}, "failed_starts": set()}
lock = threading.Lock()


BASE_TS = 1792576800  # 2026-10-21T10:00:00Z


def parse_ts(s):
    if not s:
        return None
    from datetime import datetime, timezone
    s = s.rstrip("Z")
    if "." in s:
        s = s.split(".")[0]
    return datetime.strptime(s, "%Y-%m-%dT%H:%M:%S").replace(tzinfo=timezone.utc).timestamp()


def line(i, rnd=False):
    msg = "line %d" % i
    if rnd:  # siqilmaydigan payload (arxiv volume testi uchun)
        msg += " " + os.urandom(48).hex()
    return {"_time": "2026-10-21T10:%02d:%02d.000Z" % ((i // 60) % 60, i % 60), "_msg": msg,
            "level": "info" if i % 7 else "error", "service": "mock", "_stream": '{service="mock"}',
            "n": i}


class H(BaseHTTPRequestHandler):
    def log_message(self, *a):
        pass

    def send_json(self, obj, code=200):
        body = json.dumps(obj).encode()
        self.send_response(code)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_GET(self):
        u = urlparse(self.path)
        q = parse_qs(u.query)
        query = q.get("query", [""])[0]
        if u.path == "/__last_headers":
            return self.send_json(state["last_headers"])
        with lock:
            state["last_headers"] = {k: v for k, v in self.headers.items()}
        if u.path == "/select/logsql/hits":
            return self.send_json({"hits": [{"fields": {}, "timestamps": ["2026-10-21T10:00:00Z"], "values": [LINES], "total": LINES}]})
        if u.path == "/select/logsql/field_names":
            return self.send_json({"values": [{"value": "_time", "hits": LINES}, {"value": "_msg", "hits": LINES},
                                              {"value": "level", "hits": LINES}, {"value": "service", "hits": LINES}]})
        if u.path == "/select/logsql/field_values":
            f = q.get("field", [""])[0]
            vals = {"level": ["info", "error"], "service": ["mock"]}.get(f, [])
            return self.send_json({"values": [{"value": v, "hits": 1} for v in vals]})
        if u.path == "/select/logsql/streams":
            return self.send_json({"values": [{"value": '{service="mock"}', "hits": LINES}]})
        if u.path == "/select/logsql/tail":
            self.send_response(200)
            self.send_header("Content-Type", "application/stream+json")
            self.end_headers()
            try:
                for i in range(5):
                    self.wfile.write((json.dumps(line(i)) + "\n").encode())
                    self.wfile.flush()
                    time.sleep(0.05)
                time.sleep(30)  # ochiq qoladi, client yopadi
            except (BrokenPipeError, ConnectionResetError):
                pass
            return
        if u.path != "/select/logsql/query":
            self.send_response(404)
            self.end_headers()
            return

        if "FAIL_ONCE" in query:
            start = q.get("start", [""])[0]
            with lock:
                first = start not in state["failed_starts"]
                if first:
                    state["failed_starts"].add(start)
            if first:
                body = b"simulated upstream failure"
                self.send_response(503)
                self.send_header("Content-Length", str(len(body)))
                self.end_headers()
                self.wfile.write(body)
                return
        delay = DELAY + (0.002 if "SLOW" in query else 0)
        rnd = "RANDOM" in query
        limit = int(q.get("limit", [LINES])[0])
        t_start = parse_ts(q.get("start", [""])[0])
        t_end = parse_ts(q.get("end", [""])[0])
        self.send_response(200)
        self.send_header("Content-Type", "application/stream+json")
        self.end_headers()
        try:
            sent = 0
            for i in range(LINES):
                ts = BASE_TS + i
                if (t_start is not None and ts < t_start) or (t_end is not None and ts > t_end):
                    continue
                if sent >= limit:
                    break
                self.wfile.write((json.dumps(line(i, rnd)) + "\n").encode())
                sent += 1
                if sent % 50 == 0:
                    self.wfile.flush()
                if delay:
                    time.sleep(delay)
            self.wfile.flush()
        except (BrokenPipeError, ConnectionResetError):
            pass


ThreadingHTTPServer(("127.0.0.1", PORT), H).serve_forever()
