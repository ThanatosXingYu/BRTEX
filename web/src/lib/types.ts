// ARTEX domain model — types used across the UI.
// Derived from the functional spec (section 7: key data shapes).

export type TaskStatus = "created" | "queued" | "running" | "paused" | "done" | "failed" | "timeout";
export type EngineMode = "exploring" | "paused" | "stalled" | "idle";

export interface Task {
  id: string;
  name?: string; // optional task name; empty/absent = untitled, display falls back to the description
  category_id?: number;
  category_name?: string;
  pinned?: boolean;
  pinned_at?: string | null;
  description: string;
  goal: string;
  status: TaskStatus;
  created_at: string;
  created_unix?: number; // created_at as unix seconds (run-duration calc)
  completed_at?: string; // RFC3339 finish time (done/failed); "" if unfinished
  completed_unix?: number; // completed_at as unix seconds (0/undef if unfinished)
  last_activity_unix?: number; // unix seconds of the last activity (0/undef if none)
  paused?: boolean;
  queued?: boolean;
  active?: boolean;
  in_flight?: number;
  findings?: { critical: number; high: number; medium: number; low: number }; // registered vulnerability counts (bucketed by severity)
  last_activity?: string;
  stalled?: boolean;
  goals_total?: number;
  goals_met?: number;
  engine_mode?: EngineMode;
  tokens?: TokenTotal; // whole-task token consumption
  llm_profile_id?: number; // LLM profile used; absent = default profile
  llm_profile_ids?: number[]; // ordered task-level failover chain
  llm_rotate?: boolean;
  active_llm_profile_id?: number; // profile used by the next LLM call
  llm_failover_state?: "default" | "ready" | "chain_exhausted" | string;
  llm_failover_reason?: string;
  source_task_ids?: string[]; // directly related tasks inherited as read-only context
  archive_blocked_by_task_id?: string; // live direct dependent that must be archived first
  company_ids?: number[]; // associated company scopes; current company assets join the task at creation
  coverage_enabled?: boolean; // asset coverage feature switch (fixed at creation, on by default); false = do not compute/show coverage
}

export interface TaskCategory {
  id: number;
  name: string;
  task_count: number;
  created_at: string;
  updated_at: string;
}

export interface TaskTemplate {
  id: number;
  name: string;
  description: string;
  goal: string;
  category_id?: number | null; // preset category; null/absent = none
  intercept_rules?: AssetInterceptRuleInput[]; // preset task-level interception/allow rules
  created_at: string;
  updated_at: string;
}

export interface DeleteTaskOptions {
  delete_assets: boolean;
  delete_traffic: boolean;
  delete_files: boolean;
  delete_findings: boolean;
  delete_llm_records: boolean;
}

export interface DeleteTaskResult {
  deleted: string;
  assets_deleted: number;
  assets_detached: number;
  traffic_deleted: number;
  files_deleted: boolean;
  findings_deleted: number;
  llm_records_deleted: number;
  cleanup_warning?: string;
}

export type TaskArchiveState =
  | "archive_queued"
  | "archiving"
  | "archive_failed"
  | "ready"
  | "restore_queued"
  | "restoring"
  | "restore_failed"
  | "delete_queued"
  | "deleting"
  | "delete_failed";

export interface TaskArchiveTokenStats {
  calls?: number;
  input_tokens?: number;
  output_tokens?: number;
  cache_read_tokens?: number;
  cache_write_tokens?: number;
}

export interface TaskArchive {
  storage?: "local" | "s3";
  id: number;
  task_id: number;
  state: TaskArchiveState;
  phase: string;
  progress: number;
  error?: string;
  warnings?: string[];
  format_version: number;
  sha256?: string;
  original_size: number;
  compressed_size: number;
  task_name: string;
  task_description: string;
  task_goal: string;
  original_status: TaskStatus;
  category_id?: number;
  category_name?: string;
  source_task_ids: number[];
  remaining_timeout_seconds: number;
  data_counts: Record<string, number>;
  aggregate_stats: {
    tokens?: TaskArchiveTokenStats;
    skills?: Record<string, number>;
    tools?: Record<string, number>;
    findings?: Record<string, number>;
  };
  archived_at?: string;
  requested_at: string;
  created_at: string;
  updated_at: string;
}

export interface TaskArchivePage {
  items: TaskArchive[];
  total: number;
  page: number;
  size: number;
}

export interface ArchiveBatchItem {
  id: string;
  archive_id?: number;
  ok: boolean;
  queued: boolean;
  error?: string;
}

// ---- Asset graph (global, shared across tasks) ----
export type AssetType =
  | "company"
  | "domain"
  | "ip"
  | "port"
  | "service"
  | "site"
  | "endpoint"
  | "parameter"
  | "tech"
  | "credential"
  | "data";

export type NodeState = "observed" | "confirmed" | "tombstoned";

export interface AssetNode {
  id: string;
  type: AssetType;
  name: string;
  key: string; // nkey
  value?: string;
  company_id?: string; // owning company asset id; empty = unassigned
  state: NodeState;
  confidence: number; // 0..1
  attrs?: Record<string, unknown>;
  first_seen: string;
  last_seen: string;
}

export type AssetRel =
  | "owns"
  | "resolves"
  | "exposes"
  | "runs"
  | "serves"
  | "has_endpoint"
  | "has_param"
  | "fingerprinted"
  | "authenticates_as"
  | "reachable"
  | "has_subdomain";

export interface Edge {
  src: string;
  dst: string;
  rel: AssetRel | ExploreRel;
}

// Task asset view — server-side enriched, paginated.
export interface TaskAssetRef {
  id: string;
  name?: string;
  key: string;
  attrs?: Record<string, unknown>;
}

export interface TaskAssetItem extends AssetNode {
  techs?: TaskAssetRef[];
  auth?: TaskAssetRef[];
  params?: TaskAssetRef[];
}

export interface TaskAssetView {
  counts: Record<string, number>;
  total: number;
  items: TaskAssetItem[];
}

// ---- New unified asset model (new backend) ----
export type NewAssetType = "root_domain" | "ip" | "subdomain" | "app" | "service" | "endpoint";

export interface Asset {
  id: number;
  type: NewAssetType;
  company_id?: number;
  task_ids: number[];
  domain?: string;
  root_domain?: string;
  ip?: string;
  c_segment?: string;
  port?: number;
  icp?: string;
  bound_domains?: string[];
  open_ports?: { port: number; service?: string }[];
  record_type?: string;
  record_value?: string[] | string;
  bundle_id?: string;
  app_name?: string;
  category?: string;
  app_description?: string;
  app_icp?: string;
  url?: string;
  service_type?: string;
  service_name?: string;
  favicon_mmh3?: string;
  status_code?: number;
  content_length?: number;
  page_title?: string;
  technologies?: string[];
  auth?: Record<string, unknown>[];
  method?: string;
  params?: Record<string, unknown>[];
  extra?: Record<string, unknown>;
  last_seen: string;
  task_source?: string;
  task_source_summary?: string;
  task_source_node_id?: number;
}

export interface IntentAsset {
  intent_id: number | string;
  asset_id: number;
  type: NewAssetType;
  label: string;
  source: string;
  source_summary: string;
  source_node_id?: number;
  source_task_id: number;
  inherited: boolean;
}

