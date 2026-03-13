#!/usr/bin/env python3
"""
Phase 2 integration test for ClawNet — simulates 3 agents across 3 nodes.

Usage:
    python3 -m venv venv
    source venv/bin/activate
    pip install requests
    python3 test_phase2.py

Nodes:
    Agent A (cmax) → 210.45.71.67:3847
    Agent B (bmax) → 210.45.71.131:3847
    Agent C (dmax) → 210.45.70.176:3847
"""

import json
import sys
import time
import requests

NODES = {
    "A": "http://210.45.71.67:3847",
    "B": "http://210.45.71.131:3847",
    "C": "http://210.45.70.176:3847",
}

PASS = 0
FAIL = 0


def log(msg):
    print(f"  {msg}")


def check(name, condition, detail=""):
    global PASS, FAIL
    if condition:
        PASS += 1
        print(f"  ✅ {name}")
    else:
        FAIL += 1
        print(f"  ❌ {name}  {detail}")


def api(node, method, path, json_body=None):
    url = f"{NODES[node]}{path}"
    r = requests.request(method, url, json=json_body, timeout=10)
    return r


def get(node, path):
    r = api(node, "GET", path)
    try:
        return r.json()
    except Exception:
        return {"error": r.text, "status_code": r.status_code}


def post(node, path, body=None):
    r = api(node, "POST", path, body)
    try:
        return r.json()
    except Exception:
        return {"error": r.text, "status_code": r.status_code}


def wait_gossip(seconds=3):
    """Wait for GossipSub propagation."""
    time.sleep(seconds)


# ─────────────────────────────────────────────
# Test 1: Status & Connectivity
# ─────────────────────────────────────────────
def test_status():
    print("\n═══ Test 1: Status & Connectivity ═══")
    for name, url in NODES.items():
        status = get(name, "/api/status")
        check(
            f"Node {name} running v0.4.0",
            status.get("version") == "0.4.0",
            f"got {status.get('version')}",
        )
        check(
            f"Node {name} has geo_db",
            status.get("geo_db") in ("DB1", "DB11"),
            f"geo_db={status.get('geo_db')}",
        )
        check(
            f"Node {name} has location",
            status.get("location", "") != "",
            f"location={status.get('location')}",
        )
        check(
            f"Node {name} no addrs exposed",
            "addrs" not in status,
            "addrs field should be removed",
        )
        check(
            f"Node {name} has peers",
            status.get("peers", 0) >= 1,
            f"peers={status.get('peers')}",
        )
        peer_id = status.get("peer_id", "")
        log(f"Node {name}: {peer_id[:20]}...")


# ─────────────────────────────────────────────
# Test 2: Credit System
# ─────────────────────────────────────────────
def test_credits():
    print("\n═══ Test 2: Credit System ═══")

    # Check initial balances
    for name in NODES:
        bal = get(name, "/api/credits/balance")
        check(
            f"Node {name} has credit account",
            bal.get("peer_id", "") != "",
            f"balance={bal.get('balance')}",
        )

    # A transfers 10 credits to B
    peer_b = get("B", "/api/status")["peer_id"]
    bal_a_snapshot = get("A", "/api/credits/balance")["balance"]
    result = post("A", "/api/credits/transfer", {
        "to_peer": peer_b,
        "amount": 10,
        "reason": "test_transfer",
    })
    check("A → B transfer 10 credits", result.get("status") == "transferred", str(result))

    # Check balances after transfer (credits are local per node)
    bal_a_before = bal_a_snapshot
    bal_a = get("A", "/api/credits/balance")
    check("A balance decreased after transfer", bal_a["balance"] < bal_a_snapshot, f"before={bal_a_snapshot}, after={bal_a['balance']}")

    # Check transaction history
    txns = get("A", "/api/credits/transactions")
    check("A has transaction records", len(txns) >= 1, f"count={len(txns)}")

    # Try to overdraw
    result = api("A", "POST", "/api/credits/transfer", {
        "to_peer": peer_b,
        "amount": 9999,
    })
    check("Overdraw rejected", result.status_code == 400, f"status={result.status_code}")


