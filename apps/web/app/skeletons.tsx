"use client";

import React, { CSSProperties, ReactNode } from "react";

export type SkeletonProps = {
  className?: string;
  width?: string | number;
  height?: string | number;
  minHeight?: string | number;
  maxHeight?: string | number;
  minWidth?: string | number;
  maxWidth?: string | number;
  circle?: boolean;
  rounded?: "sm" | "md" | "lg" | "xl" | "full" | "none";
  style?: CSSProperties;
  children?: ReactNode;
  "aria-label"?: string;
};

export function Skeleton({
  className = "",
  width,
  height,
  minHeight,
  maxHeight,
  minWidth,
  maxWidth,
  circle = false,
  rounded,
  style,
  children,
  "aria-label": ariaLabel,
}: SkeletonProps) {
  const roundedClass = circle
    ? "skeleton--circle"
    : rounded === "full"
    ? "skeleton--rounded-full"
    : rounded === "lg"
    ? "skeleton--rounded-lg"
    : rounded === "xl"
    ? "skeleton--rounded-xl"
    : rounded === "sm"
    ? "skeleton--rounded-sm"
    : rounded === "none"
    ? "skeleton--rounded-none"
    : "";

  const customStyle: CSSProperties = {
    ...style,
    ...(width !== undefined ? { width: typeof width === "number" ? `${width}px` : width } : {}),
    ...(height !== undefined ? { height: typeof height === "number" ? `${height}px` : height } : {}),
    ...(minHeight !== undefined ? { minHeight: typeof minHeight === "number" ? `${minHeight}px` : minHeight } : {}),
    ...(maxHeight !== undefined ? { maxHeight: typeof maxHeight === "number" ? `${maxHeight}px` : maxHeight } : {}),
    ...(minWidth !== undefined ? { minWidth: typeof minWidth === "number" ? `${minWidth}px` : minWidth } : {}),
    ...(maxWidth !== undefined ? { maxWidth: typeof maxWidth === "number" ? `${maxWidth}px` : maxWidth } : {}),
  };

  return (
    <div
      className={`skeleton ${roundedClass} ${className}`.trim()}
      style={customStyle}
      aria-hidden={!ariaLabel}
      aria-label={ariaLabel}
    >
      {children}
    </div>
  );
}

export function SkeletonText({
  lines = 3,
  widths = ["100%", "92%", "68%"],
  className = "",
  size = "md",
  style,
}: {
  lines?: number;
  widths?: string[];
  className?: string;
  size?: "sm" | "md" | "lg";
  style?: CSSProperties;
}) {
  const sizeClass = size === "sm" ? "skeleton--text-sm" : size === "lg" ? "skeleton--text-lg" : "skeleton--text";
  return (
    <div className={`skeleton-text-group ${className}`.trim()} style={style} aria-hidden="true">
      {Array.from({ length: lines }).map((_, idx) => (
        <Skeleton
          key={idx}
          className={`${sizeClass}`}
          width={widths[idx % widths.length] || "100%"}
        />
      ))}
    </div>
  );
}

export function SkeletonAvatar({
  size = "md",
  className = "",
  style,
}: {
  size?: "sm" | "md" | "lg" | "xl" | number;
  className?: string;
  style?: CSSProperties;
}) {
  if (typeof size === "number") {
    return <Skeleton circle width={size} height={size} className={className} style={style} aria-hidden="true" />;
  }
  const sizeClass =
    size === "sm"
      ? "skeleton--avatar-sm"
      : size === "lg"
      ? "skeleton--avatar-lg"
      : size === "xl"
      ? "skeleton--avatar-xl"
      : "skeleton--avatar-md";
  return <Skeleton circle className={`${sizeClass} ${className}`.trim()} style={style} aria-hidden="true" />;
}

export function SkeletonButton({
  size = "default",
  width = 120,
  className = "",
  style,
}: {
  size?: "compact" | "default" | "lg";
  width?: string | number;
  className?: string;
  style?: CSSProperties;
}) {
  const sizeClass = size === "compact" ? "skeleton--button-sm" : size === "lg" ? "skeleton--button-lg" : "skeleton--button";
  return <Skeleton className={`${sizeClass} ${className}`.trim()} width={width} style={style} aria-hidden="true" />;
}

export function SkeletonBadge({
  width = 72,
  className = "",
  style,
}: {
  width?: string | number;
  className?: string;
  style?: CSSProperties;
}) {
  return <Skeleton className={`skeleton--badge ${className}`.trim()} width={width} style={style} aria-hidden="true" />;
}

export function SkeletonImage({
  aspectRatio = "16/9",
  className = "",
  width = "100%",
  height,
  rounded = "lg",
  style,
}: {
  aspectRatio?: string;
  className?: string;
  width?: string | number;
  height?: string | number;
  rounded?: "none" | "sm" | "md" | "lg" | "xl" | "full";
  style?: CSSProperties;
}) {
  const roundedClass =
    rounded === "none"
      ? ""
      : rounded === "sm"
      ? "skeleton--rounded-sm"
      : rounded === "md"
      ? "skeleton--rounded-md"
      : rounded === "xl"
      ? "skeleton--rounded-xl"
      : rounded === "full"
      ? "skeleton--rounded-full"
      : "skeleton--rounded-lg";

  return (
    <div
      className={`skeleton skeleton--image ${roundedClass} ${className}`.trim()}
      style={{
        width: typeof width === "number" ? `${width}px` : width,
        height: typeof height === "number" ? `${height}px` : height,
        aspectRatio: height ? undefined : aspectRatio,
        ...style,
      }}
      aria-hidden="true"
    />
  );
}

export function SkeletonCard({
  className = "",
  children,
  style,
}: {
  className?: string;
  children?: ReactNode;
  style?: CSSProperties;
}) {
  return (
    <article className={`skeleton-card ${className}`.trim()} style={style} aria-hidden="true">
      {children}
    </article>
  );
}

/* ==========================================================================
   DOMAIN-SPECIFIC SKELETON COMPOSITIONS
   ========================================================================== */

/**
 * Freelancer Card Skeleton
 * Mirrors: avatar, name, badge, title, presence, rating/experience/rate, skills chips, link
 */
export function FreelancerCardSkeleton() {
  return (
    <article className="profile-card profile-card--rich skeleton-card-item" aria-hidden="true">
      <div className="profile-card__top">
        <SkeletonAvatar size="lg" />
        <Skeleton width={32} height={32} circle />
      </div>
      <div className="skeleton-line-row" style={{ marginTop: 12, marginBottom: 8, display: "flex", alignItems: "center", gap: 8 }}>
        <Skeleton height={22} width="58%" rounded="md" />
        <SkeletonBadge width={48} />
      </div>
      <Skeleton height={15} width="80%" rounded="sm" style={{ marginBottom: 10 }} />
      <div style={{ display: "flex", gap: 12, marginBottom: 14, alignItems: "center" }}>
        <Skeleton height={14} width={70} rounded="sm" />
        <Skeleton height={14} width={85} rounded="sm" />
        <Skeleton height={14} width={95} rounded="sm" />
      </div>
      <div className="chip-row" style={{ marginBottom: 18 }}>
        <Skeleton height={24} width={64} rounded="full" />
        <Skeleton height={24} width={80} rounded="full" />
        <Skeleton height={24} width={72} rounded="full" />
      </div>
      <Skeleton height={16} width={110} rounded="sm" style={{ marginTop: "auto" }} />
    </article>
  );
}

