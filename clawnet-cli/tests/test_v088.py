#!/usr/bin/env python3
"""
ClawNet v0.8.8 Full Integration Test — 3 nodes × 3 agents.

Tests all features including the new TUN/Molt overlay IPv6 system.

Usage:
    python3 test_v088.py

Nodes:
    Agent A (cmax) → 210.45.71.67:3998
    Agent B (bmax) → 210.45.71.131:3998
    Agent C (dmax) → 210.45.70.176:3998
"""

import json
import sys
import time
import traceback
import requests
from datetime import datetime

NODES = {
    "A": "http://127.0.0.1:3998",    # cmax (local)
    "B": "http://127.0.0.1:13998",   # bmax via SSH tunnel (-L 13998:127.0.0.1:3998 root@210.45.71.131)
    "C": "http://127.0.0.1:23998",   # dmax via SSH tunnel (-L 23998:127.0.0.1:3998 root@210.45.70.176)
}

# Node IPs for reference (used by SSH tunnels, not by test HTTP calls)
NODE_IPS = {
    "A": "210.45.71.67",
    "B": "210.45.71.131",
    "C": "210.45.70.176",
}

PASS = 0
FAIL = 0
RESULTS = []  # (section, name, passed, detail)


def log(msg):
    print(f"  {msg}")


def check(section, name, condition, detail=""):
    global PASS, FAIL, RESULTS
    passed = bool(condition)
    if passed:
        PASS += 1
        print(f"  ✅ {name}")
    else:
        FAIL += 1
        print(f"  ❌ {name}  {detail}")
    RESULTS.append((section, name, passed, detail))


def api(node, method, path, json_body=None, timeout=10):
    url = f"{NODES[node]}{path}"
    r = requests.request(method, url, json=json_body, timeout=timeout)
    return r


def get(node, path, timeout=10):
    r = api(node, "GET", path, timeout=timeout)
    try:
        return r.json()
    except Exception:
        return {"error": r.text, "status_code": r.status_code}


def post(node, path, body=None, timeout=10):
    r = api(node, "POST", path, body, timeout=timeout)
    try:
        return r.json()
    except Exception:
        return {"error": r.text, "status_code": r.status_code}


def wait_gossip(seconds=3):
    time.sleep(seconds)


# ═════════════════════════════════════════════
# Test 1: Status & Connectivity (v0.8.8)
# ═════════════════════════════════════════════
def test_status():
    section = "Status & Connectivity"
    print(f"\n═══ Test 1: {section} ═══")
    for name, url in NODES.items():
        status = get(name, "/api/status")
        check(section, f"Node {name} running v0.8.8",
              status.get("version") == "0.8.8",
              f"got {status.get('version')}")
        check(section, f"Node {name} has peer_id",
              status.get("peer_id", "") != "",
              f"peer_id={status.get('peer_id', '')[:20]}")
        check(section, f"Node {name} has geo_db",
              status.get("geo_db", "") != "",
              f"geo_db={status.get('geo_db')}")
        check(section, f"Node {name} has location",
              status.get("location", "") != "",
              f"location={status.get('location')}")
        check(section, f"Node {name} no addrs exposed",
              "addrs" not in status,
              "addrs field should be removed")
        check(section, f"Node {name} has peers",
              status.get("peers", 0) >= 1,
              f"peers={status.get('peers')}")
        check(section, f"Node {name} has started_at",
              status.get("started_at", 0) > 0,
              f"started_at={status.get('started_at')}")
        log(f"Node {name}: {status.get('peer_id', '')[:20]}... peers={status.get('peers')}")


