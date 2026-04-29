package runner

import (
	"bufio"
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"log"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"sf/usecases/mail-checker/internal/db"
	"sf/usecases/mail-checker/internal/validator"
)

const (
	defaultSeedBatchSize   = 10000
	defaultClaimBatchSize  = 1000
	defaultUpdateBatchSize = 1000
	defaultProgressEvery   = 5 * time.Second
	defaultIdleSleep       = 500 * time.Millisecond
)

type CSVValidationOptions struct {
	SeedBatchSize   int
	ClaimBatchSize  int
	UpdateBatchSize int
	WorkerCount     int
	ProgressEvery   time.Duration
	IdleSleep       time.Duration
}

type CSVValidationProcessor struct {
	Repo      CSVValidationRepo
	Validator *validator.Service
	Source    string
	Options   CSVValidationOptions
}

type CSVValidationRepo interface {
	FindLatestUnfinishedValidationRunBySource(ctx context.Context, sourceFile string) (string, error)
	CreateValidationRun(ctx context.Context, sourceFile string, totalRows int) (string, error)
	ReopenValidationRun(ctx context.Context, runID string) error
	RequeueValidationInProgress(ctx context.Context, runID string) (int64, error)
	MarkValidationRunFailed(ctx context.Context, runID, reason string) error
	UpdateValidationRunTotalRows(ctx context.Context, runID string, totalRows int) error
	CreateValidationResults(ctx context.Context, runID string, rows []db.CreateValidationResultInput) error
	GetValidationRunProgress(ctx context.Context, runID string) (db.ValidationRunProgress, error)
	ClaimPendingValidationResults(ctx context.Context, runID string, limit int) ([]db.ValidationRow, error)
	BulkUpdateValidationByID(ctx context.Context, updates []db.ValidationUpdate) error
	CompleteValidationRun(ctx context.Context, runID string) (bool, error)
}

type csvValidationCounters struct {
	readRows         int64
	seededRows       int64
	claimedRows      int64
	validatedRows    int64
	doneRows         int64
	failedRows       int64
	flushCount       int64
	flushRows        int64
	activeWorkers    int64
	lastFlushLatency int64
}

func (p *CSVValidationProcessor) withDefaults() {
	if p.Options.SeedBatchSize <= 0 {
		p.Options.SeedBatchSize = defaultSeedBatchSize
	}
	if p.Options.ClaimBatchSize <= 0 {
		p.Options.ClaimBatchSize = defaultClaimBatchSize
	}
	if p.Options.UpdateBatchSize <= 0 {
		p.Options.UpdateBatchSize = defaultUpdateBatchSize
	}
	if p.Options.WorkerCount <= 0 {
		workers := runtime.NumCPU() * 2
		if workers < 4 {
			workers = 4
		}
		if workers > 128 {
			workers = 128
		}
		p.Options.WorkerCount = workers
	}
	if p.Options.ProgressEvery <= 0 {
		p.Options.ProgressEvery = defaultProgressEvery
	}
	if p.Options.IdleSleep <= 0 {
		p.Options.IdleSleep = defaultIdleSleep
	}
}

func (p *CSVValidationProcessor) Validate() error {
	if p.Repo == nil {
		return fmt.Errorf("csv validator repo is required")
	}
	if p.Validator == nil {
		return fmt.Errorf("csv validator service is required")
	}
	if strings.TrimSpace(p.Source) == "" {
		return fmt.Errorf("csv source file is required")
	}
	p.withDefaults()
	return nil
}

