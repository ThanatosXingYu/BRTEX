// httpHref returns raw when it is an http(s) URL, otherwise undefined.
// Asset URLs are operator/target-supplied; javascript: and data: must not
// become href values.
export function httpHref(raw: string | undefined | null): string | undefined {
  const s = (raw ?? "").trim();
  if (!s) {
    return undefined;
  }
  try {
    const u = new URL(s);
    if (u.protocol === "http:" || u.protocol === "https:") {
      return u.href;
    }
  } catch {
    // not a URL
  }
  return undefined;
}
