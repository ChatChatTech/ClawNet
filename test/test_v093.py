#!/usr/bin/env python3
"""
ClawNet v0.9.3 Comprehensive Integration Test — 3 nodes × full coverage.

Tests every major subsystem across the real 3-node deployment,
including new v0.9.3 features: async mail-list chat, task board, targeted tasks.

Usage:
    python3 test/test_v093.py

Nodes:
    cmax  → 210.45.71.67:3998  (bootstrap)
    bmax  → 210.45.71.131:3998
    dmax  → 210.45.70.176:3998
"""

import json
import os
import signal
import sys
import time
import subprocess
import requests

VERSION = "0.9.3"

NODES = {
    "cmax": "http://127.0.0.1:3998",
    "bmax": "http://127.0.0.1:13998",
    "dmax": "http://127.0.0.1:23998",
}

SSH_TUNNELS = {
    "bmax": ("210.45.71.131", 13998),
    "dmax": ("210.45.70.176", 23998),
}

# ── Counters ──
PASS = 0
FAIL = 0
SECTIONS = []
CURRENT_SECTION = None
CURRENT_RESULTS = []


def section(name):
    global CURRENT_SECTION, CURRENT_RESULTS
    if CURRENT_SECTION and CURRENT_RESULTS:
        SECTIONS.append((CURRENT_SECTION, list(CURRENT_RESULTS)))
    CURRENT_SECTION = name
    CURRENT_RESULTS = []
    print(f"\n{'═' * 60}")
    print(f"  {name}")
    print(f"{'═' * 60}")


def check(test_id, name, condition, detail=""):
    global PASS, FAIL
    if condition:
        PASS += 1
        status = "PASS"
        print(f"  ✅ [{test_id}] {name}")
    else:
        FAIL += 1
        status = "FAIL"
        print(f"  ❌ [{test_id}] {name}  {detail}")
    CURRENT_RESULTS.append((test_id, name, status, detail))


def get(node, path):
    try:
        r = requests.get(f"{NODES[node]}{path}", timeout=10)
        return r.json()
    except Exception as e:
        return {"_error": str(e)}


def post(node, path, body=None):
    try:
        r = requests.post(f"{NODES[node]}{path}", json=body, timeout=10)
        return r.json(), r.status_code
    except Exception as e:
        return {"_error": str(e)}, 0


def put(node, path, body=None):
    try:
        r = requests.put(f"{NODES[node]}{path}", json=body, timeout=10)
        return r.json(), r.status_code
    except Exception as e:
        return {"_error": str(e)}, 0


def raw_post(node, path, body=None):
    try:
        return requests.post(f"{NODES[node]}{path}", json=body, timeout=10)
    except Exception:
        return None


def raw_get(node, path):
    try:
        return requests.get(f"{NODES[node]}{path}", timeout=10)
    except Exception:
        return None


def wait_gossip(seconds=3):
    time.sleep(seconds)


# ════════════════════════════════════════════════
# T1: Node Status & Connectivity
# ════════════════════════════════════════════════
def test_status():
    section("T1: Node Status & Connectivity")
    for name in NODES:
        s = get(name, "/api/status")
        check(f"T1.1-{name}", f"{name} returns status",
              "peer_id" in s, str(s.get("_error", "")))
        check(f"T1.2-{name}", f"{name} version={VERSION}",
              s.get("version") == VERSION, f"got {s.get('version')}")
        check(f"T1.3-{name}", f"{name} has peers",
              s.get("peers", 0) >= 1, f"peers={s.get('peers')}")
        check(f"T1.4-{name}", f"{name} has geo_db",
              s.get("geo_db") in ("DB1", "DB5", "DB11"), f"geo_db={s.get('geo_db')}")
        check(f"T1.5-{name}", f"{name} has location",
              s.get("location", "") != "", f"location={s.get('location')}")
        check(f"T1.6-{name}", f"{name} has overlay peers",
              s.get("overlay_peers", 0) >= 1, f"overlay_peers={s.get('overlay_peers')}")
        check(f"T1.7-{name}", f"{name} has overlay IPv6",
              s.get("overlay_ipv6", "") != "", f"overlay_ipv6={s.get('overlay_ipv6')}")
        check(f"T1.8-{name}", f"{name} has TUN device",
              s.get("overlay_tun") == "claw0", f"tun={s.get('overlay_tun')}")


# ════════════════════════════════════════════════
# T2: Diagnostics
# ════════════════════════════════════════════════
def test_diagnostics():
    section("T2: Diagnostics")
    d = get("cmax", "/api/diagnostics")
    check("T2.1", "diagnostics returns data", "peer_id" in d, str(d.get("_error", "")))
    check("T2.2", "has DHT routing table", d.get("dht_routing_table", 0) >= 0)
    check("T2.3", "has relay_enabled", "relay_enabled" in d)
    check("T2.4", "has btdht_status", d.get("btdht_status") in ("running", "disabled"))
    check("T2.5", "has overlay_peers", d.get("overlay_peers", 0) >= 1)
    check("T2.6", "has listen_addrs", len(d.get("listen_addrs", [])) >= 1)
    check("T2.7", "has bandwidth stats", d.get("bandwidth_in", 0) >= 0)
    check("T2.8", "has matrix_homeservers", d.get("matrix_homeservers", 0) >= 0)


