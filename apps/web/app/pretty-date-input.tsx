"use client";
import { useEffect, useMemo, useRef, useState } from "react";
import { IconCalendar } from "./icons";

const iso = (date: Date) => `${date.getFullYear()}-${String(date.getMonth()+1).padStart(2,"0")}-${String(date.getDate()).padStart(2,"0")}`;
const parse = (value: string) => { const match=/^(\d{4})-(\d{2})-(\d{2})$/.exec(value); return match ? new Date(Number(match[1]), Number(match[2])-1, Number(match[3])) : null; };
const valueLabel = (value: string) => { const date=parse(value); return date ? new Intl.DateTimeFormat("ru-RU",{day:"numeric",month:"long",year:"numeric"}).format(date) : "Выберите дату"; };
const monthNames = Array.from({length:12},(_,month)=>new Intl.DateTimeFormat("ru-RU",{month:"long"}).format(new Date(2020,month,1)));

export default function PrettyDateInput({ value, onChange, min, ariaLabel }: { value: string; onChange: (value:string)=>void; min?: string; ariaLabel?: string }) {
  const selected=parse(value);
  const [open,setOpen]=useState(false);
  const [periodMenu,setPeriodMenu]=useState<"month"|"year"|null>(null);
  const [month,setMonth]=useState(()=>selected ?? new Date());
  const root=useRef<HTMLDivElement>(null);
  useEffect(()=>{ const next=parse(value); if(next) setMonth(next); },[value]);
  useEffect(()=>{
    if(!open)return;
    const close=(event:PointerEvent)=>{if(!root.current?.contains(event.target as Node)){setOpen(false);setPeriodMenu(null)}};
    const escape=(event:KeyboardEvent)=>{if(event.key==="Escape"){if(periodMenu)setPeriodMenu(null);else setOpen(false)}};
    document.addEventListener("pointerdown",close);document.addEventListener("keydown",escape);
    return()=>{document.removeEventListener("pointerdown",close);document.removeEventListener("keydown",escape)};
  },[open,periodMenu]);
  const days=useMemo(()=>{const first=new Date(month.getFullYear(),month.getMonth(),1);const offset=(first.getDay()+6)%7;const count=new Date(month.getFullYear(),month.getMonth()+1,0).getDate();return [...Array(offset).fill(null),...Array.from({length:count},(_,i)=>new Date(month.getFullYear(),month.getMonth(),i+1))]},[month]);
  const minDate=parse(min||"");
  const today=iso(new Date());
  const years=useMemo(()=>Array.from({length:24},(_,i)=>new Date().getFullYear()-4+i),[]);
  return <div className="date-picker-v2" ref={root}>
    <button type="button" className="date-picker-v2__trigger" aria-label={ariaLabel} aria-haspopup="dialog" aria-expanded={open} onClick={()=>{setOpen(v=>!v);setPeriodMenu(null)}}>
      <span>{valueLabel(value)}</span><span className="date-picker-v2__icon" aria-hidden="true"><IconCalendar size={20}/></span>
    </button>
    {open?<div className="date-picker-v2__popover" role="dialog" aria-modal="false" aria-label="Выбор даты">
      <div className="date-picker-v2__header">
        <button type="button" className="date-picker-v2__nav" aria-label="Предыдущий месяц" onClick={()=>{setPeriodMenu(null);setMonth(new Date(month.getFullYear(),month.getMonth()-1,1))}}>‹</button>
        <div className="date-picker-v2__period">
          <div className="date-picker-v2__period-control">
            <button type="button" className="date-picker-v2__period-trigger" aria-haspopup="listbox" aria-expanded={periodMenu==="month"} onClick={()=>setPeriodMenu(current=>current==="month"?null:"month")}>{monthNames[month.getMonth()]}</button>
            {periodMenu==="month"?<div className="date-picker-v2__period-menu date-picker-v2__period-menu--months" role="listbox" aria-label="Месяц">{monthNames.map((name,index)=><button key={name} type="button" role="option" aria-selected={index===month.getMonth()} className={index===month.getMonth()?"is-selected":""} onClick={()=>{setMonth(new Date(month.getFullYear(),index,1));setPeriodMenu(null)}}><span>{name}</span></button>)}</div>:null}
          </div>
          <div className="date-picker-v2__period-control">
            <button type="button" className="date-picker-v2__period-trigger" aria-haspopup="listbox" aria-expanded={periodMenu==="year"} onClick={()=>setPeriodMenu(current=>current==="year"?null:"year")}>{month.getFullYear()}</button>
            {periodMenu==="year"?<div className="date-picker-v2__period-menu date-picker-v2__period-menu--years" role="listbox" aria-label="Год">{years.map(year=><button key={year} type="button" role="option" aria-selected={year===month.getFullYear()} className={year===month.getFullYear()?"is-selected":""} onClick={()=>{setMonth(new Date(year,month.getMonth(),1));setPeriodMenu(null)}}><span>{year}</span></button>)}</div>:null}
          </div>
        </div>
        <button type="button" className="date-picker-v2__nav" aria-label="Следующий месяц" onClick={()=>{setPeriodMenu(null);setMonth(new Date(month.getFullYear(),month.getMonth()+1,1))}}>›</button>
      </div>
      <div className="date-picker-v2__week">{["Пн","Вт","Ср","Чт","Пт","Сб","Вс"].map(day=><span key={day}>{day}</span>)}</div>
      <div className="date-picker-v2__days">{days.map((day,index)=>day?<button type="button" key={iso(day)} disabled={Boolean(minDate&&day<minDate)} className={[value===iso(day)?"is-selected":"",today===iso(day)?"is-today":""].filter(Boolean).join(" ")} onClick={()=>{onChange(iso(day));setOpen(false);setPeriodMenu(null)}}>{day.getDate()}</button>:<span aria-hidden="true" key={`blank-${index}`}/>)}</div>
      <div className="date-picker-v2__footer"><button type="button" onClick={()=>{onChange("");setOpen(false);setPeriodMenu(null)}}>Очистить</button><button type="button" onClick={()=>{const now=new Date();if(!minDate||now>=minDate)onChange(iso(now));setOpen(false);setPeriodMenu(null)}}>Сегодня</button></div>
    </div>:null}
  </div>;
}
