# ClawNet 网络架构设计：Matrix 发现 + Pinecone 传输 + NaCl E2E 加密

> **v1.0 | 2026-03-16**
>
> 基于 [research/03-irc-matrix-integration-analysis.md](../research/03-irc-matrix-integration-analysis.md) 的技术调研结论，
> 本文定义 ClawNet v0.9 的网络层升级方案——引入 Matrix 发现网络、Pinecone 备用传输、NaCl box 应用层 E2E 加密。

---

## 一、设计原则

**取什么**：

- Matrix Client-Server API → 第 6 层发现（公共 homeserver 房间交换 multiaddr）
- Pinecone 覆盖网络 → 第 7 层发现 + 备用传输（SNEK 路由，极端环境下兜底）
- NaCl box 加密（X25519 + XSalsa20-Poly1305）→ DM 应用层 E2E（升级现有 Noise-only transport encryption）

**不取什么**：

| 不集成 | 原因 |
|--------|------|
| Dendrite 嵌入 | +30–50 MB 二进制膨胀，需要域名/TLS/Federation 全套，运维复杂度不可接受 |
| Matrix Federation (S2S) | 每个 homeserver 需要独立域名 + DNS + TLS，NAT 节点无法参与 |
| Matrix 身份体系 (@user:domain) | 与 ClawNet Ed25519 PeerID 冲突，维护两套身份成本高 |
| Matrix Rooms 替代 GossipSub | GossipSub 延迟 <100ms，Matrix Room 延迟 300ms–2s，不适合实时 gossip |
| IRC 协议 | 中心化树状拓扑，无 E2E，与去中心化目标根本冲突 |

---

## 二、目标架构

```
ClawNet Node v0.9
┌─────────────────────────────────────────────────┐
│                Application Layer                 │
│  GossipSub（保留，主通信）                         │
│  DM + NaCl box E2E（升级，应用层加密）             │
│  Bundle / Knowledge Sync（保留）                  │
├─────────────────────────────────────────────────┤
│             Discovery Layer（7 层）               │
│  ① mDNS            ② Kademlia DHT               │
│  ③ HTTP Bootstrap   ④ BT Mainline DHT           │
│  ⑤ K8s DNS          ⑥ Matrix Discovery（新增）    │
│  ⑦ Pinecone Overlay（新增）                       │
├─────────────────────────────────────────────────┤
│              Transport Layer                     │
│  libp2p（主）          │  Pinecone（备）           │
│  TCP / QUIC / WS / Relay │  SNEK 路由 / BLE     │
│  ─────── 共享 Ed25519 identity ────────         │
├─────────────────────────────────────────────────┤
│               Security Layer                     │
│  Noise（传输层，保留）                              │
│  NaCl box（应用层 E2E，新增）                      │
│  Ed25519 签名（保留，统一密钥）                     │
└─────────────────────────────────────────────────┘
```

---

## 三、Layer 6 — Matrix Discovery

### 3.1 设计思路

利用公共 Matrix homeserver 作为**全球信令通道**：节点以 Matrix client 身份登录公共 homeserver，加入 `#clawnet-discovery:matrix.org` 等约定房间，定期发布自己的 libp2p multiaddr，同时监听其他节点的 multiaddr 消息，从而发现新 peer。

这是一种**带外发现**（out-of-band discovery）：Matrix 仅用于交换地址，实际数据传输仍走 libp2p。

### 3.2 多 Homeserver 策略

单一 homeserver 是单点故障。ClawNet 内置多个区域 homeserver：

| Homeserver | 区域 | 说明 |
|---|---|---|
| `matrix.org` | 全球 | 最大公共 homeserver（⚠️ 中国不可达） |
| `envs.net` | 欧洲 | 稳定的社区 homeserver |
| `tchncs.de` | 欧洲 | 无审查的开放 homeserver |
| `mozilla.org` | 美国 | Mozilla 基金会运营 |
| `converser.eu` | 欧洲 | 欧洲备选 |
| 自建 `matrix.clawnet.cc` | 亚洲 | 中国可达，自运维 |

**节点启动时随机选择 2–3 个 homeserver 注册/登录**，房间消息跨 homeserver 联邦同步，任一 homeserver 可达即可完成发现。

### 3.3 房间约定

| 房间别名 | 用途 |
|---|---|
| `#clawnet-discovery:<server>` | multiaddr 广播（周期性 + 启动时） |
| `#clawnet-relay-providers:<server>` | Relay 节点地址广播 |

房间内消息格式（JSON，作为 `m.room.message` 的 body）：

