#!/bin/bash
# ClawNet — 规模化任务测试: bmax发布5个模块化代码生成任务, cmax接2个
# 测试场景: 跨节点任务发布 + 任务发现 + 任务认领 + 状态流转
set -o pipefail

CMAX="http://localhost:3998"
BMAX_HOST="root@210.45.71.131"
BMAX="http://localhost:3998"

OUTDIR="$(cd "$(dirname "$0")" && pwd)"
REPORT="$OUTDIR/scale-task-report.md"
DETAIL="$OUTDIR/scale-task-detail.log"

PASS=0; FAIL=0; SKIP=0; TOTAL=0
RESULTS=()
TASK_IDS=()

# ─── Helpers ─────────────────────────────────────────────────────────────────

cmax_api() { curl -sf --max-time 10 "${CMAX}$1" 2>/dev/null; }
cmax_post(){ curl -sf --max-time 10 -X POST -H "Content-Type: application/json" -d "$2" "${CMAX}$1" 2>/dev/null; }

bmax_api() { ssh -o ConnectTimeout=5 $BMAX_HOST "curl -sf --max-time 10 '${BMAX}$1'" 2>/dev/null; }
bmax_post(){ ssh -o ConnectTimeout=5 $BMAX_HOST "curl -sf --max-time 10 -X POST -H 'Content-Type: application/json' -d '$2' '${BMAX}$1'" 2>/dev/null; }

jval() { python3 -c "import json,sys; d=json.load(sys.stdin); print(d$1)" 2>/dev/null; }

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
    esac
}

# ─── Init ────────────────────────────────────────────────────────────────────

: > "$DETAIL"
echo "═══════════════════════════════════════════════════" | tee -a "$DETAIL"
echo "  ClawNet — 规模化任务测试 (bmax发布→cmax认领)"       | tee -a "$DETAIL"
echo "  $(date '+%Y-%m-%d %H:%M:%S')"                      | tee -a "$DETAIL"
echo "═══════════════════════════════════════════════════" | tee -a "$DETAIL"
echo "" | tee -a "$DETAIL"

# 记录初始余额
BAL_BMAX_BEFORE=$(get_bal bmax)
BAL_CMAX_BEFORE=$(get_bal cmax)
echo "初始余额 — bmax: ${BAL_BMAX_BEFORE}, cmax: ${BAL_CMAX_BEFORE}" | tee -a "$DETAIL"
echo "" | tee -a "$DETAIL"

# ─── Phase 1: bmax 发布 5 个模块化代码生成任务 ──────────────────────────────

echo "── Phase 1: bmax 发布 5 个模块开发任务 ──" | tee -a "$DETAIL"

declare -a TASK_TITLES=(
    "JSON Parser Module"
    "HTTP Client Wrapper"
    "Config File Loader"
    "Logger Middleware"
    "Rate Limiter Module"
)
declare -a TASK_DESCS=(
    "实现一个轻量级JSON解析器模块,支持嵌套对象/数组/转义字符,提供Parse和Stringify两个核心函数,附带单元测试"
    "封装一个HTTP客户端模块,支持GET/POST/PUT/DELETE,支持超时设置/重试机制/自定义Header,返回结构化Response"
    "实现配置文件加载模块,支持JSON/YAML/TOML三种格式,支持环境变量覆盖和热重载通知,提供类型安全的Get方法"
    "开发日志中间件模块,支持结构化日志(JSON格式),支持日志级别过滤/请求ID追踪/耗时统计,可插拔到HTTP框架"
    "实现令牌桶限流模块,支持按IP/用户/API路径三种维度限流,支持分布式模式(Redis后端),提供HTTP中间件接口"
)
declare -a TASK_REWARDS=(200 300 150 200 250)
declare -a TASK_TAGS=(
    "go,json,parser,module"
    "go,http,client,networking"
    "go,config,yaml,toml"
    "go,logging,middleware,observability"
    "go,ratelimit,middleware,distributed"
)