func (p *CSVValidationProcessor) Run(ctx context.Context) (string, bool, error) {
	if err := p.Validate(); err != nil {
		return "", false, err
	}

	startedAt := time.Now()
	var (
		runID   string
		err     error
		resumed bool
	)

	runID, err = p.Repo.FindLatestUnfinishedValidationRunBySource(ctx, p.Source)
	if err != nil {
		return "", false, err
	}
	if runID == "" {
		runID, err = p.Repo.CreateValidationRun(ctx, p.Source, 0)
		if err != nil {
			return "", false, err
		}
	} else {
		resumed = true
		if err := p.Repo.ReopenValidationRun(ctx, runID); err != nil {
			return "", true, err
		}
	}

	log.Printf("[csv-validation] run started run_id=%s source=%s resumed=%t workers=%d seed_batch=%d claim_batch=%d update_batch=%d",
		runID, p.Source, resumed, p.Options.WorkerCount, p.Options.SeedBatchSize, p.Options.ClaimBatchSize, p.Options.UpdateBatchSize)

	var counters csvValidationCounters
	if resumed {
		requeued, rqErr := p.Repo.RequeueValidationInProgress(ctx, runID)
		if rqErr != nil {
			_ = p.Repo.MarkValidationRunFailed(ctx, runID, rqErr.Error())
			return runID, true, rqErr
		}
		progress, progressErr := p.Repo.GetValidationRunProgress(ctx, runID)
		if progressErr == nil {
			atomic.StoreInt64(&counters.readRows, int64(progress.TotalRows))
		}
		log.Printf("[csv-validation] resumed run_id=%s requeued_in_progress=%d", runID, requeued)
	} else {
		if err := p.seedCSV(ctx, runID, &counters); err != nil {
			_ = p.Repo.MarkValidationRunFailed(ctx, runID, err.Error())
			return runID, false, err
		}

		if err := p.Repo.UpdateValidationRunTotalRows(ctx, runID, int(atomic.LoadInt64(&counters.readRows))); err != nil {
			_ = p.Repo.MarkValidationRunFailed(ctx, runID, err.Error())
			return runID, false, err
		}

		log.Printf("[csv-validation] ingestion completed run_id=%s rows_read=%d rows_seeded=%d",
			runID, atomic.LoadInt64(&counters.readRows), atomic.LoadInt64(&counters.seededRows))
	}

	if err := p.processPendingRows(ctx, runID, &counters, startedAt); err != nil {
		_ = p.Repo.MarkValidationRunFailed(ctx, runID, err.Error())
		return runID, resumed, err
	}

	totalDuration := time.Since(startedAt)
	summary, sumErr := p.Repo.GetValidationRunProgress(ctx, runID)
	if sumErr != nil {
		log.Printf("[csv-validation] failed to read final summary run_id=%s err=%v", runID, sumErr)
	}

	rate := float64(atomic.LoadInt64(&counters.validatedRows)) / totalDuration.Seconds()
	log.Printf("[csv-validation] completed run_id=%s total_rows=%d validated=%d done=%d failed=%d pending=%d in_progress=%d duration=%s avg_rows_sec=%.2f db_flushes=%d db_flushed_rows=%d",
		runID,
		summary.TotalRows,
		atomic.LoadInt64(&counters.validatedRows),
		summary.Done,
		summary.Failed,
		summary.Pending,
		summary.InProgress,
		totalDuration.Truncate(time.Millisecond),
		rate,
		atomic.LoadInt64(&counters.flushCount),
		atomic.LoadInt64(&counters.flushRows),
	)
	return runID, resumed, nil
}

func (p *CSVValidationProcessor) seedCSV(ctx context.Context, runID string, counters *csvValidationCounters) error {
	f, err := os.Open(p.Source)
	if err != nil {
		return err
	}
	defer f.Close()

	reader := bufio.NewReaderSize(f, 1<<20)
	delim, err := detectDelimiter(reader)
	if err != nil {
		return err
	}
	csvr := csv.NewReader(reader)
	csvr.FieldsPerRecord = -1
	csvr.ReuseRecord = true
	csvr.Comma = delim

	header, err := csvr.Read()
	if err != nil {
		return fmt.Errorf("read csv header: %w", err)
	}
	keyIdx, idIdx, err := findCSVIndexes(header)
	if err != nil {
		return err
	}

	seedStarted := time.Now()
	batch := make([]db.CreateValidationResultInput, 0, p.Options.SeedBatchSize)
	rowNumber := 0
	lastReadAt := time.Now()

	flushSeed := func() error {
		if len(batch) == 0 {
			return nil
		}
		if err := p.Repo.CreateValidationResults(ctx, runID, batch); err != nil {
			return err
		}
		atomic.AddInt64(&counters.seededRows, int64(len(batch)))
		batch = batch[:0]
		return nil
	}

	for {
		record, err := csvr.Read()
		if err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("read csv row: %w", err)
		}
		if len(record) <= keyIdx || len(record) <= idIdx {
			continue
		}

		now := time.Now()
		if now.Sub(lastReadAt) > 15*time.Second {
			log.Printf("[csv-validation][warn] csv read stall run_id=%s stall=%s rows_read=%d",
				runID, now.Sub(lastReadAt).Truncate(time.Millisecond), atomic.LoadInt64(&counters.readRows))
		}
		lastReadAt = now

		rowNumber++
		atomic.AddInt64(&counters.readRows, 1)
		batch = append(batch, db.CreateValidationResultInput{
			RowNumber:     rowNumber,
			ContactID:     strings.TrimSpace(record[idIdx]),
			RawContactKey: strings.TrimSpace(record[keyIdx]),
		})

		if len(batch) >= p.Options.SeedBatchSize {
			if err := flushSeed(); err != nil {
				return err
			}
		}
	}
	if err := flushSeed(); err != nil {
		return err
	}

	log.Printf("[csv-validation] ingestion checkpoint run_id=%s rows=%d seeded=%d duration=%s",
		runID, rowNumber, atomic.LoadInt64(&counters.seededRows), time.Since(seedStarted).Truncate(time.Millisecond))
	return nil
}

