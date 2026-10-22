#!/usr/bin/env python3
"""LogDownloader smoke test — backend xatti-harakatlarini end-to-end tekshiradi.

Ishga tushirish (repo ildizidan):
    make build
    python3 -I scripts/smoke/smoke.py

Env:
    LOGDOWNLOADER_BIN  — tekshiriladigan binary (default: build/logdownloader-linux-amd64)
    SMOKE_DATA_DIR     — vaqtinchalik DATA_DIR (default: tempfile.mkdtemp())
    SMOKE_PORT         — server porti (default 13000), SMOKE_VL_PORT — mock VictoriaLogs porti (19428)
"""
import csv, gzip, io, json, os, shutil, signal, subprocess, sys, tarfile, tempfile, time, urllib.request, urllib.error

# http_proxy o'rnatilgan muhitlarda loopback'ga proxy'siz borish uchun.
urllib.request.install_opener(urllib.request.build_opener(urllib.request.ProxyHandler({})))

HERE = os.path.dirname(os.path.abspath(__file__))
ROOT = os.path.abspath(os.path.join(HERE, "..", ".."))
BIN = os.environ.get("LOGDOWNLOADER_BIN", os.path.join(ROOT, "build", "logdownloader-linux-amd64"))
DATA = os.environ.get("SMOKE_DATA_DIR") or tempfile.mkdtemp(prefix="logdownloader-smoke-")
PORT = int(os.environ.get("SMOKE_PORT", "13000"))
VL_PORT = int(os.environ.get("SMOKE_VL_PORT", "19428"))
VL_BIG_PORT = VL_PORT + 1  # bo'laklash testi uchun tez, katta mock
LINES = 3000
BIG_LINES = 30000           # ~3.3 MB xom → EXPORT_CHUNK_MB=1 bilan 4 qism
BASE = f"http://127.0.0.1:{PORT}"
failures = []


def check(cond, msg):
    print(("  OK   " if cond else "  FAIL ") + msg)
    if not cond:
        failures.append(msg)


def req(method, path, body=None, token=None, raw=False, headers=None):
    data = json.dumps(body).encode() if body is not None else None
    r = urllib.request.Request(BASE + path, data=data, method=method)
    r.add_header("Content-Type", "application/json")
    if token:
        r.add_header("Authorization", "Bearer " + token)
    for k, v in (headers or {}).items():
        r.add_header(k, v)
    try:
        with urllib.request.urlopen(r, timeout=60) as resp:
            payload = resp.read()
            hdrs = dict(resp.headers)
            if raw:
                return resp.status, payload, hdrs
            return resp.status, (json.loads(payload) if payload.strip() else None), hdrs
    except urllib.error.HTTPError as e:
        return e.code, e.read().decode(errors="replace"), dict(e.headers)


def start_server():
    env = dict(os.environ, DATA_DIR=DATA, PORT=str(PORT), HOST="127.0.0.1", EXPORT_CHUNK_MB="1")
    env.pop("CONFIG_PATH", None)
    p = subprocess.Popen([BIN], cwd=DATA, env=env, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True)
    for _ in range(50):
        try:
            urllib.request.urlopen(BASE + "/healthz", timeout=1)
            return p
        except Exception:
            time.sleep(0.1)
    p.kill()
    out, _ = p.communicate()
    raise SystemExit("server did not start:\n" + out)


def stop_server(p):
    p.send_signal(signal.SIGTERM)
    try:
        out, _ = p.communicate(timeout=5)
    except subprocess.TimeoutExpired:
        p.kill()
        out, _ = p.communicate()
    return out


def wait_job(token, job_id, want=("done", "failed"), timeout=120):
    t0 = time.time()
    while time.time() - t0 < timeout:
        _, jobs, _ = req("GET", "/api/jobs", token=token)
        for j in jobs or []:
            if j["id"] == job_id and j["status"] in want:
                return j
        time.sleep(0.2)
    return None


def export(token, ds_id, **kw):
    body = {"query": "*", "datasource_id": ds_id}
    body.update(kw)
    st, b, _ = req("POST", "/api/export", body, token)
    if st != 200:
        raise SystemExit(f"export failed {st}: {b}")
    return wait_job(token, b["job_id"])


def tar_members(path):
    with tarfile.open(path) as tf:
        return [(m.name, m.size, tf.extractfile(m).read()) for m in tf.getmembers()]


