"use client";

import { NetBirdLogo } from "@/components/NetBirdLogo";
import { Dialog, DialogContent, DialogTitle } from "@components/Dialog";
import Link from "next/link";

interface Props {
  open: boolean;
  onOpenChange: (v: boolean) => void;
  dashboardVersion?: string;
  managementVersion?: string;
}

export function AboutDialog({ open, onOpenChange, dashboardVersion, managementVersion }: Props) {
  const dash = !dashboardVersion || dashboardVersion === "development" ? "dev build" : dashboardVersion;
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-md" aria-label="About NetBirdy">
        <DialogTitle>About NetBirdy</DialogTitle>
        <div className="mt-4"><NetBirdLogo size="large" /></div>
        <dl className="mt-4 space-y-1 text-sm">
          <div className="flex justify-between"><dt className="text-nb-gray-400">Dashboard</dt><dd>{dash}</dd></div>
          {managementVersion && <div className="flex justify-between"><dt className="text-nb-gray-400">Management</dt><dd>{managementVersion}</dd></div>}
        </dl>
        <ul className="mt-4 space-y-1.5 text-sm">
          <li><a className="text-netbirdy hover:underline" target="_blank" rel="noopener noreferrer" href="https://github.com/Quad4-Software/netbirdy">Source code (AGPLv3 offer)</a></li>
          <li><Link className="text-netbirdy hover:underline" href="/docs">Documentation</Link></li>
          <li><Link className="text-netbirdy hover:underline" href="/docs/project/licenses-and-credits">Licenses and credits</Link></li>
          <li><a className="text-netbirdy hover:underline" target="_blank" rel="noopener noreferrer" href="https://github.com/Quad4-Software/netbirdy/issues">Report an issue</a></li>
          <li><a className="text-netbirdy hover:underline" target="_blank" rel="noopener noreferrer" href="https://github.com/Quad4-Software/netbirdy/security/advisories">Security advisories</a></li>
        </ul>
        <p className="mt-4 text-xs text-nb-gray-500">
          Dashboard, management, signal, relay and the combined server are AGPL-3.0;
          other components are BSD-3-Clause. NetBirdy is not affiliated with NetBird GmbH.
        </p>
      </DialogContent>
    </Dialog>
  );
}
