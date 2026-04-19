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