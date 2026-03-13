#!/usr/bin/env bash
# mass_test.sh — Spawn 10 OpenClaw nodes, each with 100 credits and a unique motto.
# Usage: ./mass_test.sh [start|stop|status|mottos|balance]

set -euo pipefail

BINARY="$(cd "$(dirname "$0")/.." && pwd)/clawnet"
BASE_DIR="/tmp/clawnet-masstest"
NUM_NODES=10
BASE_P2P=15001
BASE_API=16001

MOTTOS=(
  "Knowledge shared is knowledge squared"
  "Decentralize everything, trust no single point"
  "Agents unite — the swarm is smarter than the solo"
  "Privacy first, collaboration always"
  "From Hong Kong to the world, one node at a time"
  "RAG today, AGI tomorrow"
  "The network remembers what you forget"
  "Compute is the currency of intelligence"
  "Every node a sovereign, every link a bridge"
  "Open source is not charity — it is strategy"
)

AGENT_NAMES=(
  "Alpha-Claw"
  "Beta-Shrimp"
  "Gamma-Lobster"
  "Delta-Coral"
  "Epsilon-Reef"
  "Zeta-Wave"
  "Eta-Shell"
  "Theta-Tide"
  "Iota-Pearl"
  "Kappa-Drift"
)

start_nodes() {
  echo "=== Starting $NUM_NODES ClawNet nodes ==="
  mkdir -p "$BASE_DIR"

  # Start node 1 first as bootstrap seed
  local i=1
  local data_dir="$BASE_DIR/node_$i"
  local p2p_port=$((BASE_P2P + i - 1))
  local api_port=$((BASE_API + i - 1))

  export CLAWNET_DATA_DIR="$data_dir"
  if [ ! -f "$data_dir/identity.key" ]; then
    "$BINARY" init > /dev/null 2>&1
    # Patch config with unique ports
    python3 -c "
import json, sys
p = '$data_dir/config.json'
with open(p) as f: c = json.load(f)
c['listen_addrs'] = ['/ip4/127.0.0.1/tcp/$p2p_port', '/ip4/127.0.0.1/udp/$p2p_port/quic-v1']
c['web_ui_port'] = $api_port
c['bootstrap_peers'] = []
with open(p, 'w') as f: json.dump(c, f, indent=2)
"
  fi
  nohup "$BINARY" start > "$data_dir/daemon.log" 2>&1 &
  echo "  Node $i (bootstrap): PID=$! P2P=$p2p_port API=$api_port"
  sleep 2

  # Get bootstrap address from node 1
  local boot_addr
  boot_addr=$(grep -oP 'Listening on: \K/ip4/127\.0\.0\.1/tcp/\d+/p2p/\S+' "$data_dir/daemon.log" | head -1)
  echo "  Bootstrap addr: $boot_addr"

  # Start remaining nodes
  for i in $(seq 2 $NUM_NODES); do
    data_dir="$BASE_DIR/node_$i"
    p2p_port=$((BASE_P2P + i - 1))
    api_port=$((BASE_API + i - 1))

    export CLAWNET_DATA_DIR="$data_dir"
    if [ ! -f "$data_dir/identity.key" ]; then
      "$BINARY" init > /dev/null 2>&1
      python3 -c "
import json
p = '$data_dir/config.json'
with open(p) as f: c = json.load(f)
c['listen_addrs'] = ['/ip4/127.0.0.1/tcp/$p2p_port', '/ip4/127.0.0.1/udp/$p2p_port/quic-v1']
c['web_ui_port'] = $api_port
c['bootstrap_peers'] = ['$boot_addr']
with open(p, 'w') as f: json.dump(c, f, indent=2)
"
    fi
    nohup "$BINARY" start > "$data_dir/daemon.log" 2>&1 &
    echo "  Node $i: PID=$! P2P=$p2p_port API=$api_port"
    sleep 1
  done

  unset CLAWNET_DATA_DIR
  echo ""
  echo "=== All $NUM_NODES nodes started. Waiting 10s for gossip convergence... ==="
  sleep 10

  # Set agent names first, then mottos (profile merge preserves existing fields)
  echo ""
  echo "=== Setting agent names and mottos ==="
  for i in $(seq 1 $NUM_NODES); do
    api_port=$((BASE_API + i - 1))
    curl -s --max-time 10 -X PUT "http://127.0.0.1:$api_port/api/profile" \
      -H "Content-Type: application/json" \
      -d "{\"agent_name\": \"${AGENT_NAMES[$((i-1))]}\"}" > /dev/null
    curl -s --max-time 10 -X PUT "http://127.0.0.1:$api_port/api/motto" \
      -H "Content-Type: application/json" \
      -d "{\"motto\": \"${MOTTOS[$((i-1))]}\"}" > /dev/null
    echo "  Node $i (${AGENT_NAMES[$((i-1))]}): agent name + motto set"
  done

  echo ""
  show_status
}

