import test from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";

const read = (path) => readFile(new URL(`../${path}`, import.meta.url), "utf8");

test("protected route auth guards use minimal AuthBootstrapLoader instead of generic skeletons", async () => {
  const authGuard = await read("app/auth-guard.tsx");
  const staffDashboardGuard = await read("app/dashboard/staff-dashboard-guard.tsx");
  const dashboardRoleGuard = await read("app/dashboard/dashboard-role-guard.tsx");
  const adminGuard = await read("app/x7m4q9k2/admin-guard.tsx");

  assert.match(authGuard, /AuthBootstrapLoader/);
  assert.doesNotMatch(authGuard, /skeleton--card/);

  assert.match(staffDashboardGuard, /AuthBootstrapLoader/);
  assert.doesNotMatch(staffDashboardGuard, /skeleton--card/);

  assert.match(dashboardRoleGuard, /AuthBootstrapLoader/);
  assert.doesNotMatch(dashboardRoleGuard, /skeleton--card/);
  assert.doesNotMatch(dashboardRoleGuard, /skeleton--title/);

  assert.match(adminGuard, /AuthBootstrapLoader/);
  assert.doesNotMatch(adminGuard, /skeleton--card/);
  assert.doesNotMatch(adminGuard, /skeleton--title/);
});

test("auth loader has clean CSS animations and respects prefers-reduced-motion", async () => {
  const css = await read("app/globals.css");
  const loader = await read("app/auth-loader.tsx");

  assert.match(loader, /auth-bootstrap-screen/);
  assert.match(loader, /auth-bootstrap-spinner/);
  assert.match(css, /\.auth-bootstrap-screen\s*\{/);
  assert.match(css, /\.auth-bootstrap-spinner\s*\{/);
  assert.match(css, /border-top-color:\s*var\(--brand/);
  assert.match(css, /@keyframes auth-bootstrap-rotate/);
  assert.match(css, /@media\s*\(prefers-reduced-motion:\s*reduce\)/);
});
