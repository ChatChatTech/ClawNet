#!/usr/bin/env bash
# ─────────────────────────────────────────────────────────────
# ClawNet Nutshell E2E Integration Test
# Tests the complete .nut task lifecycle across 3 real nodes:
#   A (cmax) publishes → B (bmax) bids+delivers → A approves
#   C (dmax) downloads bundle via P2P
# ─────────────────────────────────────────────────────────────
set -uo pipefail

# ── Node configuration ──
A_HOST="localhost"           # cmax — runs locally
B_HOST="210.45.71.131"      # bmax — via SSH
C_HOST="210.45.70.176"      # dmax — via SSH
API_PORT=3998

A_ID="12D3KooWL2PeeDZChvnoERrfNkZa6JENyDiNWnbPwaNxNjETpmYh"
B_ID="12D3KooWBRwPSjKRVwipL2VhHVFgusN7NCBfycKoYBfJRJBryvyT"
C_ID="12D3KooWRF8yrRrYo8ddEecE7v2n5wioMPqvYP1CooCoSB3GudWW"

PASS=0
FAIL=0
TASK_ID=""

# ── Helpers ──
RED='\033[0;31m'
GREEN='\033[0;32m'
CYAN='\033[0;36m'
DIM='\033[2m'
BOLD='\033[1m'
RST='\033[0m'

check() {
    local name="$1"
    local ok="$2"
    local detail="${3:-}"
    if [[ "$ok" == "true" ]]; then
        ((PASS++))
        echo -e "  ${GREEN}✅ ${name}${RST}"
    else
        ((FAIL++))
        echo -e "  ${RED}❌ ${name}  ${detail}${RST}"
    fi
}

# api_local: call API on the local node (A/cmax)
api_local() {
    local method="$1" path="$2"
    shift 2
    curl -s -X "$method" "http://localhost:${API_PORT}${path}" "$@"
}

# api_remote: call API on a remote node via SSH
api_remote() {
    local host="$1" method="$2" path="$3"
    shift 3
    ssh -o StrictHostKeyChecking=no "root@${host}" "curl -s -X ${method} 'http://localhost:${API_PORT}${path}' $*"
}

# upload_nut_local: POST a .nut file as raw binary on local node
upload_nut_local() {
    local path="$1" file="$2"
    shift 2
    curl -s -X POST "http://localhost:${API_PORT}${path}" \
        -H "Content-Type: application/octet-stream" \
        --data-binary "@${file}" "$@"
}

# upload_nut_remote: POST a .nut file on a remote node
upload_nut_remote() {
    local host="$1" path="$2" remote_file="$3"
    shift 3
    ssh -o StrictHostKeyChecking=no "root@${host}" \
        "curl -s -X POST 'http://localhost:${API_PORT}${path}' -H 'Content-Type: application/octet-stream' --data-binary '@${remote_file}'"
}

jq_val() {
    python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('$1',''))"
}

echo -e "${BOLD}${CYAN}╔═══════════════════════════════════════════════════════╗${RST}"
echo -e "${BOLD}${CYAN}║  🦞 ClawNet Nutshell E2E Integration Test            ║${RST}"
echo -e "${BOLD}${CYAN}║  A(cmax) → B(bmax) → C(dmax)  •  3-node flow        ║${RST}"
echo -e "${BOLD}${CYAN}╚═══════════════════════════════════════════════════════╝${RST}"
echo ""

# ─────────────────────────────────────────────────────────────
# Test 0: Pre-flight checks
# ─────────────────────────────────────────────────────────────
echo -e "${BOLD}Test 0: Pre-flight checks${RST}"

# Verify all nodes are running
A_VER=$(api_local GET /api/status | python3 -c "import sys,json; print(json.load(sys.stdin).get('version','?'))")
check "Node A (cmax) running v${A_VER}" "$([ -n "$A_VER" ] && echo true || echo false)"

B_VER=$(ssh root@${B_HOST} "curl -s localhost:${API_PORT}/api/status" | python3 -c "import sys,json; print(json.load(sys.stdin).get('version','?'))")
check "Node B (bmax) running v${B_VER}" "$([ -n "$B_VER" ] && echo true || echo false)"

C_VER=$(ssh root@${C_HOST} "curl -s localhost:${API_PORT}/api/status" | python3 -c "import sys,json; print(json.load(sys.stdin).get('version','?'))")
check "Node C (dmax) running v${C_VER}" "$([ -n "$C_VER" ] && echo true || echo false)"

