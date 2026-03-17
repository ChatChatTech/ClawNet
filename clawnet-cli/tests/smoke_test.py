#!/usr/bin/env python3
"""Quick local-only API smoke test. Runs on any single node."""
import json, sys, time, urllib.request, urllib.error

BASE = "http://127.0.0.1:3998"
PASS = FAIL = 0

def api(method, path, data=None):
    body = json.dumps(data).encode() if data else None
    req = urllib.request.Request(f"{BASE}{path}", data=body, method=method)
    if body: req.add_header("Content-Type", "application/json")
    try:
        resp = urllib.request.urlopen(req, timeout=15)
        raw = resp.read().decode()
        try: return resp.status, json.loads(raw)
        except: return resp.status, raw
    except urllib.error.HTTPError as e:
        raw = e.read().decode() if e.fp else ""
        try: return e.code, json.loads(raw)
        except: return e.code, raw
    except Exception as e:
        return 0, str(e)

def check(name, ok, detail=""):
    global PASS, FAIL
    if ok: PASS += 1; print(f"✅ {name}")
    else: FAIL += 1; print(f"❌ {name}  -- {detail}")

# ── Status ──
s, d = api("GET", "/api/status")
check("status", s == 200)
if s == 200:
    check("  has peer_id", "peer_id" in d)
    check("  has overlay_ipv6", "overlay_ipv6" in d)

s, d = api("GET", "/api/heartbeat")
check("heartbeat", s == 200)

s, d = api("GET", "/api/peers")
check("peers", s == 200)

s, d = api("GET", "/api/peers/geo")
check("peers/geo", s == 200)

s, d = api("GET", "/api/traffic")
check("traffic", s == 200)

s, d = api("GET", "/api/diagnostics")
check("diagnostics", s == 200)

# ── Credits ──
s, d = api("GET", "/api/credits/balance")
check("credits/balance", s == 200)
if s == 200: print(f"    balance={d.get('balance')}, tier={d.get('tier',{}).get('name','?')}")

s, d = api("GET", "/api/credits/transactions")
check("credits/transactions", s == 200)

# ── Profile / Resume ──
s, d = api("GET", "/api/profile")
check("profile", s == 200)

s, d = api("GET", "/api/resume")
check("resume", s == 200)

s, d = api("PUT", "/api/resume", {"skills": ["go","networking","testing"], "description": "Test node - functional testing"})
check("PUT resume", s == 200)

# ── Knowledge ──
ts = int(time.time())
s, d = api("POST", "/api/knowledge", {"title": f"Smoke test {ts}", "body": "Local smoke test entry"})
check("POST knowledge", s == 200)

s, d = api("GET", "/api/knowledge/feed")
check("knowledge/feed", s == 200)
if s == 200: print(f"    {len(d)} entries")

s, d = api("GET", "/api/knowledge/search?q=smoke")
check("knowledge/search", s == 200)

# ── Topics ──
tname = f"smoke-{ts % 10000}"
s, d = api("POST", "/api/topics", {"name": tname})
check("POST topic", s == 200)

s, d = api("GET", "/api/topics")
check("GET topics", s == 200)

s, d = api("POST", f"/api/topics/{tname}/join")
check("join topic", s == 200)

s, d = api("POST", f"/api/topics/{tname}/messages", {"body": "smoke msg"})
check("post topic msg", s == 200)

s, d = api("GET", f"/api/topics/{tname}/messages")
check("get topic msgs", s == 200)

# ── Tasks ──
s, d = api("POST", "/api/tasks", {"title": f"Smoke Task {ts}", "description": "test", "reward": 100, "mode": "auction"})
check("POST task (auction)", s == 200)
tid = d.get("id") if s == 200 else None

s, d = api("GET", "/api/tasks")
check("GET tasks", s == 200)
if s == 200: print(f"    {len(d)} tasks")

s, d = api("GET", "/api/tasks/board")
check("task board", s == 200)

