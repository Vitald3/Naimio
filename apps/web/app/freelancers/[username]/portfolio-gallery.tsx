"use client";

import Image from "next/image";
import { useEffect, useState } from "react";
import { Cover } from "../../media-components";
import EngagementTracker from "../../engagement-tracker";

export type PortfolioGalleryItem = {
  id:string; title:string; description?:string; external_url?:string; price?:string|null; completed_on?:string;
  categories:Array<{id:string;name:string}>; skills:Array<{id:string;name:string}>; media:Array<{id:string}>;
};

export default function PortfolioGallery({username,subjectUserID,items}:{username:string;subjectUserID:string;items:PortfolioGalleryItem[]}){
  const [workIndex,setWorkIndex]=useState<number|null>(null);
  const [imageIndex,setImageIndex]=useState(0);
  const active=workIndex===null?null:items[workIndex];
  const mediaURL=(work:PortfolioGalleryItem,mediaID:string)=>`/api/v1/profiles/${encodeURIComponent(username)}/portfolio/${work.id}/media/${mediaID}`;
  function open(index:number){setWorkIndex(index);setImageIndex(0)}
  function close(){setWorkIndex(null);setImageIndex(0)}
  function changeWork(offset:number){if(workIndex===null)return;setWorkIndex((workIndex+offset+items.length)%items.length);setImageIndex(0)}
  useEffect(()=>{if(!active)return;const key=(event:KeyboardEvent)=>{if(event.key==="Escape")close();if(event.key==="ArrowLeft")setImageIndex(v=>(v-1+Math.max(active.media.length,1))%Math.max(active.media.length,1));if(event.key==="ArrowRight")setImageIndex(v=>(v+1)%Math.max(active.media.length,1))};document.addEventListener("keydown",key);document.body.classList.add("modal-open");return()=>{document.removeEventListener("keydown",key);document.body.classList.remove("modal-open")}},[active]);
  return <>
    <ul className="portfolio-media-grid">{items.map((item,index)=><li key={item.id}><article className="portfolio-media-card"><button className="portfolio-media-card__open" type="button" onClick={()=>open(index)} aria-label={`Открыть работу «${item.title}»`}>
      {item.media[0]?<Image src={mediaURL(item,item.media[0].id)} alt={item.title} width={720} height={405} sizes="(max-width: 768px) 100vw, 420px" unoptimized/>:<Cover id={item.id} title={item.title}/>}<span>Смотреть работу</span>
    </button><div className="portfolio-media-card__body"><h3>{item.title}</h3>{item.description?<p>{item.description}</p>:null}<div className="portfolio-card__meta">{item.price?<span>{item.price}</span>:null}{item.media.length>1?<span>{item.media.length} фото</span>:null}</div></div></article></li>)}</ul>
    {active?<div className="portfolio-modal" role="dialog" aria-modal="true" aria-label={`Работа «${active.title}»`} onMouseDown={event=>{if(event.target===event.currentTarget)close()}}><EngagementTracker eventType="PORTFOLIO_VIEW" subjectUserID={subjectUserID} entityID={active.id}/><div className="portfolio-modal__panel"><button type="button" className="portfolio-modal__close" onClick={close} aria-label="Закрыть">×</button><div className="portfolio-modal__stage">
      {active.media[imageIndex]?<Image src={mediaURL(active,active.media[imageIndex].id)} alt={`${active.title}, фото ${imageIndex+1}`} width={1400} height={900} sizes="90vw" unoptimized priority/>:<Cover id={active.id} title={active.title}/>} 
      {active.media.length>1?<><button type="button" className="portfolio-modal__arrow portfolio-modal__arrow--prev" onClick={()=>setImageIndex((imageIndex-1+active.media.length)%active.media.length)} aria-label="Предыдущее фото">‹</button><button type="button" className="portfolio-modal__arrow portfolio-modal__arrow--next" onClick={()=>setImageIndex((imageIndex+1)%active.media.length)} aria-label="Следующее фото">›</button><span className="portfolio-modal__counter">{imageIndex+1} / {active.media.length}</span></>:null}
    </div><aside className="portfolio-modal__info"><p className="eyebrow">Работа {workIndex!+1} из {items.length}</p><h2>{active.title}</h2>{active.description?<p>{active.description}</p>:null}{active.categories.length?<div><strong>Направления</strong><div className="chip-row">{active.categories.map(v=><span className="chip" key={v.id}>{v.name}</span>)}</div></div>:null}{active.skills.length?<div><strong>Навыки</strong><div className="chip-row">{active.skills.map(v=><span className="chip" key={v.id}>{v.name}</span>)}</div></div>:null}<div className="portfolio-modal__work-nav"><button type="button" className="button button--quiet" onClick={()=>changeWork(-1)}>← Предыдущая</button><button type="button" className="button button--quiet" onClick={()=>changeWork(1)}>Следующая →</button></div>{active.external_url?<a className="button" href={active.external_url} target="_blank" rel="noopener noreferrer">Открыть проект</a>:null}</aside></div></div>:null}
  </>;
}
