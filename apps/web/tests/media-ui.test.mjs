import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync, existsSync } from "node:fs";

const read=(p)=>readFileSync(new URL(`../${p}`,import.meta.url),"utf8");
test("built-in visual media is bundled",()=>{
  assert.ok(existsSync(new URL("../public/media/illustrations/hero-market.svg",import.meta.url)));
  assert.ok(existsSync(new URL("../public/media/avatars/avatar-01.svg",import.meta.url)));
  assert.ok(existsSync(new URL("../public/media/covers/cover-01.svg",import.meta.url)));
});
test("category page uses styled marketplace cards",()=>{
  const s=read("app/categories/[slug]/page.tsx");
  assert.match(s,/category-talent-card/); assert.match(s,/category-service-card/); assert.match(s,/category-project-card/);
  assert.doesNotMatch(s,/<li key=\{v\.username\}><a href=.*display_name/);
});
test("favorites renders resolved cards instead of UUID rows",()=>{
  const s=read("app/favorites/page.tsx");
  assert.match(s,/favorite-card/); assert.match(s,/item\.title/); assert.match(s,/remove\(item\)/);
  assert.doesNotMatch(s,/\{labels\[item\.entity_type.*\}\s*·\s*\{item\.entity_id\}/);
});
test("icons and media helpers are used",()=>{
  const icons=read("app/icons.tsx"); const css=read("app/globals.css");
  assert.match(icons,/IconWallet/); assert.match(icons,/IconShield/); assert.match(icons,/IconHeart/);
  assert.match(css,/media-avatar/); assert.match(css,/media-cover/); assert.match(css,/favorites-grid/);
});
