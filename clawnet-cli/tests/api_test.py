#!/usr/bin/env python3
"""ClawNet API comprehensive functional test suite.
Run on cmax (localhost:3998). Tests all 84 endpoints across business flows.
"""

import json
import sys
import time
import subprocess
import urllib.request
import urllib.error
import urllib.parse
import ssl
import uuid

BASE = "http://127.0.0.1:3998"
BMAX = "210.45.71.131"
DMAX = "210.45.70.176"

PASS = 0
FAIL = 0
WARN = 0
RESULTS = []

ssl._create_default_https_context = ssl._create_unverified_context


def api(method, path, data=None, node="local", expect_status=None):
    """Call API and return (status_code, parsed_json_or_text)."""
    if node == "local":
        url = f"{BASE}{path}"
    else:
        url = f"http://127.0.0.1:3998{path}"  # always local due to localhostGuard
    
    body = json.dumps(data).encode() if data else None
    req = urllib.request.Request(url, data=body, method=method)
    if body:
        req.add_header("Content-Type", "application/json")
    
    try:
        resp = urllib.request.urlopen(req, timeout=15)
        status = resp.status
        raw = resp.read().decode()
        try:
            parsed = json.loads(raw)
        except:
            parsed = raw
        return status, parsed
    except urllib.error.HTTPError as e:
        raw = e.read().decode() if e.fp else ""
        try:
            parsed = json.loads(raw)
        except:
            parsed = raw
        return e.code, parsed
    except Exception as e:
        return 0, str(e)


def remote_api(host, method, path, data=None):
    """Call API on a remote node via SSH."""
    if data:
        cmd = f'curl -s -w "\\n%{{http_code}}" -X {method} -H "Content-Type: application/json" -d \'{json.dumps(data)}\' "localhost:3998{path}"'
    else:
        cmd = f'curl -s -w "\\n%{{http_code}}" -X {method} "localhost:3998{path}"'
    
    result = subprocess.run(
        ["ssh", f"root@{host}", cmd],
        capture_output=True, text=True, timeout=20
    )
    lines = result.stdout.strip().rsplit('\n', 1)
    if len(lines) == 2:
        body_str, status_str = lines
        try:
            status = int(status_str)
        except:
            status = 0
            body_str = result.stdout.strip()
    else:
        status = 0
        body_str = result.stdout.strip()
    
    try:
        body = json.loads(body_str)
    except:
        body = body_str
    return status, body


def check(name, condition, detail=""):
    global PASS, FAIL, WARN
    if condition:
        PASS += 1
        tag = "✅"
    else:
        FAIL += 1
        tag = "❌"
    msg = f"{tag} {name}"
    if detail and not condition:
        msg += f"  -- {detail}"
    print(msg)
    RESULTS.append((tag, name, detail if not condition else ""))


def warn(name, detail=""):
    global WARN
    WARN += 1
    print(f"⚠️  {name}  -- {detail}")
    RESULTS.append(("⚠️", name, detail))


# ─────────────────────────────────────────────
# 1. SYSTEM / STATUS
# ─────────────────────────────────────────────
print("\n" + "="*60)
print("1. SYSTEM / STATUS")
print("="*60)

# GET /api/status
s, d = api("GET", "/api/status")
check("GET /api/status returns 200", s == 200)
if s == 200:
    for field in ["peer_id", "version", "peers", "overlay_ipv6", "started_at", "topics"]:
        check(f"  status has '{field}'", field in d, f"missing field: {field}")

# GET /api/heartbeat
s, d = api("GET", "/api/heartbeat")
check("GET /api/heartbeat returns 200", s == 200)

# GET /api/peers
s, d = api("GET", "/api/peers")
check("GET /api/peers returns 200", s == 200)
if s == 200 and isinstance(d, list):
    check("  peers list non-empty", len(d) > 0, f"got {len(d)} peers")

# GET /api/peers/geo
s, d = api("GET", "/api/peers/geo")
check("GET /api/peers/geo returns 200", s == 200)
if s == 200 and isinstance(d, list):
    has_self = any(p.get("is_self") for p in d)
    check("  peers/geo includes self", has_self)

# GET /api/traffic
s, d = api("GET", "/api/traffic")
check("GET /api/traffic returns 200", s == 200)

# GET /api/profile
s, d = api("GET", "/api/profile")
check("GET /api/profile returns 200", s == 200)

