# ClawNet × overlay mesh 集成方案：「蜕壳」

> 版本: v0.1 draft · 2026-03-16

---

## 一、背景

ClawNet 已通过 Ironwood 库（与 overlay mesh 完全相同的 commit `7017dbc41d8e`）实现了 overlay 层，当前连接 36+ 个 overlay mesh 公开节点，参与全球 ~4000 节点的 mesh 路由。但目前只使用了 **datagram 能力**（DM 后备、gossip 广播），没有启用 TUN 设备和 IPv6 地址。

「蜕壳」是 ClawNet 全面拥抱 overlay mesh 的渐进式功能——保留原有网络方案，同时新增 overlay mesh TUN + IPv6 能力，用户主动激活后方可使用。

---

## 二、引入优势

### 2.1 NAT 穿透（最大价值）
- 只需一条出站 TCP 到任意公开节点，即可被全球 ~4000 节点反向连接
- 不需要公网 IP、不需要端口映射
- 对教育网、运营商 NAT、双重 NAT 环境的 ClawNet 节点至关重要

### 2.2 overlay mesh IPv6 身份
- 每个节点获得一个由 Ed25519 公钥推导的 `200::/7` IPv6 地址
- 地址 = SHA-512 变换(公钥)，无需注册或分配
- 公钥已有（与 libp2p Peer ID 同源），激活即可推导
- 例：cmax 公钥 `97a983...` → 地址 `200:ac69:adb:f1e8:558a:dc4c:568f:e2f0`

### 2.3 libp2p 增强
- `detectoverlay meshAddrs()` 会自动发现 TUN 上的 200:: 地址
- 宣布 `/ip6/200:x:x:x/tcp/4001` 到 DHT
- 所有在 overlay mesh 网络上的 ClawNet 节点可以直接互连（绕过公网 NAT）
- QUIC + TCP 双栈可用

### 2.4 抗审查与隐私
- 所有流量 E2E 加密（Ed25519 + X25519 + ChaCha20-Poly1305）
- 中间转发节点无法读取内容
- 多跳路由 → 源地址对中间节点不可见
- 分布式拓扑 → 无单点可封锁

### 2.5 推广 ClawNet
- ~4000 overlay mesh 活跃用户是天然潜在用户群
- overlay mesh 社区看重去中心化和 mesh 理念，与 ClawNet 高度契合
- 可通过 NodeInfo 字段广播 ClawNet 信息

### 2.6 零成本
- 无需注册、无需付费、无需审批
- 身份 = 密钥（ClawNet 已有）
- 代码依赖已具备（ironwood 同版本）

---

## 三、风险评估

### 3.1 🔴 高风险：系统服务暴露

**TUN 设备会在系统中创建真实网卡，内核路由表新增 `200::/7 → claw0`。**

任何监听 `[::]`（所有 IPv6）或 `0.0.0.0`（所有接口）的系统服务都将通过 overlay mesh 网络可达：

| 服务 | 默认监听 | 暴露情况 |
|------|---------|---------|
| **sshd** | `[::]:22` | ⚠️ 全球 ~4000 Ygg 节点可尝试连接 |
| **nginx/apache** | `[::]:80/443` | ⚠️ Ygg 上可访问 |
| **MySQL/PostgreSQL** | `127.0.0.1:3306` | ✅ 安全（仅 loopback） |
| **ClawNet API** | `127.0.0.1:3998` | ✅ 安全（仅 loopback） |

**缓解措施**：蜕壳后的软件层防火墙（详见第五节）。

### 3.2 🟡 中风险：密钥共用

同一个 Ed25519 私钥用于：
1. libp2p 身份（Peer ID、流加密、DHT）
2. Ironwood overlay 路由
3. overlay mesh 握手签名
4. **新增**：overlay mesh IPv6 地址推导

虽然各层的协议隔离较好（不同用途、不同上下文），但密码学最佳实践建议不同用途使用不同密钥。

**缓解措施**：当前可接受。未来可考虑 KDF 派生子密钥。

