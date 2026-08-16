"use client";

import {
  createContext,
  ReactNode,
  useContext,
  useEffect,
  useState,
} from "react";

export type SiteSettings = {
  project_name: string;
  project_description: string;
  support_email: string;
  support_phone: string;
  legal_company_name: string;
  footer_copyright: string;
  primary_button_color: string;
  button_hover_color: string;
  green_heading_color: string;
  bright_blue_color: string;
  heading_color: string;
  body_text_color: string;
  page_background_color: string;
  catalog_page_size: number;
  marketplace_digest_enabled: boolean;
  marketplace_digest_threshold: number;
  marketplace_digest_interval_minutes: number;
  pro_subscriptions_enabled: boolean;
  blog_enabled: boolean;
  privacy_policy_slug: string;
  terms_slug: string;
};

export const defaultSiteSettings: SiteSettings = {
  project_name: "Naimio",
  project_description: "Маркетплейс профессиональных услуг",
  support_email: "",
  support_phone: "",
  legal_company_name: "",
  footer_copyright: "© Naimio",
  primary_button_color: "#15956a",
  button_hover_color: "#0d7452",
  green_heading_color: "#0d7452",
  bright_blue_color: "#2563a7",
  heading_color: "#0d1f16",
  body_text_color: "#13261d",
  page_background_color: "#ffffff",
  catalog_page_size: 50,
  marketplace_digest_enabled: true,
  marketplace_digest_threshold: 10,
  marketplace_digest_interval_minutes: 60,
  pro_subscriptions_enabled: false,
  blog_enabled: false,
  privacy_policy_slug: "",
  terms_slug: "",
};

const Context = createContext(defaultSiteSettings);
const safeColor = (value: string, fallback: string) =>
  /^#[0-9a-f]{6}$/i.test(value) ? value : fallback;

export function SiteSettingsProvider({ children }: { children: ReactNode }) {
  const [settings, setSettings] = useState(defaultSiteSettings);
  useEffect(() => {
    fetch("/api/v1/site-settings", { cache: "no-store" })
      .then((response) => (response.ok ? response.json() : null))
      .then((body) => {
        if (!body?.data) return;
        const next = { ...defaultSiteSettings, ...body.data } as SiteSettings;
        setSettings(next);
        const root = document.documentElement.style;
        root.setProperty(
          "--brand",
          safeColor(
            next.primary_button_color,
            defaultSiteSettings.primary_button_color,
          ),
        );
        root.setProperty(
          "--brand-dark",
          safeColor(
            next.button_hover_color,
            defaultSiteSettings.button_hover_color,
          ),
        );
        root.setProperty(
          "--green-heading-color",
          safeColor(
            next.green_heading_color,
            defaultSiteSettings.green_heading_color,
          ),
        );
        root.setProperty(
          "--bright-blue",
          safeColor(
            next.bright_blue_color,
            defaultSiteSettings.bright_blue_color,
          ),
        );
        root.setProperty(
          "--ink-strong",
          safeColor(next.heading_color, defaultSiteSettings.heading_color),
        );
        root.setProperty(
          "--ink",
          safeColor(next.body_text_color, defaultSiteSettings.body_text_color),
        );
        root.setProperty(
          "--page-background",
          safeColor(
            next.page_background_color,
            defaultSiteSettings.page_background_color,
          ),
        );
      })
      .catch(() => undefined);
  }, []);
  return <Context.Provider value={settings}>{children}</Context.Provider>;
}

export const useSiteSettings = () => useContext(Context);