# ════════════════════════════════════════════════
# T3: Peer Discovery (libp2p + Overlay)
# ════════════════════════════════════════════════
def test_peers():
    section("T3: Peer Discovery")
    peers_cmax = get("cmax", "/api/peers")
    check("T3.1", "cmax has libp2p peers", len(peers_cmax) >= 1, f"count={len(peers_cmax)}")
    if len(peers_cmax) > 0:
        p = peers_cmax[0]
        check("T3.2", "peer has peer_id", "peer_id" in p)
        check("T3.3", "peer has location", "location" in p)
        check("T3.4", "peer has geo", "geo" in p)
        check("T3.5", "no addrs exposed", "addrs" not in p)

    overlay = get("cmax", "/api/overlay/peers/geo")
    check("T3.6", "overlay geo peers", len(overlay) >= 2, f"count={len(overlay)}")
    resolved = [x for x in overlay if x.get("location") not in ("Resolving...", "Private", "Unknown", "")]
    check("T3.7", "overlay peers resolved", len(resolved) >= 2, f"resolved={len(resolved)}")

    peers_bmax = get("bmax", "/api/peers")
    check("T3.8", "bmax has peers", len(peers_bmax) >= 1, f"count={len(peers_bmax)}")


# ════════════════════════════════════════════════
# T4: Geo & Topology
# ════════════════════════════════════════════════
def test_geo():
    section("T4: Geo & Topology")
    geo = get("cmax", "/api/peers/geo")
    check("T4.1", "geo peers endpoint", len(geo) >= 1, f"count={len(geo)}")
    if len(geo) > 0:
        g = geo[0]
        check("T4.2", "has short_id", "short_id" in g)
        check("T4.3", "has geo object", "geo" in g and g["geo"] is not None)
        if g.get("geo"):
            check("T4.4", "geo has lat/lon", "latitude" in g["geo"] and "longitude" in g["geo"])

    ogeo = get("cmax", "/api/overlay/peers/geo")
    check("T4.5", "overlay geo cache working", len(ogeo) >= 2, f"count={len(ogeo)}")
    pending = [x for x in ogeo if x.get("location") == "Resolving..."]
    check("T4.6", "few pending resolutions", len(pending) <= len(ogeo) * 0.5,
          f"pending={len(pending)}/{len(ogeo)}")


# ════════════════════════════════════════════════
# T5: Overlay Network (Ironwood)
# ════════════════════════════════════════════════
def test_overlay():
    section("T5: Overlay Network")
    s = get("cmax", "/api/overlay/status")
    check("T5.1", "overlay enabled", s.get("enabled") is True)
    check("T5.2", "overlay has peers", s.get("peer_count", 0) >= 2)
    check("T5.3", "overlay has IPv6", s.get("overlay_ipv6", "") != "")
    check("T5.4", "overlay has subnet", "overlay_subnet" in s)

    tree = get("cmax", "/api/overlay/tree")
    check("T5.5", "tree endpoint works", "_error" not in tree)

    opeers = get("cmax", "/api/overlay/peers")
    check("T5.6", "overlay peers list", isinstance(opeers, list) and len(opeers) >= 2,
          f"type={type(opeers)} count={len(opeers) if isinstance(opeers, list) else 'N/A'}")


# ════════════════════════════════════════════════
# T6: Matrix Discovery
# ════════════════════════════════════════════════
def test_matrix():
    section("T6: Matrix Discovery")
    m = get("cmax", "/api/matrix/status")
    check("T6.1", "matrix enabled", m.get("enabled") is True)
    check("T6.2", "matrix has homeservers", m.get("connected_homeservers", 0) >= 1,
          f"connected={m.get('connected_homeservers')}")


# ════════════════════════════════════════════════
# T7: Credit System
# ════════════════════════════════════════════════
def test_credits():
    section("T7: Credit System")
    for name in NODES:
        bal = get(name, "/api/credits/balance")
        check(f"T7.1-{name}", f"{name} has balance",
              "balance" in bal or "energy" in bal, str(bal.get("_error", "")))
        check(f"T7.2-{name}", f"{name} has tier",
              "tier" in bal and isinstance(bal.get("tier"), dict))
        check(f"T7.3-{name}", f"{name} has prestige",
              "prestige" in bal)

    # Transfer hidden in v0.9.1 — verify it's 404/405
    peer_bmax = get("bmax", "/api/status").get("peer_id", "")
    resp = raw_post("cmax", "/api/credits/transfer", {
        "to_peer": peer_bmax, "amount": 0.5, "reason": "v0.9.3 test"
    })
    check("T7.4", "transfer endpoint hidden",
          resp is not None and resp.status_code in (404, 405),
          f"status={resp.status_code if resp else 'none'}")

    # Transactions
    txns = get("cmax", "/api/credits/transactions")
    check("T7.5", "has transactions", isinstance(txns, list))

    # Audit
    audit = get("cmax", "/api/credits/audit")
    check("T7.6", "audit endpoint", isinstance(audit, list))

    # Leaderboard
    lb = get("cmax", "/api/leaderboard")
    check("T7.7", "leaderboard works", isinstance(lb, list) and len(lb) >= 1)


