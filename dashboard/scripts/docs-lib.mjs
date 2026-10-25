// Pure helpers for the docs build pipeline. Imported by build-docs.mjs and
// unit-tested directly; no Node-only APIs belong here.

// parseFrontMatter splits "<yaml-ish>---\nkey: value\n---\nbody" into fields.
// Supported values: bare strings, quoted strings, integers.
export function parseFrontMatter(raw, file = "") {
  const m = /^---\r?\n([\s\S]*?)\r?\n---\r?\n?([\s\S]*)$/.exec(raw);
  if (!m) {
    throw new Error(`${file}: missing front matter (expected --- block)`);
  }
  const fields = {};
  for (const line of m[1].split(/\r?\n/)) {
    const kv = /^([A-Za-z_][\w-]*)\s*:\s*(.*)$/.exec(line.trim());
    if (!kv) {
      if (line.trim() === "") continue;
      throw new Error(`${file}: bad front matter line: ${line}`);
    }
    let v = kv[2].trim();
    if ((v.startsWith('"') && v.endsWith('"')) || (v.startsWith("'") && v.endsWith("'"))) {
      v = v.slice(1, -1);
    } else if (/^-?\d+$/.test(v)) {
      v = parseInt(v, 10);
    }
    fields[kv[1]] = v;
  }
  if (!fields.title || typeof fields.title !== "string") {
    throw new Error(`${file}: front matter requires a title`);
  }
  return { fields, body: m[2] };
}

// slugify turns a heading or filename into a URL-safe slug.
export function slugify(text) {
  return String(text)
    .toLowerCase()
    .replace(/&[a-z]+;/g, "")
    .replace(/<[^>]+>/g, "")
    .replace(/[^\p{L}\p{N}\s-]/gu, "")
    .trim()
    .replace(/[\s_]+/g, "-")
    .replace(/-+/g, "-")
    .replace(/^-|-$/g, "");
}

// CALLOUT_RE matches a blockquote whose first content is **Kind**.
const CALLOUT_RE = /^<blockquote>\s*<p>\s*<strong>(Note|Tip|Warning|Danger)<\/strong>[:.]?\s*/i;
const CALLOUT_ICONS = {
  note: "i",
  tip: "!",
  warning: "!",
  danger: "!",
  important: "!",
};

// transformCallouts converts "> **Note** text" blockquotes (post-marked HTML)
// into aside callouts. Returns the transformed HTML.
export function transformCallouts(html) {
  return html.replace(
    /<blockquote>([\s\S]*?)<\/blockquote>/gi,
    (block, inner) => {
      const m = /<p>\s*<strong>(Note|Tip|Warning|Danger|Important)[\.:]?<\/strong>[:.]?\s*([\s\S]*)$/i.exec(inner.trim());
      if (!m) return block;
      const kind = m[1].toLowerCase();
      const body = m[2].replace(/<\/p>\s*$/, "");
      const icon = CALLOUT_ICONS[kind] || "i";
      const label = m[1];
      return `<aside class="nb-callout nb-callout-${kind}" data-callout="${kind}"><p class="nb-callout-title"><span class="nb-callout-icon">${icon}</span>${label}</p><p>${body}</p></aside>`;
    },
  );
}

// rewriteDocLinks rewrites relative markdown links in rendered HTML to /docs
// routes. hrefs may point at ./x.md, x.md, ../sec/x.md (resolved against the
// current section) — the registry (set of "section/slug") validates them.
// Returns { html, broken: string[] } with the unresolved internal links.
export function rewriteDocLinks(html, section, registry) {
  const broken = [];
  const out = html.replace(/href="([^"]+)"/g, (match, href) => {
    if (/^https?:\/\//i.test(href)) {
      return `href="${href}" target="_blank" rel="noopener noreferrer"`;
    }
    if (href.startsWith("#") || href.startsWith("mailto:")) return match;
    const [pathPart, hashPart] = href.split("#");
    if (!/\.mdx?$/i.test(pathPart)) return match;
    // Resolve ../section/slug.md and ./slug.md against the current section.
    let target;
    if (pathPart.startsWith("../")) {
      target = pathPart.replace(/^\.\.\//, "").replace(/\.mdx?$/i, "");
    } else {
      const base = pathPart.replace(/^\.?\//, "").replace(/\.mdx?$/i, "");
      target = `${section}/${base}`;
    }
    if (!registry.has(target)) {
      broken.push(href);
      return match;
    }
    return `href="/docs/${target}${hashPart ? `#${hashPart}` : ""}"`;
  });
  return { html: out, broken };
}

// sanitizeHtml strips raw script/style/iframe blocks and on* attributes.
// Content is trusted; this is a defence-in-depth allowlist pass.
export function sanitizeHtml(html) {
  return html
    .replace(/<(script|style|iframe|object|embed|form)[\s\S]*?<\/\1>/gi, "")
    .replace(/<script[^>]*\/>/gi, "")
    .replace(/\s+on[a-z]+\s*=\s*("[^"]*"|'[^']*'|[^\s>]+)/gi, "")
    .replace(/href="javascript:[^"]*"/gi, 'href="#"');
}

// extractHeadings pulls h2/h3 from rendered HTML and gives each an id.
export function extractHeadings(html) {
  const headings = [];
  const out = html.replace(/<h([23])>([\s\S]*?)<\/h\1>/g, (match, level, inner) => {
    const text = inner.replace(/<[^>]+>/g, "").trim();
    const id = slugify(text);
    headings.push({ id, text, level: parseInt(level, 10) });
    return `<h${level} id="${id}"><a class="nb-anchor" href="#${id}" aria-hidden="true">#</a>${inner}</h${level}>`;
  });
  return { html: out, headings };
}

// wrapCodeBlocks wraps <pre><code class="language-x"> in a container with a
// data-lang attribute for styling and copy buttons.
export function wrapCodeBlocks(html) {
  return html.replace(
    /<pre><code(?:\s+class="language-([\w-]+)")?>([\s\S]*?)<\/code><\/pre>/g,
    (match, lang, code) => {
      const l = lang || "text";
      return `<div class="nb-codeblock" data-lang="${l}"><pre><code class="language-${l}">${code}</code></pre></div>`;
    },
  );
}

// htmlToText produces a plain-text search index from rendered HTML.
export function htmlToText(html) {
  return html
    .replace(/<script[\s\S]*?<\/script>/gi, "")
    .replace(/<style[\s\S]*?<\/style>/gi, "")
    .replace(/<[^>]+>/g, " ")
    .replace(/\s+/g, " ")
    .trim();
}

// buildPrevNext returns {prev,next} for a page inside the ordered page list.
export function buildPrevNext(pages, slug, section) {
  const i = pages.findIndex((p) => p.slug === slug && p.section === section);
  if (i < 0) return { prev: null, next: null };
  return {
    prev: i > 0 ? { slug: pages[i - 1].slug, section: pages[i - 1].section, title: pages[i - 1].title } : null,
    next: i < pages.length - 1 ? { slug: pages[i + 1].slug, section: pages[i + 1].section, title: pages[i + 1].title } : null,
  };
}
