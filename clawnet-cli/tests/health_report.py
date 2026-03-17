#!/usr/bin/env python3
"""ClawNet Node Health Reporter — queries local API and prints a Markdown report."""

import argparse
import json
import sys
import urllib.request
import urllib.error
from datetime import datetime, timezone


def api_get(host, path):
    """GET an API endpoint, return parsed JSON or None on error."""
    try:
        req = urllib.request.Request(f"{host}{path}")
        resp = urllib.request.urlopen(req, timeout=10)
        return json.loads(resp.read().decode())
    except (urllib.error.URLError, json.JSONDecodeError, OSError) as e:
        print(f"ERROR: failed to query {path}: {e}", file=sys.stderr)
        return None


def fmt_uptime(started_at):
    """Convert started_at (unix timestamp or ISO string) to human-readable uptime."""
    try:
        if isinstance(started_at, (int, float)):
            start = datetime.fromtimestamp(started_at, tz=timezone.utc)
        else:
            start = datetime.fromisoformat(str(started_at).replace("Z", "+00:00"))
        delta = datetime.now(timezone.utc) - start
        days = delta.days
        hours, rem = divmod(delta.seconds, 3600)
        mins, _ = divmod(rem, 60)
        parts = []
        if days:
            parts.append(f"{days}d")
        if hours:
            parts.append(f"{hours}h")
        parts.append(f"{mins}m")
        return " ".join(parts)
    except (ValueError, TypeError):
        return "unknown"


def main():
    parser = argparse.ArgumentParser(description="ClawNet Node Health Reporter")
    parser.add_argument("--host", default="http://127.0.0.1:3998", help="ClawNet daemon address")
    args = parser.parse_args()

    host = args.host.rstrip("/")

    # 1. Status
    status = api_get(host, "/api/status")
    if status is None:
        print("ERROR: cannot reach ClawNet daemon", file=sys.stderr)
        sys.exit(1)

    # 2. Credits
    credits = api_get(host, "/api/credits/balance")

    # 3. Peers
    peers = api_get(host, "/api/peers")

    # 4. Tasks
    tasks = api_get(host, "/api/tasks")

    # 5. Knowledge
    knowledge = api_get(host, "/api/knowledge/feed")

    # Build report
    peer_id = status.get("peer_id", "unknown")
    peer_short = peer_id[:12] + "..." if len(peer_id) > 12 else peer_id
    overlay = status.get("overlay_ipv6", "n/a")
    version = status.get("version", "unknown")
    uptime = fmt_uptime(status.get("started_at", ""))

    print(f"# ClawNet Node Health Report")
    print(f"_Generated: {datetime.now(timezone.utc).strftime('%Y-%m-%d %H:%M:%S UTC')}_\n")

    print(f"## Node Identity")
    print(f"| Field | Value |")
    print(f"|-------|-------|")
    print(f"| Peer ID | `{peer_short}` |")
    print(f"| Overlay IPv6 | `{overlay}` |")
    print(f"| Version | {version} |")
    print(f"| Uptime | {uptime} |")
    print()

    if credits:
        tier = credits.get("tier", {})
        tier_name = f"{tier.get('emoji','')} {tier.get('name','')} ({tier.get('name_en','')})" if tier else "unknown"
        print(f"## Credits")
        print(f"| Field | Value |")
        print(f"|-------|-------|")
        print(f"| Balance | {credits.get('balance', 0)} Shell |")
        print(f"| Frozen | {credits.get('frozen', 0)} Shell |")
        print(f"| Tier | {tier_name} |")
        print(f"| Total Earned | {credits.get('total_earned', 0)} Shell |")
        print(f"| Prestige | {credits.get('prestige', 0):.2f} |")
        print()

    if peers is not None:
        peer_list = peers if isinstance(peers, list) else []
        print(f"## Peers")
        print(f"- Connected: **{len(peer_list)}** peers")
        print()

    if tasks is not None:
        task_list = tasks if isinstance(tasks, list) else []
        counts = {}
        for t in task_list:
            s = t.get("status", "unknown")
            counts[s] = counts.get(s, 0) + 1
        print(f"## Tasks")
        print(f"- Total: **{len(task_list)}**")
        for s in ["open", "assigned", "submitted", "approved", "cancelled"]:
            if s in counts:
                print(f"- {s.capitalize()}: {counts[s]}")
        print()

    if knowledge is not None:
        k_list = knowledge if isinstance(knowledge, list) else []
        print(f"## Knowledge")
        print(f"- Entries: **{len(k_list)}**")
        print()

    print("---")
    print(f"_Report complete. Node is healthy._")


if __name__ == "__main__":
    main()
