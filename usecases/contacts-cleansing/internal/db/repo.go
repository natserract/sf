package db

import (
	"context"
	"encoding/json"
	"fmt"
	"sf/usecases/mail-checker/internal/api"
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
	TotalContacts  int
}

// ClaimedPage is returned by ClaimNextPages. It now carries the batch_id that
// was assigned to the whole batch at claim time so processPage can record it.
type ClaimedPage struct {
	RunID      string
	PageNumber int
	Attempts   int
	BatchID    int64
}

type CreateValidationResultInput struct {
	RowNumber     int
	ContactID     string
	RawContactKey string
}

type ValidationUpdate struct {
	ID              int64
	Status          string
	FailureReason   string
	CleanCandidate  string
	NormalizedEmail string
	SyntaxStatus    string
	SyntaxReason    string
	SyntaxLatencyMS int
	SyntaxScore     int
	DomainDNSStatus string
	DomainDNSReason string
	DomainLatencyMS int
	DomainScore     int
	MXStatus        string
	MXReason        string
	MXLatencyMS     int
	MXScore         int
	SMTPStatus      string
	SMTPReason      string
	SMTPLatencyMS   int
	SMTPScore       int
	HistoryStatus   string
	HistoryReason   string
	HistoryScore    int
	TotalScore      int
}

type ValidationRow struct {
	ID              int64           `json:"id"`
	RunID           string          `json:"runId"`
	RowNumber       int             `json:"rowNumber"`
	ContactID       string          `json:"contactId"`
	RawContactKey   string          `json:"rawContactKey"`
	CleanCandidate  string          `json:"cleanCandidate"`
	NormalizedEmail string          `json:"normalizedEmail"`
	Status          string          `json:"status"`
	FailureReason   string          `json:"failureReason"`
	SyntaxStatus    string          `json:"syntaxStatus"`
	SyntaxReason    string          `json:"syntaxReason"`
	SyntaxLatencyMS int             `json:"syntaxLatencyMs"`
	SyntaxScore     int             `json:"syntaxScore"`
	DomainDNSStatus string          `json:"domainDnsStatus"`
	DomainDNSReason string          `json:"domainDnsReason"`
	DomainLatencyMS int             `json:"domainDnsLatencyMs"`
	DomainScore     int             `json:"domainDnsScore"`
	MXStatus        string          `json:"mxStatus"`
	MXReason        string          `json:"mxReason"`
	MXLatencyMS     int             `json:"mxLatencyMs"`
	MXScore         int             `json:"mxScore"`
	SMTPStatus      string          `json:"smtpStatus"`
	SMTPReason      string          `json:"smtpReason"`
	SMTPLatencyMS   int             `json:"smtpLatencyMs"`
	SMTPScore       int             `json:"smtpScore"`
	HistoryStatus   string          `json:"historyStatus"`
	HistoryReason   string          `json:"historyReason"`
	HistoryScore    int             `json:"historyScore"`
	HistoryPayload  json.RawMessage `json:"historyPayload,omitempty"`
	TotalScore      int             `json:"totalScore"`
}

type RunProgress struct {
	RunID       string `json:"runId"`
	State       string `json:"state"`
	TotalRows   int    `json:"totalRows"`
	Pending     int64  `json:"pending"`
	InProgress  int64  `json:"inProgress"`
	Done        int64  `json:"done"`
	Failed      int64  `json:"failed"`
	LastUpdated string `json:"lastUpdated"`
}

