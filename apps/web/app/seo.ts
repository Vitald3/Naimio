import type { Metadata } from "next";

const localAddress = (
  process.env.local_ip_address ?? process.env.LOCAL_IP_ADDRESS
)?.trim();
export const siteURL = new URL(
  process.env.NEXT_PUBLIC_SITE_URL ??
    (localAddress && localAddress !== "0.0.0.0"
      ? `http://${localAddress}:8088`
      : "http://localhost:8088"),
);
export const canonical = (path: string) => new URL(path, siteURL).toString();
export const summary = (
  value: string | undefined,
  fallback: string,
  max = 160,
) => {
  const text = (value ?? "").replace(/\s+/g, " ").trim() || fallback;
  return text.length <= max ? text : `${text.slice(0, max - 1).trim()}…`;
};
export const publicMetadata = (
  title: string,
  description: string,
  path: string,
): Metadata => ({
  title,
  description,
  alternates: { canonical: path },
  robots: { index: true, follow: true },
  openGraph: {
    type: "website",
    locale: "ru_RU",
    url: path,
    siteName: "Naimio",
    title,
    description,
  },
});
export const missingMetadata = (title: string): Metadata => ({
  title,
  robots: { index: false, follow: false },
});
export const jsonLD = (value: unknown) =>
  JSON.stringify(value).replace(/</g, "\\u003c");
