"use client";
import { useEffect, useRef, useState } from "react";
import { EditorContent, useEditor } from "@tiptap/react";
import StarterKit from "@tiptap/starter-kit";
import Link from "@tiptap/extension-link";

export default function BlogEditor({value,onChange}:{value:string;onChange:(v:string)=>void}){
  const [uploading,setUploading]=useState(false);const file=useRef<HTMLInputElement>(null);
  const editor=useEditor({immediatelyRender:false,extensions:[StarterKit.configure({link:false}),Link.configure({openOnClick:false,autolink:true,HTMLAttributes:{rel:"noopener noreferrer nofollow"}})],content:value,editorProps:{attributes:{class:"project-rich-editor__content article-content","aria-label":"Текст статьи"}},onUpdate:({editor:e})=>onChange(e.isEmpty?"":e.getHTML())});
  useEffect(()=>{if(editor&&value!==editor.getHTML()&&!editor.isFocused)editor.commands.setContent(value,{emitUpdate:false})},[editor,value]);
  if(!editor)return <div className="skeleton"/>;
  const instance=editor;
  const cmd=(label:string,active:boolean,run:()=>void)=><button type="button" className={active?"is-active":undefined} onClick={run}>{label}</button>;
  function addLink(){const url=window.prompt("Адрес ссылки","https://");if(url)instance.chain().focus().extendMarkRange("link").setLink({href:url}).run()}
  async function imageUpload(f?:File){if(!f)return;setUploading(true);try{const p=await fetch("/api/v1/uploads/presign",{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({purpose:"BLOG_CONTENT",filename:f.name,mime_type:f.type,size_bytes:f.size})});if(!p.ok)throw new Error();const x=(await p.json()).data;const put=await fetch(x.upload_url,{method:"PUT",headers:x.headers,body:f});if(!put.ok)throw new Error();const done=await fetch(`/api/v1/uploads/${x.media_id}/complete`,{method:"POST"});if(!done.ok)throw new Error();const alt=window.prompt("Альтернативный текст изображения","Иллюстрация к статье")||"";instance.chain().focus().insertContent(`<img src="/api/v1/media/${x.media_id}" alt="${alt.replaceAll('"','&quot;')}"><p></p>`).run()}finally{setUploading(false);if(file.current)file.current.value=""}}
  return <div className="project-rich-editor blog-editor"><div className="project-rich-editor__toolbar" role="toolbar" aria-label="Форматирование статьи">
    {cmd("Текст",instance.isActive("paragraph"),()=>instance.chain().focus().setParagraph().run())}{cmd("H2",instance.isActive("heading",{level:2}),()=>instance.chain().focus().toggleHeading({level:2}).run())}{cmd("H3",instance.isActive("heading",{level:3}),()=>instance.chain().focus().toggleHeading({level:3}).run())}{cmd("Ж",instance.isActive("bold"),()=>instance.chain().focus().toggleBold().run())}{cmd("К",instance.isActive("italic"),()=>instance.chain().focus().toggleItalic().run())}{cmd("• список",instance.isActive("bulletList"),()=>instance.chain().focus().toggleBulletList().run())}{cmd("1. список",instance.isActive("orderedList"),()=>instance.chain().focus().toggleOrderedList().run())}{cmd("Цитата",instance.isActive("blockquote"),()=>instance.chain().focus().toggleBlockquote().run())}{cmd("Код",instance.isActive("codeBlock"),()=>instance.chain().focus().toggleCodeBlock().run())}
    <button type="button" onClick={addLink}>Ссылка</button><button type="button" onClick={()=>file.current?.click()} disabled={uploading}>{uploading?"Загрузка…":"Изображение"}</button><button type="button" onClick={()=>instance.chain().focus().undo().run()}>Отменить</button><button type="button" onClick={()=>instance.chain().focus().redo().run()}>Повторить</button><input ref={file} hidden type="file" accept="image/jpeg,image/png,image/webp" onChange={e=>void imageUpload(e.target.files?.[0])}/>
  </div><EditorContent editor={instance}/><div className="project-rich-editor__footer"><span>HTML очищается повторно на сервере перед сохранением.</span><span>{instance.getText().length.toLocaleString("ru-RU")} знаков</span></div></div>
}