export function FreelancersCatalogSkeleton({ count = 6 }: { count?: number }) {
  return (
    <ul className="freelancer-grid" aria-busy="true" aria-label="Загрузка каталога специалистов">
      {Array.from({ length: count }).map((_, i) => (
        <li key={i}>
          <FreelancerCardSkeleton />
        </li>
      ))}
    </ul>
  );
}

/**
 * Project Card Skeleton
 * Mirrors: category/experience eyebrow, title, plain description excerpt, budget/deadline/proposals facts, chips, link
 */
export function ProjectCardSkeleton() {
  return (
    <article className="list-card project-catalog-card skeleton-card-item" aria-hidden="true">
      <div className="card-corner-action">
        <Skeleton width={32} height={32} circle />
      </div>
      <Skeleton height={13} width={140} rounded="sm" style={{ marginBottom: 10 }} />
      <Skeleton height={22} width="75%" rounded="md" style={{ marginBottom: 12 }} />
      <SkeletonText lines={2} widths={["98%", "82%"]} size="sm" className="list-card__desc" />
      <div className="project-card-facts" style={{ marginTop: 14, marginBottom: 14 }}>
        <Skeleton height={18} width={110} rounded="sm" />
        <Skeleton height={18} width={120} rounded="sm" />
        <Skeleton height={18} width={90} rounded="sm" />
      </div>
      <div className="chip-row" style={{ marginBottom: 16 }}>
        <Skeleton height={24} width={60} rounded="full" />
        <Skeleton height={24} width={75} rounded="full" />
        <Skeleton height={24} width={68} rounded="full" />
      </div>
      <Skeleton height={15} width={120} rounded="sm" style={{ marginTop: "auto" }} />
    </article>
  );
}

export function ProjectsCatalogSkeleton({ count = 6, view = "grid" }: { count?: number; view?: "grid" | "list" }) {
  return (
    <ul className={`project-catalog-list project-catalog-list--${view}`} aria-busy="true" aria-label="Загрузка проектов">
      {Array.from({ length: count }).map((_, i) => (
        <li key={i}>
          <ProjectCardSkeleton />
        </li>
      ))}
    </ul>
  );
}

/**
 * Service Card Skeleton
 * Mirrors: cover/badge, category, title, seller with avatar & rating, price, link
 */
export function ServiceCardSkeleton() {
  return (
    <article className="service-card skeleton-card-item" aria-hidden="true">
      <div className="service-card__top" style={{ display: "flex", justifyContent: "space-between", marginBottom: 12 }}>
        <Skeleton height={14} width={100} rounded="sm" />
        <SkeletonBadge width={60} />
      </div>
      <Skeleton height={22} width="85%" rounded="md" style={{ marginBottom: 10 }} />
      <SkeletonText lines={2} widths={["95%", "70%"]} size="sm" style={{ marginBottom: 16 }} />
      <div className="service-card__seller" style={{ display: "flex", alignItems: "center", gap: 10, marginBottom: 14 }}>
        <SkeletonAvatar size="sm" />
        <div style={{ flex: 1 }}>
          <Skeleton height={14} width={100} rounded="sm" style={{ marginBottom: 4 }} />
          <Skeleton height={12} width={65} rounded="sm" />
        </div>
      </div>
      <div className="service-card__footer" style={{ display: "flex", justifyContent: "space-between", alignItems: "center", marginTop: "auto", paddingTop: 12, borderTop: "1px solid var(--line)" }}>
        <Skeleton height={20} width={90} rounded="sm" />
        <Skeleton height={14} width={75} rounded="sm" />
      </div>
    </article>
  );
}

export function ServicesCatalogSkeleton({ count = 6 }: { count?: number }) {
  return (
    <ul className="service-grid" aria-busy="true" aria-label="Загрузка каталога услуг">
      {Array.from({ length: count }).map((_, i) => (
        <li key={i}>
          <ServiceCardSkeleton />
        </li>
      ))}
    </ul>
  );
}

/**
 * Vacancy Card Skeleton
 * Mirrors: title, employer/company, employment/salary facts, tags, link
 */
export function VacancyCardSkeleton({ view = "list" }: { view?: "list" | "grid" }) {
  return (
    <article className={`vacancy-card skeleton-card-item ${view === "grid" ? "vacancy-card--grid" : ""}`} aria-hidden="true">
      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "flex-start", marginBottom: 8 }}>
        <div style={{ flex: 1 }}>
          <Skeleton height={13} width={110} rounded="sm" style={{ marginBottom: 8 }} />
          <Skeleton height={22} width="70%" rounded="md" style={{ marginBottom: 8 }} />
        </div>
        <Skeleton width={32} height={32} circle />
      </div>
      <div style={{ display: "flex", flexWrap: "wrap", gap: 12, marginBottom: 14, alignItems: "center" }}>
        <Skeleton height={18} width={120} rounded="sm" />
        <Skeleton height={18} width={90} rounded="sm" />
        <Skeleton height={18} width={100} rounded="sm" />
      </div>
      <SkeletonText lines={2} widths={["96%", "75%"]} size="sm" style={{ marginBottom: 14 }} />
      <div className="chip-row" style={{ marginBottom: 14 }}>
        <Skeleton height={24} width={65} rounded="full" />
        <Skeleton height={24} width={75} rounded="full" />
        <Skeleton height={24} width={70} rounded="full" />
      </div>
      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", marginTop: "auto" }}>
        <Skeleton height={14} width={90} rounded="sm" />
        <Skeleton height={15} width={120} rounded="sm" />
      </div>
    </article>
  );
}

export function VacanciesCatalogSkeleton({ count = 6, view = "list" }: { count?: number; view?: "list" | "grid" }) {
  return (
    <ul className={`vacancy-results vacancy-results--${view}`} aria-busy="true" aria-label="Загрузка вакансий">
      {Array.from({ length: count }).map((_, i) => (
        <li key={i}>
          <VacancyCardSkeleton view={view} />
        </li>
      ))}
    </ul>
  );
}

/**
 * Category Card Skeleton
 */
export function CategoryCardSkeleton({ index = 1 }: { index?: number }) {
  return (
    <div className="category-card skeleton-card-item" aria-hidden="true" style={{ pointerEvents: "none" }}>
      <span className="category-card__icon">
        <Skeleton width={24} height={24} rounded="sm" />
      </span>
      <span className="category-card__number">0{index}</span>
      <Skeleton height={20} width="65%" rounded="sm" style={{ margin: "10px 0 6px" }} />
      <Skeleton height={13} width="85%" rounded="sm" />
      <b aria-hidden="true" style={{ opacity: 0.3 }}>↗</b>
    </div>
  );
}

