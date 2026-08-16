"use client";

import React, { Children, isValidElement, KeyboardEvent, ReactNode, useEffect, useId, useMemo, useRef, useState } from "react";

type Option = { value: string; label: string; disabled: boolean };
type Props = { value?: string | number; defaultValue?: string | number; onChange?: (event: { target: { value: string }; currentTarget: { value: string } }) => void; children: ReactNode; disabled?: boolean; required?: boolean; name?: string; id?: string; className?: string; "aria-label"?: string; "aria-invalid"?: boolean | "true" | "false" };

function optionList(children: ReactNode): Option[] {
  const result: Option[] = [];
  const visit = (nodes: ReactNode) => Children.forEach(nodes, child => {
    if (!isValidElement(child)) return;
    if (child.type === React.Fragment) return visit((child.props as {children?:ReactNode}).children);
    if (child.type === "option") {
      const props = child.props as { value?: string | number; disabled?: boolean; children?: ReactNode };
      result.push({ value: String(props.value ?? ""), label: Children.toArray(props.children).join(""), disabled: !!props.disabled });
      return;
    }
    const props = child.props as { children?: ReactNode };
    if (props.children) visit(props.children);
  });
  visit(children);
  return result;
}

export function CustomSelect({ value, defaultValue, onChange, children, disabled, required, name, id, className = "", "aria-label": ariaLabel, "aria-invalid": invalid }: Props) {
  const options = useMemo(() => optionList(children), [children]);
  const controlled = value !== undefined;
  const [internal, setInternal] = useState(String(defaultValue ?? options[0]?.value ?? ""));
  const selected = String(controlled ? value : internal);
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const searchable = options.length > 7;
  const visible = useMemo(() => {
    const needle = query.trim().toLocaleLowerCase("ru-RU");
    return !searchable || !needle ? options : options.filter(option => option.label.toLocaleLowerCase("ru-RU").includes(needle));
  }, [options, query, searchable]);
  const selectedIndex = Math.max(0, visible.findIndex(option => option.value === selected));
  const [active, setActive] = useState(selectedIndex);
  const root = useRef<HTMLDivElement>(null);
  const trigger = useRef<HTMLButtonElement>(null);
  const search = useRef<HTMLInputElement>(null);
  const generated = useId();
  const listId = `${id ?? generated}-listbox`;

  useEffect(() => setActive(Math.max(0, selectedIndex)), [selectedIndex, query]);
  useEffect(() => {
    if (!open) { setQuery(""); return; }
    const outside = (event: PointerEvent) => { if (!root.current?.contains(event.target as Node)) setOpen(false); };
    document.addEventListener("pointerdown", outside);
    if (searchable) window.requestAnimationFrame(() => search.current?.focus());
    return () => document.removeEventListener("pointerdown", outside);
  }, [open, searchable]);

  const move = (direction: 1 | -1) => {
    if (!visible.length) return;
    let next = active;
    do next = (next + direction + visible.length) % visible.length; while (visible[next]?.disabled && next !== active);
    setActive(next);
  };
  const choose = (index: number) => {
    const option = visible[index];
    if (!option || option.disabled) return;
    if (!controlled) setInternal(option.value);
    onChange?.({ target: { value: option.value }, currentTarget: { value: option.value } });
    setOpen(false); setQuery(""); trigger.current?.focus();
  };
  const keyDown = (event: KeyboardEvent) => {
    if (event.key === "Escape") { setOpen(false); trigger.current?.focus(); return; }
    if (event.key === "ArrowDown" || event.key === "ArrowUp") { event.preventDefault(); if (!open) setOpen(true); else move(event.key === "ArrowDown" ? 1 : -1); return; }
    if (event.key === "Enter" && open) { event.preventDefault(); choose(active); }
    if (event.key === " " && open && document.activeElement !== search.current) { event.preventDefault(); choose(active); }
  };
  const current = options.find(option => option.value === selected);

  return <div ref={root} className={`custom-select ${open ? "is-open" : ""} ${invalid === true || invalid === "true" ? "is-invalid" : ""} ${className}`.trim()} onKeyDown={keyDown}>
    {name ? <input type="hidden" name={name} value={selected} required={required}/> : null}
    <button ref={trigger} id={id} type="button" className="custom-select__trigger" disabled={disabled} aria-label={ariaLabel} aria-haspopup="listbox" aria-expanded={open} aria-controls={listId} onClick={() => setOpen(currentOpen => !currentOpen)}>
      <span className={!selected ? "custom-select__placeholder" : ""}>{current?.label || "Выберите значение"}</span><span className="custom-select__chevron" aria-hidden="true"/>
    </button>
    {open ? <div id={listId} className="custom-select__panel" role="listbox" aria-activedescendant={visible.length ? `${listId}-${active}` : undefined} tabIndex={-1}>
      {searchable ? <div className="custom-select__search"><input ref={search} type="search" value={query} onChange={event => setQuery(event.target.value)} placeholder="Поиск…" aria-label="Поиск по значениям"/></div> : null}
      {visible.length ? visible.map((option, index) => <button key={`${option.value}-${index}`} id={`${listId}-${index}`} type="button" role="option" aria-selected={option.value === selected} disabled={option.disabled} className={index === active ? "is-active" : ""} onPointerMove={() => setActive(index)} onClick={() => choose(index)}>{option.label}<span aria-hidden="true">{option.value === selected ? "✓" : ""}</span></button>) : <div className="custom-select__empty">Ничего не найдено</div>}
    </div> : null}
  </div>;
}
