import { ProposalsListSkeleton, Skeleton } from "../../../../skeletons";

export default function ProposalsLoading() {
  return (
    <main aria-label="Загрузка откликов">
      <header className="page-heading">
        <div>
          <Skeleton height={32} width={220} rounded="md" style={{ marginBottom: 8 }} />
          <Skeleton height={14} width={280} rounded="sm" />
        </div>
      </header>
      <ProposalsListSkeleton count={4} />
    </main>
  );
}