# PUT /api/profile
s, d = api("PUT", "/api/profile", {"bio": "test node cmax"})
check("PUT /api/profile returns 200", s == 200)

# PUT /api/motto
s, d = api("PUT", "/api/motto", {"motto": "testing clawnet"})
check("PUT /api/motto returns 200", s == 200)

# GET /api/diagnostics
s, d = api("GET", "/api/diagnostics")
check("GET /api/diagnostics returns 200", s == 200)
if s == 200 and isinstance(d, dict):
    for field in ["dht_routing_table", "nat_mode", "overlay_peers"]:
        check(f"  diagnostics has '{field}'", field in d, f"missing: {field}")

# GET / (root - expected 404)
s, d = api("GET", "/")
check("GET / returns 404 (no welcome page)", s == 404, f"got {s}")


# ─────────────────────────────────────────────
# 2. CREDITS
# ─────────────────────────────────────────────
print("\n" + "="*60)
print("2. CREDITS")
print("="*60)

s, d = api("GET", "/api/credits/balance")
check("GET /api/credits/balance returns 200", s == 200)
cmax_balance = 0
if s == 200:
    cmax_balance = d.get("balance", 0)
    for field in ["balance", "frozen", "prestige", "tier", "regen_rate"]:
        check(f"  balance has '{field}'", field in d, f"missing: {field}")
    print(f"    balance={cmax_balance}, frozen={d.get('frozen')}, tier={d.get('tier')}")

s, d = api("GET", "/api/credits/transactions")
check("GET /api/credits/transactions returns 200", s == 200)

s, d = api("GET", "/api/credits/audit")
check("GET /api/credits/audit returns 200", s == 200)


# ─────────────────────────────────────────────
# 3. TUTORIAL
# ─────────────────────────────────────────────
print("\n" + "="*60)
print("3. TUTORIAL")
print("="*60)

s, d = api("GET", "/api/tutorial/status")
check("GET /api/tutorial/status returns 200", s == 200)
if s == 200:
    print(f"    completed={d.get('completed')}, resume_ready={d.get('resume_ready')}")

# Try completing tutorial (may already be done)
s, d = api("POST", "/api/tutorial/complete")
if s == 200:
    check("POST /api/tutorial/complete returns 200", True)
elif s in [400, 409]:
    check("POST /api/tutorial/complete (already done or prereqs)", True, f"status={s}: {d}")
else:
    check("POST /api/tutorial/complete", False, f"status={s}, body={d}")


# ─────────────────────────────────────────────
# 4. RESUME / MATCHING
# ─────────────────────────────────────────────
print("\n" + "="*60)
print("4. RESUME / MATCHING")
print("="*60)

# GET own resume
s, d = api("GET", "/api/resume")
check("GET /api/resume returns 200", s == 200)

# PUT resume
s, d = api("PUT", "/api/resume", {
    "skills": ["go", "python", "p2p-networking", "distributed-systems"],
    "data_sources": ["arxiv", "github"],
    "description": "Test node cmax - specializes in P2P networking and distributed systems research"
})
check("PUT /api/resume returns 200", s == 200)

# GET own resume again
s, d = api("GET", "/api/resume")
check("GET /api/resume after update", s == 200)
if s == 200:
    check("  resume has skills", len(d.get("skills", [])) >= 3, f"skills={d.get('skills')}")

# GET all resumes
s, d = api("GET", "/api/resumes")
check("GET /api/resumes returns 200", s == 200)
if s == 200 and isinstance(d, list):
    print(f"    {len(d)} resumes known")

# Get bmax's actual peer_id by querying bmax directly
bmax_peer_id = None
dmax_peer_id = None
try:
    s_bmax, d_bmax = remote_api(BMAX, "GET", "/api/status")
    if s_bmax == 200 and isinstance(d_bmax, dict):
        bmax_peer_id = d_bmax.get("peer_id")
        print(f"    bmax peer_id: {bmax_peer_id}")
except:
    pass
try:
    s_dmax, d_dmax = remote_api(DMAX, "GET", "/api/status")
    if s_dmax == 200 and isinstance(d_dmax, dict):
        dmax_peer_id = d_dmax.get("peer_id")
        print(f"    dmax peer_id: {dmax_peer_id}")
except:
    pass

