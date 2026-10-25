"use client";

import Link from "next/link";
import { useEffect, useMemo, useState } from "react";
import { type DocsSearchPage,searchDocs } from "./search";

interface Page extends DocsSearchPage {
  description: string;
  html: string;
  order: number;
  file: string;
}

interface Props {
  page: Page;
  pages: Page[];
  prev: { slug: string; section: string; title: string } | null;
  next: { slug: string; section: string; title: string } | null;
}

const GH_BASE =
  "https://github.com/Sudo-Ivan/netbirdy/blob/master/dashboard/content/docs/";

export function DocsArticle({ page, pages, prev, next }: Props) {
  const [query, setQuery] = useState("");
  const [active, setActive] = useState<string>("");

  const results = useMemo(() => searchDocs(pages, query), [pages, query]);
  const sections = useMemo(() => {
    const m = new Map<string, Page[]>();
    for (const p of pages) {
      const list = m.get(p.section) || [];
      list.push(p);
      m.set(p.section, list);
    }
    return [...m.entries()];
  }, [pages]);

  // Scroll spy for the right-hand table of contents.
  useEffect(() => {
    if (!page.headings.length) return;
    const obs = new IntersectionObserver(
      (entries) => {
        for (const e of entries) {
          if (e.isIntersecting) setActive(e.target.id);
        }
      },
      { rootMargin: "-10% 0px -70% 0px" },
    );
    for (const h of page.headings) {
      const el = document.getElementById(h.id);
      if (el) obs.observe(el);
    }
    return () => obs.disconnect();
  }, [page]);

  // Copy buttons on code blocks.
  useEffect(() => {
    const root = document.getElementById("nb-doc-article");
    if (!root) return;
    const blocks = root.querySelectorAll<HTMLElement>(".nb-codeblock");
    const cleanups: (() => void)[] = [];
    blocks.forEach((b) => {
      const btn = document.createElement("button");
      btn.className = "nb-codeblock-copy";
      btn.type = "button";
      btn.textContent = "Copy";
      const onClick = () => {
        const code = b.querySelector("code")?.textContent || "";
        void navigator.clipboard?.writeText(code);
        btn.textContent = "Copied";
        setTimeout(() => (btn.textContent = "Copy"), 1500);
      };
      btn.addEventListener("click", onClick);
      b.appendChild(btn);
      cleanups.push(() => btn.removeEventListener("click", onClick));
    });
    return () => cleanups.forEach((c) => c());
  }, [page]);

  return (
    <div className="mx-auto flex w-full max-w-6xl gap-10 px-6 py-10">
      {/* Left: search + section navigation */}
      <aside className="hidden w-56 shrink-0 md:block">
        <input
          type="search"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          placeholder="Search docs..."
          className="w-full rounded-md border border-nb-gray-800 bg-transparent px-3 py-1.5 text-sm outline-none focus:border-netbirdy"
        />
        {query ? (
          <ul className="mt-3 space-y-1">
            {results.length === 0 && (
              <li className="px-2 text-xs text-nb-gray-500">No results</li>
            )}
            {results.map((r) => (
              <li key={`${r.section}/${r.slug}`}>
                <Link
                  href={`/docs/${r.section}/${r.slug}`}
                  onClick={() => setQuery("")}
                  className="block rounded-md px-2 py-1.5 text-sm hover:bg-nb-gray-900"
                >
                  <span className="block truncate font-medium">{r.title}</span>
                  <span className="block truncate text-xs text-nb-gray-500">
                    {r.snippet}
                  </span>
                </Link>
              </li>
            ))}
          </ul>
        ) : (
          <nav className="mt-4 space-y-4">
            {sections.map(([section, list]) => (
              <details key={section} open={section === page.section}>
                <summary className="cursor-pointer px-2 text-xs font-medium uppercase tracking-wide text-nb-gray-500">
                  {section.replace(/-/g, " ")} ({list.length})
                </summary>
                <ul className="mt-1">
                  {list.map((p) => {
                    const isActive = p.slug === page.slug && p.section === page.section;
                    return (
                      <li key={p.slug}>
                        <Link
                          href={`/docs/${p.section}/${p.slug}`}
                          className={`block rounded-md px-2 py-1 text-sm ${
                            isActive
                              ? "bg-netbirdy/10 text-netbirdy"
                              : "text-nb-gray-300 hover:bg-nb-gray-900"
                          }`}
                        >
                          {p.title}
                        </Link>
                      </li>
                    );
                  })}
                </ul>
              </details>
            ))}
          </nav>
        )}
      </aside>

      {/* Center: article */}
      <article className="min-w-0 flex-1">
        <p className="text-xs uppercase tracking-wide text-nb-gray-500">
          {page.section.replace(/-/g, " ")}
        </p>
        <h1 className="mt-1 text-3xl font-medium">{page.title}</h1>
        {page.description && (
          <p className="mt-2 text-sm text-nb-gray-400">{page.description}</p>
        )}
        <div
          id="nb-doc-article"
          className="nb-prose mt-8"
          dangerouslySetInnerHTML={{ __html: page.html }}
        />

        <div className="mt-10 flex items-center justify-between border-t border-nb-gray-900 pt-5 text-sm">
          {prev ? (
            <Link href={`/docs/${prev.section}/${prev.slug}`} className="text-netbirdy hover:underline">
              &larr; {prev.title}
            </Link>
          ) : (
            <span />
          )}
          {next ? (
            <Link href={`/docs/${next.section}/${next.slug}`} className="text-netbirdy hover:underline">
              {next.title} &rarr;
            </Link>
          ) : (
            <span />
          )}
        </div>
        <p className="mt-4 text-xs">
          <a
            href={`${GH_BASE}${page.file}`}
            target="_blank"
            rel="noopener noreferrer"
            className="text-nb-gray-500 hover:text-netbirdy"
          >
            Edit this page on GitHub
          </a>
        </p>
      </article>

      {/* Right: on this page */}
      {page.headings.length > 0 && (
        <aside className="hidden w-44 shrink-0 xl:block">
          <p className="text-xs font-medium uppercase tracking-wide text-nb-gray-500">
            On this page
          </p>
          <ul className="mt-3 space-y-1.5">
            {page.headings.map((h) => (
              <li key={h.id} style={{ paddingLeft: h.level === 3 ? 12 : 0 }}>
                <a
                  href={`#${h.id}`}
                  className={`block text-xs ${
                    active === h.id ? "text-netbirdy" : "text-nb-gray-400 hover:text-nb-gray-200"
                  }`}
                >
                  {h.text}
                </a>
              </li>
            ))}
          </ul>
        </aside>
      )}
    </div>
  );
}