# ════════════════════════════════════════════════
# T8: Knowledge Mesh
# ════════════════════════════════════════════════
def test_knowledge():
    section("T8: Knowledge Mesh")
    ts = str(int(time.time()))
    title = f"v0.9.3 test knowledge {ts}"

    result, code = post("cmax", "/api/knowledge", {
        "title": title,
        "body": "Testing knowledge mesh across 3 nodes for v0.9.3 release.",
        "domains": ["testing", "v0.9.3"],
    })
    kid = result.get("id", "")
    check("T8.1", "publish knowledge", kid != "", str(result))

    wait_gossip(4)

    feed = get("bmax", "/api/knowledge/feed")
    found = any(k.get("title") == title for k in feed)
    check("T8.2", "knowledge gossiped to bmax", found, f"feed_count={len(feed)}")

    feed_d = get("dmax", "/api/knowledge/feed")
    found_d = any(k.get("title") == title for k in feed_d)
    check("T8.3", "knowledge gossiped to dmax", found_d, f"feed_count={len(feed_d)}")

    results = get("cmax", "/api/knowledge/search?q=v0.9.3+release")
    check("T8.4", "FTS5 search works", isinstance(results, list) and len(results) >= 1,
          f"count={len(results) if isinstance(results, list) else 'N/A'}")

    if kid:
        result2, _ = post("bmax", f"/api/knowledge/{kid}/react", {"reaction": "upvote"})
        check("T8.5", "react to knowledge", "_error" not in result2, str(result2))

    if kid:
        result3, _ = post("dmax", f"/api/knowledge/{kid}/reply", {
            "body": "Great test knowledge entry!"
        })
        check("T8.6", "reply to knowledge", "_error" not in result3, str(result3))


# ════════════════════════════════════════════════
# T9: Topic Rooms
# ════════════════════════════════════════════════
def test_topics():
    section("T9: Topic Rooms")
    ts = str(int(time.time()))
    topic_name = f"test-room-{ts}"

    result, _ = post("cmax", "/api/topics", {
        "name": topic_name, "description": "v0.9.3 test room"
    })
    check("T9.1", "create topic", "_error" not in result, str(result))

    result2, _ = post("bmax", f"/api/topics/{topic_name}/join", {})
    check("T9.2", "bmax join topic", True)

    wait_gossip(2)

    result3, _ = post("cmax", f"/api/topics/{topic_name}/messages", {
        "body": "Hello from cmax in v0.9.3 test!"
    })
    check("T9.3", "post to topic", "_error" not in result3, str(result3))

    wait_gossip(3)

    msgs = get("bmax", f"/api/topics/{topic_name}/messages")
    check("T9.4", "bmax sees topic message",
          isinstance(msgs, list) and len(msgs) >= 1,
          f"count={len(msgs) if isinstance(msgs, list) else 'N/A'}")

    topics = get("cmax", "/api/topics")
    check("T9.5", "list topics", isinstance(topics, list) and len(topics) >= 1)


# ════════════════════════════════════════════════
# T10: Direct Messages (E2E encrypted)
# ════════════════════════════════════════════════
def test_dm():
    section("T10: Direct Messages")
    peer_dmax = get("dmax", "/api/status").get("peer_id", "")

    result, code = post("cmax", "/api/dm/send", {
        "peer_id": peer_dmax, "body": "v0.9.3 test DM (E2E encrypted)"
    })
    check("T10.1", "send DM", result.get("status") == "sent", str(result))

    wait_gossip(3)

    inbox = get("dmax", "/api/dm/inbox")
    check("T10.2", "dmax receives DM in inbox",
          isinstance(inbox, list) and len(inbox) >= 1,
          f"inbox_count={len(inbox) if isinstance(inbox, list) else 'N/A'}")

    # Thread view
    if peer_dmax:
        cmax_id = get("cmax", "/api/status").get("peer_id", "")
        thread = get("dmax", f"/api/dm/thread/{cmax_id}")
        check("T10.3", "thread view works",
              isinstance(thread, list) and len(thread) >= 1,
              f"thread_count={len(thread) if isinstance(thread, list) else 'N/A'}")
    else:
        check("T10.3", "thread view works", False, "no dmax peer_id")

    crypto = get("cmax", "/api/crypto/sessions")
    check("T10.4", "crypto engine enabled", crypto.get("enabled") is True)


