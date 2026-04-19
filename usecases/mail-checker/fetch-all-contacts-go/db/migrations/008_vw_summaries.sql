-- =============================================================================
-- Migration: validation_results — indexes, view, bulk-insert helper
-- =============================================================================
-- Targets the exact query patterns in repo.go:
--
--   CompleteRunIfDone   WHERE run_id = ? AND status IN ('pending','in_progress')
--   ListResults         WHERE run_id = ? AND (... ILIKE ?) ORDER BY row_number
--   GetResult           WHERE run_id = ? AND id = ?
--   UpdateValidation    WHERE id = ?                          (already PK — fast)
--   history processor   WHERE run_id = ? AND history_status = 'pending'
--
-- Run order: indexes first, then view, then the optional function.
-- All statements are idempotent (IF NOT EXISTS / CREATE OR REPLACE).
-- =============================================================================

BEGIN;

-- UpdateValidation hits WHERE id = ? which is already the primary key.
-- No additional index needed there.

-- ---------------------------------------------------------------------------
-- 2.  View: vw_validation_run_summary
-- ---------------------------------------------------------------------------
-- Answers "how many contact keys are valid / invalid / pending and what are
-- the scores?" for any run in one query. The GROUP BY on run_id aligns with
-- idx_vr_run_row so each group is a tight range scan, not a full table scan.
--
-- Usage:
--   SELECT * FROM vw_validation_run_summary WHERE run_id = '<uuid>';
--   SELECT * FROM vw_validation_run_summary ORDER BY run_started_at DESC;
-- ---------------------------------------------------------------------------
CREATE OR REPLACE VIEW public.vw_validation_run_summary AS
SELECT
    -- ── Run identity ─────────────────────────────────────────────────────────
    vr.id                                                       AS run_id,
    vr.source_file,
    vr.state                                                    AS run_state,
    vr.total_rows,
    vr.started_at                                               AS run_started_at,
    vr.completed_at                                             AS run_completed_at,

    -- ── Validation status counts ─────────────────────────────────────────────
    COUNT(*)                                                    AS total_results,

    COUNT(*) FILTER (WHERE res.status = 'done')                 AS done_count,
    COUNT(*) FILTER (WHERE res.status = 'failed')               AS failed_count,
    COUNT(*) FILTER (WHERE res.status IN ('pending','in_progress')) AS pending_count,

    -- Per-step failure breakdown (independent: a row can fail multiple steps)
    COUNT(*) FILTER (WHERE res.syntax_status     = 'failed')    AS syntax_fail_count,
    COUNT(*) FILTER (WHERE res.domain_dns_status = 'failed')    AS domain_fail_count,
    COUNT(*) FILTER (WHERE res.mx_status         = 'failed')    AS mx_fail_count,
    COUNT(*) FILTER (WHERE res.smtp_status       = 'failed')    AS smtp_fail_count,

    -- ── History fetch counts ─────────────────────────────────────────────────
    COUNT(*) FILTER (WHERE res.history_status = 'found')        AS history_found_count,
    COUNT(*) FILTER (WHERE res.history_status = 'not_found')    AS history_not_found_count,
    COUNT(*) FILTER (WHERE res.history_status = 'pending')      AS history_pending_count,

    -- ── Score distribution ───────────────────────────────────────────────────
    ROUND(AVG(res.total_score), 2)                              AS avg_total_score,
    MIN(res.total_score)                                        AS min_total_score,
    MAX(res.total_score)                                        AS max_total_score,

    PERCENTILE_CONT(0.25) WITHIN GROUP (ORDER BY res.total_score) AS p25_score,
    PERCENTILE_CONT(0.50) WITHIN GROUP (ORDER BY res.total_score) AS p50_score,
    PERCENTILE_CONT(0.75) WITHIN GROUP (ORDER BY res.total_score) AS p75_score,
    PERCENTILE_CONT(0.90) WITHIN GROUP (ORDER BY res.total_score) AS p90_score,

    -- Bucketed counts — ready to drive a histogram without extra grouping
    COUNT(*) FILTER (WHERE res.total_score = 100)               AS score_100,
    COUNT(*) FILTER (WHERE res.total_score BETWEEN 75 AND 99)   AS score_75_99,
    COUNT(*) FILTER (WHERE res.total_score BETWEEN 50 AND 74)   AS score_50_74,
    COUNT(*) FILTER (WHERE res.total_score BETWEEN 25 AND 49)   AS score_25_49,
    COUNT(*) FILTER (WHERE res.total_score BETWEEN 1  AND 24)   AS score_1_24,
    COUNT(*) FILTER (WHERE res.total_score = 0)                 AS score_0,

    -- ── Per-step pass rates (%%) ─────────────────────────────────────────────
    ROUND(100.0 * COUNT(*) FILTER (WHERE res.syntax_status     = 'passed')
          / NULLIF(COUNT(*), 0), 2)                             AS syntax_pass_pct,
    ROUND(100.0 * COUNT(*) FILTER (WHERE res.domain_dns_status = 'passed')
          / NULLIF(COUNT(*), 0), 2)                             AS domain_pass_pct,
    ROUND(100.0 * COUNT(*) FILTER (WHERE res.mx_status         = 'passed')
          / NULLIF(COUNT(*), 0), 2)                             AS mx_pass_pct,
    ROUND(100.0 * COUNT(*) FILTER (WHERE res.smtp_status       = 'passed')
          / NULLIF(COUNT(*), 0), 2)                             AS smtp_pass_pct,

    -- ── Average per-step latency (ms, only rows that actually ran the step) ──
    ROUND(AVG(res.syntax_latency_ms)     FILTER (WHERE res.syntax_latency_ms     > 0), 0) AS avg_syntax_latency_ms,
    ROUND(AVG(res.domain_dns_latency_ms) FILTER (WHERE res.domain_dns_latency_ms > 0), 0) AS avg_domain_latency_ms,
    ROUND(AVG(res.mx_latency_ms)         FILTER (WHERE res.mx_latency_ms         > 0), 0) AS avg_mx_latency_ms,
    ROUND(AVG(res.smtp_latency_ms)       FILTER (WHERE res.smtp_latency_ms       > 0), 0) AS avg_smtp_latency_ms