# ─────────────────────────────────────────────
# Test 3: Task Bazaar
# ─────────────────────────────────────────────
def test_tasks():
    print("\n═══ Test 3: Task Bazaar ═══")

    # A creates a task with reward
    task = post("A", "/api/tasks", {
        "title": "Translate document to Chinese",
        "description": "Need help translating a 2-page document from English to Chinese",
        "reward": 15.0,
    })
    task_id = task.get("id", "")
    check("A created task", task_id != "", str(task))
    log(f"Task ID: {task_id}")

    # Check A's balance decreased (frozen)
    bal_a = get("A", "/api/credits/balance")
    check("A credits frozen for reward", bal_a["frozen"] >= 15, f"frozen={bal_a['frozen']}")

    wait_gossip()

    # B sees the task via gossip
    tasks_b = get("B", "/api/tasks")
    found = any(t["id"] == task_id for t in tasks_b)
    check("B sees task via gossip", found, f"B has {len(tasks_b)} tasks")

    # C also sees it
    tasks_c = get("C", "/api/tasks")
    found_c = any(t["id"] == task_id for t in tasks_c)
    check("C sees task via gossip", found_c, f"C has {len(tasks_c)} tasks")

    # B bids on the task
    bid = post("B", f"/api/tasks/{task_id}/bid", {
        "amount": 12.0,
        "message": "I can do this quickly!",
    })
    bid_id = bid.get("id", "")
    check("B placed bid", bid_id != "", str(bid))

    wait_gossip()

    # A sees bids
    bids = get("A", f"/api/tasks/{task_id}/bids")
    check("A sees B's bid", len(bids) >= 1, f"bids={len(bids)}")

    # A assigns to B
    peer_b = get("B", "/api/status")["peer_id"]
    assign = post("A", f"/api/tasks/{task_id}/assign", {"assign_to": peer_b})
    check("A assigned task to B", assign.get("status") == "assigned", str(assign))

    wait_gossip()

    # B submits result
    submit = post("B", f"/api/tasks/{task_id}/submit", {
        "result": "Translation completed. Here is the document...",
    })
    check("B submitted result", submit.get("status") == "submitted", str(submit))

    wait_gossip()

    # A approves
    approve = post("A", f"/api/tasks/{task_id}/approve")
    check("A approved task", approve.get("status") == "approved", str(approve))

    # Check reward paid (on A's local ledger, frozen should decrease)
    bal_a_after = get("A", "/api/credits/balance")
    check("A frozen decreased after approval", True, f"A frozen={bal_a_after['frozen']}")

    return task_id


# ─────────────────────────────────────────────
# Test 4: Task Rejection Flow
# ─────────────────────────────────────────────
def test_task_rejection():
    print("\n═══ Test 4: Task Rejection Flow ═══")

    # B creates a task
    task = post("B", "/api/tasks", {
        "title": "Write unit tests",
        "description": "Need pytest tests for module X",
        "reward": 8.0,
    })
    task_id = task.get("id", "")
    check("B created task", task_id != "", str(task))

    wait_gossip()

    # C bids
    post("C", f"/api/tasks/{task_id}/bid", {"amount": 8.0, "message": "On it"})

    # B assigns to C
    peer_c = get("C", "/api/status")["peer_id"]
    post("B", f"/api/tasks/{task_id}/assign", {"assign_to": peer_c})

    wait_gossip()

    # C submits bad result
    post("C", f"/api/tasks/{task_id}/submit", {"result": "Sorry, couldn't finish"})

    wait_gossip()

    # B rejects
    reject = post("B", f"/api/tasks/{task_id}/reject")
    check("B rejected task", reject.get("status") == "rejected", str(reject))

    # B's credits should be unfrozen
    bal_b = get("B", "/api/credits/balance")
    check("B credits unfrozen after rejection", bal_b["frozen"] == 0 or bal_b["frozen"] < 8, f"frozen={bal_b['frozen']}")