export function CategoriesCatalogSkeleton({ count = 8 }: { count?: number }) {
  return (
    <ul className="category-grid" aria-busy="true" aria-label="Загрузка категорий">
      {Array.from({ length: count }).map((_, i) => (
        <li key={i}>
          <CategoryCardSkeleton index={i + 1} />
        </li>
      ))}
    </ul>
  );
}

/**
 * Education Card Skeleton
 */
export function EducationCardSkeleton() {
  return (
    <article className="education-card service-card skeleton-card-item" aria-hidden="true">
      <div style={{ display: "flex", justifyContent: "space-between", marginBottom: 12 }}>
        <SkeletonBadge width={80} />
        <SkeletonBadge width={65} />
      </div>
      <Skeleton height={22} width="80%" rounded="md" style={{ marginBottom: 10 }} />
      <SkeletonText lines={2} widths={["95%", "75%"]} size="sm" style={{ marginBottom: 14 }} />
      <div style={{ display: "flex", gap: 12, marginBottom: 14 }}>
        <Skeleton height={14} width={75} rounded="sm" />
        <Skeleton height={14} width={85} rounded="sm" />
      </div>
      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", marginTop: "auto", paddingTop: 12, borderTop: "1px solid var(--line)" }}>
        <Skeleton height={20} width={95} rounded="sm" />
        <Skeleton height={14} width={80} rounded="sm" />
      </div>
    </article>
  );
}

export function EducationCatalogSkeleton({ count = 6 }: { count?: number }) {
  return (
    <ul className="education-grid" aria-busy="true" aria-label="Загрузка обучающих материалов">
      {Array.from({ length: count }).map((_, i) => (
        <li key={i}>
          <EducationCardSkeleton />
        </li>
      ))}
    </ul>
  );
}

/**
 * Blog Card Skeleton
 */
export function BlogCardSkeleton() {
  return (
    <article className="blog-card skeleton-card-item" aria-hidden="true">
      <SkeletonImage aspectRatio="16/9" rounded="lg" style={{ marginBottom: 16 }} />
      <div style={{ display: "flex", gap: 10, marginBottom: 10 }}>
        <SkeletonBadge width={70} />
        <Skeleton height={13} width={80} rounded="sm" />
      </div>
      <Skeleton height={24} width="88%" rounded="md" style={{ marginBottom: 10 }} />
      <SkeletonText lines={2} widths={["98%", "80%"]} size="sm" style={{ marginBottom: 16 }} />
      <Skeleton height={15} width={90} rounded="sm" />
    </article>
  );
}

export function BlogCatalogSkeleton({ count = 4 }: { count?: number }) {
  return (
    <div className="blog-grid" aria-busy="true" aria-label="Загрузка статей блога">
      {Array.from({ length: count }).map((_, i) => (
        <BlogCardSkeleton key={i} />
      ))}
    </div>
  );
}

/**
 * Favorite Card Skeleton
 */
export function FavoriteCardSkeleton() {
  return (
    <article className="favorite-card skeleton-card-item" aria-hidden="true">
      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "flex-start", marginBottom: 12 }}>
        <SkeletonBadge width={85} />
        <Skeleton width={28} height={28} circle />
      </div>
      <Skeleton height={22} width="80%" rounded="md" style={{ marginBottom: 10 }} />
      <SkeletonText lines={2} widths={["95%", "70%"]} size="sm" style={{ marginBottom: 14 }} />
      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", marginTop: "auto" }}>
        <Skeleton height={18} width={90} rounded="sm" />
        <Skeleton height={14} width={70} rounded="sm" />
      </div>
    </article>
  );
}

export function FavoritesSkeleton({ count = 6 }: { count?: number }) {
  return (
    <div className="favorites-grid" aria-busy="true" aria-label="Загрузка избранного">
      {Array.from({ length: count }).map((_, i) => (
        <FavoriteCardSkeleton key={i} />
      ))}
    </div>
  );
}

/**
 * Proposal Card Skeleton (for customer project proposals view)
 * Mirrors: freelancer avatar, name, rating, status badge, message, price breakdown, delivery time, action buttons
 */
export function ProposalCardSkeleton() {
  return (
    <li className="record skeleton-card-item" aria-hidden="true">
      <div className="record__head" style={{ display: "flex", justifyContent: "space-between", alignItems: "center", marginBottom: 12 }}>
        <div style={{ display: "flex", alignItems: "center", gap: 12 }}>
          <SkeletonAvatar size="md" />
          <div>
            <Skeleton height={18} width={130} rounded="sm" style={{ marginBottom: 4 }} />
            <Skeleton height={13} width={90} rounded="sm" />
          </div>
        </div>
        <SkeletonBadge width={90} />
      </div>
      <SkeletonText lines={3} widths={["98%", "94%", "60%"]} size="md" className="record__body" style={{ marginBottom: 16 }} />
      <div className="proposal-money" style={{ padding: "14px 16px", borderRadius: 12, background: "var(--surface-2)", marginBottom: 14, display: "grid", gap: 6 }}>
        <div style={{ display: "flex", justifyContent: "space-between" }}>
          <Skeleton height={14} width={160} rounded="sm" />
          <Skeleton height={14} width={90} rounded="sm" />
        </div>
        <div style={{ display: "flex", justifyContent: "space-between" }}>
          <Skeleton height={14} width={140} rounded="sm" />
          <Skeleton height={14} width={70} rounded="sm" />
        </div>
        <div style={{ display: "flex", justifyContent: "space-between", marginTop: 4, paddingTop: 6, borderTop: "1px solid var(--line)" }}>
          <Skeleton height={16} width={170} rounded="sm" />
          <Skeleton height={18} width={100} rounded="sm" />
        </div>
      </div>
      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", marginTop: 12 }}>
        <Skeleton height={14} width={80} rounded="sm" />
        <div className="inline-actions" style={{ display: "flex", gap: 8 }}>
          <SkeletonButton size="compact" width={90} />
          <SkeletonButton size="compact" width={90} />
          <SkeletonButton size="compact" width={140} />
        </div>
      </div>
    </li>
  );
}

export function ProposalsListSkeleton({ count = 3 }: { count?: number }) {
  return (
    <ul className="record-list" aria-busy="true" aria-label="Загрузка откликов">
      {Array.from({ length: count }).map((_, i) => (
        <ProposalCardSkeleton key={i} />
      ))}
    </ul>
  );
}

/**
 * Safe Deal Skeleton
 * Mirrors: hero header, status badge, 6-step timeline, full quote breakdown, financial status, action sidebar
 */
