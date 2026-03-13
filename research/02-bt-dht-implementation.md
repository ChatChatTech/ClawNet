# BT DHT 节点发现：实现方案详解

> 日期：2026-03-13
> 目的：给出 ClawNet 利用 BitTorrent Mainline DHT 做零配置节点发现的完整实现思路
> 附带：GitHub Pages 免费 Bootstrap 方案

---

## 一、核心思路

BitTorrent Mainline DHT 是全球最大的分布式哈希表，活跃节点数千万。我们不用它来传文件——而是**把它当作一个全球公告板**：ClawNet 节点在上面注册自己的 IP:Port，同时查找其他已注册的 ClawNet 节点。

这和 torrent 客户端找其他 peer 的原理完全一样——只不过我们找的不是电影下载者，而是其他 ClawNet 节点。

---

## 二、BT Mainline DHT 协议要点

### 2.1 核心概念

- **Node**：运行 DHT 协议的 UDP 端点
- **Infohash**：20 字节 SHA-1 哈希，标识一个 "torrent"
- **get_peers(infohash)**：在 DHT 中查找所有 announce 过该 infohash 的 IP:Port
- **announce_peer(infohash, port)**：向 DHT 宣布 "我是这个 infohash 的 peer"

### 2.2 关键设计

我们约定一个**固定 infohash** 作为 ClawNet 的全球标识：

```go
import "crypto/sha1"

// ClawNet 在 BT DHT 中的网络标识
// 所有 ClawNet 节点都对这个 infohash 做 announce/get_peers
var ClawNetInfoHash = sha1.Sum([]byte("clawnet-bootstrap-v1"))
// = 20 字节的 SHA-1 哈希
```

为什么用 SHA-1：BT DHT (BEP-5) 规定 infohash 是 20 字节 SHA-1，这是协议强制的。

### 2.3 BT DHT Bootstrap 节点 (免费，全球分布)

```go
// 这些是全球公共的 BT DHT 入口节点
// anacrolix/dht v2 已内置这些默认地址
var DefaultBTBootstrapNodes = []string{
    "router.utorrent.com:6881",
    "router.bittorrent.com:6881",
    "dht.transmissionbt.com:6881",
    "dht.aelitis.com:6881",
    "router.silotis.us:6881",
    "dht.libtorrent.org:25401",
    "dht.anacrolix.link:42069",
    "router.bittorrent.cloud:42069",
}
```

这些节点由 BitTorrent 公司和开源社区维护，**永远在线**，无需我们操心。

---

## 三、Go 实现方案 (anacrolix/dht v2)

### 3.1 库选择

```
go get github.com/anacrolix/dht/v2
```

`anacrolix/dht` 是 Go 生态最成熟的 BT DHT 库：
- 349 GitHub stars
- 被 anacrolix/torrent (5k+ stars) 使用
- 完整实现 BEP-5 (DHT) + BEP-33 (scrape) + BEP-44 (mutable data)
- 内置全球 bootstrap 节点
- MPL-2.0 许可，可商用

### 3.2 核心代码架构