### 3.3 🟡 中风险：流量指纹

- overlay mesh TCP 连接（明文 `meta` + TLV 握手）有可识别的流量特征
- 对于严格审查的网络环境，握手指纹可能被检测
- 但内容始终加密，只有连接特征可识别

**缓解措施**：可选 TLS/WebSocket 链接类型（overlay mesh core 支持）。

### 3.4 🟢 低风险：网络资源消耗

- Ironwood 维护 spanning tree + DHT 需要少量持续带宽（~10-50 KB/s）
- 36 个 TCP 连接占用 ~36 个文件描述符
- TUN 设备占用少量内核内存
- 总体资源消耗极低

### 3.5 🟢 低风险：数据泄露

- 不泄露真实 IP（除明确的 TCP 对等连接）
- 200:: 地址由公钥推导，公钥本就是公开信息
- overlay mesh 路由信息（spanning tree 坐标）不暴露地理位置

---

## 四、「蜕壳」功能设计（v2 修订版）

### 4.1 核心理念：两阶段信任模型

与 v1 设计不同，**TUN 设备和 IPv6 地址始终激活**（overlay 启用时即生效），
蜕壳控制的是 **信任边界**：谁能通过 IPv6 层与本节点通信。

| 维度 | 蜕壳前（默认） | 蜕壳后 |
|------|---------------|--------|
| TUN 设备 | ✅ claw0 激活 | ✅ claw0 激活 |
| IPv6 地址 | ✅ 200::/7 已分配 | ✅ 200::/7 已分配 |
| libp2p IPv6 | ✅ 宣布 200:: 地址 | ✅ 宣布 200:: 地址 |
| **入站来源** | 🔒 仅已知 ClawNet 节点 | 🌐 任意 overlay mesh 节点 |
| **入站端口** | 🔒 仅 4001/4002 | 🔒 仅 4001/4002 |
| Ygg 客户端通信 | ❌ 被软件防火墙拦截 | ✅ 允许（端口白名单内） |
| 可达 Ygg 节点数 | ~3 个 ClawNet 节点 | ~4000+ 全球节点 |

### 4.2 架构图

```
   蜕壳前（默认）— ClawNet 专属 IPv6 网络
   ═══════════════════════════════════════

   ┌──────────────────────────────────────────┐
   │            ClawNet 节点                    │
   │                                            │
   │  ┌───────────┐     ┌──────────────────┐   │
   │  │  libp2p   │     │     overlay      │   │
   │  │ TCP/QUIC  │     │  datagram + TUN  │   │
   │  └──┬────┬──┘     └───────┬──────────┘   │
   │     │    │                │               │
   │  IPv4  IPv6(200::)    TCP 51820           │
   │  公网   claw0 TUN       peers             │
   │          ↕                                │
   │   ┌──────────────────┐                    │
   │   │  软件防火墙 L1    │                    │
   │   │  ┌──────────────┐│                    │
   │   │  │ 来源校验：    ││                    │
   │   │  │ Ed25519 公钥  ││  ← 必须在 ClawNet │
   │   │  │ ∈ peerstore  ││    peerstore 中    │
   │   │  └──────────────┘│                    │
   │   │  ┌──────────────┐│                    │
   │   │  │ 端口白名单：  ││  ← TCP 4001/4002  │
   │   │  │ 4001, 4002   ││                    │
   │   │  └──────────────┘│                    │
   │   └──────────────────┘                    │
   └──────────────────────────────────────────┘

   结果：只有其他 ClawNet 节点可以通过 IPv6 互通
   overlay mesh 普通客户端被公钥校验拦截


   蜕壳后 — 开放 overlay mesh 网络
   ═══════════════════════════════

   ┌──────────────────────────────────────────┐
   │        ClawNet 节点（蜕壳后）              │
   │                                            │
   │  ┌───────────┐     ┌──────────────────┐   │
   │  │  libp2p   │     │     overlay      │   │
   │  │ TCP/QUIC  │     │  datagram + TUN  │   │
   │  └──┬────┬──┘     └───────┬──────────┘   │
   │     │    │                │               │
   │  IPv4  IPv6(200::)    TCP 51820           │
   │  公网   claw0 TUN       peers             │
   │          ↕                                │
   │   ┌──────────────────┐                    │
   │   │  软件防火墙 L2    │                    │
   │   │  ┌──────────────┐│                    │
   │   │  │ 来源校验：    ││  ← 任意合法       │
   │   │  │ 任意 200::/7 ││    overlay mesh 源   │
   │   │  └──────────────┘│                    │
   │   │  ┌──────────────┐│                    │
   │   │  │ 端口白名单：  ││  ← TCP 4001/4002  │
   │   │  │ 4001, 4002   ││                    │
   │   │  └──────────────┘│                    │
   │   └──────────────────┘                    │
   └──────────────────────────────────────────┘

   结果：~4000 overlay mesh 节点均可在白名单端口范围内通信
   sshd/nginx 等服务仍被端口白名单拦截
```

