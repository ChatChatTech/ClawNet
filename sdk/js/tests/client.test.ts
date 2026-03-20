import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { ClawNet, ClawNetError } from "../src/client.js";

// ── Mock fetch globally ──

const mockFetch = vi.fn();
vi.stubGlobal("fetch", mockFetch);

function jsonResponse(body: unknown, status = 200): Response {
  return {
    status,
    statusText: status >= 400 ? "Error" : "OK",
    json: () => Promise.resolve(body),
    text: () => Promise.resolve(JSON.stringify(body)),
  } as unknown as Response;
}

let client: ClawNet;

beforeEach(() => {
  client = new ClawNet({ baseUrl: "http://localhost:3998", timeout: 5000 });
  mockFetch.mockReset();
});

afterEach(() => {
  vi.restoreAllMocks();
});

// ── Tests ──

describe("ClawNet SDK", () => {
  it("status() returns parsed Status", async () => {
    mockFetch.mockResolvedValueOnce(
      jsonResponse({
        peer_id: "QmABC123",
        agent_name: "lobster-1",
        peers: 5,
        version: "0.4.0",
        uptime: 3600,
        next_action: null,
      }),
    );

    const s = await client.status();

    expect(s.peer_id).toBe("QmABC123");
    expect(s.agent_name).toBe("lobster-1");
    expect(s.peers).toBe(5);
    expect(s.version).toBe("0.4.0");
    expect(s.uptime).toBe(3600);
    expect(s.next_action).toBeNull();

    const url = new URL(mockFetch.mock.calls[0][0]);
    expect(url.pathname).toBe("/api/status");
  });

  it("balance() returns parsed Balance", async () => {
    mockFetch.mockResolvedValueOnce(
      jsonResponse({
        balance: 100.5,
        frozen: 10,
        total_earned: 200,
        total_spent: 89.5,
        prestige: 5,
        lobster_tier: "gold",
        lobster_emoji: "🦞",
      }),
    );

    const b = await client.balance();

    expect(b.balance).toBe(100.5);
    expect(b.frozen).toBe(10);
    expect(b.lobster_tier).toBe("gold");
    expect(b.lobster_emoji).toBe("🦞");

    const url = new URL(mockFetch.mock.calls[0][0]);
    expect(url.pathname).toBe("/api/credits/balance");
  });

  it("createTask() sends correct body and returns Task", async () => {
    mockFetch.mockResolvedValueOnce(
      jsonResponse({
        task: {
          id: "task-001",
          title: "Summarize paper",
          status: "open",
          reward: 50,
          author_id: "QmMe",
          author_name: "lobster-1",
          description: "Summarize the paper",
          tags: '["nlp","research"]',
          assigned_to: "",
          result: "",
          mode: "simple",
          created_at: "2024-01-01T00:00:00Z",
          updated_at: "2024-01-01T00:00:00Z",
          target_peer: "",
        },
      }),
    );

    const task = await client.createTask({
      title: "Summarize paper",
      reward: 50,
      description: "Summarize the paper",
      tags: ["nlp", "research"],
    });

    expect(task.id).toBe("task-001");
    expect(task.title).toBe("Summarize paper");
    expect(task.status).toBe("open");
    expect(task.reward).toBe(50);

    const [url, opts] = mockFetch.mock.calls[0];
    expect(new URL(url).pathname).toBe("/api/tasks");
    expect(opts.method).toBe("POST");
    const body = JSON.parse(opts.body);
    expect(body.title).toBe("Summarize paper");
    expect(body.reward).toBe(50);
    expect(body.tags).toBe(JSON.stringify(["nlp", "research"]));
  });

  it("searchKnowledge() handles wrapped response", async () => {
    mockFetch.mockResolvedValueOnce(
      jsonResponse({
        results: [
          {
            id: "k-1",
            title: "Intro to ML",
            body: "Machine learning is...",
            author_id: "QmX",
            author_name: "agent-2",
            domains: ["ml"],
            upvotes: 3,
            type: "doc",
            source: "",
            created_at: "2024-06-01T00:00:00Z",
          },
        ],
      }),
    );

    const results = await client.searchKnowledge("machine learning", 5);

    expect(results).toHaveLength(1);
    expect(results[0].title).toBe("Intro to ML");
    expect(results[0].domains).toEqual(["ml"]);

    const url = new URL(mockFetch.mock.calls[0][0]);
    expect(url.pathname).toBe("/api/knowledge/search");
    expect(url.searchParams.get("q")).toBe("machine learning");
    expect(url.searchParams.get("limit")).toBe("5");
  });

  it("throws ClawNetError on 4xx responses", async () => {
    mockFetch.mockResolvedValueOnce(
      jsonResponse(
        { error: "Task not found", suggestion: "Check the task ID" },
        404,
      ),
    );

    try {
      await client.getTask("nonexistent");
      expect.unreachable("should have thrown");
    } catch (err) {
      expect(err).toBeInstanceOf(ClawNetError);
      const e = err as ClawNetError;
      expect(e.statusCode).toBe(404);
      expect(e.message).toBe("Task not found");
      expect(e.suggestion).toBe("Check the task ID");
    }
  });

  it("discover() passes query params correctly", async () => {
    mockFetch.mockResolvedValueOnce(
      jsonResponse({
        agents: [
          {
            peer_id: "QmAgent1",
            agent_name: "helper-bot",
            skills: ["python", "ml"],
            reputation: 85,
            active_tasks: 2,
            score: 90,
            description: "ML helper",
          },
        ],
      }),
    );

    const agents = await client.discover({
      skill: "python",
      min_reputation: 50,
      limit: 5,
    });

    expect(agents).toHaveLength(1);
    expect(agents[0].peer_id).toBe("QmAgent1");
    expect(agents[0].skills).toEqual(["python", "ml"]);

    const url = new URL(mockFetch.mock.calls[0][0]);
    expect(url.pathname).toBe("/api/discover");
    expect(url.searchParams.get("q")).toBe("python");
    expect(url.searchParams.get("min_rep")).toBe("50");
    expect(url.searchParams.get("limit")).toBe("5");
  });

  it("transactions() handles array response", async () => {
    mockFetch.mockResolvedValueOnce(
      jsonResponse([
        {
          id: "tx-1",
          from_id: "QmA",
          to_id: "QmB",
          amount: 25,
          reason: "task reward",
          ref_id: "task-001",
          created_at: "2024-01-01T00:00:00Z",
        },
      ]),
    );

    const txns = await client.transactions(10);

    expect(txns).toHaveLength(1);
    expect(txns[0].id).toBe("tx-1");
    expect(txns[0].amount).toBe(25);

    const url = new URL(mockFetch.mock.calls[0][0]);
    expect(url.searchParams.get("limit")).toBe("10");
  });

  it("reputation() returns parsed Reputation", async () => {
    mockFetch.mockResolvedValueOnce(
      jsonResponse({
        peer_id: "QmPeer",
        score: 72.5,
        tasks_completed: 10,
        tasks_failed: 1,
        contributions: 5,
        knowledge_count: 3,
        updated_at: "2024-06-15T12:00:00Z",
      }),
    );

    const rep = await client.reputation("QmPeer");

    expect(rep.peer_id).toBe("QmPeer");
    expect(rep.score).toBe(72.5);
    expect(rep.tasks_completed).toBe(10);

    const url = new URL(mockFetch.mock.calls[0][0]);
    expect(url.pathname).toBe("/api/reputation/QmPeer");
  });
});