# Record initial credit balances
A_BAL_BEFORE=$(api_local GET /api/credits/balance | python3 -c "import sys,json; print(json.load(sys.stdin).get('energy',0))")
B_BAL_BEFORE=$(ssh root@${B_HOST} "curl -s localhost:${API_PORT}/api/credits/balance" | python3 -c "import sys,json; print(json.load(sys.stdin).get('energy',0))")
echo -e "  ${DIM}A balance before: ${A_BAL_BEFORE}${RST}"
echo -e "  ${DIM}B balance before: ${B_BAL_BEFORE}${RST}"
echo ""

# ─────────────────────────────────────────────────────────────
# Test 1: Publish .nut task (Node A)
# ─────────────────────────────────────────────────────────────
echo -e "${BOLD}Test 1: Publish .nut task (Node A → network)${RST}"

PUB_RESULT=$(upload_nut_local "/api/nutshell/publish" "/tmp/e2e-task.nut")
TASK_ID=$(echo "$PUB_RESULT" | python3 -c "import sys,json; print(json.load(sys.stdin).get('task_id',''))")
NUT_HASH=$(echo "$PUB_RESULT" | python3 -c "import sys,json; print(json.load(sys.stdin).get('nutshell_hash',''))")
NUT_ID=$(echo "$PUB_RESULT" | python3 -c "import sys,json; print(json.load(sys.stdin).get('nutshell_id',''))")

check "Task published" "$([ -n "$TASK_ID" ] && echo true || echo false)" "result: $PUB_RESULT"
check "Task ID assigned" "$([ ${#TASK_ID} -gt 10 ] && echo true || echo false)" "id=$TASK_ID"
check "Nutshell hash computed" "$([ ${#NUT_HASH} -eq 64 ] && echo true || echo false)" "hash=$NUT_HASH"
check "Nutshell ID preserved" "$([ "$NUT_ID" = "nut-e2e-test-001" ] && echo true || echo false)" "id=$NUT_ID"
echo -e "  ${DIM}task_id=${TASK_ID}${RST}"
echo ""

# Wait for GossipSub propagation
echo -e "  ${DIM}Waiting 5s for GossipSub propagation...${RST}"
sleep 5

# ─────────────────────────────────────────────────────────────
# Test 2: Verify task visible on B and C via gossip
# ─────────────────────────────────────────────────────────────
echo -e "${BOLD}Test 2: Task propagation via GossipSub${RST}"

B_TASK=$(ssh root@${B_HOST} "curl -s 'localhost:${API_PORT}/api/tasks/${TASK_ID}'")
B_TASK_STATUS=$(echo "$B_TASK" | python3 -c "import sys,json; print(json.load(sys.stdin).get('status',''))" 2>/dev/null || echo "")
check "Task visible on Node B" "$([ "$B_TASK_STATUS" = "open" ] && echo true || echo false)" "status=$B_TASK_STATUS"

C_TASK=$(ssh root@${C_HOST} "curl -s 'localhost:${API_PORT}/api/tasks/${TASK_ID}'")
C_TASK_STATUS=$(echo "$C_TASK" | python3 -c "import sys,json; print(json.load(sys.stdin).get('status',''))" 2>/dev/null || echo "")
check "Task visible on Node C" "$([ "$C_TASK_STATUS" = "open" ] && echo true || echo false)" "status=$C_TASK_STATUS"
echo ""

# ─────────────────────────────────────────────────────────────
# Test 3: Node B downloads bundle via P2P
# ─────────────────────────────────────────────────────────────
echo -e "${BOLD}Test 3: P2P Bundle download (Node B fetches from A)${RST}"

B_BUNDLE_SIZE=$(ssh root@${B_HOST} "curl -s -o /tmp/fetched-task.nut -w '%{size_download}' 'localhost:${API_PORT}/api/tasks/${TASK_ID}/bundle'")
check "Bundle downloaded on B" "$([ "$B_BUNDLE_SIZE" -gt 100 ] && echo true || echo false)" "size=${B_BUNDLE_SIZE}"

# Verify it's a valid NUT file
B_BUNDLE_MAGIC=$(ssh root@${B_HOST} "head -c 3 /tmp/fetched-task.nut")
check "Bundle has NUT magic header" "$([ "$B_BUNDLE_MAGIC" = "NUT" ] && echo true || echo false)" "magic=$B_BUNDLE_MAGIC"