if bmax_peer_id:
    s, d = api("GET", f"/api/resume/{bmax_peer_id}")
    check(f"GET /api/resume/{{peer_id}} returns 200", s == 200, f"status={s}")
else:
    warn("GET /api/resume/{peer_id} skipped", "no remote peer_id found")


# ─────────────────────────────────────────────
# 5. KNOWLEDGE MESH
# ─────────────────────────────────────────────
print("\n" + "="*60)
print("5. KNOWLEDGE MESH")
print("="*60)

# POST knowledge
test_title = f"Test Knowledge {int(time.time())}"
s, d = api("POST", "/api/knowledge", {
    "title": test_title,
    "body": "This is a test knowledge entry for ClawNet functional testing. Contains information about P2P overlay networking.",
    "domain": "testing"
})
check("POST /api/knowledge returns 200", s == 200, f"status={s}, body={d}")
knowledge_id = None
if s == 200 and isinstance(d, dict):
    knowledge_id = d.get("id")
    print(f"    created knowledge id={knowledge_id}")

# GET knowledge feed
s, d = api("GET", "/api/knowledge/feed")
check("GET /api/knowledge/feed returns 200", s == 200)
if s == 200 and isinstance(d, list):
    print(f"    {len(d)} knowledge entries in feed")

# GET knowledge search
s, d = api("GET", "/api/knowledge/search?q=overlay")
check("GET /api/knowledge/search returns 200", s == 200)
if s == 200 and isinstance(d, list):
    print(f"    search 'overlay' returned {len(d)} results")

# POST knowledge react
if knowledge_id:
    s, d = api("POST", f"/api/knowledge/{knowledge_id}/react", {"reaction": "upvote"})
    check("POST /api/knowledge/{id}/react (upvote)", s == 200, f"status={s}, body={d}")

    # POST reply
    s, d = api("POST", f"/api/knowledge/{knowledge_id}/reply", {"body": "Great test entry!"})
    check("POST /api/knowledge/{id}/reply", s == 200, f"status={s}, body={d}")

    # GET replies
    s, d = api("GET", f"/api/knowledge/{knowledge_id}/replies")
    check("GET /api/knowledge/{id}/replies", s == 200, f"status={s}")
else:
    warn("Knowledge react/reply skipped", "no knowledge_id")

# Missing fields test
s, d = api("POST", "/api/knowledge", {"body": "no title"})
check("POST /api/knowledge without title → error", s >= 400, f"expected 4xx, got {s}")

s, d = api("POST", "/api/knowledge", {"title": "no body"})
check("POST /api/knowledge without body → error", s >= 400, f"expected 4xx, got {s}")


# ─────────────────────────────────────────────
# 6. TOPIC ROOMS
# ─────────────────────────────────────────────
print("\n" + "="*60)
print("6. TOPIC ROOMS")
print("="*60)

topic_name = f"test-room-{int(time.time()) % 10000}"

s, d = api("POST", "/api/topics", {"name": topic_name})
check("POST /api/topics (create room)", s == 200, f"status={s}, body={d}")

s, d = api("GET", "/api/topics")
check("GET /api/topics returns 200", s == 200)
if s == 200 and isinstance(d, list):
    print(f"    {len(d)} topic rooms")

s, d = api("POST", f"/api/topics/{topic_name}/join")
check(f"POST /api/topics/{{name}}/join", s == 200, f"status={s}")

s, d = api("POST", f"/api/topics/{topic_name}/messages", {"body": "Hello from cmax test!"})
check(f"POST /api/topics/{{name}}/messages", s == 200, f"status={s}, body={d}")

s, d = api("GET", f"/api/topics/{topic_name}/messages")
check(f"GET /api/topics/{{name}}/messages", s == 200, f"status={s}")
if s == 200 and isinstance(d, list):
    print(f"    {len(d)} messages in room")

s, d = api("POST", f"/api/topics/{topic_name}/leave")
check(f"POST /api/topics/{{name}}/leave", s == 200, f"status={s}")


# ─────────────────────────────────────────────
# 7. TASK BAZAAR - Full lifecycle (auction mode)
# bmax creates task, cmax bids/submits, bmax assigns/approves
# ─────────────────────────────────────────────
print("\n" + "="*60)
print("7. TASK BAZAAR (AUCTION MODE)")
print("="*60)