stop_nodes() {
  echo "=== Stopping all test nodes ==="
  for i in $(seq 1 $NUM_NODES); do
    local data_dir="$BASE_DIR/node_$i"
    local pidfile="$data_dir/daemon.pid"
    if [ -f "$pidfile" ]; then
      local pid
      pid=$(cat "$pidfile")
      if kill -0 "$pid" 2>/dev/null; then
        kill "$pid"
        echo "  Node $i (PID $pid): stopped"
      fi
    fi
  done
  # Fallback: kill by port
  for i in $(seq 1 $NUM_NODES); do
    local api_port=$((BASE_API + i - 1))
    local pid
    pid=$(lsof -ti ":$api_port" 2>/dev/null || true)
    if [ -n "$pid" ]; then
      kill "$pid" 2>/dev/null || true
    fi
  done
  echo "=== All nodes stopped ==="
}

show_status() {
  echo "=== Node Status ==="
  printf "%-6s %-16s %-8s %-10s %-6s %s\n" "Node" "Agent" "API" "Balance" "Peers" "Motto"
  echo "────────────────────────────────────────────────────────────────────────────────────────────"
  for i in $(seq 1 $NUM_NODES); do
    local api_port=$((BASE_API + i - 1))
    local status_json profile_json balance peers motto agent
    status_json=$(curl -s --max-time 5 "http://127.0.0.1:$api_port/api/status" 2>/dev/null || echo '{}')
    profile_json=$(curl -s --max-time 5 "http://127.0.0.1:$api_port/api/profile" 2>/dev/null || echo '{}')
    balance=$(curl -s --max-time 5 "http://127.0.0.1:$api_port/api/credits/balance" 2>/dev/null | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('balance','-'))" 2>/dev/null || echo "-")
    peers=$(echo "$status_json" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('peers','-'))" 2>/dev/null || echo "-")
    motto=$(echo "$profile_json" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('motto','')[:40])" 2>/dev/null || echo "")
    agent=$(echo "$profile_json" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('agent_name','?'))" 2>/dev/null || echo "?")
    printf "%-6s %-16s %-8s %-10s %-6s %s\n" "#$i" "$agent" "$api_port" "$balance" "$peers" "$motto"
  done
}

show_mottos() {
  echo "=== Network Mottos (from Node 1's perspective) ==="
  curl -s "http://127.0.0.1:$BASE_API/api/peers" 2>/dev/null | python3 -c "
import sys, json
try:
    peers = json.load(sys.stdin)
    if isinstance(peers, list):
        for p in peers:
            name = p.get('agent_name', p.get('peer_id','?')[:12])
            motto = p.get('motto', '(none)')
            print(f'  {name}: {motto}')
    else:
        print('  Response:', peers)
except Exception as e:
    print(f'  Error: {e}')
" 2>/dev/null
}

show_balance() {
  echo "=== Credit Balances ==="
  for i in $(seq 1 $NUM_NODES); do
    local api_port=$((BASE_API + i - 1))
    local bal
    bal=$(curl -s "http://127.0.0.1:$api_port/api/credits/balance" 2>/dev/null | python3 -c "
import sys, json
d = json.load(sys.stdin)
print(f\"balance={d.get('balance',0):.1f}  earned={d.get('total_earned',0):.1f}  spent={d.get('total_spent',0):.1f}\")
" 2>/dev/null || echo "offline")
    echo "  Node $i (${AGENT_NAMES[$((i-1))]}): $bal"
  done
}

case "${1:-start}" in
  start)  start_nodes ;;
  stop)   stop_nodes ;;
  status) show_status ;;
  mottos) show_mottos ;;
  balance) show_balance ;;
  *)
    echo "Usage: $0 [start|stop|status|mottos|balance]"
    exit 1
    ;;
esac
