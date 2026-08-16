import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';

const read=(p)=>fs.readFileSync(new URL(`../${p}`, import.meta.url),'utf8');

test('header search query is consumed by freelancer catalog',()=>{
  const header=read('app/site-header.tsx');
  const freelancers=read('app/freelancers/page.tsx');
  assert.match(header,/action="\/freelancers"/);
  assert.match(header,/name="q"/);
  assert.match(freelancers,/window\.location\.search/);
  assert.match(freelancers,/get\("q"\)/);
});

test('popular category tiles have semantic icons',()=>{
  const home=read('app/page.tsx');
  const icons=read('app/icons.tsx');
  assert.match(home,/function CategoryIcon/);
  assert.match(home,/category-card__icon/);
  assert.match(icons,/IconCode/);
  assert.match(icons,/IconPalette/);
  assert.match(icons,/IconMegaphone/);
});

test('vacancy catalog has polished list and grid modes without double card borders',()=>{
  const page=read('app/vacancies/page.tsx');
  const css=read('app/globals.css');
  assert.match(page,/vacancy-results--\$\{view\}/);
  assert.match(page,/vacancy-card--\$\{view\}/);
  assert.match(page,/IconList/);
  assert.match(page,/IconGrid/);
  assert.match(page,/vacancy-card__aside/);
  assert.match(css,/\.vacancy-results\s*>\s*li\s*\{[^}]*border:\s*0\s*!important/);
  assert.match(css,/\.vacancy-results--grid\s*\{/);
  assert.match(css,/\.vacancy-card--grid\s*\{/);
});

test('staff accounts are redirected away from marketplace private areas',()=>{
  const guard=read('app/staff-marketplace-guard.tsx');
  const layout=read('app/layout.tsx');
  assert.match(guard,/"\/favorites"/);
  assert.match(guard,/"\/messages"/);
  assert.match(guard,/"\/settings"/);
  assert.match(guard,/location\.replace\(STAFF_BASE_PATH\)/);
  assert.match(layout,/StaffMarketplaceGuard/);
});
