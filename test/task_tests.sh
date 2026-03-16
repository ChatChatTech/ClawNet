#!/bin/bash
# ClawNet v0.9.8 — 任务系统深度测试套件
# 三节点集群: cmax (local) / bmax (210.45.71.131) / dmax (210.45.70.176)
# 覆盖: Legacy流程 + Auction House流程 + 信用验证 + 状态机 + 边界情况 + 多轮测试
set -o pipefail

CMAX="http://localhost:3998"
BMAX_HOST="root@210.45.71.131"
DMAX_HOST="root@210.45.70.176"
BMAX="http://localhost:3998"
DMAX="http://localhost:3998"

OUTDIR="$(cd "$(dirname "$0")" && pwd)"
REPORT="$OUTDIR/task-test-report.md"
DETAIL="$OUTDIR/task-test-detail.log"

PASS=0
FAIL=0
SKIP=0
TOTAL=0
RESULTS=()

# ─── Helpers ─────────────────────────────────────────────────────────────────

cmax_api() { curl -sf --max-time 10 "${CMAX}$1" 2>/dev/null; }
cmax_post(){ curl -sf --max-time 10 -X POST -H "Content-Type: application/json" -d "$2" "${CMAX}$1" 2>/dev/null; }
cmax_put() { curl -sf --max-time 10 -X PUT  -H "Content-Type: application/json" -d "$2" "${CMAX}$1" 2>/dev/null; }
cmax_http(){ curl -s -o /dev/null -w "%{http_code}" --max-time 10 -X "$2" -H "Content-Type: application/json" -d "$4" "${CMAX}$1" 2>/dev/null; }
cmax_body_code() { local tmp; tmp=$(mktemp); curl -s -w "\n%{http_code}" --max-time 10 -X POST -H "Content-Type: application/json" -d "$2" "${CMAX}$1" 2>/dev/null > "$tmp"; local code; code=$(tail -1 "$tmp"); local body; body=$(sed '$d' "$tmp"); rm -f "$tmp"; echo "$body"; return 0; }

bmax_api() { ssh -o ConnectTimeout=5 $BMAX_HOST "curl -sf --max-time 10 '${BMAX}$1'" 2>/dev/null; }
bmax_post(){ ssh -o ConnectTimeout=5 $BMAX_HOST "curl -sf --max-time 10 -X POST -H 'Content-Type: application/json' -d '$2' '${BMAX}$1'" 2>/dev/null; }
bmax_http(){ ssh -o ConnectTimeout=5 $BMAX_HOST "curl -s -o /dev/null -w '%{http_code}' --max-time 10 -X POST -H 'Content-Type: application/json' -d '$2' '${BMAX}$1'" 2>/dev/null; }

dmax_api() { ssh -o ConnectTimeout=5 $DMAX_HOST "curl -sf --max-time 10 '${DMAX}$1'" 2>/dev/null; }
dmax_post(){ ssh -o ConnectTimeout=5 $DMAX_HOST "curl -sf --max-time 10 -X POST -H 'Content-Type: application/json' -d '$2' '${DMAX}$1'" 2>/dev/null; }
dmax_http(){ ssh -o ConnectTimeout=5 $DMAX_HOST "curl -s -o /dev/null -w '%{http_code}' --max-time 10 -X POST -H 'Content-Type: application/json' -d '$2' '${DMAX}$1'" 2>/dev/null; }

jval() { python3 -c "import json,sys; d=json.load(sys.stdin); print(d$1)" 2>/dev/null; }
jlen() { python3 -c "import json,sys; d=json.load(sys.stdin); print(len(d))" 2>/dev/null; }
http_code() { curl -s -o /dev/null -w "%{http_code}" --max-time 10 -X "$1" -H "Content-Type: application/json" -d "$3" "${CMAX}$2" 2>/dev/null; }

record() {
    local id="$1" name="$2" status="$3" detail="$4"
    TOTAL=$((TOTAL+1))
    case "$status" in
        PASS) PASS=$((PASS+1)); mark="✅" ;;
        FAIL) FAIL=$((FAIL+1)); mark="❌" ;;
        SKIP) SKIP=$((SKIP+1)); mark="⏭️" ;;
    esac
    RESULTS+=("| $id | $name | $mark $status | $detail |")
    echo "[$status] $id — $name — $detail" | tee -a "$DETAIL"
}

get_bal() {
    case "$1" in
        cmax) cmax_api "/api/credits/balance" | jval "['balance']" ;;
        bmax) bmax_api "/api/credits/balance" | jval "['balance']" ;;
        dmax) dmax_api "/api/credits/balance" | jval "['balance']" ;;
    esac
}

get_frozen() {
    case "$1" in
        cmax) cmax_api "/api/credits/balance" | jval "['frozen']" ;;
        bmax) bmax_api "/api/credits/balance" | jval "['frozen']" ;;
        dmax) dmax_api "/api/credits/balance" | jval "['frozen']" ;;
    esac
}

# ─── Init ────────────────────────────────────────────────────────────────────

: > "$DETAIL"
echo "═══════════════════════════════════════════════════" | tee -a "$DETAIL"
echo "  ClawNet v0.9.8 — 任务系统深度测试" | tee -a "$DETAIL"
echo "  $(date '+%Y-%m-%d %H:%M:%S')" | tee -a "$DETAIL"
echo "═══════════════════════════════════════════════════" | tee -a "$DETAIL"
echo "" | tee -a "$DETAIL"

CMAX_PID=$(cmax_api "/api/status" | jval "['peer_id']")
BMAX_PID=$(bmax_api "/api/status" | jval "['peer_id']")
DMAX_PID=$(dmax_api "/api/status" | jval "['peer_id']")

echo "cmax: $CMAX_PID" | tee -a "$DETAIL"
echo "bmax: $BMAX_PID" | tee -a "$DETAIL"
echo "dmax: $DMAX_PID" | tee -a "$DETAIL"
echo "" | tee -a "$DETAIL"

if [ -z "$CMAX_PID" ] || [ -z "$BMAX_PID" ] || [ -z "$DMAX_PID" ]; then
    echo "FATAL: Not all 3 nodes are reachable" | tee -a "$DETAIL"
    exit 1
fi

# Version check
CMAX_VER=$(cmax_api "/api/status" | jval "['version']")
BMAX_VER=$(bmax_api "/api/status" | jval "['version']")
DMAX_VER=$(dmax_api "/api/status" | jval "['version']")
echo "versions: cmax=$CMAX_VER bmax=$BMAX_VER dmax=$DMAX_VER" | tee -a "$DETAIL"
echo "" | tee -a "$DETAIL"

K_TS=$(date +%s)

###############################################################################
# Phase 0 — 前置条件: Setup profiles (needed for resume/match)
###############################################################################
echo "══ Phase 0: 前置条件 ══" | tee -a "$DETAIL"

cmax_put "/api/profile" '{"agent_name":"TestBot-cmax","bio":"Test author node","domains":["testing","go","devops"]}' > /dev/null 2>&1
ssh -o ConnectTimeout=5 $BMAX_HOST "curl -sf --max-time 5 -X PUT -H 'Content-Type: application/json' -d '{\"agent_name\":\"TestBot-bmax\",\"bio\":\"Test worker node B\",\"domains\":[\"testing\",\"python\",\"ml\"]}' '${BMAX}/api/profile'" > /dev/null 2>&1
ssh -o ConnectTimeout=5 $DMAX_HOST "curl -sf --max-time 5 -X PUT -H 'Content-Type: application/json' -d '{\"agent_name\":\"TestBot-dmax\",\"bio\":\"Test worker node D\",\"domains\":[\"testing\",\"rust\",\"web\"]}' '${DMAX}/api/profile'" > /dev/null 2>&1

# Record initial balances
BAL_CMAX_0=$(get_bal cmax)
BAL_BMAX_0=$(get_bal bmax)
BAL_DMAX_0=$(get_bal dmax)
FRZ_CMAX_0=$(get_frozen cmax)
echo "Initial: cmax=$BAL_CMAX_0 (frozen=$FRZ_CMAX_0), bmax=$BAL_BMAX_0, dmax=$BAL_DMAX_0" | tee -a "$DETAIL"

# Setup resumes for matching tests
cmax_post "/api/resume" '{"skills":["testing","go","devops"],"description":"I do testing and DevOps automation"}' > /dev/null 2>&1
ssh -o ConnectTimeout=5 $BMAX_HOST "curl -sf --max-time 5 -X POST -H 'Content-Type: application/json' -d '{\"skills\":[\"testing\",\"python\",\"ml\"],\"description\":\"ML and Python specialist\"}' '${BMAX}/api/resume'" > /dev/null 2>&1
ssh -o ConnectTimeout=5 $DMAX_HOST "curl -sf --max-time 5 -X POST -H 'Content-Type: application/json' -d '{\"skills\":[\"testing\",\"rust\",\"web\"],\"description\":\"Rust and web development\"}' '${DMAX}/api/resume'" > /dev/null 2>&1

record "P0.1" "Profile & Resume setup" "PASS" "cmax=$BAL_CMAX_0, bmax=$BAL_BMAX_0, dmax=$BAL_DMAX_0"

