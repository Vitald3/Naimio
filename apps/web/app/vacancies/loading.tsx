import { Skeleton, VacanciesCatalogSkeleton } from "../skeletons";

export default function VacanciesLoading() {
  return (
    <main aria-label="Загрузка вакансий">
      <div className="page-heading">
        <div>
          <Skeleton height={14} width={130} rounded="sm" style={{ marginBottom: 8 }} />
          <Skeleton height={32} width={190} rounded="md" style={{ marginBottom: 10 }} />
          <Skeleton height={16} width="60%" rounded="sm" />
        </div>
      </div>
      <VacanciesCatalogSkeleton count={6} />
    </main>
  );
}
