"use client";

import Image from "next/image";

import { FormEvent, useEffect, useMemo, useRef, useState } from "react";
import { Avatar, PresenceLabel } from "./media-components";
import {
  IconChart,
  IconCode,
  IconDatabase,
  IconMegaphone,
  IconPalette,
  IconPenTool,
  IconTag,
} from "./icons";
import { track } from "./analytics";
import { useAuth } from "./auth-state";
import Rating from "./rating";
import { countLabel } from "./russian-plural";
import {
  CategoriesCatalogSkeleton,
  FreelancerCardSkeleton,
  ProjectCardSkeleton,
  ServiceCardSkeleton,
} from "./skeletons";

type Category = { id: string; slug: string; name: string };
type Person = {
  id?: string;
  username: string;
  display_name: string;
  professional_title?: string;
  availability?: string;
  experience_years?: number;
  hourly_rate_kopecks?: number;
  skills?: { id?: string; name: string }[];
  native_rating?: number;
  reviews_count?: number;
  completed_projects_count?: number;
};
type Service = {
  id: string;
  slug?: string;
  title: string;
  short_description?: string;
  service_type?: string;
  price_from?: { amount_kopecks: number };
  seller_display_name?: string;
};
type Project = {
  id: string;
  title: string;
  description: string;
  category?: { name: string };
  skills?: { id?: string; name: string }[];
  budget?: { min_kopecks?: number; max_kopecks?: number };
};

const rub = (v?: number) =>
  v === undefined
    ? "По договорённости"
    : `${new Intl.NumberFormat("ru-RU").format(v / 100)} ₽`;

function CategoryIcon({ slug, name }: { slug: string; name: string }) {
  const key = `${slug} ${name}`.toLowerCase();
  if (/design|дизайн|ux|ui/.test(key)) return <IconPalette size={24} />;
  if (/market|маркет|seo|реклам|smm/.test(key))
    return <IconMegaphone size={24} />;
  if (/data|данн|analytic|аналит/.test(key)) return <IconChart size={24} />;
  if (/text|контент|copy|копирай|редакт/.test(key))
    return <IconPenTool size={24} />;
  if (/backend|database|баз/.test(key)) return <IconDatabase size={24} />;
  if (/develop|разработ|program|программ|it|ai|ии/.test(key))
    return <IconCode size={24} />;
  return <IconTag size={24} />;
}

