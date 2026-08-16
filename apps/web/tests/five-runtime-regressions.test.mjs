import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';

const read = (p) => fs.readFileSync(new URL(`../${p}`, import.meta.url), 'utf8');

test('provider enable is hidden until configured', () => {
  const src = read('app/x7m4q9k2/payment-routing/page.tsx');
  assert.match(src, /configured \? <button[^>]*>[\s\S]*?Включить/);
});

test('completed projects expose no proposals or matching actions', () => {
  const detail = read('app/dashboard/projects/[id]/page.tsx');
  const dashboard = read('app/dashboard/page.tsx');
  assert.match(detail, /\["OPEN","MATCHING","IN_PROGRESS"\]\.includes\(item\.status\)/);
  assert.match(dashboard, /\["OPEN","MATCHING","IN_PROGRESS"\]\.includes\(project\.status\|\|""\)/);
});

test('calendar period selectors have transparent hover', () => {
  const css = read('app/globals.css');
  assert.match(css, /date-picker-v2__period \.custom-select__trigger:hover\{[^}]*background:transparent/);
});
