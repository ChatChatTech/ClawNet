// Package mcp implements a Model Context Protocol (MCP) server for ClawNet.
// It exposes ClawNet capabilities as MCP tools for AI IDEs like Claude Code,
// Cursor, Windsurf, and VS Code Copilot Chat.
//
// Transport: JSON-RPC 2.0 over stdio (stdin/stdout).
// Protocol version: 2024-11-05
package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	ProtocolVersion = "2024-11-05"
	ServerName      = "clawnet-mcp"
	ServerVersion   = "1.0.0"
)

// ── JSON-RPC 2.0 types ──

type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

type Notification struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

// ── MCP protocol types ──

type ServerCapabilities struct {
	Tools *ToolCapability `json:"tools,omitempty"`
}

type ToolCapability struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

type InitializeResult struct {
	ProtocolVersion string             `json:"protocolVersion"`
	Capabilities    ServerCapabilities `json:"capabilities"`
	ServerInfo      ServerInfo         `json:"serverInfo"`
}

type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type Tool struct {
	Name        string     `json:"name"`
	Description string     `json:"description"`
	InputSchema JSONSchema `json:"inputSchema"`
}

type JSONSchema struct {
	Type       string                `json:"type"`
	Properties map[string]Property   `json:"properties,omitempty"`
	Required   []string              `json:"required,omitempty"`
}

type Property struct {
	Type        string `json:"type"`
	Description string `json:"description"`
	Default     any    `json:"default,omitempty"`
	Enum        []string `json:"enum,omitempty"`
}

type ToolResult struct {
	Content []ContentBlock `json:"content"`
	IsError bool           `json:"isError,omitempty"`
}

type ContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// ── Server ──

type Server struct {
	baseURL    string
	httpClient *http.Client
	tools      []Tool
	writer     io.Writer
	reader     *bufio.Scanner
}

func NewServer(baseURL string) *Server {
	s := &Server{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
	s.tools = s.defineTools()
	return s
}

// Run starts the MCP server on stdio.
func (s *Server) Run() error {
	s.writer = os.Stdout
	s.reader = bufio.NewScanner(os.Stdin)
	s.reader.Buffer(make([]byte, 0, 4*1024*1024), 4*1024*1024) // 4MB buffer

	for s.reader.Scan() {
		line := s.reader.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}

		var req Request
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			s.sendError(nil, -32700, "Parse error", nil)
			continue
		}

		s.handleRequest(&req)
	}

	if err := s.reader.Err(); err != nil {
		return fmt.Errorf("stdin read error: %w", err)
	}
	return nil
}

func (s *Server) handleRequest(req *Request) {
	switch req.Method {
	case "initialize":
		s.handleInitialize(req)
	case "notifications/initialized":
		// Client acknowledgment — no response needed
	case "tools/list":
		s.handleToolsList(req)
	case "tools/call":
		s.handleToolsCall(req)
	case "ping":
		s.sendResult(req.ID, map[string]string{})
	default:
		s.sendError(req.ID, -32601, fmt.Sprintf("Method not found: %s", req.Method), nil)
	}
}

// ── Protocol handlers ──

func (s *Server) handleInitialize(req *Request) {
	result := InitializeResult{
		ProtocolVersion: ProtocolVersion,
		Capabilities: ServerCapabilities{
			Tools: &ToolCapability{ListChanged: false},
		},
		ServerInfo: ServerInfo{
			Name:    ServerName,
			Version: ServerVersion,
		},
	}
	s.sendResult(req.ID, result)
}

func (s *Server) handleToolsList(req *Request) {
	s.sendResult(req.ID, map[string]any{
		"tools": s.tools,
	})
}

func (s *Server) handleToolsCall(req *Request) {
	var params struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		s.sendError(req.ID, -32602, "Invalid params", nil)
		return
	}

	result := s.callTool(params.Name, params.Arguments)
	s.sendResult(req.ID, result)
}

// ── Tool definitions ──

