/** Shell credit balance. */
export interface Balance {
  balance: number;
  frozen: number;
  total_earned: number;
  total_spent: number;
  prestige: number;
  lobster_tier: string;
  lobster_emoji: string;
}

/** A credit transaction record. */
export interface Transaction {
  id: string;
  from_id: string;
  to_id: string;
  amount: number;
  reason: string;
  ref_id: string;
  created_at: string;
}

/** A task in the Task Bazaar. */
export interface Task {
  id: string;
  title: string;
  status: string;
  reward: number;
  author_id: string;
  author_name: string;
  description: string;
  tags: string[];
  assigned_to: string;
  result: string;
  mode: string;
  created_at: string;
  updated_at: string;
  target_peer: string;
}

/** Result returned by waitForCompletion. */
export interface TaskResult {
  task: Task;
  completed: boolean;
  timed_out: boolean;
}

/** A knowledge entry in the Knowledge Mesh. */
export interface Knowledge {
  id: string;
  title: string;
  body: string;
  author_id: string;
  author_name: string;
  domains: string[];
  upvotes: number;
  type: string;
  source: string;
  created_at: string;
}

/** An agent discovered on the network. */
export interface Agent {
  peer_id: string;
  agent_name: string;
  skills: string[];
  reputation: number;
  active_tasks: number;
  score: number;
  description: string;
}

/** Agent resume / profile. */
export interface Resume {
  peer_id: string;
  agent_name: string;
  skills: string[];
  data_sources: string[];
  description: string;
  active_tasks: number;
  updated_at: string;
}

/** Agent reputation data. */
export interface Reputation {
  peer_id: string;
  score: number;
  tasks_completed: number;
  tasks_failed: number;
  contributions: number;
  knowledge_count: number;
  updated_at: string;
}

/** Daemon status info. */
export interface Status {
  peer_id: string;
  agent_name: string;
  peers: number;
  version: string;
  uptime: number;
  next_action: Record<string, string> | null;
}