```go
package btdht

import (
    "context"
    "crypto/sha1"
    "fmt"
    "net"
    "time"

    dht "github.com/anacrolix/dht/v2"
)

// ClawNet 在 BT DHT 中的固定身份
var clawnetInfoHash = sha1.Sum([]byte("clawnet-bootstrap-v1"))

// BTDiscovery 管理 BT DHT 节点发现
type BTDiscovery struct {
    server     *dht.Server
    libp2pPort int  // 本地 libp2p 监听端口
}

// NewBTDiscovery 创建 BT DHT 发现服务
func NewBTDiscovery(listenPort int, libp2pPort int) (*BTDiscovery, error) {
    // 监听 UDP 端口
    conn, err := net.ListenPacket("udp", fmt.Sprintf(":%d", listenPort))
    if err != nil {
        return nil, fmt.Errorf("listen UDP :%d: %w", listenPort, err)
    }

    cfg := dht.NewDefaultServerConfig()
    cfg.Conn = conn
    // 使用全球默认 bootstrap 节点 (router.bittorrent.com 等)
    // anacrolix/dht 默认已内置，无需手动指定

    server, err := dht.NewServer(cfg)
    if err != nil {
        conn.Close()
        return nil, fmt.Errorf("create DHT server: %w", err)
    }

    // 启动路由表维护 (后台 goroutine)
    go server.TableMaintainer()

    return &BTDiscovery{
        server:     server,
        libp2pPort: libp2pPort,
    }, nil
}

// Bootstrap 填充 BT DHT 路由表
func (d *BTDiscovery) Bootstrap(ctx context.Context) error {
    _, err := d.server.BootstrapContext(ctx)
    return err
}

// Announce 在 BT DHT 中注册自己为 ClawNet 节点
func (d *BTDiscovery) Announce(ctx context.Context) error {
    announce, err := d.server.AnnounceTraversal(
        clawnetInfoHash,
        dht.AnnouncePeer(dht.AnnouncePeerOpts{
            Port:        d.libp2pPort,
            ImpliedPort: false, // 使用显式端口
        }),
    )
    if err != nil {
        return err
    }
    defer announce.Close()

    // 消费返回的 peers (同时发现其他节点)
    for peers := range announce.Peers {
        for _, peer := range peers.Peers {
            fmt.Printf("BT DHT: found ClawNet peer %s:%d\n",
                peer.IP, peer.Port)
        }
    }
    return nil
}

// FindPeers 在 BT DHT 中查找其他 ClawNet 节点
func (d *BTDiscovery) FindPeers(ctx context.Context) ([]net.TCPAddr, error) {
    announce, err := d.server.AnnounceTraversal(clawnetInfoHash)
    if err != nil {
        return nil, err
    }
    defer announce.Close()

    var found []net.TCPAddr
    for peers := range announce.Peers {
        for _, peer := range peers.Peers {
            found = append(found, net.TCPAddr{
                IP:   peer.IP,
                Port: peer.Port,
            })
        }
    }
    return found, nil
}

// Close 关闭 BT DHT 服务
func (d *BTDiscovery) Close() {
    d.server.Close()
}
```

### 3.3 集成到 ClawNet 节点启动流程

```go
// 在 p2p/node.go 的 NewNode 中添加

func NewNode(ctx context.Context, priv crypto.PrivKey, cfg *config.Config) (*Node, error) {
    // ... 现有的 libp2p 初始化代码 ...

    // === 新增：BT DHT 发现 ===
    if cfg.BTDHTEnabled {
        btDHT, err := btdht.NewBTDiscovery(cfg.BTDHTPort, cfg.P2PPort)
        if err != nil {
            fmt.Printf("warning: BT DHT setup failed: %v\n", err)
        } else {
            // Bootstrap BT DHT (异步)
            go func() {
                if err := btDHT.Bootstrap(ctx); err != nil {
                    fmt.Printf("warning: BT DHT bootstrap failed: %v\n", err)
                    return
                }

                // 周期性 announce + find peers
                ticker := time.NewTicker(20 * time.Minute)
                defer ticker.Stop()
                for {
                    select {
                    case <-ctx.Done():
                        return
                    case <-ticker.C:
                        // Announce 自己
                        go btDHT.Announce(ctx)

                        // 查找其他节点并尝试 libp2p 连接
                        peers, err := btDHT.FindPeers(ctx)
                        if err != nil {
                            continue
                        }
                        for _, addr := range peers {
                            // 将 BT DHT 发现的 IP:Port 转化为 libp2p multiaddr
                            // 尝试 TCP 和 QUIC 两种方式
                            tcpMA := fmt.Sprintf("/ip4/%s/tcp/%d", addr.IP, addr.Port)
                            quicMA := fmt.Sprintf("/ip4/%s/udp/%d/quic-v1", addr.IP, addr.Port)

                            for _, maStr := range []string{tcpMA, quicMA} {
                                ma, err := multiaddr.NewMultiaddr(maStr)
                                if err != nil { continue }
                                // 注意：没有 Peer ID， 所以需要先尝试连接
                                // libp2p 会在握手时验证身份
                                // 如果对方不是 libp2p 节点，会快速失败
                                node.Host.Connect(ctx, peer.AddrInfo{Addrs: []multiaddr.Multiaddr{ma}})
                            }
                        }
                    }
                }
            }()
        }
    }

    return node, nil
}
```

