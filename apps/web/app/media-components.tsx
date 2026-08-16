"use client";

import Image from "next/image";
import { useEffect, useState } from "react";
import { avatarFor, coverFor, initials } from "./media";

type PresenceListener=(online:boolean)=>void;
const presenceCache=new Map<string,{online:boolean;at:number}>();
const presenceListeners=new Map<string,Set<PresenceListener>>();
const presencePending=new Set<string>();
let presenceFlushTimer:number|undefined;
const PRESENCE_CACHE_MS=30_000;

function notifyPresence(id:string,online:boolean){
  presenceCache.set(id,{online,at:Date.now()});
  presenceListeners.get(id)?.forEach(listener=>listener(online));
}
function queuePresence(id:string,force=false){
  const cached=presenceCache.get(id);
  if(!force&&cached&&Date.now()-cached.at<PRESENCE_CACHE_MS)return;
  presencePending.add(id);
  if(presenceFlushTimer!==undefined)return;
  presenceFlushTimer=window.setTimeout(async()=>{
    presenceFlushTimer=undefined;
    const ids=Array.from(presencePending).slice(0,100);
    ids.forEach(value=>presencePending.delete(value));
    if(!ids.length)return;
    try{
      const response=await fetch('/api/v1/presence/batch',{method:'POST',credentials:'same-origin',cache:'no-store',headers:{'Content-Type':'application/json'},body:JSON.stringify({ids})});
      const body=response.ok?await response.json():null;
      ids.forEach(value=>notifyPresence(value,Boolean(body?.data?.[value])));
    }catch{ids.forEach(value=>notifyPresence(value,false));}
    if(presencePending.size)queuePresence(Array.from(presencePending)[0],true);
  },25);
}

export function useOnlineStatus(id?: string) {
  const [online, setOnline] = useState(()=>Boolean(id&&presenceCache.get(id)?.online));
  useEffect(() => {
    if (!id) { setOnline(false); return; }
    let listeners=presenceListeners.get(id);
    if(!listeners){listeners=new Set();presenceListeners.set(id,listeners);}
    listeners.add(setOnline);
    const cached=presenceCache.get(id);if(cached)setOnline(cached.online);
    queuePresence(id);
    const timer=window.setInterval(()=>queuePresence(id,true),60_000);
    return()=>{window.clearInterval(timer);const current=presenceListeners.get(id);current?.delete(setOnline);if(current&&!current.size)presenceListeners.delete(id);};
  }, [id]);
  return online;
}

export function PresenceLabel({ id }: { id?: string }) {
  const online = useOnlineStatus(id);
  return <span className={`presence-label${online ? " is-online" : ""}`} aria-label={online ? "Пользователь онлайн" : "Пользователь не в сети"}>{online ? "Онлайн" : "Не в сети"}</span>;
}

export function Avatar({
  name,
  id,
  size = "md",
  src,
}: {
  name: string;
  id?: string;
  size?: "sm" | "md" | "lg";
  src?: string;
}) {
  const pixels = size === "sm" ? 32 : size === "lg" ? 64 : 44;
  const fallback = avatarFor(id || name);
  const remote = src || (id ? `/api/v1/avatars/${encodeURIComponent(id)}` : fallback);
  const [source, setSource] = useState(remote);
  const online = useOnlineStatus(id);
  useEffect(() => setSource(remote), [remote]);
  return (
    <span className={`media-avatar media-avatar--${size}`}>
      <Image
        src={source}
        alt={`Аватар ${name}`}
        width={pixels}
        height={pixels}
        sizes={`${pixels}px`}
        unoptimized
        onError={() => setSource(fallback)}
      />
      <span>{initials(name)}</span>
      {online ? <i className="media-avatar__online" aria-label="В сети" title="В сети"/> : null}
    </span>
  );
}

export function Cover({
  id,
  type,
}: {
  id: string;
  title: string;
  type?: string;
}) {
  return (
    <div className="media-cover">
      <Image
        src={coverFor(id, type)}
        alt=""
        width={720}
        height={405}
        sizes="(max-width: 768px) 100vw, 360px"
        unoptimized
      />
    </div>
  );
}