export function SafeDealSkeleton() {
  return (
    <div className="deal-page-skeleton" aria-busy="true" aria-label="Загрузка условий Безопасной сделки">
      <header className="deal-hero" style={{ marginBottom: 24 }}>
        <div style={{ flex: 1 }}>
          <Skeleton height={13} width={150} rounded="sm" style={{ marginBottom: 10 }} />
          <Skeleton height={32} width="65%" rounded="md" style={{ marginBottom: 10 }} />
          <Skeleton height={16} width="40%" rounded="sm" />
        </div>
        <SkeletonBadge width={110} />
      </header>
      <div className="deal-layout">
        <div className="deal-main" style={{ display: "grid", gap: 20 }}>
          <section className="deal-panel">
            <Skeleton height={20} width={120} rounded="sm" style={{ marginBottom: 16 }} />
            <div className="deal-timeline" style={{ display: "grid", gridTemplateColumns: "repeat(6, 1fr)", gap: 12, padding: "10px 0" }}>
              {Array.from({ length: 6 }).map((_, i) => (
                <div key={i} style={{ display: "flex", flexDirection: "column", alignItems: "center", gap: 8 }}>
                  <Skeleton width={24} height={24} circle />
                  <Skeleton height={11} width="85%" rounded="sm" />
                </div>
              ))}
            </div>
          </section>

          <section className="deal-panel" style={{ padding: 24 }}>
            <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", marginBottom: 16 }}>
              <Skeleton height={20} width={140} rounded="sm" />
              <Skeleton height={24} width={120} rounded="md" />
            </div>
            <div style={{ display: "grid", gap: 12, padding: "12px 0", borderTop: "1px solid var(--line)", borderBottom: "1px solid var(--line)" }}>
              <div style={{ display: "flex", justifyContent: "space-between" }}>
                <Skeleton height={14} width={130} rounded="sm" />
                <Skeleton height={14} width={80} rounded="sm" />
              </div>
              <div style={{ display: "flex", justifyContent: "space-between" }}>
                <Skeleton height={14} width={160} rounded="sm" />
                <Skeleton height={14} width={75} rounded="sm" />
              </div>
              <div style={{ display: "flex", justifyContent: "space-between" }}>
                <Skeleton height={14} width={150} rounded="sm" />
                <Skeleton height={14} width={70} rounded="sm" />
              </div>
            </div>
            <Skeleton height={13} width="80%" rounded="sm" style={{ marginTop: 12 }} />
          </section>

          <section className="deal-panel">
            <Skeleton height={20} width={160} rounded="sm" style={{ marginBottom: 10 }} />
            <SkeletonText lines={2} widths={["95%", "65%"]} size="sm" />
          </section>
        </div>

        <aside className="deal-actions" style={{ padding: 24 }}>
          <Skeleton height={22} width={160} rounded="md" style={{ marginBottom: 16 }} />
          <SkeletonButton width="100%" size="lg" style={{ marginBottom: 12 }} />
          <SkeletonButton width="100%" size="default" style={{ marginBottom: 16 }} />
          <SkeletonText lines={2} widths={["100%", "85%"]} size="sm" />
        </aside>
      </div>
    </div>
  );
}

/**
 * Messenger Skeletons
 */
export function ConversationListSkeleton({ count = 5 }: { count?: number }) {
  return (
    <ul className="msg-convos" aria-busy="true" aria-label="Загрузка диалогов">
      {Array.from({ length: count }).map((_, i) => (
        <li key={i} style={{ padding: "14px 16px", borderBottom: "1px solid var(--line)" }}>
          <div style={{ display: "flex", alignItems: "center", gap: 12 }}>
            <SkeletonAvatar size="sm" />
            <div style={{ flex: 1 }}>
              <Skeleton height={16} width="70%" rounded="sm" style={{ marginBottom: 6 }} />
              <Skeleton height={12} width="90%" rounded="sm" />
            </div>
          </div>
        </li>
      ))}
    </ul>
  );
}

export function ChatMessagesSkeleton({ count = 6 }: { count?: number }) {
  return (
    <ul className="msg-list" aria-busy="true" aria-label="Загрузка сообщений" style={{ padding: "20px 0" }}>
      {Array.from({ length: count }).map((_, i) => {
        const isOwn = i % 2 === 1;
        return (
          <li key={i} className={isOwn ? "is-own" : ""} style={{ marginBottom: 14 }}>
            <div
              style={{
                maxWidth: "75%",
                marginLeft: isOwn ? "auto" : undefined,
                padding: "12px 16px",
                borderRadius: 16,
                background: isOwn ? "var(--brand-soft)" : "var(--surface-2)",
                display: "grid",
                gap: 6,
              }}
            >
              <Skeleton height={14} width={isOwn ? 140 : 220} rounded="sm" />
              {i % 3 === 0 ? <Skeleton height={14} width={180} rounded="sm" /> : null}
              <div style={{ display: "flex", justifyContent: isOwn ? "flex-end" : "flex-start", marginTop: 4 }}>
                <Skeleton height={11} width={45} rounded="sm" />
              </div>
            </div>
          </li>
        );
      })}
    </ul>
  );
}

export function MessengerSkeleton() {
  return (
    <div className="message-grid" aria-busy="true" aria-label="Загрузка сообщений">
      <aside aria-label="Диалоги">
        <ConversationListSkeleton count={5} />
      </aside>
      <section className="msg-thread">
        <header className="msg-thread__header" style={{ padding: "14px 20px" }}>
          <div style={{ display: "flex", alignItems: "center", gap: 12 }}>
            <SkeletonAvatar size="sm" />
            <div>
              <Skeleton height={16} width={140} rounded="sm" style={{ marginBottom: 4 }} />
              <Skeleton height={12} width={70} rounded="sm" />
            </div>
          </div>
        </header>
        <ChatMessagesSkeleton count={5} />
      </section>
    </div>
  );
}

/**
 * Notifications Skeleton
 */
export function NotificationRowSkeleton() {
  return (
    <li className="notif-item skeleton-card-item" aria-hidden="true">
      <div className="notif-item__body" style={{ display: "grid", gap: 6 }}>
        <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
          <Skeleton width={18} height={18} circle />
          <Skeleton height={16} width={180} rounded="sm" />
        </div>
        <Skeleton height={13} width="85%" rounded="sm" />
        <Skeleton height={11} width={100} rounded="sm" style={{ marginTop: 4 }} />
      </div>
      <div className="notif-item__actions" style={{ display: "flex", gap: 8, alignItems: "center" }}>
        <SkeletonButton size="compact" width={80} />
      </div>
    </li>
  );
}

export function NotificationsListSkeleton({ count = 5 }: { count?: number }) {
  return (
    <ul className="notif-list" aria-busy="true" aria-label="Загрузка уведомлений">
      {Array.from({ length: count }).map((_, i) => (
        <NotificationRowSkeleton key={i} />
      ))}
    </ul>
  );
}

/**
 * Portfolio Grid Skeleton
 */
export function PortfolioCardSkeleton() {
  return (
    <article className="portfolio-card skeleton-card-item" aria-hidden="true">
      <SkeletonImage aspectRatio="4/3" rounded="lg" style={{ marginBottom: 12 }} />
      <Skeleton height={18} width="80%" rounded="sm" style={{ marginBottom: 6 }} />
      <Skeleton height={13} width="95%" rounded="sm" />
    </article>
  );
}

export function PortfolioGridSkeleton({ count = 6 }: { count?: number }) {
  return (
    <div className="portfolio-grid" aria-busy="true" aria-label="Загрузка портфолио">
      {Array.from({ length: count }).map((_, i) => (
        <PortfolioCardSkeleton key={i} />
      ))}
    </div>
  );
}

