#!/usr/bin/env python3
import json, random, time, urllib.request, sys
from datetime import datetime, timezone, timedelta

VL_URL = "http://victorialogs:9428/insert/jsonline"

LEVELS = ["info", "warn", "error", "debug"]
SERVICES = ["api-gateway", "auth-service", "user-service", "payment-service", "order-service", "notification-service"]
HOSTS = ["node-1", "node-2", "node-3"]

MESSAGES = {
    "info": ["request completed", "health check passed", "cache hit", "connection established", "config loaded", "session started", "task finished", "data synced"],
    "warn": ["high memory usage", "slow query detected", "retry attempt", "connection pool near limit", "deprecated API called", "rate limit approaching", "disk usage above 80%"],
    "error": ["connection refused", "timeout exceeded", "null pointer exception", "out of memory", "permission denied", "database connection lost", "invalid token", "service unavailable"],
    "debug": ["processing request", "parsing payload", "validating input", "building response", "checking cache", "resolving dependency", "executing query"],
}

print("Waiting for VictoriaLogs...", flush=True)
time.sleep(5)

now = datetime.now(timezone.utc)
start = now - timedelta(hours=5)
current = start
total = 0
batch = []

while current < now:
    count = random.randint(1, 5)
    for _ in range(count):
        level = random.choice(LEVELS)
        ts = current.strftime("%Y-%m-%dT%H:%M:%S.000Z")
        trace_id = "%032x" % random.getrandbits(128)
        line = {
            "_time": ts,
            "_msg": random.choice(MESSAGES[level]),
            "level": level,
            "service": random.choice(SERVICES),
            "trace_id": trace_id[:16],
            "host": random.choice(HOSTS),
        }
        batch.append(json.dumps(line))

    if len(batch) >= 500:
        body = "\n".join(batch).encode()
        req = urllib.request.Request(VL_URL, data=body, headers={"Content-Type": "application/stream+json"})
        urllib.request.urlopen(req)
        total += len(batch)
        print(f"\rInserted {total} logs...", end="", flush=True)
        batch = []

    current += timedelta(seconds=2)

if batch:
    body = "\n".join(batch).encode()
    req = urllib.request.Request(VL_URL, data=body, headers={"Content-Type": "application/stream+json"})
    urllib.request.urlopen(req)
    total += len(batch)

print(f"\nDone! Inserted {total} logs covering last 5 hours.", flush=True)

# Keep alive
while True:
    time.sleep(3600)
