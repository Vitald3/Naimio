"use client";

import { FormEvent, ReactNode, useState } from "react";
import AdminReasonEditor from "./admin-reason-editor";
import FormattedText from "../formatted-text";
import { IconInbox } from "../icons";

export function AdminHeader({ eyebrow = "Администрирование", title, description, actions }: { eyebrow?: string; title: string; description?: string; actions?: ReactNode }) {
  return <div className="admin-page-header"><div><p className="eyebrow">{eyebrow}</p><h1>{title}</h1>{description ? <p>{description}</p> : null}</div>{actions ? <div className="admin-page-actions">{actions}</div> : null}</div>;
}

// The API emits stable English status codes; the admin UI renders them in
// Russian. Unknown codes fall back to a humanized form so no page ever regresses
// to a raw machine token like "AWAITING_FUNDING".
const statusLabels: Record<string, string> = {
  AWAITING_FUNDING: "Ожидает оплаты", FUNDED: "Оплачено", IN_PROGRESS: "В работе", SUBMITTED: "Работа передана",
  REVISION_REQUESTED: "На доработке", DISPUTED: "Спор", ACCEPTED: "Принято", RELEASE_PENDING: "Выплата",
  RELEASED: "Выплачено", COMPLETED: "Завершено", CANCEL_PENDING: "Отмена", CANCELED: "Отменено",
  REFUND_PENDING: "Возврат", REFUNDED: "Возврат выполнен", FAILED: "Ошибка", PENDING: "Ожидание",
  OPEN: "Открыт", EVIDENCE_COLLECTION: "Сбор материалов", UNDER_REVIEW: "На рассмотрении", RESOLVED: "Решён",
  RESOLVED_CUSTOMER: "Решён в пользу заказчика", RESOLVED_FREELANCER: "Решён в пользу исполнителя", RESOLVED_SPLIT: "Решён 50/50",
  DISMISSED: "Отклонён", ACTIVE: "Активен", SUSPENDED: "Ограничен", BANNED: "Заблокирован",
  VISIBLE: "Виден", HIDDEN: "Скрыт", PUBLISHED: "Опубликовано", DRAFT: "Черновик", CLOSED: "Закрыто",
  REJECTED: "Отклонено", VERIFIED: "Подтверждено", UNVERIFIED: "Не подтверждено", CONFIRMED: "Подтверждено",
  IN_REVIEW: "На проверке", REVIEWING: "На проверке", SUCCEEDED: "Успешно",
  PAST_DUE: "Просрочена оплата", EXPIRED: "Истекло", SCHEDULED: "Запланировано", ARCHIVED: "В архиве",
  SANDBOX: "Sandbox", PRODUCTION: "Production", ENABLED: "Включён", DISABLED: "Выключен", NOT_CONFIGURED: "Не настроен", READY: "Готов",
};
export const statusText = (value?: string) => value ? (statusLabels[value.toUpperCase()] ?? value.replaceAll("_", " ")) : "—";
export function StatusPill({ value }: { value: string }) {
  const normalized = (value || "UNKNOWN").toUpperCase();
  const positive = ["ACTIVE","VISIBLE","VERIFIED","PUBLISHED","COMPLETED","RESOLVED","RESOLVED_CUSTOMER","RESOLVED_FREELANCER","SUCCEEDED","FUNDED","RELEASED"].includes(normalized);
  const negative = ["BANNED","HIDDEN","REJECTED","FAILED","DISMISSED","CANCELED","REFUNDED"].includes(normalized);
  const warning = ["SUSPENDED","PENDING","OPEN","IN_REVIEW","REVIEWING","CONFIRMED","DISPUTED","UNDER_REVIEW","AWAITING_FUNDING"].includes(normalized);
  const className = positive ? "status-pill status-pill--positive" : negative ? "status-pill status-pill--negative" : warning ? "status-pill status-pill--warning" : "status-pill";
  return <span className={className}>{statusText(normalized)}</span>;
}

import {
  AdminCalculatorsSkeleton,
  AdminCmsSkeleton,
  AdminDetailSkeleton,
  AdminFeesSkeleton,
  AdminMetricsSkeleton,
  AdminMonetizationSkeleton,
  AdminPaymentRoutingSkeleton,
  AdminSettingsSkeleton,
  AdminTableRowsSkeleton,
  AdminTableSkeleton,
  AdminTaxonomySkeleton,
} from "../skeletons";

export {
  AdminCalculatorsSkeleton,
  AdminCmsSkeleton,
  AdminDetailSkeleton,
  AdminFeesSkeleton,
  AdminMetricsSkeleton,
  AdminMonetizationSkeleton,
  AdminPaymentRoutingSkeleton,
  AdminSettingsSkeleton,
  AdminTableRowsSkeleton,
  AdminTableSkeleton,
  AdminTaxonomySkeleton,
};

