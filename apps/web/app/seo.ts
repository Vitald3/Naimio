import type { Metadata } from "next";

const localAddress = (
  process.env.local_ip_address ?? process.env.LOCAL_IP_ADDRESS
)?.trim();
export const siteURL = new URL(
  process.env.NEXT_PUBLIC_SITE_URL?.trim() ||
    (process.env.NODE_ENV === "production"
      ? "https://naimio.ru"
      : localAddress && localAddress !== "0.0.0.0"
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
  options?: {
    ogImage?: string;
    noIndex?: boolean;
    type?: "website" | "article" | "profile";
  },
): Metadata => {
  const images = options?.ogImage ? [{ url: options.ogImage }] : undefined;
  return {
    title,
    description,
    alternates: { canonical: path },
    robots: options?.noIndex ? { index: false, follow: false } : { index: true, follow: true },
    openGraph: {
      type: options?.type || "website",
      locale: "ru_RU",
      url: path,
      siteName: "Naimio",
      title,
      description,
      images,
    },
    twitter: {
      card: options?.ogImage ? "summary_large_image" : "summary",
      title,
      description,
      images: options?.ogImage ? [options.ogImage] : undefined,
    },
  };
};
export const missingMetadata = (title: string): Metadata => ({
  title,
  robots: { index: false, follow: false },
});
export const jsonLD = (value: unknown) =>
  JSON.stringify(value).replace(/</g, "\\u003c");