func (p *CSVValidationProcessor) processPendingRows(ctx context.Context, runID string, counters *csvValidationCounters, startedAt time.Time) error {
	jobs := make(chan db.ValidationRow, p.Options.ClaimBatchSize*2)
	results := make(chan db.ValidationUpdate, p.Options.UpdateBatchSize*2)
	errCh := make(chan error, 1)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var wg sync.WaitGroup
	for i := 0; i < p.Options.WorkerCount; i++ {
		wg.Add(1)
		go func(workerNum int) {
			defer wg.Done()
			for row := range jobs {
				atomic.AddInt64(&counters.activeWorkers, 1)
				validateStart := time.Now()
				out := p.Validator.Validate(ctx, row.RawContactKey)
				validateDur := time.Since(validateStart)
				atomic.AddInt64(&counters.activeWorkers, -1)

				if validateDur > 5*time.Second {
					log.Printf("[csv-validation][warn] slow validation run_id=%s worker=%d row=%d contact_id=%s latency=%s",
						runID, workerNum, row.RowNumber, row.ContactID, validateDur.Truncate(time.Millisecond))
				}

				status := "done"
				if out.Status == "failed" {
					status = "failed"
					atomic.AddInt64(&counters.failedRows, 1)
				} else {
					atomic.AddInt64(&counters.doneRows, 1)
				}

				update := db.ValidationUpdate{
					ID:              row.ID,
					Status:          status,
					FailureReason:   out.Reason,
					CleanCandidate:  out.Clean.Cleaned,
					NormalizedEmail: out.Clean.Normalized,
					SyntaxStatus:    out.Syntax.Status,
					SyntaxReason:    out.Syntax.Reason,
					SyntaxLatencyMS: out.Syntax.LatencyMS,
					SyntaxScore:     out.Syntax.Score,
					DomainDNSStatus: out.Domain.Status,
					DomainDNSReason: out.Domain.Reason,
					DomainLatencyMS: out.Domain.LatencyMS,
					DomainScore:     out.Domain.Score,
					MXStatus:        out.MX.Status,
					MXReason:        out.MX.Reason,
					MXLatencyMS:     out.MX.LatencyMS,
					MXScore:         out.MX.Score,
					SMTPStatus:      out.SMTP.Status,
					SMTPReason:      out.SMTP.Reason,
					SMTPLatencyMS:   out.SMTP.LatencyMS,
					SMTPScore:       out.SMTP.Score,
					HistoryStatus:   "pending",
					HistoryReason:   "fetch on demand",
					HistoryScore:    0,
					TotalScore:      out.Total,
				}

				select {
				case <-ctx.Done():
					return
				case results <- update:
					atomic.AddInt64(&counters.validatedRows, 1)
				}
			}
		}(i + 1)
	}

	flushDone := make(chan struct{})
	go func() {
		defer close(flushDone)
		batch := make([]db.ValidationUpdate, 0, p.Options.UpdateBatchSize)
		timer := time.NewTicker(1200 * time.Millisecond)
		defer timer.Stop()

		flush := func() error {
			if len(batch) == 0 {
				return nil
			}
			start := time.Now()
			if err := p.Repo.BulkUpdateValidationByID(ctx, batch); err != nil {
				return err
			}
			latency := time.Since(start)
			atomic.StoreInt64(&counters.lastFlushLatency, latency.Milliseconds())
			atomic.AddInt64(&counters.flushCount, 1)
			atomic.AddInt64(&counters.flushRows, int64(len(batch)))
			if latency > 2*time.Second {
				log.Printf("[csv-validation][warn] slow db flush run_id=%s rows=%d latency=%s", runID, len(batch), latency.Truncate(time.Millisecond))
			}
			log.Printf("[csv-validation] db flush checkpoint run_id=%s rows=%d latency=%s total_flushed=%d",
				runID, len(batch), latency.Truncate(time.Millisecond), atomic.LoadInt64(&counters.flushRows))
			batch = batch[:0]
			return nil
		}

		for {
			select {
			case <-ctx.Done():
				return
			case r, ok := <-results:
				if !ok {
					if err := flush(); err != nil {
						select {
						case errCh <- err:
						default:
						}
						cancel()
					}
					return
				}
				batch = append(batch, r)
				if len(batch) >= p.Options.UpdateBatchSize {
					if err := flush(); err != nil {
						select {
						case errCh <- err:
						default:
						}
						cancel()
						return
					}
				}
			case <-timer.C:
				if err := flush(); err != nil {
					select {
					case errCh <- err:
					default:
					}
					cancel()
					return
				}
			}
		}
	}()

	progressDone := make(chan struct{})
	go func() {
		defer close(progressDone)
		t := time.NewTicker(p.Options.ProgressEvery)
		defer t.Stop()
		prevValidated := int64(0)
		prevTime := time.Now()

		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				now := time.Now()
				current := atomic.LoadInt64(&counters.validatedRows)
				diff := current - prevValidated
				deltaSec := now.Sub(prevTime).Seconds()
				instRate := float64(diff) / deltaSec
				avgRate := float64(current) / now.Sub(startedAt).Seconds()
				prevValidated = current
				prevTime = now

				progress, err := p.Repo.GetValidationRunProgress(ctx, runID)
				if err != nil {
					log.Printf("[csv-validation][warn] progress lookup failed run_id=%s err=%v", runID, err)
					continue
				}

				log.Printf("[csv-validation] progress run_id=%s elapsed=%s read=%d validated=%d pending=%d in_progress=%d done=%d failed=%d rate_inst=%.2f/s rate_avg=%.2f/s workers=%d/%d last_flush_ms=%d",
					runID,
					now.Sub(startedAt).Truncate(time.Second),
					atomic.LoadInt64(&counters.readRows),
					current,
					progress.Pending,
					progress.InProgress,
					progress.Done,
					progress.Failed,
					instRate,
					avgRate,
					atomic.LoadInt64(&counters.activeWorkers),
					p.Options.WorkerCount,
					atomic.LoadInt64(&counters.lastFlushLatency),
				)
			}
		}
	}()

	dispatchDone := make(chan struct{})
	go func() {
		defer close(dispatchDone)
		for {
			if ctx.Err() != nil {
				return
			}

			rows, err := p.Repo.ClaimPendingValidationResults(ctx, runID, p.Options.ClaimBatchSize)
			if err != nil {
				select {
				case errCh <- err:
				default:
				}
				cancel()
				return
			}

			if len(rows) == 0 {
				done, err := p.Repo.CompleteValidationRun(ctx, runID)
				if err != nil {
					select {
					case errCh <- err:
					default:
					}
					cancel()
					return
				}
				if done {
					return
				}
				select {
				case <-ctx.Done():
					return
				case <-time.After(p.Options.IdleSleep):
					continue
				}
			}

			atomic.AddInt64(&counters.claimedRows, int64(len(rows)))
			for _, row := range rows {
				select {
				case <-ctx.Done():
					return
				case jobs <- row:
				}
			}
		}
	}()

	select {
	case err := <-errCh:
		cancel()
		<-dispatchDone
		close(jobs)
		wg.Wait()
		close(results)
		<-flushDone
		<-progressDone
		return err
	case <-dispatchDone:
		close(jobs)
		wg.Wait()
		close(results)
		<-flushDone
		cancel()
		<-progressDone
		return nil
	}
}

func detectDelimiter(r *bufio.Reader) (rune, error) {
	peek, err := r.Peek(64 * 1024)
	if err != nil && err != io.EOF && err != bufio.ErrBufferFull {
		return ',', err
	}
	line := string(peek)
	newlineIdx := strings.IndexByte(line, '\n')
	if newlineIdx >= 0 {
		line = line[:newlineIdx]
	}
	if strings.Count(line, "\t") > strings.Count(line, ",") {
		return '\t', nil
	}
	return ',', nil
}

func findCSVIndexes(header []string) (keyIndex int, idIndex int, err error) {
	keyIndex = -1
	idIndex = -1
	for i, h := range header {
		col := strings.TrimSpace(h)
		if col == "SubscriberKey__c" {
			keyIndex = i
		}
		if col == "SubscriberID__c" {
			idIndex = i
		}
	}
	if keyIndex < 0 {
		return -1, -1, fmt.Errorf("missing required column SubscriberKey__c")
	}
	if idIndex < 0 {
		return -1, -1, fmt.Errorf("missing required column SubscriberID__c")
	}
	return keyIndex, idIndex, nil
}
