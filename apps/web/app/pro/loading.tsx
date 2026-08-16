import { PricingPlansSkeleton, Skeleton } from "../skeletons";

export default function ProLoading() {
  return (
    <main className="pro-page" aria-label="Загрузка планов PRO">
      <header className="pro-hero">
        <div>
          <Skeleton height={14} width={180} rounded="sm" style={{ marginBottom: 10 }} />
          <Skeleton height={36} width={220} rounded="md" style={{ marginBottom: 12 }} />
          <Skeleton height={16} width="65%" rounded="sm" />
        </div>
      </header>
      <PricingPlansSkeleton count={2} />
    </main>
  );
}
