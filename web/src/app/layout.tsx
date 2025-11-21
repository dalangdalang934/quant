import type { Metadata } from "next";
import Script from "next/script";
import { Geist, Geist_Mono, IBM_Plex_Mono } from "next/font/google";
import "./globals.css";
import Header from "@/components/layout/Header";
import SWRProvider from "@/components/providers/SWRProvider";
import ThemeProvider from "@/components/theme/ThemeProvider";
import PageActivityProvider from "@/components/providers/PageActivityProvider";
import { Analytics } from "@vercel/analytics/next";

const geistSans = Geist({
  variable: "--font-geist-sans",
  subsets: ["latin"],
});

const geistMono = Geist_Mono({
  variable: "--font-geist-mono",
  subsets: ["latin"],
});

const plexMono = IBM_Plex_Mono({
  variable: "--font-plex-mono",
  subsets: ["latin"],
  weight: ["400", "500", "600", "700"],
});

export const metadata: Metadata = {
  title: "量化交易监控终端",
  description:
    "实时查看量化策略的持仓、盈亏曲线与历史订单，可用于监控 Binance/Hype 等实盘账户。",
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html lang="zh-CN" suppressHydrationWarning>
      {/* Move early theme script into <head> so Next.js can order it correctly */}
      <head>
        <Script id="theme-init" strategy="beforeInteractive">
          {`(function(){
  try {
    var pref = localStorage.getItem('theme') || 'system';
    var sys = (window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches) ? 'dark' : 'light';
    var resolved = pref === 'system' ? sys : pref;
    document.documentElement.dataset.theme = resolved;
  } catch (_) {}
})();`}
        </Script>
      </head>
      <body
        className={`${geistSans.variable} ${geistMono.variable} ${plexMono.variable} antialiased`}
      >
        <ThemeProvider />
        <PageActivityProvider />
        <SWRProvider>
          <div className="min-h-screen">
            <Header />
            {children}
          </div>
        </SWRProvider>
        <Analytics />
      </body>
    </html>
  );
}
