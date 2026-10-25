#!/usr/bin/env node
// Builds src/generated/docs.json from dashboard/content/docs/<section>/<slug>.md.
// Front matter: title (required), description, order, section label optional,
// source: ../path/relative/to/dashboard (pulls the body from a repo file).

import { marked } from "marked";
import { promises as fs } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import {
  parseFrontMatter,
  slugify,
  transformCallouts,
  rewriteDocLinks,
  sanitizeHtml,
  extractHeadings,
  wrapCodeBlocks,
  htmlToText,
} from "./docs-lib.mjs";

const DASH = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const CONTENT = path.join(DASH, "content", "docs");
const OUT = path.join(DASH, "src", "generated", "docs.json");

marked.use({ gfm: true, breaks: false });

async function* walk(dir) {
  for (const e of await fs.readdir(dir, { withFileTypes: true })) {
    const p = path.join(dir, e.name);
    if (e.isDirectory()) yield* walk(p);
    else if (/\.mdx?$/i.test(e.name)) yield p;
  }
}

const files = [];
for await (const f of walk(CONTENT)) files.push(f);

// First pass: read front matter, collect slugs, build the link registry.
const pages = [];
const registry = new Set();
const seen = new Map();
for (const file of files) {
  const rel = path.relative(CONTENT, file);
  const section = rel.split(path.sep)[0];
  const slug = slugify(path.basename(file).replace(/\.mdx?$/i, ""));
  const raw = await fs.readFile(file, "utf8");
  const { fields, body } = parseFrontMatter(raw, rel);
  const key = `${section}/${slug}`;
  if (seen.has(key)) {
    console.error(`docs: duplicate slug ${key} (${file} and ${seen.get(key)})`);
    process.exit(1);
  }
  seen.set(key, file);
  registry.add(key);
  pages.push({ file, rel, section, slug, fields, body });
}

// Second pass: render, rewrite links, sanitize, index.
let failures = 0;
const out = [];
for (const p of pages) {
  let body = p.body;
  if (p.fields.source) {
    const src = path.resolve(DASH, p.fields.source);
    try {
      body = await fs.readFile(src, "utf8");
      // The sourced file may itself carry front matter: drop it.
      try {
        body = parseFrontMatter(body, p.fields.source).body;
      } catch {
        // no front matter: use as is
      }
    } catch {
      console.error(`docs: ${p.rel}: source not found: ${p.fields.source}`);
      failures++;
      continue;
    }
  }
  let html = marked.parse(body, { async: false });
  const { html: linked, broken } = rewriteDocLinks(html, p.section, registry);
  if (broken.length) {
    console.error(`docs: ${p.rel}: broken internal links: ${broken.join(", ")}`);
    failures++;
  }
  const { html: headed, headings } = extractHeadings(linked);
  html = wrapCodeBlocks(transformCallouts(sanitizeHtml(headed)));
  out.push({
    slug: p.slug,
    section: p.section,
    title: p.fields.title,
    description: p.fields.description || "",
    order: typeof p.fields.order === "number" ? p.fields.order : 999,
    html,
    headings,
    text: htmlToText(html).slice(0, 20000),
    file: p.rel,
  });
}
if (failures) process.exit(1);

out.sort((a, b) => a.order - b.order || a.title.localeCompare(b.title));
const sections = [...new Set(out.map((p) => p.section))];
await fs.mkdir(path.dirname(OUT), { recursive: true });
await fs.writeFile(
  OUT,
  JSON.stringify({ generated: new Date().toISOString(), sections, pages: out }, null, 2),
);
// Link map: upstream docs.netbird.io path -> internal /docs path, derived from
// the `upstream:` front matter (src/pages/<path>[ /index].mdx -> /<path>).
const links = {};
for (const p of out) {
  const src = p.fields_source || null;
}
const linkMap = {};
for (const file of files) {
  const raw = await fs.readFile(file, "utf8");
  const { fields } = parseFrontMatter(raw, path.relative(CONTENT, file));
  if (!fields.upstream) continue;
  const rel = String(fields.upstream).replace(/@.+$/, "");
  const route = "/" + rel.replace(/\/index\.mdx?$/i, "").replace(/\.mdx?$/i, "");
  const section = path.relative(CONTENT, file).split(path.sep)[0];
  const slug = path.basename(file).replace(/\.mdx?$/i, "");
  linkMap[`https://docs.netbird.io${route}`] = `/docs/${section}/${slug}`;
}
await fs.writeFile(
  path.join(DASH, "src", "generated", "docs-links.json"),
  JSON.stringify(linkMap, null, 2),
);
console.log(`docs: ${out.length} pages in ${sections.length} sections -> ${path.relative(DASH, OUT)}`);
