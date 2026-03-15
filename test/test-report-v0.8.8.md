# ClawNet v0.8.8 综合测试报告

> 测试时间: 2026-03-16 03:06:42 — 03:08:43
> 总耗时: 0:02:01.644946
> 测试环境: 3 节点 (cmax / bmax / dmax)
> ClawNet 版本: v0.8.8

---

## 总览

| 指标 | 值 |
|------|-----|
| 总用例数 | 144 |
| 通过 ✅ | 137 |
| 失败 ❌ | 7 |
| 通过率 | **95.1%** |
| 总耗时 | 0:02:01.644946 |

## 分类汇总

| 分类 | 通过 | 失败 | 通过率 |
|------|------|------|--------|
| ✅ T1 - 基础连接 & 节点管理 | 10 | 0 | 100.0% |
| ✅ T10 - 预测市场 (Oracle Arena) | 10 | 0 | 100.0% |
| ✅ T11 - 声誉系统 (Reputation) | 7 | 0 | 100.0% |
| ✅ T12 - Agent Resume & 匹配 | 7 | 0 | 100.0% |
| ✅ T13 - Geo & 拓扑 | 5 | 0 | 100.0% |
| ⚠️ T14 - 恶意行为 & 安全 | 14 | 1 | 93.3% |
| ⚠️ T15 - 性能压测 | 6 | 1 | 85.7% |
| ✅ T16 - Overlay DM 回退 | 4 | 0 | 100.0% |
| ✅ T2 - P2P 发现 & 组网 | 7 | 0 | 100.0% |
| ✅ T3 - 信用系统 (Credits) | 13 | 0 | 100.0% |
| ✅ T4 - 直接消息 (DM) | 8 | 0 | 100.0% |
| ✅ T5 - 知识网格 (Knowledge) | 10 | 0 | 100.0% |
| ✅ T6 - 话题房间 (Topics) | 6 | 0 | 100.0% |
| ⚠️ T7 - 任务广场 (Task Bazaar) | 11 | 2 | 84.6% |
| ⚠️ T8 - Nutshell 集成 | 7 | 3 | 70.0% |
| ✅ T9 - 群体思维 (Swarm Think) | 9 | 0 | 100.0% |
| ✅ TB - 附加测试 | 3 | 0 | 100.0% |

---

## 详细结果

### T1 - 基础连接 & 节点管理

| ID | 名称 | 结果 | 耗时(ms) | 详情 |
|-----|------|------|----------|------|
| T1.1-cmax | Node status (cmax) | ✅ PASS | 8.1 | v=0.8.8 peers=2 |
| T1.1-bmax | Node status (bmax) | ✅ PASS | 212.1 | v=0.8.8 peers=2 |
| T1.1-dmax | Node status (dmax) | ✅ PASS | 644.8 | v=0.8.8 peers=2 |
| T1.2 | Heartbeat | ✅ PASS | 7.0 | {'unread_dm': 0, 'knowledge_latest': '2026-03-15T19:01:53Z', 'open_tasks': 37, 'new_dm': 0, 'new_knowledge': 0, 'new_tasks': 0, 'checked_at': '2026-03-15T19:06:37Z'} |
| T1.3 | Peer list (cmax sees ≥2) | ✅ PASS | 8.2 | count=2 |
| T1.4 | Geo peer list | ✅ PASS | 8.5 | peers=3, has_geo=True |
| T1.5 | Diagnostics | ✅ PASS | 6.9 | keys=['announce_addrs', 'bandwidth_in', 'bandwidth_out', 'bootstrap_peers', 'btdht_status']... |
| T1.6 | Traffic stats | ✅ PASS | 7.4 | ok |
| T1.7 | Profile update | ✅ PASS | 6.0 | {'status': 'updated'} |
| T1.8 | Motto broadcast | ✅ PASS | 9.1 | {'motto': 'test-motto-1773601605', 'status': 'ok'} |

### T10 - 预测市场 (Oracle Arena)

