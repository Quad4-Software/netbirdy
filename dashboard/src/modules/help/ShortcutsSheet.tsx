"use client";

import { Dialog, DialogContent, DialogTitle } from "@components/Dialog";
import { SHEET_SHORTCUTS } from "@utils/shortcuts";

interface Props {
  open: boolean;
  onOpenChange: (v: boolean) => void;
}

export function ShortcutsSheet({ open, onOpenChange }: Props) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-md" aria-label="Keyboard shortcuts">
        <DialogTitle>Keyboard shortcuts</DialogTitle>
        <table className="mt-4 w-full text-sm">
          <tbody>
            {SHEET_SHORTCUTS.map((s) => (
              <tr key={s.keys} className="border-b border-nb-gray-900 last:border-0">
                <td className="py-1.5 pr-4 text-nb-gray-300">{s.label}</td>
                <td className="py-1.5 text-right">
                  <kbd className="rounded border border-nb-gray-700 px-1.5 py-0.5 font-mono text-xs">{s.keys}</kbd>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </DialogContent>
    </Dialog>
  );
}
