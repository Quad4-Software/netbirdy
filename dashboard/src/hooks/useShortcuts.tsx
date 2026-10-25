"use client";

import { useRouter } from "next/navigation";
import { useEffect, useRef, useState } from "react";
import useFetchApi from "@utils/api";
import { matchGoTo } from "@utils/shortcuts";
import { VersionInfo } from "@/interfaces/Instance";
import { AboutDialog } from "@/modules/help/AboutDialog";
import { ShortcutsSheet } from "@/modules/help/ShortcutsSheet";

// Registers `?` for the cheat sheet and `g <key>` go-to sequences once per app.
export function useShortcuts() {
  const router = useRouter();
  const [sheetOpen, setSheetOpen] = useState(false);
  const [aboutOpen, setAboutOpen] = useState(false);
  const { data: versionInfo } = useFetchApi<VersionInfo>("/instance/version", true, false);
  const seq = useRef({ last: "", at: 0 });

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const target = e.target;
      if (e.key === "?" && !e.ctrlKey && !e.metaKey) {
        const el = target as HTMLElement | null;
        if (el && (el.isContentEditable || ["INPUT", "TEXTAREA", "SELECT"].includes(el.tagName))) return;
        e.preventDefault();
        setSheetOpen((v) => !v);
        seq.current = { last: "", at: 0 };
        return;
      }
      const r = matchGoTo(seq.current, e, target, Date.now());
      if (r.consume === "prefix") {
        seq.current = { last: "g", at: Date.now() };
      } else {
        if (r.consume === "match" && r.path) {
          e.preventDefault();
          router.push(r.path);
        }
        seq.current = { last: "", at: 0 };
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [router]);

  useEffect(() => {
    const openAbout = () => setAboutOpen(true);
    const openSheet = () => setSheetOpen(true);
    window.addEventListener("nb:about", openAbout);
    window.addEventListener("nb:shortcuts", openSheet);
    return () => {
      window.removeEventListener("nb:about", openAbout);
      window.removeEventListener("nb:shortcuts", openSheet);
    };
  }, []);

  return (
    <>
      <ShortcutsSheet open={sheetOpen} onOpenChange={setSheetOpen} />
      <AboutDialog
        open={aboutOpen}
        onOpenChange={setAboutOpen}
        dashboardVersion={process.env.NEXT_PUBLIC_DASHBOARD_VERSION}
        managementVersion={versionInfo?.management_current_version}
      />
    </>
  );
}
