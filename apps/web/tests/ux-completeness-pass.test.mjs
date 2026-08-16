import test from "node:test";
import assert from "node:assert/strict";
import { readFile, readdir } from "node:fs/promises";

const read = (path) => readFile(new URL(`../${path}`, import.meta.url), "utf8");

test("freelancer does not see generic customer/specialist split CTA", async () => {
  const home = await read("app/page.tsx");
  assert.match(home, /authState === "anonymous"/);
});

test("built-in cover illustrations contain no duplicate text overlay", async () => {
  const dir = new URL("../public/media/covers/", import.meta.url);
  for (const name of await readdir(dir)) {
    if (!name.endsWith(".svg")) continue;
    const source = await readFile(new URL(name, dir), "utf8");
    assert.doesNotMatch(source, /<text\b/i, `${name} contains text baked into the image`);
  }
  const media = await read("app/media-components.tsx");
  assert.doesNotMatch(media, /<span>\{title\}<\/span>/);
});

test("header uses the Naimio SVG logo component", async () => {
  const logo = await read("app/logo.tsx");
  const header = await read("app/site-header.tsx");
  assert.match(logo, /<svg/);
  assert.match(logo, /naimio-logo__word/);
  assert.match(header, /NaimioLogo/);
});

test("project sharing has Web Share and clipboard fallback", async () => {
  const source = await read("app/project/[id]/share-button.tsx");
  assert.match(source, /navigator\.share/);
  assert.match(source, /navigator\.clipboard/);
  assert.match(source, /execCommand\("copy"\)/);
});

test("message sending keeps credentials and reports server errors", async () => {
  const source = await read("app/messages/page.tsx");
  assert.match(source, /credentials:\s*"same-origin"/);
  assert.match(source, /problem\?\.error\?\.message/);
  assert.match(source, /Сообщение не отправлено/);
  const backend = await readFile(new URL("../../api/internal/communication/postgres.go", import.meta.url), "utf8");
  assert.match(backend, /MESSAGE_CREATED/);
  assert.match(backend, /best-effort/);
});

test("validation replaces raw browser pattern hints with human messages", async () => {
  const feedback = await read("app/ui-feedback.tsx");
  const newProject = await read("app/dashboard/projects/new/page.tsx");
  assert.match(feedback, /event\.preventDefault\(\)/);
  assert.match(feedback, /field-error/);
  assert.match(feedback, /dataset\.patternMessage/);
  assert.doesNotMatch(newProject, /pattern=/);
  assert.match(newProject, /ProjectDescriptionEditor/);
});