if not os.path.exists(BIN):
    raise SystemExit(f"binary not found: {BIN} (run `make build` or set LOGDOWNLOADER_BIN)")

shutil.rmtree(DATA, ignore_errors=True)
os.makedirs(DATA, exist_ok=True)
mock = subprocess.Popen([sys.executable, "-I", os.path.join(HERE, "mock_vl.py"), str(VL_PORT), str(LINES)])
mock_big = subprocess.Popen([sys.executable, "-I", os.path.join(HERE, "mock_vl.py"), str(VL_BIG_PORT), str(BIG_LINES), "0"])
time.sleep(0.5)
srv = start_server()
out = ""
try:
    print("== Auth, rate limit, profile")
    st, body, _ = req("POST", "/api/login", {"username": "admin", "password": "admin"})
    check(st == 200, "admin login")
    admin = body["token"]
    st, bob_u, _ = req("POST", "/api/users", {"username": "bob", "password": "bobpass", "role": "user"}, admin)
    check(st == 201, "create user bob")
    st, body, _ = req("POST", "/api/login", {"username": "bob", "password": "bobpass"})
    bob = body["token"]
    check(st == 200, "bob login")
    codes = [req("POST", "/api/login", {"username": "eve", "password": "x"}, headers={"X-Forwarded-For": "10.9.9.9"})[0] for _ in range(6)]
    check(codes[:5] == [401] * 5 and codes[5] == 429, f"login brute-force blocked after 5 failures: {codes}")
    st, me, _ = req("PUT", "/api/me", {"email": "bob@example.com", "notify_on_done": True}, bob)
    check(st == 200 and me["email"] == "bob@example.com" and me["notify_on_done"], "profile update")
    st, _, _ = req("PUT", "/api/me", {"current_password": "wrong", "new_password": "newpass1"}, bob)
    check(st == 403, "password change with wrong current -> 403")
    st, _, _ = req("PUT", "/api/me", {"current_password": "bobpass", "new_password": "newpass1"}, bob)
    st2, body, _ = req("POST", "/api/login", {"username": "bob", "password": "newpass1"})
    check(st == 200 and st2 == 200, "own password change works")
    bob = body["token"]

    print("== System endpoints")
    st, hz, _ = req("GET", "/healthz")
    check(st == 200 and hz["status"] == "ok" and hz["version"], "healthz without auth")
    st, m, _ = req("GET", "/metrics", raw=True)
    check(st == 200 and b"logdownloader_jobs{status=\"done\"}" in m and b"logdownloader_build_info" in m, "metrics exposed")
    st, html, _ = req("GET", "/export", raw=True)
    check(st == 200 and b'<div id="root">' in html, "SPA fallback")

    print("== Datasource: password stripping, tenant headers, test connection")
    st, ds, _ = req("POST", "/api/datasources", {"name": "mock", "url": f"http://127.0.0.1:{VL_PORT}", "username": "u", "password": "secret",
                                                 "account_id": "7", "project_id": "", "global": True}, admin)
    check(st == 201 and "password" not in ds and ds.get("has_password") is True and ds["account_id"] == "7", "POST response: no password, has_password, account_id")
    ds_id = ds["id"]
    st, t, _ = req("POST", "/api/datasources/test", {"id": ds_id, "url": f"http://127.0.0.1:{VL_PORT}", "username": "u", "account_id": "7"}, admin)
    check(st == 200 and t["ok"] and t["status"] == 200, f"test connection ok ({t.get('latency_ms')} ms)")
    with urllib.request.urlopen(f"http://127.0.0.1:{VL_PORT}/__last_headers") as r:
        lh = {k.lower(): v for k, v in json.load(r).items()}  # Go header nomlarini kanonlashtiradi (Accountid)
    check(lh.get("accountid") == "7" and "projectid" not in lh and lh.get("authorization", "").startswith("Basic "), "AccountID header sent, ProjectID omitted, basic auth used")
    st, t, _ = req("POST", "/api/datasources/test", {"url": "http://127.0.0.1:1"}, admin)
    check(st == 200 and not t["ok"] and t["error"], "test connection failure reported")
    st, upd, _ = req("PUT", f"/api/datasources/{ds_id}", {"name": "mock2", "url": f"http://127.0.0.1:{VL_PORT}", "username": "u", "global": True}, admin)
    on_disk = json.load(open(os.path.join(DATA, "settings.json")))["datasources"][0]
    check(st == 200 and upd["has_password"] and on_disk["password"] == "secret", "PUT with empty password keeps stored password")

    print("== Log viewer proxies")
    st, res, _ = req("GET", f"/api/query?query=*&datasource_id={ds_id}&limit=10", token=admin)
    check(st == 200 and len(res["logs"]) == 10 and res["has_more"], "query proxy + pagination")
    st, f, _ = req("GET", f"/api/fields?datasource_id={ds_id}", token=admin)
    check(st == 200 and any(v["value"] == "level" for v in f["values"]), "field_names proxy")
    st, fv, _ = req("GET", f"/api/field_values?datasource_id={ds_id}&field=level", token=admin)
    check(st == 200 and [v["value"] for v in fv["values"]] == ["info", "error"], "field_values proxy")
    st, s, _ = req("GET", f"/api/streams?datasource_id={ds_id}", token=admin)
    check(st == 200 and len(s["values"]) == 1, "streams proxy")
    st, hs, _ = req("GET", f"/api/hits?datasource_id={ds_id}&query=*&start=2026-10-21T00:00:00Z&end=2026-10-21T06:00:00Z", token=admin)
    check(st == 200 and hs["total"] == LINES and hs["step"] == "600s", f"hits proxy with auto step ({hs.get('step')})")
    r = urllib.request.Request(f"{BASE}/api/tail?datasource_id={ds_id}&query=*")
    r.add_header("Authorization", "Bearer " + admin)
    with urllib.request.urlopen(r, timeout=10) as resp:
        got = [json.loads(resp.readline()) for _ in range(3)]
    check(len(got) == 3 and got[2]["_msg"] == "line 2", "tail streams lines")
    st, hist, _ = req("GET", "/api/history", token=admin)
    check(st == 200 and hist and hist[0]["query"] == "*", "query history recorded")

    print("== Export: default tar.gz, auto-chunk, ownership, persistence")
    job = export(admin, ds_id, name="smoke", format="")
    check(job and job["status"] == "done" and job["file_name"].endswith(".tar.gz") and job["format"] == "tar.gz", f"default export is tar.gz: {job and job['file_name']}")
    check(job["owner_name"] == "admin" and job["content"] == "ndjson" and job["started_at"] and job["finished_at"], "job metadata (owner, content, timestamps)")
    check(job["total_estimate"] == LINES, f"total estimate from hits = {job['total_estimate']}")
    members = tar_members(os.path.join(DATA, "exports", job["file_name"]))
    check(len(members) == 1 and members[0][2].count(b"\n") == LINES and job["lines"] == LINES, "small export: single inner file, all lines")
    jobs_json = json.load(open(os.path.join(DATA, "jobs.json")))
    check(job["id"] in jobs_json and jobs_json[job["id"]]["status"] == "done", "jobs.json persisted")
    st, bob_jobs, _ = req("GET", "/api/jobs", token=bob)
    check(not bob_jobs, "bob sees no admin jobs")
    st, _, _ = req("DELETE", f"/api/jobs/{job['id']}", token=bob)
    check(st == 403, "bob cannot delete admin job")

    print("== Download token")
    st, dt, _ = req("POST", f"/api/jobs/{job['id']}/download-token", None, admin)
    check(st == 200 and dt["token"], "download token issued")
    st, blob, hdrs = req("GET", f"/api/jobs/{job['id']}/download?dt={dt['token']}", raw=True)
    check(st == 200 and len(blob) == job["size"], "download with dt token (no Authorization header)")
    st, _, _ = req("GET", f"/api/jobs/{job['id']}/download?dt={admin}", raw=True)
    check(st == 401, "session JWT rejected as dt token")
    st, _, _ = req("GET", f"/api/jobs/{job['id']}/download?token={admin}", raw=True)
    check(st == 401, "old ?token= query auth no longer accepted")

    print("== Content formats: csv, msg, ndjson+fields; log.gz / log")
    j = export(admin, ds_id, name="csvtest", format="tar.gz", content="csv", fields=["_time", "level", "n"])
    members = tar_members(os.path.join(DATA, "exports", j["file_name"]))
    rows = list(csv.reader(io.StringIO(members[0][2].decode())))
    check(members[0][0].endswith(".csv") and rows[0] == ["_time", "level", "n"] and len(rows) == LINES + 1 and rows[1][1] == "error", "csv inside tar.gz with header row")
    j = export(admin, ds_id, name="msgtest", format="log.gz", content="msg")
    data = gzip.decompress(open(os.path.join(DATA, "exports", j["file_name"]), "rb").read())
    check(j["file_name"].endswith(".log.gz") and data.splitlines()[0] == b"line 0" and data.count(b"\n") == LINES, "msg-only log.gz")
    j = export(admin, ds_id, name="fieldstest", format="log", content="ndjson", fields=["_msg", "n"])
    first = open(os.path.join(DATA, "exports", j["file_name"]), "rb").readline()
    check(j["file_name"].endswith(".log") and json.loads(first) == {"_msg": "line 0", "n": 0}, "ndjson with field selection (plain log)")
    j = export(admin, ds_id, name="csvplain", format="log", content="csv", fields=["n"])
    first2 = open(os.path.join(DATA, "exports", j["file_name"]), "rb").read().splitlines()[:2]
    check(j["file_name"].endswith(".csv") and first2 == [b"n", b"0"], "csv plain file has header")

    print("== Auto-chunking parts + archive volumes (EXPORT_CHUNK_MB=1, archive_split_mb=1)")
    st, ds_big, _ = req("POST", "/api/datasources", {"name": "big", "url": f"http://127.0.0.1:{VL_BIG_PORT}", "global": True}, admin)
    big = export(admin, ds_big["id"], name="big", format="tar.gz")
    members = tar_members(os.path.join(DATA, "exports", big["file_name"]))
    check(big["status"] == "done" and len(members) >= 3 and big["parts"] == len(members), f"archive split into {len(members)} parts")
    check(all(n.endswith("_part%03d.log" % (i + 1)) for i, (n, _, _) in enumerate(members)), "part names sequential")
    check(all(0 < s <= 1024 * 1024 for _, s, _ in members) and sum(d.count(b"\n") for _, _, d in members) == BIG_LINES and all(d.endswith(b"\n") for _, _, d in members), "parts <= 1 MB, line boundaries, all lines")
    vol = export(admin, ds_big["id"], name="vol", format="tar.gz", archive_split_mb=1, query="* RANDOM")
    files = vol.get("files") or []
    names = [f["name"] for f in files]
    check(vol["status"] == "done" and len(files) >= 2 and names[0].endswith("_vol001.tar.gz") and names[-1].endswith("_vol%03d.tar.gz" % len(files)),
          f"archive split into {len(files)} volumes: {names}")
    check(vol["file_name"] == names[0] and sum(f["parts"] for f in files) == vol["parts"] and sum(f["size"] for f in files) == vol["size"], "volume parts/sizes add up")
    allparts = []
    for f in files:
        allparts += tar_members(os.path.join(DATA, "exports", f["name"]))
    check(sum(d.count(b"\n") for _, _, d in allparts) == BIG_LINES and [n for n, _, _ in allparts] == sorted(n for n, _, _ in allparts), "all lines across volumes, part names global order")
    st, vsh, _ = req("POST", f"/api/jobs/{vol['id']}/share", {"max_downloads": 0}, admin)
    check(st == 201 and len(vsh["files"]) == len(files) and vsh["files"][1]["url"].endswith("/dl/" + vsh["token"] + "/2"), "share lists per-volume URLs")
    st, blob2, _ = req("GET", f"/dl/{vsh['token']}/2", raw=True)
    check(st == 200 and len(blob2) == files[1]["size"], "download volume 2 via share")
    st, _, _ = req("GET", f"/api/jobs/{vol['id']}/download?file={names[1]}", token=admin, raw=True)
    check(st == 200, "download specific file via API")
    req("DELETE", f"/api/jobs/{vol['id']}", token=admin)
    check(not any(n in os.listdir(os.path.join(DATA, "exports")) for n in names), "all volumes removed on delete")

    print("== Time windows + retry/rollback (5h range → 5 windows, FAIL_ONCE)")
    # Mock qatorlari 10:00–10:49 da; 08:00–13:00 → 5 ta soatlik oyna, faqat 10:00 oynasida ma'lumot.
    j = export(admin, ds_id, name="win", format="tar.gz", content="msg", query="* FAIL_ONCE",
               start="2026-10-21T08:00:00Z", end="2026-10-21T13:00:00Z")
    check(j and j["status"] == "done" and j["windows_total"] == 5 and j["windows_done"] == 5, f"windows planned/done: {j and (j.get('windows_total'), j.get('windows_done'))}")
    check(j["lines"] == LINES, f"all windows exported: {j['lines']} lines")
    check(j["retries"] == 5, f"each window retried once after 503 (retries={j['retries']})")
    members = tar_members(os.path.join(DATA, "exports", j["file_name"]))
    data = members[0][2]
    check(data.count(b"\n") == LINES and data.splitlines()[0] == b"line 0" and data.splitlines()[-1] == b"line %d" % (LINES - 1) and b"\x00" not in data,
          f"file intact after rollback ({data.count(b'\\n')} lines, no zero gaps)")
    j = export(admin, ds_id, name="desc", content="msg", sort_order="desc", start="2026-10-21T08:00:00Z", end="2026-10-21T13:00:00Z")
    check(j["status"] == "done" and j["windows_total"] == 5 and j["lines"] == LINES, "desc order with windows")
    j = export(admin, ds_id, name="open", content="msg")
    check(j["status"] == "done" and j["windows_total"] >= 20 and j["lines"] == LINES, f"open range: earliest-time probe → {j['windows_total']} windows, {j['lines']} lines")

    print("== Share: files list, no Range, limit 1 -> purge")
    st, sh, _ = req("POST", f"/api/jobs/{big['id']}/share", {"max_downloads": 1}, admin)
    check(st == 201 and len(sh["files"]) == 1 and sh["files"][0]["url"].endswith("/dl/" + sh["token"]), "share created with file list")
    r = urllib.request.Request(sh["url"])
    r.add_header("Range", "bytes=0-9")
    with urllib.request.urlopen(r) as resp:
        payload = resp.read()
        check(resp.status == 200 and resp.headers.get("Accept-Ranges") == "none" and len(payload) == big["size"], "Range ignored, full file")
    st, _, _ = req("GET", f"/dl/{sh['token']}")
    check(st in (404, 410), f"second download rejected ({st})")
    time.sleep(0.3)
    check(not os.path.exists(os.path.join(DATA, "exports", big["file_name"])), "file purged after limit")

    print("== Queue + cancel")
    st, _, _ = req("PUT", "/api/settings", {"worker": {"max_concurrent": 1}}, admin)
    check(st == 200, "set max_concurrent=1")
    ids = []
    for i in range(3):
        st, b, _ = req("POST", "/api/export", {"query": "* SLOW", "datasource_id": ds_id, "name": f"q{i}"}, admin)
        ids.append(b["job_id"])
    time.sleep(0.6)
    st, jobs, _ = req("GET", "/api/jobs", token=admin)
    statuses = {j["id"]: j["status"] for j in jobs}
    check(statuses[ids[0]] == "running" and statuses[ids[1]] == "queued" and statuses[ids[2]] == "queued", f"1 running, 2 queued: {[statuses[i] for i in ids]}")
    st, stats, _ = req("GET", "/api/stats", token=admin)
    check(stats["worker"]["running"] == 1 and stats["worker"]["queued"] == 2, "stats reflect queue")
    st, _, _ = req("DELETE", f"/api/jobs/{ids[1]}", token=admin)
    check(st == 204, "delete queued job")
    st, _, _ = req("DELETE", f"/api/jobs/{ids[0]}", token=admin)
    check(st == 204, "delete running job (cancel)")
    j2 = wait_job(admin, ids[2], timeout=60)
    check(j2 and j2["status"] == "done", "remaining queued job ran after cancel")
    time.sleep(0.5)
    leftovers = [f for f in os.listdir(os.path.join(DATA, "exports")) if "q0_" in f or "q1_" in f or f.endswith(".tmp")]
    check(leftovers == [], f"no leftover files from cancelled jobs: {leftovers}")
    req("PUT", "/api/settings", {"worker": {"max_concurrent": 2}}, admin)

    print("== Schedules")
    st, sc, _ = req("POST", "/api/schedules", {"name": "every-min", "query": "*", "datasource_id": ds_id, "cron": "* * * * *",
                                                "lookback_minutes": 120, "align": True, "enabled": True, "format": "log.gz", "content": "msg"}, admin)
    check(st == 201 and sc["next_run_at"], "schedule created with next_run_at")
    st, _, _ = req("POST", "/api/schedules", {"name": "bad", "query": "*", "datasource_id": ds_id, "cron": "99 * * * *", "enabled": True}, admin)
    check(st == 400, "invalid cron rejected")
    st, run, _ = req("POST", f"/api/schedules/{sc['id']}/run", None, admin)
    sj = wait_job(admin, run["job_id"])
    check(sj and sj["status"] == "done" and sj["schedule_id"] == sc["id"] and sj["file_name"].startswith("every-min_") and sj["file_name"].endswith(".log.gz"), "run-now creates job bound to schedule")
    st, sc2, _ = req("GET", "/api/schedules", token=admin)
    check(sc2[0]["last_job_id"] == run["job_id"], "schedule last_job_id updated")
    st, _, _ = req("GET", "/api/schedules", token=bob)
    check(st == 200, "bob lists own (empty) schedules")
    st, _, _ = req("DELETE", f"/api/schedules/{sc['id']}", token=bob)
    check(st == 403, "bob cannot delete admin schedule")
    body = dict(sc); body["enabled"] = False
    st, sc3, _ = req("PUT", f"/api/schedules/{sc['id']}", body, admin)
    check(st == 200 and not sc3["enabled"] and sc3.get("next_run_at") is None, "schedule disabled (next_run_at cleared)")

    print("== Settings + notify test + audit")
    st, sv, _ = req("PUT", "/api/settings", {"retention": {"days": 7, "quota_gb": 0.5}, "public_url": "https://logs.example.com/",
                                            "notify": {"telegram_bot_token": "123:abc", "webhook_url": "", "smtp": {"host": "smtp.local", "port": 25, "username": "", "password": "", "from": "ld@x", "starttls": False}}}, admin)
    check(st == 200 and sv["retention"]["days"] == 7 and sv["public_url"] == "https://logs.example.com" and sv["notify"]["has_telegram_token"] and sv["notify"]["smtp"]["host"] == "smtp.local", "settings updated, secrets masked")
    st, sv, _ = req("PUT", "/api/settings", {"notify": {"clear_telegram_token": True}}, admin)
    check(st == 200 and not sv["notify"]["has_telegram_token"], "telegram token cleared")
    st, _, _ = req("GET", "/api/settings", token=bob)
    check(st == 403, "settings admin-only")
    st, nt, _ = req("POST", "/api/settings/notify/test", {"channel": "telegram"}, admin)
    check(st == 200 and not nt["ok"], "notify test reports not configured")
    st, sh2, _ = req("POST", f"/api/jobs/{sj['id']}/share", {"max_downloads": 0}, admin)
    check(sh2["url"].startswith("https://logs.example.com/dl/"), "share URL uses public_url")
    st, au, _ = req("GET", "/api/audit?limit=500", token=admin)
    actions = {e["action"] for e in au["entries"]}
    need = {"login.ok", "login.fail", "login.blocked", "export.start", "job.delete", "share.create", "share.download", "schedule.create", "settings.update", "user.create", "profile.update", "password.change", "job.download"}
    check(need <= actions, f"audit has expected actions (missing: {sorted(need - actions)})")
    st, au2, _ = req("GET", "/api/audit?action=login.fail&user=eve", token=admin)
    check(au2["total"] == 5 and all(e["ip"] == "10.9.9.9" for e in au2["entries"]), "audit filter by action+user, ip recorded")
    st, _, _ = req("GET", "/api/audit", token=bob)
    check(st == 403, "audit admin-only")
    st, af, _ = req("GET", "/api/audit/download", token=admin, raw=True)
    check(st == 200 and af.count(b"\n") >= au["total"], "audit download")

    print("== LDAP settings (server yo'q — faqat sozlama/oqim)")
    st, sv, _ = req("PUT", "/api/settings", {"ldap": {"enabled": True, "url": "ldap://127.0.0.1:1", "base_dn": "DC=example,DC=com", "bind_dn": "svc@example.com", "bind_password": "s3cret",
                                                       "admin_group_dn": "CN=Admins,DC=example,DC=com", "default_role": "user", "timeout_sec": 2}}, admin)
    check(st == 200 and sv["ldap"]["enabled"] and sv["ldap"]["has_bind_password"] and "bind_password" not in sv["ldap"] and sv["ldap"]["timeout_sec"] == 2, "ldap settings saved, bind password masked")
    on_disk = json.load(open(os.path.join(DATA, "settings.json")))["ldap"]
    check(on_disk["bind_password"] == "s3cret" and on_disk["url"] == "ldap://127.0.0.1:1", "ldap settings on disk")
    st, lt, _ = req("POST", "/api/settings/ldap/test", {"ldap": {"enabled": True, "url": "ldap://127.0.0.1:1", "base_dn": "DC=example,DC=com", "timeout_sec": 2}}, admin)
    check(st == 200 and not lt["ok"] and "ulanish" in lt["error"].lower(), f"ldap test reports connection error: {lt.get('error', '')[:60]}")
    st, _, _ = req("POST", "/api/login", {"username": "admin", "password": "admin"})
    check(st == 200, "local admin still logs in with LDAP enabled (local precedence)")
    st, _, _ = req("POST", "/api/login", {"username": "ldapuser", "password": "x"}, headers={"X-Forwarded-For": "10.8.8.8"})
    check(st == 401, "unknown user with unreachable LDAP -> 401")
    st, au3, _ = req("GET", "/api/audit?action=login.fail&user=ldapuser", token=admin)
    check(au3["total"] == 1 and au3["entries"][0]["details"].startswith("ldap:"), "audit records ldap failure reason")
    st, sv, _ = req("PUT", "/api/settings", {"ldap": {"enabled": False, "url": "", "base_dn": "", "clear_bind_password": True, "default_role": "user", "timeout_sec": 10}}, admin)
    check(st == 200 and not sv["ldap"]["enabled"] and not sv["ldap"]["has_bind_password"], "ldap disabled, bind password cleared")

    print("== JWT: deleted user / role change")
    st, _, _ = req("PUT", f"/api/users/{bob_u['id']}", {"role": "admin"}, admin)
    st, me, _ = req("GET", "/api/me", token=bob)
    check(st == 200 and me["role"] == "admin", "role change effective without re-login")
    st, _, _ = req("DELETE", f"/api/users/{bob_u['id']}", token=admin)
    st, _, _ = req("GET", "/api/me", token=bob)
    check(st == 401, "deleted user token rejected")

    print("== Auto-share on done")
    j = export(admin, ds_id, name="autoshare", content="msg", auto_share=True)
    check(j["status"] == "done" and j.get("share_url", "").startswith("https://logs.example.com/dl/"), f"auto share url: {j.get('share_url')}")

    print("== Restart recovery: interrupted running + queued re-enqueued + retention quota prune")
    req("PUT", "/api/settings", {"worker": {"max_concurrent": 1}, "retention": {"days": 0, "quota_gb": 0.000001}}, admin)
    st, b1, _ = req("POST", "/api/export", {"query": "* SLOW", "datasource_id": ds_id, "name": "interrupted"}, admin)
    st, b2, _ = req("POST", "/api/export", {"query": "*", "datasource_id": ds_id, "name": "queued-survivor"}, admin)
    time.sleep(0.6)
    with open(os.path.join(DATA, "exports", "stray_orphan.log"), "w") as f:
        f.write("orphan\n")
    out += stop_server(srv)
    srv = start_server()
    st, body, _ = req("POST", "/api/login", {"username": "admin", "password": "admin"})
    admin = body["token"]
    st, jobs, _ = req("GET", "/api/jobs", token=admin)
    j1 = next((j for j in jobs if j["id"] == b1["job_id"]), None)
    check(j1 is not None and j1["status"] == "failed" and "qayta ishga tushdi" in j1["error"], "interrupted running job marked failed")
    j2 = wait_job(admin, b2["job_id"], timeout=60)
    check(j2 and j2["status"] == "done", "queued job re-enqueued after restart and completed")
    check(not os.path.exists(os.path.join(DATA, "exports", "stray_orphan.log")), "orphan file removed")
    st, jobs, _ = req("GET", "/api/jobs", token=admin)
    old_done = [j for j in jobs if j["status"] == "done" and j["id"] not in (b2["job_id"],)]
    check(len(old_done) == 0, f"retention quota pruned old done jobs at startup (left: {len(old_done)})")
finally:
    out += stop_server(srv)
    mock.kill()
    mock_big.kill()
    print("---- server log tail ----")
    print("\n".join(out.strip().splitlines()[-12:]))
    if "DATA RACE" in out:
        failures.append("go race detector reported DATA RACE (see server log)")
        print(out)

print()
if failures:
    print(f"FAILED: {len(failures)}")
    for f in failures:
        print(" -", f)
    sys.exit(1)
print("ALL PASSED")