export interface TaskAssetMutation {
  requested: number;
  attached: number;
  existing: number;
}

export interface TaskAssetScopeMutation {
  requested: number;
  assets_linked: number;
  assets_existing: number;
  scopes_added: number;
  scopes_existing: number;
}

// ---- Asset coverage graph (per task) ----
// One node of the force-directed "asset coverage graph". key is unique: asset="a:<id>",
// company="c:<id>", root domain without an asset row="r:<domain>". in_scope=false nodes are grey context nodes used only for connecting.
export interface CoverageGraphNode {
  key: string;
  kind: "company" | "root_domain" | "subdomain" | "ip" | "service" | "app" | "endpoint";
  label: string;
  tested: boolean;
  in_scope: boolean;
  asset_id?: number;
  company_id?: number;
  domain?: string;
  root_domain?: string;
  ip?: string;
  url?: string;
  port?: number;
  service_type?: string;
  app_name?: string;
  page_title?: string;
  status_code?: number;
}

export interface CoverageGraphEdge {
  src: string;
  dst: string;
}

export interface CoverageGraphData {
  nodes: CoverageGraphNode[];
  edges: CoverageGraphEdge[];
}

// Intents/facts/findings of an asset in this task's exploration graph (for the coverage graph node drawer).
export interface CoverageAssetRef {
  id: number;
  kind: string;
  state: string;
  summary: string;
  source_task_id?: string;
  inherited?: boolean;
}
export interface CoverageAssetRefs {
  intents: CoverageAssetRef[];
  facts: CoverageAssetRef[];
  findings: CoverageAssetRef[];
}

// ---- Workspace file manager (workDir) ----
export interface WorkspaceEntry {
  name: string;
  path: string; // workspace-relative, forward slashes
  dir: boolean;
  size: number;
  mtime: number; // unix millis
}
export interface WorkspaceListing {
  path: string;
  entries: WorkspaceEntry[];
}
export interface WorkspaceFile {
  path: string;
  size: number;
  binary: boolean;
  too_large?: boolean;
  content?: string;
}

// One row of a task's test scope (coverage denominator; coverage scope, not enforced).
export interface TaskScopeRow {
  id: number;
  task_id: number;
  kind: "company" | "root_domain" | "subdomain" | "ip" | "cidr" | "icp" | "keyword";
  company_id?: number;
  company_name?: string; // resolved by a backend JOIN on companies; present only for kind=company
  domain?: string;
  net?: string;
  value?: string;
  source: "auto" | "agent" | "manual";
  reason?: string;
}

export type CompanyScopeKind = "domain" | "ip" | "cidr" | "icp" | "keyword";

// Structured asset scope rules submitted when creating a company.
export interface CompanyScopeRule {
  kind: CompanyScopeKind;
  value: string;
}

// Result of writing asset scopes. errors are invalid rows from this submission; warnings
// are pre-existing data problems unrelated to this submission that would make ownership results miss expectations (e.g. an asset whose ip field stores a hostname).
export interface CompanyScopeMutation {
  added: number;
  skipped: number;
  invalid: number;
  errors?: string[];
  warnings?: string[];
}

// One row of a company's asset scope rules (the single source of truth for ownership).
export interface ScopeRow {
  id: number;
  company_id: number;
  kind: CompanyScopeKind;
  domain?: string; // present when kind=domain
  net?: string; // present when kind=ip|cidr
  value?: string; // may be returned directly by the backend when kind=icp|keyword
  raw: string; // raw user input, for display and backfill
  reason?: string;
}

// Company: the type=company asset node + logo + asset counts + asset scope rules.
export interface Company {
  id: number;
  name: string;
  logo?: string; // remote logo URL; when empty the frontend uses the first letter of the name
  asset_count: number;
  scope?: ScopeRow[];
}

// ---- Exploration graph (per task) ----
export type ExploreKind = "task" | "begin" | "goal" | "intent" | "fact" | "finding" | "hint" | "digest";
export type GoalState = "open" | "met" | "abandoned";
export type IntentState = "open" | "running" | "paused" | "done" | "blocked" | "exhausted" | "stopped";
export type FindingState = "confirmed" | "dismissed";
export type HintState = "active" | "consumed";
export type ExploreRel = "spawns" | "derived_from" | "yields" | "proves" | "covers";

export interface TaskNode {
  id: string;
  type: ExploreKind;
  payload?: string;
  priority: number; // 0..10
  state: string; // GoalState | IntentState | FindingState | HintState
  origin: string;
  ts: string;
  source_task_id?: string;
  inherited?: boolean;
  delete_reason?: string; // deletion reason when the intent is soft-deleted (state='deleted')
}

// One broadcast board page: nodes paginated by creation order + the edges involved on this
// page + the nodes at the other ends (refs, indexed by id), so each broadcast can say "where it came from, what it produced" without pulling the whole graph.
export interface ExplorationNodePage {
  items: TaskNode[];
  total: number;
  page: number;
  size: number;
  edges: Edge[];
  refs: Record<string, TaskNode>;
  // node id → assets anchored to that node (shown alongside broadcast expansion, incl. this page's nodes and their neighbors).
  assets: Record<string, FindingAsset[]>;
}

export interface ExplorationNodeQuery {
  page?: number;
  size?: number;
  kinds?: ExploreKind[];
  states?: string[];
  q?: string;
  order?: "asc" | "desc";
}

// Goals for the goal management card (the backend already splits payload into text/vulnclass).
export interface TaskGoal {
  id: string;
  text: string;
  vulnclass?: string;
  state: string; // GoalState
  origin?: string;
  ts: string;
}

// Operation constraints for the constraint management card (allow=permit / deny=forbid).
export type ConstraintKind = "allow" | "deny";
export interface TaskConstraint {
  id: string;
  kind: ConstraintKind;
  text: string;
  origin?: string;
  ts?: string;
}

// ---- Findings ----
export type Severity = "critical" | "high" | "medium" | "low";

// Vulnerability handling status: Pending / In progress / Confirmed / Resolved / Fixed / False positive / Ignored / Duplicate / Risk accepted.
export type FindingStatus =
  | "pending"
  | "in_progress"
  | "confirmed"
  | "resolved"
  | "fixed"
  | "false_positive"
  | "ignored"
  | "duplicate"
  | "risk_accepted";

// Verification status (S1): the automated proof-of-exploitation outcome, distinct from the
// human triage status above. "blocked" is produced by no path yet but is displayed when present.
export type VerificationStatus = "verified" | "unverified" | "blocked" | "not_assessed" | "false_positive";

// Report stage after triage: a finding can be confirmed and submitted at the same time.
export type FindingReportState = "none" | "submitted" | "accepted" | "rejected" | "duplicate";

// FindingAsset is an asset bound to a vulnerability (label pre-rendered by the backend).
export interface FindingAsset {
  id: string;
  type: string;
  label: string;
}

