import type { Metadata } from "next";
import { notFound } from "next/navigation";
import { missingMetadata, publicMetadata } from "../../seo";
import ReviewsSection, { type NativeTrust, type Review } from "../../reviews-section";
import Breadcrumbs from "../../breadcrumbs";

async function loadReviews(username: string): Promise<{ items: Review[]; trust: NativeTrust; nextCursor: string | null } | null> {
  const baseURL = process.env.API_BASE_URL ?? "http://localhost:8080";
  const response = await fetch(`${baseURL}/api/v1/profiles/${encodeURIComponent(username)}/reviews?limit=20`, { next: { revalidate: 120 } });
  if (response.status === 404) return null;
  if (!response.ok) throw new Error("reviews request failed");
  const body = await response.json();
  return { items: body.data ?? [], trust: body.trust ?? { reviews_count: 0, completed_projects_count: 0 }, nextCursor: body.page?.next_cursor ?? null };
}

function pickName(searchParams: Record<string, string | string[] | undefined>, username: string): string {
  const raw = searchParams?.name;
  const name = Array.isArray(raw) ? raw[0] : raw;
  return name && name.length <= 120 ? name : `@${username}`;
}

export async function generateMetadata({ params, searchParams }: { params: Promise<{ username: string }>; searchParams: Promise<Record<string, string | string[] | undefined>> }): Promise<Metadata> {
  const [{ username }, resolvedSearchParams] = await Promise.all([params, searchParams]);
  const data = await loadReviews(username);
  if (!data) return missingMetadata("Заказчик не найден");
  const name = pickName(resolvedSearchParams, username);
  return publicMetadata(`${name} — репутация заказчика`, "Отзывы исполнителей о работе с заказчиком, подтверждённые завершёнными безопасными сделками.", `/customers/${username}`);
}

export default async function CustomerReputationPage({ params, searchParams }: { params: Promise<{ username: string }>; searchParams: Promise<Record<string, string | string[] | undefined>> }) {
  const [{ username }, resolvedSearchParams] = await Promise.all([params, searchParams]);
  const data = await loadReviews(username);
  if (!data) notFound();
  const name = pickName(resolvedSearchParams, username);
  return (
    <main>
      <Breadcrumbs items={[{label:"Главная",href:"/"},{label:name}]}/>
      <header>
        <p>Репутация заказчика</p>
        <h1>{name}</h1>
        <p>
          Отзывы исполнителей о работе с этим заказчиком. Каждый отзыв подтверждён завершённой безопасной сделкой —
          оставить его вручную нельзя, поэтому репутация отражает реальный опыт сотрудничества.
        </p>
      </header>
      <ReviewsSection username={username} initial={data.items} trust={data.trust} initialNextCursor={data.nextCursor} subject="customer" />
    </main>
  );
}
