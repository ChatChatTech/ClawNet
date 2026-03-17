#!/usr/bin/env bash
# deploy-test.sh — Stop, deploy, and restart a test binary on cmax/bmax/dmax.
#
# Usage:
#   ./deploy-test.sh <binary> [start-args...]
#
# Examples:
#   ./deploy-test.sh clawnet-cli/clawnet start
#   ./deploy-test.sh /data/projs/ironwood-test/ironwood-test -listen :9901 -peers tcp://...
#
# The binary is:
#   - Killed on all 3 nodes (by basename)
#   - SCP'd to bmax and dmax as /tmp/<basename>
#   - Started on all 3 nodes in background, logs to /tmp/<basename>.log
#
# Environment:
#   CMAX=210.45.71.67   (local, no SSH)
#   BMAX=210.45.71.131
#   DMAX=210.45.70.176
set -euo pipefail

BMAX=210.45.71.131
DMAX=210.45.70.176

RED='\033[0;31m'
GRN='\033[0;32m'
YEL='\033[0;33m'
DIM='\033[2m'
RST='\033[0m'

step() { printf "${GRN}▸${RST} %s\n" "$*"; }
warn() { printf "${YEL}▸${RST} %s\n" "$*"; }
fail() { printf "${RED}✗${RST} %s\n" "$*"; exit 1; }

# ── Args ──
[[ $# -lt 1 ]] && fail "Usage: $0 <binary> [start-args...]"
BINARY="$1"; shift
START_ARGS=("$@")
BASENAME=$(basename "$BINARY")
REMOTE_BIN="/tmp/${BASENAME}"
LOG="/tmp/${BASENAME}.log"

[[ -f "$BINARY" ]] || fail "Binary not found: $BINARY"

# ── SSH helper with retry ──
ssh_cmd() {
    local host=$1; shift
    local retries=3
    for ((i=1; i<=retries; i++)); do
        if ssh -o ConnectTimeout=8 -o StrictHostKeyChecking=no "root@${host}" "$@" 2>/dev/null; then
            return 0
        fi
        [[ $i -lt $retries ]] && sleep 3
    done
    warn "SSH to ${host} failed after ${retries} attempts"
    return 1
}

# ── 1. Stop everywhere ──
step "Stopping ${BASENAME} on all nodes..."
pkill -9 -f "${BASENAME}" 2>/dev/null || true
for host in $BMAX $DMAX; do
    ssh_cmd "$host" "pkill -9 -f ${BASENAME} 2>/dev/null; sleep 0.5; echo ok" &
done
wait
sleep 1

# ── 2. Deploy to remote nodes ──
step "Deploying ${BASENAME} to bmax and dmax..."
for host in $BMAX $DMAX; do
    scp -o ConnectTimeout=8 -q "$BINARY" "root@${host}:${REMOTE_BIN}" && \
        ssh_cmd "$host" "chmod +x ${REMOTE_BIN}" && \
        printf "  ${DIM}→ ${host} ✓${RST}\n" || \
        warn "  → ${host} deploy failed"
done

# ── 3. Copy locally ──
LOCAL_BIN="${REMOTE_BIN}"
cp "$BINARY" "$LOCAL_BIN" 2>/dev/null || LOCAL_BIN="$BINARY"

# ── 4. Start on remote nodes ──
if [[ ${#START_ARGS[@]} -gt 0 ]]; then
    step "Starting ${BASENAME} on all nodes..."

    # bmax
    ssh_cmd "$BMAX" "nohup ${REMOTE_BIN} ${START_ARGS[*]} > ${LOG} 2>&1 </dev/null & disown; sleep 1; pgrep -f '${BASENAME}' >/dev/null && echo running || echo failed" &
    PID_BMAX=$!

    # dmax
    ssh_cmd "$DMAX" "nohup ${REMOTE_BIN} ${START_ARGS[*]} > ${LOG} 2>&1 </dev/null & disown; sleep 1; pgrep -f '${BASENAME}' >/dev/null && echo running || echo failed" &
    PID_DMAX=$!

    # cmax (local)
    nohup "$LOCAL_BIN" "${START_ARGS[@]}" > "$LOG" 2>&1 </dev/null &
    disown
    sleep 1

    wait $PID_BMAX 2>/dev/null
    wait $PID_DMAX 2>/dev/null

    # ── 5. Verify ──
    step "Verifying..."
    printf "  cmax: "
    pgrep -f "${BASENAME}" >/dev/null 2>&1 && printf "${GRN}running${RST}\n" || printf "${RED}not running${RST}\n"
    for host in $BMAX $DMAX; do
        name=$([ "$host" = "$BMAX" ] && echo "bmax" || echo "dmax")
        printf "  ${name}: "
        ssh_cmd "$host" "pgrep -f '${BASENAME}' >/dev/null && echo running || echo failed" || echo "unreachable"
    done
else
    step "Binary deployed. No start args given — not starting."
fi

echo ""
step "Logs: ${LOG} (all nodes)"
step "To tail: ssh root@\${HOST} 'tail -f ${LOG}'"
step "To stop: $0 ${BINARY}  (run with no start-args)"