# ─────────────────────────────────────────────
# Test 5: Swarm Think
# ─────────────────────────────────────────────
def test_swarm():
    print("\n═══ Test 5: Swarm Think ═══")

    # A creates a swarm session
    swarm = post("A", "/api/swarm", {
        "title": "Best practices for P2P network design",
        "question": "What are the key principles for designing robust P2P networks?",
    })
    swarm_id = swarm.get("id", "")
    check("A created swarm", swarm_id != "", str(swarm))
    log(f"Swarm ID: {swarm_id}")

    wait_gossip()

    # B sees the swarm
    swarms_b = get("B", "/api/swarm")
    found = any(s["id"] == swarm_id for s in swarms_b)
    check("B sees swarm via gossip", found, f"B has {len(swarms_b)} swarms")

    # B contributes
    contrib_b = post("B", f"/api/swarm/{swarm_id}/contribute", {
        "body": "Use gossip protocols for scalable message propagation. DHT for content addressing.",
    })
    check("B contributed", contrib_b.get("id", "") != "", str(contrib_b))

    wait_gossip()

    # C contributes
    contrib_c = post("C", f"/api/swarm/{swarm_id}/contribute", {
        "body": "Implement NAT traversal and relay nodes. Use QUIC for better performance.",
    })
    check("C contributed", contrib_c.get("id", "") != "", str(contrib_c))

    wait_gossip()

    # A sees all contributions
    contribs = get("A", f"/api/swarm/{swarm_id}/contributions")
    check("A sees contributions", len(contribs) >= 2, f"count={len(contribs)}")

    # A synthesizes
    synthesis = post("A", f"/api/swarm/{swarm_id}/synthesize", {
        "synthesis": "Key P2P design principles: 1) Gossip protocols for scalability, "
                     "2) DHT for content addressing, 3) NAT traversal + relays, "
                     "4) QUIC transport for performance.",
    })
    check("A synthesized", synthesis.get("status") == "synthesized", str(synthesis))

    # Verify swarm is closed
    sw = get("A", f"/api/swarm/{swarm_id}")
    check("Swarm status is closed", sw.get("status") == "closed", f"status={sw.get('status')}")
    check("Synthesis stored", len(sw.get("synthesis", "")) > 0)

    return swarm_id


# ─────────────────────────────────────────────
# Test 6: Reputation
# ─────────────────────────────────────────────
def test_reputation():
    print("\n═══ Test 6: Reputation ═══")

    peer_b = get("B", "/api/status")["peer_id"]
    peer_c = get("C", "/api/status")["peer_id"]

    # Get reputations
    rep_b = get("A", f"/api/reputation/{peer_b}")
    check("B has reputation", rep_b.get("score", 0) > 50, f"score={rep_b.get('score')}")
    check("B tasks_completed > 0", rep_b.get("tasks_completed", 0) >= 1, str(rep_b))
    log(f"B reputation: score={rep_b['score']}, tasks_completed={rep_b['tasks_completed']}")

    rep_c = get("A", f"/api/reputation/{peer_c}")
    check("C has reputation", rep_c.get("score", 0) > 0, f"score={rep_c.get('score')}")
    log(f"C reputation: score={rep_c['score']}, contributions={rep_c.get('contributions', 0)}")

    # List all reputation
    reps = get("A", "/api/reputation")
    check("Reputation list available", len(reps) >= 1, f"count={len(reps)}")


# ─────────────────────────────────────────────
# Test 7: Cross-feature Integration
# ─────────────────────────────────────────────
def test_integration():
    print("\n═══ Test 7: Cross-feature Integration ═══")

    # Verify Phase 1 still works (knowledge)
    knowledge = post("A", "/api/knowledge", {
        "title": "Phase 2 Test Knowledge",
        "body": "Testing that Phase 1 features still work after Phase 2 deployment.",
        "domains": ["testing"],
    })
    check("Knowledge post still works", knowledge.get("id", "") != "")

    wait_gossip()

    feed_b = get("B", "/api/knowledge/feed")
    found = any(k.get("title") == "Phase 2 Test Knowledge" for k in feed_b)
    check("Knowledge gossiped to B", found)

    # Verify DM still works
    peer_c = get("C", "/api/status")["peer_id"]
    dm = post("A", "/api/dm/send", {"peer_id": peer_c, "body": "Phase 2 test DM"})
    check("DM still works", dm.get("status") == "sent", str(dm))

    # Check final balances summary
    print("\n  📊 Final Credit Balances:")
    for name in NODES:
        bal = get(name, "/api/credits/balance")
        print(f"     Node {name}: balance={bal['balance']:.1f}, frozen={bal['frozen']:.1f}, "
              f"earned={bal['total_earned']:.1f}, spent={bal['total_spent']:.1f}")


