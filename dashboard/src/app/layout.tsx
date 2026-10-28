import type { Metadata, Viewport } from "next";
import AppLayout from "@/layouts/AppLayout";

const siteUrl = process.env.NETBIRDY_PUBLIC_URL;

export const metadata: Metadata = {
  metadataBase: siteUrl ? new URL(siteUrl) : undefined,
  title: { default: "NetBirdy Dashboard", template: "%s | NetBirdy" },
  description:
    "NetBirdy is a self-hosted console for managing a private peer-to-peer network.",
  applicationName: "NetBirdy",
  generator: "NetBirdy",
  authors: [{ name: "Quad4-Software", url: "https://github.com/Quad4-Software/netbirdy" }],
  creator: "Quad4-Software",
  referrer: "same-origin",
  formatDetection: { telephone: false, email: false, address: false },
  robots: { index: false, follow: false, nocache: true },
  manifest: "/manifest.webmanifest",
  icons: {
    icon: [
      { url: "/icon.svg", type: "image/svg+xml" },
      { url: "/favicon.ico", sizes: "16x16 32x32 48x48" },
    ],
    apple: [{ url: "/apple-icon.png", sizes: "180x180" }],
  },
  openGraph: {
    type: "website",
    siteName: "NetBirdy",
    title: "NetBirdy Dashboard",
    description:
      "NetBirdy is a self-hosted console for managing a private peer-to-peer network.",
    locale: "en_US",
    images: [
      {
        url: "/og-image.png",
        width: 1200,
        height: 630,
        alt: "NetBirdy",
      },
    ],
  },
  twitter: {
    card: "summary_large_image",
    title: "NetBirdy Dashboard",
    images: ["/twitter-image.png"],
  },
};

export const viewport: Viewport = {
  themeColor: [
    { media: "(prefers-color-scheme: dark)", color: "#000000" },
    { media: "(prefers-color-scheme: light)", color: "#f6f2ea" },
  ],
  colorScheme: "dark light",
};

export default AppLayout;
