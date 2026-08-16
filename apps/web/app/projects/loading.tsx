import { ProjectsCatalogSkeleton, Skeleton } from "../skeletons";

export default function ProjectsLoading() {
  return (
    <main aria-label="Загрузка проектов">
      <div className="page-heading">
        <div>
          <Skeleton height={14} width={120} rounded="sm" style={{ marginBottom: 8 }} />
          <Skeleton height={32} width={200} rounded="md" style={{ marginBottom: 10 }} />
          <Skeleton height={16} width="60%" rounded="sm" />
        </div>
      </div>
      <ProjectsCatalogSkeleton count={6} />
    </main>
  );
}
