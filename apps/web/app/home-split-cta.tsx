"use client";

import { useAuth } from "./auth-state";

export default function HomeSplitCta() {
  const { state, user } = useAuth();
  const isFreelancer = state === "authenticated" && !!user?.capabilities?.includes("FREELANCER");
  if (isFreelancer) return null;

  return (
    <section className="split-cta split-cta--premium">
      <div>
        <p className="eyebrow eyebrow--light">Для заказчиков</p>
        <h2>Найдите исполнителя под результат, а не просто по ставке.</h2>
        <p>Опубликуйте задачу и сравните специалистов по релевантному опыту, портфолио и репутации.</p>
        <a className="button button--light" href="/create-project">Разместить проект</a>
      </div>
      <div>
        <p className="eyebrow">Для специалистов</p>
        <h2>Покажите опыт и переносимую репутацию.</h2>
        <p>Оформите профиль, добавьте портфолио, услуги и подтверждённые внешние источники.</p>
        <a className="button button--dark" href={state === "authenticated" ? "/settings/reputation" : "/register"}>
          {state === "authenticated" ? "Оформить профиль" : "Создать профиль"}
        </a>
      </div>
    </section>
  );
}