export interface Finding {
  traffic_count?: number;
  evidence_version?: number;
  report_evidence_version?: number;
  report_stale?: boolean;
  poe_level?: number; // proof-of-exploitation level (3 or 4 when verified); 0 = none declared
  verification_status?: VerificationStatus; // automated verification outcome (S1)
  verification_note?: string; // why the status was reached; empty when verified
  counterevidence?: string; // what was tested and did not work; optional, omitted when empty
  confidence_rationale?: string; // why the conclusion holds; optional, omitted when empty
  id: string;
  finding_id?: string; // row id in the standalone findings table, the handle for status updates (may be missing for old in-task nodes)
  vulnclass: string;
  name?: string; // vulnerability name; display falls back to vulnclass when empty
  severity: Severity;
  status: FindingStatus;
  report_state?: FindingReportState; // workflow stage after triage; default none
  summary: string;
  evidence: string;
  report?: string; // detailed report (Markdown); returned only by the detail endpoint, empty in lists
  intent_id?: string;
  param_id?: string;
  task_id?: string;
  task_description?: string;
  source_task_id?: string;
  inherited?: boolean;
  assets?: FindingAsset[];
  ts: string;
}

// FindingsPage is the server-side paginated response of the findings list.
export interface FindingsPage {
  items: Finding[];
  total: number;
  page: number;
  page_size: number;
}

export interface FindingGroup {
  task_id: string | number | null;
  task_name?: string; // optional task name; empty/absent = untitled
  task_description: string;
  task_status: string;
  count: number;
  critical: number;
  high: number;
  medium: number;
  low: number;
  last_found_at: string;
}

export interface FindingGroupsPage {
  items: FindingGroup[];
  total: number;
  finding_total: number;
  page: number;
  page_size: number;
}

export interface FindingDeepenResponse {
  task_id: string;
  intent_id: string;
  state: IntentState;
  queued: boolean;
}

// FindingStats is the whole-table findings aggregate (stat cards + vulnerability type dropdown), computed server-side, unaffected by pagination.
export interface FindingStats {
  total: number;
  pending: number;
  submitted: number;
  accepted: number;
  critical: number;
  high: number;
  medium: number;
  low: number;
  vulnclasses: string[];
  tasks: FindingTaskOption[];
}

// FindingTaskOption is one entry of the findings page's "by task" filter dropdown: tasks
// with vulnerabilities (an empty description means the task was deleted; the frontend falls back to showing the id) and their vulnerability counts.
export interface FindingTaskOption {
  id: string | number;
  name?: string; // optional task name; empty/absent = untitled
  description: string;
  count: number;
}

// FindingQuery is the findings list pagination/filter/sort parameters.
export interface FindingQuery {
  page: number;
  pageSize: number;
  severity?: "all" | Severity;
  status?: "all" | FindingStatus;
  reportState?: "all" | FindingReportState;
  verificationStatus?: "all" | VerificationStatus;
  vulnclass?: string;
  task?: string; // task id; "all"/empty = no task filter
  query?: string;
  sort?: "severity" | "time";
  // asset tree node key; selecting a node selects its whole subtree. Empty = no asset filter.
  assetScope?: string;
  // company id from the sidebar switcher; also narrows the asset tree. Empty = all companies.
  company?: string;
}

// ---- Findings by asset (asset view) ----
export type FindingAssetKind = "company" | "root_domain" | "subdomain" | "ip" | "service" | "app" | "endpoint" | "none";

// FindingAssetNode is one node of the asset tree. keys look like a:<id> (asset), c:<id>
// (company), r:<domain> (root domain with no asset row), __none__ (no linked asset).
export interface FindingAssetNode {
  key: string;
  parent?: string;
  kind: FindingAssetKind;
  label: string;
  asset_id?: number;
  company_id?: number;
  self: number; // findings attached directly to this asset
  total: number; // including descendants, deduplicated by finding
  critical: number;
  high: number;
  medium: number;
  low: number;
  last_found_at: string;
}

export interface FindingAssetTree {
  nodes: FindingAssetNode[];
  finding_total: number;
  truncated: boolean;
  dropped_kinds?: string[];
}

// FINDING_UNASSIGNED_ASSET corresponds to the backend's db.FindingUnassignedAsset.
export const FINDING_UNASSIGNED_ASSET = "__none__";

// ---- Activity / sessions ----
export type ActivityKind =
  | "tool_use"
  | "tool_result"
  | "text"
  | "thinking"
  | "result"
  | "user"
  | "intent" // LLM-generated exploration objective leading a worker session (UI-synthesized)
  | "round" // planner round boundary marker (engine-emitted)
  | "usage" // live cumulative token usage (per model turn); not rendered
  | "llm_switch" // automatic/manual task-level LLM switch
  | "llm_failover" // task-level provider switch / chain exhaustion audit event
  | "intercept_request"; // user-approval request from the intercept layer

// ChatAttachment is an uploaded file: path is relative to the session/task working directory (the agent's CWD).
export interface ChatAttachment {
  name: string;
  path: string;
  size: number;
  abs?: string; // absolute path (returned for scope=staging uploads; write it into the description before creating the task)
}

export interface Activity {
  seq: number;
  intent_id?: string;
  worker: string; // session owner: planner | mainagent | work#1 ...
  ts: string;
  kind: ActivityKind;
  tool?: string;
  tool_use_id?: string;
  is_error?: boolean;
  summary: string;
  detail?: string;
  metadata?: {
    llm_transition?: LLMTransition;
  };
  source_task_id?: string;
  inherited?: boolean;
  main_seg?: number; // main-agent conversation segment (present only on worker="mainagent" rows)
  // token usage (present only on kind='result')
  input_tokens?: number;
  output_tokens?: number;
  cache_read_tokens?: number;
  cache_write_tokens?: number;
}

export interface LLMAuditProfile {
  id: number;
  name: string;
  format: string;
  model: string;
}

export interface LLMTransition {
  mode: "automatic" | "manual" | "exhausted";
  reason: string;
  previous?: LLMAuditProfile;
  next?: LLMAuditProfile;
}

export interface TaskLLMResolution {
  profile_id?: number;
  name: string;
  format: string;
  model: string;
  source: "task_chain" | "agent_binding" | "global_profile" | "environment" | "global";
  available: boolean;
  reason?: string;
}

export interface TaskLLMResolutions {
  mainagent: TaskLLMResolution;
  planner: TaskLLMResolution;
  worker: TaskLLMResolution;
}

// ---- Agent triggers (P3 scheduling, custom agents only) ----
export interface AgentTrigger {
  id: number;
  agent_key: string;
  enabled: boolean;
  interval_sec: number; // scheduled: every N seconds (0 = not scheduled)
  on_finding: boolean; // fires when any task finds a finding
  on_goal_met: boolean; // fires when any task meets a goal
  on_task_timeout: boolean; // fires when any task times out
  on_tool_call: boolean; // fires when a selected tool is called (execution finished)
  on_task_create: boolean; // fires when any task is created
  interval_message: string; // standalone user message for each trigger condition
  finding_message: string;
  goal_message: string;
  task_timeout_message: string;
  tool_call_message: string;
  task_create_message: string;
  tool_names: string[]; // selected tool keys for on_tool_call (at least one)
  last_fire?: string;
}

// ---- Conversations (chat page) ----
export interface ActiveFindingRetest {
  id: number;
  finding_id: string;
  conversation_id: number;
  status: "pending" | "running";
}

