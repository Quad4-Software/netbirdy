// Command palette item building and shortcut matching.
// Nav items are read from the rendered sidebar (data-nav-item attributes set
// by SidebarItem) so the palette can never drift from the navigation config.

export interface PaletteItem {
  id: string;
  label: string;
  href?: string;
  kind: "page" | "action";
}

// readNavItems pulls (href, label) from the sidebar DOM. Returns [] outside
// the browser (tests inject items directly).
export function readNavItems(doc: Document): PaletteItem[] {
  const nodes = doc.querySelectorAll("[data-nav-item]");
  const items: PaletteItem[] = [];
  nodes.forEach((n) => {
    const href = n.getAttribute("data-nav-item");
    if (!href) return;
    // Skip group nodes: a leaf nav item never contains another nav item.
    if (n.querySelector("[data-nav-item]")) return;
    const label = (n.textContent || "").trim();
    if (!label) return;
    items.push({ id: href, label, href, kind: "page" });
  });
  return items;
}

const STATIC_ACTIONS: PaletteItem[] = [
  { id: "theme-light", label: "Theme: Light", kind: "action" },
  { id: "theme-dark", label: "Theme: Dark", kind: "action" },
  { id: "theme-system", label: "Theme: System", kind: "action" },
  { id: "copy-mgmt-url", label: "Copy management API URL", kind: "action" },
  { id: "docs", label: "Open documentation", kind: "action" },
  { id: "shortcuts", label: "Keyboard shortcuts", kind: "action" },
  { id: "about", label: "About NetBirdy", kind: "action" },
  { id: "source", label: "Open source repository", kind: "action" },
  { id: "sign-out", label: "Sign out", kind: "action" },
];

export function buildPaletteItems(doc?: Document): PaletteItem[] {
  const nav = doc ? readNavItems(doc) : [];
  return [...nav, ...STATIC_ACTIONS];
}

// Docs pages are appended lazily by the palette so the generated index does
// not have to load until the palette opens.
export function docsPaletteItems(): PaletteItem[] {
  try {
     
    const docs = require("@/generated/docs.json") as { pages: { slug: string; section: string; title: string }[] };
    return docs.pages.map((p) => ({
      id: `/docs/${p.section}/${p.slug}`,
      label: `${p.title} — Docs`,
      kind: "page",
    }));
  } catch {
    return [];
  }
}

export function filterPaletteItems(items: PaletteItem[], query: string): PaletteItem[] {
  const q = query.trim().toLowerCase();
  if (!q) return items;
  return items.filter((i) => i.label.toLowerCase().includes(q));
}

// isEditableTarget: the "/" shortcut must not fire while typing.
export function isEditableTarget(el: EventTarget | null): boolean {
  if (!(el instanceof HTMLElement)) return false;
  if (el.isContentEditable) return true;
  const tag = el.tagName;
  return tag === "INPUT" || tag === "TEXTAREA" || tag === "SELECT";
}

// matchShortcut: Ctrl/Cmd+Shift+K always, "/" only outside editable fields.
export function matchPaletteShortcut(e: {
  key: string;
  ctrlKey: boolean;
  metaKey: boolean;
  shiftKey: boolean;
  altKey?: boolean;
}, target: EventTarget | null): boolean {
  if ((e.ctrlKey || e.metaKey) && e.shiftKey && e.key.toLowerCase() === "k") {
    return true;
  }
  if (e.key === "/" && !e.ctrlKey && !e.metaKey && !e.altKey) {
    return !isEditableTarget(target);
  }
  return false;
}
