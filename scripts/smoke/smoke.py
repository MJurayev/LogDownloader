#!/usr/bin/env python3
"""LogDownloader smoke test — backend xatti-harakatlarini end-to-end tekshiradi.

Ishga tushirish (repo ildizidan):
    make build
    python3 -I scripts/smoke/smoke.py

Env:
    LOGDOWNLOADER_BIN  — tekshiriladigan binary (default: build/logdownloader-linux-amd64)
    SMOKE_DATA_DIR     — vaqtinchalik DATA_DIR (default: tempfile.mkdtemp())
    SMOKE_PORT         — server porti (default 13000), SMOKE_VL_PORT — mock VictoriaLogs porti (19428)

Tekshiradi: datasource parolining API'da qaytmasligi, format default .tar.gz,
job ownership (user/admin), jobs.json persist, share link (Range yo'q, limit),
running job'ni o'chirganda cancel, JWT (o'chirilgan user / rol o'zgarishi),
restart recovery (uzilgan job failed, orphan fayl o'chadi).
"""
import json, os, shutil, signal, subprocess, sys, tarfile, tempfile, time, urllib.request, urllib.error

# http_proxy o'rnatilgan muhitlarda loopback'ga proxy'siz borish uchun.
urllib.request.install_opener(urllib.request.build_opener(urllib.request.ProxyHandler({})))

HERE = os.path.dirname(os.path.abspath(__file__))
ROOT = os.path.abspath(os.path.join(HERE, "..", ".."))
BIN = os.environ.get("LOGDOWNLOADER_BIN", os.path.join(ROOT, "build", "logdownloader-linux-amd64"))
DATA = os.environ.get("SMOKE_DATA_DIR") or tempfile.mkdtemp(prefix="logdownloader-smoke-")
PORT = int(os.environ.get("SMOKE_PORT", "13000"))
VL_PORT = int(os.environ.get("SMOKE_VL_PORT", "19428"))
BASE = f"http://127.0.0.1:{PORT}"
failures = []


def check(cond, msg):
    print(("  OK   " if cond else "  FAIL ") + msg)
    if not cond:
        failures.append(msg)


def req(method, path, body=None, token=None, raw=False):
    data = json.dumps(body).encode() if body is not None else None
    r = urllib.request.Request(BASE + path, data=data, method=method)
    r.add_header("Content-Type", "application/json")
    if token:
        r.add_header("Authorization", "Bearer " + token)
    try:
        with urllib.request.urlopen(r, timeout=30) as resp:
            payload = resp.read()
            hdrs = dict(resp.headers)
            if raw:
                return resp.status, payload, hdrs
            return resp.status, (json.loads(payload) if payload.strip() else None), hdrs
    except urllib.error.HTTPError as e:
        return e.code, e.read().decode(errors="replace"), dict(e.headers)


def start_server():
    env = dict(os.environ, DATA_DIR=DATA, PORT=str(PORT), HOST="127.0.0.1")
    env.pop("CONFIG_PATH", None)
    p = subprocess.Popen([BIN], cwd=DATA, env=env, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True)
    for _ in range(50):
        try:
            urllib.request.urlopen(BASE + "/", timeout=1)
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


def wait_job(token, job_id, want=("done", "failed"), timeout=60):
    t0 = time.time()
    while time.time() - t0 < timeout:
        _, jobs, _ = req("GET", "/api/jobs", token=token)
        for j in jobs or []:
            if j["id"] == job_id and j["status"] in want:
                return j
        time.sleep(0.2)
    return None


if not os.path.exists(BIN):
    raise SystemExit(f"binary not found: {BIN} (run `make build` or set LOGDOWNLOADER_BIN)")

