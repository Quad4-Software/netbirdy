import type { Metadata } from "next";
import Link from "next/link";
import { notFound } from "next/navigation";
import docs from "@/generated/docs.json";
import { DocsArticle } from "@/modules/docs/DocsArticle";

interface Params {
  slug: string[];
}

export function generateStaticParams() {
  return docs.pages.map((p) => ({ slug: [p.section, p.slug] }));
}

export async function generateMetadata({ params }: { params: Promise<Params> }): Promise<Metadata> {
  const { slug } = await params;
  const page = docs.pages.find((p) => p.section === slug[0] && p.slug === slug[1]);
  return { title: page ? `${page.title} | Docs` : "Docs" };
}

export default async function DocsPage({ params }: { params: Promise<Params> }) {
  const { slug } = await params;
  const page = docs.pages.find((p) => p.section === slug[0] && p.slug === slug[1]);
  if (!page) notFound();

  const idx = docs.pages.findIndex((p) => p.section === page.section && p.slug === page.slug);
  const prev = idx > 0 ? docs.pages[idx - 1] : null;
  const next = idx < docs.pages.length - 1 ? docs.pages[idx + 1] : null;

  return <DocsArticle page={page} pages={docs.pages} prev={prev} next={next} />;
}
