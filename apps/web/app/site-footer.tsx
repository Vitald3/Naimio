"use client";
import { usePathname } from "next/navigation";
import { STAFF_BASE_PATH } from "./admin-path";
import { useSiteSettings } from "./site-settings";

export default function SiteFooter() {
  const pathname = usePathname() || "";
  const settings = useSiteSettings();
  if (pathname === STAFF_BASE_PATH || pathname.startsWith(STAFF_BASE_PATH + "/")) return null;
  return <footer className="site-footer"><span>{settings.footer_copyright}</span><nav aria-label="Правовая информация">{settings.privacy_policy_slug ? <a href={`/blog/${settings.privacy_policy_slug}`}>Политика конфиденциальности</a> : null}{settings.terms_slug ? <a href={`/blog/${settings.terms_slug}`}>Условия соглашения</a> : null}</nav></footer>;
}