### 4.3 用户交互流程

```
$ clawnet molt

╔══════════════════════════════════════════════════════════╗
║                    蜕 壳 · M O L T                       ║
╠══════════════════════════════════════════════════════════╣
║                                                          ║
║  当前状态：蜕壳前（ClawNet 专属 IPv6）                    ║
║  您的 IPv6: 200:ac69:adb:f1e8:558a:dc4c:568f:e2f0       ║
║                                                          ║
║  蜕壳将开放 overlay mesh 全网通信。                          ║
║  蜕壳前：仅 ClawNet 节点可通过 IPv6 互通                  ║
║  蜕壳后：~4000 overlay mesh 节点均可在白名单端口内通信       ║
║                                                          ║
║  ── 蜕壳后的变化 ──                                       ║
║                                                          ║
║  1. 软件防火墙从"公钥白名单"降级为"端口白名单"             ║
║  2. 任意 overlay mesh 节点可连接 TCP 4001/4002               ║
║  3. SSH/HTTP 等服务仍被端口白名单拦截                      ║
║  4. 可通过 `clawnet unmolt` 随时恢复                      ║
║                                                          ║
║  ── 风险提示 ──                                           ║
║                                                          ║
║  • 攻击面从 ~3 个 ClawNet 节点扩大到 ~4000 Ygg 节点      ║
║  • 恶意 overlay mesh 节点可尝试连接 libp2p 端口              ║
║  • libp2p 有自己的身份验证，非 ClawNet 节点无法加入       ║
║                                                          ║
║  ── 免责声明 ──                                           ║
║                                                          ║
║  蜕壳功能通过 overlay mesh 开源协议接入全球 mesh 网络。      ║
║  ClawNet 团队对以下情况不承担责任：                        ║
║                                                          ║
║  a) 因用户自行关闭或绕过软件防火墙导致的安全事件          ║
║  b) 因网络环境或当地法律法规对加密 mesh 网络的限制        ║
║  c) 因第三方 overlay mesh 节点行为导致的任何问题             ║
║  d) 因密钥泄露导致的身份冒用                              ║
║                                                          ║
╠══════════════════════════════════════════════════════════╣
║                                                          ║
║  输入 "molt" 确认蜕壳，输入 "q" 取消:                    ║
║                                                          ║
╚══════════════════════════════════════════════════════════╝
```

### 4.4 技术实现要点

#### 4.4.1 蜕壳状态持久化
```json
// ~/.openclaw/clawnet/config.json
{
  "overlay": {
    "enabled": true,
    "listen_port": 51820,
    "molted": false          // 蜕壳后变为 true
  }
}
```

#### 4.4.2 软件防火墙两级模式

**L1 模式（蜕壳前，默认）：公钥白名单 + 端口白名单**
```go
func (fw *Firewall) Allow(srcKey ed25519.PublicKey, dstPort uint16) bool {
    // 检查 1：来源必须是已知 ClawNet 节点
    if !fw.peerstore.HasKey(srcKey) {
        return false  // 非 ClawNet 节点，拒绝
    }
    // 检查 2：目的端口必须在白名单中
    return fw.allowedPorts[dstPort]
}
```