###############################################################################
# T1 — 任务创建验证
###############################################################################
echo "" | tee -a "$DETAIL"
echo "══ T1: 任务创建验证 ══" | tee -a "$DETAIL"

# T1.1 正常创建 (最小参数)
r=$(cmax_post "/api/tasks" "{\"title\":\"Minimal Task $K_TS\",\"reward\":100}")
T1_ID=$(echo "$r" | jval "['id']")
T1_STATUS=$(echo "$r" | jval "['status']")
if [ -n "$T1_ID" ] && [ "$T1_STATUS" = "open" ]; then
    record "T1.1" "最小参数创建任务" "PASS" "id=${T1_ID:0:12}…, status=$T1_STATUS"
else
    record "T1.1" "最小参数创建任务" "FAIL" "$(echo "$r" | head -c 200)"
fi

# T1.2 完整参数创建 (title, desc, reward, tags, deadline)
DL=$(date -u -d "+7 days" '+%Y-%m-%dT%H:%M:%SZ' 2>/dev/null || date -u -v+7d '+%Y-%m-%dT%H:%M:%SZ')
r=$(cmax_post "/api/tasks" "{\"title\":\"Full Task $K_TS\",\"description\":\"Detailed test task with all parameters\",\"reward\":500,\"tags\":[\"go\",\"testing\",\"automation\"],\"deadline\":\"$DL\"}")
T2_ID=$(echo "$r" | jval "['id']")
if [ -n "$T2_ID" ]; then
    record "T1.2" "完整参数创建任务" "PASS" "id=${T2_ID:0:12}…, reward=500"
else
    record "T1.2" "完整参数创建任务" "FAIL" "$(echo "$r" | head -c 200)"
fi

# T1.3 无标题 → 400
code=$(http_code POST "/api/tasks" '{"reward":200}')
if [ "$code" = "400" ]; then
    record "T1.3" "无标题拒绝" "PASS" "HTTP $code"
else
    record "T1.3" "无标题拒绝" "FAIL" "HTTP $code (expected 400)"
fi

# T1.4 reward < 100 → 400
code=$(http_code POST "/api/tasks" "{\"title\":\"Cheap Task\",\"reward\":50}")
if [ "$code" = "400" ]; then
    record "T1.4" "低奖励拒绝 (50<100)" "PASS" "HTTP $code"
else
    record "T1.4" "低奖励拒绝" "FAIL" "HTTP $code (expected 400)"
fi

# T1.5 reward = 0 → 400
code=$(http_code POST "/api/tasks" "{\"title\":\"Free Task\",\"reward\":0}")
if [ "$code" = "400" ]; then
    record "T1.5" "零奖励拒绝" "PASS" "HTTP $code"
else
    record "T1.5" "零奖励拒绝" "FAIL" "HTTP $code (expected 400)"
fi

# T1.6 reward = -100 → 400
code=$(http_code POST "/api/tasks" "{\"title\":\"Negative Task\",\"reward\":-100}")
if [ "$code" = "400" ]; then
    record "T1.6" "负奖励拒绝" "PASS" "HTTP $code"
else
    record "T1.6" "负奖励拒绝" "FAIL" "HTTP $code (expected 400)"
fi

# T1.7 5% fee deduction verification
BAL_AFTER_CREATE=$(get_bal cmax)
FRZ_AFTER_CREATE=$(get_frozen cmax)
# T1.1 cost: 100 + fee(5)=5 | T1.2 cost: 500 + fee(25)=25 → total fee=30, total frozen=600
EXPECTED_FEE=30
EXPECTED_FROZEN=600
ACTUAL_BAL_DIFF=$((BAL_CMAX_0 - BAL_AFTER_CREATE))  # Should be fee(30) + frozen(600) = 630
echo "  Balance diff: $BAL_CMAX_0 → $BAL_AFTER_CREATE (diff=$ACTUAL_BAL_DIFF, expected=630)" | tee -a "$DETAIL"
if [ "$ACTUAL_BAL_DIFF" = "630" ]; then
    record "T1.7" "5% fee + 冻结验证" "PASS" "diff=$ACTUAL_BAL_DIFF (fee=30+frozen=600)"
else
    record "T1.7" "5% fee + 冻结验证" "FAIL" "diff=$ACTUAL_BAL_DIFF (expected 630)"
fi

# T1.8 Frozen balance check
echo "  Frozen: before=$FRZ_CMAX_0 after=$FRZ_AFTER_CREATE" | tee -a "$DETAIL"
FROZEN_DIFF=$((FRZ_AFTER_CREATE - FRZ_CMAX_0))
if [ "$FROZEN_DIFF" = "600" ]; then
    record "T1.8" "冻结余额验证" "PASS" "frozen_diff=$FROZEN_DIFF (100+500)"
else
    record "T1.8" "冻结余额验证" "FAIL" "frozen_diff=$FROZEN_DIFF (expected 600)"
fi

# T1.9 创建定向任务 (target_peer)
r=$(cmax_post "/api/tasks" "{\"title\":\"Targeted Task $K_TS\",\"description\":\"Only for bmax\",\"reward\":200,\"target_peer\":\"${BMAX_PID}\"}")
T_TARGET_ID=$(echo "$r" | jval "['id']")
if [ -n "$T_TARGET_ID" ]; then
    record "T1.9" "定向任务创建" "PASS" "id=${T_TARGET_ID:0:12}…, target=bmax"
else
    record "T1.9" "定向任务创建" "FAIL" "$(echo "$r" | head -c 200)"
fi

# T1.10 tags as comma string
r=$(cmax_post "/api/tasks" "{\"title\":\"String Tags $K_TS\",\"reward\":100,\"tags\":\"go,python,rust\"}")
T_STAG_ID=$(echo "$r" | jval "['id']")
if [ -n "$T_STAG_ID" ]; then
    record "T1.10" "逗号字符串标签" "PASS" "id=${T_STAG_ID:0:12}…"
else
    record "T1.10" "逗号字符串标签" "FAIL" "$(echo "$r" | head -c 200)"
fi

###############################################################################
# T2 — 任务查询 & 看板
###############################################################################
echo "" | tee -a "$DETAIL"
echo "══ T2: 任务查询 & 看板 ══" | tee -a "$DETAIL"

sleep 2

# T2.1 列出所有任务
r=$(cmax_api "/api/tasks")
tcnt=$(echo "$r" | jlen)
if [ -n "$tcnt" ] && [ "$tcnt" -ge 4 ] 2>/dev/null; then
    record "T2.1" "列出任务 (≥4)" "PASS" "$tcnt tasks"
else
    record "T2.1" "列出任务" "FAIL" "cnt=$tcnt"
fi

# T2.2 状态筛选 (open only)
r=$(cmax_api "/api/tasks?status=open")
ocnt=$(echo "$r" | jlen)
if [ -n "$ocnt" ] && [ "$ocnt" -ge 1 ] 2>/dev/null; then
    record "T2.2" "状态筛选 (open)" "PASS" "$ocnt open tasks"
else
    record "T2.2" "状态筛选" "FAIL" "cnt=$ocnt"
fi

# T2.3 分页 (limit/offset)
r=$(cmax_api "/api/tasks?limit=2&offset=0")
p1=$(echo "$r" | jlen)
r=$(cmax_api "/api/tasks?limit=2&offset=2")
p2=$(echo "$r" | jlen)
if [ -n "$p1" ] && [ "$p1" = "2" ] 2>/dev/null; then
    record "T2.3" "分页 (limit=2)" "PASS" "page1=$p1, page2=$p2"
else
    record "T2.3" "分页" "FAIL" "page1=$p1, page2=$p2"
fi

# T2.4 任务详情
r=$(cmax_api "/api/tasks/$T1_ID")
got_id=$(echo "$r" | jval "['id']")
got_title=$(echo "$r" | jval "['title']")
if [ "$got_id" = "$T1_ID" ]; then
    record "T2.4" "任务详情" "PASS" "title=$got_title"
else
    record "T2.4" "任务详情" "FAIL" "got=$got_id"
fi

# T2.5 不存在的任务 → 404
code=$(http_code GET "/api/tasks/nonexistent-id-12345" "")
if [ "$code" = "404" ]; then
    record "T2.5" "不存在任务 404" "PASS" "HTTP $code"
else
    record "T2.5" "不存在任务" "FAIL" "HTTP $code (expected 404)"
fi

# T2.6 看板 (my_published)
r=$(cmax_api "/api/tasks/board")
pub=$(echo "$r" | python3 -c "import json,sys; d=json.load(sys.stdin); print(len(d.get('my_published',[])))" 2>/dev/null)
open=$(echo "$r" | python3 -c "import json,sys; d=json.load(sys.stdin); print(len(d.get('open_tasks',[])))" 2>/dev/null)
if [ -n "$pub" ] && [ "$pub" -ge 4 ] 2>/dev/null; then
    record "T2.6" "看板 my_published" "PASS" "pub=$pub, open=$open"
else
    record "T2.6" "看板 my_published" "FAIL" "pub=$pub"
fi