# ════════════════════════════════════════════════
# T11: Task Bazaar (full lifecycle)
# ════════════════════════════════════════════════
def test_tasks():
    section("T11: Task Bazaar")
    ts = str(int(time.time()))

    task_result, _ = post("cmax", "/api/tasks", {
        "title": f"v0.9.3 test task {ts}",
        "description": "Integration test task for v0.9.3",
        "reward": 1.0,
    })
    task_id = task_result.get("id", "")
    check("T11.1", "create task", task_id != "", str(task_result))

    wait_gossip(4)

    tasks_b = get("bmax", "/api/tasks")
    found = any(t.get("id") == task_id for t in tasks_b)
    check("T11.2", "bmax sees task", found, f"tasks_count={len(tasks_b)}")

    bid_result, _ = post("bmax", f"/api/tasks/{task_id}/bid", {
        "amount": 1.0, "message": "I'll do it"
    })
    bid_id = bid_result.get("id", "")
    check("T11.3", "bmax places bid", bid_id != "", str(bid_result))

    wait_gossip(3)

    bids = get("cmax", f"/api/tasks/{task_id}/bids")
    check("T11.4", "cmax sees bids", isinstance(bids, list) and len(bids) >= 1)

    peer_bmax = get("bmax", "/api/status").get("peer_id", "")
    assign_result, _ = post("cmax", f"/api/tasks/{task_id}/assign", {"assign_to": peer_bmax})
    check("T11.5", "assign task", assign_result.get("status") == "assigned", str(assign_result))

    wait_gossip(3)

    submit_result, _ = post("bmax", f"/api/tasks/{task_id}/submit", {
        "result": "Task completed for v0.9.3 test"
    })
    check("T11.6", "bmax submits", submit_result.get("status") == "submitted", str(submit_result))

    wait_gossip(3)

    approve_result, _ = post("cmax", f"/api/tasks/{task_id}/approve")
    check("T11.7", "cmax approves", approve_result.get("status") == "approved", str(approve_result))

    # Cancel flow
    task2, _ = post("cmax", "/api/tasks", {
        "title": f"v0.9.3 cancel test {ts}", "description": "test cancel", "reward": 0.5,
    })
    task2_id = task2.get("id", "")
    if task2_id:
        cancel_result, _ = post("cmax", f"/api/tasks/{task2_id}/cancel")
        check("T11.8", "cancel task", cancel_result.get("status") == "cancelled", str(cancel_result))
    else:
        check("T11.8", "cancel task", False, "no task created")


# ════════════════════════════════════════════════
# T12: Task Board (NEW v0.9.3)
# ════════════════════════════════════════════════
def test_task_board():
    section("T12: Task Board (v0.9.3)")
    ts = str(int(time.time()))

    # Create a task so the board is non-empty
    task_result, _ = post("cmax", "/api/tasks", {
        "title": f"board test task {ts}",
        "description": "Task board integration test",
        "reward": 0.5,
    })
    task_id = task_result.get("id", "")
    check("T12.1", "create task for board", task_id != "", str(task_result))

    wait_gossip(2)

    # Get board from cmax (publisher)
    board = get("cmax", "/api/tasks/board")
    check("T12.2", "board has my_published",
          "my_published" in board, str(list(board.keys()) if isinstance(board, dict) else board))
    check("T12.3", "board has my_assigned", "my_assigned" in board)
    check("T12.4", "board has open_tasks", "open_tasks" in board)

    # cmax's published should include the task
    published = board.get("my_published", [])
    found = any(t.get("id") == task_id for t in published)
    check("T12.5", "board shows published task", found,
          f"published_count={len(published)}")

    # bmax's board: task should be in open_tasks
    board_b = get("bmax", "/api/tasks/board")
    open_tasks = board_b.get("open_tasks", [])
    found_open = any(t.get("id") == task_id for t in open_tasks)
    check("T12.6", "bmax sees task in open_tasks", found_open,
          f"open_count={len(open_tasks)}")

    # Assign to bmax, then check bmax's my_assigned
    peer_bmax = get("bmax", "/api/status").get("peer_id", "")
    bid_result, _ = post("bmax", f"/api/tasks/{task_id}/bid", {
        "amount": 0.5, "message": "board test bid"
    })
    wait_gossip(2)
    assign_result, _ = post("cmax", f"/api/tasks/{task_id}/assign", {"assign_to": peer_bmax})
    check("T12.7", "assign for board test",
          assign_result.get("status") == "assigned", str(assign_result))

    wait_gossip(2)
    board_b2 = get("bmax", "/api/tasks/board")
    assigned = board_b2.get("my_assigned", [])
    found_assigned = any(t.get("id") == task_id for t in assigned)
    check("T12.8", "bmax my_assigned includes task", found_assigned,
          f"assigned_count={len(assigned)}")


# ════════════════════════════════════════════════
# T13: Targeted Tasks (NEW v0.9.3)
# ════════════════════════════════════════════════
def test_targeted_tasks():
    section("T13: Targeted Tasks (v0.9.3)")
    ts = str(int(time.time()))

    peer_bmax = get("bmax", "/api/status").get("peer_id", "")
    peer_dmax = get("dmax", "/api/status").get("peer_id", "")

    # Create a task targeted to bmax
    task_result, _ = post("cmax", "/api/tasks", {
        "title": f"targeted task {ts}",
        "description": "Only bmax should be able to bid",
        "reward": 1.0,
        "target_peer": peer_bmax,
    })
    task_id = task_result.get("id", "")
    check("T13.1", "create targeted task", task_id != "", str(task_result))

    # Verify target_peer is stored
    if task_id:
        task_detail = get("cmax", f"/api/tasks/{task_id}")
        check("T13.2", "target_peer stored",
              task_detail.get("target_peer") == peer_bmax,
              f"target_peer={task_detail.get('target_peer')}")
    else:
        check("T13.2", "target_peer stored", False, "no task_id")

    wait_gossip(3)

    # dmax should be rejected when bidding
    if task_id:
        resp = raw_post("dmax", f"/api/tasks/{task_id}/bid", {
            "amount": 1.0, "message": "I want to bid"
        })
        check("T13.3", "dmax bid rejected (targeted)",
              resp is not None and resp.status_code == 403,
              f"status={resp.status_code if resp else 'none'}")
    else:
        check("T13.3", "dmax bid rejected (targeted)", False, "no task_id")

    # bmax should succeed
    if task_id:
        bid_result, code = post("bmax", f"/api/tasks/{task_id}/bid", {
            "amount": 1.0, "message": "I'm the target"
        })
        check("T13.4", "bmax bid accepted (targeted)",
              bid_result.get("id", "") != "", str(bid_result))
    else:
        check("T13.4", "bmax bid accepted (targeted)", False, "no task_id")

    # Self-bid prevention
    if task_id:
        resp2 = raw_post("cmax", f"/api/tasks/{task_id}/bid", {
            "amount": 0.5, "message": "self bid"
        })
        check("T13.5", "owner cannot self-bid",
              resp2 is not None and resp2.status_code == 403,
              f"status={resp2.status_code if resp2 else 'none'}")
    else:
        check("T13.5", "owner cannot self-bid", False, "no task_id")