# List tasks
s, d = api("GET", "/api/tasks")
check("GET /api/tasks returns 200", s == 200)
if s == 200 and isinstance(d, list):
    print(f"    {len(d)} tasks listed")

# Get task board
s, d = api("GET", "/api/tasks/board")
check("GET /api/tasks/board returns 200", s == 200)

# bmax creates auction task
task_title = f"Test Task {int(time.time())}"
task_id = None
if bmax_peer_id:
    s, d = remote_api(BMAX, "POST", "/api/tasks", {
        "title": task_title,
        "description": "Functional test task for API validation. Please implement the requested feature.",
        "reward": 200,
        "mode": "auction",
        "tags": ["test", "api-validation"]
    })
    check("bmax POST /api/tasks (auction, reward=200)", s == 200, f"status={s}, body={d}")
    if s == 200 and isinstance(d, dict):
        task_id = d.get("id")
        print(f"    created task id={task_id}")

    # Wait for gossip to reach cmax
    if task_id:
        for attempt in range(8):
            time.sleep(2)
            s_c, d_c = api("GET", f"/api/tasks/{task_id}")
            if s_c == 200:
                break

        # Get single task
        s, d = api("GET", f"/api/tasks/{task_id}")
        check("GET /api/tasks/{id} returns 200", s == 200, f"status={s}")

        # Self-bid on bmax (author) should fail
        s, d = remote_api(BMAX, "POST", f"/api/tasks/{task_id}/bid", {"amount": 150})
        check("bmax self-bid → rejected", s >= 400, f"expected 4xx, got {s}: {d}")

        # cmax bids
        s, d = api("POST", f"/api/tasks/{task_id}/bid", {"amount": 180})
        check("cmax bids on bmax's task", s == 200, f"status={s}, body={d}")

        time.sleep(2)  # wait for gossip

        # Get bids on bmax
        s, d = remote_api(BMAX, "GET", f"/api/tasks/{task_id}/bids")
        check("GET /api/tasks/{id}/bids returns 200", s == 200, f"status={s}")
        if s == 200 and isinstance(d, list):
            print(f"    {len(d)} bids received on bmax")

        # bmax assigns to cmax
        cmax_peer_id = None
        s_st, d_st = api("GET", "/api/status")
        if s_st == 200:
            cmax_peer_id = d_st.get("peer_id")

        if cmax_peer_id:
            s, d = remote_api(BMAX, "POST", f"/api/tasks/{task_id}/assign", {"assign_to": cmax_peer_id})
            check("bmax assigns task to cmax", s == 200, f"status={s}, body={d}")

            # Wait for assign gossip to reach cmax
            for attempt in range(8):
                time.sleep(2)
                s_chk, d_chk = api("GET", f"/api/tasks/{task_id}")
                if s_chk == 200 and isinstance(d_chk, dict) and d_chk.get("status") == "assigned":
                    break

            # cmax submits work
            s, d = api("POST", f"/api/tasks/{task_id}/submit", {"result": "test work result from cmax"})
            check("cmax submits work", s == 200, f"status={s}, body={d}")

            # Wait for gossip propagation to bmax
            for attempt in range(10):
                time.sleep(2)
                s_check, d_check = remote_api(BMAX, "GET", f"/api/tasks/{task_id}")
                if s_check == 200 and isinstance(d_check, dict) and d_check.get("status") == "submitted":
                    break

            # bmax approves submission
            s, d = remote_api(BMAX, "POST", f"/api/tasks/{task_id}/approve")
            check("bmax approves task", s == 200, f"status={s}, body={d}")
        else:
            warn("Task assign/submit/approve skipped", "no cmax peer_id")
else:
    warn("Auction task flow skipped", "no bmax peer_id")

# Create task with too low reward
s, d = api("POST", "/api/tasks", {"title": "cheap task", "description": "test", "reward": 10})
check("POST /api/tasks reward=10 → rejected (min 100)", s >= 400, f"expected 4xx, got {s}: {d}")

# Create task missing title
s, d = api("POST", "/api/tasks", {"description": "no title", "reward": 200})
check("POST /api/tasks missing title → error", s >= 400, f"expected 4xx, got {s}: {d}")

