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
	CompletedAt    *time.Time
	PageSize       int
	StartedPage    int
	BaseURL        string
	FilterOperator string
	FilterValue    string
}

type ClaimedPage struct {
	RunID      string
	PageNumber int
	Attempts   int
}

type Repo struct {
	pool *pgxpool.Pool
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
select id::text, created_at, state, stop_page, completed_at, page_size, started_page, base_url, filter_operator, filter_value
from runs where id = $1
`, runID).Scan(&out.ID, &out.CreatedAt, &out.State, &out.StopPage, &out.CompletedAt, &out.PageSize, &out.StartedPage, &out.BaseURL, &out.FilterOperator, &out.FilterValue)
	if err != nil {
		return Run{}, err
	}
	return out, nil
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
)
update pages p
set status='in_progress',
    attempts=p.attempts + 1,
    last_error=null,
    started_at=coalesce(p.started_at, now()),
    locked_by=$2,
    locked_at=now(),
    updated_at=now()
from candidate c
where p.run_id = c.run_id and p.page_number = c.page_number
returning p.run_id::text, p.page_number, p.attempts
`, runID, workerID, maxAttempts).Scan(&c.RunID, &c.PageNumber, &c.Attempts)
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

func (r *Repo) ReapStaleInProgress(ctx context.Context, runID string, olderThan time.Duration) (int64, error) {
	ct, err := r.pool.Exec(ctx, `
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

func (r *Repo) InsertContactKeys(ctx context.Context, tx Tx, runID string, page int, contacts []api.ContactInfo) (int64, error) {
	var inserted int64
	for _, c := range contacts {
		ct, err := tx.Exec(ctx, `
insert into contact_keys(contact_key, contact_id, first_seen_run_id, first_seen_page)
values ($1,$2,$3,$4)
on conflict do nothing
`, []byte(c.ContactKey), c.ContactID, runID, page)
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
