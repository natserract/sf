package runner

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"math/rand"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"sf/usecases/mail-checker/fetch-all-contacts-go/internal/api"
	"sf/usecases/mail-checker/fetch-all-contacts-go/internal/db"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/sync/errgroup"
)

// Fetch engagement history for all contacts concurrently, then batch-update.
const maxEngagementWorkers = 20

type Options struct {
	MaxInFlight    int
	MaxAttempts    int
	RetryInitial   time.Duration
	RetryMax       time.Duration
	LockTimeout    time.Duration
	IdleSleep      time.Duration
	ReapInterval   time.Duration
	FilterOperator string
	FilterValue    string
	OrderBy        string
}

type Processor struct {
	DB       *pgxpool.Pool
	Repo     *db.Repo
	API      *api.Client
	Auth     api.Auth
	AuthMgr  *AuthManager
	WorkerID string
	Options  Options
	Stdin    io.Reader
	Stdout   io.Writer
	jobCh    chan engagementJob // set by Run()
}

type engagementJob struct {
	contact  api.ContactInfo
	runID    string
	pageKey  string // opaque key so the collector knows which page
	resultCh chan<- engagementResult
}

type engagementResult struct {
	contactID  string
	hasHistory bool
	err        error
}

func (p *Processor) Validate() error {
	if p.DB == nil || p.Repo == nil || p.API == nil {
		return fmt.Errorf("runner missing DB/Repo/API")
	}
	if p.WorkerID == "" {
		return fmt.Errorf("runner worker id is required")
	}
	if p.Stdin == nil {
		return fmt.Errorf("runner stdin is required")
	}
	if p.Stdout == nil {
		return fmt.Errorf("runner stdout is required")
	}
	if p.AuthMgr == nil {
		p.AuthMgr = NewAuthManager(p.Auth, p.Stdin, p.Stdout, 3)
	}
	if p.Options.MaxInFlight <= 0 {
		p.Options.MaxInFlight = 50
	}
	if p.Options.MaxAttempts <= 0 {
		p.Options.MaxAttempts = 8
	}
	if p.Options.RetryInitial <= 0 {
		p.Options.RetryInitial = time.Second
	}
	if p.Options.RetryMax <= 0 {
		p.Options.RetryMax = time.Minute
	}
	if p.Options.LockTimeout <= 0 {
		p.Options.LockTimeout = 2 * time.Minute
	}
	if p.Options.IdleSleep <= 0 {
		p.Options.IdleSleep = 2 * time.Second
	}
	if p.Options.ReapInterval <= 0 {
		p.Options.ReapInterval = 10 * time.Second
	}
	if p.Options.FilterOperator == "" {
		p.Options.FilterOperator = "Is"
	}
	if p.Options.FilterValue == "" {
		p.Options.FilterValue = "MOBILE"
	}
	if p.Options.OrderBy == "" {
		p.Options.OrderBy = "contactKey ASC"
	}
	return nil
}

func (p *Processor) Run(ctx context.Context, runID string) error {
	if err := p.Validate(); err != nil {
		return err
	}

	run, err := p.Repo.GetRun(ctx, runID)
	if err != nil {
		return err
	}

	// Shared job queue. Buffer = engagementWorkers × 4 gives backpressure
	// without blocking page workers unnecessarily.
	// jobCh := make(chan engagementJob, maxEngagementWorkers*4)

	g, gctx := errgroup.WithContext(ctx)

	for i := 0; i < p.Options.MaxInFlight; i++ {
		g.Go(func() error {
			return p.workerLoop(gctx, run)
		})
	}

	// Shared engagement pool — drains jobCh until it's closed.
	// g.Go(func() error {
	// 	return p.engagementPool(gctx, jobCh)
	// })

	g.Go(func() error {
		t := time.NewTicker(p.Options.ReapInterval)
		defer t.Stop()
		for {
			select {
			case <-gctx.Done():
				return nil
			case <-t.C:
				if _, err := p.Repo.ReapStaleInProgress(gctx, runID, p.Options.LockTimeout); err != nil {
					return err
				}
			}
		}
	})
	return g.Wait()
}

