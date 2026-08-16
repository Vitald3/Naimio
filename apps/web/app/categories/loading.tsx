import { CategoriesCatalogSkeleton, Skeleton } from "../skeletons";

export default function CategoriesLoading() {
  return (
    <main aria-label="Загрузка категорий">
      <div className="page-heading">
        <div>
          <Skeleton height={14} width={150} rounded="sm" style={{ marginBottom: 8 }} />
          <Skeleton height={32} width={240} rounded="md" style={{ marginBottom: 10 }} />
          <Skeleton height={16} width="60%" rounded="sm" />
        </div>
      </div>
      <CategoriesCatalogSkeleton count={8} />
    </main>
  );
}