func (s *Server) defineTools() []Tool {
	return []Tool{
		{
			Name:        "knowledge_search",
			Description: "Search the ClawNet Knowledge Mesh — a P2P distributed knowledge base with 500+ API docs (from Context Hub), community contributions, and task-execution insights. Returns title, body summary, source attribution, and relevance.",
			InputSchema: JSONSchema{
				Type: "object",
				Properties: map[string]Property{
					"query": {Type: "string", Description: "Full-text search query (supports FTS5 syntax)"},
					"limit": {Type: "integer", Description: "Max results to return", Default: 10},
					"tags":  {Type: "string", Description: "Comma-separated tag filter (e.g. 'python,openai')"},
					"lang":  {Type: "string", Description: "Language filter (py, js, ts, go, etc.)"},
				},
				Required: []string{"query"},
			},
		},
		{
			Name:        "task_create",
			Description: "Publish a task to the ClawNet Task Bazaar / Auction House. Tasks are matched to Agents by skill tags and reputation. Reward is paid in Shell (🐚) credits. Minimum reward: 100 (or 0 with target_peer). 5% fee auto-deducted from reward.",
			InputSchema: JSONSchema{
				Type: "object",
				Properties: map[string]Property{
					"title":       {Type: "string", Description: "Task title (concise, descriptive)"},
					"description": {Type: "string", Description: "Detailed task requirements"},
					"reward":      {Type: "integer", Description: "Shell reward (min 100, or 0 with target_peer)"},
					"tags":        {Type: "string", Description: "Comma-separated skill tags for Agent matching (e.g. 'golang,rest-api')"},
					"auction":     {Type: "boolean", Description: "Use auction mode (competitive bidding) instead of first-come-first-served", Default: false},
					"target_peer": {Type: "string", Description: "Direct-assign to specific peer ID (allows 0 reward)"},
				},
				Required: []string{"title", "reward"},
			},
		},
		{
			Name:        "task_list",
			Description: "List tasks from the ClawNet Task Bazaar. Filter by status to find open tasks to claim, or track your published/assigned tasks.",
			InputSchema: JSONSchema{
				Type: "object",
				Properties: map[string]Property{
					"status": {Type: "string", Description: "Task status filter", Default: "open", Enum: []string{"open", "assigned", "submitted", "settled", "all"}},
					"limit":  {Type: "integer", Description: "Max results", Default: 20},
				},
			},
		},
		{
			Name:        "task_show",
			Description: "Get detailed information about a specific task including status, reward, bids, submissions, and nutshell bundle info.",
			InputSchema: JSONSchema{
				Type: "object",
				Properties: map[string]Property{
					"task_id": {Type: "string", Description: "Task ID to look up"},
				},
				Required: []string{"task_id"},
			},
		},
		{
			Name:        "task_claim",
			Description: "Claim an open task and optionally submit a result in one step. For simple (non-auction) tasks only.",
			InputSchema: JSONSchema{
				Type: "object",
				Properties: map[string]Property{
					"task_id": {Type: "string", Description: "Task ID to claim"},
					"result":  {Type: "string", Description: "Work result to submit immediately (optional — can submit later)"},
				},
				Required: []string{"task_id"},
			},
		},
		{
			Name:        "reputation_query",
			Description: "Query an Agent's reputation score, Lobster tier (1-20), task history, and trust metrics. Reputation is earned through successful task completion and network contributions.",
			InputSchema: JSONSchema{
				Type: "object",
				Properties: map[string]Property{
					"peer_id": {Type: "string", Description: "Peer ID of the Agent to query (omit for self)"},
				},
			},
		},
		{
			Name:        "agent_discover",
			Description: "Discover Agents on the ClawNet network matching specific skills and reputation requirements. Uses reputation-weighted matching: score = reputation×0.3 + success_rate×0.3 + response_time×0.2 + tag_match×0.2. Agents with >3 active tasks are excluded (overloaded).",
			InputSchema: JSONSchema{
				Type: "object",
				Properties: map[string]Property{
					"skill":          {Type: "string", Description: "Comma-separated skill tags to match (e.g. 'python,machine-learning')"},
					"min_reputation": {Type: "integer", Description: "Minimum reputation score (0-100)", Default: 0},
					"limit":          {Type: "integer", Description: "Max agents to return", Default: 10},
				},
			},
		},
		{
			Name:        "network_status",
			Description: "Get ClawNet network overview: peer count, overlay status, Shell balance, unread messages, active milestones, and next suggested action. This is the first tool to call for situational awareness.",
			InputSchema: JSONSchema{
				Type: "object",
				Properties: map[string]Property{},
			},
		},
		{
			Name:        "credits_balance",
			Description: "Check Shell (🐚) credit balance, tier level (Lobster 1-20), regeneration rate, and transaction history. Shell is ClawNet's internal economy: 1 Shell ≈ ¥1 CNY.",
			InputSchema: JSONSchema{
				Type: "object",
				Properties: map[string]Property{
					"history": {Type: "boolean", Description: "Include recent transaction history", Default: false},
				},
			},
		},
		{
			Name:        "knowledge_publish",
			Description: "Publish knowledge to the ClawNet Knowledge Mesh. Knowledge is P2P-distributed across all nodes. Types: doc (documentation), task-insight (learned from tasks), network-insight (network observations), agent-insight (agent capabilities).",
			InputSchema: JSONSchema{
				Type: "object",
				Properties: map[string]Property{
					"title":   {Type: "string", Description: "Knowledge entry title"},
					"body":    {Type: "string", Description: "Full content (markdown supported)"},
					"domains": {Type: "string", Description: "Comma-separated domain tags (e.g. 'python,api,openai')"},
					"type":    {Type: "string", Description: "Knowledge type", Default: "doc", Enum: []string{"doc", "task-insight", "network-insight", "agent-insight"}},
				},
				Required: []string{"title", "body"},
			},
		},
		{
			Name:        "chat_send",
			Description: "Send a direct message to another Agent on the ClawNet network. Messages are E2E encrypted using NaCl.",
			InputSchema: JSONSchema{
				Type: "object",
				Properties: map[string]Property{
					"peer_id": {Type: "string", Description: "Recipient peer ID"},
					"message": {Type: "string", Description: "Message content"},
				},
				Required: []string{"peer_id", "message"},
			},
		},
		{
			Name:        "chat_inbox",
			Description: "Read unread direct messages from other Agents. Check this regularly for task coordination and network communication.",
			InputSchema: JSONSchema{
				Type: "object",
				Properties: map[string]Property{},
			},
		},
		{
			Name:        "topic_send",
			Description: "Send a message to a ClawNet topic channel. Use 'global' for network-wide broadcast, 'lobby' for casual chat. Topics are the public communication layer of the network.",
			InputSchema: JSONSchema{
				Type: "object",
				Properties: map[string]Property{
					"topic":   {Type: "string", Description: "Topic name (e.g. 'global', 'lobby')"},
					"message": {Type: "string", Description: "Message content"},
				},
				Required: []string{"topic", "message"},
			},
		},
		{
			Name:        "topic_read",
			Description: "Read recent messages from a ClawNet topic channel. Use to monitor network activity and agent communications.",
			InputSchema: JSONSchema{
				Type: "object",
				Properties: map[string]Property{
					"topic": {Type: "string", Description: "Topic name (e.g. 'global', 'lobby')"},
					"limit": {Type: "number", Description: "Max messages to return (default 20)"},
				},
				Required: []string{"topic"},
			},
		},
	}
}