test("customer and freelancer dashboards expose only role-appropriate proposal/profile links", async () => {
  const nav = await read("app/dashboard/dashboard-nav.tsx");
  const dashboard = await read("app/dashboard/page.tsx");
  assert.match(nav, /freelancer \? \[\{ label: "Работа"/);
  assert.match(nav, /Профессиональный профиль/);
  assert.match(dashboard, /freelancer\?fetch\("\/api\/v1\/me\/proposals"/);
  assert.match(dashboard, /Входящие отклики находятся внутри каждого проекта/);
});

test("team actions are spaced and team is distinct from favorites", async () => {
  const page = await read("app/dashboard/team/page.tsx");
  const css = await read("app/globals.css");
  assert.match(page, /Добавление выполняется отдельно от избранного/);
  assert.match(css, /\.team-card__links,\s*\.team-card__actions\s*\{[^}]*gap:\s*10px/);
});

test("favorites can be added from specialists, services and projects", async () => {
  for (const path of ["app/freelancers/page.tsx", "app/freelancers/[username]/page.tsx", "app/services/page.tsx", "app/services/[id]/page.tsx", "app/projects/page.tsx", "app/projects/[id]/page.tsx"]) {
    const source = await read(path);
    assert.match(source, /FavoriteButton/, `${path} has no FavoriteButton`);
  }
});

test("notification settings have readable layout and transient top-right toasts", async () => {
  const page = await read("app/settings/notifications/page.tsx");
  const toast = await read("app/toast.tsx");
  const css = await read("app/globals.css");
  assert.match(page, /settings-event/);
  assert.match(page, /push\(\{ kind: "success", title: "Настройки сохранены"/);
  assert.match(toast, /timeoutMs = 3600/);
  assert.match(css, /\.toast-stack\s*\{[^}]*position:\s*fixed[^}]*right:\s*22px[^}]*top:\s*94px/);
});

test("categories index is a styled catalog rather than an unfinished placeholder", async () => {
  const page = await read("app/categories/page.tsx");
  assert.match(page, /category-grid/);
  assert.match(page, /CategoryIcon/);
  assert.match(page, /\/api\/v1\/categories/);
});

test("public route breadcrumbs use semantic BreadcrumbList hierarchy", async () => {
  const breadcrumbs = await read("app/breadcrumbs.tsx");
  assert.match(breadcrumbs, /BreadcrumbList/);
  assert.match(breadcrumbs, /aria-label="Хлебные крошки"/);
  const routes = ["app/categories/page.tsx", "app/categories/[slug]/page.tsx", "app/freelancers/page.tsx", "app/freelancers/[username]/page.tsx", "app/projects/page.tsx", "app/projects/[id]/page.tsx", "app/services/page.tsx", "app/services/[id]/page.tsx", "app/vacancies/page.tsx", "app/vacancies/[id]/page.tsx", "app/education/page.tsx", "app/price/page.tsx", "app/check-offer/page.tsx"];
  for (const path of routes) assert.match(await read(path), /Breadcrumbs/, `${path} has no breadcrumbs`);
  assert.doesNotMatch(await read("app/freelancers/page.tsx"), /label:\s*"Категории"/);
});

test("people-oriented cards expose rating and richer trust information", async () => {
  assert.match(await read("app/freelancers/page.tsx"), /native_rating/);
  assert.match(await read("app/dashboard/team/page.tsx"), /reviews_count/);
  assert.match(await read("app/dashboard/projects/[id]/recommendations/page.tsx"), /native_rating/);
  assert.match(await read("app/services/page.tsx"), /seller_native_rating/);
  assert.match(await read("app/services/[id]/page.tsx"), /seller_native_rating/);
});

test("projects vacancies and education expose expanded filters", async () => {
  const projects = await read("app/projects/page.tsx");
  for (const token of ["category", "budgetType", "experience", "minBudget", "deadline"]) assert.match(projects, new RegExp(token));
  const vacancies = await read("app/vacancies/page.tsx");
  for (const token of ["category", "employment_type", "remote", "experience", "location", "min_salary_kopecks"]) assert.match(vacancies, new RegExp(token));
  const education = await read("app/education/page.tsx");
  for (const token of ["service_type", "audience", "format", "price_type", "maxDuration"]) assert.match(education, new RegExp(token));
});

test("customer cannot see project or vacancy application controls", async () => {
  const proposal = await read("app/project/[id]/proposal-form.tsx");
  const application = await read("app/vacancy/[id]/application-form.tsx");
  assert.match(proposal, /if \(!user\?\.capabilities\?\.includes\("FREELANCER"\)\) return null/);
  assert.match(application, /authState === "authenticated" && !user\?\.capabilities\?\.includes\("FREELANCER"\)/);
});

test("manual project creation and draft editing expose category skills budget deadline and visibility", async () => {
  for (const path of ["app/dashboard/projects/new/page.tsx", "app/dashboard/projects/[id]/page.tsx"]) {
    const source = await read(path);
    for (const token of ["Категория", "Навыки", "Бюджет", "срок", "Видимость"]) assert.match(source, new RegExp(token, "i"), `${path} misses ${token}`);
  }
});

test("homepage rotates among high-rated specialists and multiple task categories", async () => {
  const home = await read("app/page.tsx");
  assert.match(home, /native_rating/);
  assert.match(home, /topPeople/);
  assert.doesNotMatch(home, /Math\.random/);
  assert.match(home, /taskExamples/);
  assert.match(home, /taskOffset/);
});

test("project vacancy and service catalogs refresh in-place without caching stale lists", async () => {
  for (const path of ["app/projects/page.tsx", "app/vacancies/page.tsx", "app/services/page.tsx"]) {
    const source = await read(path);
    assert.match(source, /setInterval/);
    assert.match(source, /visibilitychange/);
    assert.match(source, /cache:\s*"no-store"/);
  }
});

test("vacancies have list and grid modes and only the card owns the border", async () => {
  const page = await read("app/vacancies/page.tsx");
  const css = await read("app/globals.css");
  assert.match(page, /"list"\s*\|\s*"grid"/);
  assert.match(page, /vacancy-results--\$\{view\}/);
  assert.match(css, /\.vacancy-results\s*>\s*li\s*\{[^}]*border:\s*0\s*!important/);
  assert.match(css, /\.vacancy-results--grid\s*\{/);
});

test("select controls are custom, keyboard accessible and never native", async () => {
  const css = await read("app/globals.css");
  const component = await read("app/custom-select.tsx");
  assert.match(css, /\.custom-select__panel/);
  assert.match(component, /role="listbox"/);
  assert.match(component, /ArrowDown/);
  assert.match(component, /Escape/);
  for (const path of await readdir(new URL("../app/", import.meta.url), {recursive:true})) if (String(path).endsWith(".tsx")) assert.doesNotMatch(await read(`app/${path}`), /<select\b/, `native select in ${path}`);
});

test("advanced project and education filters are enforced server-side", async () => {
  const projects = await read("app/projects/page.tsx");
  const education = await read("app/education/page.tsx");
  assert.match(projects, /min_budget_kopecks/);
  assert.match(projects, /deadline_before/);
  assert.match(education, /max_duration_minutes/);

  const projectHTTP = await readFile(new URL("../../api/internal/projects/http.go", import.meta.url), "utf8");
  const projectSQL = await readFile(new URL("../../api/internal/projects/postgres.go", import.meta.url), "utf8");
  const serviceHTTP = await readFile(new URL("../../api/internal/services/http.go", import.meta.url), "utf8");
  const serviceSQL = await readFile(new URL("../../api/internal/services/postgres.go", import.meta.url), "utf8");
  assert.match(projectHTTP, /min_budget_kopecks/);
  assert.match(projectHTTP, /deadline_before/);
  assert.match(projectSQL, /budget_max_kopecks,p\.budget_min_kopecks/);
  assert.match(projectSQL, /p\.deadline_at IS NULL OR p\.deadline_at<=/);
  assert.match(serviceHTTP, /max_duration_minutes/);
  assert.match(serviceSQL, /ed\.duration_minutes>\$7/);
});