export function AdminTable({
  columns,
  children,
  empty,
  loading,
  rows = 6,
}: {
  columns: string[];
  children?: ReactNode;
  empty?: boolean;
  loading?: boolean;
  rows?: number;
}) {
  if (loading) {
    return <AdminTableSkeleton columns={columns} rowCount={rows} />;
  }
  if (empty) {
    return (
      <div className="empty admin-empty">
        <span className="empty__icon" aria-hidden="true">
          <IconInbox size={28} />
        </span>
        <strong>Нет записей по выбранным условиям</strong>
        <p>Измените фильтры или вернитесь позже.</p>
      </div>
    );
  }
  return (
    <div className="admin-table-wrap">
      <table className="admin-table">
        <thead>
          <tr>
            {columns.map((column) => (
              <th key={column}>{column}</th>
            ))}
          </tr>
        </thead>
        <tbody>{children}</tbody>
      </table>
    </div>
  );
}

export function AdminError({ message, onRetry }: { message: string; onRetry?: () => void }) {
  return <div className="error"><strong>Не удалось загрузить раздел</strong><p>{message}</p>{onRetry ? <button className="button button--quiet" onClick={onRetry}>Повторить</button> : null}</div>;
}

export function AdminLoading({
  columns = ["Объект", "Статус", "Дата", "Действия"],
  rows = 6,
}: {
  columns?: string[];
  rows?: number;
} = {}) {
  return <AdminTableSkeleton columns={columns} rowCount={rows} />;
}

export function AdminToolbar({ children }: { children: ReactNode }) {
  return <div className="admin-toolbar">{children}</div>;
}

export function AdminPager({ hasMore, loading, onMore }: { hasMore: boolean; loading?: boolean; onMore: () => void }) {
  if (!hasMore) return null;
  return <div className="admin-pager"><button className="button button--quiet" disabled={loading} onClick={onMore}>{loading ? "Загружаем…" : "Показать ещё"}</button></div>;
}

export function AdminReasonAction({ label, title, description, tone = "default", minLength = 3, onConfirm }: { label: string; title: string; description?: string; tone?: "default" | "danger"; minLength?: number; onConfirm: (reason: string) => Promise<void> | void }) {
  const [open, setOpen] = useState(false);
  const [reason, setReason] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  async function submit(event: FormEvent) {
    event.preventDefault();
    if (reason.trim().length < minLength) return;
    setBusy(true); setError("");
    try { await onConfirm(reason.trim()); setOpen(false); setReason(""); }
    catch (e) { setError(e instanceof Error ? e.message : "Не удалось выполнить действие"); }
    finally { setBusy(false); }
  }
  return <>
    <button type="button" className={tone === "danger" ? "button button--danger button--compact" : "button button--quiet button--compact"} onClick={() => setOpen(true)}>{label}</button>
    {open ? <div className="admin-modal-backdrop" role="presentation" onMouseDown={(e) => { if (e.currentTarget === e.target && !busy) setOpen(false); }}>
      <section className="admin-modal" role="dialog" aria-modal="true" aria-label={title}>
        <h2>{title}</h2>{description ? <p>{description}</p> : null}
        <form onSubmit={submit}><label>Причина<AdminReasonEditor autoFocus value={reason} onChange={setReason}/></label>{error ? <p className="form-error" role="alert">{error}</p> : null}<div className="admin-modal__actions"><button type="button" className="button button--quiet" disabled={busy} onClick={() => setOpen(false)}>Отмена</button><button type="submit" className={tone === "danger" ? "button button--danger" : "button"} disabled={busy}>{busy ? "Выполняем…" : "Подтвердить"}</button></div></form>
      </section>
    </div> : null}
  </>;
}

export async function adminRequest<T>(url: string, init?: RequestInit): Promise<T> {
  const response = await fetch(url, { credentials: "same-origin", cache: "no-store", ...init, headers: { "Content-Type": "application/json", ...(init?.headers || {}) } });
  if (!response.ok) {
    let message = `HTTP ${response.status}`;
    let code = "";
    try {
      const body = await response.json();
      message = body?.error?.message || message;
      code = body?.error?.code || "";
    } catch {}
    const localized: Record<string, string> = {
      "admin permission required": "Недостаточно прав для этого раздела.",
      "admin resource not found": "Раздел или запись не найдены.",
      "invalid admin operation": "Проверьте заполненные данные.",
      "admin operation conflicts with current state": "Действие недоступно в текущем состоянии.",
      "invalid content data": "Некорректные данные материала. Проверьте заголовок, slug и обложку.",
      "content not found": "Материал не найден.",
      "content conflicts with an existing record or state": "Запись с таким slug уже существует или находится в несовместимом состоянии.",
      "invalid upload payload": "Некорректные параметры загрузки файла.",
      "upload payload is too large": "Файл превышает допустимый размер.",
      "uploaded object does not match the presign request": "Загруженный файл не прошел проверку формата или размера.",
      "authentication required": "Требуется авторизация администратора.",
      "method not allowed": "Метод запроса не поддерживается.",
      "request could not be completed": "Внутренняя ошибка сервера. Попробуйте снова.",
    };
    if (localized[message]) {
      message = localized[message];
    } else if (code === "VALIDATION_ERROR" && message === `HTTP ${response.status}`) {
      message = "Проверьте правильность заполнения полей.";
    } else if (code === "FORBIDDEN") {
      message = "Недостаточно прав для выполнения этого действия.";
    } else if (code === "CONFLICT") {
      message = "Запись с такими параметрами уже существует.";
    }
    throw new Error(message);
  }
  if (response.status === 204) return undefined as T;
  return response.json() as Promise<T>;
}

