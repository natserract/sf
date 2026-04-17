package db

import (
	"context"
	"fmt"
	"sf/usecases/mail-checker/fetch-all-contacts-go/internal/api"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Run struct {
	ID             string
	CreatedAt      time.Time
	State          string
	StopPage       *int
	LastExitBatch  *int64 // persisted on graceful shutdown — resume restarts from this batch
	CompletedAt    *time.Time
	PageSize       int
	StartedPage    int
	BaseURL        string
	FilterOperator string
	FilterValue    string
}

// ClaimedPage is returned by ClaimNextPages. It now carries the batch_id that
// was assigned to the whole batch at claim time so processPage can record it.
type ClaimedPage struct {
	RunID      string
	PageNumber int
	Attempts   int
	BatchID    int64
}

type Repo struct {
	pool *pgxpool.Pool
}

type Page struct {
	RunID      string
	PageNumber int
	Attempts   int
	BatchID    int64
}

func NewRepo(pool *pgxpool.Pool) *Repo {
	return &Repo{pool: pool}
}

type Tx interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

func (r *Repo) CreateRun(ctx context.Context, pageSize int, startedPage int, baseURL, filterOp, filterVal string, totalContacts int) (string, error) {
	var id string
	err := r.pool.QueryRow(ctx, `
insert into runs(page_size, started_page, base_url, filter_operator, filter_value, total_contacts)
values ($1,$2,$3,$4,$5,$6)
returning id::text
`, pageSize, startedPage, baseURL, filterOp, filterVal, totalContacts).Scan(&id)
	if err != nil {
		return "", err
	}
	return id, nil
}

func (r *Repo) GetRun(ctx context.Context, runID string) (Run, error) {
	var out Run
	err := r.pool.QueryRow(ctx, `
select id::text, created_at, state, stop_page, last_exit_batch, completed_at,
       page_size, started_page, base_url, filter_operator, filter_value
from runs where id = $1
`, runID).Scan(
		&out.ID, &out.CreatedAt, &out.State,
		&out.StopPage, &out.LastExitBatch, &out.CompletedAt,
		&out.PageSize, &out.StartedPage,
		&out.BaseURL, &out.FilterOperator, &out.FilterValue,
	)
	if err != nil {
		return Run{}, err
	}
	return out, nil
}

func (r *Repo) UpdateContactHasEngagementHistory(ctx context.Context, tx Tx, contactID string, hasEngagementHistory bool) error {
	_, err := tx.Exec(ctx, `
update contact_keys set has_engagement_history = $2 where contact_id = $1
`, contactID, hasEngagementHistory)
	return err
}

func (r *Repo) BatchUpdateContactHasEngagementHistory(ctx context.Context, tx Tx, contactIDs []string, hasEngagementHistory bool) error {
	_, err := tx.Exec(ctx, `
update contact_keys set has_engagement_history = $1 where contact_id = any($2)
`, hasEngagementHistory, contactIDs)
	return err
}

func (r *Repo) GetPages(ctx context.Context, runID string, limit int, workerID string) ([]Page, error) {
	rows, err := r.pool.Query(ctx, `
	WITH cte AS (
		SELECT run_id, page_number
		FROM pages
		WHERE run_id = $1
		  AND status = 'pending'
		  AND next_attempt_at <= now()
		ORDER BY page_number
		LIMIT $2
		FOR UPDATE SKIP LOCKED
	)
	UPDATE pages p
	SET 
		status = 'in_progress',
		locked_by = $3,
		locked_at = now(),
		attempts = attempts + 1,
		updated_at = now()
	FROM cte
	WHERE p.run_id = cte.run_id
	  AND p.page_number = cte.page_number
	RETURNING p.run_id::text, p.page_number, p.attempts, p.batch_id
	`, runID, limit, workerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var pages []Page

	for rows.Next() {
		var p Page
		if err := rows.Scan(&p.RunID, &p.PageNumber, &p.Attempts, &p.BatchID); err != nil {
			return nil, err
		}
		pages = append(pages, p)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return pages, nil
}

// LastTouchedBatch returns the highest batch_id that has been started
// (in_progress, done, empty, or failed) for this run. It is used to persist
// last_exit_batch on graceful shutdown so resume knows where to continue.
func (r *Repo) LastTouchedBatch(ctx context.Context, runID string) (int64, error) {
	var batchID int64
	err := r.pool.QueryRow(ctx, `
select coalesce(max(batch_id), 0)
from pages
where run_id = $1
  and status in ('in_progress', 'done', 'empty', 'failed')
`, runID).Scan(&batchID)
	return batchID, err
}

// SetLastExitBatch records the last batch the worker reached before exiting.
func (r *Repo) SetLastExitBatch(ctx context.Context, runID string, batchID int64) error {
	_, err := r.pool.Exec(ctx, `
update runs set last_exit_batch = $2 where id = $1
`, runID, batchID)
	return err
}

// ResumeFromBatch resets progress from resumeBatchID onward so those pages are
// re-fetched. It:
//  1. Deletes contact_keys first seen in batches >= resumeBatchID (partial writes).
//  2. Resets those pages' status back to 'pending' and clears their batch_id.
//
// Returns (contactKeysDropped, pagesReset, error).
func (r *Repo) ResumeFromBatch(ctx context.Context, runID string, resumeBatchID int64) (int64, int64, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return 0, 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Drop contact_keys that were inserted in batches at or after the resume batch.
	ct, err := tx.Exec(ctx, `
delete from contact_keys
where first_seen_run_id = $1
  and first_seen_batch_id >= $2
`, runID, resumeBatchID)
	if err != nil {
		return 0, 0, fmt.Errorf("drop contact_keys: %w", err)
	}
	dropped := ct.RowsAffected()

	// Reset pages in batches at or after the resume batch back to pending.
	ct, err = tx.Exec(ctx, `
update pages
set status          = 'pending',
    attempts        = 0,
    last_error      = null,
    started_at      = null,
    finished_at     = null,
    locked_by       = null,
    locked_at       = null,
    next_attempt_at = now(),
    batch_id        = null,
    updated_at      = now()
where run_id = $1
  and batch_id >= $2
`, runID, resumeBatchID)
	if err != nil {
		return 0, 0, fmt.Errorf("reset pages: %w", err)
	}
	reset := ct.RowsAffected()

	// Clear stop_page and last_exit_batch if they fall within the reset range.
	if _, err = tx.Exec(ctx, `
update runs
set stop_page       = null,
    last_exit_batch = null
where id = $1
  and (stop_page is null or exists (
    select 1 from pages
    where run_id = $1 and page_number >= stop_page and batch_id >= $2
  ))
`, runID, resumeBatchID); err != nil {
		return 0, 0, fmt.Errorf("clear stop_page/last_exit_batch: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, 0, err
	}
	return dropped, reset, nil
}

// ReopenRun sets a completed/failed run back to 'running' so workers can claim pages.
func (r *Repo) ReopenRun(ctx context.Context, runID string) error {
	_, err := r.pool.Exec(ctx, `
update runs
set state = 'running', completed_at = null
where id = $1
`, runID)
	return err
}

func (r *Repo) EnsurePagePending(ctx context.Context, tx Tx, runID string, page int) error {
	_, err := tx.Exec(ctx, `
insert into pages(run_id, page_number, status, attempts, next_attempt_at, updated_at)
values ($1, $2, 'pending', 0, now(), now())
on conflict (run_id, page_number) do nothing
`, runID, page)
	return err
}

func (r *Repo) EnsureNextPagePendingIfAllowed(ctx context.Context, tx Tx, runID string, page int) error {
	_, err := tx.Exec(ctx, `
insert into pages(run_id, page_number, status, attempts, next_attempt_at, updated_at)
select $1, $2, 'pending', 0, now(), now()
where exists (
  select 1 from runs r
  where r.id = $1 and (r.stop_page is null or $2 < r.stop_page)
)
on conflict (run_id, page_number) do nothing
`, runID, page)
	return err
}

func (r *Repo) SeedPendingPages(ctx context.Context, runID string, fromPage int, toPage int, batchSize int) (int64, error) {
	if fromPage <= 0 {
		fromPage = 1
	}
	if toPage < fromPage {
		return 0, nil
	}
	if batchSize <= 0 {
		batchSize = 10000
	}
	var insertedTotal int64
	for start := fromPage; start <= toPage; start += batchSize {
		end := start + batchSize - 1
		if end > toPage {
			end = toPage
		}
		tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			return insertedTotal, err
		}
		ct, err := tx.Exec(ctx, `
insert into pages(run_id, page_number, status, attempts, next_attempt_at, updated_at)
select $1, g, 'pending', 0, now(), now()
from generate_series($2::int, $3::int) g
on conflict (run_id, page_number) do nothing
`, runID, start, end)
		if err != nil {
			_ = tx.Rollback(ctx)
			return insertedTotal, err
		}
		if err := tx.Commit(ctx); err != nil {
			return insertedTotal, err
		}
		insertedTotal += ct.RowsAffected()
	}
	return insertedTotal, nil
}

func (r *Repo) UpsertPageInProgress(ctx context.Context, tx Tx, runID string, page int) error {
	_, err := tx.Exec(ctx, `
insert into pages(run_id, page_number, status, attempts, started_at, updated_at, next_attempt_at, locked_at)
values ($1, $2, 'in_progress', 1, now(), now(), now(), now())
on conflict (run_id, page_number) do update set
  status = 'in_progress',
  attempts = pages.attempts + 1,
  last_error = null,
  started_at = coalesce(pages.started_at, now()),
  next_attempt_at = now(),
  locked_at = now(),
  updated_at = now()
`, runID, page)
	return err
}

func (r *Repo) MarkPageDone(ctx context.Context, tx Tx, runID string, page int, status string) error {
	if status != "done" && status != "empty" {
		return fmt.Errorf("invalid page status: %s", status)
	}
	_, err := tx.Exec(ctx, `
update pages set status=$3, finished_at=now(), updated_at=now() where run_id=$1 and page_number=$2
`, runID, page, status)
	return err
}

func (r *Repo) MarkPageFailed(ctx context.Context, tx Tx, runID string, page int, errMsg string) error {
	_, err := tx.Exec(ctx, `
update pages set status='failed', last_error=$3, updated_at=now() where run_id=$1 and page_number=$2
`, runID, page, errMsg)
	return err
}

func (r *Repo) MarkPageRetry(ctx context.Context, tx Tx, runID string, page int, errMsg string, nextAttemptAt time.Time) error {
	_, err := tx.Exec(ctx, `
update pages
set status='pending',
    last_error=$3,
    next_attempt_at=$4,
    locked_by=null,
    locked_at=null,
    updated_at=now()
where run_id=$1 and page_number=$2
`, runID, page, errMsg, nextAttemptAt)
	return err
}

func (r *Repo) ClaimNextPage(ctx context.Context, runID string, workerID string, maxAttempts int) (*ClaimedPage, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var c ClaimedPage
	err = tx.QueryRow(ctx, `
with candidate as (
  select p.run_id, p.page_number
  from pages p
  join runs r on r.id = p.run_id
  where p.run_id = $1
    and r.state = 'running'
    and p.status = 'pending'
    and p.next_attempt_at <= now()
    and p.attempts < $3
  order by p.page_number
  for update skip locked
  limit 1
),
new_batch as (
  insert into page_batches(run_id) select $1 from candidate returning id
)
update pages p
set status='in_progress',
    attempts=p.attempts + 1,
    last_error=null,
    started_at=coalesce(p.started_at, now()),
    locked_by=$2,
    locked_at=now(),
    updated_at=now(),
    batch_id=(select id from new_batch)
from candidate c
where p.run_id = c.run_id and p.page_number = c.page_number
returning p.run_id::text, p.page_number, p.attempts, p.batch_id
`, runID, workerID, maxAttempts).Scan(&c.RunID, &c.PageNumber, &c.Attempts, &c.BatchID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &c, nil
}

// ClaimNextPages atomically claims up to `limit` pending pages and assigns them
// all a single shared batch_id. The batch_id is a new row in the page_batches
// table, returned in every ClaimedPage so callers can record it in contact_keys.
//
// Returns nil (not an error) when no pages are claimable right now.
func (r *Repo) ClaimNextPages(
	ctx context.Context,
	runID string,
	workerID string,
	maxAttempts int,
	limit int,
) ([]ClaimedPage, error) {

	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// First check: are there any claimable pages? If not, skip batch creation.
	var count int
	err = tx.QueryRow(ctx, `
select count(*)
from pages p
join runs r on r.id = p.run_id
where p.run_id = $1
  and r.state = 'running'
  and p.status = 'pending'
  and p.next_attempt_at <= now()
  and p.attempts < $2
limit $3
`, runID, maxAttempts, limit).Scan(&count)
	if err != nil {
		return nil, err
	}
	if count == 0 {
		// Nothing to claim — roll back (no-op) and return nil.
		return nil, nil
	}

	// Create one batch row for all pages in this claim.
	var batchID int64
	err = tx.QueryRow(ctx, `
insert into page_batches(run_id, worker_id) values ($1, $2) returning id
`, runID, workerID).Scan(&batchID)
	if err != nil {
		return nil, fmt.Errorf("create page_batch: %w", err)
	}

	// Claim the pages, stamping them with the new batch_id.
	rows, err := tx.Query(ctx, `
with candidate as (
  select p.run_id, p.page_number
  from pages p
  join runs r on r.id = p.run_id
  where p.run_id = $1
    and r.state = 'running'
    and p.status = 'pending'
    and p.next_attempt_at <= now()
    and p.attempts < $3
  order by p.page_number
  for update skip locked
  limit $4
)
update pages p
set status          = 'in_progress',
    attempts        = p.attempts + 1,
    last_error      = null,
    started_at      = coalesce(p.started_at, now()),
    locked_by       = $2,
    locked_at       = now(),
    updated_at      = now(),
    batch_id        = $5
from candidate c
where p.run_id = c.run_id
  and p.page_number = c.page_number
returning p.run_id::text, p.page_number, p.attempts, p.batch_id
`, runID, workerID, maxAttempts, limit, batchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []ClaimedPage
	for rows.Next() {
		var c ClaimedPage
		if err := rows.Scan(&c.RunID, &c.PageNumber, &c.Attempts, &c.BatchID); err != nil {
			return nil, err
		}
		results = append(results, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if len(results) == 0 {
		// Race: pages were claimed by another worker between the count and the
		// update. Roll back (removes the orphan batch row too via cascade or
		// the deferred delete below) and return nil to idle.
		return nil, nil
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return results, nil
}

// ReapStaleInProgress resets in-progress pages that have been locked for
// longer than olderThan back to pending.  Pass olderThan=0 to reap all
// in-progress pages unconditionally (useful on clean shutdown).
func (r *Repo) ReapStaleInProgress(ctx context.Context, runID string, olderThan time.Duration) (int64, error) {
	var ct pgconn.CommandTag
	var err error
	if olderThan == 0 {
		ct, err = r.pool.Exec(ctx, `
update pages
set status='pending',
    locked_by=null,
    locked_at=null,
    next_attempt_at=now(),
    updated_at=now()
where run_id = $1
  and status='in_progress'
`, runID)
	} else {
		ct, err = r.pool.Exec(ctx, `
update pages
set status='pending',
    locked_by=null,
    locked_at=null,
    next_attempt_at=now(),
    updated_at=now()
where run_id = $1
  and status='in_progress'
  and locked_at < now() - ($2::interval)
`, runID, fmt.Sprintf("%f seconds", olderThan.Seconds()))
	}
	if err != nil {
		return 0, err
	}
	return ct.RowsAffected(), nil
}

func (r *Repo) MarkRunStopPage(ctx context.Context, tx Tx, runID string, stopPage int) error {
	_, err := tx.Exec(ctx, `
update runs
set stop_page = case
  when stop_page is null then $2
  when stop_page > $2 then $2
  else stop_page
end
where id=$1
`, runID, stopPage)
	return err
}

func (r *Repo) MarkRunCompletedIfDrained(ctx context.Context, runID string) (bool, error) {
	ct, err := r.pool.Exec(ctx, `
update runs
set state='completed', completed_at=now()
where id=$1
  and state='running'
  and not exists (
    select 1 from pages p
    where p.run_id = runs.id
      and p.status in ('pending', 'in_progress')
  )
`, runID)
	if err != nil {
		return false, err
	}
	return ct.RowsAffected() > 0, nil
}

// InsertContactKeys now records the batch_id alongside the page number.
// This is the key that makes resume-by-batch work: on resume we delete all
// contact_keys whose first_seen_batch_id >= the last_exit_batch, then reset
// the corresponding pages.
func (r *Repo) InsertContactKeys(ctx context.Context, tx Tx, runID string, page int, batchID int64, contacts []api.ContactInfo) (int64, error) {
	var inserted int64
	for _, c := range contacts {
		ct, err := tx.Exec(ctx, `
insert into contact_keys(contact_key, contact_id, first_seen_run_id, first_seen_page, first_seen_batch_id)
values ($1,$2,$3,$4,$5)
on conflict do nothing
`, []byte(c.ContactKey), c.ContactID, runID, page, batchID)
		if err != nil {
			return inserted, err
		}
		inserted += ct.RowsAffected()
	}
	return inserted, nil
}

func (r *Repo) RunProgress(ctx context.Context, runID string) (done, failed, empty, inProgress, pending int64, err error) {
	err = r.pool.QueryRow(ctx, `
select
  count(*) filter (where status='done') as done,
  count(*) filter (where status='failed') as failed,
  count(*) filter (where status='empty') as empty,
  count(*) filter (where status='in_progress') as in_progress,
  count(*) filter (where status='pending') as pending
from pages
where run_id=$1
`, runID).Scan(&done, &failed, &empty, &inProgress, &pending)
	return
}

func (r *Repo) GetRunTotalContacts(ctx context.Context, runID string) (int, error) {
	var total int
	err := r.pool.QueryRow(ctx, `
select total_contacts from runs where id = $1
`, runID).Scan(&total)
	return total, err
}

func (r *Repo) CountContactKeys(ctx context.Context, runID string) (int64, error) {
	var total int64
	err := r.pool.QueryRow(ctx, `
select count(*) 
from contact_keys 
where first_seen_run_id = $1
`, runID).Scan(&total)
	return total, err
}