# Cancel a task (create on bmax since cmax may be low on credits)
if bmax_peer_id:
    canc_s, canc_d = remote_api(BMAX, "POST", "/api/tasks", {
        "title": "Cancel Test Task",
        "description": "Will be cancelled",
        "reward": 100,
        "mode": "auction"
    })
    if canc_s == 200 and isinstance(canc_d, dict):
        canc_id = canc_d.get("id")
        s, d = remote_api(BMAX, "POST", f"/api/tasks/{canc_id}/cancel")
        check("POST /api/tasks/{id}/cancel", s == 200, f"status={s}, body={d}")
    else:
        warn("Cancel test skipped", f"could not create task: {canc_d}")
else:
    warn("Cancel test skipped", "no bmax peer_id")

# Task matching
s, d = api("GET", "/api/match/tasks")
check("GET /api/match/tasks returns 200", s == 200)
if task_id:
    s, d = api("GET", f"/api/tasks/{task_id}/match")
    check("GET /api/tasks/{id}/match returns 200", s == 200, f"status={s}")


# ─────────────────────────────────────────────
# 7b. SIMPLIFIED TASK FLOW (simple mode, cross-node)
# ─────────────────────────────────────────────
print("\n" + "="*60)
print("7b. SIMPLIFIED TASK FLOW")
print("="*60)

# bmax creates a simple-mode task (default); cmax claims it
if bmax_peer_id:
    # Create simple task on bmax
    simple_title = f"Simple Task {int(time.time())}"
    s_st, d_st = remote_api(BMAX, "POST", "/api/tasks", {
        "title": simple_title,
        "description": "Simplified flow test — claim with self-eval",
        "reward": 200
    })
    check("bmax POST /api/tasks (simple mode default)", s_st == 200, f"status={s_st}, body={d_st}")
    simple_id = d_st.get("id") if s_st == 200 and isinstance(d_st, dict) else None

    if simple_id:
        # Verify mode is "simple" in response
        s_g, d_g = remote_api(BMAX, "GET", f"/api/tasks/{simple_id}")
        check("  task mode defaults to 'simple'", s_g == 200 and isinstance(d_g, dict) and d_g.get("mode") == "simple",
              f"mode={d_g.get('mode') if isinstance(d_g, dict) else d_g}")

        # Wait for gossip to reach cmax
        for attempt in range(8):
            time.sleep(2)
            s_c, d_c = api("GET", f"/api/tasks/{simple_id}")
            if s_c == 200:
                break

        # --- Validation: low self_eval → rejected ---
        s_lo, d_lo = api("POST", f"/api/tasks/{simple_id}/claim", {
            "result": "low quality work", "self_eval_score": 0.3
        })
        check("  claim with self_eval=0.3 → 400", s_lo == 400, f"status={s_lo}, body={d_lo}")

        # --- Validation: self-claim → rejected ---
        s_self, d_self = remote_api(BMAX, "POST", f"/api/tasks/{simple_id}/claim", {
            "result": "self claim attempt", "self_eval_score": 0.9
        })
        check("  self-claim (author claims own) → 403", s_self == 403, f"status={s_self}, body={d_self}")

        # --- Validation: claim auction task → rejected ---
        # Create auction task on bmax for this test
        s_at, d_at = remote_api(BMAX, "POST", "/api/tasks", {
            "title": "Auction Only Task", "description": "test", "reward": 100, "mode": "auction"
        })
        auction_for_claim = d_at.get("id") if s_at == 200 and isinstance(d_at, dict) else None
        if auction_for_claim:
            time.sleep(3)  # gossip
            s_ac, d_ac = api("POST", f"/api/tasks/{auction_for_claim}/claim", {
                "result": "trying claim on auction", "self_eval_score": 0.8
            })
            check("  claim on auction-mode → 400", s_ac == 400, f"status={s_ac}, body={d_ac}")
            # Clean up: cancel the auction task
            remote_api(BMAX, "POST", f"/api/tasks/{auction_for_claim}/cancel")
        else:
            warn("  claim-on-auction test skipped", "could not create auction task")

        # --- Happy path: cmax claims bmax's simple task ---
        cmax_bal_before = 0
        s_b, d_b = api("GET", "/api/credits/balance")
        if s_b == 200:
            cmax_bal_before = d_b.get("balance", 0)
            print(f"    cmax balance before claim: {cmax_bal_before}")

        s_cl, d_cl = api("POST", f"/api/tasks/{simple_id}/claim", {
            "result": "Claimed work result from cmax — high quality delivery",
            "self_eval_score": 0.85
        })
        check("  cmax claims simple task → 200", s_cl == 200, f"status={s_cl}, body={d_cl}")

        # Wait for auto-approve via gossip on bmax (author node)
        approved = False
        for attempt in range(12):
            time.sleep(2)
            s_chk, d_chk = api("GET", f"/api/tasks/{simple_id}")
            if s_chk == 200 and isinstance(d_chk, dict) and d_chk.get("status") == "approved":
                approved = True
                break
        check("  task auto-approved via gossip", approved,
              f"status={d_chk.get('status') if isinstance(d_chk, dict) else d_chk}")

        if approved:
            # Verify self_eval_score preserved
            s_t, d_t = api("GET", f"/api/tasks/{simple_id}")
            if s_t == 200 and isinstance(d_t, dict):
                check("  self_eval_score preserved", d_t.get("self_eval_score", 0) >= 0.8,
                      f"self_eval_score={d_t.get('self_eval_score')}")

            # Verify cmax received credits (reward minus fee)
            s_b2, d_b2 = api("GET", "/api/credits/balance")
            if s_b2 == 200:
                cmax_bal_after = d_b2.get("balance", 0)
                gained = cmax_bal_after - cmax_bal_before
                print(f"    cmax balance after approve: {cmax_bal_after} (gained {gained})")
                check("  cmax received reward credits", gained > 0, f"gained={gained}")

        # --- Validation: re-claim already claimed → conflict ---
        s_re, d_re = remote_api(DMAX, "POST", f"/api/tasks/{simple_id}/claim", {
            "result": "late claim attempt", "self_eval_score": 0.9
        })
        check("  re-claim approved task → rejected", s_re >= 400, f"status={s_re}, body={d_re}")

    else:
        warn("  Simplified flow tests skipped", "could not create simple task on bmax")
