-- +goose NO TRANSACTION
-- ---------------------------------------------------------------------------
-- 1.  Indexes on validation_results
-- ---------------------------------------------------------------------------

-- 1a. Primary workhorse: drives ListResults ORDER BY row_number + its count
--     query, and the history processor batch claim.
--     Postgres satisfies both the WHERE and the ORDER BY from this index
--     without a separate sort step.
CREATE INDEX IF NOT EXISTS idx_vr_run_row
    ON public.validation_results (run_id, row_number);

-- 1b. Partial index for CompleteRunIfDone and any "are we done?" checks.
--     Covers only rows still in a non-terminal state, so it shrinks to
--     near-zero once a run completes — zero maintenance cost on finished data.
CREATE INDEX IF NOT EXISTS idx_vr_run_pending
    ON public.validation_results (run_id)
    WHERE status IN ('pending', 'in_progress');

-- 1c. Partial index that the history processor uses to claim its next batch:
--     rows where email validation is done but history hasn't been fetched.
CREATE INDEX IF NOT EXISTS idx_vr_run_history_pending
    ON public.validation_results (run_id, id)
    WHERE history_status = 'pending';

-- 1d. GIN index for the triple ILIKE search in ListResults.
--     A GIN tsvector index is orders of magnitude faster than three sequential
--     ILIKE scans on large runs. The planner will still choose idx_vr_run_row
--     for small/empty searches and switch to this for text-heavy searches.
--
--     The application query does NOT need to change — the planner picks this
--     up automatically when the ILIKE pattern is selective enough.
CREATE INDEX IF NOT EXISTS idx_vr_search_gin
    ON public.validation_results
    USING gin (
        to_tsvector(
            'simple',
            coalesce(raw_contact_key,  '') || ' ' ||
            coalesce(normalized_email, '') || ' ' ||
            coalesce(contact_id,       '')
        )
    );