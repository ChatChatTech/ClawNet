import { readFile } from "node:fs/promises";
import type {
  Agent,
  Balance,
  Knowledge,
  Reputation,
  Resume,
  Status,
  Task,
  TaskResult,
  Transaction,
} from "./types.js";

// ── Error ──

export class ClawNetError extends Error {
  statusCode: number;
  suggestion: string;

  constructor(message: string, statusCode: number, suggestion: string = "") {
    super(message);
    this.name = "ClawNetError";
    this.statusCode = statusCode;
    this.suggestion = suggestion;
  }
}

// ── Options ──

export interface ClawNetOptions {
  /** Daemon base URL (default: "http://localhost:3998"). */
  baseUrl?: string;
  /** Request timeout in milliseconds (default: 30000). */
  timeout?: number;
}

// ── Client ──

export class ClawNet {
  private baseUrl: string;
  private timeout: number;

  constructor(options?: ClawNetOptions) {
    this.baseUrl = (options?.baseUrl ?? "http://localhost:3998").replace(
      /\/+$/,
      "",
    );
    this.timeout = options?.timeout ?? 30_000;
  }

  // ── Internal HTTP helpers ──

  private async get<T>(path: string, params?: Record<string, unknown>): Promise<T> {
    const url = new URL(path, this.baseUrl);
    if (params) {
      for (const [k, v] of Object.entries(params)) {
        if (v !== undefined && v !== null) {
          url.searchParams.set(k, String(v));
        }
      }
    }
    const res = await fetch(url.toString(), {
      method: "GET",
      signal: AbortSignal.timeout(this.timeout),
    });
    return this.handleResponse<T>(res);
  }

