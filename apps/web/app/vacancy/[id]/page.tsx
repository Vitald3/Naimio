import { permanentRedirect } from "next/navigation";

export default async function LegacyVacancyPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  permanentRedirect(`/vacancies/${encodeURIComponent(id)}`);
}
