import { Skeleton, SkeletonCard, SkeletonText } from "./skeletons";

export default function Loading() {
  return (
    <main aria-label="Загрузка страницы" aria-busy="true" style={{ maxWidth: 1160, margin: "0 auto", padding: "32px 16px" }}>
      <div style={{ marginBottom: 32 }}>
        <Skeleton height={14} width={120} rounded="sm" style={{ marginBottom: 12 }} />
        <Skeleton height={36} width="60%" rounded="md" style={{ marginBottom: 12 }} />
        <Skeleton height={16} width="40%" rounded="sm" />
      </div>
      <div className="catalog-skeleton" style={{ display: "grid", gridTemplateColumns: "repeat(auto-fill, minmax(320px, 1fr))", gap: 20 }}>
        {Array.from({ length: 6 }).map((_, i) => (
          <SkeletonCard key={i} style={{ padding: 24, borderRadius: 16, border: "1px solid var(--line)" }}>
            <Skeleton height={20} width="70%" rounded="sm" style={{ marginBottom: 12 }} />
            <SkeletonText lines={3} widths={["100%", "92%", "65%"]} size="sm" style={{ marginBottom: 16 }} />
            <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", marginTop: "auto" }}>
              <Skeleton height={16} width={90} rounded="sm" />
              <Skeleton height={32} width={100} rounded="full" />
            </div>
          </SkeletonCard>
        ))}
      </div>
    </main>
  );
}
