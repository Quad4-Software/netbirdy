import { test } from "node:test";
import assert from "node:assert/strict";
import { parseFrontMatter, slugify, transformCallouts, rewriteDocLinks, sanitizeHtml, extractHeadings, wrapCodeBlocks, htmlToText, buildPrevNext } from "./docs-lib.mjs";

test("parseFrontMatter reads fields", () => {
  const { fields, body } = parseFrontMatter('---\ntitle: "Hi There"\norder: 3\n---\nBody text', "f.md");
  assert.equal(fields.title, "Hi There"); assert.equal(fields.order, 3); assert.match(body, /Body/);
});
test("parseFrontMatter rejects missing block", () => assert.throws(() => parseFrontMatter("no fm", "f.md"), /missing front matter/));
test("slugify handles punctuation", () => { assert.equal(slugify("Connect Your First Peer!"), "connect-your-first-peer"); assert.equal(slugify("a  b--c"), "a-b-c"); });
test("transformCallouts renders note aside", () => {
  const out = transformCallouts("<blockquote><p><strong>Note</strong> careful here</p></blockquote>");
  assert.match(out, /data-callout="note"/); assert.match(out, /careful here/);
});
test("rewriteDocLinks maps md links and flags broken", () => {
  const reg = new Set(["guides/offline-mode.md".replace(".md",""), "getting-started/overview"]);
  const { html, broken } = rewriteDocLinks('<p><a href="./overview.md">a</a><a href="../guides/offline-mode.md">b</a><a href="./nope.md">c</a><a href="https://x.io">d</a></p>', "getting-started", reg);
  assert.match(html, /href="\/docs\/getting-started\/overview"/);
  assert.match(html, /href="\/docs\/guides\/offline-mode"/);
  assert.match(html, /target="_blank"/);
  assert.deepEqual(broken, ["./nope.md"]);
});
test("sanitizeHtml strips scripts and handlers", () => {
  const out = sanitizeHtml('<p onclick="x()">a</p><script>bad()</script><a href="javascript:x()">y</a>');
  assert.ok(!/script|onclick|javascript/.test(out));
});
test("extractHeadings adds ids", () => {
  const { html, headings } = extractHeadings("<h2>Big Title</h2><h3>Sub One</h3>");
  assert.equal(headings[0].id, "big-title"); assert.match(html, /id="sub-one"/);
});
test("wrapCodeBlocks adds data-lang", () => assert.match(wrapCodeBlocks('<pre><code class="language-bash">x</code></pre>'), /data-lang="bash"/));
test("htmlToText strips markup", () => assert.equal(htmlToText("<p>Hi <b>there</b></p>"), "Hi there"));
test("buildPrevNext navigates", () => {
  const pages = [{slug:"a",section:"s",title:"A"},{slug:"b",section:"s",title:"B"},{slug:"c",section:"s",title:"C"}];
  const r = buildPrevNext(pages, "b", "s");
  assert.equal(r.prev.title, "A"); assert.equal(r.next.title, "C");
});

test("transformCallouts: bare **Note** without colon", () => {
  const out = transformCallouts("<blockquote><p><strong>Note</strong> keep it private</p></blockquote>");
  assert.match(out, /data-callout="note"/); assert.match(out, /keep it private/);
});
test("transformCallouts: **Warning.** and Important", () => {
  assert.match(transformCallouts("<blockquote><p><strong>Warning.</strong> careful</p></blockquote>"), /data-callout="warning"/);
  assert.match(transformCallouts("<blockquote><p><strong>Important</strong>: read</p></blockquote>"), /data-callout="important"/);
});
test("transformCallouts: plain quote untouched", () => {
  const q = "<blockquote><p>ordinary quote</p></blockquote>";
  assert.equal(transformCallouts(q), q);
});

test("sanitizeHtml neutralizes quoted, single quoted and unquoted dangerous URLs", () => {
  assert.ok(!sanitizeHtml('<a href="javascript:alert(1)">x</a>').includes("javascript:"));
  assert.ok(!sanitizeHtml("<a href='javascript:alert(1)'>x</a>").includes("javascript:"));
  assert.ok(!sanitizeHtml("<a href=javascript:alert(1)>x</a>").includes("javascript:"));
  assert.ok(!sanitizeHtml('<a href=" java\tscript:alert(1)">x</a>').includes("script:alert"));
  assert.ok(!sanitizeHtml('<img src="data:text/html;base64,AAAA">').includes("data:text/html"));
  assert.ok(sanitizeHtml('<a href="https://x.test">x</a>').includes("https://x.test"));
});
