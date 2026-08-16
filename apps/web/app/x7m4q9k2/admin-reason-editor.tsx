"use client";

import { useEffect, useState } from "react";
import { EditorContent, useEditor } from "@tiptap/react";
import StarterKit from "@tiptap/starter-kit";
import Link from "@tiptap/extension-link";
import Placeholder from "@tiptap/extension-placeholder";
import FormattedText from "../formatted-text";

const MAX_REASON_LENGTH = 15_000;

export default function AdminReasonEditor({value,onChange,autoFocus=false}:{value:string;onChange:(value:string)=>void;autoFocus?:boolean}){
  const [preview,setPreview]=useState(false);
  const editor=useEditor({
    immediatelyRender:false,
    extensions:[
      StarterKit.configure({link:false}),
      Link.configure({openOnClick:false,autolink:true,HTMLAttributes:{rel:"noopener noreferrer nofollow",target:"_blank"}}),
      Placeholder.configure({placeholder:"Опишите причину решения, контекст и важные детали"}),
    ],
    content:value,
    autofocus:autoFocus?"end":false,
    editorProps:{attributes:{class:"project-rich-editor__content admin-reason-editor__content","aria-label":"Причина"}},
    onUpdate:({editor:e})=>onChange(e.isEmpty?"":e.getHTML()),
  });

  useEffect(()=>{
    if(editor&&value!==editor.getHTML()&&!editor.isFocused)editor.commands.setContent(value,{emitUpdate:false});
  },[editor,value]);

  if(!editor)return <div className="project-rich-editor admin-reason-editor"><div className="project-rich-editor__content admin-reason-editor__content skeleton"/></div>;

  const link=()=>{
    const previous=editor.getAttributes("link").href as string|undefined;
    const url=window.prompt("Адрес ссылки",previous||"https://");
    if(url===null)return;
    if(!url.trim())editor.chain().focus().unsetLink().run();
    else editor.chain().focus().extendMarkRange("link").setLink({href:url.trim()}).run();
  };
  const button=(label:string,title:string,active:boolean,disabled:boolean,action:()=>void)=><button type="button" title={title} aria-label={title} aria-pressed={active} className={active?"is-active":""} disabled={disabled} onClick={action}>{label}</button>;

  return <div className="project-rich-editor admin-reason-editor">
    <div className="project-rich-editor__toolbar admin-reason-editor__toolbar" role="toolbar" aria-label="Форматирование причины">
      {button("Текст","Обычный текст",editor.isActive("paragraph"),false,()=>editor.chain().focus().setParagraph().run())}
      {button("H2","Заголовок второго уровня",editor.isActive("heading",{level:2}),false,()=>editor.chain().focus().toggleHeading({level:2}).run())}
      {button("H3","Заголовок третьего уровня",editor.isActive("heading",{level:3}),false,()=>editor.chain().focus().toggleHeading({level:3}).run())}
      {button("Ж","Жирный — Ctrl+B",editor.isActive("bold"),!editor.can().chain().focus().toggleBold().run(),()=>editor.chain().focus().toggleBold().run())}
      {button("К","Курсив — Ctrl+I",editor.isActive("italic"),!editor.can().chain().focus().toggleItalic().run(),()=>editor.chain().focus().toggleItalic().run())}
      {button("• список","Маркированный список",editor.isActive("bulletList"),false,()=>editor.chain().focus().toggleBulletList().run())}
      {button("1. список","Нумерованный список",editor.isActive("orderedList"),false,()=>editor.chain().focus().toggleOrderedList().run())}
      {button("Цитата","Цитата",editor.isActive("blockquote"),false,()=>editor.chain().focus().toggleBlockquote().run())}
      {button("Ссылка","Добавить или изменить ссылку",editor.isActive("link"),false,link)}
      {button("Отменить","Отменить последнее действие",false,!editor.can().chain().focus().undo().run(),()=>editor.chain().focus().undo().run())}
      {button("Повторить","Вернуть отменённое действие",false,!editor.can().chain().focus().redo().run(),()=>editor.chain().focus().redo().run())}
      {button("Очистить","Очистить форматирование",false,false,()=>editor.chain().focus().clearNodes().unsetAllMarks().run())}
      <button type="button" className={preview?"is-active":""} onClick={()=>setPreview(current=>!current)}>{preview?"Редактор":"Предпросмотр"}</button>
    </div>
    {preview?<div className="project-rich-editor__preview admin-reason-editor__preview"><FormattedText value={editor.getHTML()}/></div>:<EditorContent editor={editor}/>} 
    <div className="project-rich-editor__footer admin-reason-editor__footer">
      <span>Причина сохраняется с форматированием и безопасно отображается в админке, уведомлениях и аудите.</span>
      <span>{editor.getText().length.toLocaleString("ru-RU")} / {MAX_REASON_LENGTH.toLocaleString("ru-RU")}</span>
    </div>
  </div>;
}
