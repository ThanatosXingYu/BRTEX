// Helpers for the Traffic > Hooks panel (P4.2 PR2):
// - Parse raw recorded HTTP responses and turn a recorded exchange into dry-run I/O
// - Parse/format header maps, header matchers, and CSV lists
// - Decode trace line After payloads and detect HMAC-masked fields

import { parseRawRequest } from "./replay";
import type { TamperDryRunIO, TamperHookHeaderMatcher, TamperHookMatchers, TamperTraceLine } from "./types";

export const DEFAULT_REQUEST_HOOK_SOURCE = `package hook

func OnRequest(req *Request) error {
\treq.SetHeader("X-Artex-Hook", "1")
\treturn nil
}
`;

export const DEFAULT_RESPONSE_HOOK_SOURCE = `package hook

func OnResponse(req *Request, resp *Response) error {
\tresp.SetHeader("X-Artex-Hook", "1")
\treturn nil
}
`;

const BLOB_MARKER = /\n?…\[truncated\] @blob sha256:[0-9a-f]{64} \(len=\d+\)$/;

export interface ParsedRawResponse {
  status: number;
  headers: Record<string, string[]>;
  body: string;
  binary: boolean;
  bodyPreviewOnly: boolean;
}

export function parseRawResponse(raw: string): ParsedRawResponse {
  if (!raw.trim()) {
    return { status: 200, headers: {}, body: "", binary: false, bodyPreviewOnly: false };
  }
  const separator = /\r?\n\r?\n/.exec(raw);
  const head = separator ? raw.slice(0, separator.index) : raw;
  let body = separator ? raw.slice(separator.index + separator[0].length) : "";
  const [statusLine = "", ...lines] = head.split(/\r?\n/);
  const statusMatch = /^HTTP\/\S+\s+(\d{3})\b/i.exec(statusLine.trim());
  const status = statusMatch ? Number.parseInt(statusMatch[1], 10) : 200;
  const headers: Record<string, string[]> = {};
  for (const line of lines) {
    const colon = line.indexOf(":");
    if (colon <= 0) continue;
    const name = line.slice(0, colon).trim();
    const value = line.slice(colon + 1).trim();
    if (!name) continue;
    if (!headers[name]) headers[name] = [];
    headers[name].push(value);
  }
  const bodyPreviewOnly = BLOB_MARKER.test(body);
  if (bodyPreviewOnly) body = body.replace(BLOB_MARKER, "");
  return {
    status,
    headers,
    body,
    binary: raw.includes("\u0000"),
    bodyPreviewOnly,
  };
}

export interface ExchangeDryRunConversion {
  io: TamperDryRunIO;
  reqBinary: boolean;
  respBinary: boolean;
  reqPreviewOnly: boolean;
  respPreviewOnly: boolean;
}

export function exchangeDetailToDryRunIO(
  detail: { req: string; resp: string },
  fallbackUrl?: string,
): ExchangeDryRunConversion {
  const req = parseRawRequest(detail.req || "");
  const resp = parseRawResponse(detail.resp || "");

  const reqHeaders: Record<string, string[]> = {};
  let hostHeader = "";
  for (const h of req.headers) {
    if (!h.name) continue;
    if (h.name.toLowerCase() === "host") {
      if (!hostHeader) hostHeader = h.value;
      continue;
    }
    if (!reqHeaders[h.name]) reqHeaders[h.name] = [];
    reqHeaders[h.name].push(h.value);
  }

  let url = fallbackUrl?.trim() || "";
  if (!url) {
    const target = req.target || "/";
    if (/^https?:\/\//i.test(target)) {
      url = target;
    } else {
      const host = hostHeader || "example.test";
      const path = target.startsWith("/") ? target : `/${target}`;
      url = `https://${host}${path}`;
    }
  }

  return {
    io: {
      request: {
        method: req.method || "GET",
        url,
        headers: reqHeaders,
        body: req.body,
      },
      response: {
        status: resp.status || 200,
        headers: resp.headers,
        body: resp.body,
      },
    },
    reqBinary: req.binary,
    respBinary: resp.binary,
    reqPreviewOnly: req.bodyPreviewOnly,
    respPreviewOnly: resp.bodyPreviewOnly,
  };
}

