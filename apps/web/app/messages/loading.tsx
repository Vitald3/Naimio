import { MessengerSkeleton, Skeleton } from "../skeletons";

export default function MessagesLoading() {
  return (
    <main aria-label="Загрузка сообщений">
      <div className="page-heading">
        <div>
          <Skeleton height={14} width={100} rounded="sm" style={{ marginBottom: 8 }} />
          <Skeleton height={32} width={180} rounded="md" />
        </div>
      </div>
      <MessengerSkeleton />
    </main>
  );
}
