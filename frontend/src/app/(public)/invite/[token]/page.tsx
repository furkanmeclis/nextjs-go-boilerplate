import type { Metadata } from "next";

import { InviteAcceptPage } from "@/features/tenant";

// The token is in this URL: never leak it to other origins via Referer.
export const metadata: Metadata = { referrer: "no-referrer" };

type PageProps = {
  params: Promise<{ token: string }>;
};

export default async function InvitePage({ params }: PageProps) {
  const { token } = await params;
  return <InviteAcceptPage token={token} />;
}
