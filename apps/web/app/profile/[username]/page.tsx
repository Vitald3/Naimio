import { permanentRedirect } from "next/navigation";

export default async function LegacyProfilePage({ params }: { params: Promise<{ username: string }> }) {
  const { username } = await params;
  permanentRedirect(`/freelancers/${encodeURIComponent(username)}`);
}