export default function Home() {
  const { state: authState, user } = useAuth();
  const [text, setText] = useState("");
  const [categories, setCategories] = useState<Category[]>([]);
  const [people, setPeople] = useState<Person[]>([]);
  const [services, setServices] = useState<Service[]>([]);
  const [projects, setProjects] = useState<Project[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    Promise.all([
      fetch("/api/v1/categories"),
      fetch("/api/v1/freelancers?limit=50"),
      fetch("/api/v1/services?limit=6"),
      fetch("/api/v1/projects?limit=6"),
    ])
      .then(async (responses) =>
        Promise.all(
          responses.map((response) =>
            response.ok ? response.json() : { data: [] },
          ),
        ),
      )
      .then(([categoryBody, peopleBody, serviceBody, projectBody]) => {
        setCategories(categoryBody.data ?? []);
        setPeople(peopleBody.data ?? []);
        setServices(serviceBody.data ?? []);
        setProjects(projectBody.data ?? []);
      })
      .catch(() => undefined)
      .finally(() => setLoading(false));
  }, []);

  const topPeople = useMemo(
    () => authState === "loading" ? [] :
      [...people]
        .filter(person => person.id !== user?.id && person.username !== user?.username)
        .sort((a, b) => (b.native_rating ?? 0) - (a.native_rating ?? 0))
        .slice(0, Math.min(10, people.length)),
    [authState, people, user?.id, user?.username],
  );
  const [featuredIndex, setFeaturedIndex] = useState(0);
  const [featuredCount, setFeaturedCount] = useState(2);
  const heroMarketRef = useRef<HTMLElement>(null);
  const heroPeopleRef = useRef<HTMLDivElement>(null);
  const heroToplineRef = useRef<HTMLDivElement>(null);
  const heroTrustRef = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!topPeople.length) return;
    const day = Math.floor(Date.now() / 86_400_000);
    setFeaturedIndex(day % topPeople.length);
    if (topPeople.length < 2) return;
    const timer = window.setInterval(
      () => setFeaturedIndex((current) => (current + 1) % topPeople.length),
      9000,
    );
    return () => window.clearInterval(timer);
  }, [topPeople.length]);
  useEffect(() => {
    const market = heroMarketRef.current;
    if (!market || !topPeople.length) return;
    const recalculate = () => {
      if (window.matchMedia("(max-width: 860px)").matches) {
        setFeaturedCount(Math.min(2, topPeople.length));
        return;
      }
      const candidate = heroPeopleRef.current?.querySelector<HTMLElement>(".hero-market__candidate");
      const top = heroToplineRef.current?.offsetHeight ?? 0;
      const trust = heroTrustRef.current?.offsetHeight ?? 0;
      const cardHeight = (candidate?.offsetHeight ?? 126) + 10;
      const available = market.clientHeight - top - trust - 76;
      setFeaturedCount(Math.max(1, Math.min(topPeople.length, Math.floor(available / cardHeight))));
    };
    recalculate();
    const observer = new ResizeObserver(recalculate);
    observer.observe(market);
    return () => observer.disconnect();
  }, [topPeople.length]);
  const featuredPeople = topPeople.length
    ? Array.from({ length: featuredCount }, (_, offset) => offset)
        .map((offset) => topPeople[(featuredIndex + offset) % topPeople.length])
        .filter(
          (person, index, list) =>
            list.findIndex((item) => item.username === person.username) ===
            index,
        )
    : [];
  const categoryNames = useMemo(
    () => categories.slice(0, 5).map((item) => item.name),
    [categories],
  );

  const taskExamples = useMemo(
    () => [
      [
        "Разработка",
        "Разработать мобильное приложение на Flutter с авторизацией и оплатой",
      ],
      ["Дизайн", "Сделать UX-аудит и редизайн личного кабинета B2B-сервиса"],
      ["Маркетинг", "Настроить рекламную кампанию и сквозную аналитику"],
      ["Контент", "Подготовить экспертные статьи и контент-план на месяц"],
      ["AI", "Интегрировать AI-помощника в поддержку клиентов"],
      ["SEO", "Провести технический SEO-аудит интернет-магазина"],
      [
        "Аналитика",
        "Настроить продуктовую аналитику и дашборд ключевых метрик",
      ],
      ["Backend", "Спроектировать API и интеграцию с платёжным сервисом"],
    ],
    [],
  );
  const [taskOffset, setTaskOffset] = useState(0);
  useEffect(() => {
    setTaskOffset(Math.floor(Date.now() / 86_400_000) % taskExamples.length);
    const timer = window.setInterval(
      () => setTaskOffset((current) => (current + 1) % taskExamples.length),
      12000,
    );
    return () => clearInterval(timer);
  }, [taskExamples.length]);
  const taskPool = useMemo(
    () =>
      [
        ...taskExamples.slice(taskOffset),
        ...taskExamples.slice(0, taskOffset),
      ].slice(0, 3),
    [taskExamples, taskOffset],
  );

  function start(event: FormEvent) {
    event.preventDefault();
    sessionStorage.setItem("guest-project-input", text);
    track("HOMEPAGE_TASK_STARTED");
    location.assign("/create-project");
  }

  return (
    <>
      <main className="home-main">
        <section className="hero hero--premium">
          <div className="hero__inner">
            <div className="hero__copy">
              <p className="eyebrow hero__eyebrow">
                Маркетплейс профессиональных услуг
              </p>
              <h1>Найдите сильного специалиста под конкретный результат.</h1>
              <p className="lead hero__lead">
                Опишите задачу своими словами. Мы поможем собрать бриф, дать
                ориентир цены и выбрать исполнителя по опыту, отзывам и
                подтверждённой репутации.
              </p>

              <form
                className="task-composer task-composer--hero"
                onSubmit={start}
              >
                <label>
                  <span className="task-composer__label">
                    Что вам нужно сделать?
                  </span>
                  <textarea
                    required
                    minLength={3}
                    maxLength={30000}
                    placeholder="Например: разработать приложение для доставки на Flutter с авторизацией, каталогом и оплатой"
                    value={text}
                    onChange={(event) => setText(event.target.value)}
                  />
                </label>
                <div className="task-examples" aria-label="Примеры задач">
                  {taskPool.map(([category, example]) => (
                    <button
                      key={example}
                      type="button"
                      onClick={() => setText(example)}
                    >
                      <small>{category}</small>
                      {example}
                    </button>
                  ))}
                </div>
                <div className="task-composer__actions">
                  <a href="/create-project">Создать вручную</a>
                  <button type="submit">
                    Получить бриф <span aria-hidden="true">→</span>
                  </button>
                </div>
              </form>

              <div className="hero__secondary-actions">
                <a href="/check-offer">Проверить коммерческое предложение</a>
                <a href="/price">Рассчитать стоимость</a>
              </div>
            </div>

            <aside
              ref={heroMarketRef}
              className="hero-market"
              aria-label="Пример профиля специалиста"
            >
              <div ref={heroToplineRef} className="hero-market__topline">
                <span>Подбор под задачу</span>
                <span className="live-dot">Каталог онлайн</span>
              </div>
              {featuredPeople.length ? (
                <div ref={heroPeopleRef} className="hero-market__people">
                  {featuredPeople.map((featured) => (
                    <a
                      className="hero-market__candidate"
                      href={`/freelancers/${featured.username}`}
                      key={featured.username}
                    >
                      <Avatar
                        name={featured.display_name}
                        id={featured.username}
                        size="lg"
                      />
                      <span className="hero-market__candidate-copy">
                        <small>Подходящий специалист</small>
                        <strong>{featured.display_name}</strong>
                        <PresenceLabel id={featured.username} />
                        <span>
                          {featured.professional_title ||
                            "Профессиональный исполнитель"}
                        </span>
                        <span className="hero-market__candidate-meta">
                          {featured.experience_years
                            ? countLabel(featured.experience_years, [
                                "год",
                                "года",
                                "лет",
                              ])
                            : "Опыт в профиле"}
                          {" · "}
                          {featured.hourly_rate_kopecks
                            ? `${rub(featured.hourly_rate_kopecks)}/ч`
                            : "Цена по договорённости"}
                        </span>
                        {featured.native_rating ? (
                          <Rating
                            value={featured.native_rating}
                            reviews={featured.reviews_count ?? 0}
                            compact
                          />
                        ) : (
                          <span>Новый профиль</span>
                        )}
                      </span>
                      <b aria-hidden="true">→</b>
                    </a>
                  ))}
                </div>
              ) : (
                <div className="hero-market__empty">
                  <Image
                    className="hero-market__illustration"
                    src="/media/illustrations/hero-market.svg"
                    alt=""
                    width={360}
                    height={250}
                    sizes="(max-width: 860px) 80vw, 360px"
                    priority
                    unoptimized
                  />
                  <h2>Подбор по навыкам и репутации</h2>
                  <p>
                    После заполнения каталога здесь появятся реальные
                    специалисты из API.
                  </p>
                </div>
              )}
              <div ref={heroTrustRef} className="hero-market__trust">
                <span>✓ Внешняя репутация отдельно</span>
                <span>✓ Safe Deal</span>
              </div>
            </aside>
          </div>
        </section>

        <section className="home-strip" aria-label="Быстрый переход">
          <span>Популярно:</span>
          {(categoryNames.length
            ? categoryNames
            : ["Разработка", "Дизайн", "AI", "Маркетинг", "SEO"]
          ).map((name) => (
            <a key={name} href={`/freelancers?q=${encodeURIComponent(name)}`}>
              {name}
            </a>
          ))}
          <a className="home-strip__all" href="/categories">
            Все направления →
          </a>
        </section>

        <section className="home-section">
          <div className="page-heading page-heading--home">
            <div>
              <p className="eyebrow">Найдите нужную экспертизу</p>
              <h2>Популярные категории</h2>
              <p>
                От разработки до маркетинга — начните с направления или сразу
                опишите задачу.
              </p>
            </div>
            <a className="text-link" href="/categories">
              Все категории <span>→</span>
            </a>
          </div>
          {loading ? (
            <CategoriesCatalogSkeleton count={8} />
          ) : categories.length ? (
            <ul className="category-grid">
              {categories.slice(0, 8).map((category, index) => (
                <li key={category.id}>
                  <a
                    className="category-card"
                    href={`/categories/${category.slug}`}
                  >
                    <span className="category-card__icon" aria-hidden="true">
                      <CategoryIcon slug={category.slug} name={category.name} />
                    </span>
                    <span className="category-card__number">0{index + 1}</span>
                    <strong>{category.name}</strong>
                    <span>Специалисты, услуги и проекты</span>
                    <b aria-hidden="true">↗</b>
                  </a>
                </li>
              ))}
            </ul>
          ) : (
            <div className="empty">
              Каталог направлений пока не заполнен. Опишите задачу — мы поможем
              определить специализацию.
            </div>
          )}
        </section>

        <section className="home-section">
          <div className="page-heading page-heading--home">
            <div>
              <p className="eyebrow">Профессионалы</p>
              <h2>Специалисты, которых можно изучить до отклика</h2>
              <p>
                Портфолио, навыки, условия работы и репутация — в одном профиле.
              </p>
            </div>
            <a className="text-link" href="/freelancers">
              Смотреть всех <span>→</span>
            </a>
          </div>
          {loading ? (
            <ul className="talent-grid" aria-busy="true">
              {Array.from({ length: 4 }).map((_, i) => (
                <li key={i}>
                  <FreelancerCardSkeleton />
                </li>
              ))}
            </ul>
          ) : people.length ? (
            <ul className="talent-grid">
              {people.slice(0, 4).map((person) => (
                <li key={person.username}>
                  <article className="talent-card">
                    <div className="talent-card__head">
                      <Avatar
                        name={person.display_name}
                        id={person.username}
                        size="lg"
                      />
                      <span className="availability-dot">
                        {person.availability === "AVAILABLE"
                          ? "Доступен"
                          : "Профиль открыт"}
                      </span>
                    </div>
                    <h3>
                      <a href={`/freelancers/${person.username}`}>
                        {person.display_name}
                      </a>
                    </h3>
                    <p className="talent-card__title">
                      {person.professional_title ||
                        "Профессиональный исполнитель"}
                    </p>
                    <PresenceLabel id={person.username} />
                    <div className="talent-card__meta">
                      {person.native_rating ? (
                        <Rating
                          value={person.native_rating}
                          reviews={person.reviews_count ?? 0}
                          compact
                        />
                      ) : (
                        <span>Новый профиль</span>
                      )}
                      <span>
                        {person.experience_years
                          ? `${countLabel(person.experience_years, ["год", "года", "лет"])} опыта`
                          : "Опыт в профиле"}
                      </span>
                      <span>
                        {person.hourly_rate_kopecks
                          ? `от ${rub(person.hourly_rate_kopecks)}/ч`
                          : "Цена по договорённости"}
                      </span>
                    </div>
                    <div className="talent-card__skills">
                      {(person.skills ?? []).slice(0, 4).map((skill) => (
                        <span className="chip" key={skill.name}>
                          {skill.name}
                        </span>
                      ))}
                    </div>
                    <a
                      className="card-link"
                      href={`/freelancers/${person.username}`}
                    >
                      Открыть профиль <span>→</span>
                    </a>
                  </article>
                </li>
              ))}
            </ul>
          ) : (
            <div className="empty">
              Открытые профили пока не найдены. Попробуйте поиск по
              специализации.
            </div>
          )}
        </section>

        <section className="home-section home-section--soft">
          <div className="page-heading page-heading--home">
            <div>
              <p className="eyebrow">Готовый формат</p>
              <h2>Услуги и консультации</h2>
              <p>
                Когда задача понятна заранее — выберите готовое предложение и
                обсудите детали.
              </p>
            </div>
            <a className="text-link" href="/services">
              Все услуги <span>→</span>
            </a>
          </div>
          {loading ? (
            <ul className="service-grid" aria-busy="true">
              {Array.from({ length: 4 }).map((_, i) => (
                <li key={i}>
                  <ServiceCardSkeleton />
                </li>
              ))}
            </ul>
          ) : services.length ? (
            <ul className="service-grid">
              {services.slice(0, 4).map((service) => (
                <li key={service.id}>
                  <article className="service-card">
                    <span className="service-card__type">
                      {service.service_type === "CONSULTATION"
                        ? "Консультация"
                        : "Услуга"}
                    </span>
                    <h3>
                      <a href={`/services/${service.slug || service.id}`}>
                        {service.title}
                      </a>
                    </h3>
                    <p>
                      {service.short_description ||
                        "Условия, состав и сроки доступны в карточке услуги."}
                    </p>
                    <div className="service-card__bottom">
                      <strong>{rub(service.price_from?.amount_kopecks)}</strong>
                      {service.seller_display_name ? (
                        <span>{service.seller_display_name}</span>
                      ) : null}
                    </div>
                  </article>
                </li>
              ))}
            </ul>
          ) : (
            <div className="empty">
              Опубликованных услуг пока нет. Вернитесь позже или разместите свой
              запрос.
            </div>
          )}
        </section>

        <section className="home-section">
          <div className="page-heading page-heading--home">
            <div>
              <p className="eyebrow">Свежие задачи</p>
              <h2>Открытые проекты</h2>
              <p>
                Реальные запросы заказчиков — с бюджетом, навыками и понятным
                контекстом.
              </p>
            </div>
            <a className="text-link" href="/projects">
              Все проекты <span>→</span>
            </a>
          </div>
          {loading ? (
            <ul className="project-catalog-list" aria-busy="true">
              {Array.from({ length: 4 }).map((_, i) => (
                <li key={i}>
                  <ProjectCardSkeleton />
                </li>
              ))}
            </ul>
          ) : projects.length ? (
            <ul className="project-list-home">
              {projects.slice(0, 4).map((project) => (
                <li key={project.id}>
                  <article className="project-row">
                    <div className="project-row__body">
                      <span className="project-row__category">
                        {project.category?.name || "Проект"}
                      </span>
                      <h3>
                        <a href={`/projects/${project.id}`}>{project.title}</a>
                      </h3>
                      <p>
                        {project.description.slice(0, 180)}
                        {project.description.length > 180 ? "…" : ""}
                      </p>
                      <div className="project-row__skills">
                        {(project.skills ?? []).slice(0, 4).map((skill) => (
                          <span key={skill.name}>{skill.name}</span>
                        ))}
                      </div>
                    </div>
                    <div className="project-row__aside">
                      <small>Бюджет</small>
                      <strong>{rub(project.budget?.min_kopecks)}</strong>
                      <a href={`/projects/${project.id}`}>Подробнее →</a>
                    </div>
                  </article>
                </li>
              ))}
            </ul>
          ) : (
            <div className="empty">
              Сейчас нет открытых проектов. Создайте первый запрос и получите
              отклики.
            </div>
          )}
        </section>

        <section className="trust-story trust-story--premium">
          <div>
            <p className="eyebrow eyebrow--light">Не начинайте с нуля</p>
            <h2>Репутация из других площадок остаётся с вами.</h2>
            <p className="lead">
              Добавьте профессиональные профили и пройдите проверку. Внешняя
              репутация всегда отображается отдельно от отзывов, полученных
              внутри платформы.
            </p>
            <a className="button button--light" href="/settings/reputation">
              Добавить репутацию
            </a>
          </div>
          <div className="trust-story__points">
            <article>
              <span className="trust-icon">01</span>
              <h3>Проверяем источник</h3>
              <p>Статус нельзя назначить себе самостоятельно.</p>
            </article>
            <article>
              <span className="trust-icon">02</span>
              <h3>Не смешиваем рейтинги</h3>
              <p>Внешние оценки и отзывы платформы остаются раздельными.</p>
            </article>
            <article>
              <span className="trust-icon">03</span>
              <h3>Safe Deal</h3>
              <p>
                Работа, сдача, приёмка и спор отражаются отдельными статусами.
              </p>
            </article>
          </div>
        </section>

        <section className="home-section">
          <div className="page-heading page-heading--home">
            <div>
              <p className="eyebrow">Прозрачный процесс</p>
              <h2>От задачи до результата — без хаоса в откликах</h2>
            </div>
          </div>
          <div className="steps steps--premium">
            <article>
              <strong>01</strong>
              <h3>Сформулируйте результат</h3>
              <p>
                Создайте проект вручную, из материалов или через редактируемый
                AI-бриф.
              </p>
            </article>
            <article>
              <strong>02</strong>
              <h3>Сравните кандидатов</h3>
              <p>
                Изучите специализацию, портфолио, условия и раздельные источники
                репутации.
              </p>
            </article>
            <article>
              <strong>03</strong>
              <h3>Работайте по этапам</h3>
              <p>
                Чат, Safe Deal, сдача результата, доработки и споры — в одном
                рабочем контуре.
              </p>
            </article>
          </div>
        </section>

        {authState === "anonymous" ? (
          <section className="split-cta split-cta--premium">
            <div>
              <p className="eyebrow eyebrow--light">Для заказчиков</p>
              <h2>Найдите исполнителя под результат, а не просто по ставке.</h2>
              <p>
                Опубликуйте задачу и сравните специалистов по релевантному
                опыту, портфолио и репутации.
              </p>
              <a className="button button--light" href="/create-project">
                Разместить проект
              </a>
            </div>
            <div>
              <p className="eyebrow">Для специалистов</p>
              <h2>Покажите опыт и переносимую репутацию.</h2>
              <p>
                Оформите профиль, добавьте портфолио, услуги и подтверждённые
                внешние источники.
              </p>
              <a className="button button--dark" href="/register">
                Создать профиль
              </a>
            </div>
          </section>
        ) : null}
      </main>
    </>
  );
}