```json
{
  "type": "clawnet.announce",
  "version": 1,
  "peer_id": "12D3KooW...",
  "addrs": [
    "/ip4/210.45.71.67/tcp/4001",
    "/ip4/210.45.71.67/udp/4001/quic-v1"
  ],
  "agent": "clawnet/0.9.0",
  "ts": 1710590400
}
```

### 3.4 账号管理

- 启动时自动注册 guest 账号或用 `clawnet_<peerID-prefix>` 用户名注册
- 密码派生自 Ed25519 私钥（HKDF-SHA256，salt = homeserver domain）
- **无需用户手动注册 Matrix 账号**——完全自动化
- Token 缓存到 `$CLAWNET_DATA_DIR/matrix_tokens.json`（0600 权限）

### 3.5 代码位置

```
internal/
  matrix/
    discovery.go    — MatrixDiscovery struct, Start/Stop, announce/listen 循环
    client.go       — Matrix C-S API 客户端（登录/注册/发消息/同步）
    config.go       — Homeserver 列表, 房间别名, 重发策略
```

### 3.6 集成点

在 `p2p.NewNode()` 中，紧接 `k8sDiscovery(ctx)` 之后调用：

```go
// Layer 6: Matrix Discovery
if cfg.MatrixDiscovery.Enabled {
    md, err := matrix.NewDiscovery(ctx, priv, cfg.MatrixDiscovery)
    if err != nil {
        log.Printf("[matrix] discovery init failed: %v (non-fatal)", err)
    } else {
        n.MatrixDiscovery = md
        go md.Run(ctx, func(pi peer.AddrInfo) {
            n.Host.Connect(ctx, pi)
        })
    }
}
```

### 3.7 配置

```go
// config.go 新增
type MatrixDiscoveryConfig struct {
    Enabled     bool     `json:"matrix_discovery_enabled"`
    Homeservers []string `json:"matrix_homeservers,omitempty"`   // 默认内置列表
    AnnounceInterval time.Duration `json:"-"`                    // 默认 5min
}
```

环境变量覆盖：`CLAWNET_MATRIX_ENABLED=true/false`

### 3.8 依赖

- **无外部 SDK**：Matrix C-S API 是标准 HTTP REST，直接用 `net/http` 实现核心 3 个 endpoint（`/login`, `/register`, `/rooms/{id}/send`, `/sync`）即可，避免引入大型 SDK
- 预计代码量：~500 行
- 二进制增量：~0 MB（纯 HTTP 调用）

---

## 四、Layer 7 — Pinecone 备用传输

### 4.1 设计思路

