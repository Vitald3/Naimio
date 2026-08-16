import UIFeedback from "./ui-feedback";
import type { ReactNode } from "react";
import type { Metadata } from "next";
import AcquisitionTracker from "./acquisition-tracker";
import { siteURL } from "./seo";
import "./globals.css";
import { AuthProvider } from "./auth-state";
import SiteHeader from "./site-header";
import MobileNav from "./mobile-nav";
import StaffMarketplaceGuard from "./staff-marketplace-guard";
import { ToastProvider } from "./toast";
import { SiteSettingsProvider } from "./site-settings";
import SiteFooter from "./site-footer";

export const metadata: Metadata = {
  metadataBase: siteURL,
  title: {
    default: "Naimio — специалисты, услуги и проекты",
    template: "%s | Naimio",
  },
  description:
    "Найдите специалиста, услугу, проект или вакансию и получите ориентир стоимости задачи.",
  robots: { index: true, follow: true },
  openGraph: {
    type: "website",
    locale: "ru_RU",
    siteName: "Naimio",
    title: "Naimio",
    description: "Специалисты, услуги, проекты и вакансии.",
  },
  icons: {
    icon: [
      { url: "/favicon.ico", sizes: "any" },
      { url: "/icon.svg", type: "image/svg+xml" },
    ],
    shortcut: "/favicon.ico",
    apple: [{ url: "/icon.svg", type: "image/svg+xml" }],
  },
};

export default function RootLayout({ children }: { children: ReactNode }) {
  return (
    <html lang="ru">
      <body>
        <SiteSettingsProvider>
          <AuthProvider>
            <ToastProvider>
              <SiteHeader />
              <AcquisitionTracker />
              <StaffMarketplaceGuard>{children}</StaffMarketplaceGuard>
              <SiteFooter />
              <MobileNav />
            </ToastProvider>
          </AuthProvider>
        </SiteSettingsProvider>
        <UIFeedback />
      </body>
    </html>
  );
}