/**
 * Pricing Plans Skeleton (/pro)
 */
export function PricingPlansSkeleton({ count = 2 }: { count?: number }) {
  return (
    <div className="pro-plans" aria-busy="true" aria-label="Загрузка тарифов">
      {Array.from({ length: count }).map((_, i) => (
        <article key={i} className="skeleton-card-item" style={{ padding: 28, display: "grid", gap: 14 }}>
          <SkeletonBadge width={80} />
          <Skeleton height={24} width="60%" rounded="md" />
          <Skeleton height={14} width="90%" rounded="sm" />
          <Skeleton height={32} width="70%" rounded="md" style={{ margin: "8px 0" }} />
          <div style={{ display: "grid", gap: 10, margin: "10px 0" }}>
            <Skeleton height={14} width="95%" rounded="sm" />
            <Skeleton height={14} width="88%" rounded="sm" />
            <Skeleton height={14} width="92%" rounded="sm" />
            <Skeleton height={14} width="75%" rounded="sm" />
          </div>
          <SkeletonButton width="100%" size="lg" style={{ marginTop: 12 }} />
        </article>
      ))}
    </div>
  );
}

/**
 * Dashboard Overview Skeleton
 */
export function DashboardOverviewSkeleton() {
  return (
    <div className="dashboard-skeleton" aria-busy="true" aria-label="Загрузка личного кабинета">
      <div className="page-heading" style={{ marginBottom: 28 }}>
        <div style={{ flex: 1 }}>
          <Skeleton height={13} width={130} rounded="sm" style={{ marginBottom: 10 }} />
          <Skeleton height={34} width="55%" rounded="md" style={{ marginBottom: 10 }} />
          <Skeleton height={15} width="75%" rounded="sm" />
        </div>
        <SkeletonButton width={160} size="default" />
      </div>
      <section style={{ marginBottom: 36 }}>
        <Skeleton height={22} width={140} rounded="sm" style={{ marginBottom: 18 }} />
        <ul className="dash-cards">
          {Array.from({ length: 3 }).map((_, i) => (
            <li key={i}>
              <article className="dash-card skeleton-card-item">
                <Skeleton height={12} width={80} rounded="sm" style={{ marginBottom: 8 }} />
                <Skeleton height={32} width={48} rounded="md" style={{ marginBottom: 8 }} />
                <Skeleton height={13} width="90%" rounded="sm" style={{ marginBottom: 14 }} />
                <Skeleton height={14} width={70} rounded="sm" />
              </article>
            </li>
          ))}
        </ul>
      </section>
      <section className="dash-split" style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 20 }}>
        <div className="skeleton-card-item" style={{ padding: 24, borderRadius: 18, border: "1px solid var(--line)" }}>
          <Skeleton height={22} width={180} rounded="sm" style={{ marginBottom: 10 }} />
          <Skeleton height={14} width="90%" rounded="sm" style={{ marginBottom: 18 }} />
          <SkeletonButton width={160} size="default" />
        </div>
        <div className="skeleton-card-item" style={{ padding: 24, borderRadius: 18, border: "1px solid var(--line)" }}>
          <Skeleton height={22} width={180} rounded="sm" style={{ marginBottom: 10 }} />
          <Skeleton height={14} width="90%" rounded="sm" style={{ marginBottom: 18 }} />
          <SkeletonButton width={160} size="default" />
        </div>
      </section>
    </div>
  );
}

/**
 * Detail Page Skeletons
 */
export function ProjectDetailSkeleton() {
  return (
    <div className="project-detail-skeleton" aria-busy="true" aria-label="Загрузка проекта">
      <div className="project-layout" style={{ display: "grid", gridTemplateColumns: "minmax(0, 1.4fr) minmax(300px, 0.6fr)", gap: 32 }}>
        <div className="project-main" style={{ display: "grid", gap: 24 }}>
          <div>
            <Skeleton height={13} width={140} rounded="sm" style={{ marginBottom: 12 }} />
            <Skeleton height={36} width="85%" rounded="md" style={{ marginBottom: 16 }} />
            <div style={{ display: "flex", gap: 16, alignItems: "center" }}>
              <Skeleton height={16} width={120} rounded="sm" />
              <Skeleton height={16} width={100} rounded="sm" />
              <Skeleton height={16} width={90} rounded="sm" />
            </div>
          </div>
          <div className="project-panel" style={{ padding: 28, borderRadius: 18, border: "1px solid var(--line)" }}>
            <Skeleton height={20} width={160} rounded="sm" style={{ marginBottom: 16 }} />
            <SkeletonText lines={6} widths={["100%", "98%", "94%", "97%", "85%", "60%"]} size="md" />
            <div className="chip-row" style={{ marginTop: 24 }}>
              <Skeleton height={26} width={75} rounded="full" />
              <Skeleton height={26} width={85} rounded="full" />
              <Skeleton height={26} width={68} rounded="full" />
              <Skeleton height={26} width={90} rounded="full" />
            </div>
          </div>
        </div>
        <aside className="project-sidebar" style={{ display: "grid", gap: 20 }}>
          <div style={{ padding: 24, borderRadius: 18, border: "1px solid var(--line)", background: "var(--surface-2)" }}>
            <Skeleton height={13} width={100} rounded="sm" style={{ marginBottom: 8 }} />
            <Skeleton height={32} width={160} rounded="md" style={{ marginBottom: 16 }} />
            <SkeletonButton width="100%" size="lg" style={{ marginBottom: 12 }} />
            <SkeletonButton width="100%" size="default" />
          </div>
          <div style={{ padding: 24, borderRadius: 18, border: "1px solid var(--line)" }}>
            <Skeleton height={18} width={130} rounded="sm" style={{ marginBottom: 14 }} />
            <div style={{ display: "flex", alignItems: "center", gap: 12, marginBottom: 12 }}>
              <SkeletonAvatar size="md" />
              <div>
                <Skeleton height={16} width={120} rounded="sm" style={{ marginBottom: 4 }} />
                <Skeleton height={12} width={80} rounded="sm" />
              </div>
            </div>
            <SkeletonText lines={2} widths={["90%", "65%"]} size="sm" />
          </div>
        </aside>
      </div>
    </div>
  );
}

