"use client";

import { useEffect, useState } from "react";
import { useAuth } from "./auth-state";
import { IconHeart } from "./icons";
import { notify } from "./toast";

type EntityType = "FREELANCER" | "SERVICE" | "PROJECT";
const cache = new Map<EntityType, Promise<Set<string>>>();
function favorites(type: EntityType) {
  if (!cache.has(type)) {
    cache.set(type, fetch(`/api/v1/me/favorites?type=${type}&limit=50`, { credentials: "same-origin", cache: "no-store" })
      .then((r) => r.ok ? r.json() : { data: [] })
      .then((body) => new Set<string>((body.data ?? []).map((item: { entity_id: string }) => item.entity_id)))
      .catch(() => new Set<string>()));
  }
  return cache.get(type)!;
}

export default function FavoriteButton({ type, id, compact = false }: { type: EntityType; id: string; compact?: boolean }) {
  const { state } = useAuth();
  const [saved, setSaved] = useState(false);
  const [busy, setBusy] = useState(false);
  useEffect(() => {
    if (state !== "authenticated") return;
    favorites(type).then((items) => setSaved(items.has(id)));
  }, [state, type, id]);
  if (state === "loading") return <span className="favorite-toggle favorite-toggle--loading skeleton" aria-label="Проверяем избранное" />;
  async function toggle() {
    if (state !== "authenticated") {
      location.assign(`/login?next=${encodeURIComponent(location.pathname)}`);
      return;
    }
    setBusy(true);
    const method = saved ? "DELETE" : "PUT";
    try {
      const response = await fetch(`/api/v1/me/favorites/${type}/${encodeURIComponent(id)}`, { method, credentials: "same-origin" });
      if (!response.ok) throw new Error();
      const next = !saved;
      setSaved(next);
      cache.delete(type);
      notify(next ? "Добавлено в избранное" : "Удалено из избранного", "success");
    } catch {
      notify("Не удалось изменить избранное. Попробуйте ещё раз.", "error");
    } finally { setBusy(false); }
  }
  const label = saved ? "Удалить из избранного" : "Добавить в избранное";
  return <button type="button" className={`favorite-toggle favorite-toggle--icon${saved ? " is-saved" : ""}${compact ? " favorite-toggle--compact" : ""}`} onClick={toggle} disabled={busy} aria-pressed={saved} aria-label={label} title={label}><IconHeart size={19}/></button>;
}
