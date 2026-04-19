import { useApi } from "./lib/useApi";
import { api } from "./lib/api";
import {
  AreaChart, Area, BarChart, Bar, RadarChart, Radar, PolarGrid,
  PolarAngleAxis, PolarRadiusAxis, PieChart, Pie, Cell,
  XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer, Legend,
} from "recharts";
import StatCard from "./components/StatCard";
import SectionTitle from "./components/SectionTitle";
import Skeleton from "./components/Skeleton";
import ResultsTable from "./components/ResultsTable";

const COLORS = {
  done: "#00e5a0",
  failed: "#ff4d6d",
  pending: "#f5a623",
  accent: "#5b8dee",
  muted: "#7b8fa1",
};

const PIE_COLORS = ["#00e5a0", "#ff4d6d", "#f5a623", "#5b8dee", "#c084fc"];

export default function Dashboard({ runId }) {
  const params = [runId];

  const summary = useApi(() => api.summary(runId), params);
  const trend = useApi(() => api.trend(runId), params);
  const scoreDist = useApi(() => api.scoreDistribution(runId), params);
  const statusBreakdown = useApi(() => api.statusBreakdown(runId), params);
  const failureReasons = useApi(() => api.failureReasons(runId), params);
  const stagePerf = useApi(() => api.stagePerformance(runId), params);

  const s = summary.data;

  return (
    <div className="dashboard">
      {/* KPI Row */}
      <div className="kpi-grid">
        {summary.loading ? (
          Array(6).fill(0).map((_, i) => <Skeleton key={i} height={110} />)
        ) : s ? (
          <>
            <StatCard label="Total Records" value={s.total.toLocaleString()} icon="📋" accent="#5b8dee" />
            <StatCard label="Passed" value={s.done.toLocaleString()} icon="✅" accent={COLORS.done} sub={`${s.success_rate}%`} />
            <StatCard label="Failed" value={s.failed.toLocaleString()} icon="❌" accent={COLORS.failed} sub={`${((s.failed / s.total) * 100 || 0).toFixed(1)}%`} />
            <StatCard label="Avg Score" value={s.avg_score} icon="📊" accent="#c084fc" sub={`Max ${s.max_score}`} />
            <StatCard label="Avg Latency" value={`${s.avg_latency_ms}ms`} icon="⚡" accent={COLORS.pending} />
            <StatCard label="Unique Contacts" value={s.unique_contacts.toLocaleString()} icon="👥" accent={COLORS.muted} sub={`${s.unique_runs} run(s)`} />
          </>
        ) : null}
      </div>

      {/* Trend + Score Distribution */}
      <div className="chart-row two-col">
        <div className="chart-card">
          <SectionTitle>Validation Trend (Hourly)</SectionTitle>
          {trend.loading ? <Skeleton height={220} /> : (
            <ResponsiveContainer width="100%" height={220}>
              <AreaChart data={trend.data || []} margin={{ top: 10, right: 20, left: 0, bottom: 0 }}>
                <defs>
                  <linearGradient id="gDone" x1="0" y1="0" x2="0" y2="1">
                    <stop offset="5%" stopColor={COLORS.done} stopOpacity={0.3} />
                    <stop offset="95%" stopColor={COLORS.done} stopOpacity={0} />
                  </linearGradient>
                  <linearGradient id="gFailed" x1="0" y1="0" x2="0" y2="1">
                    <stop offset="5%" stopColor={COLORS.failed} stopOpacity={0.3} />
                    <stop offset="95%" stopColor={COLORS.failed} stopOpacity={0} />
                  </linearGradient>
                </defs>
                <CartesianGrid stroke="rgba(255,255,255,0.05)" />
                <XAxis dataKey="date" tick={{ fill: "#7b8fa1", fontSize: 11 }} />
                <YAxis tick={{ fill: "#7b8fa1", fontSize: 11 }} />
                <Tooltip contentStyle={{ background: "#13151f", border: "1px solid #2a2d3e", borderRadius: 8 }} />
                <Legend />
                <Area type="monotone" dataKey="done" stroke={COLORS.done} fill="url(#gDone)" strokeWidth={2} />
                <Area type="monotone" dataKey="failed" stroke={COLORS.failed} fill="url(#gFailed)" strokeWidth={2} />
              </AreaChart>
            </ResponsiveContainer>
          )}
        </div>

        <div className="chart-card">
          <SectionTitle>Score Distribution</SectionTitle>
          {scoreDist.loading ? <Skeleton height={220} /> : (
            <ResponsiveContainer width="100%" height={220}>
              <BarChart data={scoreDist.data || []} margin={{ top: 10, right: 20, left: 0, bottom: 0 }}>
                <CartesianGrid stroke="rgba(255,255,255,0.05)" vertical={false} />
                <XAxis dataKey="range" tick={{ fill: "#7b8fa1", fontSize: 11 }} />
                <YAxis tick={{ fill: "#7b8fa1", fontSize: 11 }} />
                <Tooltip contentStyle={{ background: "#13151f", border: "1px solid #2a2d3e", borderRadius: 8 }} />
                <Bar dataKey="count" fill="#5b8dee" radius={[4, 4, 0, 0]} />
              </BarChart>
            </ResponsiveContainer>
          )}
        </div>
      </div>

      {/* Status Pie + Stage Radar + Failure Reasons */}
      <div className="chart-row three-col">
        <div className="chart-card">
          <SectionTitle>Status Breakdown</SectionTitle>
          {statusBreakdown.loading ? <Skeleton height={220} /> : (
            <ResponsiveContainer width="100%" height={220}>
              <PieChart>
                <Pie
                  data={statusBreakdown.data || []}
                  dataKey="count"
                  nameKey="status"
                  cx="50%"
                  cy="50%"
                  innerRadius={55}
                  outerRadius={85}
                  paddingAngle={3}
                  label={({ status, percent }: any) => `${status} ${(percent * 100).toFixed(0)}%`}
                  labelLine={false}
                >
                  {(statusBreakdown.data || []).map((_, i) => (
                    <Cell key={i} fill={PIE_COLORS[i % PIE_COLORS.length]} />
                  ))}
                </Pie>
                <Tooltip contentStyle={{ background: "#13151f", border: "1px solid #2a2d3e", borderRadius: 8 }} />
              </PieChart>
            </ResponsiveContainer>
          )}
        </div>

        <div className="chart-card">
          <SectionTitle>Stage Pass Rate (%)</SectionTitle>
          {stagePerf.loading ? <Skeleton height={220} /> : (
            <ResponsiveContainer width="100%" height={220}>
              <RadarChart data={stagePerf.data || []}>
                <PolarGrid stroke="rgba(255,255,255,0.08)" />
                <PolarAngleAxis dataKey="stage" tick={{ fill: "#7b8fa1", fontSize: 11 }} />
                <PolarRadiusAxis angle={30} domain={[0, 100]} tick={{ fill: "#7b8fa1", fontSize: 10 }} />
                <Radar name="Pass Rate" dataKey="pass_rate" stroke="#5b8dee" fill="#5b8dee" fillOpacity={0.25} />
                <Tooltip contentStyle={{ background: "#13151f", border: "1px solid #2a2d3e", borderRadius: 8 }} />
              </RadarChart>
            </ResponsiveContainer>
          )}
        </div>

        <div className="chart-card">
          <SectionTitle>Top Failure Reasons</SectionTitle>
          {failureReasons.loading ? <Skeleton height={220} /> : (
            <div className="failure-list">
              {(failureReasons.data || []).slice(0, 8).map((f, i) => (
                <div key={i} className="failure-item">
                  <span className="failure-label">{f.reason}</span>
                  <div className="failure-bar-wrap">
                    <div
                      className="failure-bar"
                      style={{
                        width: `${Math.round((f.count / (failureReasons.data[0]?.count || 1)) * 100)}%`,
                      }}
                    />
                  </div>
                  <span className="failure-count">{f.count.toLocaleString()}</span>
                </div>
              ))}
            </div>
          )}
        </div>
      </div>

      {/* Stage Performance Table */}
      <div className="chart-card full-width">
        <SectionTitle>Stage Performance Details</SectionTitle>
        {stagePerf.loading ? <Skeleton height={160} /> : (
          <table className="perf-table">
            <thead>
              <tr>
                <th>Stage</th>
                <th>Avg Score</th>
                <th>Avg Latency</th>
                <th>Pass Rate</th>
                <th>Health</th>
              </tr>
            </thead>
            <tbody>
              {(stagePerf.data || []).map((row) => (
                <tr key={row.stage}>
                  <td className="stage-name">{row.stage}</td>
                  <td>{row.avg_score}</td>
                  <td>{row.avg_latency_ms > 0 ? `${row.avg_latency_ms}ms` : "—"}</td>
                  <td>
                    <span className="pass-rate" style={{ color: row.pass_rate >= 80 ? COLORS.done : row.pass_rate >= 50 ? COLORS.pending : COLORS.failed }}>
                      {row.pass_rate}%
                    </span>
                  </td>
                  <td>
                    <div className="health-bar-wrap">
                      <div
                        className="health-bar"
                        style={{
                          width: `${row.pass_rate}%`,
                          background: row.pass_rate >= 80 ? COLORS.done : row.pass_rate >= 50 ? COLORS.pending : COLORS.failed,
                        }}
                      />
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}

        <ResultsTable runId="" />
      </div>
    </div>
  );
}
