package smfcvalidate

import (
	"context"
	"fmt"
	"io"
	"os"
	"runtime"

	"sf/usecases/mail-checker/internal/config"
	"sf/usecases/mail-checker/internal/db"
	"sf/usecases/mail-checker/internal/runner"
	"sf/usecases/mail-checker/internal/validator"
)

// Run validates SubscriberKey__c rows from the CSV at sourcePath into validation tables.
// It applies DB migrations, then runs CSVValidationProcessor (resumable).
func Run(ctx context.Context, cfg config.Config, sourcePath string, stdout io.Writer) error {
	if cfg.DBDSN == "" {
		return fmt.Errorf("DB_DSN is required")
	}
	if _, err := os.Stat(sourcePath); err != nil {
		return fmt.Errorf("source csv not found at %s: %w", sourcePath, err)
	}

	pool, err := db.Open(ctx, cfg.DBDSN)
	if err != nil {
		return err
	}
	defer pool.Close()

	if err := db.ApplyMigrations(ctx, pool); err != nil {
		return err
	}

	repo := db.NewRepo(pool)
	validatorSvc := validator.NewService()
	processor := &runner.CSVValidationProcessor{
		Repo:      repo,
		Validator: validatorSvc,
		Source:    sourcePath,
		Options: runner.CSVValidationOptions{
			SeedBatchSize:   10000,
			ClaimBatchSize:  1000,
			UpdateBatchSize: 1000,
			WorkerCount:     max(8, runtime.NumCPU()*2),
		},
	}

	runID, resumed, err := processor.Run(ctx)
	if err != nil {
		return err
	}
	if resumed {
		fmt.Fprintf(stdout, "validation run resumed and completed run_id=%s\n", runID)
	} else {
		fmt.Fprintf(stdout, "validation run completed run_id=%s\n", runID)
	}
	return nil
}