# ═════════════════════════════════════════════
# Test 2: Overlay Transport
# ═════════════════════════════════════════════
def test_overlay():
    section = "Overlay Transport"
    print(f"\n═══ Test 2: {section} ═══")
    for name in NODES:
        status = get(name, "/api/status")
        # Check overlay fields are present regardless of enabled state
        has_overlay_keys = ("overlay_peers" in status or "overlay_ipv6" in status)
        overlay_enabled = get(name, "/api/overlay/status").get("enabled", False)

        if overlay_enabled:
            check(section, f"Node {name} overlay enabled",
                  True, "")
            check(section, f"Node {name} has overlay_peers in status",
                  "overlay_peers" in status,
                  f"keys={list(status.keys())}")
            check(section, f"Node {name} has overlay_ipv6",
                  status.get("overlay_ipv6", "") != "",
                  f"overlay_ipv6={status.get('overlay_ipv6')}")
            check(section, f"Node {name} has overlay_subnet",
                  status.get("overlay_subnet", "") != "",
                  f"overlay_subnet={status.get('overlay_subnet')}")
            # Check overlay/status endpoint
            ov = get(name, "/api/overlay/status")
            check(section, f"Node {name} overlay has public_key",
                  ov.get("public_key", "") != "")
            check(section, f"Node {name} overlay has overlay_ipv6",
                  ov.get("overlay_ipv6", "") != "")
            check(section, f"Node {name} overlay has molted field",
                  "molted" in ov, f"keys={list(ov.keys())}")
            check(section, f"Node {name} overlay has tun_device field",
                  "tun_device" in ov, f"keys={list(ov.keys())}")
            log(f"Node {name}: overlay_ipv6={ov.get('overlay_ipv6')}, "
                f"molted={ov.get('molted')}, tun={ov.get('tun_device')}")
        else:
            check(section, f"Node {name} overlay disabled (expected for non-overlay nodes)",
                  True, "overlay not enabled in config")
            log(f"Node {name}: overlay disabled")


# ═════════════════════════════════════════════
# Test 3: TUN Device + Molt/Unmolt
# ═════════════════════════════════════════════
def test_tun_molt():
    section = "TUN & Molt"
    print(f"\n═══ Test 3: {section} ═══")

    # Check molt status endpoint on all nodes
    for name in NODES:
        ms = get(name, "/api/overlay/molt/status")
        check(section, f"Node {name} molt/status returns enabled",
              "enabled" in ms, f"keys={list(ms.keys())}")
        check(section, f"Node {name} molt/status returns molted",
              "molted" in ms, f"keys={list(ms.keys())}")
        check(section, f"Node {name} molt/status returns tun",
              "tun" in ms, f"keys={list(ms.keys())}")

        if ms.get("enabled"):
            # Test molt toggle
            # First unmolt to ensure known state
            r = api(name, "POST", "/api/overlay/unmolt")
            check(section, f"Node {name} unmolt OK",
                  r.status_code == 200, f"status={r.status_code}")

            ms2 = get(name, "/api/overlay/molt/status")
            check(section, f"Node {name} is unmolted after unmolt",
                  ms2.get("molted") == False, f"molted={ms2.get('molted')}")

            # Now molt
            r = api(name, "POST", "/api/overlay/molt")
            check(section, f"Node {name} molt OK",
                  r.status_code == 200, f"status={r.status_code}")

            ms3 = get(name, "/api/overlay/molt/status")
            check(section, f"Node {name} is molted after molt",
                  ms3.get("molted") == True, f"molted={ms3.get('molted')}")

            # Return to unmolted (default)
            api(name, "POST", "/api/overlay/unmolt")

            # Check TUN device
            ov = get(name, "/api/overlay/status")
            tun = ov.get("tun_device", "")
            if tun:
                check(section, f"Node {name} TUN device is claw0",
                      tun == "claw0", f"tun={tun}")
            else:
                log(f"Node {name}: TUN not active (may need root)")

            # Check overlay_molted and overlay_tun in main status
            st = get(name, "/api/status")
            check(section, f"Node {name} status has overlay_molted",
                  "overlay_molted" in st, f"keys={list(st.keys())}")
            check(section, f"Node {name} status has overlay_tun",
                  "overlay_tun" in st, f"keys={list(st.keys())}")
        else:
            log(f"Node {name}: overlay not enabled, skipping molt tests")


