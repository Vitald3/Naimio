import test from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
const read=(p)=>readFile(new URL(`../app/${p}`,import.meta.url),"utf8");

test("staff uses a separate opaque control-center route",async()=>{
  const path=await read("admin-path.ts");
  const guard=await read("x7m4q9k2/admin-guard.tsx");
  assert.match(path,/STAFF_BASE_PATH = "\/x7m4q9k2"/);
  assert.match(guard,/STAFF_LOGIN_PATH/);
  await assert.rejects(()=>read("admin/page.tsx"));
});

test("staff accounts are excluded from marketplace account chrome",async()=>{
  const header=await read("site-header.tsx");
  const mobile=await read("mobile-nav.tsx");
  const dashboard=await read("dashboard/staff-dashboard-guard.tsx");
  assert.match(header,/isStaffRoles/);
  assert.match(header,/Control Center/);
  assert.match(mobile,/staff\)return null/);
  assert.match(dashboard,/location\.replace\(STAFF_BASE_PATH\)/);
});

test("marketplace and staff login portals are explicit",async()=>{
  const regular=await read("login/page.tsx");
  const staff=await read("x7m4q9k2/login/page.tsx");
  assert.match(regular,/portal:"marketplace"/);
  assert.match(staff,/portal: "admin"/);
});

test("control center auth is independent from marketplace auth state",async()=>{
  const auth=await read("x7m4q9k2/admin-auth.tsx");
  const nav=await read("x7m4q9k2/admin-nav.tsx");
  const guard=await read("x7m4q9k2/admin-guard.tsx");
  assert.match(auth,/\/api\/v1\/auth\/admin-session/);
  assert.match(auth,/\/api\/v1\/auth\/admin-logout/);
  assert.doesNotMatch(nav,/useAuth/);
  assert.doesNotMatch(guard,/useAuth/);
  assert.match(nav,/useAdminAuth/);
  assert.match(guard,/useAdminAuth/);
});
