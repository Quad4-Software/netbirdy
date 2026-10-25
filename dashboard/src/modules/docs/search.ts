// Client-side docs search over the generated plain-text index. No deps.

export interface DocsSearchPage {
  slug: string;
  section: string;
  title: string;
  description: string;
  headings: { id: string; text: string; level: number }[];
  text: string;
}

export interface DocsSearchResult {
  slug: string;
  section: string;
  title: string;
  snippet: string;
  score: number;
}

function snippetFor(text: string, terms: string[]): string {
  const lower = text.toLowerCase();
  let idx = -1;
  for (const t of terms) {
    const i = lower.indexOf(t);
    if (i >= 0 && (idx < 0 || i < idx)) idx = i;
  }
  if (idx < 0) return text.slice(0, 140);
  const start = Math.max(0, idx - 40);
  return (start > 0 ? "…" : "") + text.slice(start, start + 160) + "…";
}

export function searchDocs(pages: DocsSearchPage[], query: string, limit = 20): DocsSearchResult[] {
  const terms = query.toLowerCase().split(/\s+/).filter(Boolean);
  if (!terms.length) return [];
  const out: DocsSearchResult[] = [];
  for (const p of pages) {
    let score = 0;
    const title = p.title.toLowerCase();
    const headings = p.headings.map((h) => h.text.toLowerCase()).join(" ");
    const body = p.text.toLowerCase();
    for (const t of terms) {
      if (title.includes(t)) score += 10;
      if (title.startsWith(t)) score += 4;
      if (headings.includes(t)) score += 4;
      if (body.includes(t)) score += 1;
      if (p.description.toLowerCase().includes(t)) score += 3;
    }
    if (score > 0) {
      out.push({ slug: p.slug, section: p.section, title: p.title, snippet: snippetFor(p.text, terms), score });
    }
  }
  return out.sort((a, b) => b.score - a.score).slice(0, limit);
}