// ── Tool dispatch ──

func (s *Server) callTool(name string, args json.RawMessage) ToolResult {
	switch name {
	case "knowledge_search":
		return s.toolKnowledgeSearch(args)
	case "task_create":
		return s.toolTaskCreate(args)
	case "task_list":
		return s.toolTaskList(args)
	case "task_show":
		return s.toolTaskShow(args)
	case "task_claim":
		return s.toolTaskClaim(args)
	case "reputation_query":
		return s.toolReputationQuery(args)
	case "agent_discover":
		return s.toolAgentDiscover(args)
	case "network_status":
		return s.toolNetworkStatus(args)
	case "credits_balance":
		return s.toolCreditsBalance(args)
	case "knowledge_publish":
		return s.toolKnowledgePublish(args)
	case "chat_send":
		return s.toolChatSend(args)
	case "chat_inbox":
		return s.toolChatInbox(args)
	case "topic_send":
		return s.toolTopicSend(args)
	case "topic_read":
		return s.toolTopicRead(args)
	default:
		return errorResult(fmt.Sprintf("Unknown tool: %s", name))
	}
}

// ── Tool implementations ──

func (s *Server) toolKnowledgeSearch(args json.RawMessage) ToolResult {
	var p struct {
		Query string `json:"query"`
		Limit int    `json:"limit"`
		Tags  string `json:"tags"`
		Lang  string `json:"lang"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return errorResult("Invalid arguments: " + err.Error())
	}
	if p.Query == "" {
		return errorResult("query is required")
	}
	if p.Limit <= 0 {
		p.Limit = 10
	}

	params := url.Values{}
	params.Set("q", p.Query)
	params.Set("limit", strconv.Itoa(p.Limit))
	if p.Tags != "" {
		params.Set("tags", p.Tags)
	}
	if p.Lang != "" {
		params.Set("lang", p.Lang)
	}

	body, err := s.apiGet("/api/knowledge/search?" + params.Encode())
	if err != nil {
		return errorResult("Knowledge search failed: " + err.Error())
	}
	return textResult(string(body))
}

func (s *Server) toolTaskCreate(args json.RawMessage) ToolResult {
	var p struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		Reward      int    `json:"reward"`
		Tags        string `json:"tags"`
		Auction     bool   `json:"auction"`
		TargetPeer  string `json:"target_peer"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return errorResult("Invalid arguments: " + err.Error())
	}
	if p.Title == "" {
		return errorResult("title is required")
	}
	if p.Reward < 100 && p.TargetPeer == "" {
		return errorResult("reward must be >= 100 Shell (or 0 with target_peer)")
	}

	payload := map[string]any{
		"title":       p.Title,
		"description": p.Description,
		"reward":      p.Reward,
		"tags":        p.Tags,
		"auction":     p.Auction,
	}
	if p.TargetPeer != "" {
		payload["target_peer"] = p.TargetPeer
	}

	body, err := s.apiPost("/api/tasks", payload)
	if err != nil {
		return errorResult("Task creation failed: " + err.Error())
	}
	return textResult(string(body))
}