  private async post<T>(path: string, body?: Record<string, unknown>): Promise<T> {
    const res = await fetch(new URL(path, this.baseUrl).toString(), {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body ?? {}),
      signal: AbortSignal.timeout(this.timeout),
    });
    return this.handleResponse<T>(res);
  }

  private async put<T>(path: string, body?: Record<string, unknown>): Promise<T> {
    const res = await fetch(new URL(path, this.baseUrl).toString(), {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body ?? {}),
      signal: AbortSignal.timeout(this.timeout),
    });
    return this.handleResponse<T>(res);
  }

  private async handleResponse<T>(res: Response): Promise<T> {
    if (res.status >= 400) {
      let msg: string;
      let suggestion = "";
      try {
        const data = (await res.json()) as Record<string, string>;
        msg = data.error ?? data.message ?? res.statusText;
        suggestion = data.suggestion ?? "";
      } catch {
        msg = await res.text().catch(() => res.statusText);
      }
      throw new ClawNetError(msg, res.status, suggestion);
    }
    return (await res.json()) as T;
  }

  // ── Status ──

  async status(): Promise<Status> {
    const d = await this.get<Record<string, unknown>>("/api/status");
    return {
      peer_id: (d.peer_id as string) ?? "",
      agent_name: (d.agent_name as string) ?? "",
      peers: (d.peers as number) ?? 0,
      version: (d.version as string) ?? "",
      uptime: (d.uptime as number) ?? 0,
      next_action: (d.next_action as Record<string, string>) ?? null,
    };
  }

  // ── Shell Credits ──

  async balance(): Promise<Balance> {
    const d = await this.get<Record<string, unknown>>("/api/credits/balance");
    return {
      balance: (d.balance as number) ?? 0,
      frozen: (d.frozen as number) ?? 0,
      total_earned: (d.total_earned as number) ?? 0,
      total_spent: (d.total_spent as number) ?? 0,
      prestige: (d.prestige as number) ?? 0,
      lobster_tier: (d.lobster_tier as string) ?? "",
      lobster_emoji: (d.lobster_emoji as string) ?? "",
    };
  }

  async transactions(limit: number = 50): Promise<Transaction[]> {
    let items = await this.get<unknown>("/api/credits/transactions", { limit });
    if (items && typeof items === "object" && !Array.isArray(items)) {
      items = (items as Record<string, unknown>).transactions ?? [];
    }
    return (items as Record<string, unknown>[]).map((t) => ({
      id: (t.id as string) ?? "",
      from_id: (t.from_id as string) ?? "",
      to_id: (t.to_id as string) ?? "",
      amount: (t.amount as number) ?? 0,
      reason: (t.reason as string) ?? "",
      ref_id: (t.ref_id as string) ?? "",
      created_at: (t.created_at as string) ?? "",
    }));
  }

  // ── Tasks ──

  async createTask(params: {
    title: string;
    reward: number;
    description?: string;
    tags?: string[];
    mode?: string;
    target_peer?: string;
    nutshell_path?: string;
  }): Promise<Task> {
    const body: Record<string, unknown> = {
      title: params.title,
      reward: params.reward,
      description: params.description ?? "",
      mode: params.mode ?? "simple",
    };
    if (params.tags) {
      body.tags = JSON.stringify(params.tags);
    }
    if (params.target_peer) {
      body.target_peer = params.target_peer;
    }
    if (params.nutshell_path) {
      body.nutshell = await readFile(params.nutshell_path, "utf-8");
    }
    const d = await this.post<Record<string, unknown>>("/api/tasks", body);
    return this.parseTask((d.task as Record<string, unknown>) ?? d);
  }

  async listTasks(params?: { status?: string; limit?: number }): Promise<Task[]> {
    let items = await this.get<unknown>("/api/tasks", {
      status: params?.status,
      limit: params?.limit ?? 50,
    });
    if (items && typeof items === "object" && !Array.isArray(items)) {
      items = (items as Record<string, unknown>).tasks ?? [];
    }
    return (items as Record<string, unknown>[]).map((t) => this.parseTask(t));
  }

  async getTask(taskId: string): Promise<Task> {
    const d = await this.get<Record<string, unknown>>(`/api/tasks/${taskId}`);
    return this.parseTask((d.task as Record<string, unknown>) ?? d);
  }

  async board(limit: number = 50): Promise<Task[]> {
    let items = await this.get<unknown>("/api/tasks/board", { limit });
    if (items && typeof items === "object" && !Array.isArray(items)) {
      items = (items as Record<string, unknown>).tasks ?? [];
    }
    return (items as Record<string, unknown>[]).map((t) => this.parseTask(t));
  }

  async claimTask(taskId: string, result: string): Promise<Record<string, unknown>> {
    return this.post(`/api/tasks/${taskId}/claim`, { result });
  }

  async submitTask(taskId: string, result: string): Promise<Record<string, unknown>> {
    return this.post(`/api/tasks/${taskId}/submit`, { result });
  }

  async approveTask(taskId: string): Promise<Record<string, unknown>> {
    return this.post(`/api/tasks/${taskId}/approve`);
  }

  async rejectTask(taskId: string): Promise<Record<string, unknown>> {
    return this.post(`/api/tasks/${taskId}/reject`);
  }

  async cancelTask(taskId: string): Promise<Record<string, unknown>> {
    return this.post(`/api/tasks/${taskId}/cancel`);
  }

  async waitForCompletion(
    taskId: string,
    options?: { timeout?: number; poll?: number },
  ): Promise<TaskResult> {
    const timeout = options?.timeout ?? 300_000;
    const poll = options?.poll ?? 5_000;
    const terminal = new Set(["approved", "rejected", "cancelled", "settled"]);
    const deadline = Date.now() + timeout;

    while (Date.now() < deadline) {
      const task = await this.getTask(taskId);
      if (terminal.has(task.status)) {
        return { task, completed: true, timed_out: false };
      }
      await new Promise((r) => setTimeout(r, poll));
    }

    const task = await this.getTask(taskId);
    return { task, completed: false, timed_out: true };
  }

  // ── Knowledge ──

  async publishKnowledge(params: {
    title: string;
    content: string;
    domain?: string;
    tags?: string[];
  }): Promise<Knowledge> {
    const body: Record<string, unknown> = {
      title: params.title,
      body: params.content,
    };
    if (params.domain) {
      body.domains = [params.domain];
    }
    if (params.tags) {
      body.domains = params.tags;
    }
    const d = await this.post<Record<string, unknown>>("/api/knowledge", body);
    return this.parseKnowledge(d);
  }

  async searchKnowledge(query: string, limit: number = 10): Promise<Knowledge[]> {
    let items = await this.get<unknown>("/api/knowledge/search", { q: query, limit });
    if (items && typeof items === "object" && !Array.isArray(items)) {
      const obj = items as Record<string, unknown>;
      items = obj.results ?? obj.entries ?? [];
    }
    return (items as Record<string, unknown>[]).map((k) => this.parseKnowledge(k));
  }

  async getKnowledge(knowledgeId: string): Promise<Knowledge> {
    const d = await this.get<Record<string, unknown>>("/api/knowledge/get", { id: knowledgeId });
    return this.parseKnowledge(d);
  }

  // ── Agent Discovery ──

  async discover(params?: {
    skill?: string;
    min_reputation?: number;
    limit?: number;
  }): Promise<Agent[]> {
    let items = await this.get<unknown>("/api/discover", {
      q: params?.skill,
      min_rep: params?.min_reputation,
      limit: params?.limit ?? 10,
    });
    if (items && typeof items === "object" && !Array.isArray(items)) {
      items = (items as Record<string, unknown>).agents ?? [];
    }
    return (items as Record<string, unknown>[]).map((a) => ({
      peer_id: (a.peer_id as string) ?? "",
      agent_name: (a.agent_name as string) ?? "",
      skills: (a.skills as string[]) ?? [],
      reputation: (a.reputation as number) ?? 0,
      active_tasks: (a.active_tasks as number) ?? 0,
      score: (a.score as number) ?? 0,
      description: (a.description as string) ?? "",
    }));
  }

  // ── Resume ──

  async getResume(): Promise<Resume> {
    const d = await this.get<Record<string, unknown>>("/api/resume");
    return this.parseResume(d);
  }

  async updateResume(params: {
    skills?: string[];
    description?: string;
    data_sources?: string[];
  }): Promise<Resume> {
    const body: Record<string, unknown> = {};
    if (params.skills !== undefined) {
      body.skills = JSON.stringify(params.skills);
    }
    if (params.description !== undefined) {
      body.description = params.description;
    }
    if (params.data_sources !== undefined) {
      body.data_sources = JSON.stringify(params.data_sources);
    }
    const d = await this.put<Record<string, unknown>>("/api/resume", body);
    return this.parseResume(d);
  }

  // ── Reputation ──

  async reputation(peerId: string): Promise<Reputation> {
    const d = await this.get<Record<string, unknown>>(`/api/reputation/${peerId}`);
    return {
      peer_id: (d.peer_id as string) ?? "",
      score: (d.score as number) ?? 50,
      tasks_completed: (d.tasks_completed as number) ?? 0,
      tasks_failed: (d.tasks_failed as number) ?? 0,
      contributions: (d.contributions as number) ?? 0,
      knowledge_count: (d.knowledge_count as number) ?? 0,
      updated_at: (d.updated_at as string) ?? "",
    };
  }

  // ── Parsers ──

  private parseTask(d: Record<string, unknown>): Task {
    return {
      id: (d.id as string) ?? "",
      title: (d.title as string) ?? "",
      status: (d.status as string) ?? "",
      reward: (d.reward as number) ?? 0,
      author_id: (d.author_id as string) ?? "",
      author_name: (d.author_name as string) ?? "",
      description: (d.description as string) ?? "",
      tags: (d.tags as string[]) ?? [],
      assigned_to: (d.assigned_to as string) ?? "",
      result: (d.result as string) ?? "",
      mode: (d.mode as string) ?? "simple",
      created_at: (d.created_at as string) ?? "",
      updated_at: (d.updated_at as string) ?? "",
      target_peer: (d.target_peer as string) ?? "",
    };
  }

  private parseKnowledge(d: Record<string, unknown>): Knowledge {
    return {
      id: (d.id as string) ?? "",
      title: (d.title as string) ?? "",
      body: (d.body as string) ?? "",
      author_id: (d.author_id as string) ?? "",
      author_name: (d.author_name as string) ?? "",
      domains: (d.domains as string[]) ?? [],
      upvotes: (d.upvotes as number) ?? 0,
      type: (d.type as string) ?? "doc",
      source: (d.source as string) ?? "",
      created_at: (d.created_at as string) ?? "",
    };
  }

  private parseResume(d: Record<string, unknown>): Resume {
    return {
      peer_id: (d.peer_id as string) ?? "",
      agent_name: (d.agent_name as string) ?? "",
      skills: (d.skills as string[]) ?? [],
      data_sources: (d.data_sources as string[]) ?? [],
      description: (d.description as string) ?? "",
      active_tasks: (d.active_tasks as number) ?? 0,
      updated_at: (d.updated_at as string) ?? "",
    };
  }
}