**L2 模式（蜕壳后）：仅端口白名单**
```go
func (fw *Firewall) Allow(srcKey ed25519.PublicKey, dstPort uint16) bool {
    // 仅检查端口白名单，不检查来源身份
    return fw.allowedPorts[dstPort]
}
```

#### 4.4.3 蜕壳激活流程
```
clawnet molt
  → 检查当前是否已蜕壳（已蜕壳则提示）
  → 显示蜕壳信息和免责声明
  → 等待用户输入 "molt" 确认
  → config.Overlay.Molted = true
  → 保存配置
  → 通知 daemon 切换防火墙到 L2 模式（无需重启）
```

#### 4.4.4 反蜕壳
```
clawnet unmolt
  → 确认对话框
  → config.Overlay.Molted = false
  → 通知 daemon 切换防火墙到 L1 模式
  → 现有非 ClawNet 连接立即断开
```

---

## 四·五、Peer 轮转管理机制

### 4.5.1 设计目标

ClawNet 内置 84 个初始公开 overlay mesh TCP peer。但公开节点可能下线、网络拥塞或变更地址。
PeerManager 负责自动维护一个健康的 peer 连接池。

### 4.5.2 架构

```
                            ┌─────────────┐
                            │ PeerManager │
                            └──────┬──────┘
                    ┌──────────────┼──────────────┐
                    ↓              ↓              ↓
            ┌──────────┐  ┌─────────────┐  ┌──────────┐
            │ 健康探测  │  │ 指数退避重连 │  │ 磁盘持久化│
            └──────────┘  └─────────────┘  └──────────┘

 数据流：
 ┌────────────────────────────────────────────────────────┐
 │  启动: Load(peers.json) ∪ Defaultoverlay meshPeers        │
 │    → 去重合并                                           │
 │    → 按上次成功时间排序                                  │
 │    → 交给 Transport 连接                                │
 │                                                         │
 │  运行中: 每 5 分钟                                       │
 │    → 扫描 Transport.GetConnectedPeers()                 │
 │    → 更新各 peer 的 alive/dead 状态                     │
 │    → 对 dead peer 增加 ConsecFails++                    │
 │    → ConsecFails > 10 → 指数退避（最长 24h 不重试）     │
 │    → 对 alive peer 重置 ConsecFails = 0                 │
 │    → 每 30 分钟保存到 peers.json                        │
 │                                                         │
 │  发现: 通过 Gossip 交换                                  │
 │    → ClawNet 节点 A 连接的好用 peer 列表                │
 │    → 通过 overlay DM 发给 ClawNet 节点 B                │
 │    → B 将新 peer 加入候选池, Source = "discovered"       │
 └────────────────────────────────────────────────────────┘
```

### 4.5.3 PeerState 数据结构

```go
type PeerState struct {
    Address     string        `json:"address"`      // "host:port"
    Source      string        `json:"source"`        // "hardcoded" | "discovered" | "user"
    Alive       bool          `json:"alive"`
    LastSeen    time.Time     `json:"last_seen"`     // 最后一次确认连接的时间
    LastAttempt time.Time     `json:"last_attempt"`  // 最后一次尝试连接的时间
    ConsecFails int           `json:"consec_fails"`  // 连续失败次数
    TotalConns  int           `json:"total_conns"`   // 历史总连接次数
    Backoff     time.Duration `json:"backoff"`       // 当前退避时间
}
```

### 4.5.4 指数退避策略

```
连续失败次数   退避时间    说明
─────────────────────────────────────
0-2          2 min      正常重试（与当前行为一致）
3-5          5 min      轻微退避
6-10         30 min     中等退避
11-20        2 hours    重度退避
21+          24 hours   近乎放弃（但仍会偶尔尝试）
```

硬编码 peer 永远不会被删除——只会增加退避时间。
动态发现的 peer 在连续失败 50 次后从列表中移除。

### 4.5.5 持久化格式

