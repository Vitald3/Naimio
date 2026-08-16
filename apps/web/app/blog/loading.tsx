import { BlogCatalogSkeleton, Skeleton } from "../skeletons";

export default function BlogLoading() {
  return (
    <main className="blog-page" aria-label="Загрузка статей блога">
      <header className="blog-hero">
        <div>
          <Skeleton height={14} width={120} rounded="sm" style={{ marginBottom: 8 }} />
          <Skeleton height={32} width={280} rounded="md" style={{ marginBottom: 10 }} />
          <Skeleton height={16} width="60%" rounded="sm" />
        </div>
      </header>
      <BlogCatalogSkeleton count={4} />
    </main>
  );
}