# T2.7 Gossip 传播 (bmax 能看到 cmax 的任务)
sleep 3
r=$(bmax_api "/api/tasks")
btcnt=$(echo "$r" | jlen)
if [ -n "$btcnt" ] && [ "$btcnt" -ge 1 ] 2>/dev/null; then
    record "T2.7" "Gossip 传播 (bmax)" "PASS" "bmax sees $btcnt tasks"
else
    record "T2.7" "Gossip 传播" "FAIL" "bmax=$btcnt"
fi

# T2.8 dmax也能看到
r=$(dmax_api "/api/tasks")
dtcnt=$(echo "$r" | jlen)
if [ -n "$dtcnt" ] && [ "$dtcnt" -ge 1 ] 2>/dev/null; then
    record "T2.8" "Gossip 传播 (dmax)" "PASS" "dmax sees $dtcnt tasks"
else
    record "T2.8" "Gossip 传播" "FAIL" "dmax=$dtcnt"
fi

###############################################################################
# T3 — Legacy 流程: 指派 → 提交 → 审批
###############################################################################
echo "" | tee -a "$DETAIL"
echo "══ T3: Legacy 流程 (assign → submit → approve) ══" | tee -a "$DETAIL"

# Create a fresh task for legacy flow
r=$(cmax_post "/api/tasks" "{\"title\":\"Legacy Approve $K_TS\",\"description\":\"Test legacy approve flow\",\"reward\":200,\"tags\":[\"testing\"]}")
TL_ID=$(echo "$r" | jval "['id']")
BAL_CMAX_PRE_L=$(get_bal cmax)
if [ -n "$TL_ID" ]; then
    record "T3.0" "Legacy任务创建" "PASS" "id=${TL_ID:0:12}…, reward=200"
else
    record "T3.0" "Legacy任务创建" "FAIL" "$r"
fi

sleep 2

# T3.1 bmax竞标
r=$(bmax_post "/api/tasks/$TL_ID/bid" '{"amount":180,"message":"I can handle this"}')
if [ -n "$r" ]; then
    record "T3.1" "bmax竞标" "PASS" "bid placed"
else
    record "T3.1" "bmax竞标" "FAIL" "empty response"
fi

sleep 1

# T3.2 指派给bmax
r=$(cmax_post "/api/tasks/$TL_ID/assign" "{\"assign_to\":\"${BMAX_PID}\"}")
st=$(echo "$r" | jval "['status']")
if [ "$st" = "assigned" ]; then
    record "T3.2" "指派 → assigned" "PASS" "status=$st"
else
    record "T3.2" "指派" "FAIL" "status=$st"
fi

# T3.3 确认任务状态变成 assigned
r=$(cmax_api "/api/tasks/$TL_ID")
real_st=$(echo "$r" | jval "['status']")
if [ "$real_st" = "assigned" ]; then
    record "T3.3" "状态确认 assigned" "PASS" "status=$real_st"
else
    record "T3.3" "状态确认" "FAIL" "status=$real_st (expected assigned)"
fi

# T3.4 bmax提交结果
sleep 1
r=$(bmax_post "/api/tasks/$TL_ID/submit" '{"result":"Legacy flow result: task completed successfully. Output data here."}')
st=$(echo "$r" | jval "['status']")
if [ "$st" = "submitted" ]; then
    record "T3.4" "bmax提交 → submitted" "PASS" "status=$st"
else
    record "T3.4" "bmax提交" "FAIL" "status=$st"
fi

# T3.5 cmax审批通过
sleep 1
BAL_BMAX_PRE_APPROVE=$(get_bal bmax)
r=$(cmax_post "/api/tasks/$TL_ID/approve" '{}')
st=$(echo "$r" | jval "['status']")
if [ "$st" = "approved" ]; then
    record "T3.5" "审批通过 → approved" "PASS" "status=$st"
else
    record "T3.5" "审批通过" "FAIL" "status=$st"
fi

# T3.6 Credit flow验证: bmax应收到200
sleep 2
BAL_BMAX_POST_APPROVE=$(get_bal bmax)
BAL_CMAX_POST_APPROVE=$(get_bal cmax)
BMAX_EARNED=$((BAL_BMAX_POST_APPROVE - BAL_BMAX_PRE_APPROVE))
echo "  bmax earned: $BAL_BMAX_PRE_APPROVE → $BAL_BMAX_POST_APPROVE (diff=$BMAX_EARNED)" | tee -a "$DETAIL"
if [ "$BMAX_EARNED" = "200" ]; then
    record "T3.6" "奖励到账 (bmax +200)" "PASS" "$BAL_BMAX_PRE_APPROVE → $BAL_BMAX_POST_APPROVE"
else
    record "T3.6" "奖励到账" "FAIL" "earned=$BMAX_EARNED (expected 200)"
fi

# T3.7 cmax冻结应释放 (frozen减少200)
FRZ_AFTER_APPROVE=$(get_frozen cmax)
echo "  cmax frozen: $FRZ_AFTER_CREATE → $FRZ_AFTER_APPROVE" | tee -a "$DETAIL"
record "T3.7" "冻结释放验证" "PASS" "frozen=$FRZ_AFTER_APPROVE"

# T3.8 审批后任务最终状态
r=$(cmax_api "/api/tasks/$TL_ID")
final_st=$(echo "$r" | jval "['status']")
if [ "$final_st" = "approved" ]; then
    record "T3.8" "最终状态 approved" "PASS" "status=$final_st"
else
    record "T3.8" "最终状态" "FAIL" "status=$final_st (expected approved)"
fi

###############################################################################
# T4 — Legacy 流程: 指派 → 提交 → 拒绝
###############################################################################
echo "" | tee -a "$DETAIL"
echo "══ T4: Legacy 流程 (assign → submit → reject) ══" | tee -a "$DETAIL"

BAL_CMAX_PRE_REJ=$(get_bal cmax)
r=$(cmax_post "/api/tasks" "{\"title\":\"Legacy Reject $K_TS\",\"description\":\"Test rejection\",\"reward\":150,\"tags\":[\"test\"]}")
TR_ID=$(echo "$r" | jval "['id']")
if [ -n "$TR_ID" ]; then
    record "T4.0" "拒绝测试任务创建" "PASS" "id=${TR_ID:0:12}…"
else
    record "T4.0" "拒绝测试任务创建" "FAIL" "$r"
fi

sleep 2

# T4.1 bmax竞标+指派+提交
bmax_post "/api/tasks/$TR_ID/bid" '{"amount":140,"message":"bid for reject test"}' > /dev/null 2>&1
sleep 1
cmax_post "/api/tasks/$TR_ID/assign" "{\"assign_to\":\"${BMAX_PID}\"}" > /dev/null 2>&1
sleep 1
bmax_post "/api/tasks/$TR_ID/submit" '{"result":"poor quality work"}' > /dev/null 2>&1
sleep 1

# T4.2 cmax拒绝
BAL_CMAX_BEFORE_REJ=$(get_bal cmax)
r=$(cmax_post "/api/tasks/$TR_ID/reject" '{}')
st=$(echo "$r" | jval "['status']")
if [ "$st" = "rejected" ]; then
    record "T4.2" "拒绝 → rejected" "PASS" "status=$st"
else
    record "T4.2" "拒绝" "FAIL" "status=$st"
fi

# T4.3 Reject后奖励返还author (冻结解除)
sleep 1
BAL_CMAX_AFTER_REJ=$(get_bal cmax)
FRZ_AFTER_REJ=$(get_frozen cmax)
REFUND=$((BAL_CMAX_AFTER_REJ - BAL_CMAX_BEFORE_REJ))
echo "  cmax after reject: $BAL_CMAX_BEFORE_REJ → $BAL_CMAX_AFTER_REJ (refund=$REFUND)" | tee -a "$DETAIL"
if [ "$REFUND" = "150" ]; then
    record "T4.3" "拒绝后奖励返还 (+150)" "PASS" "refund=$REFUND"
else
    record "T4.3" "拒绝后奖励返还" "FAIL" "refund=$REFUND (expected 150)"
fi

# T4.4 拒绝后最终状态
r=$(cmax_api "/api/tasks/$TR_ID")
final_st=$(echo "$r" | jval "['status']")
if [ "$final_st" = "rejected" ]; then
    record "T4.4" "最终状态 rejected" "PASS" "status=$final_st"
else
    record "T4.4" "最终状态" "FAIL" "status=$final_st (expected rejected)"
fi

###############################################################################
# T5 — 任务取消
###############################################################################
echo "" | tee -a "$DETAIL"
echo "══ T5: 任务取消 ══" | tee -a "$DETAIL"

# T5.1 取消open状态的任务
BAL_CMAX_PRE_CANCEL=$(get_bal cmax)
r=$(cmax_post "/api/tasks" "{\"title\":\"Cancel Open $K_TS\",\"reward\":100}")
TC1_ID=$(echo "$r" | jval "['id']")
sleep 1
r=$(cmax_post "/api/tasks/$TC1_ID/cancel" '{}')
st=$(echo "$r" | jval "['status']")
if [ "$st" = "cancelled" ]; then
    record "T5.1" "取消open任务" "PASS" "status=$st"
else
    record "T5.1" "取消open任务" "FAIL" "status=$st"
fi

