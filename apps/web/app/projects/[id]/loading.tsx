import { ProjectDetailSkeleton } from "../../skeletons";

export default function ProjectDetailLoading() {
  return (
    <main aria-label="Загрузка проекта">
      <ProjectDetailSkeleton />
    </main>
  );
}