export interface FindingRetest {
  id: number;
  finding_id: number;
  conversation_id: number | null;
  status: "pending" | "running" | "completed" | "failed" | "stopped";
  verdict: "" | "reproduced" | "fixed" | "inconclusive";
  notes: string;
  summary: string;
  evidence: string;
  error: string;
  created_at: string;
  started_at: string | null;
  finished_at: string | null;
}

export interface Conversation {
  id: number;
  running?: boolean; // live server state, returned with the conversation list
  agent_key: string;
  title: string;
  llm_profile_id?: number;
  pinned?: boolean;
  pinned_at?: string | null;
  created_at: string;
  updated_at: string;
}

// ---- Backend logs (/logs page) ----
export interface LogLine {
  seq: number;
  db_id?: number; // server_logs.id; present for DB-persisted lines
  ts: string;
  level: "info" | "warn" | "error";
  tag: string;
  text: string;
}

export type SessionRole = "mainagent" | "planner" | "worker" | "system";
export type SessionStatus = "running" | "paused" | "done" | "blocked" | "exhausted" | "pending" | "stopped" | "deleted";

// Daily token aggregate bucket (GET /api/tokens/daily).
export interface DailyTokenBucket {
  date: string; // "YYYY-MM-DD"
  input_tokens: number;
  output_tokens: number;
  cache_read_tokens: number;
  cache_write_tokens: number;
}

// Per-worker token usage (GET /api/exploration/tokens).
export interface TokenUsage {
  worker: string;
  input_tokens: number;
  output_tokens: number;
  cache_read_tokens: number;
  cache_write_tokens: number;
}

export interface SessionTokenUsage {
  session: string;
  input_tokens: number;
  output_tokens: number;
  cache_read_tokens: number;
  cache_write_tokens: number;
}

export interface BatchControlItem {
  id: string;
  ok: boolean;
  status?: string;
  queued?: boolean;
  error?: string;
}

// Per-task results of batch category changes. The only failure is the task already being deleted; the category write itself is atomic.
export interface BatchCategoryItem {
  id: string;
  ok: boolean;
  error?: string;
}

// Whole-task (all agents) token aggregate.
export interface TokenTotal {
  input_tokens: number;
  output_tokens: number;
  cache_read_tokens: number;
  cache_write_tokens: number;
}

// Global per-profile token spend from the llm_usage ledger (GET /api/tokens/usage).
export interface ProfileUsage {
  profile_name: string;
  calls: number;
  tasks: number;
  input_tokens: number;
  output_tokens: number;
  cache_read_tokens: number;
  cache_write_tokens: number;
  // EUR cost from the profile's stored prices; absent when the profile has no price set.
  cost_eur?: number | null;
}

// One (profile, UTC day) token bucket for the dashboard's daily chart (new source).
export interface ProfileDayUsage {
  profile_name: string;
  date: string; // YYYY-MM-DD
  input_tokens: number;
  output_tokens: number;
  cache_read_tokens: number;
}

// Response of GET /api/tokens/usage — the dashboard's "new" (llm_usage) token view.
export interface UsageStats {
  by_profile: ProfileUsage[];
  daily: ProfileDayUsage[];
}

// Per-model token usage for one task (GET /api/llm/records/by-model), from the
// always-on llm_usage metering ledger. calls = number of LLM calls on this model.
export interface ModelTokenStat {
  model: string;
  calls: number;
  input_tokens: number;
  output_tokens: number;
  cache_read_tokens: number;
  cache_write_tokens: number;
}

export interface Session {
  id: string;
  role: SessionRole;
  title: string;
  status: SessionStatus;
  live: boolean;
  last_activity: string;
  intent_id?: string;
  source_task_id?: string;
  inherited?: boolean;
  seg?: number; // main-agent session: which conversation segment (0 = original)
}

// ---- Security ----
export interface AuditEntry {
  ts: string;
  tool: string;
  action: "allow" | "block";
  reason?: string;
  command?: string;
}

export interface Audit {
  entries?: AuditEntry[];
  attributions?: Record<string, number>;
}

// ---- Traffic ----
export interface TrafficExchange {
  id: string;
  ts: string;
  host: string;
  method: string;
  url: string;
  status: number;
  content_type: string;
  resp_len: number;
  rewritten?: boolean;
}

export interface TrafficRewriteStats {
  large_responses: number;
  capacity_refusals: number;
  tombstone_drops: number;
  conversions: number;
  sweeps: number;
}

export interface TrafficHARImportError {
  index: number;
  url?: string;
  reason: string;
}

export interface TrafficHARImportResult {
  imported: number;
  skipped: number;
  failed: number;
  truncated?: boolean;
  errors?: TrafficHARImportError[];
}

export interface TrafficHARExportMeta {
  exported: number;
  matched: number;
  truncated: boolean;
  truncatedReason?: string;
}

export interface TrafficResp {
  enabled: boolean;
  proxy?: string;
  count?: number; // global total (unfiltered)
  total?: number; // rows matching the current filter (for pagination)
  page?: number;
  size?: number;
  exchanges?: TrafficExchange[];
  rewrite?: TrafficRewriteStats;
}

// Full raw request/response of one exchange (lazy-loaded on row select).
export interface TrafficDetail {
  req: string;
  resp: string;
  // WebSocket/SSE exchanges carry their messages separately (trafficStream);
  // stream_kind is "" for an ordinary exchange.
  stream_count?: number;
  stream_kind?: "websocket" | "sse" | "";
}

// One recorded WebSocket message or SSE event. ts is in Unix seconds; seq
// orders. payload is a preview of at most 4 KiB (base64 when encoding is
// "base64"); blob names the full payload when it was stored separately.
export interface TrafficStreamMessage {
  seq: number;
  ts: number;
  direction: "c2s" | "s2c" | "meta" | (string & {});
  kind: number;
  type: "text" | "binary" | "event" | "meta" | (string & {});
  event?: string;
  event_id?: string;
  retry?: number;
  size: number;
  payload: string;
  encoding?: string;
  blob?: string;
  truncated?: boolean;
}

export interface TrafficStreamPage {
  items: TrafficStreamMessage[];
  total: number;
}

export interface TrafficReplayHeader {
  name: string;
  value: string;
}

// POST /api/traffic/replay. Host is not sent: the server takes it from url.
// apply_rules defaults to false (bypasses capture rewrite rules).
export interface TrafficReplayRequest {
  method: string;
  url: string;
  headers: TrafficReplayHeader[];
  body: string;
  body_base64: boolean;
  apply_rules?: boolean;
}

export interface TrafficReplayResult {
  status: number;
  headers: TrafficReplayHeader[];
  raw: string;
  duration_ms: number;
  truncated: boolean;
  // false when the host is in passthrough: the response was not recorded.
  recorded: boolean;
}

// One distinct recorded host with its exchange count (target picker).
export interface TrafficHost {
  host: string;
  count: number;
}

