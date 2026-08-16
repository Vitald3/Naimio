import { ServicesCatalogSkeleton, Skeleton } from "../skeletons";

export default function ServicesLoading() {
  return (
    <main aria-label="Загрузка услуг">
      <div className="page-heading">
        <div>
          <Skeleton height={14} width={140} rounded="sm" style={{ marginBottom: 8 }} />
          <Skeleton height={32} width={180} rounded="md" style={{ marginBottom: 10 }} />
          <Skeleton height={16} width="60%" rounded="sm" />
        </div>
      </div>
      <ServicesCatalogSkeleton count={6} />
    </main>
  );
}