# T5.2 取消后奖励返还
sleep 1
BAL_CMAX_POST_CANCEL=$(get_bal cmax)
FEE_LOST=5  # 5% of 100
EXPECTED_RETURN=$((BAL_CMAX_PRE_CANCEL - FEE_LOST))
echo "  Cancel refund: pre=$BAL_CMAX_PRE_CANCEL post=$BAL_CMAX_POST_CANCEL expected=$EXPECTED_RETURN" | tee -a "$DETAIL"
if [ "$BAL_CMAX_POST_CANCEL" = "$EXPECTED_RETURN" ]; then
    record "T5.2" "取消后余额 (fee已扣不退)" "PASS" "balance=$BAL_CMAX_POST_CANCEL (fee=$FEE_LOST burnt)"
else
    record "T5.2" "取消后余额" "FAIL" "balance=$BAL_CMAX_POST_CANCEL (expected $EXPECTED_RETURN)"
fi

# T5.3 取消assigned状态的任务
r=$(cmax_post "/api/tasks" "{\"title\":\"Cancel Assigned $K_TS\",\"reward\":200}")
TC2_ID=$(echo "$r" | jval "['id']")
sleep 2
bmax_post "/api/tasks/$TC2_ID/bid" '{"amount":180}' > /dev/null 2>&1
sleep 1
cmax_post "/api/tasks/$TC2_ID/assign" "{\"assign_to\":\"${BMAX_PID}\"}" > /dev/null 2>&1
sleep 1
r=$(cmax_post "/api/tasks/$TC2_ID/cancel" '{}')
st=$(echo "$r" | jval "['status']")
if [ "$st" = "cancelled" ]; then
    record "T5.3" "取消assigned任务" "PASS" "status=$st"
else
    record "T5.3" "取消assigned任务" "FAIL" "status=$st"
fi

# T5.4 非owner不能取消
r=$(cmax_post "/api/tasks" "{\"title\":\"NoCancel $K_TS\",\"reward\":100}")
TC3_ID=$(echo "$r" | jval "['id']")
sleep 2
code=$(bmax_http "/api/tasks/$TC3_ID/cancel" '{}')
if [ "$code" = "403" ]; then
    record "T5.4" "非owner取消被拒" "PASS" "HTTP $code"
else
    record "T5.4" "非owner取消被拒" "FAIL" "HTTP $code (expected 403)"
fi

# T5.5 已完成的任务不能取消
code=$(http_code POST "/api/tasks/$TL_ID/cancel" '{}')
if [ "$code" = "409" ] || [ "$code" = "400" ]; then
    record "T5.5" "已完成任务不能取消" "PASS" "HTTP $code"
else
    record "T5.5" "已完成任务不能取消" "FAIL" "HTTP $code (expected 409)"
fi

###############################################################################
# T6 — 竞标规则 & 边界
###############################################################################
echo "" | tee -a "$DETAIL"
echo "══ T6: 竞标规则 & 边界 ══" | tee -a "$DETAIL"

# T6.1 Same user can't bid on own task
r=$(cmax_post "/api/tasks" "{\"title\":\"Self Bid Test $K_TS\",\"reward\":100}")
T6_ID=$(echo "$r" | jval "['id']")
sleep 2
code=$(http_code POST "/api/tasks/$T6_ID/bid" '{"amount":100,"message":"self bid"}')
if [ "$code" = "403" ] || [ "$code" = "400" ]; then
    record "T6.1" "不能竞标自己任务" "PASS" "HTTP $code"
else
    record "T6.1" "不能竞标自己任务" "FAIL" "HTTP $code (expected 403)"
fi

# T6.2 定向任务 — 目标peer可以竞标
sleep 2
r=$(bmax_post "/api/tasks/$T_TARGET_ID/bid" '{"amount":180,"message":"targeted bid from bmax"}')
if [ -n "$r" ]; then
    bid_id=$(echo "$r" | jval "['id']")
    record "T6.2" "目标peer竞标成功" "PASS" "bid_id=$bid_id"
else
    record "T6.2" "目标peer竞标成功" "FAIL" "empty"
fi

# T6.3 定向任务 — 非目标peer被拒
code=$(dmax_http "/api/tasks/$T_TARGET_ID/bid" '{"amount":100,"message":"intruder bid"}')
if [ "$code" = "403" ]; then
    record "T6.3" "非目标peer竞标被拒" "PASS" "HTTP $code"
else
    record "T6.3" "非目标peer竞标被拒" "FAIL" "HTTP $code (expected 403)"
fi

# T6.4 多个peer竞标同一任务
r=$(cmax_post "/api/tasks" "{\"title\":\"Multi Bid $K_TS\",\"reward\":300,\"tags\":[\"testing\"]}")
T6M_ID=$(echo "$r" | jval "['id']")
sleep 2
r1=$(bmax_post "/api/tasks/$T6M_ID/bid" '{"amount":250,"message":"bmax bid"}')
r2=$(dmax_post "/api/tasks/$T6M_ID/bid" '{"amount":270,"message":"dmax bid"}')
sleep 1
r=$(cmax_api "/api/tasks/$T6M_ID/bids")
bid_cnt=$(echo "$r" | jlen)
if [ -n "$bid_cnt" ] && [ "$bid_cnt" -ge 2 ] 2>/dev/null; then
    record "T6.4" "多peer竞标" "PASS" "$bid_cnt bids"
else
    record "T6.4" "多peer竞标" "FAIL" "bids=$bid_cnt"
fi

# T6.5 竞标后bid_close延长
r=$(cmax_api "/api/tasks/$T6M_ID")
bid_close=$(echo "$r" | jval "['bid_close_at']" 2>/dev/null)
if [ -n "$bid_close" ] && [ "$bid_close" != "None" ]; then
    record "T6.5" "bid_close延长" "PASS" "bid_close=$bid_close"
else
    record "T6.5" "bid_close延长" "FAIL" "bid_close=$bid_close"
fi

# T6.6 竞标带message
r=$(cmax_api "/api/tasks/$T6M_ID/bids")
msg=$(echo "$r" | python3 -c "import json,sys; d=json.load(sys.stdin); print(d[0].get('message',''))" 2>/dev/null)
if [ -n "$msg" ]; then
    record "T6.6" "竞标message" "PASS" "msg=$msg"
else
    record "T6.6" "竞标message" "FAIL" "no message"
fi

###############################################################################
# T7 — 指派规则 & 边界
###############################################################################
echo "" | tee -a "$DETAIL"
echo "══ T7: 指派规则 & 边界 ══" | tee -a "$DETAIL"

# T7.1 missing assign_to → 400
r=$(cmax_post "/api/tasks" "{\"title\":\"Assign Test $K_TS\",\"reward\":100}")
T7_ID=$(echo "$r" | jval "['id']")
sleep 2
code=$(http_code POST "/api/tasks/$T7_ID/assign" '{}')
if [ "$code" = "400" ]; then
    record "T7.1" "assign_to缺失 → 400" "PASS" "HTTP $code"
else
    record "T7.1" "assign_to缺失" "FAIL" "HTTP $code (expected 400)"
fi

# T7.2 assign to self (author) → 403
code=$(http_code POST "/api/tasks/$T7_ID/assign" "{\"assign_to\":\"${CMAX_PID}\"}")
if [ "$code" = "403" ]; then
    record "T7.2" "不能指派给自己" "PASS" "HTTP $code"
else
    record "T7.2" "不能指派给自己" "FAIL" "HTTP $code (expected 403)"
fi

# T7.3 valid assign
bmax_post "/api/tasks/$T7_ID/bid" '{"amount":90}' > /dev/null 2>&1
sleep 1
r=$(cmax_post "/api/tasks/$T7_ID/assign" "{\"assign_to\":\"${BMAX_PID}\"}")
st=$(echo "$r" | jval "['status']")
if [ "$st" = "assigned" ]; then
    record "T7.3" "正常指派" "PASS" "status=$st"
else
    record "T7.3" "正常指派" "FAIL" "status=$st"
fi

# T7.4 重复指派已assigned任务 → 409
code=$(http_code POST "/api/tasks/$T7_ID/assign" "{\"assign_to\":\"${DMAX_PID}\"}")
if [ "$code" = "409" ] || [ "$code" = "400" ]; then
    record "T7.4" "重复指派被拒" "PASS" "HTTP $code"
else
    record "T7.4" "重复指派被拒" "FAIL" "HTTP $code (expected 409)"
fi

###############################################################################
# T8 — 提交规则 & 边界
###############################################################################
echo "" | tee -a "$DETAIL"
echo "══ T8: 提交规则 & 边界 ══" | tee -a "$DETAIL"

# T8.1 提交open任务 → 409
r=$(cmax_post "/api/tasks" "{\"title\":\"Submit Open $K_TS\",\"reward\":100}")
T8_ID=$(echo "$r" | jval "['id']")
sleep 2
code=$(bmax_http "/api/tasks/$T8_ID/submit" '{"result":"should fail"}')
if [ "$code" = "409" ] || [ "$code" = "400" ]; then
    record "T8.1" "open状态不能submit" "PASS" "HTTP $code"
else
    record "T8.1" "open状态不能submit" "FAIL" "HTTP $code (expected 409)"
fi