// Dispute reason codes are stable English enums from the API; render them in Russian.
const disputeReasons: Record<string, string> = {
  WORK_NOT_DELIVERED: "Работа не передана", WORK_DOES_NOT_MATCH_SCOPE: "Результат не соответствует заданию",
  CUSTOMER_UNRESPONSIVE: "Заказчик не отвечает", FREELANCER_UNRESPONSIVE: "Исполнитель не отвечает",
  QUALITY_DISPUTE: "Претензии к качеству", OTHER: "Другое",
};
export const disputeReasonText = (code?: string) => code ? (disputeReasons[code.toUpperCase()] ?? code.replaceAll("_", " ").toLowerCase()) : "—";
const entities:Record<string,string>={PROJECT:"Проект",SERVICE:"Услуга",VACANCY:"Вакансия",REVIEW:"Отзыв",USER:"Пользователь"};
const reportReasons:Record<string,string>={SUSPICIOUS_SCOPE:"Спорные условия",PERSONAL_DATA:"Персональные данные",SPAM:"Спам",FRAUD:"Мошенничество",ABUSE:"Оскорбления",OTHER:"Другое"};
const fraudSignals:Record<string,string>={PROPOSAL_VELOCITY:"Необычная частота откликов",REFERRAL_PATTERN:"Подозрительная реферальная активность",PAYMENT_PATTERN:"Подозрительная платёжная активность",ACCOUNT_LINKAGE:"Связанные аккаунты"};
export const entityText=(value?:string)=>value?(entities[value.toUpperCase()]??value.replaceAll("_"," ").toLowerCase()):"—";
export const reportReasonText=(value?:string)=>value?(reportReasons[value.toUpperCase()]??value.replaceAll("_"," ").toLowerCase()):"—";
export const fraudSignalText=(value?:string)=>value?(fraudSignals[value.toUpperCase()]??value.replaceAll("_"," ").toLowerCase()):"—";
const roles:Record<string,string>={CUSTOMER:"Заказчик",FREELANCER:"Исполнитель",MODERATOR:"Модератор",ADMIN:"Администратор",SUPER_ADMIN:"Суперадминистратор"};
export const roleText=(value?:string)=>value?(roles[value.toUpperCase()]??value.replaceAll("_"," ").toLowerCase()):"—";
export const capabilityText=(value?:string)=>value?(roles[value.toUpperCase()]??value.replaceAll("_"," ").toLowerCase()):"—";
export function EvidenceSummary({value}:{value?:Record<string,unknown>}){const entries=Object.entries(value||{});if(!entries.length)return <small>Дополнительных данных нет</small>;const labels:Record<string,string>={count:"Количество",window:"Период",accounts:"Аккаунты",note:"Примечание",screenshot_ref:"Материал"};return <dl className="admin-dl admin-dl--compact">{entries.map(([key,item])=><div key={key}><dt>{labels[key]||key.replaceAll("_"," ")}</dt><dd>{String(item)}</dd></div>)}</dl>}
export const formatDate = (value?: string) => value ? new Intl.DateTimeFormat("ru-RU", { dateStyle: "medium", timeStyle: "short" }).format(new Date(value)) : "—";
export const formatMoney = (kopecks?: number) => kopecks === undefined ? "—" : `${new Intl.NumberFormat("ru-RU").format(kopecks / 100)} ₽`;
export const compactID = (value?: string) => value ? `${value.slice(0, 8)}…${value.slice(-4)}` : "—";

export function AdminRichText({value,html,className="admin-rich-text"}:{value?:string;html?:string;className?:string}){const content=value??html;if(!content)return null;return <div className={className}><FormattedText value={content}/></div>}
export function adminEntityHref(type?:string,id?:string){
  if(!type||!id)return "";
  switch(type.toUpperCase()){
    case "PROJECT":return `/x7m4q9k2/projects/${encodeURIComponent(id)}`;
    case "SERVICE":return `/x7m4q9k2/services/${encodeURIComponent(id)}`;
    case "VACANCY":return `/x7m4q9k2/vacancies/${encodeURIComponent(id)}`;
    case "USER":return `/x7m4q9k2/users/${id}`;
    case "REVIEW":return `/x7m4q9k2/reviews?q=${encodeURIComponent(id)}`;
    default:return "";
  }
}
export function AdminEntityLink({type,id,entityType,entityID,label,newTab=true}:{type?:string;id?:string;entityType?:string;entityID?:string;label?:string;newTab?:boolean}){
  const resolvedType=type??entityType;const resolvedID=id??entityID;
  const href=adminEntityHref(resolvedType,resolvedID);
  if(!href)return <span>{label||compactID(resolvedID)}</span>;
  return <a className="admin-primary-link" href={href} target={newTab?"_blank":undefined} rel={newTab?"noopener noreferrer":undefined}>{label||compactID(resolvedID)}</a>;
}