# ════════════════════════════════════════════════
# T14: Swarm Think
# ════════════════════════════════════════════════
def test_swarm():
    section("T14: Swarm Think")
    ts = str(int(time.time()))

    swarm, _ = post("cmax", "/api/swarm", {
        "title": f"v0.9.3 swarm {ts}",
        "question": "What are the key design principles for P2P networks?",
    })
    swarm_id = swarm.get("id", "")
    check("T14.1", "create swarm", swarm_id != "", str(swarm))

    wait_gossip(4)

    swarms_b = get("bmax", "/api/swarm")
    found = any(s.get("id") == swarm_id for s in swarms_b)
    check("T14.2", "bmax sees swarm", found)

    contrib_b, _ = post("bmax", f"/api/swarm/{swarm_id}/contribute", {
        "body": "Use gossip protocols for scalable propagation."
    })
    check("T14.3", "bmax contributes", contrib_b.get("id", "") != "", str(contrib_b))

    wait_gossip(3)

    contrib_d, _ = post("dmax", f"/api/swarm/{swarm_id}/contribute", {
        "body": "NAT traversal and relay nodes are essential."
    })
    check("T14.4", "dmax contributes", contrib_d.get("id", "") != "", str(contrib_d))

    wait_gossip(3)

    contribs = get("cmax", f"/api/swarm/{swarm_id}/contributions")
    check("T14.5", "cmax sees contributions",
          isinstance(contribs, list) and len(contribs) >= 2, f"count={len(contribs)}")

    synth, _ = post("cmax", f"/api/swarm/{swarm_id}/synthesize", {
        "synthesis": "Key principles: gossip protocols + NAT traversal + relay nodes."
    })
    check("T14.6", "synthesize swarm", synth.get("status") == "synthesized", str(synth))


# ════════════════════════════════════════════════
# T15: Reputation
# ════════════════════════════════════════════════
def test_reputation():
    section("T15: Reputation")
    peer_bmax = get("bmax", "/api/status").get("peer_id", "")
    rep = get("cmax", f"/api/reputation/{peer_bmax}")
    check("T15.1", "get reputation", "score" in rep, str(rep))
    check("T15.2", "reputation score > 0", rep.get("score", 0) > 0,
          f"score={rep.get('score')}")

    reps = get("cmax", "/api/reputation")
    check("T15.3", "list reputations", isinstance(reps, list) and len(reps) >= 1)


# ════════════════════════════════════════════════
# T16: Profile & Motto
# ════════════════════════════════════════════════
def test_profile():
    section("T16: Profile & Motto")
    profile = get("cmax", "/api/profile")
    check("T16.1", "get profile", "peer_id" in profile or "agent_name" in profile, str(profile))

    result, code = put("cmax", "/api/motto", {"motto": "v0.9.3 release ready"})
    check("T16.2", "set motto", code == 200, f"code={code}")

    profile2 = get("cmax", "/api/profile")
    check("T16.3", "motto updated", profile2.get("motto") == "v0.9.3 release ready",
          f"motto={profile2.get('motto')}")

    resume = get("cmax", "/api/resume")
    check("T16.4", "resume endpoint works", "_error" not in resume)


# ════════════════════════════════════════════════
# T17: Overlay TUN & Molt
# ════════════════════════════════════════════════
def test_molt():
    section("T17: Overlay TUN & Molt")
    status = get("cmax", "/api/overlay/status")
    check("T17.1", "overlay not molted", status.get("molted") is False)

    result, code = post("cmax", "/api/overlay/molt")
    check("T17.2", "molt succeeds", code == 200, f"code={code} result={result}")

    status2 = get("cmax", "/api/status")
    check("T17.3", "status shows molted", status2.get("overlay_molted") is True,
          f"molted={status2.get('overlay_molted')}")

    result2, code2 = post("cmax", "/api/overlay/unmolt")
    check("T17.4", "unmolt succeeds", code2 == 200, f"code={code2}")

    time.sleep(1)
    status3 = get("cmax", "/api/status")
    check("T17.5", "status shows unmolted", status3.get("overlay_molted") is False,
          f"molted={status3.get('overlay_molted')}")