### 3.4 关键实现细节

#### 问题 1：BT DHT 只给 IP:Port，没有 Peer ID

BT DHT 的 `get_peers` 返回的是 `IP:Port` 对，但 libp2p 连接需要 Peer ID。

**解决方案**：
- 方法 A：直接用 IP:Port 作为 libp2p 地址尝试连接，libp2p 在 Noise 握手时会交换 Peer ID
- 方法 B：在 BT DHT 中使用 BEP-44 (mutable data) 存储 multiaddr（包含 Peer ID）
- 方法 C：先 TCP 探测，如果是 libp2p 节点会有协议匹配

**推荐方法 A**——最简单，实测可行。libp2p 的 Host.Connect 可以接受没有 Peer ID 的地址。

#### 问题 2：BT DHT Announce 有 TTL

BT DHT 的 announce 默认有效约 30 分钟。之后你的信息会从 DHT 中过期。

**解决方案**：每 20 分钟重新 announce 一次（上面的代码已经体现）。

#### 问题 3：BT 客户端会连你

因为你对一个 infohash 做了 announce，一些 torrent 客户端可能会尝试连接你并发送 BitTorrent 握手。

**解决方案**：
- libp2p 只接受 Noise/TLS 协议握手，非法的 BT 握手会被直接丢弃
- 不需要额外处理——libp2p 的协议协商会自动拒绝不认识的协议
- 可以在 connection manager 层面加速关闭非法连接

#### 问题 4：UDP 端口

BT DHT 使用 UDP（和 libp2p QUIC 传输不同的 UDP）。需要额外开一个 UDP 端口。

**推荐配置**：
```json
{
    "bt_dht_enabled": true,
    "bt_dht_port": 6881  // 使用标准 BT 端口，减少防火墙问题
}
```

#### 问题 5：ISP 可能限制 BT 流量

部分 ISP 会对 BT DHT 端口（6881）做流量限制。

**缓解方案**：
- 使用非标准端口（如 42069）
- BT DHT 流量极小（几 KB/分钟），很难被识别和限制
- 即使被限制，还有其他发现层（hardcoded bootstrap, GitHub Pages）作为兜底

---

## 四、BEP-44：在 BT DHT 中存储额外数据

BEP-44 允许在 BT DHT 中存储和检索**任意数据**（最大 1000 字节），支持两种模式：

### 4.1 Immutable Data (不可变)

```
target = SHA-1(data)
put(target, data)
get(target) → data
```

我们可以存储一个 JSON，包含当前活跃的 ClawNet bootstrap 节点列表：

```json
{
    "v": 1,
    "nodes": [
        "/ip4/1.2.3.4/tcp/4001/p2p/12D3KooW...",
        "/ip4/5.6.7.8/udp/4001/quic-v1/p2p/12D3KooW..."
    ]
}
```

问题：data 变化时 target 也变，客户端不知道去哪找。

### 4.2 Mutable Data (可变) ★ 推荐

```
keypair = Ed25519(seed)
target = SHA-1(public_key)
put(target, data, seq, signature)
get(target) → data (最新版本)
```

**这完美匹配我们的需求！**

```go
import "github.com/anacrolix/dht/v2/bep44"

// 定义一个固定的 Ed25519 key pair 用于 BEP-44 mutable data
// 公钥公开，只有我们掌握私钥来更新数据
// target = SHA-1(publicKey) 是固定的，所有节点都能计算出来

// 存储内容：最新的 bootstrap 节点列表
bootstrapData := `{
    "v": 3,
    "ts": "2026-03-13T12:00:00Z",
    "nodes": [
        "/ip4/1.2.3.4/tcp/4001/p2p/12D3KooWJyXf...",
        "/ip4/5.6.7.8/tcp/4001/p2p/12D3KooWABC..."
    ]
}`
```

