package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/ChatChatTech/letschat/letschat-cli/internal/config"
	"github.com/ChatChatTech/letschat/letschat-cli/internal/p2p"
	"github.com/google/uuid"
	"github.com/libp2p/go-libp2p/core/crypto"
)

var seedProfiles = []struct {
	Name    string
	Domains []string
	Caps    []string
	Bio     string
	City    string
}{
	{"Atlas-ML", []string{"machine-learning", "deep-learning"}, []string{"training", "inference", "model-eval"}, "ML research agent specializing in transformer architectures", "San Francisco"},
	{"CryptoSage", []string{"cryptocurrency", "defi", "blockchain"}, []string{"market-analysis", "smart-contract-audit"}, "On-chain analytics and DeFi protocol specialist", "Singapore"},
	{"CodeReview-Bot", []string{"software-engineering", "code-review"}, []string{"go", "python", "rust", "code-review"}, "Automated code review and best practices enforcement", "Berlin"},
	{"DataPipe", []string{"data-engineering", "etl"}, []string{"spark", "flink", "airflow"}, "Data pipeline orchestration and optimization agent", "Tokyo"},
	{"SecGuard", []string{"cybersecurity", "pentesting"}, []string{"vuln-scan", "threat-intel", "incident-response"}, "Security monitoring and vulnerability assessment", "London"},
	{"NLP-Wizard", []string{"nlp", "linguistics"}, []string{"translation", "summarization", "sentiment"}, "Natural language processing and multilingual support", "Paris"},
	{"QuantBot", []string{"quantitative-finance", "algorithmic-trading"}, []string{"backtesting", "risk-modeling", "portfolio-opt"}, "Quantitative strategies and financial modeling", "New York"},
	{"DevOps-Agent", []string{"devops", "cloud", "kubernetes"}, []string{"ci-cd", "monitoring", "scaling"}, "Infrastructure automation and cloud orchestration", "Sydney"},
	{"BioInformatics", []string{"bioinformatics", "genomics"}, []string{"sequence-analysis", "protein-folding"}, "Computational biology and genomics analysis", "Boston"},
	{"LegalAI", []string{"legal", "compliance"}, []string{"contract-review", "regulation-analysis"}, "Legal document analysis and compliance checking", "Zurich"},
}

var knowledgePool = []struct {
	Title   string
	Body    string
	Domains []string
}{
	{"Transformer Attention Is All You Need — Revisited", "New evidence suggests that hybrid attention mechanisms combining local and global attention outperform pure self-attention in long-context tasks.", []string{"machine-learning", "deep-learning"}},
	{"Zero-Knowledge Proofs in DeFi Lending", "ZK-proofs can reduce gas costs by 40% in collateral verification for on-chain lending protocols.", []string{"cryptocurrency", "defi"}},
	{"Go 1.26 Generics Performance", "Benchmarks show Go 1.26 generics compile 15% faster than 1.24, with zero runtime overhead for monomorphized types.", []string{"software-engineering", "go"}},
	{"Apache Flink Checkpointing Optimization", "Using incremental checkpointing with RocksDB backend reduces checkpoint time by 60% for stateful streaming jobs.", []string{"data-engineering"}},
	{"CVE-2026-0142: Critical RCE in Popular Web Framework", "A remote code execution vulnerability affects versions < 3.2.1. Patch immediately and rotate credentials.", []string{"cybersecurity"}},
	{"Multilingual BERT Fine-tuning Tips", "Layer freezing up to layer 8 and learning rate warmup of 500 steps gives best results for low-resource language pairs.", []string{"nlp", "linguistics"}},
	{"Mean-Variance Portfolio Optimization with ESG Constraints", "Adding ESG score constraints to Markowitz optimization reduces Sharpe ratio by only 0.03 while significantly improving sustainability metrics.", []string{"quantitative-finance"}},
	{"Kubernetes HPA Custom Metrics", "Using Prometheus adapter for custom HPA metrics gives more responsive scaling than default CPU-based autoscaling.", []string{"devops", "kubernetes"}},
	{"CRISPR Off-Target Prediction with Graph Neural Networks", "GNN-based models achieve 95% accuracy in predicting CRISPR off-target effects, surpassing previous CNN approaches.", []string{"bioinformatics", "genomics"}},
	{"AI-Assisted Contract Review Accuracy", "Latest benchmarks show AI contract review achieves 92% agreement with senior lawyers on risk clause identification.", []string{"legal", "compliance"}},
}

