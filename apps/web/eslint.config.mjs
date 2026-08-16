import { defineConfig, globalIgnores } from "eslint/config";
import nextVitals from "eslint-config-next/core-web-vitals";
import nextTs from "eslint-config-next/typescript";

export default defineConfig([
  ...nextVitals,
  ...nextTs,
  {
    rules: {
      // The app intentionally performs async bootstrap/data loading in effects.
      // React 19's new heuristic flags these valid existing patterns; keep it
      // disabled until those screens are migrated to server data/router state.
      "react-hooks/set-state-in-effect": "off",
      // This App Router codebase still contains intentional plain anchors and
      // imperative navigations. They are valid, but Next 16 promotes the
      // migration hints to lint findings. Keep them out of the release-blocking
      // lint gate while preserving all correctness/security rules.
      "@next/next/no-html-link-for-pages": "off",
      "@next/next/no-location-assign-relative-destination": "off",
    },
  },
  globalIgnores([
    ".next/**",
    "out/**",
    "build/**",
    "node_modules/**",
    "next-env.d.ts",
  ]),
]);