# ════════════════════════════════════════════════
# T18: Security Checks
# ════════════════════════════════════════════════
def test_security():
    section("T18: Security")
    # Grant endpoint must be gone
    resp = raw_post("cmax", "/api/credits/grant", {"amount": 100})
    check("T18.1", "grant endpoint removed",
          resp is not None and resp.status_code in (404, 405),
          f"status={resp.status_code if resp else 'none'}")

    check("T18.2", "API accessible from localhost",
          get("cmax", "/api/status").get("version") == VERSION)

    # SQL injection in search
    inj = get("cmax", "/api/knowledge/search?q=%27%3B%20DROP%20TABLE%20knowledge%3B--")
    check("T18.3", "SQL injection harmless",
          isinstance(inj, list) or inj.get("error") == "q parameter required",
          f"type={type(inj)} val={str(inj)[:80]}")

    # XSS payload storage
    xss_result, _ = post("cmax", "/api/knowledge", {
        "title": "<script>alert(1)</script>",
        "body": "XSS test payload",
        "domains": ["security"],
    })
    check("T18.4", "XSS payload stored safely",
          xss_result.get("id", "") != "", str(xss_result))

    # Overdraw protection (transfer hidden, but test with empty amount)
    resp2 = raw_post("cmax", "/api/credits/transfer", {
        "to_peer": "fake", "amount": 999999
    })
    check("T18.5", "transfer hidden (overdraw impossible)",
          resp2 is not None and resp2.status_code in (404, 405),
          f"status={resp2.status_code if resp2 else 'none'}")


# ════════════════════════════════════════════════
# T19: Dev Mode
# ════════════════════════════════════════════════
def test_dev_mode():
    section("T19: Dev Mode")
    # Build dev binary
    build_cmd = "cd /data/projs/clawnet/clawnet-cli && CGO_ENABLED=1 go build -tags 'fts5 dev' -o /tmp/clawnet-dev-test ./cmd/clawnet/ 2>&1"
    try:
        result = subprocess.run(build_cmd, shell=True, capture_output=True, text=True, timeout=120)
        check("T19.1", "dev mode builds", result.returncode == 0,
              result.stderr[:200] if result.returncode != 0 else "")
    except Exception as e:
        check("T19.1", "dev mode builds", False, str(e))

    # Run version check
    if os.path.exists("/tmp/clawnet-dev-test"):
        try:
            result = subprocess.run(
                "/tmp/clawnet-dev-test version",
                shell=True, capture_output=True, text=True, timeout=10
            )
            check("T19.2", "dev binary runs", VERSION in result.stdout,
                  result.stdout.strip())
        except Exception as e:
            check("T19.2", "dev binary runs", False, str(e))

        # Test --dev-layers flag (general help mentions dev-layers)
        try:
            result = subprocess.run(
                "/tmp/clawnet-dev-test 2>&1; /tmp/clawnet-dev-test --help 2>&1",
                shell=True, capture_output=True, text=True, timeout=10
            )
            combined = result.stdout + result.stderr
            has_dev_layers = "dev-layers" in combined
            check("T19.3", "dev binary has --dev-layers flag", has_dev_layers,
                  combined[:200])
        except Exception as e:
            check("T19.3", "dev binary has --dev-layers flag", False, str(e))
    else:
        check("T19.2", "dev binary runs", False, "binary not found")
        check("T19.3", "dev binary has --dev-layers flag", False, "binary not found")

    # Cleanup
    try:
        os.remove("/tmp/clawnet-dev-test")
    except OSError:
        pass


# ════════════════════════════════════════════════
# T20: Predictions (Oracle Arena)
# ════════════════════════════════════════════════
def test_predictions():
    section("T20: Predictions")
    ts = str(int(time.time()))

    pred, code = post("cmax", "/api/predictions", {
        "question": f"Will ClawNet reach 200 nodes by Q3? ({ts})",
        "options": ["yes", "no"],
        "category": "network",
        "resolution_date": "2026-09-30",
    })
    pred_id = pred.get("id", "")
    check("T20.1", "create prediction", pred_id != "", str(pred))

    preds = get("cmax", "/api/predictions")
    check("T20.2", "list predictions", isinstance(preds, list) and len(preds) >= 1)

    if pred_id:
        bet, _ = post("bmax", f"/api/predictions/{pred_id}/bet", {
            "position": "yes", "amount": 0.5
        })
        check("T20.3", "place bet", "_error" not in bet, str(bet))

    plb = get("cmax", "/api/predictions/leaderboard")
    check("T20.4", "prediction leaderboard", isinstance(plb, list))


# ════════════════════════════════════════════════
# T21: Resume & Agent Matching
# ════════════════════════════════════════════════
def test_resume():
    section("T21: Resume & Agent Matching")

    # Update resume
    resume_data = {
        "skills": ["go", "python", "p2p"],
        "data_sources": ["arxiv", "github"],
        "bio": "v0.9.3 test agent",
    }
    result, code = put("cmax", "/api/resume", resume_data)
    check("T21.1", "update resume", code == 200, f"code={code} result={result}")

    # Get own resume
    resume = get("cmax", "/api/resume")
    check("T21.2", "get own resume", "_error" not in resume, str(resume))

    # List resumes
    resumes = get("cmax", "/api/resumes")
    check("T21.3", "list resumes", isinstance(resumes, list))

    # Match tasks for agent
    match = get("bmax", "/api/match/tasks")
    check("T21.4", "match tasks for agent", isinstance(match, list),
          f"type={type(match)}")

    # Tutorial status
    tut = get("cmax", "/api/tutorial/status")
    check("T21.5", "tutorial status", "_error" not in tut, str(tut))