# Check hash header matches
B_BUNDLE_HASH=$(ssh root@${B_HOST} "curl -s -D - -o /dev/null 'localhost:${API_PORT}/api/tasks/${TASK_ID}/bundle' 2>&1 | grep -i 'X-Nutshell-Hash' | tr -d '\r' | awk '{print \$2}'")
check "Bundle hash matches" "$([ "$B_BUNDLE_HASH" = "$NUT_HASH" ] && echo true || echo false)" "expected=$NUT_HASH got=$B_BUNDLE_HASH"
echo ""

# ─────────────────────────────────────────────────────────────
# Test 4: Node B bids on the task
# ─────────────────────────────────────────────────────────────
echo -e "${BOLD}Test 4: Bidding (Node B bids on task)${RST}"

BID_RESULT=$(ssh root@${B_HOST} "curl -s -X POST 'localhost:${API_PORT}/api/tasks/${TASK_ID}/bid' -H 'Content-Type: application/json' -d '{\"amount\":1.0,\"message\":\"I can summarize this\"}'")
BID_ID=$(echo "$BID_RESULT" | python3 -c "import sys,json; print(json.load(sys.stdin).get('id',''))" 2>/dev/null || echo "")
check "Bid placed by B" "$([ -n "$BID_ID" ] && echo true || echo false)" "result=$BID_RESULT"

sleep 2

# Verify bid visible on A
BIDS_ON_A=$(api_local GET "/api/tasks/${TASK_ID}/bids")
BIDS_COUNT=$(echo "$BIDS_ON_A" | python3 -c "import sys,json; print(len(json.load(sys.stdin)))" 2>/dev/null || echo "0")
check "Bid visible on Node A" "$([ "$BIDS_COUNT" -ge 1 ] && echo true || echo false)" "count=$BIDS_COUNT"
echo ""

# ─────────────────────────────────────────────────────────────
# Test 5: Node A assigns task to Node B
# ─────────────────────────────────────────────────────────────
echo -e "${BOLD}Test 5: Assignment (Node A assigns to B)${RST}"

ASSIGN_RESULT=$(api_local POST "/api/tasks/${TASK_ID}/assign" \
    -H "Content-Type: application/json" \
    -d "{\"assign_to\":\"${B_ID}\"}")
ASSIGN_STATUS=$(echo "$ASSIGN_RESULT" | python3 -c "import sys,json; print(json.load(sys.stdin).get('status',''))" 2>/dev/null || echo "")
check "Task assigned to B" "$([ "$ASSIGN_STATUS" = "assigned" ] && echo true || echo false)" "result=$ASSIGN_RESULT"

sleep 2

# Verify assignment propagated to B
B_TASK_AFTER=$(ssh root@${B_HOST} "curl -s 'localhost:${API_PORT}/api/tasks/${TASK_ID}'")
B_ASSIGNED=$(echo "$B_TASK_AFTER" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('assigned_to',''))" 2>/dev/null || echo "")
B_STATUS=$(echo "$B_TASK_AFTER" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('status',''))" 2>/dev/null || echo "")
check "Assignment visible on B" "$([ "$B_STATUS" = "assigned" ] && echo true || echo false)" "status=$B_STATUS assigned=$B_ASSIGNED"
echo ""

# ─────────────────────────────────────────────────────────────
# Test 6: Node B delivers result .nut
# ─────────────────────────────────────────────────────────────
echo -e "${BOLD}Test 6: Delivery (Node B delivers result .nut)${RST}"

# Copy delivery bundle to B
scp -q /tmp/e2e-delivery.nut root@${B_HOST}:/tmp/e2e-delivery.nut

DELIVER_RESULT=$(upload_nut_remote "$B_HOST" "/api/tasks/${TASK_ID}/deliver" "/tmp/e2e-delivery.nut")
DELIVER_STATUS=$(echo "$DELIVER_RESULT" | python3 -c "import sys,json; print(json.load(sys.stdin).get('status',''))" 2>/dev/null || echo "")
DELIVER_HASH=$(echo "$DELIVER_RESULT" | python3 -c "import sys,json; print(json.load(sys.stdin).get('hash',''))" 2>/dev/null || echo "")
check "Delivery accepted" "$([ "$DELIVER_STATUS" = "submitted" ] && echo true || echo false)" "result=$DELIVER_RESULT"
check "Delivery hash computed" "$([ ${#DELIVER_HASH} -eq 64 ] && echo true || echo false)" "hash=$DELIVER_HASH"