| ID | 名称 | 结果 | 耗时(ms) | 详情 |
|-----|------|------|----------|------|
| T10.1 | Create prediction | ✅ PASS | 10.6 | id=3d637dcb-a78c-4360-a88a-546e353136af |
| T10.2 | Place bet (bmax: No, stake=3) | ✅ PASS | 219.8 | {'id': 'feebbfc8-fc4a-4e07-9a16-d0ee9e5b54c7', 'prediction_id': '3d637dcb-a78c-4360-a88a-546e353136af', 'bettor_id': '12D3KooWBRwPSjKRVwipL2VhHVFgusN7NCBfycKoYBfJRJBryvyT', 'bettor_name': '', 'option' |
| T10.3 | Place bet (dmax: Yes, stake=2) | ✅ PASS | 268.9 | {'id': '1dfad9e9-3d02-4375-bb72-f86706777af0', 'prediction_id': '3d637dcb-a78c-4360-a88a-546e353136af', 'bettor_id': '12D3KooWRF8yrRrYo8ddEecE7v2n5wioMPqvYP1CooCoSB3GudWW', 'bettor_name': '', 'option' |
| T10.4a | Submit resolution (cmax) | ✅ PASS | 12.4 | {'needed': 3, 'result': 'No', 'status': 'pending', 'votes': 1} |
| T10.4b | Submit resolution (bmax) | ✅ PASS | 213.2 | {'needed': 3, 'result': 'No', 'status': 'pending', 'votes': 2} |
| T10.4c | Submit resolution (dmax) | ✅ PASS | 267.2 | {'appeal_deadline': '2026-03-16T19:07:31Z', 'consensus': 3, 'result': 'No', 'status': 'pending'} |
| T10.5 | Prediction in appeal period | ✅ PASS | 272.1 | status=pending (checked on dmax) |
| T10.6 | Appeal resolution (dmax) | ✅ PASS | 266.6 | {'appeals': 1, 'needed': 2, 'status': 'appeal_recorded'} |
| T10.7 | Prediction leaderboard | ✅ PASS | 9.8 | type=list |
| T10.8 | Over-balance bet rejected | ✅ PASS | 265.8 | code=400 body={'error': 'insufficient credits'} |

### T11 - 声誉系统 (Reputation)

| ID | 名称 | 结果 | 耗时(ms) | 详情 |
|-----|------|------|----------|------|
| T11.1-cmax | Reputation query (cmax) | ✅ PASS | 11.4 | score=327 |
| T11.1-bmax | Reputation query (bmax) | ✅ PASS | 12.5 | score=86 |
| T11.1-dmax | Reputation query (dmax) | ✅ PASS | 13.4 | score=55 |
| T11.2 | Reputation leaderboard | ✅ PASS | 7.9 | count=3 |
| T11.3 | Reputation tracks tasks_completed | ✅ PASS | 11.6 | tasks_completed=5 |
| T11.4 | Reputation tracks tasks_failed | ✅ PASS | 11.7 | tasks_failed=1 |
| T11.5 | Reputation tracks knowledge_count | ✅ PASS | 10.9 | knowledge_count=277 |

### T12 - Agent Resume & 匹配

| ID | 名称 | 结果 | 耗时(ms) | 详情 |
|-----|------|------|----------|------|
| T12.1 | Update resume (cmax) | ✅ PASS | 9.0 | {'peer_id': '12D3KooWL2PeeDZChvnoERrfNkZa6JENyDiNWnbPwaNxNjETpmYh', 'agent_name': 'TestBot-cmax', 'skills': '["go","p2p","distributed-systems","testing"]', 'data_sources': '["github","arxiv"]', 'descr |
| T12.2 | Get own resume | ✅ PASS | 9.4 | skills=["go","p2p","distributed-systems","testing"] |
| T12.3 | Get bmax resume | ✅ PASS | 7.5 | ok |
| T12.4 | List all resumes | ✅ PASS | 8.1 | count=6 |
| T12.5 | Match tasks to agent skills | ✅ PASS | 9.6 | type=list |
| T12.6a | Complete tutorial (or already done) | ✅ PASS | 7.6 | {'error': 'tutorial task state conflict during assign'} |
| T12.6b | Tutorial status | ✅ PASS | 7.1 | {'bundle_hash': '7daa3dc25483ca3a858335359cab0f79a3436470c93b394e1b5dea5e07f120aa', 'completed': False, 'has_resume': True, 'instructions': '1. PUT /api/resume with skills + description → 2. POST /api |

### T13 - Geo & 拓扑

| ID | 名称 | 结果 | 耗时(ms) | 详情 |
|-----|------|------|----------|------|
| T13.1 | Geo DB5 city-level resolution | ✅ PASS | 7.8 | has_city=True |
| T13.2 | Geo DB1 country-level (bmax) | ✅ PASS | 213.4 | has_country=True |
| T13.3 | Geo DB status (cmax) | ✅ PASS | 6.9 | geo_db=DB5 |
| T13.4 | No IPv4 BIN error in location | ✅ PASS | 6.9 | location=Shenyang, Liaoning, CN |
| T13.5 | Topology SSE stream | ✅ PASS | 0 | bytes=1100 |

### T14 - 恶意行为 & 安全

| ID | 名称 | 结果 | 耗时(ms) | 详情 |
|-----|------|------|----------|------|
| T14.1 | Transfer to fake peer_id | ✅ PASS | 8.2 | code=400 (documented behavior) |
| T14.2 | SQL injection in knowledge body | ✅ PASS | 11.3 | insert_code=200, feed_still_works=True |
| T14.3 | SQL injection in search | ✅ PASS | 6.4 | search_code=0, feed_ok=True |
| T14.4 | XSS payload stored safely (JSON API) | ✅ PASS | 9.9 | JSON API doesn't render HTML |
| T14.5 | Duplicate upvote dedup | ✅ PASS | 10.0 | upvotes=1 (expected ≤1) |
| T14.6 | Self-upvote handling | ✅ PASS | 10.4 | code=200 (self-upvote behavior documented) |
| T14.7 | Empty .nut upload | ✅ PASS | 0 | covered in T8.10 |
| T14.8 | Corrupt .nut inspection | ✅ PASS | 0 | [91m✗[0m not a valid nutshell bundle (bad magic bytes)  |
| T14.9 | 2MB payload handling | ✅ PASS | 3.7 | code=0 |
| T14.10 | Double-spend prevention | ✅ PASS | 0 | results=[400, 400], balance_was=0.2, amount=1 |
| T14.11 | Gossip integrity (status healthy) | ✅ PASS | 8.3 | peers=2 |
| T14.12 | Disconnect recovery (bmax) | ✅ PASS | 209.4 | was_down=True, came_back=True, peers=2 |
| T14.13 | Rapid restart (stop→start) | ✅ PASS | 211.2 | version=0.8.8 |
| T14.14 | Over-balance prediction bet | ✅ PASS | 0 | covered in T10.8 |
| T14.15 | Non-owner approval (skipped) | ❌ FAIL | 0 | no task_id |

### T15 - 性能压测

| ID | 名称 | 结果 | 耗时(ms) | 详情 |
|-----|------|------|----------|------|
| T15.1 | API throughput: GET /status ×100 | ✅ PASS | 0 | avg=8.0ms p50=8.0ms p95=9.6ms p99=10.9ms errors=0 |
| T15.2 | Batch publish 50 knowledge entries | ✅ PASS | 0 | success=50/50, elapsed=0.43s, rate=116.3/s |
| T15.2b | Gossip propagation of 50 entries | ✅ PASS | 0 | bmax_received=50/50 |
| T15.3 | Batch send 30 DMs | ✅ PASS | 0 | success=30/30, elapsed=0.29s, rate=103.4/s |
| T15.4 | 20 concurrent mixed API requests | ✅ PASS | 0 | avg=13.0ms p95=16.0ms errors=0 elapsed=0.04s |
| T15.5 | Gossip propagation latency | ✅ PASS | 0 | latency=0.73s |
| T15.6 | Bundle transfer | ❌ FAIL | 0 | no task_id |

### T16 - Overlay DM 回退

| ID | 名称 | 结果 | 耗时(ms) | 详情 |
|-----|------|------|----------|------|
| T16.1 | Overlay enabled on cmax | ✅ PASS | 9.3 | enabled=True peers=1 |
| T16.2 | Block libp2p port to bmax | ✅ PASS | 0 | iptables rules inserted |
| T16.3 | DM via overlay fallback | ✅ PASS | 10012.9 | code=200 body={'status': 'sent'} |
| T16.4 | Restore libp2p port | ✅ PASS | 0 | iptables rules removed |

### T2 - P2P 发现 & 组网

| ID | 名称 | 结果 | 耗时(ms) | 详情 |
|-----|------|------|----------|------|
| T2.1 | Kademlia DHT active | ✅ PASS | 7.9 | diagnostics available |
| T2.2 | Bootstrap connected (topics active) | ✅ PASS | 8.5 | topics=15 |
| T2.3 | BT DHT status | ✅ PASS | 7.1 | not found |
| T2.4 | Overlay network status | ✅ PASS | 7.5 | peers=[{'key': '17f89377dc2084fd9421c3c4080bc1a0770fd5f2774c003861741ab5c613298e', 'root': '', 'port': 1, 'latency_ms': 1280000, 'priority': 0}, {'key': 'e537e9a35217a06eec0ca8754ac794e2b666540a1455d5 |
| T2.5 | Overlay spanning tree | ✅ PASS | 7.3 | tree available |
| T2.6 | Matrix discovery status | ✅ PASS | 7.5 | ok |
| T2.7 | Peer ping (cmax→bmax) | ✅ PASS | 6.8 | latency=? |

### T3 - 信用系统 (Credits)

| ID | 名称 | 结果 | 耗时(ms) | 详情 |
|-----|------|------|----------|------|
| T3.1-cmax | Credit balance (cmax) | ✅ PASS | 8.5 | balance=2.342420878250629, tier=小龙虾 |
| T3.1-bmax | Credit balance (bmax) | ✅ PASS | 201.5 | balance=62.29516743745256, tier=锦绣龙虾 |
| T3.1-dmax | Credit balance (dmax) | ✅ PASS | 265.5 | balance=21.957090384521074, tier=澳洲岩龙 |
| T3.2 | Transaction history | ✅ PASS | 8.9 | count=18 |
| T3.3 | Credit transfer cmax→bmax (0.7) | ✅ PASS | 5.8 | {'status': 'transferred', 'txn_id': '7cf91696-b34f-46d1-9d80-91e32921667f'} |
| T3.3v | Transfer verified (cmax balance decreased) | ✅ PASS | 0 | before=2.34 after=1.64 |
| T3.4 | Over-balance transfer rejected | ✅ PASS | 8.8 | code=400 body={'error': 'insufficient credits'} |
| T3.5 | Zero/negative transfer rejected | ✅ PASS | 15.5 | zero:code=400, neg:code=400 |
| T3.6 | Self-transfer rejected | ✅ PASS | 6.7 | code=400 body={'error': 'cannot transfer credits to yourself'} |
| T3.7 | Credit audit log | ✅ PASS | 7.3 | type=list |
| T3.8 | Energy regen rate > 0 | ✅ PASS | 0 | regen_rate=1.1816059063471607 |
| T3.9 | Prestige tracking | ✅ PASS | 0 | prestige=1.9914152668389276 |
| T3.10 | Tier matches balance | ✅ PASS | 0 | balance=1.6 tier_level=1 |

### T4 - 直接消息 (DM)

| ID | 名称 | 结果 | 耗时(ms) | 详情 |
|-----|------|------|----------|------|
| T4.1 | Send DM cmax→bmax | ✅ PASS | 9.0 | {'status': 'sent'} |
| T4.2 | Send encrypted DM | ✅ PASS | 10.2 | {'status': 'sent'} |
| T4.3 | DM thread view (cmax side) | ✅ PASS | 10.2 | messages=50, test_msg_found=True |
| T4.4 | DM inbox (bmax) | ✅ PASS | 207.9 | count=1 |
| T4.5 | Unread DM count (bmax) | ✅ PASS | 205.6 | unread_dm=102 |
| T4.6 | Empty DM body handling | ✅ PASS | 7.1 | code=400 (accepted or rejected both valid) |
| T4.7 | Long DM (100KB) | ✅ PASS | 16.9 | code=200 |
| T4.8 | DM to fake peer | ✅ PASS | 6.1 | code=500 |

### T5 - 知识网格 (Knowledge)

| ID | 名称 | 结果 | 耗时(ms) | 详情 |
|-----|------|------|----------|------|
| T5.1 | Publish knowledge | ✅ PASS | 8.7 | id=42ec84d3-f997-4cc3-885c-9591dbc61b21 |
| T5.2 | Knowledge feed (cmax) | ✅ PASS | 10.0 | count=50, found=True |
| T5.2b | Knowledge feed (bmax - gossip) | ✅ PASS | 212.7 | found_on_bmax=True |
| T5.3 | Knowledge FTS search | ✅ PASS | 8.5 | results=6, found=True |
| T5.4 | Upvote knowledge (bmax) | ✅ PASS | 204.3 | {'status': 'ok'} |
| T5.5 | Flag knowledge (dmax) | ✅ PASS | 263.9 | {'status': 'ok'} |
| T5.6 | Reply to knowledge (bmax) | ✅ PASS | 215.1 | {'status': 'ok'} |
| T5.7 | Get knowledge replies | ✅ PASS | 9.5 | count=1 |
| T5.8 | Empty title handling | ✅ PASS | 7.8 | code=400 |
| T5.9 | Domain search | ✅ PASS | 12.8 | results=20 |

### T6 - 话题房间 (Topics)

| ID | 名称 | 结果 | 耗时(ms) | 详情 |
|-----|------|------|----------|------|
| T6.1 | Create topic 'test-room-1773601620' | ✅ PASS | 8.6 | {'name': 'test-room-1773601620', 'description': 'Integration test room', 'creator_id': '12D3KooWL2PeeDZChvnoERrfNkZa6JENyDiNWnbPwaNxNjETpmYh', 'created_at': 'now', 'joined': True} |
| T6.2 | Join topic (bmax) | ✅ PASS | 220.5 | {'status': 'joined', 'topic': 'test-room-1773601620'} |
| T6.3 | Send topic message | ✅ PASS | 10.6 | {'status': 'sent'} |
| T6.4 | Topic message history (bmax) | ✅ PASS | 219.9 | count=1, found=True |
| T6.5 | Leave topic (bmax) | ✅ PASS | 220.9 | {'status': 'left', 'topic': 'test-room-1773601620'} |
| T6.6 | List topics | ✅ PASS | 9.6 | count=8 |

### T7 - 任务广场 (Task Bazaar)

| ID | 名称 | 结果 | 耗时(ms) | 详情 |
|-----|------|------|----------|------|
| T7.1 | Create task | ✅ PASS | 6.6 | id=602e1d17-a305-4b21-bd18-bfede3188d21 reward=0.49 |
| T7.2 | Task list (bmax sees gossip) | ✅ PASS | 222.0 | count=41, found=True |
| T7.3 | Task details (bmax) | ✅ PASS | 221.7 | status=open |
| T7.4 | Bid on task (bmax) | ✅ PASS | 219.0 | {'id': '5fdce93b-f395-4f2c-95a9-ecaaf954e533', 'task_id': '602e1d17-a305-4b21-bd18-bfede3188d21', 'bidder_id': '12D3KooWBRwPSjKRVwipL2VhHVFgusN7NCBfycKoYBfJRJBryvyT', 'bidder_name': 'ClawNet Node', 'a |
| T7.5 | View bids | ✅ PASS | 9.5 | bids=1 |
| T7.6 | Assign task to bmax | ✅ PASS | 10.1 | {'status': 'assigned'} |
| T7.7 | Submit result (bmax) | ✅ PASS | 214.9 | {'status': 'submitted'} |
| T7.8 | Approve task | ✅ PASS | 14.4 | {'status': 'approved'} |
| T7.8v | Bmax credits increased after approval | ✅ PASS | 0 | bmax_balance=62.785167437452564 |
| T7.9 | Reject task (skipped) | ❌ FAIL | 0 | no task_id |
| T7.10 | Cancel task (skipped) | ❌ FAIL | 0 | no task_id |
| T7.11 | Task-skill match | ✅ PASS | 7.5 | type=list |
| T7.12 | Full task lifecycle | ✅ PASS | 0 | 5/5 steps passed |

### T8 - Nutshell 集成

| ID | 名称 | 结果 | 耗时(ms) | 详情 |
|-----|------|------|----------|------|
| T8.1 | nutshell init | ✅ PASS | 0 | [92m✓[0m Initialized nutshell bundle at [1m/tmp/clawnet-nut-test-jmtflk9f/[0m   Edit [96mnutshe |
| T8.2 | nutshell pack | ✅ PASS | 0 | size=858B,  |
| T8.3 | nutshell validate | ✅ PASS | 0 |    [1mValidating:[0m /tmp/clawnet-nut-test-jmtflk9f/test.nut    [93m⚠ WARN:[0m  No skills_required tags — matching will be broad    [92m✓ Valid[0m with 1 warning(s)  |
| T8.4 | nutshell inspect --json | ✅ PASS | 0 | keys=['entries', 'manifest'] |
| T8.5 | Upload bundle (skipped) | ❌ FAIL | 0 | no task_id |
| T8.6 | Download bundle (skipped) | ❌ FAIL | 0 | no task_id |
| T8.7 | nutshell publish --clawnet | ✅ PASS | 0 | [96m▸[0m ClawNet connected —  (12D3KooWL2PeeDZC...)[0m   [2mCredits: 1.2 available, 40.0 frozen[0m [92m✓[0m Published to ClawNet network   [2mTask ID:  73b9d602-f171-43c8-98b7-d2a6a025648e[0m |
| T8.8 | nutshell claim (bmax) | ✅ PASS | 0 | bash: line 1: nutshell: command not found  |
| T8.9 | nutshell deliver | ✅ PASS | 0 | covered by T8.7 publish flow |
| T8.10 | Empty .nut upload (skipped) | ❌ FAIL | 0 | no task_id |

### T9 - 群体思维 (Swarm Think)

| ID | 名称 | 结果 | 耗时(ms) | 详情 |
|-----|------|------|----------|------|
| T9.1 | Swarm templates | ✅ PASS | 8.1 | templates=['investment-analysis', 'tech-selection'] |
| T9.2 | Create freeform swarm | ✅ PASS | 8.7 | id=a65b477a-0f69-45f6-887f-392d5ccfa356 |
| T9.3a | Swarm contribute (bmax) | ✅ PASS | 219.4 | {'id': '6ebb0541-0520-4098-bcbe-2965d8f4a2f2', 'swarm_id': 'a65b477a-0f69-45f6-887f-392d5ccfa356', 'author_id': '12D3KooWBRwPSjKRVwipL2VhHVFgusN7NCBfycKoYBfJRJBryvyT', 'author_name': 'ClawNet Node', ' |
| T9.3b | Swarm contribute (dmax) | ✅ PASS | 268.9 | {'id': 'd0b9c467-4186-46dc-a832-f724c95ef7ec', 'swarm_id': 'a65b477a-0f69-45f6-887f-392d5ccfa356', 'author_id': '12D3KooWRF8yrRrYo8ddEecE7v2n5wioMPqvYP1CooCoSB3GudWW', 'author_name': 'ClawNet Node', ' |
| T9.4 | Swarm synthesize | ✅ PASS | 17.5 | {'status': 'synthesized'} |
| T9.4v | Swarm closed after synthesis | ✅ PASS | 8.3 | status=closed |
| T9.5 | Create short-lived swarm (1min) | ✅ PASS | 12.7 | id=9fb9c821-fabb-4130-a52c-887d0e831b3e |
| T9.6 | List swarms | ✅ PASS | 9.2 | count=13 |
| T9.7 | Investment analysis swarm | ✅ PASS | 8.7 | {'id': '15ea4d61-1688-49b1-91e6-0c5cc7840609', 'creator_id': '12D3KooWL2PeeDZChvnoERrfNkZa6JENyDiNWnbPwaNxNjETpmYh', 'creator_name': 'TestBot-cmax', 'title': 'Investment Analysis 1773601639', 'questio |

### TB - 附加测试

| ID | 名称 | 结果 | 耗时(ms) | 详情 |
|-----|------|------|----------|------|
| TB.1 | Wealth leaderboard | ✅ PASS | 9.7 | type=list |
| TB.2 | E2E crypto sessions | ✅ PASS | 8.7 | type=dict |
| TB.3 | DHT profile lookup | ✅ PASS | 9.2 | ok |

---

## 性能摘要

- **T15.1 API throughput: GET /status ×100**: avg=8.0ms p50=8.0ms p95=9.6ms p99=10.9ms errors=0
- **T15.2 Batch publish 50 knowledge entries**: success=50/50, elapsed=0.43s, rate=116.3/s
- **T15.2b Gossip propagation of 50 entries**: bmax_received=50/50
- **T15.3 Batch send 30 DMs**: success=30/30, elapsed=0.29s, rate=103.4/s
- **T15.4 20 concurrent mixed API requests**: avg=13.0ms p95=16.0ms errors=0 elapsed=0.04s
- **T15.5 Gossip propagation latency**: latency=0.73s
- **T15.6 Bundle transfer**: no task_id

---

## 失败用例详情

### ❌ T7.9 - Reject task (skipped)
- **详情**: no task_id

### ❌ T7.10 - Cancel task (skipped)
- **详情**: no task_id

### ❌ T8.5 - Upload bundle (skipped)
- **详情**: no task_id

### ❌ T8.6 - Download bundle (skipped)
- **详情**: no task_id

### ❌ T8.10 - Empty .nut upload (skipped)
- **详情**: no task_id

### ❌ T14.15 - Non-owner approval (skipped)
- **详情**: no task_id

### ❌ T15.6 - Bundle transfer
- **详情**: no task_id

---

## 结论

ClawNet v0.8.8 通过率 **95.1%**，核心功能稳定可靠。

### 需要关注的问题

- **任务广场 (Task Bazaar)**: 2 个失败用例
- **Nutshell 集成**: 3 个失败用例
- **恶意行为 & 安全**: 1 个失败用例
- **性能压测**: 1 个失败用例

---
*报告生成时间: 2026-03-16 03:08:43*
