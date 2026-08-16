import { FreelancersCatalogSkeleton, Skeleton } from "../skeletons";

export default function FreelancersLoading() {
  return (
    <main aria-label="Загрузка специалистов">
      <div className="page-heading">
        <div>
          <Skeleton height={14} width={130} rounded="sm" style={{ marginBottom: 8 }} />
          <Skeleton height={32} width={220} rounded="md" style={{ marginBottom: 10 }} />
          <Skeleton height={16} width="60%" rounded="sm" />
        </div>
      </div>
      <FreelancersCatalogSkeleton count={6} />
    </main>
  );
}
