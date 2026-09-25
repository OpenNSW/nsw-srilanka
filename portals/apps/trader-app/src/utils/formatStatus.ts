// Best-effort fallback for backend status codes that don't have a curated
// translation (e.g. workflow-engine states like PENDING_USER, OGA_REVIEWING
// — these are defined per workflow artifact, not a fixed enum the frontend
// can exhaustively map). Turns SCREAMING_SNAKE_CASE into "Title Case With
// Spaces" so a badge never shows a raw backend code, even for a status this
// app has never seen before.
export function humanizeStatus(status: string): string {
  return status
    .toLowerCase()
    .replace(/_/g, ' ')
    .replace(/\b\w/g, (c) => c.toUpperCase())
}