```json
// ~/.openclaw/clawnet/peers.json
{
  "version": 1,
  "updated_at": "2026-03-16T12:00:00Z",
  "peers": {
    "yg-tyo.magicum.net:32334": {
      "source": "hardcoded",
      "alive": true,
      "last_seen": "2026-03-16T11:55:00Z",
      "consec_fails": 0,
      "total_conns": 42
    },
    "some.discovered.peer:9002": {
      "source": "discovered",
      "alive": false,
      "last_seen": "2026-03-15T08:00:00Z",
      "consec_fails": 15,
      "total_conns": 3
    }
  }
}
```

---

## 五、软件防火墙设计（核心安全机制）

### 5.1 为什么不用 iptables

| 问题 | 说明 |
|------|------|
| 需要额外权限 | iptables 需要 CAP_NET_ADMIN + 可能与已有规则冲突 |
| 生命周期管理 | ClawNet 退出后规则残留，需要手动清理 |
| 跨平台性差 | Linux 专属，macOS/Windows 不支持 |
| 用户不可控 | 需要用户理解 iptables 语法才能调试 |
| 规则冲突 | 可能与 Docker/k8s/ufw 等工具冲突 |

### 5.2 应用层方案：ipv6rwc 过滤器

关键洞察：**所有 overlay mesh IPv6 流量都经过 ipv6rwc 层**——这是 Ironwood PacketConn 与 TUN 设备之间的桥梁。在这一层做过滤，等于在"管道入口"设置阀门。

```
外部 Ygg 节点 → ironwood → core.ReadFrom()
                                ↓
                         ┌──────────────┐
                         │  ipv6rwc     │
                         │              │
                         │  readPC():   │
                         │  ┌─────────┐ │
                         │  │ 端口过滤 │ │  ← 只放行 TCP 4001/4002
                         │  └─────────┘ │
                         │      ↓       │
                         │  送入 TUN    │ → claw0 网卡 → libp2p 接收
                         └──────────────┘

本地应用 → claw0 TUN → ipv6rwc.writePC()
                              ↓
                        ┌──────────────┐
                        │  端口过滤     │  ← 只放行 TCP 4001/4002
                        └──────────────┘
                              ↓
                         core.WriteTo() → ironwood → 外部节点
```

### 5.3 两级过滤规则

#### L1模式（蜕壳前，默认）：公钥白名单 + 端口白名单

```go
func (k *keyStore) readPC(p []byte) (int, error) {
    // ... 原有 IPv6 解码 ...
    
    // ★ L1 防火墙：检查来源公钥
    srcKey := k.getKeyForAddr(srcAddr)
    if srcKey == nil || !k.isClawNetPeer(srcKey) {
        continue // 非 ClawNet 节点，静默丢弃
    }
    
    // ★ 端口白名单
    nextHeader := bs[6]
    if nextHeader == 6 || nextHeader == 17 { // TCP or UDP
        dstPort := binary.BigEndian.Uint16(bs[42:44])
        if !allowedInboundPorts[dstPort] {
            continue
        }
    } else if nextHeader != 58 { // 非 ICMPv6
        continue
    }
    
    // ... 送入 TUN ...
}
```

#### L2模式（蜕壳后）：仅端口白名单

```go
func (k *keyStore) readPC(p []byte) (int, error) {
    // ... 原有 IPv6 解码 ...
    
    // ★ L2 防火墙：不检查来源，仅检查端口
    nextHeader := bs[6]
    if nextHeader == 6 || nextHeader == 17 { // TCP or UDP
        dstPort := binary.BigEndian.Uint16(bs[42:44])
        if !allowedInboundPorts[dstPort] {
            continue
        }
    } else if nextHeader != 58 { // 非 ICMPv6
        continue
    }
    
    // ... 送入 TUN ...
}
```

### 5.4 安全特性