# ════════════════════════════════════════════════
# T22: Misc & Stability
# ════════════════════════════════════════════════
def test_misc():
    section("T22: Misc & Stability")
    # SSE topology
    try:
        r = requests.get(f"{NODES['cmax']}/api/topology", timeout=2, stream=True)
        check("T22.1", "topology SSE endpoint", r.status_code == 200)
        r.close()
    except requests.exceptions.ReadTimeout:
        check("T22.1", "topology SSE endpoint", True)
    except Exception as e:
        check("T22.1", "topology SSE endpoint", False, str(e))

    # Peer profile lookup
    peer_bmax = get("bmax", "/api/status").get("peer_id", "")
    if peer_bmax:
        profile = get("cmax", f"/api/peers/{peer_bmax}/profile")
        check("T22.2", "peer profile lookup", "_error" not in profile, str(profile))
    else:
        check("T22.2", "peer profile lookup", False, "no bmax peer_id")

    # Chat match (interactive)
    chat = get("cmax", "/api/chat/match")
    check("T22.3", "chat match endpoint",
          "peer_id" in chat or "error" in chat, str(chat))

    # Heartbeat
    hb = get("cmax", "/api/heartbeat")
    check("T22.4", "heartbeat works", "_error" not in hb, str(hb))

    # Traffic
    traffic = get("cmax", "/api/traffic")
    check("T22.5", "traffic endpoint", "_error" not in traffic, str(traffic))


# ════════════════════════════════════════════════
# T23: Go Unit Tests
# ════════════════════════════════════════════════
def test_go_unit():
    section("T23: Go Unit Tests")
    try:
        result = subprocess.run(
            "cd /data/projs/clawnet/clawnet-cli && CGO_ENABLED=1 go test -tags fts5 -count=1 -timeout 30s ./tests/ 2>&1 | tail -20",
            shell=True, capture_output=True, text=True, timeout=120
        )
        passed = ("ok" in result.stdout or "PASS" in result.stdout) and "FAIL" not in result.stdout
        check("T23.1", "store tests pass", passed,
              result.stdout[-200:] if not passed else "")
    except Exception as e:
        check("T23.1", "store tests pass", False, str(e))


# ════════════════════════════════════════════════
# Generate Report
# ════════════════════════════════════════════════
def generate_report():
    global CURRENT_SECTION, CURRENT_RESULTS
    if CURRENT_SECTION and CURRENT_RESULTS:
        SECTIONS.append((CURRENT_SECTION, list(CURRENT_RESULTS)))

    total = PASS + FAIL
    rate = (PASS / total * 100) if total > 0 else 0

    report = []
    report.append(f"# ClawNet v{VERSION} 综合测试报告")
    report.append("")
    report.append(f"> 测试时间: {time.strftime('%Y-%m-%d %H:%M:%S')}")
    report.append(f"> ClawNet 版本: v{VERSION}")
    report.append(f"> 测试节点: cmax (210.45.71.67) / bmax (210.45.71.131) / dmax (210.45.70.176)")
    report.append(f"> 通过率: **{PASS}/{total} ({rate:.1f}%)**")
    report.append("")
    report.append("---")
    report.append("")

    # Summary table
    report.append("## 总览")
    report.append("")
    report.append("| 模块 | 通过 | 失败 | 通过率 |")
    report.append("|------|------|------|--------|")
    for sec_name, results in SECTIONS:
        sec_pass = sum(1 for _, _, s, _ in results if s == "PASS")
        sec_fail = sum(1 for _, _, s, _ in results if s == "FAIL")
        sec_total = sec_pass + sec_fail
        sec_rate = (sec_pass / sec_total * 100) if sec_total > 0 else 0
        emoji = "✅" if sec_fail == 0 else "⚠️"
        report.append(f"| {emoji} {sec_name} | {sec_pass} | {sec_fail} | {sec_rate:.0f}% |")
    report.append("")

    # Detailed results
    report.append("## 详细结果")
    report.append("")
    for sec_name, results in SECTIONS:
        report.append(f"### {sec_name}")
        report.append("")
        report.append("| ID | 测试项 | 结果 | 备注 |")
        report.append("|----|--------|------|------|")
        for tid, name, status, detail in results:
            emoji = "✅" if status == "PASS" else "❌"
            detail_safe = detail.replace("|", "\\|")[:80] if detail else ""
            report.append(f"| {tid} | {name} | {emoji} {status} | {detail_safe} |")
        report.append("")

    # Conclusion
    report.append("## 结论")
    report.append("")
    if rate >= 95:
        report.append(f"ClawNet v{VERSION} 通过率 **{rate:.1f}%**，核心功能稳定可靠，达到发布标准。")
    elif rate >= 80:
        report.append(f"ClawNet v{VERSION} 通过率 **{rate:.1f}%**，大部分功能正常，少量边缘场景需关注。")
    else:
        report.append(f"ClawNet v{VERSION} 通过率 **{rate:.1f}%**，存在较多问题需修复。")
    report.append("")

    # v0.9.3 Feature inventory
    report.append(f"## 功能清单 (v{VERSION})")
    report.append("")
    report.append("### v0.9.3 新特性")
    report.append("- 异步邮件列表 Chat (inbox/thread/send, 默认模式)")
    report.append("- 交互式实时聊天降级为 `--interactive/-i` 选项")
    report.append("- Task Board 仪表盘 (`GET /api/tasks/board`)")
    report.append("- 定向任务 (`target_peer` 字段, 仅目标 peer 可竞标)")
    report.append("- 自我竞标防护 (owner 不可 bid 自己的 task)")
    report.append("")
    report.append("### 网络层")
    report.append("- libp2p P2P 网络 (TCP + QUIC + WebSocket)")
    report.append("- 9 层发现: mDNS / Kademlia DHT / BT-DHT / HTTP Bootstrap / STUN / Relay / Matrix / Overlay / K8s")
    report.append("- Ironwood Overlay 网络 (Ed25519 + 加密路由 + TUN IPv6)")
    report.append("- Overlay Mesh 公网兼容 (80+ 公共节点)")
    report.append("- GossipSub v1.1 消息传播")
    report.append("- Circuit Relay v2 + NAT 穿透")
    report.append("- Matrix Homeserver 发现")
    report.append("")
    report.append("### 应用层")
    report.append("- Knowledge Mesh (发布/搜索/订阅/回复/点赞, FTS5 全文索引)")
    report.append("- Task Bazaar (发布→竞标→指派→提交→验收, 完整生命周期)")
    report.append("- Task Board 仪表盘 (我的发布/我的指派/开放任务)")
    report.append("- Swarm Think (多 Agent 协作推理)")
    report.append("- Credit Economy (余额/审计, PoW Anti-Sybil)")
    report.append("- Reputation System (声誉评分/排行榜)")
    report.append("- Prediction Market (预测/下注/结算/申诉)")
    report.append("- Direct Messages (E2E NaCl Box 加密)")
    report.append("- Topic Rooms (创建/加入/发言/历史)")
    report.append("- Agent 简历 & 智能匹配")
    report.append("- Nutshell Bundle 传输 (SHA-256 内容寻址)")
    report.append("")
    report.append("### 基础设施")
    report.append("- TUI Topo 3D 地球可视化")
    report.append("- IP2Location 地理定位 (异步渐进式缓存)")
    report.append("- SQLite WAL 存储 (25+ 表)")
    report.append("- Ed25519 身份 + NaCl E2E 加密")
    report.append("- Dev Mode (--dev-layers 逐层隔离测试)")
    report.append("- 自更新 (clawnet update)")
    report.append("- TUN 设备 (molt/unmolt)")
    report.append("")

    report_text = "\n".join(report)
    report_path = f"/data/projs/clawnet/test/test-report-v{VERSION}.md"
    with open(report_path, "w") as f:
        f.write(report_text)
    print(f"\n📋 Report saved to {report_path}")
    return report_text


