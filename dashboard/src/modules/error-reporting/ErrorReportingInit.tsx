"use client";

import { useEffect } from "react";
import { useAccount } from "@/modules/account/useAccount";
import {
  resolveErrorReportingConfig,
  syncErrorReporting,
} from "./errorReporting";

// ErrorReportingInit applies the account's error reporting config once the
// account resolves and on every later refetch. It renders nothing.
export default function ErrorReportingInit() {
  const account = useAccount();

  useEffect(() => {
    if (!account) return;
    void syncErrorReporting(resolveErrorReportingConfig(account));
  }, [account]);

  return null;
}
