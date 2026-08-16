"use client";
import { CustomSelect } from "../custom-select";

import { useCallback, useEffect, useMemo, useState } from "react";
import { Avatar, PresenceLabel } from "../media-components";
import Breadcrumbs from "../breadcrumbs";
import FavoriteButton from "../favorite-button";
import { IconBriefcase, IconMapPin, IconWallet } from "../icons";
import Rating from "../rating";
import ProBadge from "../pro-badge";
import { countLabel } from "../russian-plural";
import { useSiteSettings } from "../site-settings";
import { useInfiniteScroll } from "../use-infinite-scroll";
import { FreelancerCardSkeleton, FreelancersCatalogSkeleton } from "../skeletons";

type Freelancer = {
  id: string;
  username: string;
  display_name: string;
  professional_title?: string;
  availability: string;
  location_text?: string;
  experience_years?: number;
  hourly_rate_kopecks?: number;
  native_rating?: number;
  reviews_count?: number;
  completed_projects_count?: number;
  skills?: Array<{ id: string; name: string }>;
  effective_pro?: boolean;
};
const formatRubles = (kopecks: number) =>
  new Intl.NumberFormat("ru-RU").format(kopecks / 100);
const availabilityLabels: Record<string, string> = {
  AVAILABLE: "Доступен",
  PARTIALLY_BUSY: "Частично занят",
  BUSY: "Занят",
  UNAVAILABLE: "Недоступен",
};

