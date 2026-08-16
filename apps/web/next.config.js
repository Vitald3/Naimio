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

const nextConfig = {
  reactStrictMode: true,
  async rewrites() {
    if (!proxyTarget) return [];
    return [
      { source: "/api/:path*", destination: `${proxyTarget}/api/:path*` },
      { source: "/health/:path*", destination: `${proxyTarget}/health/:path*` },
    ];
  },
};

module.exports = nextConfig;