// ---- App settings (runtime toggles) ----
export interface Settings {
  traffic_capture: boolean;
  agent_traffic_binding: boolean; // agents auto-bind traffic evidence, off by default; does not affect manual binding
  llm_record: boolean; // LLM recording switch (off by default); when off no LLM calls are recorded
  // Web search. brave_key_set / tavily_key_set / keenable_key_set reflect whether a key is stored
  // (the values are never returned). On PUT, send the corresponding field to set/clear.
  web_search_enabled: boolean;
  web_search_backend: string; // "ddgs" | "brave-free" | "tavily" | "keenable" | "deepseek"
  brave_key_set: boolean;
  tavily_key_set: boolean;
  keenable_key_set: boolean;
  // write-only: only sent on PUT to store/clear the key.
  brave_search_api_key?: string;
  tavily_search_api_key?: string;
  keenable_search_api_key?: string;
  // Dedicated egress proxy (http/https/socks5) for reaching search endpoints; unrelated to the MITM proxy that records traffic. Empty = direct.
  web_search_proxy?: string;
  // Global egress proxy (http/https/socks5, may carry user:pass) that all target traffic goes
  // through. Acts as the MITM upstream when traffic capture is on; injected directly into the agent's bash/WebFetch when capture is off. Empty = direct.
  global_proxy?: string;
  python_interpreter?: string; // python interpreter path for custom script tools (empty = detect at runtime)
  workers?: number; // concurrent worker agent count (default 3); applies to tasks started afterwards
  // Task concurrency cap: max simultaneously "running" tasks. Off = unlimited; on, new tasks beyond the limit queue and start automatically when a slot frees up.
  task_concurrency_enabled?: boolean; // false by default
  task_concurrency_limit?: number; // defaults to 5 when enabled
  // LLM pooling (failover). Off by default; when on, an agent with "no model specified"
  // automatically switches to the next profile when the current one is unavailable (out of balance / dead key / rate limited / service error).
  llm_pool_enabled?: boolean; // false by default
  // Whether agents/tasks bound to a specific profile also fall back to the pool chain on failure. Default false = binding is exclusive.
  llm_pool_bind_fallback?: boolean;
  // Operation constraint injection scope (both on by default): splice the task's allow/deny constraints into the matching agent's system prompt.
  constraints_inject_planner?: boolean;
  constraints_inject_worker?: boolean;
  // Experimental: noa model-driven context compression (off by default). When on, the
  // platform's four agent kinds (planner/worker/main agent/chat) have context compression
  // taken over by noa, replacing built-in compaction; read once per run, applying to runs started afterwards.
  noa_compaction?: boolean;
  // ---- Vulnerability IM push (channels are a standalone resource, see /api/notify/*; only three global settings live here) ----
  notify_enabled?: boolean; // master push switch, on by default; one-click kill switch for maintenance windows
  notify_public_base_url?: string; // external base URL for vulnerability detail links; empty = messages carry no link
  notify_digest_interval_min?: number; // digest mode period (minutes), default 30
  // Render limits of IM messages; 0 = no truncation. The backend answers 400 outside the bounds.
  notify_summary_max_runes?: number; // summary characters per finding, 0-4000, default 120
  notify_assets_max_shown?: number; // asset names listed per finding, 0-50, default 3
  // Target noise profile; the stored setting wins over ARTEX_NOISE_PROFILE. Applies to the next request.
  noise_profile?: NoiseProfileName;
}

export type NoiseProfileName = "off" | "quiet" | "balanced" | "aggressive";

// GET /api/settings/noise: the current profile and its per-host counters (current minute, no reset).
export interface NoiseStatus {
  profile: NoiseProfileName;
  hosts: {
    host: string;
    rpm: number;
    peak_rpm: number;
    peak_inflight: number;
    inflight: number;
    duplicates_avoided: number;
  }[];
}

// ---- Vulnerability IM push ----

// NotificationFilter is a channel's filter conditions; all fields are optional, absent means no filtering.
// The backend validates none of them: a malformed config counts as "matched" (better to over-push than to miss).
export interface NotificationFilter {
  min_severity?: string; // "" | low | medium | high | critical
  task_ids?: number[]; // empty = no limit; non-empty requires overlap with the vulnerability's task
  asset_ids?: number[]; // empty = no limit; non-empty requires overlap with the vulnerability's anchored assets
  vulnclass_include?: string[]; // empty = accept all; non-empty requires the vulnerability type to match any keyword (case-insensitive substring)
  vulnclass_exclude?: string[]; // matching any keyword excludes (exclusion wins over inclusion)
  on_status_change?: boolean; // whether to also receive vulnerability status change events
}

// NotificationChannel is one channel instance. config fields vary by kind, and
// credential fields are replaced on read with mask values starting with "__masked__" — passing them back as-is means "do not change".
export interface NotificationChannel {
  id: number;
  name: string;
  kind: string;
  enabled: boolean;
  mode: "realtime" | "digest";
  config: Record<string, unknown>;
  filter: NotificationFilter;
  rate_per_min: number;
  created_at: string;
  updated_at: string;
  // secret_keys come from the backend per channel kind; the frontend renders password fields
  // and "leave empty to keep" hints from them, hardcoding no channel knowledge.
  secret_keys: string[];
}

// NotificationKind is the channel kind metadata returned by /api/notify/meta.
export interface NotificationKind {
  kind: string;
  default_rate_per_min: number;
  secret_keys: string[];
}

export interface NotificationMeta {
  kinds: NotificationKind[];
  enabled: boolean;
  public_base_url: string;
  digest_interval_min: string;
  defaults: { digest_interval_min: number };
  stats: {
    channels: number;
    channels_on: number;
    pending: number;
    failed: number;
    sent_today: number;
    backlog_age_ms: number;
  };
}

// NotificationDelivery is one delivery record, for delivery history and retrying failures.
export interface NotificationDelivery {
  id: number;
  finding_id: string;
  event_kind: string; // finding_created | finding_status_changed
  channel_id: number;
  channel_name: string;
  channel_kind: string;
  state: "pending" | "sending" | "sent" | "failed" | "skipped";
  attempts: number;
  last_error: string;
  batch_id?: number;
  created_at: string;
  sent_at?: string;
  next_attempt_at: string;
  title: string;
  severity: string;
}

// ---- LLM config ----
export interface LLMProfile {
  id: string;
  name: string;
  format: "openai" | "anthropic" | "openai-responses" | "ollama";
  base_url?: string;
  proxy?: string;
  model: string;
  api_key_hint?: string;
  rate_per_second: number;
  rate_per_minute: number;
  context_window_k?: number;
  // Thinking switch (thinking.type): ""=not sent (default) | "disabled"=off | "enabled"=on
  thinking_type?: string;
  // Thinking effort: ""=not sent (default) | "low"/"medium"/"high"/"xhigh"/"max"
  reasoning_effort?: string;
  is_default: boolean;
  // Pool order: the larger, the earlier chosen. The active profile is always chain head, regardless of this value.
  priority?: number;
  // true = not a failover target (still usable via explicit agent/task binding).
  pool_exclude?: boolean;
  // true (default) = streaming (SSE) | false = genuinely non-streaming (stream:false, one-shot response).
  streaming?: boolean;
  // Cost ledger: EUR per 1M tokens (0/absent = unset).
  price_input_per_m?: number;
  price_output_per_m?: number;
  price_cache_read_per_m?: number;
  price_cache_write_per_m?: number;
  // Output cap per reply (tokens). 0 = do not send the field; the server default decides.
  // Note the distinction from context_window_k: that is the model's total capacity, used locally only for the compression threshold.
  max_tokens?: number;
  // Which request field name carries the cap; only meaningful for format="openai":
  // ""=max_tokens (default) | "max_completion_tokens" (OpenAI reasoning models only accept it)
  max_tokens_field?: string;
  // Custom session header name: when non-empty every request carries this HTTP header, value = the current session/intent session id.
  // "" = not sent. For gateways that do prompt caching/sticky routing on a session-id header.
  session_header_key?: string;
  // Custom User-Agent: when non-empty, every outbound LLM request overrides its User-Agent header with this value. "" = do not override (keep the default UA).
  user_agent?: string;
  // This profile's retry overrides (connect / empty response / same-provider safety window). Empty / all zeros = follow the global policy.
  retry?: LLMRetryOverride;
}