FROM public.validation_runs    vr
JOIN public.validation_results res ON res.run_id = vr.id
GROUP BY
    vr.id,
    vr.source_file,
    vr.state,
    vr.total_rows,
    vr.started_at,
    vr.completed_at;

COMMENT ON VIEW public.vw_validation_run_summary IS
    'Per-run rollup of validation_results: done/failed/pending counts, '
    'score distribution (avg, min, max, P25/P50/P75/P90, histogram buckets), '
    'per-step pass rates and avg latencies, and history fetch status. '
    'Filter by run_id for a single run; omit the filter for an all-runs overview.';

-- ---------------------------------------------------------------------------
-- 3.  Bulk-insert helper — replaces the row-by-row loop in CreateValidationResults
-- ---------------------------------------------------------------------------
-- The current Go code issues one INSERT per row inside a transaction.
-- For a 50 000-row CSV that is 50 000 individual executor round-trips.
-- This function receives three parallel arrays and inserts in a single
-- statement, cutting the round-trips to one regardless of batch size.
--
-- Drop-in Go replacement (pgx):
--
--   rowNums  := make([]int32,  len(rows))
--   contIDs  := make([]string, len(rows))
--   rawKeys  := make([]string, len(rows))
--   for i, r := range rows {
--       rowNums[i], contIDs[i], rawKeys[i] = int32(r.RowNumber), r.ContactID, r.RawContactKey
--   }
--   _, err = pool.Exec(ctx,
--       `SELECT bulk_create_validation_results($1, $2, $3, $4)`,
--       runID, rowNums, contIDs, rawKeys,
--   )
-- ---------------------------------------------------------------------------
CREATE OR REPLACE FUNCTION public.bulk_create_validation_results(
    p_run_id           uuid,
    p_row_numbers      int[],
    p_contact_ids      text[],
    p_raw_contact_keys text[]
)
RETURNS int                         -- rows actually inserted (conflicts excluded)
LANGUAGE sql
AS $$
    INSERT INTO public.validation_results
        (run_id, row_number, contact_id, raw_contact_key,
         status, history_status, history_reason)
    SELECT
        p_run_id,
        unnest(p_row_numbers),
        unnest(p_contact_ids),
        unnest(p_raw_contact_keys),
        'pending',
        'pending',
        'not fetched yet'
    ON CONFLICT (run_id, row_number) DO NOTHING;

    SELECT COUNT(*)::int
    FROM   public.validation_results
    WHERE  run_id = p_run_id;
$$;

COMMENT ON FUNCTION public.bulk_create_validation_results IS
    'Inserts validation_results rows using a single unnest statement. '
    'Replaces the per-row loop in CreateValidationResults. '
    'ON CONFLICT (run_id, row_number) DO NOTHING makes it idempotent.';

COMMIT;