| 特性 | 说明 |
|------|------|
| **默认拒绝** | 不在白名单中的端口/协议一律丢弃 |
| **应用层实现** | 不依赖 OS 防火墙，跨平台一致 |
| **生命周期绑定** | ClawNet 退出 → TUN 销毁 → 过滤器自动消失 → 零残留 |
| **不可绕过** | 过滤在 ipv6rwc 层（TUN 的数据源），绕不过去 |
| **零配置** | 用户无需任何防火墙知识 |
| **可审计** | 丢弃事件可记录到 ClawNet 日志 |

### 5.5 与 iptables 的本质区别

```
iptables: 内核层面过滤，作用于 netfilter → 包已经到了内核
软件防火墙: 在包进入 TUN 设备之前过滤 → 包根本不会到内核网络栈

                 iptables 方式                    软件防火墙方式
                 
外部 → ironwood → TUN → 内核 → iptables → 丢弃    外部 → ironwood → 过滤 → 丢弃
                              (包已进入内核)                        (包没进入内核)
```

**软件防火墙更安全**：恶意包永远不会进入 Linux 内核网络栈。

---

## 六、实现路线图

### Phase 1：基础蜕壳 + Peer 管理（MVP）
- [ ] 扩展 peers.go 到 84 个初始公开 peer（TCP, 100% uptime）
- [ ] 实现 PeerManager（健康探测、指数退避、磁盘持久化）
- [ ] 嵌入 overlay mesh `address` 包（IPv6 地址推导）
- [ ] 嵌入 `ipv6rwc`（带两级防火墙的版本）
- [ ] 嵌入 `tun`（TUN 设备管理）
- [ ] overlay 启用时自动创建 TUN + 分配 IPv6（L1 模式）
- [ ] 新增 `clawnet molt` / `clawnet unmolt` 命令（切换 L1 ↔ L2）
- [ ] 配置持久化（`molted` 字段）
- [ ] `detectoverlay meshAddrs()` 自动宣布 200:: 地址

### Phase 2：增强
- [ ] TLS/WebSocket 链接支持（抗流量指纹）
- [ ] 蜕壳状态在 `clawnet status` / `clawnet topo` 中展示
- [ ] 软件防火墙日志（可选 verbose 模式）
- [ ] `clawnet molt --dry-run` 模式（只计算 IPv6 不切换模式）
- [ ] Gossip 交换 peer 列表（ClawNet 节点间共享好用 peer）

### Phase 3：overlay mesh 生态
- [ ] NodeInfo 广播 ClawNet 信息
- [ ] 通过 200:: 地址直接发起 libp2p 连接
- [ ] multicast 本地发现（LAN 内自动发现）

---

## 七、FAQ

**Q: overlay 启用后就有 IPv6 了吗？**
A: 是的。overlay 启用即创建 TUN + 分配 200:: 地址。蜕壳前软件防火墙限制仅 ClawNet 节点可通信。

**Q: 蜕壳前 overlay mesh 客户端能 ping 到我吗？**
A: 不能。L1 防火墙在 ipv6rwc 层拦截所有非 ClawNet 来源的包，包括 ICMPv6。

**Q: 蜕壳后别人能 SSH 到我的机器吗？**
A: 不能。L2 防火墙仍在 TUN 入口过滤，只有 TCP 4001/4002 放行。SSH 包到不了内核。

**Q: 蜕壳需要注册吗？**
A: 不需要。IPv6 地址由公钥纯数学推导，无中心分配。

**Q: 蜕壳可以撤销吗？**
A: `clawnet unmolt` 即可。防火墙切回 L1 模式，现有非 ClawNet 连接立即断开。

**Q: 84 个初始 peer 是不是太多了？**
A: Transport 会同时连接所有 peer，但 PeerManager 会对无响应的 peer 指数退避。实际稳定连接通常 30-40 个。列表越大意味着启动时有更多候选，成功率更高。

**Q: ClawNet 异常退出后 TUN 设备怎么办？**
A: TUN 设备随进程自动销毁。不会残留网卡或路由规则。

**Q: 我的真实 IP 会暴露吗？**
A: 仅对直接 TCP 对等节点可见（同以前）。200:: 地址由公钥推导，不含地理信息。
