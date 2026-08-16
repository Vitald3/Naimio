import { permanentRedirect } from "next/navigation";

export default async function LegacyServicePage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  permanentRedirect(`/services/${encodeURIComponent(id)}`);
}
