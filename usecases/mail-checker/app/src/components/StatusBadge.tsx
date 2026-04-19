type Props = { status?: string; mini?: boolean };

export default function StatusBadge({ status, mini }: Props) {
  if (!status) return <span className="badge badge-empty">—</span>;
  const map: Record<string, string> = {
    done: "badge-done",
    pass: "badge-done",
    failed: "badge-failed",
    fail: "badge-failed",
    pending: "badge-pending",
    error: "badge-error",
    skip: "badge-skip",
  };
  const cls = map[status.toLowerCase()] || "badge-default";
  return (
    <span className={`badge ${cls} ${mini ? "badge-mini" : ""}`}>
      {mini ? status[0]?.toUpperCase() : status}
    </span>
  );
}