func (p *Processor) workerLoop(ctx context.Context, run db.Run) error {
	for {
		if err := ctx.Err(); err != nil {
			return nil
		}
		claimed, err := p.Repo.ClaimNextPage(ctx, run.ID, p.WorkerID, p.Options.MaxAttempts)
		if err != nil {
			return err
		}
		if claimed == nil {
			done, err := p.Repo.MarkRunCompletedIfDrained(ctx, run.ID)
			if err != nil {
				return err
			}
			if done {
				return nil
			}
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(p.Options.IdleSleep):
				continue
			}
		}
		if err := p.processPage(ctx, run, claimed.PageNumber, claimed.Attempts); err != nil {
			return err
		}
	}
}

// Runs engagementWorkers goroutines, each pulling from jobCh.
// Uses its own independent context so a single fetch failure doesn't
// nuke unrelated page workers.
func (p *Processor) engagementPool(ctx context.Context, jobCh <-chan engagementJob) error {
	g, gctx := errgroup.WithContext(ctx)
	sem := make(chan struct{}, maxEngagementWorkers)

	for job := range jobCh {
		job := job
		select {
		case sem <- struct{}{}:
		case <-gctx.Done():
			return gctx.Err()
		}
		g.Go(func() error {
			defer func() { <-sem }()

			const (
				maxAttempts    = 4
				baseTimeout    = 60 * time.Second // raised from 30s — history can be large
				maxTimeout     = 3 * time.Minute  // cap for later attempts
				retryBaseDelay = time.Second
			)

			var history api.MessageHistoryResponse
			var fetchErr error

			for attempt := 0; attempt < maxAttempts; attempt++ {
				// Exponential timeout per attempt: 60s, 90s, 135s, 180s (capped)
				// This handles "big history = slow response" without giving up early.
				timeout := time.Duration(float64(baseTimeout) * math.Pow(1.5, float64(attempt)))
				if timeout > maxTimeout {
					timeout = maxTimeout
				}

				// Own timeout, fully isolated from page ctx.
				reqCtx, cancel := context.WithTimeout(ctx, timeout)
				history, _, fetchErr = p.API.FetchMessageHistory(reqCtx, p.AuthMgr.GetAuth(), job.contact.ContactID)
				cancel()

				if fetchErr == nil {
					break
				}
				// Distinguish retryable from permanent.
				isTimeout := errors.Is(fetchErr, context.DeadlineExceeded) || reqCtx.Err() == context.DeadlineExceeded
				isNetErr := func() bool {
					var ne net.Error
					return errors.As(fetchErr, &ne)
				}()

				if !isTimeout && !isNetErr {
					// Permanent: 401, 403, 404, JSON parse error — don't retry.
					log.Printf("[ENGAGEMENT] permanent error contactID=%s attempt=%d err=%v",
						job.contact.ContactID, attempt+1, fetchErr)
					break
				}

				if attempt < maxAttempts-1 {
					delay := retryBaseDelay * (1 << attempt) // 1s, 2s, 4s
					log.Printf("[ENGAGEMENT] retryable error contactID=%s attempt=%d/%d timeout=%s retrying in %s err=%v",
						job.contact.ContactID, attempt+1, maxAttempts, timeout, delay, fetchErr)

					select {
					case <-ctx.Done():
						fetchErr = ctx.Err()
						goto done
					case <-time.After(delay):
					}
				}
			}

		done:
			job.resultCh <- engagementResult{
				contactID:  job.contact.ContactID,
				hasHistory: fetchErr == nil && len(history.DataSources) > 0,
				err:        fetchErr,
			}
			return nil // pool itself never returns an error; errors flow via resultCh
		})
	}
	return g.Wait()
}