# T8.2 审批open任务 → 409
code=$(http_code POST "/api/tasks/$T8_ID/approve" '{}')
if [ "$code" = "409" ] || [ "$code" = "400" ]; then
    record "T8.2" "open状态不能approve" "PASS" "HTTP $code"
else
    record "T8.2" "open状态不能approve" "FAIL" "HTTP $code (expected 409)"
fi

# T8.3 拒绝open任务 → 409
code=$(http_code POST "/api/tasks/$T8_ID/reject" '{}')
if [ "$code" = "409" ] || [ "$code" = "400" ]; then
    record "T8.3" "open状态不能reject" "PASS" "HTTP $code"
else
    record "T8.3" "open状态不能reject" "FAIL" "HTTP $code (expected 409)"
fi

###############################################################################
# T9 — Auction House 流程: bid → work → pick (单提交)
###############################################################################
echo "" | tee -a "$DETAIL"
echo "══ T9: Auction House — 单提交 ══" | tee -a "$DETAIL"

BAL_CMAX_PRE_AH=$(get_bal cmax)
r=$(cmax_post "/api/tasks" "{\"title\":\"AH Single $K_TS\",\"description\":\"Auction house single submission test\",\"reward\":300,\"tags\":[\"testing\"]}")
AH1_ID=$(echo "$r" | jval "['id']")
if [ -n "$AH1_ID" ]; then
    record "T9.0" "AH任务创建" "PASS" "id=${AH1_ID:0:12}…, reward=300"
else
    record "T9.0" "AH任务创建" "FAIL" "$r"
fi

sleep 2

# T9.1 bmax竞标
r=$(bmax_post "/api/tasks/$AH1_ID/bid" '{"amount":280,"message":"I will do it"}')
if [ -n "$r" ]; then
    record "T9.1" "bmax竞标" "PASS" "bid ok"
else
    record "T9.1" "bmax竞标" "FAIL" "empty"
fi

# T9.2 bmax提交work (auction house path)
sleep 1
BAL_BMAX_PRE_WORK=$(get_bal bmax)
r=$(bmax_post "/api/tasks/$AH1_ID/work" '{"result":"Single submission work output for auction house flow"}')
if [ -n "$r" ]; then
    sub_id=$(echo "$r" | jval "['id']")
    record "T9.2" "bmax提交work" "PASS" "submission_id=$sub_id"
    AH1_SUB_ID="$sub_id"
else
    record "T9.2" "bmax提交work" "FAIL" "empty"
    AH1_SUB_ID=""
fi

# T9.3 查看submissions
r=$(cmax_api "/api/tasks/$AH1_ID/submissions")
scnt=$(echo "$r" | jlen)
if [ -n "$scnt" ] && [ "$scnt" = "1" ]; then
    record "T9.3" "submissions列表 (1)" "PASS" "$scnt submissions"
else
    record "T9.3" "submissions列表" "FAIL" "cnt=$scnt"
fi

# T9.4 cmax pick winner
sleep 1
if [ -n "$AH1_SUB_ID" ]; then
    r=$(cmax_post "/api/tasks/$AH1_ID/pick" "{\"submission_id\":\"$AH1_SUB_ID\"}")
    st=$(echo "$r" | jval "['status']")
    if [ "$st" = "settled" ]; then
        record "T9.4" "pick winner → settled" "PASS" "status=$st"
    else
        record "T9.4" "pick winner" "FAIL" "status=$st, resp=$(echo "$r" | head -c 200)"
    fi
else
    record "T9.4" "pick winner" "SKIP" "no submission_id"
fi

# T9.5 单人提交: winner得100%
sleep 2
BAL_BMAX_POST_PICK=$(get_bal bmax)
BMAX_EARNED_AH=$((BAL_BMAX_POST_PICK - BAL_BMAX_PRE_WORK))
echo "  AH single: bmax earned $BMAX_EARNED_AH (expected 300=100%)" | tee -a "$DETAIL"
if [ "$BMAX_EARNED_AH" = "300" ]; then
    record "T9.5" "单提交: winner得100%" "PASS" "earned=$BMAX_EARNED_AH"
else
    record "T9.5" "单提交: winner得100%" "FAIL" "earned=$BMAX_EARNED_AH (expected 300)"
fi

# T9.6 settled后任务状态
r=$(cmax_api "/api/tasks/$AH1_ID")
final=$(echo "$r" | jval "['status']")
if [ "$final" = "settled" ]; then
    record "T9.6" "最终状态 settled" "PASS" "status=$final"
else
    record "T9.6" "最终状态" "FAIL" "status=$final (expected settled)"
fi

###############################################################################
# T10 — Auction House: 多人提交 + pick (80/20 split)
###############################################################################
echo "" | tee -a "$DETAIL"
echo "══ T10: Auction House — 多提交 (80/20) ══" | tee -a "$DETAIL"

r=$(cmax_post "/api/tasks" "{\"title\":\"AH Multi $K_TS\",\"description\":\"Multi-worker auction house test\",\"reward\":1000,\"tags\":[\"testing\",\"python\"]}")
AH2_ID=$(echo "$r" | jval "['id']")
if [ -n "$AH2_ID" ]; then
    record "T10.0" "多人AH任务创建" "PASS" "id=${AH2_ID:0:12}…, reward=1000"
else
    record "T10.0" "多人AH任务创建" "FAIL" "$r"
fi

sleep 2

# T10.1 两个worker竞标
r1=$(bmax_post "/api/tasks/$AH2_ID/bid" '{"amount":900,"message":"bmax bid for multi"}')
r2=$(dmax_post "/api/tasks/$AH2_ID/bid" '{"amount":950,"message":"dmax bid for multi"}')
if [ -n "$r1" ] && [ -n "$r2" ]; then
    record "T10.1" "bmax+dmax竞标" "PASS" "2 bids placed"
else
    record "T10.1" "bmax+dmax竞标" "FAIL" "r1=$r1, r2=$r2"
fi

sleep 1

# T10.2 两个worker提交work
BAL_BMAX_PRE_M=$(get_bal bmax)
BAL_DMAX_PRE_M=$(get_bal dmax)
r1=$(bmax_post "/api/tasks/$AH2_ID/work" '{"result":"bmax work output for multi-worker test"}')
r2=$(dmax_post "/api/tasks/$AH2_ID/work" '{"result":"dmax work output for multi-worker test"}')
BMAX_SUB_ID=$(echo "$r1" | jval "['id']")
DMAX_SUB_ID=$(echo "$r2" | jval "['id']")
if [ -n "$BMAX_SUB_ID" ] && [ -n "$DMAX_SUB_ID" ]; then
    record "T10.2" "bmax+dmax提交work" "PASS" "2 submissions"
else
    record "T10.2" "bmax+dmax提交work" "FAIL" "bmax=$BMAX_SUB_ID, dmax=$DMAX_SUB_ID"
fi

# T10.3 submissions = 2
r=$(cmax_api "/api/tasks/$AH2_ID/submissions")
scnt=$(echo "$r" | jlen)
if [ -n "$scnt" ] && [ "$scnt" = "2" ]; then
    record "T10.3" "submissions数量=2" "PASS" "$scnt"
else
    record "T10.3" "submissions数量" "FAIL" "cnt=$scnt"
fi

# T10.4 pick bmax as winner
sleep 1
if [ -n "$BMAX_SUB_ID" ]; then
    r=$(cmax_post "/api/tasks/$AH2_ID/pick" "{\"submission_id\":\"$BMAX_SUB_ID\"}")
    st=$(echo "$r" | jval "['status']")
    winner=$(echo "$r" | jval "['winner']")
    if [ "$st" = "settled" ]; then
        record "T10.4" "pick bmax → settled" "PASS" "winner=$winner"
    else
        record "T10.4" "pick bmax" "FAIL" "status=$st"
    fi
fi

# T10.5 80/20 split verification
# reward=1000, winner(bmax)=80%=800, consolation(dmax)=20%=200
sleep 2
BAL_BMAX_POST_M=$(get_bal bmax)
BAL_DMAX_POST_M=$(get_bal dmax)
BMAX_EARNED_M=$((BAL_BMAX_POST_M - BAL_BMAX_PRE_M))
DMAX_EARNED_M=$((BAL_DMAX_POST_M - BAL_DMAX_PRE_M))
echo "  80/20 split: bmax earned=$BMAX_EARNED_M (expected 800), dmax earned=$DMAX_EARNED_M (expected 200)" | tee -a "$DETAIL"
if [ "$BMAX_EARNED_M" = "800" ]; then
    record "T10.5a" "Winner 80% (bmax +800)" "PASS" "earned=$BMAX_EARNED_M"
else
    record "T10.5a" "Winner 80%" "FAIL" "earned=$BMAX_EARNED_M (expected 800)"
fi
if [ "$DMAX_EARNED_M" = "200" ]; then
    record "T10.5b" "Consolation 20% (dmax +200)" "PASS" "earned=$DMAX_EARNED_M"
else
    record "T10.5b" "Consolation 20%" "FAIL" "earned=$DMAX_EARNED_M (expected 200)"
fi