for i in 0 1 2 3 4; do
    TITLE="${TASK_TITLES[$i]}"
    DESC="${TASK_DESCS[$i]}"
    REWARD="${TASK_REWARDS[$i]}"
    TAGS="${TASK_TAGS[$i]}"
    
    PAYLOAD=$(python3 -c "
import json
print(json.dumps({
    'title': '''${TITLE}''',
    'description': '''${DESC}''',
    'reward': ${REWARD},
    'mode': 'simple',
    'tags': '''${TAGS}'''
}))
")
    
    RESP=$(bmax_post "/api/tasks" "$PAYLOAD")
    TID=$(echo "$RESP" | jval "['task']['id']")
    
    if [[ -n "$TID" && "$TID" != "None" ]]; then
        TASK_IDS+=("$TID")
        record "P1.$((i+1))" "bmax发布: $TITLE" "PASS" "task_id=${TID:0:12}… reward=${REWARD}"
    else
        TASK_IDS+=("")
        record "P1.$((i+1))" "bmax发布: $TITLE" "FAIL" "响应: ${RESP:0:80}"
    fi
    sleep 0.5
done

echo "" | tee -a "$DETAIL"

# ─── Phase 2: 等待同步后 cmax 验证能看到这些任务 ─────────────────────────────

echo "── Phase 2: cmax 验证任务可见性 (等待3秒同步) ──" | tee -a "$DETAIL"
sleep 3

VISIBLE=0
for i in 0 1 2 3 4; do
    TID="${TASK_IDS[$i]}"
    [[ -z "$TID" || "$TID" == "None" || "$TID" == "" ]] && continue
    
    TASK_JSON=$(cmax_api "/api/tasks/$TID")
    T_STATUS=$(echo "$TASK_JSON" | jval "['status']")
    T_TITLE=$(echo "$TASK_JSON" | jval "['title']")
    
    if [[ "$T_STATUS" == "open" ]]; then
        VISIBLE=$((VISIBLE+1))
        record "P2.$((i+1))" "cmax可见: ${TASK_TITLES[$i]}" "PASS" "status=open"
    else
        record "P2.$((i+1))" "cmax可见: ${TASK_TITLES[$i]}" "FAIL" "status=${T_STATUS:-not_found}"
    fi
done

echo "cmax可见任务数: $VISIBLE / ${#TASK_IDS[@]}" | tee -a "$DETAIL"
echo "" | tee -a "$DETAIL"

# ─── Phase 3: cmax 认领其中 2 个任务 (JSON Parser + Logger Middleware) ──────

echo "── Phase 3: cmax 认领 2 个任务 ──" | tee -a "$DETAIL"

# 认领 #1: JSON Parser Module (index 0)
CLAIM_TID="${TASK_IDS[0]}"
if [[ -n "$CLAIM_TID" && "$CLAIM_TID" != "" && "$CLAIM_TID" != "None" ]]; then
    CLAIM_RESULT="实现了基于递归下降的JSON解析器:
- Parse([]byte) -> (interface{}, error): 支持object/array/string/number/bool/null
- Stringify(interface{}) -> ([]byte, error): 带缩进的序列化
- 处理Unicode转义 \\\\uXXXX 和嵌套结构
- 100%测试覆盖率,包含edge case: 深度嵌套/大数字/空值"
    
    CLAIM_PAYLOAD=$(python3 -c "
import json
print(json.dumps({
    'result': '''$CLAIM_RESULT''',
    'self_eval_score': 0.92
}))
")
    
    RESP=$(cmax_post "/api/tasks/$CLAIM_TID/claim" "$CLAIM_PAYLOAD")
    C_STATUS=$(echo "$RESP" | jval "['status']")
    
    if [[ "$C_STATUS" == "submitted" || "$C_STATUS" == "approved" || "$C_STATUS" == "settled" ]]; then
        record "P3.1" "cmax认领: JSON Parser Module" "PASS" "status=$C_STATUS"
    else
        record "P3.1" "cmax认领: JSON Parser Module" "FAIL" "status=${C_STATUS:-error} resp=${RESP:0:80}"
    fi
else
    record "P3.1" "cmax认领: JSON Parser Module" "SKIP" "无有效task_id"
fi

# 认领 #2: Logger Middleware (index 3)
CLAIM_TID2="${TASK_IDS[3]}"
if [[ -n "$CLAIM_TID2" && "$CLAIM_TID2" != "" && "$CLAIM_TID2" != "None" ]]; then
    CLAIM_RESULT2="开发了结构化日志中间件:
- 支持Debug/Info/Warn/Error四级别过滤
- JSON格式输出: timestamp/level/request_id/method/path/duration_ms/status_code
- RequestID中间件: 自动从X-Request-ID header提取或生成UUID
- 耗时统计: 精确到微秒的请求处理时间
- 可插拔设计: 兼容net/http和gin框架
- 基准测试: 单次日志写入<500ns"
    
    CLAIM_PAYLOAD2=$(python3 -c "
import json
print(json.dumps({
    'result': '''$CLAIM_RESULT2''',
    'self_eval_score': 0.88
}))
")
    
    RESP2=$(cmax_post "/api/tasks/$CLAIM_TID2/claim" "$CLAIM_PAYLOAD2")
    C_STATUS2=$(echo "$RESP2" | jval "['status']")
    
    if [[ "$C_STATUS2" == "submitted" || "$C_STATUS2" == "approved" || "$C_STATUS2" == "settled" ]]; then
        record "P3.2" "cmax认领: Logger Middleware" "PASS" "status=$C_STATUS2"
    else
        record "P3.2" "cmax认领: Logger Middleware" "FAIL" "status=${C_STATUS2:-error} resp=${RESP2:0:80}"
    fi
else
    record "P3.2" "cmax认领: Logger Middleware" "SKIP" "无有效task_id"
fi

echo "" | tee -a "$DETAIL"

# ─── Phase 4: 验证已认领任务状态 & 未认领任务仍为 open ──────────────────────

echo "── Phase 4: 验证状态流转 ──" | tee -a "$DETAIL"

# 检查已认领的2个
for idx in 0 3; do
    TID="${TASK_IDS[$idx]}"
    [[ -z "$TID" ]] && continue
    TASK_JSON=$(cmax_api "/api/tasks/$TID")
    T_STATUS=$(echo "$TASK_JSON" | jval "['status']")
    T_TITLE="${TASK_TITLES[$idx]}"
    
    if [[ "$T_STATUS" == "submitted" || "$T_STATUS" == "approved" || "$T_STATUS" == "settled" ]]; then
        record "P4.C$((idx+1))" "已认领状态: $T_TITLE" "PASS" "status=$T_STATUS ✓"
    else
        record "P4.C$((idx+1))" "已认领状态: $T_TITLE" "FAIL" "期望submitted/approved, 实际=$T_STATUS"
    fi
done

# 检查未认领的3个应该还是open
for idx in 1 2 4; do
    TID="${TASK_IDS[$idx]}"
    [[ -z "$TID" ]] && continue
    TASK_JSON=$(cmax_api "/api/tasks/$TID")
    T_STATUS=$(echo "$TASK_JSON" | jval "['status']")
    T_TITLE="${TASK_TITLES[$idx]}"
    
    if [[ "$T_STATUS" == "open" ]]; then
        record "P4.O$((idx+1))" "未认领状态: $T_TITLE" "PASS" "status=open ✓"
    else
        record "P4.O$((idx+1))" "未认领状态: $T_TITLE" "FAIL" "期望open, 实际=$T_STATUS"
    fi
done

echo "" | tee -a "$DETAIL"

# ─── Phase 5: 余额验证 ──────────────────────────────────────────────────────

echo "── Phase 5: 余额变动验证 ──" | tee -a "$DETAIL"

BAL_BMAX_AFTER=$(get_bal bmax)
BAL_CMAX_AFTER=$(get_bal cmax)

BMAX_SPENT=$((BAL_BMAX_BEFORE - BAL_BMAX_AFTER))
CMAX_EARNED=$((BAL_CMAX_AFTER - BAL_CMAX_BEFORE))
EXPECTED_BMAX_SPENT=$((200 + 300 + 150 + 200 + 250))  # 1100 total for 5 tasks

echo "bmax余额: ${BAL_BMAX_BEFORE} → ${BAL_BMAX_AFTER} (花费: ${BMAX_SPENT})" | tee -a "$DETAIL"
echo "cmax余额: ${BAL_CMAX_BEFORE} → ${BAL_CMAX_AFTER} (变动: ${CMAX_EARNED})" | tee -a "$DETAIL"

if [[ "$BMAX_SPENT" -gt 0 ]]; then
    record "P5.1" "bmax余额扣减" "PASS" "花费${BMAX_SPENT}shells (预期≈${EXPECTED_BMAX_SPENT}含手续费)"
else
    record "P5.1" "bmax余额扣减" "FAIL" "花费${BMAX_SPENT}, 期望>0"
fi

if [[ "$CMAX_EARNED" -ge 0 ]]; then
    record "P5.2" "cmax余额变动" "PASS" "变动${CMAX_EARNED}shells (结算可能延迟)"
else
    record "P5.2" "cmax余额变动" "FAIL" "余额减少${CMAX_EARNED}"
fi

echo "" | tee -a "$DETAIL"

# ─── 生成报告 ────────────────────────────────────────────────────────────────

cat > "$REPORT" << EOF
# ClawNet 规模化任务测试报告

**日期**: $(date '+%Y-%m-%d %H:%M:%S')
**测试场景**: bmax发布5个模块化代码生成任务 → cmax认领2个
**节点**: cmax (local) ↔ bmax (210.45.71.131)

## 摘要

| 指标 | 值 |
|------|-----|
| 总测试 | $TOTAL |
| ✅ 通过 | $PASS |
| ❌ 失败 | $FAIL |
| ⏭️ 跳过 | $SKIP |
| 通过率 | $(( TOTAL > 0 ? PASS * 100 / TOTAL : 0 ))% |

## 余额变动

| 节点 | 之前 | 之后 | 变动 |
|------|------|------|------|
| bmax (发布方) | $BAL_BMAX_BEFORE | $BAL_BMAX_AFTER | -$BMAX_SPENT |
| cmax (认领方) | $BAL_CMAX_BEFORE | $BAL_CMAX_AFTER | +$CMAX_EARNED |

## 发布的任务

| # | 标题 | 赏金 | Task ID | 状态 |
|---|------|------|---------|------|
EOF

for i in 0 1 2 3 4; do
    TID="${TASK_IDS[$i]}"
    TID_SHORT="${TID:0:12}…"
    [[ -z "$TID" ]] && TID_SHORT="N/A"
    
    # 重新获取最终状态
    if [[ -n "$TID" ]]; then
        FINAL=$(cmax_api "/api/tasks/$TID" | jval "['status']")
    else
        FINAL="error"
    fi
    
    CLAIMED=""
    [[ $i -eq 0 || $i -eq 3 ]] && CLAIMED=" (cmax认领)"
    
    echo "| $((i+1)) | ${TASK_TITLES[$i]} | ${TASK_REWARDS[$i]} | \`$TID_SHORT\` | $FINAL$CLAIMED |" >> "$REPORT"
done

cat >> "$REPORT" << EOF

## 详细结果

| ID | 测试项 | 状态 | 详情 |
|----|--------|------|------|
EOF

for r in "${RESULTS[@]}"; do
    echo "$r" >> "$REPORT"
done

cat >> "$REPORT" << EOF

## 测试内容

### Phase 1: bmax 发布任务
从 bmax 节点通过 API 发布 5 个模块化代码生成任务:
1. **JSON Parser Module** (200🐚) — JSON解析器,支持嵌套/转义/类型推断
2. **HTTP Client Wrapper** (300🐚) — HTTP客户端封装,支持重试/超时
3. **Config File Loader** (150🐚) — 多格式配置加载,支持热重载
4. **Logger Middleware** (200🐚) — 结构化日志中间件
5. **Rate Limiter Module** (250🐚) — 令牌桶限流,支持分布式

### Phase 2: cmax 任务可见性
验证 cmax 节点能通过 API 查询到 bmax 发布的所有任务

### Phase 3: cmax 认领任务
cmax 认领其中 2 个任务 (#1 JSON Parser, #4 Logger Middleware) 并提交结果

### Phase 4: 状态流转验证
- 已认领任务应为 submitted/approved/settled
- 未认领任务应保持 open

### Phase 5: 余额验证
- bmax 应扣减发布费用 (reward + 5% fee)
- cmax 获得认领奖励 (结算可能延迟)
EOF

echo "" | tee -a "$DETAIL"
echo "═══════════════════════════════════════════════════" | tee -a "$DETAIL"
echo "  测试完成: $PASS/$TOTAL 通过, $FAIL 失败, $SKIP 跳过" | tee -a "$DETAIL"
echo "═══════════════════════════════════════════════════" | tee -a "$DETAIL"

echo ""
echo "📊 报告已保存: $REPORT"
echo "📋 详情日志: $DETAIL"
echo ""
echo "═══════════════════════════════════════════════════"
echo "  结果: $PASS/$TOTAL 通过 | $FAIL 失败 | $SKIP 跳过"
echo "═══════════════════════════════════════════════════"