export function FreelancerProfileDetailSkeleton() {
  return (
    <div className="profile-detail-skeleton" aria-busy="true" aria-label="Загрузка профиля">
      <header className="profile-hero" style={{ display: "flex", gap: 24, padding: 32, borderRadius: 24, border: "1px solid var(--line)", marginBottom: 28 }}>
        <SkeletonAvatar size="xl" />
        <div style={{ flex: 1, display: "grid", gap: 8 }}>
          <div style={{ display: "flex", alignItems: "center", gap: 12 }}>
            <Skeleton height={28} width={200} rounded="md" />
            <SkeletonBadge width={60} />
          </div>
          <Skeleton height={18} width="50%" rounded="sm" />
          <div style={{ display: "flex", gap: 16, marginTop: 6 }}>
            <Skeleton height={15} width={90} rounded="sm" />
            <Skeleton height={15} width={110} rounded="sm" />
            <Skeleton height={15} width={100} rounded="sm" />
          </div>
        </div>
      </header>
      <div className="profile-grid-layout" style={{ display: "grid", gridTemplateColumns: "1.4fr 0.6fr", gap: 28 }}>
        <div style={{ display: "grid", gap: 24 }}>
          <div style={{ padding: 24, borderRadius: 18, border: "1px solid var(--line)" }}>
            <Skeleton height={20} width={120} rounded="sm" style={{ marginBottom: 14 }} />
            <SkeletonText lines={4} widths={["100%", "96%", "92%", "70%"]} size="md" />
          </div>
          <div style={{ padding: 24, borderRadius: 18, border: "1px solid var(--line)" }}>
            <Skeleton height={20} width={140} rounded="sm" style={{ marginBottom: 16 }} />
            <PortfolioGridSkeleton count={4} />
          </div>
        </div>
        <aside style={{ display: "grid", gap: 20 }}>
          <div style={{ padding: 24, borderRadius: 18, border: "1px solid var(--line)", background: "var(--surface-2)" }}>
            <Skeleton height={24} width={130} rounded="md" style={{ marginBottom: 16 }} />
            <SkeletonButton width="100%" size="lg" style={{ marginBottom: 10 }} />
            <SkeletonButton width="100%" size="default" />
          </div>
        </aside>
      </div>
    </div>
  );
}

export function ServiceDetailSkeleton() {
  return (
    <div className="service-detail-skeleton" aria-busy="true" aria-label="Загрузка услуги">
      <div style={{ display: "grid", gridTemplateColumns: "1.3fr 0.7fr", gap: 32 }}>
        <div style={{ display: "grid", gap: 24 }}>
          <div>
            <Skeleton height={13} width={110} rounded="sm" style={{ marginBottom: 10 }} />
            <Skeleton height={32} width="85%" rounded="md" style={{ marginBottom: 16 }} />
            <div style={{ display: "flex", alignItems: "center", gap: 12, marginBottom: 20 }}>
              <SkeletonAvatar size="sm" />
              <Skeleton height={15} width={130} rounded="sm" />
              <Skeleton height={15} width={80} rounded="sm" />
            </div>
          </div>
          <div style={{ padding: 24, borderRadius: 18, border: "1px solid var(--line)" }}>
            <Skeleton height={20} width={140} rounded="sm" style={{ marginBottom: 14 }} />
            <SkeletonText lines={5} widths={["100%", "98%", "92%", "95%", "60%"]} size="md" />
          </div>
        </div>
        <aside style={{ display: "grid", gap: 20 }}>
          <div style={{ padding: 24, borderRadius: 18, border: "1px solid var(--line)", background: "var(--surface-2)" }}>
            <Skeleton height={13} width={90} rounded="sm" style={{ marginBottom: 8 }} />
            <Skeleton height={32} width={140} rounded="md" style={{ marginBottom: 18 }} />
            <SkeletonButton width="100%" size="lg" />
          </div>
        </aside>
      </div>
    </div>
  );
}

export function VacancyDetailSkeleton() {
  return (
    <div className="vacancy-detail-skeleton" aria-busy="true" aria-label="Загрузка вакансии">
      <div style={{ display: "grid", gridTemplateColumns: "1.3fr 0.7fr", gap: 32 }}>
        <div style={{ display: "grid", gap: 24 }}>
          <div>
            <Skeleton height={13} width={120} rounded="sm" style={{ marginBottom: 10 }} />
            <Skeleton height={34} width="80%" rounded="md" style={{ marginBottom: 14 }} />
            <div style={{ display: "flex", gap: 14 }}>
              <Skeleton height={18} width={130} rounded="sm" />
              <Skeleton height={18} width={100} rounded="sm" />
              <Skeleton height={18} width={90} rounded="sm" />
            </div>
          </div>
          <div style={{ padding: 24, borderRadius: 18, border: "1px solid var(--line)" }}>
            <Skeleton height={20} width={160} rounded="sm" style={{ marginBottom: 14 }} />
            <SkeletonText lines={6} widths={["100%", "97%", "94%", "96%", "88%", "65%"]} size="md" />
          </div>
        </div>
        <aside style={{ display: "grid", gap: 20 }}>
          <div style={{ padding: 24, borderRadius: 18, border: "1px solid var(--line)", background: "var(--surface-2)" }}>
            <Skeleton height={13} width={90} rounded="sm" style={{ marginBottom: 8 }} />
            <Skeleton height={30} width={150} rounded="md" style={{ marginBottom: 18 }} />
            <SkeletonButton width="100%" size="lg" />
          </div>
        </aside>
      </div>
    </div>
  );
}

/* ==========================================================================
   ADMIN CONTROL CENTER SKELETONS
   ========================================================================== */

/**
 * Admin Table Skeleton Rows
 * Renders multiple rows where each cell has a realistic skeleton bar
 * and action column has button skeleton.
 */
export function AdminTableRowsSkeleton({
  columns,
  rowCount = 6,
}: {
  columns: string[];
  rowCount?: number;
}) {
  return (
    <>
      {Array.from({ length: rowCount }).map((_, rowIndex) => (
        <tr key={rowIndex} className="admin-table-row-skeleton" aria-hidden="true">
          {columns.map((col, colIndex) => {
            const isLast = colIndex === columns.length - 1;
            const isFirst = colIndex === 0;
            return (
              <td key={colIndex}>
                {isLast && (!col || col === "Действия" || col === "Действие" || col === "Решение") ? (
                  <div className="admin-row-actions" style={{ display: "flex", gap: 6 }}>
                    <Skeleton height={28} width={76} rounded="full" />
                    {colIndex % 2 === 0 ? <Skeleton height={28} width={64} rounded="full" /> : null}
                  </div>
                ) : isFirst ? (
                  <div style={{ display: "grid", gap: 4 }}>
                    <Skeleton height={15} width="85%" rounded="sm" />
                    <Skeleton height={11} width="60%" rounded="sm" />
                  </div>
                ) : (
                  <Skeleton
                    height={14}
                    width={colIndex % 3 === 0 ? "70%" : colIndex % 3 === 1 ? "85%" : "60%"}
                    rounded="sm"
                  />
                )}
              </td>
            );
          })}
        </tr>
      ))}
    </>
  );
}

/**
 * Admin Table Skeleton Component
 * Renders complete table with the given column headers and skeleton rows.
 */
export function AdminTableSkeleton({
  columns,
  rowCount = 6,
}: {
  columns: string[];
  rowCount?: number;
}) {
  return (
    <div className="admin-table-wrap" aria-busy="true" aria-label="Загрузка таблицы">
      <table className="admin-table">
        <thead>
          <tr>
            {columns.map((col) => (
              <th key={col}>{col}</th>
            ))}
          </tr>
        </thead>
        <tbody>
          <AdminTableRowsSkeleton columns={columns} rowCount={rowCount} />
        </tbody>
      </table>
    </div>
  );
}