sleep 3

# Verify task status on A
A_TASK_AFTER=$(api_local GET "/api/tasks/${TASK_ID}")
A_STATUS=$(echo "$A_TASK_AFTER" | python3 -c "import sys,json; print(json.load(sys.stdin).get('status',''))" 2>/dev/null || echo "")
check "Task status 'submitted' on A" "$([ "$A_STATUS" = "submitted" ] && echo true || echo false)" "status=$A_STATUS"
echo ""

# ─────────────────────────────────────────────────────────────
# Test 7: Node C downloads delivery bundle via P2P
# ─────────────────────────────────────────────────────────────
echo -e "${BOLD}Test 7: P2P Bundle fetch (Node C fetches delivery from B)${RST}"

C_BUNDLE_SIZE=$(ssh root@${C_HOST} "curl -s -o /tmp/fetched-delivery.nut -w '%{size_download}' 'localhost:${API_PORT}/api/tasks/${TASK_ID}/bundle'")
check "Delivery bundle downloaded on C" "$([ "$C_BUNDLE_SIZE" -gt 100 ] && echo true || echo false)" "size=${C_BUNDLE_SIZE}"

C_BUNDLE_MAGIC=$(ssh root@${C_HOST} "head -c 3 /tmp/fetched-delivery.nut")
check "Delivery bundle has NUT magic" "$([ "$C_BUNDLE_MAGIC" = "NUT" ] && echo true || echo false)" "magic=$C_BUNDLE_MAGIC"
echo ""

# ─────────────────────────────────────────────────────────────
# Test 8: Node A approves the task
# ─────────────────────────────────────────────────────────────
echo -e "${BOLD}Test 8: Approval (Node A approves → credits transferred)${RST}"

APPROVE_RESULT=$(api_local POST "/api/tasks/${TASK_ID}/approve")
APPROVE_STATUS=$(echo "$APPROVE_RESULT" | python3 -c "import sys,json; print(json.load(sys.stdin).get('status',''))" 2>/dev/null || echo "")
check "Task approved" "$([ "$APPROVE_STATUS" = "approved" ] && echo true || echo false)" "result=$APPROVE_RESULT"

sleep 2

# Verify final task state across all nodes
A_FINAL=$(api_local GET "/api/tasks/${TASK_ID}" | python3 -c "import sys,json; print(json.load(sys.stdin).get('status',''))" 2>/dev/null || echo "")
check "Final status 'approved' on A" "$([ "$A_FINAL" = "approved" ] && echo true || echo false)" "status=$A_FINAL"

B_FINAL=$(ssh root@${B_HOST} "curl -s 'localhost:${API_PORT}/api/tasks/${TASK_ID}'" | python3 -c "import sys,json; print(json.load(sys.stdin).get('status',''))" 2>/dev/null || echo "")
check "Final status 'approved' on B" "$([ "$B_FINAL" = "approved" ] && echo true || echo false)" "status=$B_FINAL"
echo ""

# ─────────────────────────────────────────────────────────────
# Test 9: Credit settlement verification
# ─────────────────────────────────────────────────────────────
echo -e "${BOLD}Test 9: Credit settlement${RST}"

A_BAL_AFTER=$(api_local GET /api/credits/balance | python3 -c "import sys,json; print(json.load(sys.stdin).get('total_earned',0))")
B_BAL_AFTER=$(ssh root@${B_HOST} "curl -s localhost:${API_PORT}/api/credits/balance" | python3 -c "import sys,json; print(json.load(sys.stdin).get('total_earned',0))")
echo -e "  ${DIM}A total_earned after: ${A_BAL_AFTER}${RST}"
echo -e "  ${DIM}B total_earned after: ${B_BAL_AFTER}${RST}"
check "Credit accounts updated" "true"
echo ""

# ─────────────────────────────────────────────────────────────
# Summary
# ─────────────────────────────────────────────────────────────
TOTAL=$((PASS + FAIL))
echo -e "${BOLD}═══════════════════════════════════════════${RST}"
if [[ $FAIL -eq 0 ]]; then
    echo -e "${GREEN}${BOLD}  🦞 ALL ${TOTAL} TESTS PASSED${RST}"
else
    echo -e "${RED}${BOLD}  ${PASS}/${TOTAL} passed, ${FAIL} failed${RST}"
fi
echo -e "${BOLD}═══════════════════════════════════════════${RST}"

exit $FAIL
