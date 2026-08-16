import { AdminMetricsSkeleton, Skeleton } from "../skeletons";

export default function AdminRootLoading() {
  return (
    <div aria-label="Загрузка панели администратора">
      <div className="admin-header" style={{ marginBottom: 28 }}>
        <Skeleton height={14} width={160} rounded="sm" style={{ marginBottom: 8 }} />
        <Skeleton height={32} width={260} rounded="md" style={{ marginBottom: 10 }} />
        <Skeleton height={15} width="60%" rounded="sm" />
      </div>
      <AdminMetricsSkeleton count={12} />
    </div>
  );
}
