"use client";

import { FormEvent, useCallback, useEffect, useRef, useState } from "react";
import { useAuth } from "../auth-state";
import Breadcrumbs from "../breadcrumbs";
import { useToast } from "../toast";
import FileTypeBadge from "../file-type-badge";
import { createRandomID } from "../random-id";
import { useOnlineStatus } from "../media-components";
import { IconMic } from "../icons";
import { ChatMessagesSkeleton, ConversationListSkeleton } from "../skeletons";

type Conversation = {
  id: string;
  kind: "DIRECT" | "PROJECT";
  project_id?: string;
  project_title?: string;
  counterparty_name?: string;
  counterparty_user_id?: string;
  counterparty_username?: string;
  counterparty_role?: "FREELANCER" | "CUSTOMER";
  unread_count: number;
  updated_at: string;
};
type Message = {
  id: string;
  conversation_id: string;
  sender_user_id: string;
  type: string;
  body?: string;
  reply_to_message_id?: string;
  reply_quote?: string;
  media_ids: string[];
  edited_at?: string;
  deleted_at?: string;
  created_at: string;
};
type Deal = { id: string; status: string; gross_amount_kopecks: number };
type AttachmentView = { original_filename?: string; mime_type?: string; size_bytes?: number; download_url?: string };
const fmtTime = (iso: string) => {
  try {
    return new Intl.DateTimeFormat("ru-RU", {
      hour: "2-digit",
      minute: "2-digit",
    }).format(new Date(iso));
  } catch {
    return "";
  }
};
const convTitle = (c: Conversation) =>
  c.project_title ||
  c.counterparty_name ||
  (c.kind === "PROJECT" ? "Диалог по проекту" : "Личный диалог");

function MessageAttachment({ messageID, mediaID }: { messageID: string; mediaID: string }) {
  const [view, setView] = useState<AttachmentView | null>(null);
  const [failed, setFailed] = useState(false);
  useEffect(() => {
    fetch(`/api/v1/messages/${messageID}/attachments/${mediaID}`, { credentials: "same-origin", cache: "no-store" })
      .then((response) => response.ok ? response.json() : Promise.reject())
      .then((body) => setView(body.data ?? null))
      .catch(() => setFailed(true));
  }, [messageID, mediaID]);
  if (failed) return <span className="message-attachment message-attachment--failed">Вложение недоступно</span>;
  const name = view?.original_filename || "Вложение";
  if (view?.mime_type?.startsWith("audio/")) {
    return <div className="message-audio"><span className="message-audio__icon"><IconMic/></span><div><strong>Голосовое сообщение</strong>{view.download_url ? <audio controls preload="metadata" src={view.download_url}/> : <small>Подготавливаем аудио…</small>}</div></div>;
  }
  return <a className="message-attachment" href={view?.download_url || "#"} download target="_blank" rel="noopener noreferrer" aria-disabled={!view?.download_url} onClick={(event) => { if (!view?.download_url) event.preventDefault(); }}>
    <FileTypeBadge name={name} mimeType={view?.mime_type}/><span><strong>{name}</strong><small>{view?.size_bytes ? `${Math.max(1, Math.round(view.size_bytes / 1024))} КБ` : "Подготавливаем файл…"}</small></span><b>↓</b>
  </a>;
}

function communicationError(
  problem: { error?: { code?: string; message?: string } } | null,
  status: number,
) {
  const code = problem?.error?.code;
  if (status === 401 || code === "UNAUTHENTICATED")
    return "Сессия завершилась. Войдите снова и повторите отправку.";
  if (status === 404 || code === "NOT_FOUND")
    return "Диалог больше недоступен или был закрыт.";
  if (status === 409 || code === "CONFLICT")
    return "Сообщение уже обработано или состояние диалога изменилось. Обновите переписку.";
  if (status === 422 || code === "VALIDATION_ERROR")
    return "Проверьте текст сообщения или вложение: данные не прошли проверку.";
  if (status >= 500)
    return "Не удалось отправить сообщение. Проверьте соединение и попробуйте ещё раз.";
  return (
    problem?.error?.message ||
    "Не удалось отправить сообщение. Проверьте данные и попробуйте ещё раз."
  );
}