func (p *Processor) processPage(ctx context.Context, run db.Run, pageNumber int, attempts int) error {
	fmt.Fprintf(p.Stdout, "[PROCESS] page=%d start\n", pageNumber)

	params := api.FetchPageParams{
		PageSize:                run.PageSize,
		Page:                    pageNumber,
		OrderBy:                 p.Options.OrderBy,
		FilterConditionOperator: run.FilterOperator,
		FilterConditionValue:    run.FilterValue,
	}
	usedAuth := p.AuthMgr.GetAuth()
	resp, httpResp, err := p.API.FetchPage(ctx, usedAuth, params)
	if err != nil {
		if httpResp != nil && httpResp.StatusCode == http.StatusForbidden {
			log.Printf("[PROCESS] page=%d got 403, attempting re-auth runID=%s", pageNumber, run.ID)
			if reauthErr := p.reauthenticate(ctx, usedAuth); reauthErr != nil {
				return p.handleProcessError(ctx, run.ID, pageNumber, attempts, httpResp, reauthErr)
			}
			log.Printf("[PROCESS] page=%d re-auth succeeded, retrying fetch runID=%s", pageNumber, run.ID)
			resp, httpResp, err = p.API.FetchPage(ctx, p.AuthMgr.GetAuth(), params)
		}
	}
	if err != nil {
		return p.handleProcessError(ctx, run.ID, pageNumber, attempts, httpResp, err)
	}

	contacts, empty := api.ExtractContactInfo(resp)

	// resultCh := make(chan engagementResult, len(contacts))
	// for _, c := range contacts {
	// 	p.jobCh <- engagementJob{ // jobCh stored on Processor (see below)
	// 		contact:  c,
	// 		runID:    run.ID,
	// 		resultCh: resultCh,
	// 	}
	// }

	// Collect exactly len(contacts) results. The pool handles retries;
	// this page worker just waits for its batch — but crucially, the
	// pool's goroutines are shared across ALL pages so other pages make
	// progress concurrently while we wait.
	// var withHistory, withoutHistory []string
	// for i := 0; i < len(contacts); i++ {
	// 	select {
	// 	case <-ctx.Done():
	// 		return ctx.Err()
	// 	case r := <-resultCh:
	// 		if r.err != nil {
	// 			isTimeout := errors.Is(r.err, context.DeadlineExceeded)

	// 			if isTimeout {
	// 				// History API timed out even after retries — treat as unknown,
	// 				// not as "no history". You can add a third bucket for this if needed.
	// 				log.Printf("[PROCESS] page=%d contactID=%s history timeout after retries, skipping",
	// 					pageNumber, r.contactID)
	// 			} else {
	// 				log.Printf("[PROCESS] page=%d contactID=%s history fetch error: %v",
	// 					pageNumber, r.contactID, r.err)
	// 			}
	// 			// Don't append to withoutHistory — don't write a false negative.
	// 			// Either skip entirely or add a "unknown" update if your schema supports it.
	// 			continue
	// 		}
	// 		if r.hasHistory {
	// 			withHistory = append(withHistory, r.contactID)
	// 		} else {
	// 			withoutHistory = append(withoutHistory, r.contactID)
	// 		}
	// 	}
	// }

	tx, err := p.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := p.Repo.InsertContactKeys(ctx, tx, run.ID, pageNumber, contacts); err != nil {
		log.Printf("[PROCESS] page=%d InsertContactKeys error runID=%s err=%v", pageNumber, run.ID, err)
		return err
	}

	// Batch update those WITH history (True)
	// if len(withHistory) > 0 {
	// 	log.Printf("[PROCESS] Updating %d contacts as HAS history", len(withHistory))
	// 	if err := p.Repo.BatchUpdateContactHasEngagementHistory(ctx, tx, withHistory, true); err != nil {
	// 		return err
	// 	}
	// }

	// // Batch update those WITHOUT history (False)
	// if len(withoutHistory) > 0 {
	// 	log.Printf("[PROCESS] Updating %d contacts as NO history", len(withoutHistory))
	// 	if err := p.Repo.BatchUpdateContactHasEngagementHistory(ctx, tx, withoutHistory, false); err != nil {
	// 		return err
	// 	}
	// }

	status := "done"
	if empty {
		status = "empty"
		if err := p.Repo.MarkRunStopPage(ctx, tx, run.ID, pageNumber); err != nil {
			return err
		}
	}
	if err := p.Repo.MarkPageDone(ctx, tx, run.ID, pageNumber, status); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	fmt.Fprintf(p.Stdout, "[PROCESS] page=%d done\n", pageNumber)
	totalContacts, err := p.Repo.GetRunTotalContacts(ctx, run.ID)
	if err != nil {
		return err
	}
	logProgress(ctx, p.Repo, run.ID, totalContacts)
	_, err = p.Repo.MarkRunCompletedIfDrained(ctx, run.ID)
	return err
}