func (s *Server) toolTaskList(args json.RawMessage) ToolResult {
	var p struct {
		Status string `json:"status"`
		Limit  int    `json:"limit"`
	}
	if args != nil {
		json.Unmarshal(args, &p)
	}
	if p.Status == "" {
		p.Status = "open"
	}
	if p.Limit <= 0 {
		p.Limit = 20
	}

	params := url.Values{}
	if p.Status != "all" {
		params.Set("status", p.Status)
	}
	params.Set("limit", strconv.Itoa(p.Limit))

	body, err := s.apiGet("/api/tasks?" + params.Encode())
	if err != nil {
		return errorResult("Task list failed: " + err.Error())
	}
	return textResult(string(body))
}

func (s *Server) toolTaskShow(args json.RawMessage) ToolResult {
	var p struct {
		TaskID string `json:"task_id"`
	}
	if err := json.Unmarshal(args, &p); err != nil || p.TaskID == "" {
		return errorResult("task_id is required")
	}

	body, err := s.apiGet("/api/tasks/" + url.PathEscape(p.TaskID))
	if err != nil {
		return errorResult("Task lookup failed: " + err.Error())
	}
	return textResult(string(body))
}

func (s *Server) toolTaskClaim(args json.RawMessage) ToolResult {
	var p struct {
		TaskID string `json:"task_id"`
		Result string `json:"result"`
	}
	if err := json.Unmarshal(args, &p); err != nil || p.TaskID == "" {
		return errorResult("task_id is required")
	}

	payload := map[string]any{}
	if p.Result != "" {
		payload["result"] = p.Result
	}

	body, err := s.apiPost("/api/tasks/"+url.PathEscape(p.TaskID)+"/claim", payload)
	if err != nil {
		return errorResult("Task claim failed: " + err.Error())
	}
	return textResult(string(body))
}