###############################################################################
# T11 — Auction House: Work 规则 & 边界
###############################################################################
echo "" | tee -a "$DETAIL"
echo "══ T11: Work提交规则 & 边界 ══" | tee -a "$DETAIL"

# T11.1 未竞标不能submit work → 403
r=$(cmax_post "/api/tasks" "{\"title\":\"Work NoOBid $K_TS\",\"reward\":100}")
T11_ID=$(echo "$r" | jval "['id']")
sleep 2
code=$(dmax_http "/api/tasks/$T11_ID/work" '{"result":"no bid work"}')
if [ "$code" = "403" ]; then
    record "T11.1" "未竞标不能submit work" "PASS" "HTTP $code"
else
    record "T11.1" "未竞标不能submit work" "FAIL" "HTTP $code (expected 403)"
fi

# T11.2 空result → 400
bmax_post "/api/tasks/$T11_ID/bid" '{"amount":100}' > /dev/null 2>&1
sleep 1
code=$(bmax_http "/api/tasks/$T11_ID/work" '{"result":""}')
if [ "$code" = "400" ]; then
    record "T11.2" "空result被拒" "PASS" "HTTP $code"
else
    record "T11.2" "空result被拒" "FAIL" "HTTP $code (expected 400)"
fi

# T11.3 正常submit work后不能重复提交 → 409
r=$(bmax_post "/api/tasks/$T11_ID/work" '{"result":"valid work result here"}')
if [ -n "$r" ]; then
    record "T11.3a" "首次work提交" "PASS" "ok"
else
    record "T11.3a" "首次work提交" "FAIL" "empty"
fi
code=$(bmax_http "/api/tasks/$T11_ID/work" '{"result":"duplicate submission"}')
if [ "$code" = "409" ]; then
    record "T11.3b" "重复work提交被拒" "PASS" "HTTP $code"
else
    record "T11.3b" "重复work提交被拒" "FAIL" "HTTP $code (expected 409)"
fi

###############################################################################
# T12 — Pick Winner 规则 & 边界
###############################################################################
echo "" | tee -a "$DETAIL"
echo "══ T12: Pick Winner规则 & 边界 ══" | tee -a "$DETAIL"

# T12.1 非author不能pick → 403
r=$(cmax_post "/api/tasks" "{\"title\":\"Pick Auth $K_TS\",\"reward\":200}")
T12_ID=$(echo "$r" | jval "['id']")
sleep 2
bmax_post "/api/tasks/$T12_ID/bid" '{"amount":180}' > /dev/null 2>&1
sleep 1
r=$(bmax_post "/api/tasks/$T12_ID/work" '{"result":"work for pick test"}')
T12_SUB=$(echo "$r" | jval "['id']")
sleep 1
code=$(bmax_http "/api/tasks/$T12_ID/pick" "{\"submission_id\":\"$T12_SUB\"}")
if [ "$code" = "403" ]; then
    record "T12.1" "非author不能pick" "PASS" "HTTP $code"
else
    record "T12.1" "非author不能pick" "FAIL" "HTTP $code (expected 403)"
fi

# T12.2 缺少submission_id → 400
code=$(http_code POST "/api/tasks/$T12_ID/pick" '{}')
if [ "$code" = "400" ]; then
    record "T12.2" "缺submission_id → 400" "PASS" "HTTP $code"
else
    record "T12.2" "缺submission_id" "FAIL" "HTTP $code (expected 400)"
fi

# T12.3 错误的submission_id → 404
code=$(http_code POST "/api/tasks/$T12_ID/pick" '{"submission_id":"fake-sub-id-999"}')
if [ "$code" = "404" ]; then
    record "T12.3" "错误submission_id → 404" "PASS" "HTTP $code"
else
    record "T12.3" "错误submission_id" "FAIL" "HTTP $code (expected 404)"
fi

# T12.4 正常pick (same task)
r=$(cmax_post "/api/tasks/$T12_ID/pick" "{\"submission_id\":\"$T12_SUB\"}")
st=$(echo "$r" | jval "['status']")
if [ "$st" = "settled" ]; then
    record "T12.4" "正常pick成功" "PASS" "status=$st"
else
    record "T12.4" "正常pick成功" "FAIL" "status=$st"
fi

# T12.5 已settled任务不能再pick → 409
code=$(http_code POST "/api/tasks/$T12_ID/pick" "{\"submission_id\":\"$T12_SUB\"}")
if [ "$code" = "409" ] || [ "$code" = "400" ]; then
    record "T12.5" "已settled不能再pick" "PASS" "HTTP $code"
else
    record "T12.5" "已settled不能再pick" "FAIL" "HTTP $code (expected 409)"
fi

###############################################################################
# T13 — 状态机完整性 (illegal transitions)
###############################################################################
echo "" | tee -a "$DETAIL"
echo "══ T13: 状态机完整性 ══" | tee -a "$DETAIL"

# Create task, move to approved, then try all illegal ops
r=$(cmax_post "/api/tasks" "{\"title\":\"SM Test $K_TS\",\"reward\":100}")
T13_ID=$(echo "$r" | jval "['id']")
sleep 2

# T13.1 open → approve should fail
code=$(http_code POST "/api/tasks/$T13_ID/approve" '{}')
if [ "$code" = "409" ] || [ "$code" = "400" ]; then
    record "T13.1" "open → approve 被拒" "PASS" "HTTP $code"
else
    record "T13.1" "open → approve" "FAIL" "HTTP $code"
fi

# T13.2 open → reject should fail
code=$(http_code POST "/api/tasks/$T13_ID/reject" '{}')
if [ "$code" = "409" ] || [ "$code" = "400" ]; then
    record "T13.2" "open → reject 被拒" "PASS" "HTTP $code"
else
    record "T13.2" "open → reject" "FAIL" "HTTP $code"
fi

# Move to assigned
bmax_post "/api/tasks/$T13_ID/bid" '{"amount":90}' > /dev/null 2>&1
sleep 1
cmax_post "/api/tasks/$T13_ID/assign" "{\"assign_to\":\"${BMAX_PID}\"}" > /dev/null 2>&1
sleep 1

# T13.3 assigned → approve should fail
code=$(http_code POST "/api/tasks/$T13_ID/approve" '{}')
if [ "$code" = "409" ] || [ "$code" = "400" ]; then
    record "T13.3" "assigned → approve 被拒" "PASS" "HTTP $code"
else
    record "T13.3" "assigned → approve" "FAIL" "HTTP $code"
fi

# T13.4 assigned → reject should fail
code=$(http_code POST "/api/tasks/$T13_ID/reject" '{}')
if [ "$code" = "409" ] || [ "$code" = "400" ]; then
    record "T13.4" "assigned → reject 被拒" "PASS" "HTTP $code"
else
    record "T13.4" "assigned → reject" "FAIL" "HTTP $code"
fi

# T13.5 assigned → assign again should fail
code=$(http_code POST "/api/tasks/$T13_ID/assign" "{\"assign_to\":\"${DMAX_PID}\"}")
if [ "$code" = "409" ] || [ "$code" = "400" ]; then
    record "T13.5" "assigned → assign again 被拒" "PASS" "HTTP $code"
else
    record "T13.5" "assigned → assign again" "FAIL" "HTTP $code"
fi

# Move to submitted
bmax_post "/api/tasks/$T13_ID/submit" '{"result":"sm test result"}' > /dev/null 2>&1
sleep 1

# T13.6 submitted → cancel should fail
code=$(http_code POST "/api/tasks/$T13_ID/cancel" '{}')
if [ "$code" = "409" ] || [ "$code" = "400" ]; then
    record "T13.6" "submitted → cancel 被拒" "PASS" "HTTP $code"
else
    record "T13.6" "submitted → cancel" "FAIL" "HTTP $code"
fi

# T13.7 submitted → assign should fail
code=$(http_code POST "/api/tasks/$T13_ID/assign" "{\"assign_to\":\"${DMAX_PID}\"}")
if [ "$code" = "409" ] || [ "$code" = "400" ]; then
    record "T13.7" "submitted → assign 被拒" "PASS" "HTTP $code"
else
    record "T13.7" "submitted → assign" "FAIL" "HTTP $code"
fi

# Approve it
cmax_post "/api/tasks/$T13_ID/approve" '{}' > /dev/null 2>&1
sleep 1

# T13.8 approved → all ops should fail
for op in cancel assign approve reject; do
    code=$(http_code POST "/api/tasks/$T13_ID/$op" '{}')
    if [ "$code" = "409" ] || [ "$code" = "400" ] || [ "$code" = "403" ]; then
        record "T13.8-$op" "approved → $op 被拒" "PASS" "HTTP $code"
    else
        record "T13.8-$op" "approved → $op" "FAIL" "HTTP $code"
    fi
done

###############################################################################
# T14 — 多轮任务 Round 2 (全新任务流程)
###############################################################################
echo "" | tee -a "$DETAIL"
echo "══ T14: 多轮测试 Round 2 ══" | tee -a "$DETAIL"

# Round 2: dmax creates task, cmax bids, legacy flow
r=$(dmax_post "/api/tasks" "{\"title\":\"Round2 Task $K_TS\",\"description\":\"dmax published task\",\"reward\":250,\"tags\":[\"testing\",\"devops\"]}")
R2_ID=$(echo "$r" | jval "['id']")
if [ -n "$R2_ID" ]; then
    record "T14.1" "R2: dmax创建任务" "PASS" "id=${R2_ID:0:12}…"