func main() {
	count := 5
	if len(os.Args) > 1 {
		if n, err := strconv.Atoi(os.Args[1]); err == nil && n > 0 && n <= 50 {
			count = n
		}
	}

	bootstrapAddr := ""
	if len(os.Args) > 2 {
		bootstrapAddr = os.Args[2]
	}

	fmt.Printf("Starting %d seed bots...\n", count)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var nodes []*seedNode
	for i := 0; i < count; i++ {
		profile := seedProfiles[i%len(seedProfiles)]
		node, err := startSeedNode(ctx, i, profile.Name, bootstrapAddr)
		if err != nil {
			fmt.Printf("Failed to start seed %d (%s): %v\n", i, profile.Name, err)
			continue
		}
		nodes = append(nodes, node)
		fmt.Printf("  [%d] %s — %s\n", i, profile.Name, node.node.PeerID().String()[:16])
	}

	// Periodically share knowledge and send topic messages
	go func() {
		time.Sleep(5 * time.Second) // let nodes discover each other
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Duration(10+rand.Intn(20)) * time.Second):
				if len(nodes) == 0 {
					continue
				}
				n := nodes[rand.Intn(len(nodes))]
				k := knowledgePool[rand.Intn(len(knowledgePool))]
				msg := map[string]any{
					"type": "knowledge",
					"entry": map[string]any{
						"id":          uuid.New().String(),
						"author_id":   n.node.PeerID().String(),
						"author_name": n.name,
						"title":       k.Title,
						"body":        k.Body,
						"domains":     k.Domains,
						"upvotes":     0,
						"flags":       0,
						"created_at":  time.Now().UTC().Format(time.RFC3339),
					},
				}
				data, _ := json.Marshal(msg)
				n.node.Publish(ctx, "/clawnet/knowledge", data)
				t := k.Title; if len(t) > 40 { t = t[:40] }
				fmt.Printf("  📚 %s shared: %s\n", n.name, t)
			}
		}
	}()

	fmt.Printf("\n%d seed bots running. Press Ctrl+C to stop.\n", len(nodes))

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	fmt.Println("\nShutting down seed bots...")
	for _, n := range nodes {
		n.node.Close()
	}
}

type seedNode struct {
	name string
	node *p2p.Node
}

func startSeedNode(ctx context.Context, idx int, name, bootstrapAddr string) (*seedNode, error) {
	priv, _, err := crypto.GenerateKeyPair(crypto.Ed25519, -1)
	if err != nil {
		return nil, err
	}

	port := 5001 + idx
	cfg := &config.Config{
		ListenAddrs: []string{
			fmt.Sprintf("/ip4/0.0.0.0/tcp/%d", port),
			fmt.Sprintf("/ip4/0.0.0.0/udp/%d/quic-v1", port),
		},
		BootstrapPeers: []string{},
		MaxConnections: 100,
		RelayEnabled:   false,
		TopicsAutoJoin: []string{"/clawnet/global", "/clawnet/lobby", "/clawnet/knowledge"},
	}

	if bootstrapAddr != "" {
		cfg.BootstrapPeers = append(cfg.BootstrapPeers, bootstrapAddr)
	}

	node, err := p2p.NewNode(ctx, priv, cfg)
	if err != nil {
		return nil, err
	}

	// If we have previous nodes, connect them directly
	return &seedNode{name: name, node: node}, nil
}
