#!/usr/bin/env node
// One-off importer: converts selected MDX pages from the upstream
// netbirdio/docs repo (BSD-3-Clause) into dashboard/content/docs/upstream/.
// Refresh: UPSTREAM_DIR=/path/to/docs node scripts/import-upstream-docs.mjs

import { promises as fs } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const DASH = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const UP = process.env.UPSTREAM_DIR || "/tmp/upstream-docs";
const OUT = path.join(DASH, "content", "docs", "upstream");
const IMG_OUT = path.join(DASH, "public", "docs-img");
const UPSTREAM_SHA = process.env.UPSTREAM_SHA || "2382247";
const ADAPTED =
  "> **Note** This page is adapted from the upstream NetBird documentation (BSD-3-Clause) and may describe features that NetBirdy does not include.\n\n";

// Ordered list of upstream pages (relative to src/pages) to import.
const PAGES = (await fs.readFile(new URL("./upstream-pages.txt", import.meta.url), "utf8"))
  .split("\n").map((l) => l.trim()).filter((l) => l && !l.startsWith("#"));

const TITLES = {
  "how-netbird-works": "How NetBird Works",
};

// slugify a filename or title
const slugify = (s) => s.toLowerCase().replace(/\.mdx?$/i, "").replace(/[^\p{L}\p{N}\s-]/gu, "").trim().replace(/[\s_]+/g, "-").replace(/-+/g, "-").replace(/^-|-$/g, "");

// stripJSX: crude MDX->MD conversion for the components used upstream.
export function stripJSX(src) {
  let s = src;
  // imports/exports
  s = s.replace(/^import\s+.*$/gm, "").replace(/^export\s+(const|default).*\n?/gm, "");
  // <Note>/<Warning>/<Tip>/<Success>/<Danger>/<Important> -> blockquote
  s = s.replace(
    /<(Note|Warning|Tip|Success|Danger|Important)\b[^>]*>([\s\S]*?)<\/\1>/gi,
    (_m, kind, inner) => {
      const k = kind === "Success" ? "Tip" : kind;
      const body = inner.trim().split("\n").map((l) => `> ${l}`.trimEnd()).join("\n");
      return `> **${k}** ${body.split("\n").map((l, i) => (i === 0 ? l.slice(2).trim() : l)).join("\n")}\n`;
    },
  );
  // self-closing layout/structural tags and their containers
  s = s.replace(/<\/?(Row|Col|Properties|Tiles|CodeGroup|Frame|Card|Columns|Steps|Step|Accordion|AccordionGroup|Tabs|Tab|Section|Snippet)\b[^>]*>/g, "");
  // <Property name="x" type="y"> -> **x** (y):
  s = s.replace(/<Property\s+name="([^"]+)"(?:\s+type="([^"]*)")?[^>]*>/g, (_m, n, t) => `\n- **${n}**${t ? ` (${t})` : ""} — `);
  s = s.replace(/<\/Property>/g, "");
  // <Image src="..."/> -> markdown image
  s = s.replace(/<Image[^>]*src="([^"]+)"[^>]*\/?>/g, (_m, src) => `\n![](${src})\n`);
  // <YouTube id> -> link
  s = s.replace(/<YouTube[^>]*videoId="([^"]+)"[^>]*\/?>/g, (_m, id) => `\n[Watch video](https://youtube.com/watch?v=${id})\n`);
  // remaining JSX tags -> drop tag but keep children
  s = s.replace(/<\/?[A-Z][A-Za-z]*\b[^>]*>/g, "");
  return s;
}

