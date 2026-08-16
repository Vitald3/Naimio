"use client";

import { IconGrid, IconList } from "./icons";

export type CatalogView = "list" | "grid";

export default function ViewToggle({ value, onChange }: { value: CatalogView; onChange: (value: CatalogView) => void }) {
  return <div className="view-toggle" role="group" aria-label="Вид каталога">
    <button type="button" className={value === "list" ? "is-active" : ""} aria-label="Список" aria-pressed={value === "list"} onClick={() => onChange("list")}><IconList size={18}/></button>
    <button type="button" className={value === "grid" ? "is-active" : ""} aria-label="Плитка" aria-pressed={value === "grid"} onClick={() => onChange("grid")}><IconGrid size={18}/></button>
  </div>;
}
