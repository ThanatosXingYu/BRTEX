// Helpers for the traffic Replay tab: turn a recorded raw request into an
// editable form, and decide which headers the server will not send.

export interface ParsedHeader {
  name: string;
  value: string;
}

export interface ParsedRawRequest {
  method: string;
  target: string;
  headers: ParsedHeader[];
  body: string;
  // The body holds a NUL byte, so it is binary and cannot be edited as text.
  binary: boolean;
  // The recorded body was stored separately; only its preview is in the text.
  bodyPreviewOnly: boolean;
}

// The recorder appends this marker after the inline preview of a body that was
// spilled to the blob store (traffic/traffic.go, assembleRaw).
const BLOB_MARKER = /\n?…\[truncated\] @blob sha256:[0-9a-f]{64} \(len=\d+\)$/;

// parseRawRequest splits a recorded request ("METHOD target PROTO", header
// lines, a blank line, the body) into its parts. LF and CRLF line endings are
// both accepted in the head; the body is kept byte for byte (a multipart body
// needs its CRLFs). A header value keeps everything after the first ":".
export function parseRawRequest(raw: string): ParsedRawRequest {
  const separator = /\r?\n\r?\n/.exec(raw);
  const head = separator ? raw.slice(0, separator.index) : raw;
  let body = separator ? raw.slice(separator.index + separator[0].length) : "";
  const [requestLine = "", ...lines] = head.split(/\r?\n/);
  const [method = "", target = ""] = requestLine.trim().split(/\s+/);
  const headers: ParsedHeader[] = [];
  for (const line of lines) {
    const colon = line.indexOf(":");
    if (colon <= 0) continue;
    headers.push({ name: line.slice(0, colon).trim(), value: line.slice(colon + 1).trim() });
  }
  const bodyPreviewOnly = BLOB_MARKER.test(body);
  if (bodyPreviewOnly) body = body.replace(BLOB_MARKER, "");
  return {
    method: method.toUpperCase(),
    target,
    headers,
    body,
    binary: raw.includes("\u0000"),
    bodyPreviewOnly,
  };
}

// Headers the replay endpoint never sends as given: Host comes from the URL,
// the rest are hop-by-hop or recomputed by the client (server: traffic/replay.go).
const NOT_SENT = new Set([
  "host",
  "content-length",
  "transfer-encoding",
  "connection",
  "proxy-connection",
  "proxy-authorization",
  "keep-alive",
  "te",
  "trailer",
  "expect",
  "upgrade",
]);

// splitReplayHeaders separates the headers that will be sent from those the
// server drops (or, for Host and Upgrade, refuses). Headers named in a
// Connection header are hop-by-hop too.
export function splitReplayHeaders(headers: ParsedHeader[]): { sent: ParsedHeader[]; notSent: string[] } {
  const connectionNamed = new Set<string>();
  for (const h of headers) {
    if (h.name.trim().toLowerCase() !== "connection") continue;
    for (const token of h.value.split(",")) {
      const name = token.trim().toLowerCase();
      if (name) connectionNamed.add(name);
    }
  }
  const sent: ParsedHeader[] = [];
  const notSent: string[] = [];
  for (const h of headers) {
    const key = h.name.trim().toLowerCase();
    if (!key) continue;
    if (NOT_SENT.has(key) || connectionNamed.has(key)) {
      if (!notSent.some((n) => n.toLowerCase() === key)) notSent.push(h.name.trim());
    } else {
      sent.push({ name: h.name.trim(), value: h.value });
    }
  }
  return { sent, notSent };
}
