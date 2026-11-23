// Centralised status → color/label semantics, reused across the whole app.
// Spec §8.3: intent / coverage / task / severity each have a consistent color set.

import { t } from "@/i18n";

export type Tone = "neutral" | "blue" | "green" | "amber" | "red" | "rose" | "violet" | "slate";

export const toneClasses: Record<Tone, string> = {
  neutral: "bg-muted text-muted-foreground border-transparent",
  blue: "bg-blue-500/15 text-blue-600 dark:text-blue-400 border-blue-500/20",
  green: "bg-emerald-500/15 text-emerald-600 dark:text-emerald-400 border-emerald-500/20",
  amber: "bg-amber-500/15 text-amber-600 dark:text-amber-400 border-amber-500/20",
  red: "bg-red-500/15 text-red-600 dark:text-red-400 border-red-500/20",
  // rose is used for "Critical" — a solid high-emphasis fill clearly above the soft red outline of "High".
  rose: "bg-rose-600 text-white border-rose-600 dark:bg-rose-600 dark:text-white",
  violet: "bg-violet-500/15 text-violet-600 dark:text-violet-400 border-violet-500/20",
  slate: "bg-slate-500/15 text-slate-600 dark:text-slate-400 border-slate-500/20",
};

export const toneDot: Record<Tone, string> = {
  neutral: "bg-muted-foreground",
  blue: "bg-blue-500",
  green: "bg-emerald-500",
  amber: "bg-amber-500",
  red: "bg-red-500",
  rose: "bg-white",
  violet: "bg-violet-500",
  slate: "bg-slate-500",
};

interface StatusMeta {
  label: string;
  tone: Tone;
}

const intent: Record<string, StatusMeta> = {
  open: { label: t("Open"), tone: "slate" },
  running: { label: t("Running"), tone: "blue" },
  paused: { label: t("Paused"), tone: "amber" },
  done: { label: t("Completed"), tone: "green" },
  // blocked = model/API/network failures exhausted the retries; this intent barely explored anything (not an interception).
  blocked: { label: t("Error"), tone: "red" },
  // exhausted = the step/time budget cut the run short midway, only partial results written back (not "direction exhausted").
  exhausted: { label: t("Budget exhausted"), tone: "violet" },
  // stopped = legacy soft-deleted state (kept for historical data).
  stopped: { label: t("Stopped"), tone: "slate" },
  // deleted = the user soft-deleted this intent (nodes and lineage kept; see delete_reason for the reason).
  deleted: { label: t("Deleted"), tone: "slate" },
};

const task: Record<string, StatusMeta> = {
  created: { label: t("Created"), tone: "slate" },
  queued: { label: t("Queued"), tone: "amber" },
  running: { label: t("Running"), tone: "blue" },
  paused: { label: t("Paused"), tone: "amber" },
  done: { label: t("Completed"), tone: "green" },
  failed: { label: t("Failed"), tone: "red" },
  timeout: { label: t("Timed out"), tone: "amber" },
};

const severity: Record<string, StatusMeta> = {
  critical: { label: t("Critical"), tone: "rose" },
  high: { label: t("High"), tone: "red" },
  medium: { label: t("Medium"), tone: "amber" },
  low: { label: t("Low"), tone: "slate" },
};

const finding: Record<string, StatusMeta> = {
  pending: { label: t("Pending"), tone: "amber" },
  in_progress: { label: t("In progress"), tone: "blue" },
  confirmed: { label: t("Confirmed"), tone: "red" },
  resolved: { label: t("Resolved"), tone: "green" },
  fixed: { label: t("Fixed"), tone: "green" },
  false_positive: { label: t("False positive"), tone: "slate" },
  ignored: { label: t("Ignored"), tone: "neutral" },
  duplicate: { label: t("Duplicate"), tone: "neutral" },
  risk_accepted: { label: t("Risk accepted"), tone: "violet" },
};

const engine: Record<string, StatusMeta> = {
  exploring: { label: t("Exploring"), tone: "blue" },
  paused: { label: t("Paused"), tone: "amber" },
  stalled: { label: t("Stalled"), tone: "red" },
  idle: { label: t("Idle"), tone: "neutral" },
};

const goal: Record<string, StatusMeta> = {
  open: { label: t("In progress"), tone: "blue" },
  met: { label: t("Met"), tone: "green" },
  abandoned: { label: t("Abandoned"), tone: "slate" },
};

const audit: Record<string, StatusMeta> = {
  allow: { label: t("Allow"), tone: "green" },
  block: { label: t("Block"), tone: "red" },
};

const node: Record<string, StatusMeta> = {
  observed: { label: t("Observed"), tone: "slate" },
  confirmed: { label: t("Confirmed"), tone: "green" },
  tombstoned: { label: t("Tombstoned"), tone: "neutral" },
};

// Notification delivery states. sending uses blue rather than amber: it is not "a problem"
// but "claimed and sending now", which must stay distinguishable from pending's waiting.
const delivery: Record<string, StatusMeta> = {
  pending: { label: t("Pending"), tone: "amber" },
  sending: { label: t("Sending"), tone: "blue" },
  sent: { label: t("Sent"), tone: "green" },
  failed: { label: t("Failed"), tone: "red" },
  skipped: { label: t("Skipped"), tone: "neutral" },
};

const maps = {
  intent,
  task,
  severity,
  finding,
  engine,
  goal,
  audit,
  node,
  delivery,
} as const;

export type StatusDomain = keyof typeof maps;

export function statusMeta(domain: StatusDomain, key: string): StatusMeta {
  return maps[domain][key] ?? { label: t(key), tone: "neutral" };
}
