import type { Metadata } from "next";
import Link from "next/link";
import docs from "@/generated/docs.json";

export const metadata: Metadata = {
  title: "Documentation",
};

export default function DocsIndex() {
  return (
    <div className="mx-auto w-full max-w-4xl px-6 py-10">
      <h1 className="text-2xl font-medium">Documentation</h1>
      <p className="mt-2 text-sm text-nb-gray-400">
        Guides and references for running NetBirdy.
      </p>
      <div className="mt-8 grid gap-4 sm:grid-cols-2">
        {docs.sections.map((section) => {
          const pages = docs.pages.filter((p) => p.section === section);
          return (
            <div
              key={section}
              className="rounded-md border border-nb-gray-930 p-5"
            >
              <h2 className="text-sm font-medium capitalize">
                {section.replace(/-/g, " ")} ({pages.length})
              </h2>
              <ul className="mt-3 space-y-2">
                {pages.map((p) => (
                  <li key={p.slug}>
                    <Link
                      href={`/docs/${p.section}/${p.slug}`}
                      className="text-sm text-netbirdy hover:underline"
                    >
                      {p.title}
                    </Link>
                    {p.description && (
                      <p className="mt-0.5 text-xs text-nb-gray-400">
                        {p.description}
                      </p>
                    )}
                  </li>
                ))}
              </ul>
            </div>
          );
        })}
      </div>
    </div>
  );
}
