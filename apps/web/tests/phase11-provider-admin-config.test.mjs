import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

const read = (path) => readFileSync(new URL(`../app/${path}`, import.meta.url), "utf8");

test("payment provider admin config supports credentials and explicit sandbox/production mode", () => {
  const page = read("x7m4q9k2/payment-routing/page.tsx");
  assert.match(page, /providers\/\$\{provider\}\/config/);
  assert.match(page, /Sandbox — тестовые платежи/);
  assert.match(page, /Production — реальные платежи/);
  assert.match(page, /Секретные поля после сохранения очищаются/);
  assert.match(page, /Настроить/);
  assert.match(page, /Изменить настройки/);
  assert.match(page, /Сохранить конфигурацию/);
});
