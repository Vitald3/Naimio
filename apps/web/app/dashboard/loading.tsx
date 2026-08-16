import { DashboardOverviewSkeleton } from "../skeletons";

export default function DashboardLoading() {
  return (
    <main aria-label="Загрузка личного кабинета">
      <DashboardOverviewSkeleton />
    </main>
  );
}