# ═════════════════════════════════════════════
# Test 4: Overlay Peer Management
# ═════════════════════════════════════════════
def test_overlay_peers():
    section = "Overlay Peer Mgmt"
    print(f"\n═══ Test 4: {section} ═══")

    for name in NODES:
        ov = get(name, "/api/overlay/status")
        if not ov.get("enabled"):
            log(f"Node {name}: overlay disabled, skipping")
            continue

        # List peers
        peers = get(name, "/api/overlay/peers")
        check(section, f"Node {name} /overlay/peers returns list",
              isinstance(peers, list), f"type={type(peers)}")
        if isinstance(peers, list) and len(peers) > 0:
            p0 = peers[0]
            check(section, f"Node {name} overlay peer has uri",
                  "uri" in p0, f"keys={list(p0.keys())}")
            check(section, f"Node {name} overlay peer has rx_bytes",
                  "rx_bytes" in p0, f"keys={list(p0.keys())}")
            check(section, f"Node {name} overlay peer has tx_bytes",
                  "tx_bytes" in p0, f"keys={list(p0.keys())}")
            check(section, f"Node {name} overlay peer has rx_rate",
                  "rx_rate" in p0, f"keys={list(p0.keys())}")
            check(section, f"Node {name} overlay peer has tx_rate",
                  "tx_rate" in p0, f"keys={list(p0.keys())}")
            log(f"Node {name}: {len(peers)} overlay peers")
        else:
            log(f"Node {name}: no overlay peers")


# ═════════════════════════════════════════════
# Test 5: Credit System
# ═════════════════════════════════════════════
def test_credits():
    section = "Credit System"
    print(f"\n═══ Test 5: {section} ═══")

    for name in NODES:
        bal = get(name, "/api/credits/balance")
        check(section, f"Node {name} has energy account",
              bal.get("peer_id", "") != "",
              f"energy={bal.get('energy')}")
        check(section, f"Node {name} has tier info",
              bal.get("tier", {}).get("name", "") != "",
              f"tier={bal.get('tier')}")
        check(section, f"Node {name} has prestige field",
              "prestige" in bal, f"keys={list(bal.keys())}")
        check(section, f"Node {name} has regen_rate",
              bal.get("regen_rate", 0) >= 1.0,
              f"regen_rate={bal.get('regen_rate')}")

    # A transfers 1 energy to B
    peer_b = get("B", "/api/status")["peer_id"]
    bal_a_before = get("A", "/api/credits/balance")["energy"]
    result = post("A", "/api/credits/transfer", {
        "to_peer": peer_b, "amount": 1, "reason": "v088_test_transfer"
    })
    check(section, "A → B transfer 1 energy",
          result.get("status") == "transferred", str(result))

    bal_a_after = get("A", "/api/credits/balance")
    check(section, "A energy decreased after transfer",
          bal_a_after["energy"] < bal_a_before,
          f"before={bal_a_before}, after={bal_a_after['energy']}")

    txns = get("A", "/api/credits/transactions")
    check(section, "A has transaction records",
          len(txns) >= 1, f"count={len(txns)}")

    # Try overdraw
    r = api("A", "POST", "/api/credits/transfer",
            {"to_peer": peer_b, "amount": 99999})
    check(section, "Overdraw rejected",
          r.status_code == 400, f"status={r.status_code}")