// ---- LLM retry policy ----
// Two knobs per retry layer. Both use "0 = not configured":
//   attempts    0 = default count | -1 = disable this layer's retries | >0 = retry count
//   interval_ms 0 = default exponential backoff | >0 = use this fixed millisecond interval
export interface LLMRetryRule {
  attempts: number;
  interval_ms: number;
}

// The three layers a single LLM profile can override (all "endpoint-bound" retries).
export interface LLMRetryOverride {
  connect: LLMRetryRule; // connect retries: connection reset/timeout/429/5xx, before the stream starts
  empty: LLMRetryRule; // empty-response retries: completed but with no content (openai format only)
  stream: LLMRetryRule; // same-provider safety-window retries: replay of a dropped stream before output is delivered
}

// Global policy = defaults for the three layers above + two global-only layers:
//   breaker pool breaker (attempts = consecutive transient failures before tripping, interval_ms = fixed cooldown)
//   intent  intent rerun (the whole intent reruns after a worker ends with model_error)
export interface LLMRetryPolicy extends LLMRetryOverride {
  breaker: LLMRetryRule;
  intent: LLMRetryRule;
}

// ---- LLM pooling (failover) ----
// A profile's position and health in the pool chain. state:
//   ok       healthy
//   degraded consecutive failures but below the breaker threshold
//   tripped  breaker open, skipped during the cooldown (cooldown_secs = seconds left)
export interface LLMPoolMember {
  profile_id: string;
  name: string;
  model: string;
  format: string;
  priority: number;
  active: boolean; // whether this is the currently active profile (always chain head)
  excluded: boolean; // pool_exclude: not part of the pool
  state: "ok" | "degraded" | "tripped";
  fails: number;
  trips: number;
  cooldown_secs: number;
  last_error?: string;
  last_at?: string;
}

export interface LLMPoolStatus {
  enabled: boolean;
  bind_fallback: boolean;
  chain: LLMPoolMember[];
}

// ---- Agents ----
export interface Agent {
  id: string;
  key: string; // built-ins are goals/planner/mainagent/worker; custom agents use user-defined keys
  name: string;
  description?: string;
  role: string;
  builtin: boolean;
  enabled: boolean;
  llm_profile_id?: number | null; // bound LLM profile; null/absent = follow task/session/global
  max_turns?: number; // 0 = unlimited
  run_seconds?: number; // worker per-run wall-clock cap (seconds); 0 = unlimited
  web_search?: boolean; // whether web search is enabled (gated by the global system switch)
  interactive_shell?: boolean; // whether the interactive shell is enabled (persistent PTY session tool family)
  // P3 post-trigger handling policy (only meaningful for custom agents)
  trigger_run_mode?: "serial" | "parallel"; // queue serially / each trigger gets its own concurrent session
  trigger_merge_mode?: "by_task" | "all" | "none"; // serial only: merge per task / merge all / no merge
  trigger_max_parallel?: number; // parallel only: per-agent concurrency cap; 0 = unlimited
  // Binding counts (returned only by the list endpoint): visible MCP / visible Skill / bound tools
  mcp_count?: number;
  skill_count?: number;
  tool_count?: number;
}

export interface PromptVar {
  name: string;
  description: string;
  example: string;
  source: "exploration" | "runtime" | "distilled";
}

export interface PromptVersion {
  version: number;
  ts: string;
  note: string;
  template_text: string;
}

export interface AgentDetail {
  agent: Agent;
  prompt: string;
  variables: PromptVar[];
  versions: PromptVersion[];
  visibility: { mcp: number[]; skill: string[] };
  // Bindable LLM profile candidates (for the "default model" dropdown); the current binding is agent.llm_profile_id
  llm_profiles?: { id: number; name: string; model: string; is_default: boolean }[];
  wrapup_prompt?: string; // saved wrap-up prompt (empty = built-in default)
  wrapup_default?: string; // built-in default wrap-up prompt (placeholder / restore default)
  wrapup_max_turns?: number; // saved wrap-up turn count (0 = built-in default)
  wrapup_max_turns_default?: number; // built-in default wrap-up turn count (for the "0 = default N" hint)
  // Task-level timeout wrap-up prompt (worker/planner only; this section shows only when task_timeout_wrapup_supported=true)
  task_timeout_wrapup_supported?: boolean;
  task_timeout_wrapup_prompt?: string;
  task_timeout_wrapup_default?: string;
  task_timeout_wrapup_max_turns?: number;
  task_timeout_wrapup_max_turns_default?: number;
}

// ---- MCP ----
export interface MCPServer {
  id: number;
  name: string;
  transport: "stdio" | "http" | "sse";
  command?: string;
  args: string[];
  env: Record<string, string>;
  url?: string;
  enabled: boolean;
  insecure?: boolean; // http: skip TLS cert verification (self-signed servers)
  timeout_seconds?: number; // http/sse: per-request timeout in seconds (default 120)
  last_connect_at?: string | null; // last connect attempt (ISO time)
  last_connect_ok?: boolean | null; // outcome of the last connect attempt
  last_connect_err?: string | null; // failure reason when last_connect_ok is false
  tools?: string[]; // mcp_tools_cache (names only, for the count)
}

export interface MCPTool {
  name: string;
  description: string;
}

// ---- Skills ----
// Fields align with the agentskills.io open specification.
// description covers both "what the skill does" and "when to use it".
export interface SkillItem {
  name: string; // unique key = directory name
  description?: string; // required per spec; covers what + when to use
  license?: string; // optional: SPDX identifier or free text
  compatibility?: string; // optional: environment requirements
  mcps?: string[]; // MCP server names this skill unlocks on load
  files: string[]; // files in the skill directory
  // Call stats (the skill_usage ledger). A skill never called: calls=0, last_used absent.
  calls: number;
  tasks: number; // number of tasks that loaded it (chat sessions excluded)
  usage_agents: string[]; // agent keys that loaded it
  last_used?: string;
}

// SkillCall is one Skill() invocation (recent-call list for a single skill).
export interface SkillCall {
  ts: string;
  agent_key: string;
  task_id: number; // 0 = non-task context (chat session)
  session_id: string;
  args_len: number;
}