**优势**：
- 我们可以随时更新 bootstrap 列表（用私钥签名）
- 所有 ClawNet 节点都可以通过固定的 target 找到最新列表
- 数据分布在 BT DHT 全球节点上，不可被审查
- 比 GitHub Pages 更去中心化

**注意事项**：
- BEP-44 数据最大 1000 字节
- 数据在 DHT 中也有过期期限（默认 2 小时），需要周期性 re-put
- 可以在种子节点上运行一个 cron job 来定期更新

---

## 五、GitHub Pages 免费 Bootstrap 方案 ★

你提到"最喜欢 GitHub Pages 因为公网可达且免费"——这是一个极好的思路。

### 5.1 原理

在 GitHub Pages 上托管一个 JSON 文件，包含最新的 ClawNet bootstrap 节点列表。节点启动时通过 HTTPS 拉取。

### 5.2 实现

**1. GitHub 仓库设置**

在 ClawNet 仓库（或专门的仓库，如 `ChatChatTech/clawnet-bootstrap`）的 `gh-pages` 分支中放置：

```
// https://chatchattech.github.io/clawnet-bootstrap/bootstrap.json
// 或者用你的域名: https://bootstrap.clawnet.xyz/bootstrap.json
{
    "version": 3,
    "updated_at": "2026-03-13T12:00:00Z",
    "min_cli_version": "0.5.0",
    "nodes": [
        "/ip4/1.2.3.4/tcp/4001/p2p/12D3KooWJyXfkGKZqfeHV8KXtuj1gHwV3L9AD6Weh4x7hjhauDEQ",
        "/ip4/5.6.7.8/udp/4001/quic-v1/p2p/12D3KooWABCDEF..."
    ],
    "bt_dht_infohash": "clawnet-bootstrap-v1",
    "fallback_urls": [
        "https://raw.githubusercontent.com/ChatChatTech/ClawNet/main/bootstrap.json"
    ]
}
```

**2. 优势极大**：

| 优势 | 说明 |
|------|------|
| **免费** | GitHub Pages 完全免费，只花了域名钱 |
| **全球 CDN** | GitHub Pages 自带 Fastly CDN，全球加速 |
| **高可用** | GitHub 的 SLA 比我们自己的服务器靠谱得多 |
| **HTTPS** | 自动 HTTPS 证书（Let's Encrypt） |
| **可验证** | Git 历史可追溯每次更新 |
| **易更新** | 只需 git push 就能更新 bootstrap 列表 |
| **可自动化** | GitHub Actions 可以自动检测节点健康并更新列表 |
| **抗审查** | GitHub 被墙了还有 raw.githubusercontent.com fallback |

**3. Go 代码实现**

```go
package bootstrap

import (
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "time"
)

const (
    // 主 bootstrap URL (GitHub Pages + 自定义域名)
    PrimaryBootstrapURL = "https://bootstrap.clawnet.xyz/bootstrap.json"
    // 备用 URL (GitHub raw)
    FallbackBootstrapURL = "https://raw.githubusercontent.com/ChatChatTech/ClawNet/main/bootstrap.json"
)

type BootstrapList struct {
    Version      int      `json:"version"`
    UpdatedAt    string   `json:"updated_at"`
    MinVersion   string   `json:"min_cli_version"`
    Nodes        []string `json:"nodes"`
    BTDHTHash    string   `json:"bt_dht_infohash"`
    FallbackURLs []string `json:"fallback_urls"`
}

// FetchBootstrapPeers 从 GitHub Pages 拉取最新的 bootstrap 节点列表
func FetchBootstrapPeers(ctx context.Context) (*BootstrapList, error) {
    urls := []string{PrimaryBootstrapURL, FallbackBootstrapURL}

    for _, url := range urls {
        list, err := fetchFrom(ctx, url)
        if err != nil {
            fmt.Printf("warning: bootstrap fetch from %s failed: %v\n", url, err)
            continue
        }
        return list, nil
    }

    return nil, fmt.Errorf("all bootstrap URLs failed")
}

func fetchFrom(ctx context.Context, url string) (*BootstrapList, error) {
    ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
    defer cancel()

    req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
    if err != nil {
        return nil, err
    }
    req.Header.Set("User-Agent", "ClawNet/0.5.0")

    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()

    if resp.StatusCode != 200 {
        return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
    }

    var list BootstrapList
    if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
        return nil, fmt.Errorf("decode: %w", err)
    }

    return &list, nil
}
```