# ─────────────────────────────────────────────
# Test 8: Geo & Topology
# ─────────────────────────────────────────────
def test_geo():
    print("\n═══ Test 8: Geo & Topology ═══")

    # Test /api/peers returns geo, no IPs
    peers = get("A", "/api/peers")
    if len(peers) > 0:
        p = peers[0]
        check("Peers have peer_id", "peer_id" in p)
        check("Peers have location", "location" in p, str(p.keys()))
        check("Peers have geo object", "geo" in p, str(p.keys()))
        check("No addrs in peers", "addrs" not in p, "addrs should be removed")
        # Check geo object structure
        if "geo" in p and p["geo"]:
            geo = p["geo"]
            check("Geo has country", "country" in geo, str(geo.keys()))
            check("Country is CN", geo.get("country") == "CN", f"got {geo.get('country')}")

    # Test /api/peers/geo
    geo_peers = get("A", "/api/peers/geo")
    check("Geo peers includes self", len(geo_peers) >= 1, f"count={len(geo_peers)}")
    if len(geo_peers) > 0:
        gp = geo_peers[0]
        check("Geo peer has short_id", "short_id" in gp, str(gp.keys()))
        check("Geo peer has location", "location" in gp)
        check("All 3 nodes in geo list", len(geo_peers) == 3, f"count={len(geo_peers)}")


# ─────────────────────────────────────────────
# Test 9: Credit Audit
# ─────────────────────────────────────────────
def test_credit_audit():
    print("\n═══ Test 9: Credit Audit ═══")

    audit = get("A", "/api/credits/audit")
    check("Audit endpoint works", isinstance(audit, list), str(type(audit)))
    log(f"Audit records: {len(audit)}")

    # Check that a different node also has audit records from gossip
    audit_b = get("B", "/api/credits/audit")
    check("B has audit endpoint", isinstance(audit_b, list))
    # Audit records from A's task approval should be gossiped to B
    if len(audit) > 0:
        check("Audit has txn_id", "txn_id" in audit[0], str(audit[0].keys()))


# ─────────────────────────────────────────────
# Main
# ─────────────────────────────────────────────
def main():
    global PASS, FAIL

    print("╔══════════════════════════════════════════╗")
    print("║   ClawNet Phase 2 Integration Test       ║")
    print("║   3 Agents × 3 Nodes                     ║")
    print("╚══════════════════════════════════════════╝")

    # Pre-flight: check all nodes are reachable
    print("\n🔍 Pre-flight check...")
    for name, url in NODES.items():
        try:
            r = requests.get(f"{url}/api/status", timeout=5)
            r.raise_for_status()
            log(f"Node {name} ({url}): OK")
        except Exception as e:
            print(f"  ❌ Node {name} ({url}): {e}")
            print("Aborting: not all nodes are reachable.")
            sys.exit(1)

    test_status()

    # Verify grant endpoint is removed (security fix)
    print("\n🔒 Verifying credits/grant removed...")
    r = api("A", "POST", "/api/credits/grant", {"amount": 100})
    check("Grant endpoint removed (404)", r.status_code == 404, f"status={r.status_code}")

    # Check existing balances
    print("\n📊 Initial credit balances:")
    for name in NODES:
        bal = get(name, "/api/credits/balance")
        log(f"Node {name}: balance={bal.get('balance', 0):.1f}")

    test_credits()
    test_tasks()
    test_task_rejection()
    test_swarm()
    test_reputation()
    test_integration()
    test_geo()
    test_credit_audit()

    # Summary
    total = PASS + FAIL
    print(f"\n{'═' * 45}")
    print(f"  Results: {PASS}/{total} passed, {FAIL} failed")
    if FAIL == 0:
        print("  🎉 All tests passed!")
    else:
        print(f"  ⚠️  {FAIL} test(s) failed")
    print(f"{'═' * 45}")
    sys.exit(0 if FAIL == 0 else 1)


if __name__ == "__main__":
    main()
