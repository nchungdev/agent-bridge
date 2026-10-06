export interface Workspace {
  id: string;
  path: string;
  name: string;
}

export interface NativeTurn {
  role: string;
  content: string;
}

export interface NativeSession {
  agent: string;
  id: string;
  title: string;
  workspace: string;
  updated_at: string;
  last_user: string;
  last_assistant: string;
  turn_count: number;
  turns?: NativeTurn[];
}

export interface ToolBinding {
  agent: string;
  native_session_id: string;
  title?: string;
  turn_count?: number;
  last_synced_hash?: string;
  updated_at: string;
}

export interface BridgeSession {
  id: string;
  workspace_id?: string;
  workspace?: string;
  title: string;
  status?: string;
  current_agent?: string;
  last_handoff_summary?: string;
  bindings?: Record<string, string>; // agent -> native_session_id
  tools?: ToolBinding[];
  created_at: string;
  updated_at: string;
}

export interface EngineStatus {
  id: string;
  name: string;
  base?: string;
  binary: string;
  installCmd?: string;
  installed: boolean;
  auth_status?: string;
  has_auth?: boolean;
}

export interface TermTab {
  key: string;
  sessionId: string;
  label: string;
  workDir: string;
  createdAt: number;
  agent?: string;
  launch?: { agent: string; resume?: string; fresh?: boolean; remote?: boolean; name?: string };
  remoteOn?: boolean;
  /** bumped to remount the terminal when the agent is restarted with other launch options */
  rev?: number;
  from?: { agent: string; id: string };
}

export interface UpdateStatus {
  current_version?: string;
  latest_version?: string;
  current_build?: number;
  latest_build?: number;
  current_commit: string;
  current_message: string;
  current_date: string;
  branch: string;
  remote_commit: string;
  has_update: boolean;
  commits_behind: number;
  commits: string[];
  is_updating: boolean;
  step: "idle" | "checking" | "pulling" | "building_web" | "building_binary" | "restarting" | "success" | "error";
  error?: string;
  logs: string[];
  last_checked?: string;
}