else:
    warn("  Simplified flow tests skipped", "no bmax peer_id")


# ─────────────────────────────────────────────
# 8. DM (Direct Messages)
# ─────────────────────────────────────────────
print("\n" + "="*60)
print("8. DIRECT MESSAGES")
print("="*60)

# DM inbox
s, d = api("GET", "/api/dm/inbox")
check("GET /api/dm/inbox returns 200", s == 200)

# Send DM to bmax
if bmax_peer_id:
    s, d = api("POST", "/api/dm/send", {"peer_id": bmax_peer_id, "body": "Hello bmax from cmax test!"})
    check("POST /api/dm/send to bmax", s == 200, f"status={s}, body={d}")

    # Check thread
    s, d = api("GET", f"/api/dm/thread/{bmax_peer_id}")
    check(f"GET /api/dm/thread/{{peer_id}}", s == 200, f"status={s}")
    if s == 200 and isinstance(d, list):
        print(f"    {len(d)} messages in thread with bmax")

    # Check bmax received it
    time.sleep(3)
    s_inbox, d_inbox = remote_api(BMAX, "GET", "/api/dm/inbox")
    check("  bmax DM inbox accessible", s_inbox == 200, f"status={s_inbox}")
else:
    warn("DM tests skipped", "no bmax peer_id")

# Nonexistent endpoint test
s, d = api("POST", "/api/dm")
check("POST /api/dm → 404/405 (wrong endpoint)", s in [404, 405], f"got {s}")

# POST /api/chat/message → should be 404 (doesn't exist)
s, d = api("POST", "/api/chat/message")
check("POST /api/chat/message → 404 (not implemented)", s == 404, f"got {s}")


# ─────────────────────────────────────────────
# 9. CHAT MATCH
# ─────────────────────────────────────────────
print("\n" + "="*60)
print("9. CHAT MATCH")
print("="*60)

s, d = api("GET", "/api/chat/match")
check("GET /api/chat/match returns 200 or 503", s in [200, 503], f"got {s}")


# ─────────────────────────────────────────────
# 10. SWARM THINK
# ─────────────────────────────────────────────
print("\n" + "="*60)
print("10. SWARM THINK")
print("="*60)

# Templates
s, d = api("GET", "/api/swarm/templates")
check("GET /api/swarm/templates returns 200", s == 200)
if s == 200 and isinstance(d, list):
    print(f"    {len(d)} templates available")

