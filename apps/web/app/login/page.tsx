"use client";
import{FormEvent,useState}from"react";
export default function LoginPage(){
  const[email,setEmail]=useState("");const[password,setPassword]=useState("");const[state,setState]=useState("");
  async function submit(e:FormEvent){
    e.preventDefault();setState("Входим…");
    const response=await fetch("/api/v1/auth/login",{method:"POST",credentials:"same-origin",headers:{"Content-Type":"application/json"},body:JSON.stringify({email,password,portal:"marketplace"})});
    if(!response.ok){setState(response.status===403?"Эта служебная учётная запись не использует пользовательский кабинет Naimio.":"Неверная почта или пароль");return}
    const next=new URLSearchParams(location.search).get("next");location.assign(next?.startsWith("/")&&!next.startsWith("//")?next:"/")
  }
  return <main><h1>Вход</h1><form onSubmit={submit}><label>Email <input type="email" autoComplete="email" required maxLength={320} value={email} onChange={e=>setEmail(e.target.value)}/></label><label>Пароль <input type="password" autoComplete="current-password" required maxLength={128} value={password} onChange={e=>setPassword(e.target.value)}/></label><button>Войти</button></form><p role="status">{state}</p><p><a href="/register">Создать аккаунт</a></p></main>
}
