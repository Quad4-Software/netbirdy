"use client";

import "../app/globals.css";
import CommandPalette from "@components/CommandPalette";
import { DisableDarkReader } from "@components/DisableDarkReader";
import { TooltipProvider } from "@components/Tooltip";
import { cn } from "@utils/helpers";
import dayjs from "dayjs";
import relativeTime from "dayjs/plugin/relativeTime";
import { Viewport } from "next";
import localFont from "next/font/local";
import React, { Suspense } from "react";
import { Toaster } from "sonner";
import OIDCProvider from "@/auth/OIDCProvider";
import FullScreenLoading from "@/components/ui/FullScreenLoading";
import DialogProvider from "@/contexts/DialogProvider";
import ErrorBoundaryProvider from "@/contexts/ErrorBoundary";
import InstanceSetupProvider from "@/contexts/InstanceSetupProvider";
import { NavigationEvents } from "@/contexts/NavigationEvents";
import { ThemeProvider } from "@/contexts/ThemeProvider";

const sans = localFont({
  src: [{ path: "../assets/fonts/instrument-sans-latin-wght-normal.woff2", weight: "100 900" }],
  variable: "--font-sans",
});
const mono = localFont({
  src: [{ path: "../assets/fonts/jetbrains-mono-latin-wght-normal.woff2", weight: "100 800" }],
  variable: "--font-mono",
});

// Extend dayjs with relativeTime plugin
dayjs.extend(relativeTime);

export const viewport: Viewport = {
  width: "device-width",
  initialScale: 1,
};

export default function AppLayout({
  children,
}: Readonly<{ children: React.ReactNode }>) {

  return (
    <html lang="en" className="dark" style={{ colorScheme: "dark" }}>
      <head>
      </head>
      <body className={cn(sans.className, sans.variable, mono.variable)}>
        <Suspense fallback={<FullScreenLoading />}>
            <DialogProvider>
              <ThemeProvider>
                <ErrorBoundaryProvider>
                  <InstanceSetupProvider>
                    <OIDCProvider>
                      <TooltipProvider delayDuration={0}>
                        {children}
                        <CommandPalette />
                      </TooltipProvider>
                    </OIDCProvider>
                  </InstanceSetupProvider>
                </ErrorBoundaryProvider>
              </ThemeProvider>
            </DialogProvider>
            <Toaster
              position="top-center"
              duration={3000}
              toastOptions={{ unstyled: true }}
              style={{ "--width": "28rem" } as React.CSSProperties}
              gap={0}
              visibleToasts={5}
              offset="12px"
            />
            <NavigationEvents />
            <DisableDarkReader />
        </Suspense>
      <noscript>NetBirdy requires JavaScript.</noscript></body>
    </html>
  );
}
