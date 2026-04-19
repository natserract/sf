export default function ScoreBar({ score }) {
  const pct = Math.min(100, Math.max(0, score));
  const color =
    pct >= 80 ? "#00e5a0" : pct >= 50 ? "#f5a623" : "#ff4d6d";

  return (
    <div className="score-bar-wrap" title={`Score: ${score}`}>
      <div
        className="score-bar-fill"
        style={{ width: `${pct}%`, background: color }}
      />
      <span className="score-bar-label">{score}</span>
    </div>
  );
}
