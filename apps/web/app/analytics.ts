"use client";

import { createRandomID } from "./random-id";

const allowedUTM = ["utm_source", "utm_medium", "utm_campaign", "utm_content", "ref"] as const;
const allowedMetadata = new Set(["calculator_slug", "source", "draft_source"]);
export function anonymousID() {
  let value = localStorage.getItem("acquisition-anonymous-id");
  if (!value) { value = createRandomID(); localStorage.setItem("acquisition-anonymous-id", value); }
  return value;
}
export function attribution() {
  const query = new URLSearchParams(location.search);
  const values: Record<string, string> = { anonymous_id: anonymousID(), landing_path: location.pathname };
  for (const key of allowedUTM) { const value = query.get(key); if (value) values[key === "ref" ? "referral_code" : key] = value.slice(0, 200); }
  return values;
}
export function track(event_type: string, metadata: Record<string, string> = {}) {
  const safeMetadata = Object.fromEntries(Object.entries(metadata).filter(([key, value]) => allowedMetadata.has(key) && value.length <= 160));
  void fetch("/api/v1/acquisition/events", { method: "POST", credentials: "same-origin", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ event_type, ...attribution(), metadata: safeMetadata }), keepalive: true });
}