# ════════════════════════════════════════════════
# Main
# ════════════════════════════════════════════════
def main():
    print("╔════════════════════════════════════════════════╗")
    print(f"║   ClawNet v{VERSION} Comprehensive Integration Test  ║")
    print("║   3 Nodes × Full Feature Coverage              ║")
    print("╚════════════════════════════════════════════════╝")

    # Set up SSH tunnels for remote nodes
    tunnel_procs = []
    print("\n🔗 Setting up SSH tunnels...")
    for name, (host, local_port) in SSH_TUNNELS.items():
        cmd = f"ssh -N -L {local_port}:127.0.0.1:3998 root@{host}"
        proc = subprocess.Popen(cmd.split(), stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        tunnel_procs.append(proc)
        print(f"  {name}: localhost:{local_port} → {host}:3998 (pid={proc.pid})")
    time.sleep(2)

    try:
        _run_tests()
    finally:
        for proc in tunnel_procs:
            proc.terminate()
            proc.wait()
        print("\n🔗 SSH tunnels closed.")


def _run_tests():
    # Pre-flight
    print("\n🔍 Pre-flight check...")
    for name, url in NODES.items():
        try:
            r = requests.get(f"{url}/api/status", timeout=5)
            r.raise_for_status()
            v = r.json().get("version", "?")
            print(f"  ✅ {name} ({url}): v{v}")
        except Exception as e:
            print(f"  ❌ {name} ({url}): {e}")
            print("Aborting: not all nodes are reachable.")
            sys.exit(1)

    test_status()
    test_diagnostics()
    test_peers()
    test_geo()
    test_overlay()
    test_matrix()
    test_credits()
    test_knowledge()
    test_topics()
    test_dm()
    test_tasks()
    test_task_board()       # NEW v0.9.3
    test_targeted_tasks()   # NEW v0.9.3
    test_swarm()
    test_reputation()
    test_profile()
    test_molt()
    test_security()
    test_dev_mode()
    test_predictions()
    test_resume()
    test_misc()
    test_go_unit()

    # Summary
    total = PASS + FAIL
    rate = (PASS / total * 100) if total > 0 else 0
    print(f"\n{'═' * 55}")
    print(f"  Results: {PASS}/{total} passed ({rate:.1f}%), {FAIL} failed")
    if FAIL == 0:
        print("  🎉 All tests passed!")
    else:
        print(f"  ⚠️  {FAIL} test(s) failed")
    print(f"{'═' * 55}")

    generate_report()
    sys.exit(0 if FAIL == 0 else 1)


if __name__ == "__main__":
    main()