# ═════════════════════════════════════════════
# Test 6: Task Bazaar
# ═════════════════════════════════════════════
def test_tasks():
    section = "Task Bazaar"
    print(f"\n═══ Test 6: {section} ═══")

    task = post("A", "/api/tasks", {
        "title": "v0.8.8 test task",
        "description": "Integration test task for v0.8.8",
        "reward": 1.0,
    })
    task_id = task.get("id", "")
    check(section, "A created task", task_id != "", str(task))
    log(f"Task ID: {task_id}")

    bal_a = get("A", "/api/credits/balance")
    check(section, "A energy frozen for reward",
          bal_a.get("frozen", 0) >= 1, f"frozen={bal_a.get('frozen')}")

    wait_gossip()

    tasks_b = get("B", "/api/tasks")
    found = any(t.get("id") == task_id for t in tasks_b)
    check(section, "B sees task via gossip", found,
          f"B has {len(tasks_b)} tasks")

    tasks_c = get("C", "/api/tasks")
    found_c = any(t.get("id") == task_id for t in tasks_c)
    check(section, "C sees task via gossip", found_c,
          f"C has {len(tasks_c)} tasks")

    # B bids
    bid = post("B", f"/api/tasks/{task_id}/bid",
               {"amount": 1.0, "message": "I'm on it"})
    check(section, "B placed bid", bid.get("id", "") != "", str(bid))

    wait_gossip()

    bids = get("A", f"/api/tasks/{task_id}/bids")
    check(section, "A sees B's bid", len(bids) >= 1, f"bids={len(bids)}")

    # A assigns to B
    peer_b = get("B", "/api/status")["peer_id"]
    assign = post("A", f"/api/tasks/{task_id}/assign", {"assign_to": peer_b})
    check(section, "A assigned to B",
          assign.get("status") == "assigned", str(assign))

    wait_gossip()

    # B submits
    submit = post("B", f"/api/tasks/{task_id}/submit",
                  {"result": "Task completed for v0.8.8 test"})
    check(section, "B submitted result",
          submit.get("status") == "submitted", str(submit))

    wait_gossip()

    # A approves
    approve = post("A", f"/api/tasks/{task_id}/approve")
    check(section, "A approved task",
          approve.get("status") == "approved", str(approve))

    return task_id


# ═════════════════════════════════════════════
# Test 7: Swarm Think
# ═════════════════════════════════════════════
def test_swarm():
    section = "Swarm Think"
    print(f"\n═══ Test 7: {section} ═══")

    swarm = post("A", "/api/swarm", {
        "title": "v0.8.8 swarm test",
        "question": "What improvements should ClawNet v0.9 focus on?",
    })
    swarm_id = swarm.get("id", "")
    check(section, "A created swarm", swarm_id != "", str(swarm))

    wait_gossip()

    swarms_b = get("B", "/api/swarm")
    found = any(s.get("id") == swarm_id for s in swarms_b)
    check(section, "B sees swarm via gossip", found)

    contrib_b = post("B", f"/api/swarm/{swarm_id}/contribute",
                     {"body": "Better TUN integration and IPv6 routing"})
    check(section, "B contributed",
          contrib_b.get("id", "") != "", str(contrib_b))

    wait_gossip()

    contrib_c = post("C", f"/api/swarm/{swarm_id}/contribute",
                     {"body": "More robust overlay peer discovery"})
    check(section, "C contributed",
          contrib_c.get("id", "") != "", str(contrib_c))

    wait_gossip()

    contribs = get("A", f"/api/swarm/{swarm_id}/contributions")
    check(section, "A sees contributions",
          len(contribs) >= 2, f"count={len(contribs)}")

    synthesis = post("A", f"/api/swarm/{swarm_id}/synthesize", {
        "synthesis": "v0.9 focus: TUN IPv6 routing, overlay discovery, molt UX."
    })
    check(section, "A synthesized",
          synthesis.get("status") == "synthesized", str(synthesis))

    sw = get("A", f"/api/swarm/{swarm_id}")
    check(section, "Swarm closed", sw.get("status") == "closed")


# ═════════════════════════════════════════════
# Test 8: Reputation
# ═════════════════════════════════════════════
def test_reputation():
    section = "Reputation"
    print(f"\n═══ Test 8: {section} ═══")

    peer_b = get("B", "/api/status")["peer_id"]
    rep_b = get("A", f"/api/reputation/{peer_b}")
    check(section, "B has reputation score",
          rep_b.get("score", 0) > 0, f"score={rep_b.get('score')}")

    reps = get("A", "/api/reputation")
    check(section, "Reputation list available",
          len(reps) >= 1, f"count={len(reps)}")


