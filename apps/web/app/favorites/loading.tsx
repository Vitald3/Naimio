import { FavoritesSkeleton, Skeleton } from "../skeletons";

export default function FavoritesLoading() {
  return (
    <main className="favorites-page" aria-label="Загрузка избранного">
      <header className="favorites-hero">
        <div>
          <Skeleton height={14} width={130} rounded="sm" style={{ marginBottom: 8 }} />
          <Skeleton height={32} width={180} rounded="md" style={{ marginBottom: 10 }} />
          <Skeleton height={16} width="60%" rounded="sm" />
        </div>
      </header>
      <FavoritesSkeleton count={6} />
    </main>
  );
}