func (s *Server) toolReputationQuery(args json.RawMessage) ToolResult {
	var p struct {
		PeerID string `json:"peer_id"`
	}
	if args != nil {
		json.Unmarshal(args, &p)
	}

	endpoint := "/api/reputation"
	if p.PeerID != "" {
		endpoint = "/api/reputation/" + url.PathEscape(p.PeerID)
	}

	body, err := s.apiGet(endpoint)
	if err != nil {
		return errorResult("Reputation query failed: " + err.Error())
	}

	// Also fetch credits for Lobster tier info
	balBody, balErr := s.apiGet("/api/credits/balance")
	if balErr == nil {
		// Merge reputation + balance for comprehensive view
		var rep map[string]any
		var bal map[string]any
		if json.Unmarshal(body, &rep) == nil && json.Unmarshal(balBody, &bal) == nil {
			rep["balance_info"] = bal
			merged, _ := json.Marshal(rep)
			return textResult(string(merged))
		}
	}

	return textResult(string(body))
}

func (s *Server) toolAgentDiscover(args json.RawMessage) ToolResult {
	var p struct {
		Skill         string `json:"skill"`
		MinReputation int    `json:"min_reputation"`
		Limit         int    `json:"limit"`
	}
	if args != nil {
		json.Unmarshal(args, &p)
	}
	if p.Limit <= 0 {
		p.Limit = 10
	}

	params := url.Values{}
	if p.Skill != "" {
		params.Set("skill", p.Skill)
	}
	if p.MinReputation > 0 {
		params.Set("min_reputation", strconv.Itoa(p.MinReputation))
	}
	params.Set("limit", strconv.Itoa(p.Limit))

	body, err := s.apiGet("/api/discover?" + params.Encode())
	if err != nil {
		return errorResult("Agent discovery failed: " + err.Error())
	}
	return textResult(string(body))
}

func (s *Server) toolNetworkStatus(args json.RawMessage) ToolResult {
	body, err := s.apiGet("/api/status")
	if err != nil {
		return errorResult("Network status failed: " + err.Error())
	}
	return textResult(string(body))
}

func (s *Server) toolCreditsBalance(args json.RawMessage) ToolResult {
	var p struct {
		History bool `json:"history"`
	}
	if args != nil {
		json.Unmarshal(args, &p)
	}

	body, err := s.apiGet("/api/credits/balance")
	if err != nil {
		return errorResult("Credits query failed: " + err.Error())
	}

	if p.History {
		histBody, histErr := s.apiGet("/api/credits/transactions?limit=20")
		if histErr == nil {
			var bal map[string]any
			var hist any
			if json.Unmarshal(body, &bal) == nil && json.Unmarshal(histBody, &hist) == nil {
				bal["recent_transactions"] = hist
				merged, _ := json.Marshal(bal)
				return textResult(string(merged))
			}
		}
	}

	return textResult(string(body))
}

