import { SafeDealSkeleton } from "../../../skeletons";

export default function SafeDealDetailLoading() {
  return (
    <main aria-label="Загрузка Безопасной сделки">
      <SafeDealSkeleton />
    </main>
  );
}