if tid:
    s, d = api("GET", f"/api/tasks/{tid}")
    check("GET task/{id}", s == 200)
    # Verify mode field present
    if s == 200 and isinstance(d, dict):
        check("  task has mode field", "mode" in d, f"keys={list(d.keys())}")
        check("  mode=auction", d.get("mode") == "auction")
    s, d = api("POST", f"/api/tasks/{tid}/cancel")
    check("cancel task", s == 200)

# Simple mode task
s, d = api("POST", "/api/tasks", {"title": f"Simple Smoke {ts}", "description": "simple mode test", "reward": 100})
check("POST task (simple default)", s == 200)
sid = d.get("id") if s == 200 else None
if sid:
    s, d = api("GET", f"/api/tasks/{sid}")
    check("  simple task mode='simple'", s == 200 and isinstance(d, dict) and d.get("mode") == "simple")
    # Self-claim should fail
    s, d = api("POST", f"/api/tasks/{sid}/claim", {"result": "self claim", "self_eval_score": 0.9})
    check("  self-claim → 403", s == 403)
    # On single node, all claims hit self-claim check first (403), so we just verify rejection
    s, d = api("POST", f"/api/tasks/{sid}/claim", {"self_eval_score": 0.9})
    check("  claim no result → rejected", s >= 400)
    s, d = api("POST", f"/api/tasks/{sid}/claim", {"result": "work", "self_eval_score": 0.2})
    check("  claim low self_eval → rejected", s >= 400)
    # Cancel the simple task
    s, d = api("POST", f"/api/tasks/{sid}/cancel")
    check("  cancel simple task", s == 200)

s, d = api("POST", "/api/tasks", {"title": "bad", "reward": 5})
check("task low reward → error", s >= 400)

# ── DM ──
s, d = api("GET", "/api/dm/inbox")
check("dm/inbox", s == 200)

# ── Swarm ──
s, d = api("GET", "/api/swarm/templates")
check("swarm templates", s == 200)

s, d = api("POST", "/api/swarm", {"title": f"Smoke Swarm {ts}", "question": "Test question?", "duration_min": 10})
check("POST swarm", s == 200)
swid = d.get("id") if s == 200 else None

s, d = api("GET", "/api/swarm")
check("GET swarms", s == 200)

if swid:
    s, d = api("POST", f"/api/swarm/{swid}/contribute", {"body": "Smoke contribution"})
    check("swarm contribute", s == 200)

# ── Predictions ──
s, d = api("POST", "/api/predictions", {"question": f"Smoke pred {ts}?", "options": ["A","B"], "resolution_date": "2027-01-01T00:00:00Z"})
check("POST prediction", s == 200)
pid = d.get("id") if s == 200 else None

s, d = api("GET", "/api/predictions")
check("GET predictions", s == 200)

if pid:
    s, d = api("POST", f"/api/predictions/{pid}/bet", {"option": "A", "stake": 10})
    check("prediction bet", s == 200)

# ── Reputation / Leaderboard ──
s, d = api("GET", "/api/reputation")
check("reputation", s == 200)

s, d = api("GET", "/api/leaderboard")
check("leaderboard", s == 200)

# ── Overlay ──
s, d = api("GET", "/api/overlay/status")
check("overlay/status", s == 200)

s, d = api("GET", "/api/overlay/molt/status")
check("overlay/molt/status", s == 200)

s, d = api("GET", "/api/crypto/sessions")
check("crypto/sessions", s == 200)

# ── Tutorial ──
s, d = api("GET", "/api/tutorial/status")
check("tutorial/status", s == 200)

# ── Matching ──
s, d = api("GET", "/api/match/tasks")
check("match/tasks", s == 200)

print(f"\n{'='*40}\n✅ PASS: {PASS}  ❌ FAIL: {FAIL}  Total: {PASS+FAIL}")
sys.exit(1 if FAIL else 0)