type ValidationRunProgress struct {
	TotalRows   int
	Pending     int64
	InProgress  int64
	Done        int64
	Failed      int64
	LastUpdated string
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

func (r *Repo) UpdateTotalContacts(ctx context.Context, runID string, totalContacts int) error {
	_, err := r.pool.Exec(ctx, `
update runs
set total_contacts = $2
where id = $1
`, runID, totalContacts)

	return err
}

func (r *Repo) GetRun(ctx context.Context, runID string) (Run, error) {
	var out Run
	err := r.pool.QueryRow(ctx, `
select id::text, created_at, state, stop_page, last_exit_batch, completed_at,
       page_size, started_page, base_url, filter_operator, filter_value, total_contacts
from runs where id = $1
`, runID).Scan(
		&out.ID, &out.CreatedAt, &out.State,
		&out.StopPage, &out.LastExitBatch, &out.CompletedAt,
		&out.PageSize, &out.StartedPage,
		&out.BaseURL, &out.FilterOperator, &out.FilterValue, &out.TotalContacts,
	)
	if err != nil {
		return Run{}, err
	}
	return out, nil
}

// GetLatestRunID returns the ID of the most recently created run, or ("", nil)
// if no runs exist yet. Used to top-up an existing unfiltered run instead of
// creating a duplicate.
func (r *Repo) GetLatestRunID(ctx context.Context) (string, error) {
	return r.GetLatestRunIDByFilter(ctx, "Is", "")
}

// GetLatestRunIDByFilter returns the most recently created run matching the
// given filter, or ("", nil) if none exists.
func (r *Repo) GetLatestRunIDByFilter(ctx context.Context, filterOperator, filterValue string) (string, error) {
	var id string
	err := r.pool.QueryRow(ctx, `
select id::text from runs
where filter_operator = $1 and filter_value = $2
order by created_at desc limit 1
`, filterOperator, filterValue).Scan(&id)
	if err == pgx.ErrNoRows {
		return "", nil
	}
	return id, err
}

func (r *Repo) CountRunsByFilter(ctx context.Context, filterOperator, filterValue string) (int, error) {
	var total int
	err := r.pool.QueryRow(ctx, `
select count(*)
from runs
where filter_operator = $1
  and filter_value = $2
`, filterOperator, filterValue).Scan(&total)
	if err != nil {
		return 0, err
	}
	return total, nil
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
  and (p.status = 'pending' or p.status = 'failed')
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
    and (p.status = 'pending' or p.status = 'failed')
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

// CountPages returns the total number of page rows across all runs.
func (r *Repo) CountPages(ctx context.Context) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx, `
select count(*) from pages
`).Scan(&count)
	return count, err
}

// CountPagesByRun returns the number of page rows already seeded for a run,
// regardless of their status.
func (r *Repo) CountPagesByRun(ctx context.Context, runID string) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx, `
select count(*)
from pages
where run_id = $1
`, runID).Scan(&count)
	return count, err
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

