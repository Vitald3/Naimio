"use client";
import { usePathname } from "next/navigation";
import { useAdminAuth } from "./admin-auth";

import { STAFF_BASE_PATH } from "../admin-path";

const links = [
  [STAFF_BASE_PATH, "Обзор", "all"],
  [`${STAFF_BASE_PATH}/users`, "Пользователи", "admin"],
  [`${STAFF_BASE_PATH}/taxonomy`, "Категории и навыки", "admin"],
  [`${STAFF_BASE_PATH}/reputation`, "Репутация", "all"],
  [`${STAFF_BASE_PATH}/reports`, "Жалобы", "all"],
  [`${STAFF_BASE_PATH}/fraud`, "Fraud-сигналы", "all"],
  [`${STAFF_BASE_PATH}/projects`, "Проекты", "all"],
  [`${STAFF_BASE_PATH}/services`, "Услуги", "all"],
  [`${STAFF_BASE_PATH}/vacancies`, "Вакансии", "all"],
  [`${STAFF_BASE_PATH}/reviews`, "Отзывы", "all"],
  [`${STAFF_BASE_PATH}/support`, "Поддержка", "all"],
  [`${STAFF_BASE_PATH}/matching`, "Ручной подбор", "admin"],
  [`${STAFF_BASE_PATH}/calculators`, "Калькуляторы", "admin"],
  [`${STAFF_BASE_PATH}/safe-deals`, "Safe Deal", "all"],
  [`${STAFF_BASE_PATH}/finance/fees`, "Экономика и комиссии", "admin"],
  [`${STAFF_BASE_PATH}/monetization`, "Монетизация / PRO", "admin"],
  [`${STAFF_BASE_PATH}/payment-routing`, "Платёжные провайдеры", "admin"],
  [`${STAFF_BASE_PATH}/payments`, "Платёжные операции", "admin"],
  [`${STAFF_BASE_PATH}/content`, "Блог / CMS", "admin"],
  [`${STAFF_BASE_PATH}/disputes`, "Споры", "all"],
  [`${STAFF_BASE_PATH}/referral-rules`, "Реферальные правила", "admin"],
  [`${STAFF_BASE_PATH}/feature-flags`, "Feature Flags", "admin"],
  [`${STAFF_BASE_PATH}/settings`, "Настройки проекта", "admin"],
  [`${STAFF_BASE_PATH}/audit`, "Аудит", "admin"],
] as const;

export default function AdminNav() {
  const pathname = usePathname();
  const { user, logout } = useAdminAuth();
  const isAdmin =
    user?.roles?.some((role) => role === "ADMIN" || role === "SUPER_ADMIN") ??
    false;
  return (
    <aside className="admin-sidebar" aria-label="Администрирование">
      <div className="admin-sidebar__brand">
        <span className="brand-mark">nm</span>
        <div>
          <strong>Control Center</strong>
          <small>Marketplace operations</small>
        </div>
      </div>
      <nav>
        {links
          .filter(([, , scope]) => scope === "all" || isAdmin)
          .map(([href, label]) => (
            <a
              key={href}
              href={href}
              className={pathname === href ? "is-active" : undefined}
            >
              {label}
            </a>
          ))}
      </nav>
      <div className="admin-sidebar__footer">
        <strong>{user?.display_name}</strong>
        <div>Служебная зона Naimio</div>
        <button
          className="button button--quiet button--compact"
          type="button"
          onClick={async () => {
            await logout();
            location.replace(`${STAFF_BASE_PATH}/login`);
          }}
        >
          Выйти
        </button>
      </div>
    </aside>
  );
}
