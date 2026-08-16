import assert from "node:assert/strict";
import {readFileSync} from "node:fs";
import test from "node:test";
const read=(path)=>readFileSync(new URL(`../app/${path}`,import.meta.url),"utf8");
test("customer and freelancer Safe Deal UI obey funding and provider truth",()=>{const detail=read("dashboard/safe-deals/[id]/page.tsx");for(const action of ["fund","start","submit","revision","accept","disputes","cancel"])assert.match(detail,new RegExp(`\\"${action}\\"`));assert.match(detail,/provider_capability_notice/);assert.doesNotMatch(detail,/деньги защищены|гарантированн/i)});
test("admin resolves disputes through audited actions without direct status editing",()=>{const admin=read("x7m4q9k2/safe-deals/page.tsx");assert.match(admin,/resolve/);assert.match(admin,/reconcile/);assert.doesNotMatch(admin,/method:\s*"PATCH"/)});
test("project chat displays authoritative Safe Deal context",()=>{const chat=read("messages/page.tsx");assert.match(chat,/safe-deals\?project_id/);assert.match(chat,/gross_amount_kopecks/)});
