"use client";

import * as React from "react";

import { Loader2Icon, RefreshCwIcon } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { api } from "@/lib/api";
import type { TrafficStreamMessage } from "@/lib/types";
import { intlLocale, t } from "@/i18n";

const PAGE_SIZE = 50;
// Binary payloads loaded in full are shown as hex of their first 64 KiB.
const HEX_PREVIEW_BYTES = 64 * 1024;

function fmtBytes(n: number) {
  if (n <= 0) return "0 B";
  const units = ["B", "KB", "MB", "GB"];
  const i = Math.min(Math.floor(Math.log(n) / Math.log(1024)), units.length - 1);
  const v = n / 1024 ** i;
  return `${i === 0 ? v : v.toFixed(1)} ${units[i]}`;
}

function fmtDirection(direction: string) {
  if (direction === "c2s") return t("client -> server");
  if (direction === "s2c") return t("server -> client");
  return direction;
}

// Hex dump: 16 bytes per line, offset first.
function hexDump(bytes: Uint8Array) {
  const lines: string[] = [];
  for (let i = 0; i < bytes.length; i += 16) {
    const row = Array.from(bytes.subarray(i, i + 16), (b) => b.toString(16).padStart(2, "0")).join(" ");
    lines.push(`${i.toString(16).padStart(8, "0")}  ${row}`);
  }
  return lines.join("\n");
}

type FullPayload = { loading: boolean; text?: string; error?: string };

const payloadClass =
  "max-h-64 overflow-auto rounded-md bg-muted/50 p-2 font-mono text-xs break-all whitespace-pre-wrap";

function StreamMessageRow({ message }: { message: TrafficStreamMessage }) {
  const [full, setFull] = React.useState<FullPayload | null>(null);
  const binary = message.type === "binary";

  const loadFull = () => {
    if (!message.blob) return;
    setFull({ loading: true });
    api
      .trafficBlob(message.blob)
      .then((bytes) => {
        const text = binary
          ? `${hexDump(bytes.subarray(0, HEX_PREVIEW_BYTES))}\n${t("({count} bytes total)", { count: bytes.length })}`
          : new TextDecoder().decode(bytes);
        setFull({ loading: false, text });
      })
      .catch((e: Error) => setFull({ loading: false, error: e.message }));
  };

  if (message.type === "meta") {
    return (
      <div className="mx-5 my-3 space-y-1 rounded-md border border-amber-500/40 bg-amber-500/10 p-3">
        <div className="font-medium text-amber-700 text-xs dark:text-amber-400">
          {t("#{seq} Recording limit reached: later messages were not stored", { seq: message.seq })}
        </div>
        <pre className={payloadClass}>{message.payload}</pre>
      </div>
    );
  }

  return (
    <div className="space-y-1.5 border-b px-5 py-3 last:border-b-0">
      <div className="flex flex-wrap items-center gap-2 text-xs">
        <span className="font-mono text-muted-foreground tabular-nums">#{message.seq}</span>
        <Badge variant="outline">{fmtDirection(message.direction)}</Badge>
        <Badge variant="secondary">{t(message.type)}</Badge>
        {binary && message.encoding === "base64" && <Badge variant="outline">base64</Badge>}
        {message.truncated && <Badge variant="destructive">{t("truncated")}</Badge>}
        {message.event && <span className="font-mono">event: {message.event}</span>}
        {message.event_id && <span className="font-mono">id: {message.event_id}</span>}
        {message.retry ? <span className="font-mono">retry: {message.retry}</span> : null}
        <span className="ml-auto text-muted-foreground tabular-nums">
          {fmtBytes(message.size)} · {new Date(message.ts * 1000).toLocaleTimeString(intlLocale)}
        </span>
      </div>
      <pre className={payloadClass}>{full?.text ?? message.payload}</pre>
      {message.blob && (
        <div className="flex flex-wrap items-center gap-2 text-muted-foreground text-xs">
          <span className="break-all font-mono">blob {message.blob}</span>
          {!full?.text && (
            <Button variant="outline" size="xs" disabled={full?.loading} onClick={loadFull}>
              {full?.loading && <Loader2Icon className="animate-spin" />}
              {t("Load full payload")}
            </Button>
          )}
          {full?.error && <span className="text-destructive">{full.error}</span>}
        </div>
      )}
    </div>
  );
}

// StreamMessagesPanel lists the messages of one WebSocket or SSE exchange, a
// page at a time. It does not poll: Refresh reloads from the start.
export function StreamMessagesPanel({ exchangeId, kind }: { exchangeId: string; kind: "websocket" | "sse" }) {
  const [items, setItems] = React.useState<TrafficStreamMessage[]>([]);
  const [total, setTotal] = React.useState(0);
  const [loading, setLoading] = React.useState(true);
  const [loadingMore, setLoadingMore] = React.useState(false);
  const [error, setError] = React.useState<string | null>(null);
  // Answers for an older exchange (or an older refresh) are dropped.
  const generation = React.useRef(0);

  const load = React.useCallback(
    (offset: number) => {
      const gen = offset === 0 ? ++generation.current : generation.current;
      if (offset === 0) {
        setLoading(true);
        setError(null);
      } else {
        setLoadingMore(true);
      }
      api
        .trafficStream(exchangeId, offset, PAGE_SIZE)
        .then((page) => {
          if (gen !== generation.current) return;
          const next = page.items ?? [];
          setItems((prev) => (offset === 0 ? next : [...prev, ...next]));
          setTotal(page.total ?? 0);
        })
        .catch((e: Error) => {
          if (gen === generation.current) setError(e.message);
        })
        .finally(() => {
          if (gen !== generation.current) return;
          setLoading(false);
          setLoadingMore(false);
        });
    },
    [exchangeId],
  );

  React.useEffect(() => {
    setItems([]);
    setTotal(0);
    load(0);
  }, [load]);

  return (
    <div className="flex flex-col">
      <div className="flex items-center gap-2 px-5 py-3 text-muted-foreground text-xs">
        <span>
          {t("{kind} · {count} of {total} messages", {
            kind: kind === "websocket" ? "WebSocket" : "Server-Sent Events",
            count: items.length,
            total,
          })}
        </span>
        <Button variant="outline" size="xs" className="ml-auto" disabled={loading} onClick={() => load(0)}>
          <RefreshCwIcon className={loading ? "animate-spin" : undefined} />
          {t("Refresh")}
        </Button>
      </div>
      {renderBody()}
    </div>
  );

  function renderBody() {
    if (loading) {
      return (
        <div className="flex items-center gap-2 p-5 text-muted-foreground text-xs">
          <Loader2Icon className="size-3.5 animate-spin" />
          {t("Loading messages…")}
        </div>
      );
    }
    if (error) {
      return <div className="p-5 text-destructive text-xs">{t("Failed to load messages: {error}", { error })}</div>;
    }
    if (items.length === 0) {
      return <div className="p-5 text-muted-foreground text-xs">{t("No messages recorded for this stream.")}</div>;
    }
    return (
      <>
        <div className="border-t">
          {items.map((m) => (
            <StreamMessageRow key={`${exchangeId}:${m.seq}`} message={m} />
          ))}
        </div>
        {items.length < total && (
          <div className="flex justify-center p-4">
            <Button variant="outline" size="sm" disabled={loadingMore} onClick={() => load(items.length)}>
              {loadingMore && <Loader2Icon className="animate-spin" />}
              {t("Load more")}
            </Button>
          </div>
        )}
      </>
    );
  }
}
