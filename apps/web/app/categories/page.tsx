import Breadcrumbs from "../breadcrumbs";
import SkillSearch from "./skill-search";
import {
  IconArrowRight,
  IconChart,
  IconCode,
  IconMegaphone,
  IconPalette,
  IconPenTool,
  IconTag,
} from "../icons";

type Category = {
  id: string;
  slug: string;
  name: string;
  description?: string;
  children?: Category[];
};
async function load(): Promise<Category[]> {
  const base = process.env.API_BASE_URL ?? "http://localhost:8080";
  try {
    const response = await fetch(`${base}/api/v1/categories`, {
      cache: "no-store",
    });
    if (!response.ok) return [];
    return (await response.json()).data ?? [];
  } catch {
    return [];
  }
}
function CategoryIcon({ category }: { category: Category }) {
  const key = `${category.slug} ${category.name}`.toLowerCase();
  if (/design|дизайн|ux|ui/.test(key)) return <IconPalette />;
  if (/market|маркет|seo|реклам|smm/.test(key)) return <IconMegaphone />;
  if (/data|аналит|данн/.test(key)) return <IconChart />;
  if (/text|контент|copy|редакт/.test(key)) return <IconPenTool />;
  if (/develop|разработ|program|it|ai|ии/.test(key)) return <IconCode />;
  return <IconTag />;
}
export default async function CategoriesPage() {
  const categories = await load();
  return (
    <main className="categories-index">
      <Breadcrumbs
        items={[{ label: "Главная", href: "/" }, { label: "Категории" }]}
      />
      <header className="page-heading">
        <div>
          <p className="eyebrow">Навигация по экспертизе</p>
          <h1>Категории работ</h1>
          <p className="lead">
            Выберите направление, чтобы посмотреть специалистов, услуги и
            открытые проекты. Структура категорий едина для поиска и SEO.
          </p>
        </div>
      </header>
      {categories.length ? (
        <ul className="categories-index__grid category-grid">
          {categories.map((c) => (
            <li key={c.id}>
              <a
                className="categories-index__card"
                href={`/categories/${c.slug}`}
              >
                <span className="categories-index__icon">
                  <CategoryIcon category={c} />
                </span>
                <div>
                  <h2>{c.name}</h2>
                  {c.description ? (
                    <p>{c.description}</p>
                  ) : (
                    <p>Специалисты, услуги и проекты в этом направлении.</p>
                  )}
                </div>
                <span className="categories-index__arrow">
                  <IconArrowRight />
                </span>
              </a>
              {c.children?.length ? (
                <div className="categories-index__children">
                  {c.children.slice(0, 6).map((v) => (
                    <a key={v.id} href={`/categories/${v.slug}`}>
                      {v.name}
                    </a>
                  ))}
                </div>
              ) : null}
            </li>
          ))}
        </ul>
      ) : (
        <div className="empty">
          <h2>Категории временно недоступны</h2>
          <p>
            Каталог не удалось загрузить. Обновите страницу через несколько
            секунд.
          </p>
        </div>
      )}
      <SkillSearch />
    </main>
  );
}
