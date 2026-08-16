import { NotificationsListSkeleton, Skeleton } from "../skeletons";

export default function NotificationsLoading() {
  return (
    <main aria-label="Загрузка уведомлений">
      <header className="page-heading">
        <div>
          <Skeleton height={32} width={180} rounded="md" style={{ marginBottom: 8 }} />
          <Skeleton height={14} width={140} rounded="sm" />
        </div>
      </header>
      <NotificationsListSkeleton count={6} />
    </main>
  );
}
