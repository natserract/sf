import { useState, useCallback, useRef, useEffect } from "react";
import { useApi } from "../lib/useApi";
import { api } from "../lib/api";
import StatusBadge from "./StatusBadge"
import ScoreBar from "./ScoreBar"
import Skeleton from "./Skeleton"
import DetailDrawer from "./DetailDrawer";

const PAGE_SIZE = 100;

const COLUMNS = [
  { key: "row_number", label: "#", width: 60 },
  { key: "contact_id", label: "Contact ID", width: 160 },
  { key: "normalized_email", label: "Email", width: 220 },
  { key: "status", label: "Status", width: 100 },
  { key: "total_score", label: "Score", width: 120 },
  { key: "syntax_status", label: "Syntax", width: 90 },
  { key: "domain_dns_status", label: "DNS", width: 90 },
  { key: "mx_status", label: "MX", width: 90 },
  { key: "smtp_status", label: "SMTP", width: 90 },
  { key: "failure_reason", label: "Failure Reason", width: 200 },
  { key: "created_at", label: "Created", width: 160 },
];

const ROW_HEIGHT = 44;
const VISIBLE_BUFFER = 10;

export default function ResultsTable({ runId }) {
  const [page, setPage] = useState(1);
  const [search, setSearch] = useState("");
  const [searchInput, setSearchInput] = useState("");
  const [statusFilter, setStatusFilter] = useState("");
  const [sortBy, setSortBy] = useState("id");
  const [sortDir, setSortDir] = useState("DESC");
  const [selected, setSelected] = useState(null);

  // scroll virtualization
  const containerRef = useRef(null);
  const [scrollTop, setScrollTop] = useState(0);

  const params = [runId, page, search, statusFilter, sortBy, sortDir];

  const { data, loading, error } = useApi(
    () =>
      api.results({
        run_id: runId,
        page,
        page_size: PAGE_SIZE,
        search,
        status: statusFilter,
        sort_by: sortBy,
        sort_dir: sortDir,
      }),
    params
  );

  // reset page when filters change
  useEffect(() => { setPage(1); }, [runId, search, statusFilter, sortBy, sortDir]);

  const rows = data?.data || [];
  const total = data?.total || 0;
  const totalPages = data?.total_pages || 1;

  // Virtual scroll calculation
  const visibleCount = containerRef.current
    ? Math.ceil(containerRef.current.clientHeight / ROW_HEIGHT) + VISIBLE_BUFFER * 2
    : 30;
  const startIdx = Math.max(0, Math.floor(scrollTop / ROW_HEIGHT) - VISIBLE_BUFFER);
  const endIdx = Math.min(rows.length, startIdx + visibleCount);
  const visibleRows = rows.slice(startIdx, endIdx);
  const paddingTop = startIdx * ROW_HEIGHT;
  const paddingBottom = (rows.length - endIdx) * ROW_HEIGHT;

  const handleSort = (col) => {
    if (sortBy === col) {
      setSortDir((d) => (d === "ASC" ? "DESC" : "ASC"));
    } else {
      setSortBy(col);
      setSortDir("DESC");
    }
  };

  const handleSearch = useCallback(() => {
    setSearch(searchInput);
  }, [searchInput]);

  const formatDate = (d) => {
    if (!d) return "—";
    return new Date(d).toLocaleString("en-US", {
      month: "short", day: "numeric",
      hour: "2-digit", minute: "2-digit",
    });
  };

  return (
    <div className="results-page">
      {/* Toolbar */}
      <div className="results-toolbar">
        <div className="search-group">
          <input
            className="search-input"
            placeholder="Search by email, contact ID…"
            value={searchInput}
            onChange={(e) => setSearchInput(e.target.value)}
            onKeyDown={(e) => e.key === "Enter" && handleSearch()}
          />
          <button className="search-btn" onClick={handleSearch}>Search</button>
        </div>

        <select
          className="status-select"
          value={statusFilter}
          onChange={(e) => setStatusFilter(e.target.value)}
        >
          <option value="">All Statuses</option>
          <option value="done">Done</option>
          <option value="failed">Failed</option>
          <option value="pending">Pending</option>
        </select>

        <span className="results-count">
          {total.toLocaleString()} records
        </span>
      </div>

      {/* Table */}
      <div className="table-wrapper">
        <div className="table-head">
          <table style={{ tableLayout: "fixed", width: "100%" }}>
            <colgroup>
              {COLUMNS.map((c) => (
                <col key={c.key} style={{ width: c.width }} />
              ))}
            </colgroup>
            <thead>
              <tr>
                {COLUMNS.map((col) => (
                  <th
                    key={col.key}
                    onClick={() => handleSort(col.key)}
                    className={`th-cell ${sortBy === col.key ? "th-sorted" : ""}`}
                  >
                    {col.label}
                    {sortBy === col.key && (
                      <span className="sort-icon">{sortDir === "ASC" ? " ↑" : " ↓"}</span>
                    )}
                  </th>
                ))}
              </tr>
            </thead>
          </table>
        </div>

        <div
          ref={containerRef}
          className="table-body-scroll"
          onScroll={(e) => setScrollTop(e.currentTarget.scrollTop)}
        >
          {loading ? (
            <div className="table-loading">
              {Array(12).fill(0).map((_, i) => (
                <Skeleton key={i} height={ROW_HEIGHT - 4} style={{ marginBottom: 4 }} />
              ))}
            </div>
          ) : error ? (
            <div className="table-error">⚠ {error}</div>
          ) : rows.length === 0 ? (
            <div className="table-empty">No records found.</div>
          ) : (
            <table style={{ tableLayout: "fixed", width: "100%", borderCollapse: "collapse" }}>
              <colgroup>
                {COLUMNS.map((c) => (
                  <col key={c.key} style={{ width: c.width }} />
                ))}
              </colgroup>
              <tbody>
                {paddingTop > 0 && (
                  <tr style={{ height: paddingTop }}>
                    <td colSpan={COLUMNS.length} />
                  </tr>
                )}
                {visibleRows.map((row) => (
                  <tr
                    key={row.id}
                    className={`tr-row ${row.status === "failed" ? "tr-failed" : ""}`}
                    onClick={() => setSelected(row)}
                  >
                    <td className="td-cell td-muted">{row.row_number}</td>
                    <td className="td-cell td-mono">{row.contact_id}</td>
                    <td className="td-cell td-mono">{row.normalized_email || row.raw_contact_key}</td>
                    <td className="td-cell">
                      <StatusBadge status={row.status} />
                    </td>
                    <td className="td-cell">
                      <ScoreBar score={row.total_score} />
                    </td>
                    <td className="td-cell"><StatusBadge status={row.syntax_status} mini /></td>
                    <td className="td-cell"><StatusBadge status={row.domain_dns_status} mini /></td>
                    <td className="td-cell"><StatusBadge status={row.mx_status} mini /></td>
                    <td className="td-cell"><StatusBadge status={row.smtp_status} mini /></td>
                    <td className="td-cell td-reason">{row.failure_reason || "—"}</td>
                    <td className="td-cell td-muted">{formatDate(row.created_at)}</td>
                  </tr>
                ))}
                {paddingBottom > 0 && (
                  <tr style={{ height: paddingBottom }}>
                    <td colSpan={COLUMNS.length} />
                  </tr>
                )}
              </tbody>
            </table>
          )}
        </div>
      </div>

      {/* Pagination */}
      <div className="pagination">
        <button
          className="page-btn"
          disabled={page <= 1}
          onClick={() => setPage(1)}
        >«</button>
        <button
          className="page-btn"
          disabled={page <= 1}
          onClick={() => setPage((p) => p - 1)}
        >‹</button>

        <span className="page-info">
          Page <strong>{page}</strong> of <strong>{totalPages}</strong>
        </span>

        <button
          className="page-btn"
          disabled={page >= totalPages}
          onClick={() => setPage((p) => p + 1)}
        >›</button>
        <button
          className="page-btn"
          disabled={page >= totalPages}
          onClick={() => setPage(totalPages)}
        >»</button>

        <span className="page-size-info">
          {PAGE_SIZE} / page
        </span>
      </div>

      {/* Detail Drawer */}
      {selected && (
        <DetailDrawer row={selected} onClose={() => setSelected(null)} />
      )}
    </div>
  );
}
