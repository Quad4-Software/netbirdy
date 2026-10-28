"use client";

import { Command } from "cmdk";
import { useTheme } from "@/contexts/ThemeProvider";
import { useLoggedInUser } from "@/contexts/UsersProvider";
import { useRouter } from "next/navigation";
import { useCallback, useEffect, useMemo, useState } from "react";
import { buildPaletteItems, docsPaletteItems, filterPaletteItems, matchPaletteShortcut } from "@utils/palette";
import { Dialog, DialogContent, DialogPortal, DialogTitle } from "@components/Dialog";
import { Search } from "lucide-react";

export default function CommandPalette() {
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const { setTheme } = useTheme();
  const { logout } = useLoggedInUser();
  const router = useRouter();

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (matchPaletteShortcut(e, e.target)) {
        e.preventDefault();
        setOpen((o) => !o);
      }
      if (e.key === "Escape") setOpen(false);
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, []);

  const run = useCallback((id: string) => {
    setOpen(false);
    if (id.startsWith("theme-")) return setTheme(id.replace("theme-", "") as never);
    if (id === "copy-mgmt-url") {
      const url = (window as unknown as { __NB_CONFIG__?: { managementURL?: string } }).__NB_CONFIG__?.managementURL || window.location.origin;
      void navigator.clipboard?.writeText(url);
      return;
    }
    if (id === "docs") { router.push("/docs"); return; }
    if (id === "shortcuts") { window.dispatchEvent(new Event("nb:shortcuts")); return; }
    if (id === "about") { window.dispatchEvent(new Event("nb:about")); return; }
    if (id === "source") return void window.open("https://github.com/Quad4-Software/netbirdy", "_blank");
    if (id === "sign-out") return void logout();
    if (id.startsWith("/")) router.push(id);
  }, [setTheme, logout, router]);

  const items = useMemo(
    () => filterPaletteItems([...buildPaletteItems(open ? document : undefined), ...(open ? docsPaletteItems() : [])], query),
    [open, query],
  );

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogPortal>
        <DialogContent
          aria-label="Command palette"
          className="fixed left-1/2 top-[12vh] z-50 w-[min(32rem,100vw-2rem)] -translate-x-1/2 translate-y-0 rounded-lg border border-neutral-200 bg-white p-0 shadow-xl dark:border-nb-gray-700 dark:bg-nb-gray-800 flex flex-col max-h-[70vh] overflow-hidden"
        >
          <Command label="Command palette" loop>
            <div className="nb-focus-wrap flex items-center gap-2 border-b border-neutral-200 px-3 dark:border-nb-gray-700">
              <Search size={16} className="text-neutral-400" />
              <Command.Input
                autoFocus
                value={query}
                onValueChange={setQuery}
                placeholder="Type a command or search..."
                data-no-focus-ring
                className="h-11 w-full bg-transparent text-sm outline-none"
              />
            </div>
            <Command.List className="flex-1 overflow-y-auto p-2">
              <Command.Empty className="py-6 text-center text-sm text-neutral-500">
                No results — press ? for shortcuts
              </Command.Empty>
              <Command.Group>
                {items.map((item) => (
                  <Command.Item
                    key={item.id}
                    value={item.label}
                    onSelect={() => run(item.id)}
                    className="cursor-pointer rounded-md px-3 py-2 text-sm aria-selected:bg-neutral-100 dark:aria-selected:bg-nb-gray-700"
                  >
                    {item.label}
                  </Command.Item>
                ))}
              </Command.Group>
            </Command.List>
          </Command>
        </DialogContent>
      </DialogPortal>
    </Dialog>
  );
}