func (p *Processor) reauthenticate(ctx context.Context, failedAuth api.Auth) error {
	return p.AuthMgr.ReauthenticateIfUnchanged(ctx, failedAuth, func(ctx context.Context, next api.Auth) error {
		resp, err := p.API.PingAuth(ctx, next, api.PingAuthParams{
			PageSize:                1,
			FilterConditionOperator: p.Options.FilterOperator,
			FilterConditionValue:    p.Options.FilterValue,
			OrderBy:                 p.Options.OrderBy,
		})
		if err == nil {
			return nil
		}
		if api.IsAuthError(resp) {
			return fmt.Errorf("http %d auth failed", resp.StatusCode)
		}
		return err
	})
}

func (p *Processor) handleProcessError(ctx context.Context, runID string, pageNumber int, attempts int, httpResp *http.Response, processErr error) error {
	errMsg := processErr.Error()
	retryable := isRetryableHTTP(httpResp) || isRetryableNet(processErr) || isRetryableDB(processErr)
	if isAuthError(httpResp) || strings.Contains(strings.ToLower(errMsg), "max re-auth attempts reached") {
		if err := withTx(ctx, p.DB, func(tx db.Tx) error {
			return p.Repo.MarkPageFailed(ctx, tx, runID, pageNumber, errMsg)
		}); err != nil {
			return err
		}
		return fmt.Errorf("auth failure: %w", processErr)
	}
	if !retryable || attempts >= p.Options.MaxAttempts {
		return withTx(ctx, p.DB, func(tx db.Tx) error {
			return p.Repo.MarkPageFailed(ctx, tx, runID, pageNumber, errMsg)
		})
	}

	nextAttempt := time.Now().Add(backoffWithJitter(attempts, p.Options.RetryInitial, p.Options.RetryMax))
	return withTx(ctx, p.DB, func(tx db.Tx) error {
		return p.Repo.MarkPageRetry(ctx, tx, runID, pageNumber, errMsg, nextAttempt)
	})
}

func withTx(ctx context.Context, pool *pgxpool.Pool, fn func(tx db.Tx) error) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback(ctx)
		return err
	}
	return tx.Commit(ctx)
}

func backoffWithJitter(attempt int, initial, max time.Duration) time.Duration {
	if attempt <= 1 {
		return initial
	}
	base := initial
	for i := 1; i < attempt; i++ {
		base *= 2
		if base >= max {
			base = max
			break
		}
	}
	jitterRange := base / 5
	if jitterRange <= 0 {
		return base
	}
	rnd := rand.New(rand.NewSource(time.Now().UnixNano()))
	jitter := time.Duration(rnd.Int63n(int64(jitterRange)*2)) - jitterRange
	out := base + jitter
	if out < initial {
		return initial
	}
	if out > max {
		return max
	}
	return out
}

func isRetryableHTTP(resp *http.Response) bool {
	if resp == nil {
		return false
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return true
	}
	return resp.StatusCode >= 500 && resp.StatusCode <= 599
}

func isAuthError(resp *http.Response) bool {
	if resp == nil {
		return false
	}
	return resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden
}

func isRetryableNet(err error) bool {
	var netErr net.Error
	if errors.As(err, &netErr) {
		return netErr.Timeout() || netErr.Temporary()
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	if errors.Is(err, context.Canceled) {
		// Treat cancellation as transient so interrupted workers can resume safely.
		return true
	}
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "connection reset") || strings.Contains(s, "broken pipe")
}

func isRetryableDB(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "40001" || pgErr.Code == "40P01"
	}
	return false
}

type Shutdown struct {
	once sync.Once
	done chan struct{}
}

func NewShutdown() *Shutdown {
	return &Shutdown{done: make(chan struct{})}
}

func (s *Shutdown) Stop() {
	s.once.Do(func() { close(s.done) })
}

func (s *Shutdown) Done() <-chan struct{} {
	return s.done
}

func logProgress(ctx context.Context, repo *db.Repo, runID string, totalCount int) {
	current, err := repo.CountContactKeys(ctx, runID)
	if err != nil {
		log.Printf("progress error: %v", err)
		return
	}

	percent := 0.0
	if totalCount > 0 {
		percent = float64(current) / float64(totalCount) * 100
	}

	log.Printf(
		"[PROGRESS] %d/%d (%.2f%%)",
		current,
		totalCount,
		percent,
	)
}