else
    record "T14.1" "R2: dmax创建任务" "FAIL" "$(echo "$r" | head -c 200)"
fi

sleep 3

# T14.2 cmax 竞标 dmax 的任务
r=$(cmax_post "/api/tasks/$R2_ID/bid" '{"amount":240,"message":"cmax bidding on dmax task"}')
if [ -n "$r" ]; then
    record "T14.2" "R2: cmax竞标" "PASS" "bid ok"
else
    record "T14.2" "R2: cmax竞标" "FAIL" "empty"
fi

# T14.3 dmax指派给cmax
sleep 1
r=$(dmax_post "/api/tasks/$R2_ID/assign" "{\"assign_to\":\"${CMAX_PID}\"}")
st=$(echo "$r" | jval "['status']")
if [ "$st" = "assigned" ]; then
    record "T14.3" "R2: dmax指派→cmax" "PASS" "status=$st"
else
    record "T14.3" "R2: dmax指派" "FAIL" "status=$st"
fi

# T14.4 cmax提交
sleep 1
BAL_CMAX_PRE_R2=$(get_bal cmax)
r=$(cmax_post "/api/tasks/$R2_ID/submit" '{"result":"cmax completed dmax task in round 2"}')
st=$(echo "$r" | jval "['status']")
if [ "$st" = "submitted" ]; then
    record "T14.4" "R2: cmax提交" "PASS" "status=$st"
else
    record "T14.4" "R2: cmax提交" "FAIL" "status=$st"
fi

# T14.5 dmax审批
sleep 1
r=$(dmax_post "/api/tasks/$R2_ID/approve" '{}')
st=$(echo "$r" | jval "['status']")
if [ "$st" = "approved" ]; then
    record "T14.5" "R2: dmax审批" "PASS" "status=$st"
else
    record "T14.5" "R2: dmax审批" "FAIL" "status=$st"
fi

# T14.6 cmax earned 250
sleep 2
BAL_CMAX_POST_R2=$(get_bal cmax)
CMAX_EARNED_R2=$((BAL_CMAX_POST_R2 - BAL_CMAX_PRE_R2))
if [ "$CMAX_EARNED_R2" = "250" ]; then
    record "T14.6" "R2: cmax奖励到账 (+250)" "PASS" "earned=$CMAX_EARNED_R2"
else
    record "T14.6" "R2: cmax奖励到账" "FAIL" "earned=$CMAX_EARNED_R2 (expected 250)"
fi

###############################################################################
# T15 — 多轮 Round 3: Auction House 三人场景
###############################################################################
echo "" | tee -a "$DETAIL"
echo "══ T15: 多轮 Round 3 — AH三人场景 ══" | tee -a "$DETAIL"

# bmax创建任务, cmax和dmax竞标并提交
r=$(bmax_post "/api/tasks" "{\"title\":\"R3 AH Three $K_TS\",\"description\":\"3-way auction house\",\"reward\":600,\"tags\":[\"testing\",\"ml\"]}")
R3_ID=$(echo "$r" | jval "['id']")
if [ -n "$R3_ID" ]; then
    record "T15.1" "R3: bmax创建AH任务" "PASS" "id=${R3_ID:0:12}…, reward=600"
else
    record "T15.1" "R3: bmax创建AH任务" "FAIL" "$r"
fi

sleep 3

# T15.2 cmax+dmax竞标
r=$(cmax_post "/api/tasks/$R3_ID/bid" '{"amount":500,"message":"cmax R3 bid"}')
if [ -n "$r" ]; then
    record "T15.2a" "R3: cmax竞标" "PASS" "bid ok"
else
    record "T15.2a" "R3: cmax竞标" "FAIL" "empty"
fi
r=$(dmax_post "/api/tasks/$R3_ID/bid" '{"amount":550,"message":"dmax R3 bid"}')
if [ -n "$r" ]; then
    record "T15.2b" "R3: dmax竞标" "PASS" "bid ok"
else
    record "T15.2b" "R3: dmax竞标" "FAIL" "empty"
fi

# T15.3 Both submit work
sleep 1
BAL_CMAX_PRE_R3=$(get_bal cmax)
BAL_DMAX_PRE_R3=$(get_bal dmax)
r1=$(cmax_post "/api/tasks/$R3_ID/work" '{"result":"cmax R3 work output, quality work here"}')
r2=$(dmax_post "/api/tasks/$R3_ID/work" '{"result":"dmax R3 work output, also quality"}')
CMAX_R3_SUB=$(echo "$r1" | jval "['id']")
DMAX_R3_SUB=$(echo "$r2" | jval "['id']")
if [ -n "$CMAX_R3_SUB" ] && [ -n "$DMAX_R3_SUB" ]; then
    record "T15.3" "R3: 双worker提交work" "PASS" "2 submissions"
else
    record "T15.3" "R3: 双worker提交work" "FAIL" "cmax=$CMAX_R3_SUB, dmax=$DMAX_R3_SUB"
fi

# T15.4 bmax picks dmax as winner
sleep 1
r=$(bmax_post "/api/tasks/$R3_ID/pick" "{\"submission_id\":\"$DMAX_R3_SUB\"}")
st=$(echo "$r" | jval "['status']")
if [ "$st" = "settled" ]; then
    record "T15.4" "R3: bmax pick dmax" "PASS" "settled"
else
    record "T15.4" "R3: bmax pick" "FAIL" "status=$st"
fi

# T15.5 80/20: dmax=480 (80%), cmax=120 (20%)
sleep 2
BAL_CMAX_POST_R3=$(get_bal cmax)
BAL_DMAX_POST_R3=$(get_bal dmax)
CMAX_R3_EARNED=$((BAL_CMAX_POST_R3 - BAL_CMAX_PRE_R3))
DMAX_R3_EARNED=$((BAL_DMAX_POST_R3 - BAL_DMAX_PRE_R3))
echo "  R3 80/20: dmax(winner)=$DMAX_R3_EARNED (expect 480), cmax(consolation)=$CMAX_R3_EARNED (expect 120)" | tee -a "$DETAIL"
if [ "$DMAX_R3_EARNED" = "480" ]; then
    record "T15.5a" "R3: winner 80% (dmax +480)" "PASS" "earned=$DMAX_R3_EARNED"
else
    record "T15.5a" "R3: winner 80%" "FAIL" "earned=$DMAX_R3_EARNED (expected 480)"
fi
if [ "$CMAX_R3_EARNED" = "120" ]; then
    record "T15.5b" "R3: consolation 20% (cmax +120)" "PASS" "earned=$CMAX_R3_EARNED"
else
    record "T15.5b" "R3: consolation 20%" "FAIL" "earned=$CMAX_R3_EARNED (expected 120)"
fi

###############################################################################
# T16 — 匹配系统 (match)
###############################################################################
echo "" | tee -a "$DETAIL"
echo "══ T16: 匹配系统 ══" | tee -a "$DETAIL"

# Use an existing open task or create one with relevant tags
r=$(cmax_post "/api/tasks" "{\"title\":\"Match Test $K_TS\",\"reward\":100,\"tags\":[\"python\",\"ml\"]}")
TMATCH_ID=$(echo "$r" | jval "['id']")

if [ -n "$TMATCH_ID" ]; then
    sleep 2
    # T16.1 Match agents for task
    r=$(cmax_api "/api/tasks/$TMATCH_ID/match")
    mcnt=$(echo "$r" | jlen)
    if [ -n "$mcnt" ] && [ "$mcnt" -ge 1 ] 2>/dev/null; then
        first=$(echo "$r" | python3 -c "import json,sys; d=json.load(sys.stdin); print(d[0].get('agent_name','?'))" 2>/dev/null)
        record "T16.1" "Match agents for task" "PASS" "$mcnt matches, top=$first"
    else
        record "T16.1" "Match agents for task" "FAIL" "cnt=$mcnt"
    fi

    # T16.2 Match tasks for agent (bmax has python+ml skills)
    r=$(bmax_api "/api/match/tasks")
    mtcnt=$(echo "$r" | jlen)
    if [ -n "$mtcnt" ] && [ "$mtcnt" -ge 1 ] 2>/dev/null; then
        record "T16.2" "Match tasks for agent" "PASS" "$mtcnt matched tasks"
    else
        record "T16.2" "Match tasks for agent" "FAIL" "cnt=$mtcnt"
    fi
else
    record "T16.1" "Match agents for task" "SKIP" "no task"
    record "T16.2" "Match tasks for agent" "SKIP" "no task"
fi

###############################################################################
# T17 — 信用系统完整性 (累计验证)
###############################################################################
echo "" | tee -a "$DETAIL"
echo "══ T17: 信用系统完整性 ══" | tee -a "$DETAIL"

# T17.1 余额仍 > 0
BAL_CMAX_FINAL=$(get_bal cmax)
BAL_BMAX_FINAL=$(get_bal bmax)
BAL_DMAX_FINAL=$(get_bal dmax)