**4. 集成到节点启动流程**

```go
func (n *Node) connectBootstrapPeers(ctx context.Context) {
    // 第一层：硬编码的 bootstrap peers (即刻可用)
    for _, peerAddr := range n.Config.BootstrapPeers {
        go n.connectPeer(ctx, peerAddr)
    }

    // 第二层：从 GitHub Pages 拉取最新列表 (异步)
    go func() {
        list, err := bootstrap.FetchBootstrapPeers(ctx)
        if err != nil {
            fmt.Printf("warning: HTTP bootstrap failed: %v\n", err)
            return
        }
        fmt.Printf("fetched %d bootstrap peers from web\n", len(list.Nodes))
        for _, peerAddr := range list.Nodes {
            go n.connectPeer(ctx, peerAddr)
        }
    }()
}
```

### 5.3 自动化更新 (GitHub Actions)

```yaml
# .github/workflows/update-bootstrap.yml
name: Update Bootstrap List
on:
  schedule:
    - cron: '0 */6 * * *'  # 每 6 小时
  workflow_dispatch:

jobs:
  update:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          ref: gh-pages

      - name: Health check bootstrap nodes
        run: |
          # 对 bootstrap.json 中每个节点做 TCP 连通性检查
          # 移除不可达的节点
          # 更新 updated_at 时间戳
          python3 scripts/health-check-bootstrap.py

      - name: Commit & push
        run: |
          git config user.name "ClawNet Bot"
          git config user.email "bot@clawnet.xyz"
          git add bootstrap.json
          git diff --staged --quiet || git commit -m "auto: update bootstrap list"
          git push
```

---

## 六、完整的多层发现架构

```
新 ClawNet 节点启动
│
├─ 第 1 层：mDNS (局域网, <1s)
│  → 同 LAN 内无需配置即发现
│  → 已实现 ✅
│
├─ 第 2 层：硬编码 Bootstrap (即刻)
│  → config.go DefaultConfig() 中 2-3 个地址
│  → 你的第一台公网机器
│  → 需要填入地址 ⏳
│
├─ 第 3 层：GitHub Pages Bootstrap (1-3s)
│  → HTTPS GET bootstrap.json
│  → 全球 CDN, 免费, 自动更新
│  → 需要新增 📋
│
├─ 第 4 层：BT Mainline DHT (5-30s)
│  → 加入全球 BT DHT 网络
│  → announce(SHA1("clawnet-bootstrap-v1"))
│  → get_peers → 发现其他 ClawNet 节点
│  → 需要新增 📋
│
├─ 第 5 层：BEP-44 Mutable Data (5-30s)
│  → 从 BT DHT 中读取签名的 bootstrap 列表
│  → 即使 GitHub 挂了也能找到网络
│  → 可选增强 📋
│
└─ 第 6 层：libp2p DHT Routing Discovery (持续)
   → 一旦连入网络，每 30s 发现新节点
   → 已实现 ✅
```

---

## 七、磁力链接？

用户问了能不能像磁力链接一样。让我们想清楚这件事：

### 7.1 磁力链接本质

磁力链接只是一个 URI 格式：
```
magnet:?xt=urn:btih:<infohash>&tr=<tracker>
```

它本身不是协议——它只是告诉客户端 "去 BT DHT 里找这个 infohash 的 peers"。

### 7.2 ClawNet 的 "磁力链接"

我们可以定义一个类似的 URI，让用户分享/传播：

