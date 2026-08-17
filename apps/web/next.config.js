/**
 * Next.js configuration.
 *
 * In production the reverse proxy (infra/nginx/nginx.conf) routes `/api/` to the
 * Go API and `/` to this Next server, so Next MUST NOT rewrite `/api/*` there.
 * For local development we proxy same-origin `/api/v1` and `/health` requests to
 * a backend so the browser's relative fetches resolve without CORS. The target is
 * `API_PROXY_TARGET` (e.g. the Go API on http://127.0.0.1:8080), defaulting to
 * 127.0.0.1:8080 in non-production. This block is completely skipped in
 * production unless API_PROXY_TARGET is explicitly set, preserving nginx behavior.
 *
 * @type {import('next').NextConfig}
 */
const isProd = process.env.NODE_ENV === "production";
const proxyTarget = process.env.API_PROXY_TARGET || (isProd ? undefined : "http://127.0.0.1:8080");

const siteUrlStr = process.env.NEXT_PUBLIC_SITE_URL || process.env.PUBLIC_BASE_URL || "https://naimio.ru";
let configuredHost = "naimio.ru";
let configuredProtocol = "https";
try {
  const parsed = new URL(siteUrlStr);
  configuredHost = parsed.hostname;
  configuredProtocol = parsed.protocol.replace(":", "");
} catch {}

const remotePatterns = [
  {
    protocol: "https",
    hostname: "naimio.ru",
    pathname: "/api/v1/media/**",
  },
  {
    protocol: "https",
    hostname: "naimio.ru",
    pathname: "/api/v1/blog/media/**",
  },
  {
    protocol: "http",
    hostname: "localhost",
    pathname: "/api/v1/media/**",
  },
  {
    protocol: "http",
    hostname: "127.0.0.1",
    pathname: "/api/v1/media/**",
  },
];

if (configuredHost && configuredHost !== "naimio.ru" && configuredHost !== "localhost" && configuredHost !== "127.0.0.1") {
  remotePatterns.push(
    {
      protocol: configuredProtocol,
      hostname: configuredHost,
      pathname: "/api/v1/media/**",
    },
    {
      protocol: configuredProtocol,
      hostname: configuredHost,
      pathname: "/api/v1/blog/media/**",
    }
  );
}

const nextConfig = {
  reactStrictMode: true,
  images: {
    remotePatterns,
  },
  async rewrites() {
    if (!proxyTarget) return [];
    return [
      { source: "/api/:path*", destination: `${proxyTarget}/api/:path*` },
      { source: "/health/:path*", destination: `${proxyTarget}/health/:path*` },
    ];
  },
};

module.exports = nextConfig;