// dropCloudSections removes H2/H3 blocks whose heading mentions cloud-only terms.
export function dropCloudSections(src) {
  const lines = src.split("\n");
  const out = [];
  let skipDepth = 0;
  const re = /^(#{1,4})\s.*\b(Cloud|Enterprise|Billing|MSP|NetBird Cloud|Pricing)\b/i;
  for (const line of lines) {
    const h = /^(#{1,4})\s/.exec(line);
    if (h) {
      if (skipDepth && h[1].length > skipDepth) {
        continue; // still inside a dropped section
      }
      skipDepth = 0;
      if (re.test(line)) {
        skipDepth = h[1].length;
        continue;
      }
    }
    if (!skipDepth) out.push(line);
  }
  return out.join("\n");
}

// rewriteLinks: relative .mdx links between imported pages -> ./slug.md;
// links to non-imported upstream pages -> absolute docs.netbird.io URL.
export function rewriteLinks(src, relPath, imported) {
  return src.replace(/\]\(([^)]+)\)/g, (m, href) => {
    if (/^(https?:|mailto:|#)/.test(href)) {
      if (href.includes("docs.netbird.io")) return m;
      return m;
    }
    const [p, hash] = href.split("#");
    if (!/\.mdx?$/i.test(p)) return m;
    // resolve relative to the page dir
    const pageDir = path.posix.dirname(relPath);
    let target = path.posix.normalize(path.posix.join(pageDir, p));
    if (imported.has(target)) {
      const sec = target.split("/").slice(0, -1).join("/");
      const slug = slugify(path.basename(target).replace(/\.mdx?$/i, ""));
      const cur = pageDir.split("/").slice(0, -1).join("/");
      const rel = sec === cur ? `./${slug}.md` : `../${sec}/${slug}.md`;
      return `](${rel}${hash ? `#${hash}` : ""})`;
    }
    return `](https://docs.netbird.io/${target.replace(/\.mdx?$/i, "")}${hash ? `#${hash}` : ""})`;
  });
}

const imported = new Set(PAGES);
await fs.mkdir(OUT, { recursive: true });
await fs.mkdir(IMG_OUT, { recursive: true });
let totalImg = 0, imgCount = 0;
const written = [];

for (const rel of PAGES) {
  const srcPath = path.join(UP, "src/pages", rel);
  let src;
  try { src = await fs.readFile(srcPath, "utf8"); } catch { console.error("MISS", rel); continue; }

  // front matter from export const description or first heading
  const titleMatch = /^#\s+(.+)$/m.exec(src);
  const descMatch = /export const description\s*=\s*['"`]([^'"`]+)['"`]/.exec(src);
  const title = TITLES[slugify(path.basename(rel).replace(/\.mdx?$/i, ""))] || (titleMatch ? titleMatch[1].trim() : slugify(rel));
  const description = descMatch ? descMatch[1] : "";

  let body = src;
  // images: copy referenced files if small enough
  const imgRefs = [...body.matchAll(/(?:src="|!\[[^\]]*\]\()([^"\s)]+?\.(?:png|jpe?g|webp|gif|svg))/g)];
  for (const r of imgRefs) {
    const imgRel = r[1].replace(/^\//, "");
    const srcImg = path.join(UP, "public", imgRel);
    try {
      const st = await fs.stat(srcImg);
      if (st.size > 400 * 1024 || totalImg + st.size > 5 * 1024 * 1024) {
        body = body.replaceAll(r[1], ""); // drop the reference
        continue;
      }
      const flat = imgRel.replace(/\//g, "-");
      await fs.copyFile(srcImg, path.join(IMG_OUT, flat));
      body = body.replaceAll(r[1], `/docs-img/${flat}`);
      totalImg += st.size; imgCount++;
    } catch { /* referenced but missing: leave */ }
  }

  body = stripJSX(body);
  body = dropCloudSections(body);
  body = rewriteLinks(body, rel, imported);
  // drop the original first H1 (we render title from front matter)
  body = body.replace(/^#\s+.+\n/, "");

  const section = rel.split("/")[0].replace(/^manage$/, "manage").replace(/^selfhosted$/, "selfhosted").replace(/^about-netbird$/, "about").replace(/^get-started$/, "get-started").replace(/^client$/, "client");
  // index pages take the parent dir name so slugs stay unique within a section
  const base = path.basename(rel).replace(/\.mdx?$/i, "");
  const slug = base === "index"
    ? slugify(path.basename(path.dirname(rel)))
    : slugify(base);
  const dir = path.join(OUT, section);
  await fs.mkdir(dir, { recursive: true });
  const outPath = path.join(dir, `${slug}.md`);
  const fm = [
    "---",
    `title: "${title.replace(/"/g, '\\"')}"`,
    `description: "${description.replace(/"/g, '\\"')}"`,
    `order: ${100 + written.length}`,
    `upstream: "${rel}@${UPSTREAM_SHA}"`,
    "---",
    "",
    ADAPTED + body.trim(),
    "",
  ].join("\n");
  await fs.writeFile(outPath, fm);
  written.push(`${section}/${slug}`);
}

console.log(`imported ${written.length} pages, ${imgCount} images (${(totalImg / 1024).toFixed(0)} KB)`);
for (const w of written) console.log(" ", w);
