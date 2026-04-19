package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repo struct {
	pool *pgxpool.Pool
}

func NewRepo(pool *pgxpool.Pool) *Repo {
	return &Repo{pool: pool}
}

type ValidationRun struct {
	ID          string     `json:"id"`
	SourceFile  string     `json:"sourceFile"`
	TotalRows   int        `json:"totalRows"`
	State       string     `json:"state"`
	CreatedAt   time.Time  `json:"createdAt"`
	StartedAt   *time.Time `json:"startedAt,omitempty"`
	CompletedAt *time.Time `json:"completedAt,omitempty"`
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

type CreateInput struct {
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

func (r *Repo) CreateValidationRun(ctx context.Context, sourceFile string, totalRows int) (string, error) {
	var id string
	err := r.pool.QueryRow(ctx, `
insert into validation_runs(source_file, total_rows, state, started_at)
values ($1, $2, 'running', now())
returning id::text
`, sourceFile, totalRows).Scan(&id)
	return id, err
}

func (r *Repo) InsertRows(ctx context.Context, runID string, rows []CreateInput) error {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	for _, row := range rows {
		_, err := tx.Exec(ctx, `
insert into validation_results(run_id, row_number, contact_id, raw_contact_key, status, history_status, history_reason)
values($1,$2,$3,$4,'pending','pending','not fetched yet')
`, runID, row.RowNumber, row.ContactID, row.RawContactKey)
		if err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (r *Repo) ClaimNextPending(ctx context.Context, runID string) (*ValidationRow, error) {
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

func (r *Repo) SaveHistory(ctx context.Context, rowID int64, status, reason string, score int, payload []byte) error {
	_, err := r.pool.Exec(ctx, `
update validation_results
set history_status=$2,
    history_reason=$3,
    history_score=$4,
    history_payload=$5,
    history_fetched_at=now(),
    updated_at=now()
where id=$1
`, rowID, status, reason, score, payload)
	return err
}

func (r *Repo) CompleteRunIfDone(ctx context.Context, runID string) (bool, error) {
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

func (r *Repo) RunProgress(ctx context.Context, runID string) (RunProgress, error) {
	var out RunProgress
	var updatedAt time.Time
	out.RunID = runID
	err := r.pool.QueryRow(ctx, `
select vr.state, vr.total_rows,
count(*) filter(where r.status='pending') as pending,
count(*) filter(where r.status='in_progress') as in_progress,
count(*) filter(where r.status='done') as done,
count(*) filter(where r.status='failed') as failed,
max(r.updated_at) as updated_at
from validation_runs vr
left join validation_results r on r.run_id = vr.id
where vr.id = $1
group by vr.state, vr.total_rows
`, runID).Scan(&out.State, &out.TotalRows, &out.Pending, &out.InProgress, &out.Done, &out.Failed, &updatedAt)
	if err != nil {
		return RunProgress{}, err
	}
	out.LastUpdated = updatedAt.Format(time.RFC3339)
	return out, nil
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