shutil.rmtree(DATA, ignore_errors=True)
os.makedirs(DATA, exist_ok=True)
mock = subprocess.Popen([sys.executable, "-I", os.path.join(HERE, "mock_vl.py"), str(VL_PORT)])
time.sleep(0.5)
srv = start_server()
out = ""
try:
    print("== Auth")
    st, body, _ = req("POST", "/api/login", {"username": "admin", "password": "admin"})
    check(st == 200, "admin login")
    admin = body["token"]
    st, bob_u, _ = req("POST", "/api/users", {"username": "bob", "password": "bobpass", "role": "user"}, admin)
    check(st == 201, "create user bob")
    st, body, _ = req("POST", "/api/login", {"username": "bob", "password": "bobpass"})
    bob = body["token"]
    check(st == 200, "bob login")

    print("== Static / SPA")
    st, html, _ = req("GET", "/export", raw=True)
    check(st == 200 and b'<div id="root">' in html, "GET /export -> SPA index (client-side redirect to /)")

    print("== Datasource password stripping")
    st, ds, _ = req("POST", "/api/datasources", {"name": "mock", "url": f"http://127.0.0.1:{VL_PORT}", "username": "u", "password": "secret", "global": True}, admin)
    check(st == 201 and "password" not in ds and ds.get("has_password") is True, "POST response: no password, has_password=true")
    st, lst, _ = req("GET", "/api/datasources", token=admin)
    check(all("password" not in d for d in lst) and lst[0]["has_password"], "GET list: password stripped")
    ds_id = ds["id"]
    st, upd, _ = req("PUT", f"/api/datasources/{ds_id}", {"name": "mock2", "url": f"http://127.0.0.1:{VL_PORT}", "username": "u", "global": True}, admin)
    on_disk = json.load(open(os.path.join(DATA, "settings.json")))["datasources"][0]
    check(st == 200 and upd["has_password"] and on_disk["password"] == "secret" and on_disk["name"] == "mock2", "PUT with empty password keeps stored password")
    st, upd, _ = req("PUT", f"/api/datasources/{ds_id}", {"name": "mock2", "url": f"http://127.0.0.1:{VL_PORT}", "username": "", "global": True}, admin)
    on_disk = json.load(open(os.path.join(DATA, "settings.json")))["datasources"][0]
    check(st == 200 and not upd["has_password"] and on_disk.get("password", "") == "", "PUT with empty username clears password")

    print("== Export (format bo'sh -> tar.gz), ownership, persistence")
    st, body, _ = req("POST", "/api/export", {"query": "*", "datasource_id": ds_id, "name": "smoke", "format": ""}, admin)
    check(st == 200, "start export as admin")
    job_id = body["job_id"]
    job = wait_job(admin, job_id)
    check(job is not None and job["status"] == "done", f"export done: {job and job.get('error')}")
    check(job["file_name"].endswith(".tar.gz") and job["format"] == "tar.gz", f"file is tar.gz: {job['file_name']}")
    check(job["owner_name"] == "admin" and job["owner_id"], "job has owner_name=admin and owner_id")
    fpath = os.path.join(DATA, "exports", job["file_name"])
    with tarfile.open(fpath) as tf:
        names = tf.getnames()
        n_lines = sum(1 for _ in tf.extractfile(names[0]))
    check(n_lines == 3000 and job["lines"] == 3000, f"tar.gz contains {n_lines} lines, job.lines={job['lines']}")
    jobs_json = json.load(open(os.path.join(DATA, "jobs.json")))
    check(job_id in jobs_json and jobs_json[job_id]["status"] == "done", "jobs.json persisted with done status")

    st, bob_jobs, _ = req("GET", "/api/jobs", token=bob)
    check(not bob_jobs, "bob sees no admin jobs")
    st, _, _ = req("GET", f"/api/jobs/{job_id}/download", token=bob)
    check(st == 403, "bob cannot download admin job (403)")
    st, _, _ = req("DELETE", f"/api/jobs/{job_id}", token=bob)
    check(st == 403, "bob cannot delete admin job (403)")
    st, _, _ = req("POST", f"/api/jobs/{job_id}/share", {"max_downloads": 1}, bob)
    check(st == 403, "bob cannot share admin job (403)")
    st, blob, hdrs = req("GET", f"/api/jobs/{job_id}/download", token=admin, raw=True)
    check(st == 200 and len(blob) == job["size"] and hdrs.get("Content-Type") == "application/gzip", "admin download OK, size matches")

    print("== Share: no Range, limit 1 -> purge")
    st, sh, _ = req("POST", f"/api/jobs/{job_id}/share", {"max_downloads": 1}, admin)
    check(st == 201, "share created")
    tok = sh["token"]
    r = urllib.request.Request(f"{BASE}/dl/{tok}")
    r.add_header("Range", "bytes=0-9")
    with urllib.request.urlopen(r) as resp:
        payload = resp.read()
        check(resp.status == 200 and resp.headers.get("Accept-Ranges") == "none" and len(payload) == job["size"], "Range ignored: full file, Accept-Ranges: none")
    st, _, _ = req("GET", f"/dl/{tok}")
    check(st in (404, 410), f"second download rejected ({st})")
    time.sleep(0.3)
    check(not os.path.exists(fpath), "file purged after limit")
    st, jobs, _ = req("GET", "/api/jobs", token=admin)
    check(all(j["id"] != job_id for j in jobs or []), "job purged after limit")

    print("== Cancel running export on delete")
    st, body, _ = req("POST", "/api/export", {"query": "*", "datasource_id": ds_id, "name": "cancelme"}, admin)
    jid2 = body["job_id"]
    time.sleep(0.5)
    st, jobs, _ = req("GET", "/api/jobs", token=admin)
    j2 = next(j for j in jobs if j["id"] == jid2)
    check(j2["status"] == "running", "job running before delete")
    st, _, _ = req("DELETE", f"/api/jobs/{jid2}", token=admin)
    check(st == 204, "delete running job -> 204")
    time.sleep(1.5)
    leftovers = os.listdir(os.path.join(DATA, "exports"))
    check(leftovers == [], f"no leftover files after cancel: {leftovers}")
    st, jobs, _ = req("GET", "/api/jobs", token=admin)
    check(all(j["id"] != jid2 for j in jobs or []), "cancelled job removed")

    print("== JWT: deleted user / role change")
    st, _, _ = req("GET", "/api/me", token=bob)
    check(st == 200, "bob token valid")
    st, _, _ = req("PUT", f"/api/users/{bob_u['id']}", {"role": "admin"}, admin)
    st, me, _ = req("GET", "/api/me", token=bob)
    check(st == 200 and me["role"] == "admin", "role change effective without re-login")
    st, _, _ = req("DELETE", f"/api/users/{bob_u['id']}", token=admin)
    st, _, _ = req("GET", "/api/me", token=bob)
    check(st == 401, "deleted user token rejected (401)")

    print("== Restart recovery: interrupted running job + orphan file")
    st, body, _ = req("POST", "/api/export", {"query": "*", "datasource_id": ds_id, "name": "interrupted"}, admin)
    jid3 = body["job_id"]
    time.sleep(0.5)
    with open(os.path.join(DATA, "exports", "stray_orphan.log"), "w") as f:
        f.write("orphan\n")
    out += stop_server(srv)
    srv = start_server()
    st, body, _ = req("POST", "/api/login", {"username": "admin", "password": "admin"})
    admin = body["token"]
    st, jobs, _ = req("GET", "/api/jobs", token=admin)
    j3 = next((j for j in jobs if j["id"] == jid3), None)
    check(j3 is not None and j3["status"] == "failed" and "qayta ishga tushdi" in j3["error"], f"interrupted job marked failed: {j3 and j3['error']}")
    leftovers = os.listdir(os.path.join(DATA, "exports"))
    check(leftovers == [], f"partial + orphan files removed after restart: {leftovers}")
finally:
    out += stop_server(srv)
    mock.kill()
    print("---- server log tail ----")
    print("\n".join(out.strip().splitlines()[-8:]))
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
