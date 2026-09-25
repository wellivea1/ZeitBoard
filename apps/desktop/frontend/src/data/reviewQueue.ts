export interface ReviewQueueSummary {
  pendingCount: number;
  nextExpiryAt: string;
}

export interface ReviewQueuePagination {
  nextCursor: string;
  hasMore: boolean;
}

export const emptyReviewSummary: ReviewQueueSummary = { pendingCount: 0, nextExpiryAt: "" };
export const terminalReviewPage = (): ReviewQueuePagination => ({ nextCursor: "", hasMore: false });

export function normalizeReviewSummary(
  value: Record<string, unknown>,
): ReviewQueueSummary | undefined {
  const { pendingCount, nextExpiryAt } = value;
  if (
    typeof pendingCount !== "number" ||
    !Number.isSafeInteger(pendingCount) ||
    pendingCount < 0 ||
    typeof nextExpiryAt !== "string" ||
    (nextExpiryAt !== "" && !Number.isFinite(Date.parse(nextExpiryAt)))
  )
    return undefined;
  return { pendingCount, nextExpiryAt };
}

export function normalizeReviewPagination(value: unknown): ReviewQueuePagination | undefined {
  if (typeof value !== "object" || value === null) return undefined;
  const { nextCursor, hasMore } = value as Record<string, unknown>;
  if (
    typeof nextCursor !== "string" ||
    typeof hasMore !== "boolean" ||
    hasMore !== nextCursor.length > 0
  )
    return undefined;
  return { nextCursor, hasMore };
}

export function reviewIsPending(
  item: { status: string; decisionToken?: string; expiresAt: string },
  now = Date.now(),
) {
  return (
    item.status === "pending" && Boolean(item.decisionToken) && Date.parse(item.expiresAt) > now
  );
}

export function reviewStatus(
  item: { status: string; decisionToken?: string; expiresAt: string },
  now = Date.now(),
) {
  if (item.status !== "pending") return item.status;
  if (Date.parse(item.expiresAt) <= now) return "expired";
  return item.decisionToken ? "pending" : "unavailable";
}

export const reviewQueueChangedEvent = "zeitboard:review-queue-changed";
export function notifyReviewQueueChanged() {
  window.dispatchEvent(new Event(reviewQueueChangedEvent));
}
