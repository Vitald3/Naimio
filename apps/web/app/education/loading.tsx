import { EducationCatalogSkeleton, Skeleton } from "../skeletons";

export default function EducationLoading() {
  return (
    <main aria-label="Загрузка обучающих материалов">
      <div className="page-heading">
        <div>
          <Skeleton height={14} width={140} rounded="sm" style={{ marginBottom: 8 }} />
          <Skeleton height={32} width={200} rounded="md" style={{ marginBottom: 10 }} />
          <Skeleton height={16} width="60%" rounded="sm" />
        </div>
      </div>
      <EducationCatalogSkeleton count={6} />
    </main>
  );
}