export default function MessagesPage() {
  const { user } = useAuth();
  const { push } = useToast();
  const [conversations, setConversations] = useState<Conversation[]>([]);
  const [active, setActive] = useState("");
  const [messages, setMessages] = useState<Message[]>([]);
  const [deal, setDeal] = useState<Deal | null>(null);
  const [body, setBody] = useState("");
  const [file, setFile] = useState<File | null>(null);
  const [state, setState] = useState<
    "idle" | "sending" | "failed" | "scanning"
  >("idle");
  const [error, setError] = useState("");
  const [realtimeReady, setRealtimeReady] = useState(false);
  const [typingUser, setTypingUser] = useState("");
  const [onlineUsers, setOnlineUsers] = useState<Record<string, number>>({});
  const [replyTo, setReplyTo] = useState<Message | null>(null);
  const [replyQuote, setReplyQuote] = useState("");
  const [editing, setEditing] = useState<Message | null>(null);
  const [recording, setRecording] = useState(false);
  const [recordingSeconds, setRecordingSeconds] = useState(0);
  const recorder = useRef<MediaRecorder | null>(null);
  const recordingStream = useRef<MediaStream | null>(null);
  const recordingTimer = useRef<number | null>(null);
  const activeConversation = conversations.find((item) => item.id === active);
  const counterpartyOnline = useOnlineStatus(activeConversation?.counterparty_user_id);
  const socket = useRef<WebSocket | null>(null);
  const typingTimer = useRef<number | null>(null);
  const longPressTimer = useRef<number | null>(null);
  const pointerStartX = useRef<number | null>(null);
  const retry = useRef<{
    body: string;
    file: File | null;
    clientID: string;
    mediaID?: string;
  } | null>(null);
  const loadConversations = useCallback(async () => {
    const r = await fetch("/api/v1/conversations", {
      credentials: "same-origin",
    });
    if (!r.ok) throw new Error("Не удалось загрузить диалоги");
    const data = await r.json();
    const items: Conversation[] = data.data ?? [];
    const requested = new URLSearchParams(location.search).get("conversation");
    setConversations(items);
    setActive(
      (current) =>
        current ||
        (requested && items.some((item) => item.id === requested)
          ? requested
          : (items[0]?.id ?? "")),
    );
    setRealtimeReady(true);
  }, []);
  const loadMessages = useCallback(async (id: string) => {
    if (!id) return;
    const r = await fetch(`/api/v1/conversations/${id}/messages?limit=50`, {
      credentials: "same-origin",
    });
    if (!r.ok) throw new Error("Не удалось загрузить сообщения");
    const data = await r.json();
    setMessages((data.data ?? []).reverse());
    const last = data.data?.[0];
    if (last)
      await fetch(`/api/v1/conversations/${id}/read`, {
        method: "POST",
        credentials: "same-origin",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ last_read_message_id: last.id }),
      });
  }, []);
  useEffect(() => {
    loadConversations().catch((e) => setError(e.message));
  }, [loadConversations]);
  useEffect(() => {
    loadMessages(active).catch((e) => setError(e.message));
  }, [active, loadMessages]);
  useEffect(() => {
    const project = conversations.find((v) => v.id === active)?.project_id;
    if (!project) {
      setDeal(null);
      return;
    }
    fetch(`/api/v1/me/safe-deals?project_id=${encodeURIComponent(project)}`, {
      credentials: "same-origin",
    })
      .then((r) => (r.ok ? r.json() : null))
      .then((b) => setDeal(b?.data?.[0] ?? null))
      .catch(() => setDeal(null));
  }, [active, conversations]);
  useEffect(() => {
    if (!user || !realtimeReady) return;
    const protocol = location.protocol === "https:" ? "wss" : "ws";
    const explicitWS = process.env.NEXT_PUBLIC_WS_URL;
    let disposed = false;
    let reconnectTimer: number | undefined;
    let presenceTimer: number | undefined;
    const connect = () => {
      if (disposed) return;
      const ws = new WebSocket(
        explicitWS || `${protocol}://${location.host}/api/v1/ws`,
      );
      socket.current = ws;
      ws.onopen = () => {
        if (presenceTimer !== undefined) window.clearInterval(presenceTimer);
        if (active) ws.send(JSON.stringify({ event: "presence.ping", conversation_id: active }));
        presenceTimer = window.setInterval(() => {
          if (ws.readyState === WebSocket.OPEN && active) ws.send(JSON.stringify({ event: "presence.ping", conversation_id: active }));
          setOnlineUsers(current => Object.fromEntries(Object.entries(current).filter(([, seen]) => Date.now() - seen < 45000)));
        }, 20000);
      };
      ws.onmessage = (event) => {
        const envelope = JSON.parse(event.data);
        const data = envelope.data as Message & { user_id?: string; state?: string; last_read_message_id?: string };
        if (envelope.event === "presence.updated" && data.user_id) setOnlineUsers(current => ({ ...current, [data.user_id!]: Date.now() }));
        if (envelope.event === "typing.started" && data.conversation_id === active && data.user_id !== user.id) setTypingUser(data.user_id || "");
        if (envelope.event === "typing.stopped" && data.conversation_id === active && data.user_id !== user.id) setTypingUser("");
        if (envelope.event === "message.created" && data.id) {
          if (data.conversation_id === active) setMessages(current => current.some(item => item.id === data.id) ? current : [...current, data]);
          setConversations(current => current.map(item => item.id === data.conversation_id ? { ...item, updated_at: data.created_at || item.updated_at, unread_count: data.sender_user_id !== user.id && data.conversation_id !== active ? item.unread_count + 1 : item.unread_count } : item));
        }
        if ((envelope.event === "message.updated" || envelope.event === "message.deleted") && data.id && data.conversation_id === active) {
          setMessages(current => current.map(item => item.id === data.id ? { ...item, ...data } : item));
        }
        if (envelope.event === "conversation.read" && data.user_id === user.id && data.conversation_id) {
          setConversations(current => current.map(item => item.id === data.conversation_id ? { ...item, unread_count: 0 } : item));
        }
      };
      ws.onclose = () => {
        if (presenceTimer !== undefined) { window.clearInterval(presenceTimer); presenceTimer = undefined; }
        if (socket.current === ws) socket.current = null;
        if (!disposed) reconnectTimer = window.setTimeout(connect, 1500);
      };
      ws.onerror = () => {
        // onclose schedules reconnect. Message loading still works over HTTP while
        // realtime is temporarily unavailable.
      };
    };
    connect();
    return () => {
      disposed = true;
      if (reconnectTimer !== undefined) window.clearTimeout(reconnectTimer);
      if (presenceTimer !== undefined) window.clearInterval(presenceTimer);
      const current = socket.current; socket.current = null; current?.close();
    };
  }, [user, realtimeReady, active]);
  useEffect(() => {
    if (!active) return;
    const raw = sessionStorage.getItem("service-conversation-context");
    if (!raw) return;
    try { const context = JSON.parse(raw); if (context.conversation_id === active && !body) { setBody(context.prompt || ""); sessionStorage.removeItem("service-conversation-context"); } } catch { sessionStorage.removeItem("service-conversation-context"); }
  }, [active, body]);
  function changeBody(value: string) {
    setBody(value);
    const ws = socket.current;
    if (ws?.readyState !== WebSocket.OPEN || !active) return;
    ws.send(JSON.stringify({ event: "typing.start", conversation_id: active }));
    if (typingTimer.current) window.clearTimeout(typingTimer.current);
    typingTimer.current = window.setTimeout(() => ws.readyState === WebSocket.OPEN && ws.send(JSON.stringify({ event: "typing.stop", conversation_id: active })), 1200);
  }
  function startReply(message: Message, quote = "") {
    setEditing(null);
    setReplyTo(message);
    setReplyQuote((quote || message.body || "Вложение").trim().slice(0, 1000));
  }
  function cancelReply() { setReplyTo(null); setReplyQuote(""); }
  function scrollToMessage(id: string) {
    const element = document.getElementById(`message-${id}`);
    if (!element) return;
    element.scrollIntoView({ behavior: "smooth", block: "center" });
    element.classList.add("is-highlighted");
    window.setTimeout(() => element.classList.remove("is-highlighted"), 1400);
  }
  async function startRecording() {
    if (recording || editing) return;
    try {
      if (!navigator.mediaDevices?.getUserMedia) throw new Error("MICROPHONE_REQUIRES_SECURE_CONTEXT");
      const permission = await navigator.permissions?.query?.({ name: "microphone" as PermissionName }).catch(() => null);
      if (permission?.state === "denied") throw new Error("MICROPHONE_DENIED");
      const stream = await navigator.mediaDevices.getUserMedia({ audio: true });
      const preferred = ["audio/webm;codecs=opus", "audio/webm", "audio/ogg;codecs=opus"].find((mime) => typeof MediaRecorder !== "undefined" && MediaRecorder.isTypeSupported(mime));
      const mediaRecorder = preferred ? new MediaRecorder(stream, { mimeType: preferred }) : new MediaRecorder(stream);
      const chunks: BlobPart[] = [];
      mediaRecorder.ondataavailable = (event) => { if (event.data.size) chunks.push(event.data); };
      mediaRecorder.onstop = () => {
        const mime = mediaRecorder.mimeType.split(";")[0] || "audio/webm";
        const extension = mime.includes("ogg") ? "ogg" : "webm";
        const blob = new Blob(chunks, { type: mime });
        if (blob.size) setFile(new File([blob], `voice-${Date.now()}.${extension}`, { type: mime }));
        stream.getTracks().forEach((track) => track.stop());
        recordingStream.current = null;
        recorder.current = null;
      };
      recorder.current = mediaRecorder;
      recordingStream.current = stream;
      mediaRecorder.start(250);
      setRecording(true);
      setRecordingSeconds(0);
      recordingTimer.current = window.setInterval(() => setRecordingSeconds((value) => value + 1), 1000);
    } catch (error) {
      const code = error instanceof Error ? error.message : "";
      push({ kind: "error", title: "Не удалось включить микрофон", message: code === "MICROPHONE_REQUIRES_SECURE_CONTEXT" ? "Браузер разрешает запрос микрофона только в защищённом контексте (HTTPS или localhost). На локальном IP откройте Naimio по HTTPS." : code === "MICROPHONE_DENIED" ? "Доступ к микрофону запрещён в настройках браузера. Разрешите микрофон для Naimio и повторите." : "Подтвердите появившийся браузерный запрос доступа к микрофону." });
    }
  }
  function stopRecording(discard = false) {
    const activeRecorder = recorder.current;
    if (!activeRecorder || activeRecorder.state === "inactive") return;
    if (discard) activeRecorder.ondataavailable = null;
    activeRecorder.stop();
    recordingStream.current?.getTracks().forEach((track) => track.stop());
    if (recordingTimer.current) window.clearInterval(recordingTimer.current);
    recordingTimer.current = null;
    setRecording(false);
    setRecordingSeconds(0);
    if (discard) setFile(null);
  }
  function startEdit(message: Message) { cancelReply(); setEditing(message); setBody(message.body || ""); }
  async function removeMessage(message: Message) {
    if (!window.confirm("Удалить это сообщение?")) return;
    const response = await fetch(`/api/v1/messages/${message.id}`, { method: "DELETE", credentials: "same-origin" });
    if (!response.ok) { push({ kind: "error", title: "Не удалось удалить сообщение" }); return; }
    try {
      const payload = await response.json();
      const deleted = payload?.data as Message | undefined;
      if (deleted?.id) setMessages(current => current.map(item => item.id === deleted.id ? deleted : item));
    } catch {
      setMessages(current => current.map(item => item.id === message.id ? { ...item, deleted_at: new Date().toISOString(), body: "" } : item));
    }
  }
  async function upload(selected: File) {
    setState("scanning");
    const presign = await fetch("/api/v1/uploads/presign", {
      method: "POST",
      credentials: "same-origin",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        purpose: "CHAT",
        filename: selected.name,
        mime_type: selected.type,
        size_bytes: selected.size,
      }),
    });
    if (!presign.ok) throw new Error("Файл не прошёл проверку");
    const { data } = await presign.json();
    const put = await fetch(data.upload_url, {
      method: "PUT",
      headers: data.headers,
      body: selected,
    });
    if (!put.ok) throw new Error("Не удалось загрузить файл");
    const complete = await fetch(`/api/v1/uploads/${data.media_id}/complete`, {
      method: "POST",
      credentials: "same-origin",
    });
    if (!complete.ok) throw new Error("Не удалось завершить загрузку");
    for (let attempt = 0; attempt < 15; attempt++) {
      const status = await fetch(`/api/v1/uploads/${data.media_id}`, {
        credentials: "same-origin",
      });
      const result = await status.json();
      if (result.data?.scan_status === "CLEAN") return data.media_id;
      if (["FAILED", "INFECTED"].includes(result.data?.scan_status))
        throw new Error("Файл отклонён проверкой безопасности");
      await new Promise((resolve) => setTimeout(resolve, 1000));
    }
    throw new Error("Файл ещё проверяется. Попробуйте отправить позже.");
  }
  async function send(event?: FormEvent, override = retry.current) {
    event?.preventDefault();
    const text = override?.body ?? body;
    const selected = override?.file ?? file;
    const clientID = override?.clientID ?? createRandomID();
    let mediaID = override?.mediaID;
    if (!active || (!text.trim() && !selected)) return;
    setState("sending");
    setError("");
    try {
      if (editing) {
        const response = await fetch(`/api/v1/messages/${editing.id}`, { method: "PATCH", credentials: "same-origin", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ body: text.trim() }) });
        if (!response.ok) throw new Error("Не удалось изменить сообщение");
        const payload = await response.json();
        const updated = payload?.data as Message | undefined;
        if (updated?.id) setMessages(current => current.map(item => item.id === updated.id ? updated : item));
        setEditing(null); setBody(""); setState("idle"); return;
      }
      if (selected && !mediaID) mediaID = await upload(selected);
      const response = await fetch(`/api/v1/conversations/${active}/messages`, {
        method: "POST",
        credentials: "same-origin",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          client_message_id: clientID,
          type: mediaID
            ? selected?.type.startsWith("image/")
              ? "IMAGE"
              : selected?.type.startsWith("audio/")
                ? "AUDIO"
                : "FILE"
            : "TEXT",
          body: text.trim(),
          reply_to_message_id: replyTo?.id || "",
          reply_quote: replyTo ? replyQuote : "",
          media_ids: mediaID ? [mediaID] : [],
        }),
      });
      if (!response.ok) {
        let problem: null | { error?: { code?: string; message?: string } } =
          null;
        try {
          problem = await response.json();
        } catch {}
        throw new Error(communicationError(problem, response.status));
      }
      const payload = await response.json();
      const created = payload?.data as Message | undefined;
      if (created?.id) setMessages(current => current.some(item => item.id === created.id) ? current : [...current, created]);
      setBody("");
      cancelReply();
      setFile(null);
      retry.current = null;
      setState("idle");
    } catch (reason) {
      retry.current = { body: text, file: selected, clientID, mediaID };
      setState("failed");
      const message =
        reason instanceof Error ? reason.message : "Сообщение не отправлено";
      setError(message);
      push({ kind: "error", title: "Сообщение не отправлено", message });
    }
  }
  return (
    <main>
      <Breadcrumbs
        items={[
          { label: "Главная", href: "/" },
          { label: "Кабинет", href: "/dashboard" },
          { label: "Сообщения" },
        ]}
      />
      <div className="page-heading">
        <div>
          <p className="eyebrow">Общение</p>
          <h1>Сообщения</h1>
        </div>
      </div>
      <div className="message-grid">
        <aside aria-label="Диалоги">
          {!realtimeReady ? (
            <ConversationListSkeleton count={5} />
          ) : conversations.length === 0 ? (
            <div className="empty">Диалогов пока нет.</div>
          ) : (
            <ul className="msg-convos">
              {conversations.map((item) => (
                <li key={item.id}>
                  <button
                    type="button"
                    aria-current={active === item.id}
                    onClick={() => setActive(item.id)}
                  >
                    <span className="msg-convos__title">{convTitle(item)}</span>
                    <small>
                      {item.kind === "PROJECT"
                        ? item.counterparty_name
                          ? `Проект · ${item.counterparty_name}`
                          : "Диалог по проекту"
                        : "Личный диалог"}
                      {item.unread_count ? ` · ${item.unread_count} новых` : ""}
                    </small>
                  </button>
                </li>
              ))}
            </ul>
          )}
        </aside>
        <section className="msg-thread" aria-label="Активный диалог">
          {!active ? (
            !realtimeReady ? (
              <ChatMessagesSkeleton count={4} />
            ) : (
              <div className="empty">
                Выберите диалог, чтобы открыть переписку.
              </div>
            )
          ) : (
            <>
              {(() => { const conversation = activeConversation; const online = counterpartyOnline || Boolean(conversation?.counterparty_user_id && onlineUsers[conversation.counterparty_user_id]); const personHref = conversation?.counterparty_username ? (conversation.counterparty_role === "FREELANCER" ? `/freelancers/${conversation.counterparty_username}` : `/customers/${conversation.counterparty_username}`) : conversation?.counterparty_user_id ? `/profile/${conversation.counterparty_user_id}` : ""; return <header className="msg-thread__header"><div className="msg-thread__identity">{conversation?.project_id ? <><a className="msg-thread__project-link" href={`/projects/${conversation.project_id}`}>{conversation.project_title || "Проект"}</a>{personHref ? <a className="msg-thread__person-link" href={personHref}>{conversation.counterparty_name || "Собеседник"}</a> : null}</> : personHref ? <a className="msg-thread__person-link" href={personHref}>{conversation?.counterparty_name || "Собеседник"}</a> : <strong>Личный диалог</strong>}<small><span className={`presence-dot${online ? " is-online" : ""}`}/>{online ? "Онлайн" : "Не в сети"}</small></div></header>; })()}
              {deal ? (
                <aside className="msg-deal-context">
                  <span className="msg-deal-context__badge">Safe Deal</span>
                  <strong>
                    {new Intl.NumberFormat("ru-RU").format(
                      deal.gross_amount_kopecks / 100,
                    )}{" "}
                    ₽
                  </strong>
                  <a href={`/dashboard/safe-deals/${deal.id}`}>
                    Действия и история →
                  </a>
                </aside>
              ) : null}
              <ul className="msg-list" aria-live="polite">
                {messages.map((message) => {
                  const own = !!user && message.sender_user_id === user.id;
                  return (
                    <li id={`message-${message.id}`} key={message.id} className={own ? "is-own" : ""} onDoubleClick={() => startReply(message)} onMouseUp={() => { const selected = window.getSelection()?.toString().trim(); if (selected) startReply(message, selected); }} onPointerDown={(event) => { pointerStartX.current = event.clientX; longPressTimer.current = window.setTimeout(() => startReply(message), 550); }} onPointerMove={(event) => { if (pointerStartX.current !== null && Math.abs(event.clientX - pointerStartX.current) > 10 && longPressTimer.current) window.clearTimeout(longPressTimer.current); }} onPointerUp={(event) => { if (longPressTimer.current) window.clearTimeout(longPressTimer.current); if (pointerStartX.current !== null && Math.abs(event.clientX - pointerStartX.current) > 48) startReply(message); pointerStartX.current = null; }} onPointerCancel={() => { if (longPressTimer.current) window.clearTimeout(longPressTimer.current); pointerStartX.current = null; }}>
                      {message.deleted_at ? (
                        <em>Сообщение удалено</em>
                      ) : (
                        <>
                          {message.reply_to_message_id ? <button type="button" className="msg-reply-quote" onClick={() => scrollToMessage(message.reply_to_message_id!)}>{message.reply_quote || messages.find((item) => item.id === message.reply_to_message_id)?.body || "Вложение"}</button> : null}
                          <p>{message.body}</p>
                          {message.media_ids?.map((media) => (
                            <MessageAttachment messageID={message.id} mediaID={media} key={media}/>
                          ))}
                          <span className="msg-time"><button type="button" className="msg-reply-action" onClick={() => startReply(message)}>Ответить</button>{own ? <><button type="button" className="msg-reply-action" onClick={() => startEdit(message)}>Изменить</button><button type="button" className="msg-reply-action" onClick={() => void removeMessage(message)}>Удалить</button></> : null}
                            {fmtTime(message.created_at)}
                            {message.edited_at ? " · изменено" : ""}
                          </span>
                        </>
                      )}
                    </li>
                  );
                })}
              </ul>
              {typingUser ? <p className="msg-typing"><span/><span/><span/> Собеседник печатает…</p> : null}
              <form className="msg-composer" onSubmit={send}>
                {replyTo ? <div className="msg-composer__reply"><div><strong>Ответ с цитатой</strong><span>{replyQuote}</span></div><button type="button" aria-label="Отменить ответ" onClick={cancelReply}>×</button></div> : null}
                {editing ? <div className="msg-composer__reply"><div><strong>Редактирование сообщения</strong><span>{editing.body}</span></div><button type="button" aria-label="Отменить редактирование" onClick={() => { setEditing(null); setBody(""); }}>×</button></div> : null}
                <label>
                  Сообщение
                  <textarea
                    maxLength={10000}
                    value={body}
                    onChange={(e) => changeBody(e.target.value)}
                    placeholder="Напишите сообщение…"
                  />
                </label>
                <div className="msg-composer__row">
                  <div className="msg-composer__attachment">
                    <label className="msg-composer__file">
                      Вложение
                      <input
                        type="file"
                        onChange={(e) => setFile(e.target.files?.[0] ?? null)}
                      />
                    </label>
                    {file ? (
                      <div className="selected-file">
                        <FileTypeBadge name={file.name} mimeType={file.type} />
                        <span>
                          <strong>{file.name}</strong>
                          <small>
                            {Math.max(1, Math.round(file.size / 1024))} КБ
                          </small>
                        </span>
                        <button
                          type="button"
                          aria-label="Убрать вложение"
                          onClick={() => setFile(null)}
                        >
                          ×
                        </button>
                      </div>
                    ) : null}
                  </div>
                  <div className="inline-actions">
                    {!editing ? (recording ? <><span className="voice-recording-time">● {Math.floor(recordingSeconds / 60)}:{String(recordingSeconds % 60).padStart(2, "0")}</span><button type="button" className="button button--quiet" onClick={() => stopRecording(false)}>Завершить запись</button><button type="button" className="button button--quiet" onClick={() => stopRecording(true)}>Отменить</button></> : <button type="button" className="button button--quiet voice-record-button" onClick={() => void startRecording()} aria-label="Записать голосовое сообщение"><IconMic/> Голос</button>) : null}
                    <button
                      disabled={state === "sending" || state === "scanning" || recording}
                    >
                      {state === "scanning"
                        ? "Проверяем файл…"
                        : state === "sending"
                          ? "Отправляем…"
                          : editing ? "Сохранить" : "Отправить"}
                    </button>
                    {state === "failed" ? (
                      <button
                        type="button"
                        className="button button--quiet"
                        onClick={() => send()}
                      >
                        Повторить
                      </button>
                    ) : null}
                  </div>
                </div>
              </form>
              {error ? (
                <p role="alert" className="form-error">
                  {error}
                </p>
              ) : null}
            </>
          )}
        </section>
      </div>
    </main>
  );
}
