import test from "node:test";
import assert from "node:assert/strict";
import { readFile, stat } from "node:fs/promises";

const root = new URL("../", import.meta.url);

test("production image ships public assets and app favicon", async () => {
  const dockerfile = await readFile(new URL("Dockerfile", root), "utf8");
  const layout = await readFile(new URL("app/layout.tsx", root), "utf8");
  await stat(new URL("app/favicon.ico", root));
  assert.match(dockerfile, /\/app\/public \.\/public/);
  assert.match(layout, /\/favicon\.ico/);
});

test("guest session endpoint is a non-error anonymous probe", async () => {
  const main = await readFile(new URL("../api/cmd/api/main.go", root), "utf8");
  const handler = await readFile(new URL("../api/internal/auth/http.go", root), "utf8");
  const auth = await readFile(new URL("app/auth-state.tsx", root), "utf8");
  assert.match(main, /auth\/session.*OptionalSession/);
  assert.match(handler, /authJSON\(w, http\.StatusOK, map\[string\]any\{"data": nil\}\)/);
  assert.match(auth, /if \(!body\?\.data\)/);
});