/**
 * Admin Detail Page Skeleton
 */
export function AdminDetailSkeleton() {
  return (
    <div className="admin-detail-skeleton" aria-busy="true" aria-label="Загрузка карточки объекта">
      <div className="admin-detail-grid">
        <section className="admin-panel" style={{ padding: 24 }}>
          <Skeleton height={22} width={180} rounded="sm" style={{ marginBottom: 18 }} />
          <dl className="admin-dl">
            {Array.from({ length: 6 }).map((_, i) => (
              <div key={i} style={{ display: "grid", gridTemplateColumns: "150px 1fr", gap: 20, padding: "10px 0" }}>
                <Skeleton height={14} width={100} rounded="sm" />
                <Skeleton height={14} width={i % 2 === 0 ? "65%" : "80%"} rounded="sm" />
              </div>
            ))}
          </dl>
        </section>
        <section className="admin-panel" style={{ padding: 24 }}>
          <Skeleton height={22} width={140} rounded="sm" style={{ marginBottom: 18 }} />
          <div style={{ display: "grid", gap: 12 }}>
            <Skeleton height={14} width="90%" rounded="sm" />
            <div style={{ display: "flex", gap: 8, margin: "8px 0" }}>
              <SkeletonBadge width={80} />
              <SkeletonBadge width={90} />
            </div>
            <div className="admin-stack-actions" style={{ display: "grid", gap: 8, marginTop: 12 }}>
              <SkeletonButton width="100%" size="default" />
              <SkeletonButton width="100%" size="default" />
            </div>
          </div>
        </section>
      </div>
    </div>
  );
}

/**
 * Admin Dashboard KPI Grid Skeleton
 */
export function AdminMetricsSkeleton({ count = 12 }: { count?: number }) {
  return (
    <div className="admin-kpi-grid" aria-busy="true" aria-label="Загрузка метрик">
      {Array.from({ length: count }).map((_, i) => (
        <div key={i} className="admin-kpi skeleton-card-item" style={{ pointerEvents: "none" }}>
          <Skeleton height={12} width={100} rounded="sm" style={{ marginBottom: 8 }} />
          <Skeleton height={32} width={50} rounded="md" style={{ marginBottom: 8 }} />
          <Skeleton height={11} width={120} rounded="sm" />
        </div>
      ))}
    </div>
  );
}

/**
 * Admin CMS / Blog Skeleton
 */
export function AdminCmsSkeleton() {
  return (
    <div className="cms-layout" aria-busy="true" aria-label="Загрузка материалов блога">
      <section className="admin-section">
        <Skeleton height={20} width={120} rounded="sm" style={{ marginBottom: 16 }} />
        <div className="cms-post-list">
          {Array.from({ length: 5 }).map((_, i) => (
            <div key={i} className="skeleton-card-item" style={{ padding: "14px 16px", borderBottom: "1px solid var(--border-subtle, rgba(0,0,0,0.06))" }}>
              <div style={{ display: "flex", justifyContent: "space-between", marginBottom: 8 }}>
                <Skeleton height={20} width={80} rounded="full" />
                <Skeleton height={12} width={90} rounded="sm" />
              </div>
              <Skeleton height={16} width={i % 2 === 0 ? "80%" : "65%"} rounded="sm" style={{ marginBottom: 6 }} />
              <Skeleton height={12} width={110} rounded="sm" />
            </div>
          ))}
        </div>
      </section>
      <section className="admin-section">
        <Skeleton height={20} width={110} rounded="sm" style={{ marginBottom: 16 }} />
        <div style={{ display: "grid", gap: 10, marginBottom: 16 }}>
          <Skeleton height={38} width="100%" rounded="md" />
          <Skeleton height={38} width="100%" rounded="md" />
          <Skeleton height={36} width={100} rounded="md" />
        </div>
        <div className="chip-row" style={{ display: "flex", flexWrap: "wrap", gap: 6 }}>
          {Array.from({ length: 6 }).map((_, i) => (
            <Skeleton key={i} height={26} width={60 + (i % 4) * 15} rounded="full" />
          ))}
        </div>
      </section>
      <section className="admin-section">
        <Skeleton height={20} width={80} rounded="sm" style={{ marginBottom: 16 }} />
        <div style={{ display: "grid", gap: 10, marginBottom: 16 }}>
          <Skeleton height={38} width="100%" rounded="md" />
          <Skeleton height={38} width="100%" rounded="md" />
          <Skeleton height={36} width={100} rounded="md" />
        </div>
        <div className="chip-row" style={{ display: "flex", flexWrap: "wrap", gap: 6 }}>
          {Array.from({ length: 8 }).map((_, i) => (
            <Skeleton key={i} height={26} width={50 + (i % 3) * 20} rounded="full" />
          ))}
        </div>
      </section>
    </div>
  );
}

/**
 * Admin Calculators Builder Skeleton
 */
export function AdminCalculatorsSkeleton() {
  return (
    <div className="calculator-admin-grid" aria-busy="true" aria-label="Загрузка калькуляторов">
      {Array.from({ length: 2 }).map((_, i) => (
        <div key={i} className="calculator-builder" style={{ pointerEvents: "none" }}>
          <header className="calculator-builder__header">
            <div style={{ display: "grid", gap: 6 }}>
              <Skeleton height={20} width={90} rounded="full" />
              <Skeleton height={22} width={240} rounded="sm" />
              <Skeleton height={14} width={140} rounded="sm" />
            </div>
            <Skeleton height={24} width={160} rounded="full" />
          </header>
          <section className="calculator-builder__section">
            <Skeleton height={16} width={90} rounded="sm" style={{ marginBottom: 12 }} />
            <div className="field-row" style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 12, marginBottom: 12 }}>
              <Skeleton height={40} width="100%" rounded="md" />
              <Skeleton height={40} width="100%" rounded="md" />
            </div>
            <Skeleton height={60} width="100%" rounded="md" />
          </section>
          <section className="calculator-builder__section">
            <Skeleton height={16} width={120} rounded="sm" style={{ marginBottom: 12 }} />
            <div className="field-row" style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 12, marginBottom: 12 }}>
              <Skeleton height={40} width="100%" rounded="md" />
              <Skeleton height={40} width="100%" rounded="md" />
            </div>
            <div className="field-row" style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 12 }}>
              <Skeleton height={40} width="100%" rounded="md" />
              <Skeleton height={40} width="100%" rounded="md" />
            </div>
          </section>
        </div>
      ))}
    </div>
  );
}

/**
 * Admin Settings Skeleton
 */
