type Props = {
  label: string;
  value: string;
  icon: string;
  accent?: string;
  sub?: React.ReactNode;
};

export default function StatCard({ label, value, icon, accent = "#5b8dee", sub }: Props) {
  return (
    <div className="stat-card" style={{ ["--accent" as any]: accent }}>
      <div className="stat-icon">{icon}</div>
      <div className="stat-body">
        <div className="stat-label">{label}</div>
        <div className="stat-value">{value}</div>
        {sub && <div className="stat-sub">{sub}</div>}
      </div>
      <div className="stat-glow" />
    </div>
  );
}