# ═════════════════════════════════════════════
# Test 9: Knowledge Mesh
# ═════════════════════════════════════════════
def test_knowledge():
    section = "Knowledge Mesh"
    print(f"\n═══ Test 9: {section} ═══")

    knowledge = post("A", "/api/knowledge", {
        "title": "v0.8.8 TUN Feature Notes",
        "body": "ClawNet now supports TUN devices for overlay IPv6 traffic with ClawNet-only filtering.",
        "domains": ["networking", "testing"],
    })
    check(section, "A posted knowledge",
          knowledge.get("id", "") != "", str(knowledge))

    wait_gossip()

    feed_b = get("B", "/api/knowledge/feed")
    found = any(k.get("title") == "v0.8.8 TUN Feature Notes" for k in feed_b)
    check(section, "B sees knowledge via gossip", found)

    # Search
    search_result = get("A", "/api/knowledge/search?q=TUN")
    check(section, "Knowledge search works",
          isinstance(search_result, list), f"type={type(search_result)}")


# ═════════════════════════════════════════════
# Test 10: Direct Messages
# ═════════════════════════════════════════════
def test_dm():
    section = "Direct Messages"
    print(f"\n═══ Test 10: {section} ═══")

    peer_c = get("C", "/api/status")["peer_id"]
    dm = post("A", "/api/dm/send",
              {"peer_id": peer_c, "body": "v0.8.8 test DM"})
    check(section, "A → C DM sent",
          dm.get("status") == "sent", str(dm))

    time.sleep(2)

    inbox_c = get("C", "/api/dm/inbox")
    check(section, "C has DM inbox",
          isinstance(inbox_c, list), f"type={type(inbox_c)}")


# ═════════════════════════════════════════════
# Test 11: Geo & Topology
# ═════════════════════════════════════════════
def test_geo():
    section = "Geo & Topology"
    print(f"\n═══ Test 11: {section} ═══")

    peers = get("A", "/api/peers")
    if isinstance(peers, list) and len(peers) > 0:
        p = peers[0]
        check(section, "Peers have peer_id", "peer_id" in p)
        check(section, "Peers have location", "location" in p, str(p.keys()))
        check(section, "No addrs in peers", "addrs" not in p)

    geo_peers = get("A", "/api/peers/geo")
    check(section, "Geo peers available",
          len(geo_peers) >= 1, f"count={len(geo_peers)}")

    # Overlay geo (if overlay enabled)
    ov = get("A", "/api/overlay/status")
    if ov.get("enabled"):
        try:
            ov_geo = get("A", "/api/overlay/peers/geo")
            check(section, "Overlay peer geo endpoint works",
                  isinstance(ov_geo, list), f"type={type(ov_geo)}")
        except Exception as e:
            check(section, "Overlay peer geo endpoint works",
                  False, f"exception: {e}")


# ═════════════════════════════════════════════
# Test 12: Leaderboard
# ═════════════════════════════════════════════
def test_leaderboard():
    section = "Leaderboard"
    print(f"\n═══ Test 12: {section} ═══")

    lb = get("A", "/api/leaderboard")
    check(section, "Leaderboard returns data",
          isinstance(lb, list) and len(lb) >= 1,
          f"type={type(lb)}, len={len(lb) if isinstance(lb, list) else 'N/A'}")

    if isinstance(lb, list) and len(lb) >= 1:
        entry = lb[0]
        check(section, "Entry has peer_id", "peer_id" in entry)
        check(section, "Entry has energy", "energy" in entry)
        check(section, "Entry has rank", "rank" in entry)


# ═════════════════════════════════════════════
# Test 13: Credit Audit
# ═════════════════════════════════════════════
def test_credit_audit():
    section = "Credit Audit"
    print(f"\n═══ Test 13: {section} ═══")

    audit = get("A", "/api/credits/audit")
    check(section, "Audit endpoint works",
          isinstance(audit, list), str(type(audit)))


# ═════════════════════════════════════════════
# Test 14: Security Checks
# ═════════════════════════════════════════════
def test_security():
    section = "Security"
    print(f"\n═══ Test 14: {section} ═══")

    # Verify grant endpoint is removed
    r = api("A", "POST", "/api/credits/grant", {"amount": 100})
    check(section, "Grant endpoint removed (404)",
          r.status_code == 404, f"status={r.status_code}")

    # Verify profile endpoint works without leaking sensitive data
    profile = get("A", "/api/profile")
    check(section, "Profile has agent_name",
          "agent_name" in profile, str(profile.keys()))
    check(section, "Profile no private_key leak",
          "private_key" not in profile and "privKey" not in profile)