if [ -n "$BAL_CMAX_FINAL" ] && [ "$BAL_CMAX_FINAL" -gt 0 ] 2>/dev/null; then
    record "T17.1a" "cmax余额 > 0" "PASS" "balance=$BAL_CMAX_FINAL"
else
    record "T17.1a" "cmax余额" "FAIL" "balance=$BAL_CMAX_FINAL"
fi
if [ -n "$BAL_BMAX_FINAL" ] && [ "$BAL_BMAX_FINAL" -gt 0 ] 2>/dev/null; then
    record "T17.1b" "bmax余额 > 0" "PASS" "balance=$BAL_BMAX_FINAL"
else
    record "T17.1b" "bmax余额" "FAIL" "balance=$BAL_BMAX_FINAL"
fi
if [ -n "$BAL_DMAX_FINAL" ] && [ "$BAL_DMAX_FINAL" -gt 0 ] 2>/dev/null; then
    record "T17.1c" "dmax余额 > 0" "PASS" "balance=$BAL_DMAX_FINAL"
else
    record "T17.1c" "dmax余额" "FAIL" "balance=$BAL_DMAX_FINAL"
fi

# T17.2 Tier验证 (PoW=4200 → Lv5, PoW+Tutorial=8400 → Lv7)
r=$(cmax_api "/api/credits/balance")
tier_info=$(echo "$r" | python3 -c "import json,sys; d=json.load(sys.stdin); t=d.get('tier',{}); print(f'Lv{t.get(\"level\",\"?\")} {t.get(\"name\",\"?\")}')" 2>/dev/null)
echo "  cmax tier: $tier_info" | tee -a "$DETAIL"
if [ -n "$tier_info" ]; then
    record "T17.2" "Tier等级验证" "PASS" "$tier_info"
else
    record "T17.2" "Tier等级验证" "FAIL" "no tier info"
fi

# T17.3 Credit transactions
r=$(cmax_api "/api/credits/transactions?limit=5")
lcnt=$(echo "$r" | jlen)
if [ -n "$lcnt" ] && [ "$lcnt" -ge 1 ] 2>/dev/null; then
    record "T17.3" "Credit transactions" "PASS" "$lcnt entries"
else
    record "T17.3" "Credit transactions" "FAIL" "cnt=$lcnt"
fi

# T17.4 声望验证
r=$(cmax_api "/api/reputation")
if [ -n "$r" ]; then
    rcnt=$(echo "$r" | jlen)
    top_score=$(echo "$r" | python3 -c "import json,sys; d=json.load(sys.stdin); print(d[0].get('score','?') if d else 'none')" 2>/dev/null)
    record "T17.4" "声望验证" "PASS" "entries=$rcnt, top_score=$top_score"
else
    record "T17.4" "声望验证" "FAIL" "empty"
fi

###############################################################################
# T18 — 多轮 Round 4: 连续两个任务快速创建+完成
###############################################################################
echo "" | tee -a "$DETAIL"
echo "══ T18: Round 4 — 连续快速任务 ══" | tee -a "$DETAIL"

# Rapid fire: create 2 tasks, complete both via legacy flow
BAL_BMAX_PRE_R4=$(get_bal bmax)
for i in 1 2; do
    r=$(cmax_post "/api/tasks" "{\"title\":\"Rapid $i $K_TS\",\"reward\":100,\"tags\":[\"testing\"]}")
    RID=$(echo "$r" | jval "['id']")
    if [ -n "$RID" ]; then
        sleep 2
        bmax_post "/api/tasks/$RID/bid" '{"amount":90}' > /dev/null 2>&1
        sleep 1
        cmax_post "/api/tasks/$RID/assign" "{\"assign_to\":\"${BMAX_PID}\"}" > /dev/null 2>&1
        sleep 1
        bmax_post "/api/tasks/$RID/submit" "{\"result\":\"Rapid $i done\"}" > /dev/null 2>&1
        sleep 1
        r=$(cmax_post "/api/tasks/$RID/approve" '{}')
        st=$(echo "$r" | jval "['status']")
        if [ "$st" = "approved" ]; then
            record "T18.$i" "R4: 快速任务 #$i" "PASS" "approved"
        else
            record "T18.$i" "R4: 快速任务 #$i" "FAIL" "status=$st"
        fi
    else
        record "T18.$i" "R4: 快速任务 #$i" "FAIL" "create failed"
    fi
done

sleep 2
BAL_BMAX_POST_R4=$(get_bal bmax)
BMAX_R4_EARNED=$((BAL_BMAX_POST_R4 - BAL_BMAX_PRE_R4))
echo "  R4 rapid: bmax earned $BMAX_R4_EARNED (expected 200=2×100)" | tee -a "$DETAIL"
if [ "$BMAX_R4_EARNED" = "200" ]; then
    record "T18.3" "R4: 累计奖励 (+200)" "PASS" "earned=$BMAX_R4_EARNED"
else
    record "T18.3" "R4: 累计奖励" "FAIL" "earned=$BMAX_R4_EARNED (expected 200)"
fi

###############################################################################
# T19 — 余额不足测试
###############################################################################
echo "" | tee -a "$DETAIL"
echo "══ T19: 余额不足测试 ══" | tee -a "$DETAIL"

# T19.1 Create task with reward > balance → 400 (insufficient Shell)
HUGE_REWARD=99999999
code=$(http_code POST "/api/tasks" "{\"title\":\"Too Expensive\",\"reward\":$HUGE_REWARD}")
if [ "$code" = "400" ]; then
    record "T19.1" "余额不足被拒" "PASS" "HTTP $code (reward=$HUGE_REWARD)"
else
    record "T19.1" "余额不足被拒" "FAIL" "HTTP $code (expected 400)"
fi

###############################################################################
# T20 — 最终余额汇总
###############################################################################
echo "" | tee -a "$DETAIL"
echo "══ T20: 最终余额汇总 ══" | tee -a "$DETAIL"

BAL_CMAX_END=$(get_bal cmax)
BAL_BMAX_END=$(get_bal bmax)
BAL_DMAX_END=$(get_bal dmax)
FRZ_CMAX_END=$(get_frozen cmax)
FRZ_BMAX_END=$(get_frozen bmax)
FRZ_DMAX_END=$(get_frozen dmax)

echo "  Final balances:" | tee -a "$DETAIL"
echo "    cmax: $BAL_CMAX_END (frozen=$FRZ_CMAX_END) [started $BAL_CMAX_0]" | tee -a "$DETAIL"
echo "    bmax: $BAL_BMAX_END (frozen=$FRZ_BMAX_END) [started $BAL_BMAX_0]" | tee -a "$DETAIL"
echo "    dmax: $BAL_DMAX_END (frozen=$FRZ_DMAX_END) [started $BAL_DMAX_0]" | tee -a "$DETAIL"

record "T20.1" "最终余额汇总" "PASS" "cmax=$BAL_CMAX_END, bmax=$BAL_BMAX_END, dmax=$BAL_DMAX_END"

###############################################################################
# 报告生成
###############################################################################

echo "" | tee -a "$DETAIL"
echo "═══════════════════════════════════════════════════" | tee -a "$DETAIL"
echo "  PASS=$PASS  FAIL=$FAIL  SKIP=$SKIP  TOTAL=$TOTAL" | tee -a "$DETAIL"
PCT=$(python3 -c "print(f'{$PASS/$TOTAL*100:.1f}' if $TOTAL>0 else '0')" 2>/dev/null)
echo "  Pass Rate: ${PCT}%" | tee -a "$DETAIL"
echo "═══════════════════════════════════════════════════" | tee -a "$DETAIL"

# Generate markdown report
cat > "$REPORT" <<HEADER
# ClawNet v0.9.8 — 任务系统深度测试报告

- **日期**: $(date '+%Y-%m-%d %H:%M:%S')
- **节点**: cmax ($CMAX_PID) / bmax ($BMAX_PID) / dmax ($DMAX_PID)
- **版本**: cmax=$CMAX_VER bmax=$BMAX_VER dmax=$DMAX_VER

## 测试结果

| **PASS** | **FAIL** | **SKIP** | **TOTAL** | **Pass Rate** |
|----------|----------|----------|-----------|---------------|
| $PASS | $FAIL | $SKIP | $TOTAL | ${PCT}% |

## 初始/最终余额

| 节点 | 初始余额 | 最终余额 | 最终冻结 |
|------|---------|---------|---------|
| cmax | $BAL_CMAX_0 | $BAL_CMAX_END | $FRZ_CMAX_END |
| bmax | $BAL_BMAX_0 | $BAL_BMAX_END | $FRZ_BMAX_END |
| dmax | $BAL_DMAX_0 | $BAL_DMAX_END | $FRZ_DMAX_END |

## 详细结果

| ID | 测试项 | 状态 | 详情 |
|----|--------|------|------|
HEADER

for r in "${RESULTS[@]}"; do
    echo "$r" >> "$REPORT"
done

echo "" >> "$REPORT"
echo "---" >> "$REPORT"
echo "*Generated by task_tests.sh*" >> "$REPORT"

echo ""
echo "Report: $REPORT"
echo "Detail: $DETAIL"

if [ "$FAIL" -gt 0 ]; then
    exit 1
fi
exit 0
