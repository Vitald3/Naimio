import type { Metadata } from "next";

export const metadata: Metadata = {
  title: "Страница не найдена — Naimio",
  robots: { index: false, follow: false },
};

export default function NotFound() {
  return (
    <main className="status-page">
      <p className="eyebrow">Ошибка 404</p>
      <h1>Страница не найдена</h1>
      <p className="lead">
        Возможно, ссылка устарела или страница была перемещена. Проверьте адрес
        или продолжите с одного из разделов ниже.
      </p>
      <div className="status-page__actions">
        <a className="button" href="/">
          На главную
        </a>
        <a className="button button--quiet" href="/freelancers">
          Каталог специалистов
        </a>
        <a className="button button--quiet" href="/projects">
          Открытые проекты
        </a>
      </div>
    </main>
  );
}
