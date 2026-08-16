import { VacancyDetailSkeleton } from "../../skeletons";

export default function VacancyDetailLoading() {
  return (
    <main aria-label="Загрузка вакансии">
      <VacancyDetailSkeleton />
    </main>
  );
}
