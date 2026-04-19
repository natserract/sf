import StatusBadge from "./StatusBadge";

const STAGES = [
  { key: "syntax", label: "Syntax" },
  { key: "domain_dns", label: "Domain DNS" },
  { key: "mx", label: "MX" },
  { key: "smtp", label: "SMTP" },
  { key: "history", label: "History" },
];

export default function DetailDrawer({ row, onClose }) {
  if (!row) return null;

  const scoreColor = (s) =>
    s >= 80 ? "#00e5a0" : s >= 50 ? "#f5a623" : "#ff4d6d";

  return (
    <>
      <div className="drawer-overlay" onClick={onClose} />
      <div className="drawer">
        <div className="drawer-header">
          <div>
            <div className="drawer-title">Validation Detail</div>
            <div className="drawer-sub">Row #{row.row_number} · {row.contact_id}</div>
          </div>
          <button className="drawer-close" onClick={onClose}>✕</button>
        </div>

        <div className="drawer-body">
          {/* Main info */}
          <section className="drawer-section">
            <div className="drawer-kv">
              <span>Run ID</span><code>{row.run_id}</code>
            </div>
            <div className="drawer-kv">
              <span>Raw Key</span><code>{row.raw_contact_key}</code>
            </div>
            <div className="drawer-kv">
              <span>Clean Candidate</span><code>{row.clean_candidate || "—"}</code>
            </div>
            <div className="drawer-kv">
              <span>Normalized Email</span><code>{row.normalized_email || "—"}</code>
            </div>
            <div className="drawer-kv">
              <span>Status</span><StatusBadge status={row.status} />
            </div>
            {row.failure_reason && (
              <div className="drawer-kv">
                <span>Failure Reason</span>
                <span className="drawer-reason">{row.failure_reason}</span>
              </div>
            )}
          </section>

          {/* Score overview */}
          <section className="drawer-section">
            <div className="drawer-section-title">Total Score</div>
            <div className="drawer-score-hero" style={{ color: scoreColor(row.total_score) }}>
              {row.total_score}
            </div>
          </section>

          {/* Per-stage */}
          <section className="drawer-section">
            <div className="drawer-section-title">Stage Results</div>
            <div className="stage-grid">
              {STAGES.map(({ key, label }) => {
                const status = row[`${key}_status`];
                const score = row[`${key}_score`];
                const reason = row[`${key}_reason`];
                const latency = row[`${key}_latency_ms`];
                return (
                  <div key={key} className="stage-card">
                    <div className="stage-card-label">{label}</div>
                    <StatusBadge status={status} />
                    <div className="stage-card-score" style={{ color: scoreColor(score) }}>{score} pts</div>
                    {latency > 0 && <div className="stage-card-latency">{latency}ms</div>}
                    {reason && <div className="stage-card-reason">{reason}</div>}
                  </div>
                );
              })}
            </div>
          </section>

          {/* Timestamps */}
          <section className="drawer-section">
            <div className="drawer-kv">
              <span>Created At</span><span>{new Date(row.created_at).toLocaleString()}</span>
            </div>
            <div className="drawer-kv">
              <span>Updated At</span><span>{row.updated_at ? new Date(row.updated_at).toLocaleString() : "—"}</span>
            </div>
          </section>
        </div>
      </div>
    </>
  );
}