// MissingSkill is a skill that was named but does not exist — the "wanted but missing" gap.
export interface MissingSkill {
  skill: string;
  calls: number;
  agents: string[];
  last_used?: string;
}

// ---- Tools (built-in tool catalog) ----
// key + handler live in Go; only these fields are page-editable. system tools lock
// the key and the parameter *structure* (name/type/required) — the per-param
// description/default and the agent binding are what move.
export interface Tool {
  key: string;
  system: boolean;
  description: string;
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  schema: Record<string, any>; // full JSON-Schema (object with properties)
  agents: string[]; // bound agent keys
  enabled: boolean;
  kind?: "builtin" | "shell" | "command" | "script" | "http"; // custom tool kind
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  exec?: Record<string, any>; // custom tool execution spec (kind!=builtin)
  deferred?: boolean; // schema deferred (SearchExtraTools/ExecuteExtraTool)
  calls?: number; // persistent runtime invocation count (older APIs may omit it)
}

// ---- Stats ----
export interface Stats {
  assets: number;
  engine_mode: EngineMode;
  llm_configured: boolean;
  roe_enabled: boolean;
  findings_confirmed: number;
  active_task?: Partial<Task>;
}

// ---- Intercept Rules ----
export type InterceptAction = "allow" | "deny" | "ask";
export type InterceptMatchTarget = "tool_name" | "tool_input";
// argv: pattern is a JSON array of tokens, the prefix of a Bash command (program and subcommands; flags are not constrained).
export type InterceptMatchType = "string" | "regex" | "argv";

export interface InterceptRule {
  id: number;
  name: string;
  enabled: boolean;
  priority: number;
  match_target: InterceptMatchTarget;
  match_type: InterceptMatchType;
  pattern: string;
  action: InterceptAction;
  message: string;
  timeout_enabled: boolean;
  timeout_seconds: number;
  timeout_action: "deny" | "allow";
  created_at: string;
  updated_at: string;
}

// ---- Asset Intercept Rules (asset interception: global blocklist) ----
export type AssetInterceptKind =
  | "exact_domain"
  | "exact_ip"
  | "exact_url"
  | "fuzzy_domain"
  | "fuzzy_ip"
  | "fuzzy_url"
  | "cidr";

// action is only for task-level rules: block = intercept (forbid testing), allow = permit (allowlist).
export type AssetInterceptAction = "block" | "allow";

// Input row for task-level asset interception/allow rules (used when creating a task and editing task details).
export interface AssetInterceptRuleInput {
  action: AssetInterceptAction;
  kind: AssetInterceptKind;
  pattern: string;
  note: string;
  enabled: boolean;
}

// Task credentials (#3): per-task test accounts. An empty password on a user-declared
// entry means "agent generates": the agent registers that account and records it back.
export type TaskCredentialInput = {
  domain: string;
  username: string;
  password: string;
  note?: string;
};

export type TaskCredential = TaskCredentialInput & {
  id: number;
  task_id: number;
  source: "user" | "agent";
  note: string;
  created_at: string;
  updated_at: string;
};


export interface AssetInterceptRule {
  id: number;
  enabled: boolean;
  action?: AssetInterceptAction; // global rules omit this field (always intercept); task-level rules distinguish block/allow
  kind: AssetInterceptKind;
  pattern: string;
  note: string;
  builtin: boolean;
  created_at: string;
  updated_at: string;
}

export interface InterceptPending {
  decision_source?: "rule" | "model" | "unknown" | "";
  // Actor that settled the request, exactly as stored: "" while pending and on
  // rows decided before the column existed (shown as "Legacy").
  decided_by?: "human" | "rule" | "model" | "system" | "";
  id: number;
  rule_id?: number;
  conversation_id?: number;
  task_id?: string;
  agent_name: string;
  tool_name: string;
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  tool_input: Record<string, any>;
  status: "pending" | "allowed" | "denied" | "timeout";
  reason: string; // the rule's message or the model's verdict reason (model verdicts carry a [model] prefix)
  decided_at?: string;
  created_at: string;
  // Deterministic risk score, computed by the server when a pending row is read.
  risk?: InterceptRisk;
}

export interface InterceptRisk {
  score: number; // 0..100
  level: "low" | "medium" | "high";
  reasons: string[];
}

export interface InterceptBatchDecideResult {
  decided: number;
  failed: number;
  results: { id: number; ok: boolean; error?: string }[];
}

// JudgeConfig: global config for model fallback approval (the model decides only when no interception rule matches).
export interface JudgeConfig {
  enabled: boolean;
  mode?: "closed" | "freeform"; // closed (default): fixed questions, code decides; freeform: the prompt decides
  profile_id: number; // 0 = follow the active/default profile
  prompt: string; // verdict prompt; on GET the backend backfills the full built-in template when unset
  timeout_seconds: number; // model call timeout
  fail_action: "allow" | "ask" | "deny"; // fallback when the model errors/times out/is unparseable
  ask_timeout_seconds: number; // approval wait timeout after the model says ask and it goes to a human
  ask_timeout_action: "allow" | "deny"; // default action after the approval timeout
}

// JudgeUsage: cumulative token usage of model fallback approval (the judge channel) + daily series for the last N days.
export interface JudgeDayUsage {
  date: string; // YYYY-MM-DD (UTC)
  calls: number;
  input_tokens: number;
  output_tokens: number;
}
export interface JudgeUsage {
  calls: number;
  input_tokens: number;
  output_tokens: number;
  cache_read_tokens: number;
  cache_write_tokens: number;
  daily: JudgeDayUsage[];
}

export interface InterceptApprovalFilter {
  status?: InterceptPending["status"];
  decision_source?: "rule" | "model" | "unknown";
  decided_by?: "human" | "rule" | "model" | "system";
}

// InterceptApprovalRow enriches InterceptPending with conversation/task and rule context.
export interface InterceptApprovalRow extends InterceptPending {
  conv_title: string; // "" if no linked conversation
  conv_agent_key: string; // "" if no linked conversation
  rule_name: string; // "" if rule was deleted
}

// ── Asset sync (ScopeSentry data source) ──────────────────────────────────────────────
export interface SSProject {
  id: string; // MongoDB ObjectID — used as filter.project
  name: string;
  logo?: string;
  AssetCount?: number;
  tag?: string;
}

export interface SSTask {
  id: string;
  name: string; // used as filter.task
  status?: number;
  progress?: number;
  creatTime?: string;
  endTime?: string;
}

// ConvTokenSummary — one conversation's token total (+ profile/date) for merging
// chat usage into the dashboard token stats. GET /api/tokens/conversations.
export interface ConvTokenSummary {
  llm_profile_id: number | null;
  created_at: string;
  input_tokens: number;
  output_tokens: number;
  cache_read_tokens: number;
  cache_write_tokens: number;
}

// ---- Command recording (Bash execution history) ----
export interface CommandRecord {
  id: number;
  exploration_id: number;
  worker: string;
  tool: string;
  command: string; // raw tool input (JSON)
  output: string;
  is_error: boolean;
  created_at: string;
}

// Call stats for one tool (/commands/stats); errors is the failed subset.
export interface ToolStat {
  tool: string;
  total: number;
  errors: number;
}