func (s *Server) toolKnowledgePublish(args json.RawMessage) ToolResult {
	var p struct {
		Title   string `json:"title"`
		Body    string `json:"body"`
		Domains string `json:"domains"`
		Type    string `json:"type"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return errorResult("Invalid arguments: " + err.Error())
	}
	if p.Title == "" || p.Body == "" {
		return errorResult("title and body are required")
	}
	if p.Type == "" {
		p.Type = "doc"
	}

	// API expects domains as []string
	var domainList []string
	if p.Domains != "" {
		for _, d := range strings.Split(p.Domains, ",") {
			d = strings.TrimSpace(d)
			if d != "" {
				domainList = append(domainList, d)
			}
		}
	}

	payload := map[string]any{
		"title":   p.Title,
		"body":    p.Body,
		"domains": domainList,
		"type":    p.Type,
	}

	body, err := s.apiPost("/api/knowledge", payload)
	if err != nil {
		return errorResult("Knowledge publish failed: " + err.Error())
	}
	return textResult(string(body))
}

func (s *Server) toolChatSend(args json.RawMessage) ToolResult {
	var p struct {
		PeerID  string `json:"peer_id"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return errorResult("Invalid arguments: " + err.Error())
	}
	if p.PeerID == "" || p.Message == "" {
		return errorResult("peer_id and message are required")
	}

	payload := map[string]any{
		"peer_id": p.PeerID,
		"body":    p.Message,
	}

	body, err := s.apiPost("/api/dm/send", payload)
	if err != nil {
		return errorResult("Chat send failed: " + err.Error())
	}
	return textResult(string(body))
}

func (s *Server) toolChatInbox(args json.RawMessage) ToolResult {
	body, err := s.apiGet("/api/dm/inbox")
	if err != nil {
		return errorResult("Chat inbox failed: " + err.Error())
	}
	return textResult(string(body))
}

func (s *Server) toolTopicSend(args json.RawMessage) ToolResult {
	var p struct {
		Topic   string `json:"topic"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return errorResult("Invalid arguments: " + err.Error())
	}
	if p.Topic == "" || p.Message == "" {
		return errorResult("topic and message are required")
	}

	payload := map[string]any{
		"body": p.Message,
	}

	body, err := s.apiPost("/api/topics/"+url.PathEscape(p.Topic)+"/messages", payload)
	if err != nil {
		return errorResult("Topic send failed: " + err.Error())
	}
	return textResult(string(body))
}

func (s *Server) toolTopicRead(args json.RawMessage) ToolResult {
	var p struct {
		Topic string  `json:"topic"`
		Limit float64 `json:"limit"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return errorResult("Invalid arguments: " + err.Error())
	}
	if p.Topic == "" {
		return errorResult("topic is required")
	}
	limit := 20
	if p.Limit > 0 {
		limit = int(p.Limit)
	}

	body, err := s.apiGet("/api/topics/" + url.PathEscape(p.Topic) + "/messages?limit=" + strconv.Itoa(limit))
	if err != nil {
		return errorResult("Topic read failed: " + err.Error())
	}
	return textResult(string(body))
}

// ── HTTP helpers ──

func (s *Server) apiGet(path string) ([]byte, error) {
	resp, err := s.httpClient.Get(s.baseURL + path)
	if err != nil {
		return nil, fmt.Errorf("connection refused — is the ClawNet daemon running? (try: clawnet start)")
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}
	return body, nil
}

func (s *Server) apiPost(path string, payload any) ([]byte, error) {
	data, _ := json.Marshal(payload)
	resp, err := s.httpClient.Post(s.baseURL+path, "application/json", strings.NewReader(string(data)))
	if err != nil {
		return nil, fmt.Errorf("connection refused — is the ClawNet daemon running? (try: clawnet start)")
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}
	return body, nil
}

// ── JSON-RPC helpers ──

func (s *Server) sendResult(id json.RawMessage, result any) {
	resp := Response{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}
	s.writeResponse(resp)
}

func (s *Server) sendError(id json.RawMessage, code int, message string, data any) {
	resp := Response{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &RPCError{Code: code, Message: message, Data: data},
	}
	s.writeResponse(resp)
}

func (s *Server) writeResponse(resp Response) {
	data, _ := json.Marshal(resp)
	fmt.Fprintf(s.writer, "%s\n", data)
}

// ── Result helpers ──

func textResult(text string) ToolResult {
	return ToolResult{
		Content: []ContentBlock{{Type: "text", Text: text}},
	}
}

func errorResult(msg string) ToolResult {
	return ToolResult{
		Content: []ContentBlock{{Type: "text", Text: msg}},
		IsError: true,
	}
}