export function parseHeaderLines(text: string): Record<string, string[]> {
  const out: Record<string, string[]> = {};
  for (const rawLine of text.split(/\r?\n/)) {
    const line = rawLine.trim();
    if (!line) continue;
    const colon = line.indexOf(":");
    if (colon <= 0) continue;
    const name = line.slice(0, colon).trim();
    const value = line.slice(colon + 1).trim();
    if (!name) continue;
    if (!out[name]) out[name] = [];
    out[name].push(value);
  }
  return out;
}

export function formatHeaderLines(headers?: Record<string, string[]> | null): string {
  if (!headers) return "";
  const lines: string[] = [];
  for (const name of Object.keys(headers).sort()) {
    const vals = headers[name] ?? [];
    for (const v of vals) {
      lines.push(`${name}: ${v}`);
    }
  }
  return lines.join("\n");
}

export function parseHeaderMatcherLines(text: string): TamperHookHeaderMatcher[] {
  const out: TamperHookHeaderMatcher[] = [];
  for (const rawLine of text.split(/\r?\n/)) {
    const line = rawLine.trim();
    if (!line) continue;
    const colon = line.indexOf(":");
    if (colon < 0) {
      out.push({ name: line });
      continue;
    }
    const name = line.slice(0, colon).trim();
    const valueRegex = line.slice(colon + 1).trim();
    if (!name) continue;
    out.push(valueRegex ? { name, value_regex: valueRegex } : { name });
  }
  return out;
}

export function formatHeaderMatcherLines(matchers?: TamperHookHeaderMatcher[]): string {
  if (!matchers?.length) return "";
  return matchers.map((m) => (m.value_regex ? `${m.name}: ${m.value_regex}` : m.name)).join("\n");
}

export function parseCsvList(text: string, upper = false): string[] {
  return text
    .split(",")
    .map((s) => (upper ? s.trim().toUpperCase() : s.trim()))
    .filter(Boolean);
}

export function buildMatchers(fields: {
  methods: string;
  host: string;
  urlRegex: string;
  headers: string;
  bodyRegex: string;
  status: string;
  respHeaders: string;
  respBodyRegex: string;
  phase: "request" | "response";
}): TamperHookMatchers {
  const out: TamperHookMatchers = {};
  const methods = parseCsvList(fields.methods, true);
  if (methods.length) out.methods = methods;
  if (fields.host.trim()) out.host = fields.host.trim();
  if (fields.urlRegex.trim()) out.url_regex = fields.urlRegex.trim();
  const headers = parseHeaderMatcherLines(fields.headers);
  if (headers.length) out.headers = headers;
  if (fields.bodyRegex.trim()) out.body_regex = fields.bodyRegex.trim();
  if (fields.phase === "response") {
    const status = parseCsvList(fields.status, false);
    if (status.length) out.status = status;
    const respHeaders = parseHeaderMatcherLines(fields.respHeaders);
    if (respHeaders.length) out.resp_headers = respHeaders;
    if (fields.respBodyRegex.trim()) out.resp_body_regex = fields.respBodyRegex.trim();
  }
  return out;
}

// Decode Go's JSON-marshaled []byte (base64 string) into UTF-8 text.
export function decodeTraceAfter(after: string | null | undefined): string | null {
  if (after === null || after === undefined) return null;
  if (after === "") return "";
  try {
    const binary = atob(after);
    const bytes = new Uint8Array(binary.length);
    for (let i = 0; i < binary.length; i++) {
      bytes[i] = binary.charCodeAt(i);
    }
    return new TextDecoder("utf-8", { fatal: false }).decode(bytes);
  } catch {
    return after;
  }
}

// A trace line is masked/tainted when:
// - Outcome is "applied" and After is null (plaintext suppressed), OR
// - BeforeHash/AfterHash is a 128-bit keyed HMAC fingerprint (32 hex chars) instead of SHA-256 (64 hex chars).
export function isTraceLineMasked(line: TamperTraceLine): boolean {
  const isHmac = (h: string) => /^[0-9a-f]{32}$/i.test(h);
  if (isHmac(line.BeforeHash) || isHmac(line.AfterHash)) return true;
  return line.Outcome === "applied" && line.After === null;
}