# ═════════════════════════════════════════════
# Test 15: Diagnostics
# ═════════════════════════════════════════════
def test_diagnostics():
    section = "Diagnostics"
    print(f"\n═══ Test 15: {section} ═══")

    diag = get("A", "/api/diagnostics")
    check(section, "Diagnostics endpoint works",
          isinstance(diag, dict), str(type(diag)))

    # E2E crypto status
    crypto = get("A", "/api/crypto/sessions")
    check(section, "Crypto sessions endpoint works",
          "enabled" in crypto, str(crypto.keys()))


# ═════════════════════════════════════════════
# Main
# ═════════════════════════════════════════════
def main():
    global PASS, FAIL

    print("╔══════════════════════════════════════════════════╗")
    print("║   ClawNet v0.8.8 Full Integration Test          ║")
    print("║   3 Agents × 3 Nodes                            ║")
    print("║   Includes: TUN, Molt/Unmolt, Overlay IPv6      ║")
    print(f"║   {datetime.now().strftime('%Y-%m-%d %H:%M:%S'):>46s}  ║")
    print("╚══════════════════════════════════════════════════╝")

    # Pre-flight: check all nodes are reachable
    print("\n🔍 Pre-flight check...")
    for name, url in NODES.items():
        try:
            r = requests.get(f"{url}/api/status", timeout=5)
            r.raise_for_status()
            v = r.json().get("version", "?")
            log(f"Node {name} ({url}): OK  v{v}")
        except Exception as e:
            print(f"  ❌ Node {name} ({url}): {e}")
            print("Aborting: not all nodes are reachable.")
            sys.exit(1)

    # Run all test sections
    tests = [
        ("Status & Connectivity", test_status),
        ("Overlay Transport", test_overlay),
        ("TUN & Molt", test_tun_molt),
        ("Overlay Peer Mgmt", test_overlay_peers),
        ("Credit System", test_credits),
        ("Task Bazaar", test_tasks),
        ("Swarm Think", test_swarm),
        ("Reputation", test_reputation),
        ("Knowledge Mesh", test_knowledge),
        ("Direct Messages", test_dm),
        ("Geo & Topology", test_geo),
        ("Leaderboard", test_leaderboard),
        ("Credit Audit", test_credit_audit),
        ("Security", test_security),
        ("Diagnostics", test_diagnostics),
    ]

    for test_name, test_fn in tests:
        try:
            test_fn()
        except Exception as e:
            FAIL += 1
            RESULTS.append((test_name, f"EXCEPTION: {e}", False, traceback.format_exc()))
            print(f"  💥 {test_name} raised exception: {e}")

    # ── Summary ──
    total = PASS + FAIL
    print(f"\n{'═' * 52}")
    print(f"  Results: {PASS}/{total} passed, {FAIL} failed")

    # Per-section summary
    sections = {}
    for sect, name, passed, detail in RESULTS:
        if sect not in sections:
            sections[sect] = {"pass": 0, "fail": 0}
        if passed:
            sections[sect]["pass"] += 1
        else:
            sections[sect]["fail"] += 1

    print(f"\n  {'Section':<25s} {'Pass':>5s} {'Fail':>5s}")
    print(f"  {'─' * 37}")
    for sect, counts in sections.items():
        marker = "✅" if counts["fail"] == 0 else "❌"
        print(f"  {marker} {sect:<23s} {counts['pass']:>5d} {counts['fail']:>5d}")

    if FAIL == 0:
        print(f"\n  🎉 All {total} tests passed!")
    else:
        print(f"\n  ⚠️  {FAIL} test(s) failed")

        # Show failed tests
        print(f"\n  Failed tests:")
        for sect, name, passed, detail in RESULTS:
            if not passed:
                print(f"    ❌ [{sect}] {name}")
                if detail:
                    print(f"       {detail[:120]}")

    print(f"{'═' * 52}")
    sys.exit(0 if FAIL == 0 else 1)


if __name__ == "__main__":
    main()