// ---- LLM recording ----
export interface LLMRecordItem {
  id: number;
  ts: string;
  model: string;
  profile_name: string;
  session_id: string;
  task_id: string;
  worker: string;
  latency_ms: number;
  input_tokens: number;
  output_tokens: number;
  cache_read: number;
  cache_write: number;
  status: string;
  error?: string;
}

export interface LLMRecordDetail extends LLMRecordItem {
  request_body: string;
  response_body: string;
  // Raw HTTP actually exchanged with the provider: the request is the full body sent by
  // buildBody() (incl. tool schemas), the response is raw SSE frames. The
  // request_body/response_body above are the normalized view, dropping tool schemas and tool_use blocks. Empty for old records.
  raw_request?: string;
  raw_response?: string;
}

// One distinct task with its LLM-record count (task picker on the records page).
export interface LLMTask {
  task_id: string;
  count: number;
}

// The exact JSON sent to the review model, retained for all model verdicts.
export interface InterceptReviewInput {
  version: number;
  background?: {
    // worker_summary is retained only for immutable v2/v3 snapshots.
    source: "user_message" | "worker_summary";
    text: string;
    truncated?: boolean;
  };
  // Version 1 snapshots are immutable and remain readable in historical audits.
  task?: {
    task_id: number;
    description: string;
    goal: string;
    constraints: { id: number; kind: string; text: string; origin: string; created_at: number }[];
    truncated?: boolean;
  };
  working_directory?: string;
  worker_intent?: string;
  turn_input?: string;
  background_truncated?: boolean;
  // Legacy v1/v2 snapshots only; v3 never sends execution history.
  history?: {
    tool_use_id: string;
    tool: string;
    arguments_preview: string;
    result: string;
    status: "succeeded" | "failed";
    truncated?: boolean;
  }[];
  history_truncated?: boolean;
  correlation?: "exact" | "ambiguous" | "unavailable";
  tool_name: string;
  arguments: Record<string, unknown>;
}

// Immutable review snapshot plus separately recorded execution outcome.
export interface InterceptAudit {
  model_input?: InterceptReviewInput;
  model_input_digest?: string;
  run_id?: string;
  tool_use_id?: string;
  correlation: "exact" | "ambiguous" | "unavailable";
  input_digest: string;
  user_message: string;
  user_truncated?: boolean;
  context:
    | { kind: string; tool?: string; tool_use_id?: string; text: string; is_error?: boolean; truncated?: boolean }[]
    | null;
  context_truncated?: boolean;
  captured_at: string;
  model_fallback?: boolean;
  initial_action: "allow" | "ask" | "deny";
  initial_reason: string;
  effective_action?: "allow" | "deny";
  decision_reason?: string;
  rule_name?: string;
  config_digest?: string;
  profile_id?: number;
  judge_answers?: { id: string; answer: "yes" | "no" | "unknown"; evidence: string }[];
  execution_status: "not_started" | "not_executed" | "awaiting_result" | "succeeded" | "failed" | "unknown";
  output?: string;
  output_truncated?: boolean;
  execution_ended_at?: string;
}
export interface InterceptDetail extends InterceptApprovalRow {
  audit: InterceptAudit | null;
  always_allow_prefix?: string; // pending Bash request: the argv prefix "always allow" would allow
}

export type TrafficEvidenceRole = "baseline" | "proof" | "verification" | "supporting";
export interface TrafficEvidenceRef {
  traffic_id: string;
  role?: TrafficEvidenceRole;
  note?: string;
}
export interface TrafficEvidenceSnapshot {
  id: string;
  source_traffic_id: string;
  captured_at: number;
  url: string;
  method: string;
  status: number;
  content_type: string;
  req_head?: string;
  resp_head?: string;
  req_hash: string;
  resp_hash: string;
  req_len: number;
  resp_len: number;
  rewritten?: boolean;
  rewrite_rule_ids?: number[];
}
export interface FindingTrafficBinding {
  id: string;
  finding_id: string;
  snapshot_id: string;
  role: TrafficEvidenceRole;
  note: string;
  position: number;
  created_at: string;
  snapshot: TrafficEvidenceSnapshot;
}
export interface FindingTraffic {
  finding_id: string;
  version: number;
  report_version: number;
  bindings: FindingTrafficBinding[];
}
export interface EvidenceBodyPreview {
  content: string;
  offset: number;
  total: number;
  next_offset: number;
  truncated: boolean;
  binary: boolean;
}
export interface FindingTrafficDetail {
  binding: FindingTrafficBinding;
  request: EvidenceBodyPreview;
  response: EvidenceBodyPreview;
}

// Original execution selected from an approval, never submitted to the reviewer.
export interface InterceptExecution {
  conversation_id: number | null;
  task_id: string | null;
  session: string;
  seq: number;
  items: Activity[];
}

// ---- capture-proxy constrained Go tampering hooks (P4.2) ----
export type TamperHookPhase = "request" | "response";

export interface TamperHookHeaderMatcher {
  name: string;
  value_regex?: string;
}

export interface TamperHookMatchers {
  methods?: string[];
  host?: string;
  url_regex?: string;
  headers?: TamperHookHeaderMatcher[];
  body_regex?: string;
  status?: string[];
  resp_headers?: TamperHookHeaderMatcher[];
  resp_body_regex?: string;
}

export interface TamperHookLastError {
  class: string;
  at: string;
  count: number;
}

export interface TamperHook {
  id: number;
  name: string;
  enabled: boolean;
  phase: TamperHookPhase;
  priority: number;
  matchers: TamperHookMatchers;
  source: string;
  source_sha256: string;
  version: number;
  note: string;
  created_at: string;
  updated_at: string;
  load_error?: string;
  last_error?: TamperHookLastError | null;
}

export interface TamperHooksListResp {
  hooks: TamperHook[];
  enabled_by_env: boolean;
}

export interface TamperHookVersion {
  id: number;
  hook_id: number;
  version: number;
  name: string;
  phase: TamperHookPhase;
  priority: number;
  matchers: TamperHookMatchers;
  source: string;
  source_sha256: string;
  note: string;
  created_at: string;
}

export interface TamperHookInput {
  name: string;
  enabled?: boolean;
  phase: TamperHookPhase;
  priority: number;
  matchers: TamperHookMatchers;
  source: string;
  note?: string;
}

export interface TamperDryRunIO {
  request: {
    method: string;
    url: string;
    headers: Record<string, string[]>;
    body: string;
  };
  response: {
    status: number;
    headers: Record<string, string[]>;
    body: string;
  };
}

export interface TamperDryRunRequest extends TamperDryRunIO {
  hook_id?: number;
  name?: string;
  phase?: TamperHookPhase;
  matchers?: TamperHookMatchers;
  source?: string;
}

export interface TamperTraceLine {
  Phase: string;
  SourceKind: string;
  RuleID: number;
  HookID: number;
  HookVersion: number;
  RuleSnapshot: string;
  Action: string;
  Field: string;
  Outcome: string;
  Reason: string;
  BeforeHash: string;
  AfterHash: string;
  After: string | null;
}

export interface TamperDryRunResult extends TamperDryRunIO {
  changed: boolean;
  lines: TamperTraceLine[] | null;
}