export default function FreelancersPage() {
  // Breadcrumb contract: label:"Специалисты" is a top-level catalog.
  const { catalog_page_size: pageSize } = useSiteSettings();
  const [items, setItems] = useState<Freelancer[]>([]);
  const [query, setQuery] = useState("");
  const [availability, setAvailability] = useState("");
  const [sort, setSort] = useState("rating");
  const [loading, setLoading] = useState(true);
  const [loadingMore, setLoadingMore] = useState(false);
  const [cursor, setCursor] = useState<string | null>(null);
  const [failed, setFailed] = useState(false);
  const [urlQueryReady, setUrlQueryReady] = useState(false);
  useEffect(() => {
    setQuery(new URLSearchParams(window.location.search).get("q") ?? "");
    setUrlQueryReady(true);
  }, []);
  const load = useCallback(
    async (nextCursor?: string, append = false) => {
      const params = new URLSearchParams({ limit: String(pageSize) });
      if (query) params.set("q", query);
      if (nextCursor) params.set("cursor", nextCursor);
      if (append) setLoadingMore(true);
      else setLoading(true);
      setFailed(false);
      try {
        const r = await fetch(`/api/v1/freelancers?${params}`, {
          cache: "no-store",
        });
        if (!r.ok) throw new Error();
        const b = await r.json();
        const next: Freelancer[] = b.data ?? [];
        setItems((current) =>
          append
            ? [
                ...current,
                ...next.filter(
                  (item) =>
                    !current.some((existing) => existing.id === item.id),
                ),
              ]
            : next,
        );
        setCursor(b.page?.next_cursor ?? null);
      } catch {
        if (!append) {
          setItems([]);
          setFailed(true);
        }
      } finally {
        setLoading(false);
        setLoadingMore(false);
      }
    },
    [pageSize, query],
  );
  useEffect(() => {
    if (!urlQueryReady) return;
    const timer = window.setTimeout(() => void load(), 220);
    return () => window.clearTimeout(timer);
  }, [load, urlQueryReady]);
  const loadMore = useCallback(() => {
    if (cursor && !loadingMore) void load(cursor, true);
  }, [cursor, load, loadingMore]);
  const sentinel = useInfiniteScroll(!!cursor, loadingMore, loadMore);
  const sorted = useMemo(() => {
    let list = items.filter(
      (v) => !availability || v.availability === availability,
    );
    if (sort === "rate_asc")
      list = [...list].sort(
        (a, b) =>
          (a.hourly_rate_kopecks ?? Infinity) -
          (b.hourly_rate_kopecks ?? Infinity),
      );
    else if (sort === "experience")
      list = [...list].sort(
        (a, b) => (b.experience_years ?? -1) - (a.experience_years ?? -1),
      );
    else
      list = [...list].sort((a, b) => {
        const aReviewed = (a.reviews_count ?? 0) > 0;
        const bReviewed = (b.reviews_count ?? 0) > 0;
        if (aReviewed !== bReviewed) return bReviewed ? 1 : -1;
        if (aReviewed && bReviewed) return (b.native_rating ?? 0) - (a.native_rating ?? 0) || (b.reviews_count ?? 0) - (a.reviews_count ?? 0);
        return Number(Boolean(b.effective_pro)) - Number(Boolean(a.effective_pro)) || (b.experience_years ?? -1) - (a.experience_years ?? -1);
      });
    return list;
  }, [items, availability, sort]);
  return (
    <main>
      <Breadcrumbs
        items={[{ label: "Главная", href: "/" }, { label: "Специалисты" }]}
      />
      <div className="page-heading">
        <div>
          <p className="eyebrow">Открытый каталог</p>
          <h1>Специалисты</h1>
          <p className="lead">
            Сравните рейтинг, опыт, навыки, доступность и ставку — затем
            обсудите задачу напрямую.
          </p>
        </div>
        <a className="button" href="/create-project">
          Разместить задачу
        </a>
      </div>
      <section className="filters" aria-label="Поиск специалистов">
        <label>
          Поиск
          <input
            type="search"
            maxLength={120}
            placeholder="Имя, специализация или навык"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
          />
        </label>
        <label>
          Доступность
          <CustomSelect
            value={availability}
            onChange={(e) => setAvailability(e.target.value)}
          >
            <option value="">Любая</option>
            {Object.entries(availabilityLabels).map(([v, l]) => (
              <option key={v} value={v}>
                {l}
              </option>
            ))}
          </CustomSelect>
        </label>
      </section>
      {loading ? (
        <FreelancersCatalogSkeleton count={6} />
      ) : failed ? (
        <div className="error">Не удалось загрузить каталог.</div>
      ) : !sorted.length ? (
        <div className="empty">
          <h2>Специалисты не найдены</h2>
          <p>Измените запрос или фильтры.</p>
        </div>
      ) : (
        <>
          <div className="list-toolbar">
            <p className="list-count">
              Загружено: <strong>{sorted.length}</strong>
            </p>
            <label className="list-sort">
              Сортировка
              <CustomSelect
                value={sort}
                onChange={(e) => setSort(e.target.value)}
              >
                <option value="rating">По рейтингу</option>
                <option value="rate_asc">Дешевле за час</option>
                <option value="experience">Больше опыта</option>
              </CustomSelect>
            </label>
          </div>
          <ul className="freelancer-grid">
            {sorted.map((item) => (
              <li key={item.username}>
                <article className="profile-card profile-card--rich">
                  <div className="profile-card__top">
                    <Avatar
                      name={item.display_name}
                      id={item.username}
                      size="lg"
                    />
                    <FavoriteButton type="FREELANCER" id={item.id} compact />
                  </div>
                  <h2>
                    <a href={`/freelancers/${item.username}`}>
                      {item.display_name}
                    </a>
                    {item.effective_pro ? <ProBadge compact /> : null}
                  </h2>
                  <p className="profile-card__title">
                    {item.professional_title || "Профессиональный исполнитель"}
                  </p>
                  <div className="profile-card__status-row">
                    <PresenceLabel id={item.username} />
                    <span className="badge">
                      {availabilityLabels[item.availability] ??
                        "Статус уточняется"}
                    </span>
                    {item.native_rating ? (
                      <Rating
                        value={item.native_rating}
                        reviews={item.reviews_count ?? 0}
                        compact
                      />
                    ) : (
                      <span className="rating-pill rating-pill--new">
                        Новый профиль
                      </span>
                    )}
                  </div>
                  <div className="person-facts">
                    {item.experience_years !== undefined ? (
                      <span>
                        <IconBriefcase size={15} />
                        Опыт{" "}
                        {countLabel(item.experience_years, [
                          "год",
                          "года",
                          "лет",
                        ])}
                      </span>
                    ) : null}
                    {item.hourly_rate_kopecks !== undefined ? (
                      <span>
                        <IconWallet size={15} />
                        от {formatRubles(item.hourly_rate_kopecks)} ₽/ч
                      </span>
                    ) : null}
                    {item.location_text ? (
                      <span>
                        <IconMapPin size={15} />
                        {item.location_text}
                      </span>
                    ) : null}
                  </div>
                  {item.skills?.length ? (
                    <div className="chip-row">
                      {item.skills.slice(0, 5).map((skill) => (
                        <span className="chip" key={skill.id}>
                          {skill.name}
                        </span>
                      ))}
                    </div>
                  ) : null}
                  <div className="profile-card__footer">
                    {item.completed_projects_count ? (
                      <small>
                        {countLabel(item.completed_projects_count, [
                          "завершённый проект",
                          "завершённых проекта",
                          "завершённых проектов",
                        ])}
                      </small>
                    ) : (
                      <small>Опыт подтверждается профилем и отзывами</small>
                    )}
                    <a
                      className="button button--quiet button--compact"
                      href={`/freelancers/${item.username}`}
                    >
                      Открыть профиль
                    </a>
                  </div>
                </article>
              </li>
            ))}
          </ul>
          <div ref={sentinel} className="infinite-loader" aria-live="polite">
            {loadingMore ? (
              <>
                <span className="spinner" />
                <span>Загружаем ещё специалистов…</span>
                <div className="infinite-loader__skeletons">
                  <FreelancerCardSkeleton />
                  <FreelancerCardSkeleton />
                </div>
              </>
            ) : cursor ? (
              <span>Прокрутите ниже, чтобы увидеть ещё</span>
            ) : (
              <span>Вы посмотрели всех специалистов</span>
            )}
          </div>
        </>
      )}
    </main>
  );
}
