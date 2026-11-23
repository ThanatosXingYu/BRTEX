import assert from "node:assert/strict";
import test from "node:test";
import { httpHref } from "./http-href.ts";

test("httpHref allows http and https", () => {
  assert.equal(httpHref("https://example.com/a"), "https://example.com/a");
  assert.equal(httpHref("http://192.0.2.1:8080/x"), "http://192.0.2.1:8080/x");
  assert.equal(httpHref("  https://example.com/a  "), "https://example.com/a");
});

test("httpHref refuses javascript, data, and junk", () => {
  assert.equal(httpHref("javascript:alert(1)"), undefined);
  assert.equal(httpHref("JAVASCRIPT:alert(1)"), undefined);
  assert.equal(httpHref("data:text/html,x"), undefined);
  assert.equal(httpHref("vbscript:x"), undefined);
  assert.equal(httpHref("/relative"), undefined);
  assert.equal(httpHref(""), undefined);
  assert.equal(httpHref(null), undefined);
});