export function AdminSettingsSkeleton() {
  return (
    <div className="admin-settings-skeleton" aria-busy="true" aria-label="Загрузка настроек">
      <div className="settings-tabs" style={{ display: "flex", gap: 8, marginBottom: 24, overflowX: "auto" }}>
        {Array.from({ length: 6 }).map((_, i) => (
          <Skeleton key={i} height={38} width={120 + (i % 3) * 20} rounded="md" />
        ))}
      </div>
      <div style={{ display: "grid", gap: 20 }}>
        <section className="admin-panel" style={{ padding: 24 }}>
          <Skeleton height={14} width={100} rounded="sm" style={{ marginBottom: 8 }} />
          <Skeleton height={24} width={220} rounded="sm" style={{ marginBottom: 16 }} />
          <div style={{ display: "grid", gap: 14 }}>
            <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 16 }}>
              <Skeleton height={40} width="100%" rounded="md" />
              <Skeleton height={40} width="100%" rounded="md" />
            </div>
            <Skeleton height={80} width="100%" rounded="md" />
            <div style={{ display: "flex", gap: 12, marginTop: 8 }}>
              <Skeleton height={38} width={140} rounded="md" />
              <Skeleton height={38} width={100} rounded="md" />
            </div>
          </div>
        </section>
      </div>
    </div>
  );
}

/**
 * Admin Monetization Skeleton
 */
export function AdminMonetizationSkeleton() {
  return (
    <div className="admin-monetization-skeleton" aria-busy="true" aria-label="Загрузка монетизации">
      <div className="admin-kpi-grid" style={{ marginBottom: 24 }}>
        {Array.from({ length: 4 }).map((_, i) => (
          <div key={i} className="admin-kpi skeleton-card-item">
            <Skeleton height={12} width={90} rounded="sm" style={{ marginBottom: 8 }} />
            <Skeleton height={30} width={60} rounded="md" style={{ marginBottom: 8 }} />
            <Skeleton height={11} width={130} rounded="sm" />
          </div>
        ))}
      </div>
      <section className="admin-section" style={{ marginBottom: 24 }}>
        <Skeleton height={20} width={160} rounded="sm" style={{ marginBottom: 16 }} />
        <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(280px, 1fr))", gap: 16 }}>
          {Array.from({ length: 3 }).map((_, i) => (
            <div key={i} className="admin-panel" style={{ padding: 20 }}>
              <div style={{ display: "flex", justifyContent: "space-between", marginBottom: 12 }}>
                <Skeleton height={22} width={100} rounded="sm" />
                <Skeleton height={20} width={60} rounded="full" />
              </div>
              <Skeleton height={28} width={120} rounded="sm" style={{ marginBottom: 12 }} />
              <div style={{ display: "grid", gap: 8, marginBottom: 16 }}>
                <Skeleton height={14} width="90%" rounded="sm" />
                <Skeleton height={14} width="75%" rounded="sm" />
                <Skeleton height={14} width="85%" rounded="sm" />
              </div>
              <Skeleton height={36} width="100%" rounded="md" />
            </div>
          ))}
        </div>
      </section>
    </div>
  );
}

/**
 * Admin Fees / Economics Skeleton
 */
export function AdminFeesSkeleton() {
  return (
    <div className="admin-fees-skeleton" aria-busy="true" aria-label="Загрузка комиссий и экономики">
      <div className="admin-quick-grid" style={{ marginBottom: 24 }}>
        <section className="admin-panel" style={{ padding: 20 }}>
          <Skeleton height={14} width={120} rounded="sm" style={{ marginBottom: 8 }} />
          <Skeleton height={22} width={180} rounded="sm" style={{ marginBottom: 12 }} />
          <Skeleton height={14} width="90%" rounded="sm" style={{ marginBottom: 16 }} />
          <div style={{ display: "grid", gap: 10 }}>
            <Skeleton height={38} width="100%" rounded="md" />
            <Skeleton height={38} width="100%" rounded="md" />
          </div>
        </section>
        <section className="admin-panel" style={{ padding: 20 }}>
          <Skeleton height={14} width={120} rounded="sm" style={{ marginBottom: 8 }} />
          <Skeleton height={22} width={180} rounded="sm" style={{ marginBottom: 12 }} />
          <Skeleton height={14} width="90%" rounded="sm" style={{ marginBottom: 16 }} />
          <div style={{ display: "grid", gap: 10 }}>
            <Skeleton height={38} width="100%" rounded="md" />
            <Skeleton height={38} width="100%" rounded="md" />
          </div>
        </section>
      </div>
      <AdminTableSkeleton columns={["Версия", "Комиссия платформы", "Мин. комиссия", "Плательщик", "Статус", "Действие"]} rowCount={3} />
    </div>
  );
}

/**
 * Admin Payment Routing Skeleton
 */
export function AdminPaymentRoutingSkeleton() {
  return (
    <div className="admin-routing-skeleton" aria-busy="true" aria-label="Загрузка маршрутизации платежей">
      <div className="admin-quick-grid" style={{ marginBottom: 24 }}>
        {Array.from({ length: 3 }).map((_, i) => (
          <section key={i} className="admin-panel" style={{ padding: 20 }}>
            <div style={{ display: "flex", justifyContent: "space-between", marginBottom: 12 }}>
              <Skeleton height={20} width={120} rounded="sm" />
              <Skeleton height={20} width={70} rounded="full" />
            </div>
            <Skeleton height={14} width="80%" rounded="sm" style={{ marginBottom: 16 }} />
            <Skeleton height={38} width="100%" rounded="md" />
          </section>
        ))}
      </div>
      <AdminTableSkeleton columns={["Домен платежа", "Основной провайдер", "Резервный", "Статус", "Действие"]} rowCount={4} />
    </div>
  );
}

/**
 * Admin Taxonomy Skeleton
 */
export function AdminTaxonomySkeleton() {
  return (
    <div className="admin-taxonomy-skeleton" aria-busy="true" aria-label="Загрузка категорий и навыков">
      <div className="admin-quick-grid admin-taxonomy-create" style={{ marginBottom: 24 }}>
        <section className="admin-panel admin-taxonomy-card" style={{ padding: 20 }}>
          <Skeleton height={14} width={120} rounded="sm" style={{ marginBottom: 8 }} />
          <div style={{ display: "grid", gap: 10 }}>
            <Skeleton height={38} width="100%" rounded="md" />
            <Skeleton height={38} width="100%" rounded="md" />
            <Skeleton height={60} width="100%" rounded="md" />
            <Skeleton height={36} width={140} rounded="md" />
          </div>
        </section>
        <section className="admin-panel admin-taxonomy-card" style={{ padding: 20 }}>
          <Skeleton height={14} width={100} rounded="sm" style={{ marginBottom: 8 }} />
          <div style={{ display: "grid", gap: 10 }}>
            <Skeleton height={38} width="100%" rounded="md" />
            <Skeleton height={38} width="100%" rounded="md" />
            <Skeleton height={60} width="100%" rounded="md" />
            <Skeleton height={36} width={120} rounded="md" />
          </div>
        </section>
      </div>
      <div style={{ marginBottom: 24 }}>
        <Skeleton height={22} width={130} rounded="sm" style={{ marginBottom: 12 }} />
        <AdminTableSkeleton columns={["Название", "Slug", "Порядок", "Статус", "Действие"]} rowCount={4} />
      </div>
      <div>
        <Skeleton height={22} width={100} rounded="sm" style={{ marginBottom: 12 }} />
        <AdminTableSkeleton columns={["Навык", "Slug", "Статус", "Действие"]} rowCount={4} />
      </div>
    </div>
  );
}

