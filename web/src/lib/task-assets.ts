import { t } from "@/i18n";
import type { NewAssetType } from "@/lib/types";

const ASSET_TYPE_LABELS: Record<NewAssetType, string> = {
  app: t("App"),
  endpoint: t("Endpoint"),
  ip: "IP",
  root_domain: t("Root domain"),
  service: t("Service"),
  subdomain: t("Subdomain"),
};

const TASK_ASSET_SOURCE_LABELS: Record<string, string> = {
  agent: t("Agent-discovered"),
  anchor: t("Blackboard anchor"),
  api: t("Assets API"),
  company: t("Company association"),
  legacy: t("Legacy association"),
  manual: t("Manually added"),
  system: t("System association"),
  task: t("Task initialization"),
};

export function taskAssetTypeLabel(type: NewAssetType): string {
  return ASSET_TYPE_LABELS[type];
}

export function taskAssetSourceLabel(source: string): string {
  return TASK_ASSET_SOURCE_LABELS[source] ?? source;
}
