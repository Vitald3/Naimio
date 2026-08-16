import { ServiceDetailSkeleton } from "../../skeletons";

export default function ServiceDetailLoading() {
  return (
    <main aria-label="Загрузка услуги">
      <ServiceDetailSkeleton />
    </main>
  );
}