# Create swarm
swarm_title = f"Test Swarm {int(time.time())}"
s, d = api("POST", "/api/swarm", {
    "title": swarm_title,
    "question": "What is the best approach to P2P relay optimization?",
    "duration_min": 30
})
check("POST /api/swarm (create)", s == 200, f"status={s}, body={d}")
swarm_id = None
if s == 200 and isinstance(d, dict):
    swarm_id = d.get("id")
    print(f"    created swarm id={swarm_id}")

# List swarms
s, d = api("GET", "/api/swarm")
check("GET /api/swarm returns 200", s == 200)
if s == 200 and isinstance(d, list):
    print(f"    {len(d)} swarms listed")

if swarm_id:
    # Get swarm detail
    s, d = api("GET", f"/api/swarm/{swarm_id}")
    check("GET /api/swarm/{id} returns 200", s == 200, f"status={s}")

    # Contribute
    s, d = api("POST", f"/api/swarm/{swarm_id}/contribute", {
        "body": "We should use bloom filter optimizations for path discovery.",
        "section": "approach",
        "confidence": 0.8
    })
    check("POST /api/swarm/{id}/contribute", s == 200, f"status={s}, body={d}")

    # Get contributions
    s, d = api("GET", f"/api/swarm/{swarm_id}/contributions")
    check("GET /api/swarm/{id}/contributions", s == 200, f"status={s}")

    # Synthesize
    s, d = api("POST", f"/api/swarm/{swarm_id}/synthesize", {
        "synthesis": "The swarm consensus is to use bloom filter optimizations with multi-hop relay."
    })
    check("POST /api/swarm/{id}/synthesize", s == 200, f"status={s}, body={d}")
else:
    warn("Swarm contribute/synthesize skipped", "no swarm_id")


# ─────────────────────────────────────────────
# 11. REPUTATION
# ─────────────────────────────────────────────
print("\n" + "="*60)
print("11. REPUTATION")
print("="*60)

s, d = api("GET", "/api/reputation")
check("GET /api/reputation returns 200", s == 200)
if s == 200 and isinstance(d, list):
    print(f"    {len(d)} entries in leaderboard")

# Get own reputation
s_status, d_status = api("GET", "/api/status")
if s_status == 200:
    own_peer_id = d_status.get("peer_id")
    if own_peer_id:
        s, d = api("GET", f"/api/reputation/{own_peer_id}")
        check("GET /api/reputation/{self}", s == 200, f"status={s}")


# ─────────────────────────────────────────────
# 12. PREDICTION MARKET
# ─────────────────────────────────────────────
print("\n" + "="*60)
print("12. PREDICTION MARKET")
print("="*60)

# Create prediction
pred_question = f"Will ClawNet reach 100 nodes by 2027? (test {int(time.time())})"
s, d = api("POST", "/api/predictions", {
    "question": pred_question,
    "options": ["Yes", "No", "Maybe"],
    "resolution_date": "2027-01-01T00:00:00Z",
    "category": "tech"
})
check("POST /api/predictions (create)", s == 200, f"status={s}, body={d}")
pred_id = None
if s == 200 and isinstance(d, dict):
    pred_id = d.get("id")
    print(f"    created prediction id={pred_id}")

# List predictions
s, d = api("GET", "/api/predictions")
check("GET /api/predictions returns 200", s == 200)
if s == 200 and isinstance(d, list):
    print(f"    {len(d)} predictions listed")

# Leaderboard
s, d = api("GET", "/api/predictions/leaderboard")
check("GET /api/predictions/leaderboard returns 200", s == 200)

if pred_id:
    # Get prediction
    s, d = api("GET", f"/api/predictions/{pred_id}")
    check("GET /api/predictions/{id}", s == 200, f"status={s}")

    # Place bet
    s, d = api("POST", f"/api/predictions/{pred_id}/bet", {"option": "Yes", "stake": 50})
    check("POST /api/predictions/{id}/bet", s == 200, f"status={s}, body={d}")

    # Place duplicate option bet? (test)
    s, d = api("POST", f"/api/predictions/{pred_id}/bet", {"option": "No", "stake": 30})
    check("POST /api/predictions/{id}/bet second option", s == 200, f"status={s}, body={d}")

    # Invalid stake
    s, d = api("POST", f"/api/predictions/{pred_id}/bet", {"option": "Yes", "stake": 0})
    check("POST /api/predictions/{id}/bet stake=0 → error", s >= 400, f"expected 4xx, got {s}")

    # Resolve
    s, d = api("POST", f"/api/predictions/{pred_id}/resolve", {"result": "Yes"})
    check("POST /api/predictions/{id}/resolve", s == 200, f"status={s}, body={d}")

    # Appeal
    s, d = api("POST", f"/api/predictions/{pred_id}/appeal", {"reason": "Test appeal"})
    # This may or may not succeed depending on state
    check("POST /api/predictions/{id}/appeal", s in [200, 400], f"status={s}, body={d}")

    # List appeals
    s, d = api("GET", f"/api/predictions/{pred_id}/appeals")
    check("GET /api/predictions/{id}/appeals", s == 200, f"status={s}")
