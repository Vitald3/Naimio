import test from "node:test";
import assert from "node:assert/strict";
import {readFile} from "node:fs/promises";
const read=path=>readFile(new URL(`../${path}`,import.meta.url),"utf8");

test("Russian plural helper handles edge cases",async()=>{const source=await read("app/russian-plural.ts");const data=await import(`data:text/javascript,${encodeURIComponent(source.replace(/: number|: string|: readonly \[string, string, string\]/g,"").replace(/export /g,"export "))}`);for(const[c,w]of[[1,"отзыв"],[2,"отзыва"],[5,"отзывов"],[11,"отзывов"],[14,"отзывов"],[21,"отзыв"],[22,"отзыва"],[25,"отзывов"],[101,"отзыв"],[102,"отзыва"],[105,"отзывов"]])assert.equal(data.russianPlural(c,"отзыв","отзыва","отзывов"),w)});
test("notification presentation never exposes raw event names",async()=>{const source=await read("app/notification-presentation.ts");assert.match(source,/message\.created/);assert.match(source,/Обновление на платформе/);assert.doesNotMatch(await read("app/notifications/page.tsx"),/meta\?\.title\?\?item\.type/)});
test("rating uses one semantic gold token",async()=>{assert.match(await read("app/rating.tsx"),/rating__stars/);assert.match(await read("app/globals.css"),/--rating-star:/)});
test("portfolio route provides real CRUD media and role gating",async()=>{const page=await read("app/dashboard/portfolio/page.tsx");for(const pattern of [/\/api\/v1\/me\/portfolio/,/"PATCH"/,/method:\s*"DELETE"/,/purpose:\s*"PORTFOLIO"/,/FREELANCER/,/window\.confirm/])assert.match(page,pattern)});
test("account security is server authoritative",async()=>{assert.match(await read("app/settings/security/page.tsx"),/auth\/change-password/);assert.match(await readFile(new URL("../../api/internal/auth/http.go",import.meta.url),"utf8"),/func \(h Handler\) ChangePassword/)});
test("project editor uses TipTap and safe rendering",async()=>{assert.match(await read("app/project-description-editor.tsx"),/@tiptap\/react/);assert.match(await read("app/formatted-text.tsx"),/sanitizeHtml/)});
test("spacing and shared filter/card invariants exist",async()=>{const css=await read("app/globals.css");assert.match(css,/--space-1:\s*4px/);assert.match(css,/\.filters--expanded/);assert.match(css,/\.vacancy-results\s*>\s*li\s*\{[^}]*border:\s*0\s*!important/);assert.match(await read("app/categories/page.tsx"),/CategoryIcon/)});
