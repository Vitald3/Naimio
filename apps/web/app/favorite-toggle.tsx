"use client";

import { useEffect, useState } from "react";
import { IconHeart } from "./icons";
import { useToast } from "./toast";

type FavoriteType = "FREELANCER" | "SERVICE" | "PROJECT";

export default function FavoriteToggle({
  entityType,
  entityId,
  compact = false,
}: {
  entityType: FavoriteType;
  entityId: string;
  compact?: boolean;
}) {
  const { push } = useToast();
  const [active, setActive] = useState(false);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    let aborted = false;
    fetch(`/api/v1/me/favorites?type=${entityType}`, { credentials: "same-origin" })
      .then((response) => (response.ok ? response.json() : { data: [] }))
      .then((body) => {
        if (aborted) return;
        const items = Array.isArray(body?.data) ? body.data : [];
        setActive(items.some((item: { entity_id?: string }) => item.entity_id === entityId));
      })
      .catch(() => undefined);
    return () => {
      aborted = true;
    };
  }, [entityId, entityType]);

  async function toggle() {
    if (busy) return;
    setBusy(true);
    const next = !active;
    setActive(next);
    try {
      const response = await fetch(
        next ? "/api/v1/me/favorites" : `/api/v1/me/favorites/${entityType}/${entityId}`,
        next
          ? {
              method: "POST",
              credentials: "same-origin",
              headers: { "Content-Type": "application/json" },
              body: JSON.stringify({ entity_type: entityType, entity_id: entityId }),
            }
          : { method: "DELETE", credentials: "same-origin" },
      );
      if (!response.ok) throw new Error();
      push({ kind: "success", title: next ? "Добавлено в избранное" : "Удалено из избранного" });
    } catch {
      setActive(!next);
      push({ kind: "error", title: "Не удалось изменить избранное", message: "Попробуйте ещё раз чуть позже." });
    } finally {
      setBusy(false);
    }
  }

  return (
    <button
      type="button"
      onClick={toggle}
      disabled={busy}
      className={`favorite-toggle favorite-toggle--icon${compact ? " favorite-toggle--compact" : ""}${active ? " is-saved" : ""}`}
      aria-pressed={active}
      aria-label={active ? "Удалить из избранного" : "Добавить в избранное"}
      title={active ? "Удалить из избранного" : "Добавить в избранное"}
    >
      <IconHeart size={compact ? 16 : 18} />
    </button>
  );
}