else:
    warn("Prediction bet/resolve skipped", "no pred_id")

# Missing fields
s, d = api("POST", "/api/predictions", {"question": "Test?", "options": ["A"]})
check("POST /api/predictions with 1 option → error", s >= 400, f"expected 4xx, got {s}")


# ─────────────────────────────────────────────
# 13. OVERLAY / P2P
# ─────────────────────────────────────────────
print("\n" + "="*60)
print("13. OVERLAY / P2P")
print("="*60)

s, d = api("GET", "/api/overlay/status")
check("GET /api/overlay/status returns 200", s == 200)

s, d = api("GET", "/api/overlay/tree")
check("GET /api/overlay/tree returns 200", s == 200)

s, d = api("GET", "/api/overlay/peers")
check("GET /api/overlay/peers returns 200", s == 200)

s, d = api("GET", "/api/overlay/peers/geo")
check("GET /api/overlay/peers/geo returns 200", s == 200)

s, d = api("GET", "/api/overlay/molt/status")
check("GET /api/overlay/molt/status returns 200", s == 200)

s, d = api("GET", "/api/crypto/sessions")
check("GET /api/crypto/sessions returns 200", s == 200)

# Peer ping
if bmax_peer_id:
    s, d = api("GET", f"/api/peers/{bmax_peer_id}/ping")
    check(f"GET /api/peers/{{id}}/ping", s == 200, f"status={s}, body={d}")
    if s == 200 and isinstance(d, dict):
        rtt = d.get("rtt_ms", d.get("rtt"))
        print(f"    ping RTT to bmax: {rtt}")

# Peer profile lookup
if bmax_peer_id:
    s, d = api("GET", f"/api/peers/{bmax_peer_id}/profile")
    check(f"GET /api/peers/{{id}}/profile", s == 200, f"status={s}")


# ─────────────────────────────────────────────
# 14. WEALTH LEADERBOARD
# ─────────────────────────────────────────────
print("\n" + "="*60)
print("14. LEADERBOARD")
print("="*60)

s, d = api("GET", "/api/leaderboard")
check("GET /api/leaderboard returns 200", s == 200)
if s == 200 and isinstance(d, list):
    print(f"    {len(d)} entries in leaderboard")


# ─────────────────────────────────────────────
# 15. NONEXISTENT ENDPOINTS (confirm 404s)
# ─────────────────────────────────────────────
print("\n" + "="*60)
print("15. NONEXISTENT / WRONG ENDPOINTS")
print("="*60)

for method, path, desc in [
    ("POST", "/api/dm", "POST /api/dm"),
    ("POST", "/api/chat/message", "POST /api/chat/message"),
    ("POST", "/api/tasks/999/accept", "POST /api/tasks/{id}/accept"),
    ("POST", "/api/tasks/999/review", "POST /api/tasks/{id}/review"),
]:
    s, d = api(method, path)
    check(f"{desc} → 404 (not implemented)", s == 404, f"got {s}")


# ─────────────────────────────────────────────
# SUMMARY
# ─────────────────────────────────────────────
print("\n" + "="*60)
print("TEST SUMMARY")
print("="*60)
print(f"✅ PASS: {PASS}")
print(f"❌ FAIL: {FAIL}")
print(f"⚠️  WARN: {WARN}")
print(f"Total:  {PASS + FAIL + WARN}")

if FAIL > 0:
    print("\n--- FAILURES ---")
    for tag, name, detail in RESULTS:
        if tag == "❌":
            print(f"  {name}: {detail}")

if WARN > 0:
    print("\n--- WARNINGS ---")
    for tag, name, detail in RESULTS:
        if tag == "⚠️":
            print(f"  {name}: {detail}")

sys.exit(1 if FAIL > 0 else 0)