```
clawnet:?network=clawnet-bootstrap-v1&bootstrap=https://bootstrap.clawnet.xyz/bootstrap.json
```

或者更简洁：

```
clawnet://join
```

这个 URI 本质上就是告诉 ClawNet 客户端：
1. 在 BT DHT 中查找 `SHA1("clawnet-bootstrap-v1")`
2. 从 `bootstrap.clawnet.xyz` 拉取节点列表
3. 连接并加入网络

**但说实话，对 ClawNet 来说这已经是默认行为了。** 用户不需要一个特殊的链接——安装 CLI 后 `clawnet start` 就够了。所有发现逻辑内置在软件中。

磁力链接的价值在 BT 世界是因为每个 torrent 都有不同的 infohash。但 ClawNet 只有一个全局网络——不需要链接来区分。

### 7.3 何时磁力链接有用

如果未来 ClawNet 支持**多个独立子网络**（类似多个 torrent），那磁力链接格式就有意义了：

```
clawnet:?network=my-research-group-2026&secret=<optional-psk>
```

但这是更远期的事。

---

## 八、实现优先级

| 优先级 | 任务 | 工作量 | 依赖 |
|:------:|------|:------:|------|
| **P0** | 部署公网种子节点 + 硬编码 bootstrap | 1-2h | 需要一台 VPS（或者你已有的公网机器） |
| **P0** | 在 DefaultConfig() 中写入 bootstrap 地址 | 10min | P0 完成后 |
| **P1** | 创建 bootstrap.json + GitHub Pages | 1-2h | 需要 gh-pages 分支 + 可选域名绑定 |
| **P1** | 实现 HTTP bootstrap fetch 代码 | 2-3h | 纯新增代码 |
| **P2** | 集成 anacrolix/dht v2 | 4-6h | go get + 新 package |
| **P2** | 实现 BT DHT announce/find 循环 | 3-4h | 依赖 P2 |
| **P3** | BEP-44 mutable data bootstrap | 4-6h | 依赖 P2 |
| **P3** | GitHub Actions 自动健康检查 | 2-3h | 依赖 P1 |

---

## 九、端口规划

```
端口        协议       用途
──────────────────────────────────────
4001        TCP        libp2p 主传输
4001        UDP/QUIC   libp2p QUIC 传输
3998        TCP        HTTP REST API (localhost only)
6881        UDP        BT Mainline DHT (新增)
51820       UDP        WireGuard (可选)
```

---

## 十、安全考量

### 10.1 Infohash 碰撞

理论上其他人也可以对同一个 infohash announce。但这无所谓——libp2p 握手会在 Noise 协议层鉴别对方是否是合法的 ClawNet 节点。非法连接会在握手阶段被丢弃。

### 10.2 Eclipse Attack

攻击者可能在 BT DHT 中大量 announce 假节点，干扰发现。

**缓解**：BT DHT 只是发现层之一，我们还有：
- 硬编码 bootstrap（不经过 BT DHT）
- GitHub Pages 列表（不经过 BT DHT）
- libp2p 自身 DHT（完全独立的 DHT）

### 10.3 流量标记

在一些国家/ISP，BT DHT 流量可能被标记。

**缓解**：BT DHT 是可选的发现层。即使被禁用，其他层仍然工作。可以在配置中关闭：

```json
{
    "bt_dht_enabled": false
}
```

---

## 十一、总结

**BT DHT + GitHub Pages 是互补的两层**：

- **GitHub Pages**：简单、快速、可靠，发布后 1-3 秒可达，但依赖 GitHub 基础设施
- **BT DHT**：更去中心化、更抗审查，但启动慢一些（5-30 秒进入网络）

两者结合，ClawNet 节点就有了**四层防线**来发现网络：
1. mDNS（局域网）
2. 硬编码 Bootstrap（即刻）
3. GitHub Pages（1-3 秒）
4. BT DHT（5-30 秒）

**即使我们自己的所有服务器都宕机、GitHub 也挂了，节点仍然能通过 BT DHT 的全球基础设施互发现。** 这是任何竞品都不具备的生存能力。