// GetContactsForHistory fetches a batch of contacts whose history has not been
// checked yet (history_checked_at IS NULL). This makes the processor resumable:
// restarting after a crash simply picks up where it left off instead of
// re-scanning the entire table.
func (r *Repo) GetContactsForHistory(ctx context.Context, limit int) ([]api.ContactInfo, error) {
	rows, err := r.pool.Query(ctx, `
SELECT contact_id,
       encode(contact_key, 'escape') AS contact_key
FROM   contact_keys
WHERE  history_checked_at IS NULL
ORDER  BY contact_id          -- stable ordering avoids re-visiting the same rows
LIMIT  $1
`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var contacts []api.ContactInfo
	for rows.Next() {
		var c api.ContactInfo
		if err := rows.Scan(&c.ContactID, &c.ContactKey); err != nil {
			return nil, err
		}
		contacts = append(contacts, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return contacts, nil
}

// UpdateHistoryStatus marks a batch of contacts as checked.
//
//   - withHistory  – contacts that had ≥1 message history entry; sets
//     has_engagement_history = true AND history_checked_at = now()
//   - allProcessed – every contact that was attempted in this batch (superset
//     of withHistory); sets history_checked_at = now() so they are never
//     re-visited, even when no history was found.
//
// Using two separate UPDATE statements keeps the query simple and lets
// Postgres use the partial index on history_checked_at IS NULL efficiently.
func (r *Repo) UpdateHistoryStatus(ctx context.Context, withHistory []string, allProcessed []string) error {
	if len(allProcessed) == 0 {
		return nil
	}

	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// 1. Flag contacts that actually have history.
	if len(withHistory) > 0 {
		if _, err := tx.Exec(ctx, `
UPDATE contact_keys
SET    has_engagement_history = true,
       history_checked_at     = now()
WHERE  contact_id = ANY($1)
`, withHistory); err != nil {
			return fmt.Errorf("update with-history: %w", err)
		}
	}

	// 2. Mark the rest of the batch as checked (no history found).
	//    Using a NOT IN sub-select would be expensive; pass the two slices
	//    directly and let Postgres handle the overlap.
	if _, err := tx.Exec(ctx, `
UPDATE contact_keys
SET    history_checked_at = now()
WHERE  contact_id         = ANY($1)
  AND  history_checked_at IS NULL   -- skip rows already written above
`, allProcessed); err != nil {
		return fmt.Errorf("update checked-at: %w", err)
	}

	return tx.Commit(ctx)
}

// SavePageResult atomically inserts all contact keys for a page and marks the
// page done in a single transaction.
//
// This is the only safe way to write page results: either both the contact keys
// AND the page status land together, or neither does. It eliminates the
// "batch_done but no contact_keys" failure mode that occurs when callers manage
// their own transaction and commit/crash between the two writes.
//
// status must be "done" or "empty". Pass an empty contacts slice (not nil) when
// status is "empty" — the insert loop is a no-op and the page is still marked
// correctly.
//
// Returns the number of contact_keys rows actually inserted (conflicts excluded).
func (r *Repo) SavePageResult(
	ctx context.Context,
	runID string,
	page int,
	batchID int64,
	status string,
	contacts []api.ContactInfo,
) (inserted int64, err error) {
	if status != "done" && status != "empty" {
		return 0, fmt.Errorf("SavePageResult: invalid status %q (want \"done\" or \"empty\")", status)
	}

	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return 0, fmt.Errorf("SavePageResult: begin tx: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	// 1. Insert contact keys — all or nothing inside this transaction.
	//    Using a single unnest-based statement instead of a per-row loop
	//    avoids partial inserts if the loop errors halfway through.
	if len(contacts) > 0 {
		keys := make([][]byte, len(contacts))
		ids := make([]string, len(contacts))
		for i, c := range contacts {
			keys[i] = []byte(c.ContactKey)
			ids[i] = c.ContactID
		}

		ct, execErr := tx.Exec(ctx, `
INSERT INTO contact_keys
    (contact_key, contact_id, first_seen_run_id, first_seen_page, first_seen_batch_id)
SELECT
    unnest($1::bytea[]),
    unnest($2::text[]),
    $3,
    $4,
    $5
ON CONFLICT DO NOTHING
`, keys, ids, runID, page, batchID)
		if execErr != nil {
			err = fmt.Errorf("SavePageResult: insert contact_keys: %w", execErr)
			return 0, err
		}
		inserted = ct.RowsAffected()
	}

	// 2. Mark page done — same transaction, so it only commits if step 1 succeeded.
	if _, execErr := tx.Exec(ctx, `
UPDATE pages
SET    status      = $3,
       finished_at = now(),
       updated_at  = now()
WHERE  run_id      = $1
  AND  page_number = $2
`, runID, page, status); execErr != nil {
		err = fmt.Errorf("SavePageResult: mark page %s: %w", status, execErr)
		return 0, err
	}

	if err = tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("SavePageResult: commit: %w", err)
	}
	return inserted, nil
}

func (r *Repo) CreateValidationRun(ctx context.Context, sourceFile string, totalRows int) (string, error) {
	var id string
	err := r.pool.QueryRow(ctx, `
insert into validation_runs(source_file, total_rows, state, started_at)
values ($1, $2, 'running', now())
returning id::text
`, sourceFile, totalRows).Scan(&id)
	return id, err
}

func (r *Repo) FindLatestUnfinishedValidationRunBySource(ctx context.Context, sourceFile string) (string, error) {
	var runID string
	err := r.pool.QueryRow(ctx, `
select vr.id::text
from validation_runs vr
where vr.source_file = $1
  and vr.state <> 'completed'
  and exists (
    select 1
    from validation_results r
    where r.run_id = vr.id
      and r.status in ('pending', 'in_progress')
  )
order by vr.created_at desc
limit 1
`, sourceFile).Scan(&runID)
	if err == pgx.ErrNoRows {
		return "", nil
	}
	return runID, err
}

func (r *Repo) UpdateValidationRunTotalRows(ctx context.Context, runID string, totalRows int) error {
	_, err := r.pool.Exec(ctx, `
update validation_runs
set total_rows = $2
where id = $1
`, runID, totalRows)
	return err
}

func (r *Repo) ReopenValidationRun(ctx context.Context, runID string) error {
	_, err := r.pool.Exec(ctx, `
update validation_runs
set state = 'running',
    completed_at = null
where id = $1
`, runID)
	return err
}

func (r *Repo) MarkValidationRunFailed(ctx context.Context, runID, reason string) error {
	_ = reason
	_, err := r.pool.Exec(ctx, `
update validation_runs
set state='failed',
    completed_at=now()
where id = $1
`, runID)
	return err
}

func (r *Repo) RequeueValidationInProgress(ctx context.Context, runID string) (int64, error) {
	ct, err := r.pool.Exec(ctx, `
update validation_results
set status = 'pending',
    updated_at = now()
where run_id = $1
  and status = 'in_progress'
`, runID)
	if err != nil {
		return 0, err
	}
	return ct.RowsAffected(), nil
}

func (r *Repo) CreateValidationResults(ctx context.Context, runID string, rows []CreateValidationResultInput) error {
	if len(rows) == 0 {
		return nil
	}

	rowNumbers := make([]int32, len(rows))
	contactIDs := make([]string, len(rows))
	rawKeys := make([]string, len(rows))
	for i, row := range rows {
		rowNumbers[i] = int32(row.RowNumber)
		contactIDs[i] = row.ContactID
		rawKeys[i] = row.RawContactKey
	}

	// bulk_create_validation_results inserts all rows in a single unnest
	// statement. ON CONFLICT DO NOTHING makes it safe to call more than once.
	_, err := r.pool.Exec(ctx,
		`SELECT bulk_create_validation_results($1, $2, $3, $4)`,
		runID, rowNumbers, contactIDs, rawKeys,
	)
	return err
}

func (r *Repo) UpdateValidation(ctx context.Context, in ValidationUpdate) error {
	_, err := r.pool.Exec(ctx, `
update validation_results
set
  status=$2,
  failure_reason=$3,
  clean_candidate=$4,
  normalized_email=$5,
  syntax_status=$6,
  syntax_reason=$7,
  syntax_latency_ms=$8,
  syntax_score=$9,
  domain_dns_status=$10,
  domain_dns_reason=$11,
  domain_dns_latency_ms=$12,
  domain_dns_score=$13,
  mx_status=$14,
  mx_reason=$15,
  mx_latency_ms=$16,
  mx_score=$17,
  smtp_status=$18,
  smtp_reason=$19,
  smtp_latency_ms=$20,
  smtp_score=$21,
  history_status=$22,
  history_reason=$23,
  history_score=$24,
  total_score=$25,
  updated_at=now()
where id = $1
`, in.ID, in.Status, in.FailureReason, in.CleanCandidate, in.NormalizedEmail, in.SyntaxStatus, in.SyntaxReason, in.SyntaxLatencyMS, in.SyntaxScore,
		in.DomainDNSStatus, in.DomainDNSReason, in.DomainLatencyMS, in.DomainScore, in.MXStatus, in.MXReason, in.MXLatencyMS, in.MXScore,
		in.SMTPStatus, in.SMTPReason, in.SMTPLatencyMS, in.SMTPScore, in.HistoryStatus, in.HistoryReason, in.HistoryScore, in.TotalScore)
	return err
}

func (r *Repo) CompleteValidationRun(ctx context.Context, runID string) (bool, error) {
	ct, err := r.pool.Exec(ctx, `
update validation_runs
set state='completed',
    completed_at=now()
where id = $1
  and state='running'
  and not exists (
    select 1
    from validation_results
    where run_id = $1 and status in ('pending', 'in_progress')
  )
`, runID)
	if err != nil {
		return false, err
	}
	return ct.RowsAffected() > 0, nil
}

func (r *Repo) GetValidationRunProgress(ctx context.Context, runID string) (ValidationRunProgress, error) {
	var out ValidationRunProgress
	err := r.pool.QueryRow(ctx, `
select
  vr.total_rows,
  count(*) filter (where r.status='pending') as pending,
  count(*) filter (where r.status='in_progress') as in_progress,
  count(*) filter (where r.status='done') as done,
  count(*) filter (where r.status='failed') as failed,
  to_char(coalesce(max(r.updated_at), now()), 'YYYY-MM-DD"T"HH24:MI:SSOF')
from validation_runs vr
left join validation_results r on r.run_id = vr.id
where vr.id = $1
group by vr.total_rows
`, runID).Scan(&out.TotalRows, &out.Pending, &out.InProgress, &out.Done, &out.Failed, &out.LastUpdated)
	return out, err
}

func (r *Repo) ListResults(ctx context.Context, runID string, offset int, limit int, q string) ([]ValidationRow, int64, error) {
	if limit <= 0 {
		limit = 100
	}
	if q == "" {
		q = "%"
	} else {
		q = "%" + q + "%"
	}

	var total int64
	if err := r.pool.QueryRow(ctx, `
select count(*)
from validation_results
where run_id = $1 and (raw_contact_key ilike $2 or normalized_email ilike $2 or contact_id ilike $2)
`, runID, q).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := r.pool.Query(ctx, `
select id, run_id::text, row_number, contact_id, raw_contact_key, coalesce(clean_candidate, ''), coalesce(normalized_email, ''),
status, coalesce(failure_reason, ''), coalesce(syntax_status, ''), coalesce(domain_dns_status, ''), coalesce(mx_status, ''),
coalesce(smtp_status, ''), coalesce(history_status, ''), total_score
from validation_results
where run_id = $1 and (raw_contact_key ilike $2 or normalized_email ilike $2 or contact_id ilike $2)
order by row_number
offset $3 limit $4
`, runID, q, offset, limit)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := make([]ValidationRow, 0, limit)
	for rows.Next() {
		var row ValidationRow
		if err := rows.Scan(&row.ID, &row.RunID, &row.RowNumber, &row.ContactID, &row.RawContactKey, &row.CleanCandidate, &row.NormalizedEmail,
			&row.Status, &row.FailureReason, &row.SyntaxStatus, &row.DomainDNSStatus, &row.MXStatus, &row.SMTPStatus, &row.HistoryStatus, &row.TotalScore); err != nil {
			return nil, 0, err
		}
		out = append(out, row)
	}
	return out, total, rows.Err()
}

func (r *Repo) GetResult(ctx context.Context, runID string, rowID int64) (ValidationRow, error) {
	var row ValidationRow
	err := r.pool.QueryRow(ctx, `
select id, run_id::text, row_number, contact_id, raw_contact_key, coalesce(clean_candidate, ''), coalesce(normalized_email, ''),
status, coalesce(failure_reason, ''),
coalesce(syntax_status, ''), coalesce(syntax_reason, ''), syntax_latency_ms, syntax_score,
coalesce(domain_dns_status, ''), coalesce(domain_dns_reason, ''), domain_dns_latency_ms, domain_dns_score,
coalesce(mx_status, ''), coalesce(mx_reason, ''), mx_latency_ms, mx_score,
coalesce(smtp_status, ''), coalesce(smtp_reason, ''), smtp_latency_ms, smtp_score,
coalesce(history_status, ''), coalesce(history_reason, ''), history_score, coalesce(history_payload, '{}'::jsonb)::text, total_score
from validation_results
where run_id = $1 and id = $2
`, runID, rowID).Scan(
		&row.ID, &row.RunID, &row.RowNumber, &row.ContactID, &row.RawContactKey, &row.CleanCandidate, &row.NormalizedEmail,
		&row.Status, &row.FailureReason,
		&row.SyntaxStatus, &row.SyntaxReason, &row.SyntaxLatencyMS, &row.SyntaxScore,
		&row.DomainDNSStatus, &row.DomainDNSReason, &row.DomainLatencyMS, &row.DomainScore,
		&row.MXStatus, &row.MXReason, &row.MXLatencyMS, &row.MXScore,
		&row.SMTPStatus, &row.SMTPReason, &row.SMTPLatencyMS, &row.SMTPScore,
		&row.HistoryStatus, &row.HistoryReason, &row.HistoryScore, &row.HistoryPayload, &row.TotalScore,
	)
	return row, err
}

// MarkValidationResultsInProgress transitions validation_results rows for the
// given contact IDs to status='in_progress'. Called before the validator loop
// starts so a mid-batch crash leaves rows recoverable rather than stuck on
// 'pending'. Rows already in a terminal state are left untouched.
func (r *Repo) MarkValidationResultsInProgress(ctx context.Context, contactIDs []string) error {
	if len(contactIDs) == 0 {
		return nil
	}
	_, err := r.pool.Exec(ctx, `
UPDATE validation_results
SET    status     = 'in_progress',
       updated_at = now()
WHERE  contact_id = ANY($1)
  AND  status NOT IN ('done', 'failed')
`, contactIDs)
	return err
}

// ContactValidationUpdate carries the full validator result for one contact.
// Used by BulkUpdateValidationByContactID.
type ContactValidationUpdate struct {
	ContactID       string
	Status          string // "done" | "failed"
	FailureReason   string
	CleanCandidate  string
	NormalizedEmail string
	SyntaxStatus    string
	SyntaxReason    string
	SyntaxLatencyMS int
	SyntaxScore     int
	DomainDNSStatus string
	DomainDNSReason string
	DomainLatencyMS int
	DomainScore     int
	MXStatus        string
	MXReason        string
	MXLatencyMS     int
	MXScore         int
	SMTPStatus      string
	SMTPReason      string
	SMTPLatencyMS   int
	SMTPScore       int
	TotalScore      int
}

// BulkUpdateValidationByContactID writes all validator results for a batch in
// a single transaction using a temporary table + UPDATE … FROM join instead of
// one UPDATE per contact. It keys on contact_id because the history processor
// works off contact_keys and does not hold the validation_results bigserial id.
func (r *Repo) BulkUpdateValidationByContactID(ctx context.Context, updates []ContactValidationUpdate) error {
	if len(updates) == 0 {
		return nil
	}

	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("BulkUpdateValidationByContactID: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `
CREATE TEMP TABLE _val_batch (
    contact_id            text NOT NULL,
    status                text NOT NULL,
    failure_reason        text NOT NULL DEFAULT '',
    clean_candidate       text NOT NULL DEFAULT '',
    normalized_email      text NOT NULL DEFAULT '',
    syntax_status         text NOT NULL DEFAULT '',
    syntax_reason         text NOT NULL DEFAULT '',
    syntax_latency_ms     int  NOT NULL DEFAULT 0,
    syntax_score          int  NOT NULL DEFAULT 0,
    domain_dns_status     text NOT NULL DEFAULT '',
    domain_dns_reason     text NOT NULL DEFAULT '',
    domain_dns_latency_ms int  NOT NULL DEFAULT 0,
    domain_dns_score      int  NOT NULL DEFAULT 0,
    mx_status             text NOT NULL DEFAULT '',
    mx_reason             text NOT NULL DEFAULT '',
    mx_latency_ms         int  NOT NULL DEFAULT 0,
    mx_score              int  NOT NULL DEFAULT 0,
    smtp_status           text NOT NULL DEFAULT '',
    smtp_reason           text NOT NULL DEFAULT '',
    smtp_latency_ms       int  NOT NULL DEFAULT 0,
    smtp_score            int  NOT NULL DEFAULT 0,
    total_score           int  NOT NULL DEFAULT 0
) ON COMMIT DROP
`); err != nil {
		return fmt.Errorf("BulkUpdateValidationByContactID: create temp table: %w", err)
	}

	batchRows := make([][]any, len(updates))
	for i, u := range updates {
		batchRows[i] = []any{
			u.ContactID, u.Status, u.FailureReason, u.CleanCandidate, u.NormalizedEmail,
			u.SyntaxStatus, u.SyntaxReason, u.SyntaxLatencyMS, u.SyntaxScore,
			u.DomainDNSStatus, u.DomainDNSReason, u.DomainLatencyMS, u.DomainScore,
			u.MXStatus, u.MXReason, u.MXLatencyMS, u.MXScore,
			u.SMTPStatus, u.SMTPReason, u.SMTPLatencyMS, u.SMTPScore,
			u.TotalScore,
		}
	}

	if _, err := tx.CopyFrom(
		ctx,
		pgx.Identifier{"_val_batch"},
		[]string{
			"contact_id", "status", "failure_reason", "clean_candidate", "normalized_email",
			"syntax_status", "syntax_reason", "syntax_latency_ms", "syntax_score",
			"domain_dns_status", "domain_dns_reason", "domain_dns_latency_ms", "domain_dns_score",
			"mx_status", "mx_reason", "mx_latency_ms", "mx_score",
			"smtp_status", "smtp_reason", "smtp_latency_ms", "smtp_score",
			"total_score",
		},
		pgx.CopyFromRows(batchRows),
	); err != nil {
		return fmt.Errorf("BulkUpdateValidationByContactID: copy rows: %w", err)
	}

	if _, err := tx.Exec(ctx, `
UPDATE validation_results vr
SET    status                = b.status,
       failure_reason        = b.failure_reason,
       clean_candidate       = b.clean_candidate,
       normalized_email      = b.normalized_email,
       syntax_status         = b.syntax_status,
       syntax_reason         = b.syntax_reason,
       syntax_latency_ms     = b.syntax_latency_ms,
       syntax_score          = b.syntax_score,
       domain_dns_status     = b.domain_dns_status,
       domain_dns_reason     = b.domain_dns_reason,
       domain_dns_latency_ms = b.domain_dns_latency_ms,
       domain_dns_score      = b.domain_dns_score,
       mx_status             = b.mx_status,
       mx_reason             = b.mx_reason,
       mx_latency_ms         = b.mx_latency_ms,
       mx_score              = b.mx_score,
       smtp_status           = b.smtp_status,
       smtp_reason           = b.smtp_reason,
       smtp_latency_ms       = b.smtp_latency_ms,
       smtp_score            = b.smtp_score,
       total_score           = b.total_score,
       updated_at            = now()
FROM   _val_batch b
WHERE  vr.contact_id = b.contact_id
`); err != nil {
		return fmt.Errorf("BulkUpdateValidationByContactID: apply updates: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("BulkUpdateValidationByContactID: commit: %w", err)
	}
	return nil
}

func (r *Repo) GetPendingValidationResult(ctx context.Context, runID string) (*ValidationRow, error) {
	row := ValidationRow{}
	err := r.pool.QueryRow(ctx, `
with candidate as (
  select id
  from validation_results
  where run_id = $1 and status = 'pending'
  order by row_number
  for update skip locked
  limit 1
)
update validation_results vr
set status = 'in_progress',
    updated_at = now()
from candidate c
where vr.id = c.id
returning vr.id, vr.run_id::text, vr.row_number, vr.contact_id, vr.raw_contact_key
`, runID).Scan(&row.ID, &row.RunID, &row.RowNumber, &row.ContactID, &row.RawContactKey)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &row, nil
}

func (r *Repo) ClaimPendingValidationResults(ctx context.Context, runID string, limit int) ([]ValidationRow, error) {
	if limit <= 0 {
		limit = 1000
	}
	rows, err := r.pool.Query(ctx, `
with candidate as (
  select id
  from validation_results
  where run_id = $1 and status = 'pending'
  order by row_number
  for update skip locked
  limit $2
)
update validation_results vr
set status='in_progress',
    updated_at=now()
from candidate c
where vr.id = c.id
returning vr.id, vr.run_id::text, vr.row_number, vr.contact_id, vr.raw_contact_key
`, runID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]ValidationRow, 0, limit)
	for rows.Next() {
		var row ValidationRow
		if err := rows.Scan(&row.ID, &row.RunID, &row.RowNumber, &row.ContactID, &row.RawContactKey); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (r *Repo) BulkUpdateValidationByID(ctx context.Context, updates []ValidationUpdate) error {
	if len(updates) == 0 {
		return nil
	}

	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("BulkUpdateValidationByID: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `
CREATE TEMP TABLE _val_batch_by_id (
    id                    bigint NOT NULL,
    status                text NOT NULL,
    failure_reason        text NOT NULL DEFAULT '',
    clean_candidate       text NOT NULL DEFAULT '',
    normalized_email      text NOT NULL DEFAULT '',
    syntax_status         text NOT NULL DEFAULT '',
    syntax_reason         text NOT NULL DEFAULT '',
    syntax_latency_ms     int  NOT NULL DEFAULT 0,
    syntax_score          int  NOT NULL DEFAULT 0,
    domain_dns_status     text NOT NULL DEFAULT '',
    domain_dns_reason     text NOT NULL DEFAULT '',
    domain_dns_latency_ms int  NOT NULL DEFAULT 0,
    domain_dns_score      int  NOT NULL DEFAULT 0,
    mx_status             text NOT NULL DEFAULT '',
    mx_reason             text NOT NULL DEFAULT '',
    mx_latency_ms         int  NOT NULL DEFAULT 0,
    mx_score              int  NOT NULL DEFAULT 0,
    smtp_status           text NOT NULL DEFAULT '',
    smtp_reason           text NOT NULL DEFAULT '',
    smtp_latency_ms       int  NOT NULL DEFAULT 0,
    smtp_score            int  NOT NULL DEFAULT 0,
    history_status        text NOT NULL DEFAULT '',
    history_reason        text NOT NULL DEFAULT '',
    history_score         int  NOT NULL DEFAULT 0,
    total_score           int  NOT NULL DEFAULT 0
) ON COMMIT DROP
`); err != nil {
		return fmt.Errorf("BulkUpdateValidationByID: create temp table: %w", err)
	}

	vals := make([][]any, len(updates))
	for i, u := range updates {
		vals[i] = []any{
			u.ID, u.Status, u.FailureReason, u.CleanCandidate, u.NormalizedEmail,
			u.SyntaxStatus, u.SyntaxReason, u.SyntaxLatencyMS, u.SyntaxScore,
			u.DomainDNSStatus, u.DomainDNSReason, u.DomainLatencyMS, u.DomainScore,
			u.MXStatus, u.MXReason, u.MXLatencyMS, u.MXScore,
			u.SMTPStatus, u.SMTPReason, u.SMTPLatencyMS, u.SMTPScore,
			u.HistoryStatus, u.HistoryReason, u.HistoryScore, u.TotalScore,
		}
	}

	if _, err := tx.CopyFrom(
		ctx,
		pgx.Identifier{"_val_batch_by_id"},
		[]string{
			"id", "status", "failure_reason", "clean_candidate", "normalized_email",
			"syntax_status", "syntax_reason", "syntax_latency_ms", "syntax_score",
			"domain_dns_status", "domain_dns_reason", "domain_dns_latency_ms", "domain_dns_score",
			"mx_status", "mx_reason", "mx_latency_ms", "mx_score",
			"smtp_status", "smtp_reason", "smtp_latency_ms", "smtp_score",
			"history_status", "history_reason", "history_score", "total_score",
		},
		pgx.CopyFromRows(vals),
	); err != nil {
		return fmt.Errorf("BulkUpdateValidationByID: copy rows: %w", err)
	}

	if _, err := tx.Exec(ctx, `
UPDATE validation_results vr
SET    status                = b.status,
       failure_reason        = b.failure_reason,
       clean_candidate       = b.clean_candidate,
       normalized_email      = b.normalized_email,
       syntax_status         = b.syntax_status,
       syntax_reason         = b.syntax_reason,
       syntax_latency_ms     = b.syntax_latency_ms,
       syntax_score          = b.syntax_score,
       domain_dns_status     = b.domain_dns_status,
       domain_dns_reason     = b.domain_dns_reason,
       domain_dns_latency_ms = b.domain_dns_latency_ms,
       domain_dns_score      = b.domain_dns_score,
       mx_status             = b.mx_status,
       mx_reason             = b.mx_reason,
       mx_latency_ms         = b.mx_latency_ms,
       mx_score              = b.mx_score,
       smtp_status           = b.smtp_status,
       smtp_reason           = b.smtp_reason,
       smtp_latency_ms       = b.smtp_latency_ms,
       smtp_score            = b.smtp_score,
       history_status        = b.history_status,
       history_reason        = b.history_reason,
       history_score         = b.history_score,
       total_score           = b.total_score,
       updated_at            = now()
FROM   _val_batch_by_id b
WHERE  vr.id = b.id
`); err != nil {
		return fmt.Errorf("BulkUpdateValidationByID: apply updates: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("BulkUpdateValidationByID: commit: %w", err)
	}
	return nil
}