[Pinecone](https://github.com/matrix-org/pinecone) 是 Matrix.org 团队开发的覆盖网络路由库，使用 SNEK（Sequentially Networked Edwards Key）算法，以 Ed25519 公钥作为全局路由地址，实现无需中央协调的 overlay 路由。

ClawNet 将 Pinecone 作为 **备用传输层**：当 libp2p 所有传输（TCP/QUIC/WS/Relay）均失败时，通过 Pinecone overlay 路由数据。

### 4.2 架构角色

```
正常路径:  Node A ──libp2p TCP/QUIC──→ Node B     (延迟低，吞吐高)
Relay路径: Node A ──libp2p Relay──→ Node B         (NAT 穿越)
兜底路径:  Node A ──Pinecone SNEK──→ Node B        (极端环境，如双重 NAT)
```

Pinecone **不替代** libp2p，仅在以下场景激活：

1. 目标 peer 通过 libp2p 无法直连且无可用 relay
2. 网络环境极端受限（企业防火墙、双重 NAT、移动网络）
3. 作为第 7 层发现——Pinecone 网络中的 peer 自动可见

### 4.3 共享 Ed25519 密钥

Pinecone 使用 Ed25519 公钥作为路由地址。ClawNet 已使用 Ed25519 作为 libp2p 身份（`identity.key`）。两者共享同一密钥对：

```go
// identity.go 中加载的 Ed25519 私钥
priv, _ := identity.LoadOrGenerate(dataDir)

// 提取原始 Ed25519 字节给 Pinecone
rawPriv, _ := priv.Raw()  // 64 bytes: seed(32) + pubkey(32)
var pineconeKey ed25519.PrivateKey = rawPriv

// Pinecone router 使用同一密钥
router := pinecone.NewRouter(log, pineconeKey, ...)
```

### 4.4 代码位置

```
internal/
  pine/
    transport.go    — PineconeTransport struct, Start/Stop, 连接/监听
    router.go       — Pinecone Router 管理, peer 发现回调
    bridge.go       — libp2p ↔ Pinecone 桥接（封装 Pinecone conn 为 libp2p stream）
```

### 4.5 集成点

两处集成：

**1. 在 `daemon.Start()` 中初始化 Pinecone（独立于 libp2p）：**

```go
// Pinecone backup transport
if cfg.Pinecone.Enabled {
    pt, err := pine.NewTransport(ctx, priv, cfg.Pinecone)
    if err != nil {
        log.Printf("[pinecone] init failed: %v (non-fatal)", err)
    } else {
        d.Pinecone = pt
        go pt.Run(ctx)
    }
}
```

**2. 在 DM/Bundle 发送失败时 fallback：**

```go
func (d *Daemon) sendToPeer(ctx context.Context, peerID peer.ID, data []byte) error {
    // 优先 libp2p
    err := d.sendViaLibp2p(ctx, peerID, data)
    if err == nil {
        return nil
    }
    // fallback Pinecone
    if d.Pinecone != nil {
        return d.Pinecone.Send(ctx, peerID, data)
    }
    return err
}
```

### 4.6 配置

```go
type PineconeConfig struct {
    Enabled    bool     `json:"pinecone_enabled"`
    ListenPort int      `json:"pinecone_port,omitempty"`  // 默认 0（随机）
    StaticPeers []string `json:"pinecone_peers,omitempty"` // 种子 Pinecone peer
}
```

环境变量：`CLAWNET_PINECONE_ENABLED=true/false`

### 4.7 依赖

- `github.com/matrix-org/pinecone` — Go 库，直接 import
- 预计二进制增量：~5 MB
- 预计代码量：~400 行

---

## 五、NaCl Box E2E 加密升级

### 5.1 现状问题

当前 DM 加密仅依赖 **Noise 协议**（传输层加密）：

```
Node A ──Noise(X25519+ChaCha20)──→ Node B
```

局限：

1. **仅加密传输链路**——如果消息经过 Relay 中继，Relay 节点理论上可审查明文（虽然 Circuit Relay v2 也有加密，但信任边界模糊）
2. **无 Forward Secrecy 持久化**——Noise session 结束后无法解密历史消息（这是优点也是限制）
3. **无群聊 E2E**——GossipSub topic 消息全明文

### 5.2 升级方案

使用 **NaCl box**（X25519 + XSalsa20-Poly1305）作为**应用层**端到端加密：

```
                   ┌── 传输层：Noise（保留）───┐
Node A ── 应用层：NaCl box E2E ── │ TCP / QUIC / Relay │ ── Node B
                      └──────────────────────────┘
```

- **DM**：每条消息使用 NaCl box 加密（X25519 密钥交换 + XSalsa20-Poly1305 认证加密），消息在发送端加密、接收端解密
- **双层加密**：传输层 Noise + 应用层 NaCl box，即使 Relay 中继也无法读取明文
- **零新依赖**：使用 `golang.org/x/crypto/nacl/box`（已在依赖树中）

> **为什么不用 Olm？** go-olm 仓库已删除，libolm CGo 绑定增加编译复杂度。NaCl box 提供同等安全级别（Curve25519 + 认证加密），且为纯 Go 实现，零额外依赖。

### 5.3 密钥关系

```
identity.key (Ed25519)
  ├── libp2p PeerID        — 节点身份
  ├── Pinecone 路由地址     — overlay 路由
  ├── GossipSub 签名        — 消息认证
  └── NaCl Identity Key     — E2E 加密身份
       └── 派生 Curve25519   — 双有理映射 Edwards → Montgomery
```

Ed25519 → Curve25519 转换使用标准双有理映射 `u = (1+y)/(1-y) mod p`。

### 5.4 加密流程

```
1. Node A 想给 Node B 发加密 DM
2. A 从 B 的 PeerID 提取 Ed25519 公钥，转换为 Curve25519 公钥
3. A 生成 24 字节随机 nonce
4. A 使用 NaCl box.Seal(plaintext, nonce, B_pubkey, A_privkey) 加密
5. A 发送 EncryptedEnvelope（含 A 的公钥 + nonce + 密文）
6. B 收到后，使用 NaCl box.Open(ciphertext, nonce, A_pubkey, B_privkey) 解密
```

### 5.5 代码位置

```
internal/
  crypto/
    keys.go         — Ed25519 → Curve25519 转换, PeerID → Curve25519 公钥
    edwards.go      — Edwards → Montgomery 双有理映射（math/big 实现）
    engine.go       — CryptoEngine struct: Encrypt / Decrypt / Sessions
    engine_test.go  — 单元测试（密钥转换 + 加解密 + IsEncrypted）
```

### 5.6 集成点

修改 `internal/daemon/dm.go`：

```go
// 发送 DM 时
func (d *Daemon) sendDM(ctx context.Context, peerIDStr, body string) error {
    wm := DMWireMsg{ID: uuid.New().String(), Body: body, ...}
    wmData, _ := json.Marshal(wm)

    // NaCl box 加密（向后兼容：失败时发送明文）
    if d.Crypto != nil {
        encrypted, err := d.Crypto.Encrypt(pid, wmData)
        if err == nil {
            data = encrypted
        }
    }
    // 发送到 peer（libp2p stream）
}

// 接收 DM 时
func (d *Daemon) registerDMHandler() {
    // 检测 IsEncrypted(line) → Decrypt → 解析 DMWireMsg
    // 否则直接解析 DMWireMsg（兼容旧版节点）
}
```

### 5.7 Wire Format

加密消息使用 `EncryptedEnvelope` JSON 格式：

```json
{
  "v": 1,
  "encrypted": true,
  "pub_key": "<base64 sender Curve25519 pubkey>",
  "nonce": "<base64 24-byte random nonce>",
  "ciphertext": "<base64 NaCl box ciphertext>"
}
```

### 5.8 依赖

- `golang.org/x/crypto/nacl/box`（已存在）
- `golang.org/x/crypto/curve25519`（已存在）
- 二进制增量：~0 KB（纯 Go，无新依赖）
- 代码量：~350 行（keys.go + edwards.go + engine.go）

---

## 六、配置汇总

`config.json` 新增字段：

```json
{
  "matrix_discovery": {
    "enabled": true,
    "homeservers": [
      "https://matrix.org",
      "https://envs.net",
      "https://matrix.clawnet.cc"
    ],
    "announce_interval_sec": 300
  },
  "pinecone": {
    "enabled": false,
    "port": 0,
    "peers": []
  },
  "olm_encryption": {
    "enabled": true
  }
}
```

默认值策略：

| 功能 | 默认 | 理由 |
|------|------|------|
| Matrix Discovery | **开启** | 零配置增加发现维度，无副作用 |
| Pinecone Transport | **关闭** | 实验性，二进制增大 5MB，按需开启 |
| Olm Encryption | **开启** | 安全升级，对用户透明 |

---

## 七、启动顺序（更新后）

```
daemon.Start()
  1.  config.Load()
  2.  identity.LoadOrGenerate()           ← Ed25519 密钥（共享给所有子系统）
  3.  p2p.DetectExternalIP()              ← STUN
  4.  p2p.NewNode(ctx, priv, cfg)         ← libp2p + 5 层发现
        ├── Layer 1: mDNS
        ├── Layer 2: Kademlia DHT
        ├── Layer 3: HTTP Bootstrap
        ├── Layer 4: BT Mainline DHT
        ├── Layer 5: K8s DNS
        └── Layer 6: Matrix Discovery     ← 新增
  5.  store.Open()                        ← SQLite
  6.  crypto.NewOlmEngine(priv, store)    ← 新增：Olm 初始化
  7.  pine.NewTransport(priv, cfg)        ← 新增：Pinecone 备用传输
  8.  d.StartAPI()                        ← HTTP API
  9.  d.startGossipHandlers()
  10. d.registerDMHandler()               ← 升级：Olm 加解密
  ... (其余不变)
```

---

## 八、二进制体积影响

| 组件 | 当前 | 新增 | 说明 |
|------|------|------|------|
| clawnet 主二进制 | ~49 MB | — | DB1 精简版 |
| Matrix Discovery | — | ~0 MB | 纯 HTTP 调用，无新依赖 |
| Pinecone | — | ~5 MB | `matrix-org/pinecone` 库 |
| Olm | — | ~2 MB | CGo libolm 绑定 |
| **合计** | **49 MB** | **~56 MB** | Pinecone 关闭时不影响启动 |

如果 Pinecone 默认关闭 + build tag 隔离，精简版可维持 ~51 MB。

---

## 九、API 新增

| Method | Path | 说明 |
|--------|------|------|
| `GET` | `/api/matrix/status` | Matrix Discovery 状态（已连接 homeserver、房间成员数、最近发现数） |
| `GET` | `/api/pinecone/status` | Pinecone 状态（routing table size、已知 peer、上行/下行流量） |
| `GET` | `/api/crypto/sessions` | Olm Session 列表（peer_id、创建时间、消息计数） |

---

## 十、风险与缓解

| 风险 | 概率 | 影响 | 缓解策略 |
|------|------|------|------|
| 公共 homeserver 封禁 bot 注册 | 中 | Layer 6 不可用 | 多 homeserver + 自建 matrix.clawnet.cc |
| Pinecone 库不稳定 | 中 | 备用传输不可靠 | 默认关闭，仅高级用户启用；隔离 goroutine + recover |
| libolm CGo 交叉编译复杂 | 低 | CI/CD 复杂化 | 优先尝试纯 Go 实现 `go-olm`；降级方案：AES-GCM 自实现 |
| Matrix 同步流量过大 | 低 | 带宽浪费 | 仅同步 #clawnet-discovery 房间，filter 过滤其他事件 |
| 中国 homeserver 可达性 | 中 | 中国节点 Layer 6 不可用 | 优先部署 matrix.clawnet.cc 在国内可达 IP |

---

## 十一、实现路线图

### Phase A：Matrix Discovery（~3 天）

1. 创建 `internal/matrix/` 包
2. 实现 Matrix C-S API 最小客户端（register/login/send/sync）
3. 实现 `MatrixDiscovery` — 多 homeserver 连接、announce 循环、peer 解析
4. 集成到 `p2p.NewNode()`
5. 更新 `config.go` 添加 `MatrixDiscoveryConfig`
6. 测试：3 节点通过 Matrix 互相发现

### Phase B：Pinecone 备用传输（~3 天）

1. 创建 `internal/pine/` 包
2. 引入 `matrix-org/pinecone` 依赖
3. 实现 Pinecone Router 初始化 + Ed25519 密钥共享
4. 实现 `PineconeTransport.Send()` / `Listen()`
5. 实现 libp2p 优先 + Pinecone fallback 逻辑
6. 更新 `config.go` 添加 `PineconeConfig`
7. 测试：libp2p 断开后通过 Pinecone 传输 DM

### Phase C：Olm E2E 加密（~2 天）

1. 创建 `internal/crypto/` 包
2. 实现 Ed25519 → Curve25519 密钥转换
3. 实现 `OlmEngine`（session 管理、加密、解密）
4. 修改 `daemon/dm.go` 接入 Olm
5. SQLite 表 `olm_sessions` 存储 session state
6. 测试：DM 端到端加密通过 Relay 中继

### Phase D：集成测试 + 部署（~1 天）

1. 3 节点全功能测试（7 层发现 + 双传输 + Olm DM）
2. 更新 `clawnet doctor` 输出新层状态
3. 版本号升到 v0.9.0
4. 部署到 cmax / bmax / dmax

---

## 附录 A：Matrix C-S API 最小实现

仅需以下 endpoint（避免引入完整 Matrix SDK）：

| Endpoint | 用途 |
|----------|------|
| `POST /_matrix/client/v3/register` | guest/user 注册 |
| `POST /_matrix/client/v3/login` | 密码登录获取 access_token |
| `POST /_matrix/client/v3/join/{roomAlias}` | 加入发现房间 |
| `PUT /_matrix/client/v3/rooms/{roomId}/send/{eventType}/{txnId}` | 发送 announce 消息 |
| `GET /_matrix/client/v3/sync` | 长轮询获取房间消息（带 filter） |

## 附录 B：Pinecone SNEK 路由简述

SNEK（Sequentially Networked Edwards Key）是一种基于 Ed25519 公钥空间的 overlay 路由算法：

1. 每个节点的路由地址 = 其 Ed25519 公钥
2. 路由表按密钥空间距离维护邻居
3. 数据包按目标公钥逐跳转发，类似 DHT 的贪婪路由
4. 支持 TCP、WebSocket、蓝牙 (BLE) 作为底层传输
5. 无需全局协调，节点加入/离开自动收敛

## 附录 C：Olm Double Ratchet 简述

Olm 实现了 Signal Protocol 的 Double Ratchet 算法：

1. **3DH Key Agreement**：发起方用 (identity_key, ephemeral_key) × 接收方 (identity_key, one-time_key) 建立共享密钥
2. **Symmetric Ratchet**：每条消息递进 HMAC-SHA256 chain key，派生 AES-256-CBC message key
3. **DH Ratchet**：每轮对话交换新 DH 公钥，实现 Forward Secrecy
4. **Pre-key Message**：首条消息携带发起方公钥 + 密文，接收方无需在线即可解密
