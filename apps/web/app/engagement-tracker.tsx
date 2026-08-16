"use client";

import { useEffect } from "react";

type EventType = "PROFILE_VIEW" | "PORTFOLIO_VIEW" | "SERVICE_VIEW";

// The API deduplicates one view per viewer/entity/day. This component sends no
// page content or behavioural data, and analytics never blocks the public UI.
export default function EngagementTracker({ eventType, subjectUserID, entityID }: { eventType: EventType; subjectUserID: string; entityID?: string }) {
  useEffect(() => {
    if (!subjectUserID) return;
    void fetch("/api/v1/engagement/events", {
      method: "POST",
      credentials: "same-origin",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ event_type: eventType, subject_user_id: subjectUserID, entity_id: entityID ?? "" }),
    }).catch(() => undefined);
  }, [entityID, eventType, subjectUserID]);
  return null;
}
