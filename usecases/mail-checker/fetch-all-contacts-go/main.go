package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"syscall"

	"github.com/joho/godotenv"
	"github.com/spf13/cobra"

	"sf/usecases/mail-checker/fetch-all-contacts-go/internal/api"
	"sf/usecases/mail-checker/fetch-all-contacts-go/internal/config"
	"sf/usecases/mail-checker/fetch-all-contacts-go/internal/db"
	"sf/usecases/mail-checker/fetch-all-contacts-go/internal/runner"
)

func main() {
	_ = godotenv.Load()

	root := &cobra.Command{
		Use:   "fetch-all-contacts-go",
		Short: "Fetch all contacts via paged API using Postgres-backed workers",
	}

	var bearerToken string
	var csrfToken string
	var cookie string

	root.PersistentFlags().StringVar(&bearerToken, "bearer-token", "", "Bearer token (no 'Bearer ' prefix)")
	root.PersistentFlags().StringVar(&csrfToken, "csrf-token", "", "X-CSRF-Token header value")
	root.PersistentFlags().StringVar(&cookie, "cookie", "", "Cookie header value")

	root.AddCommand(newStartRunCmd(func() (config.Config, api.Auth, error) {
		cfg, err := config.FromEnvFull()
		if err != nil {
			return config.Config{}, api.Auth{}, err
		}
		auth := api.Auth{BearerToken: bearerToken, CsrfToken: csrfToken, Cookie: cookie}
		return cfg, auth, nil
	}))
	root.AddCommand(newWorkerCmd(func() (config.Config, api.Auth, error) {
		cfg, err := config.FromEnvFull()
		if err != nil {
			return config.Config{}, api.Auth{}, err
		}
		auth := api.Auth{BearerToken: bearerToken, CsrfToken: csrfToken, Cookie: cookie}
		return cfg, auth, nil
	}))
	root.AddCommand(newStatusCmd(func() (config.Config, error) {
		cfg, err := config.FromEnvFull()
		if err != nil {
			return config.Config{}, err
		}
		return cfg, nil
	}))
	root.AddCommand(newFetchOnceCmd(func() (api.Auth, config.Config, *api.Client, error) {
		cfg, err := config.FromEnv()
		if err != nil {
			return api.Auth{}, config.Config{}, nil, err
		}
		client := api.NewClient(cfg.APIBaseURL)
		auth := api.Auth{BearerToken: bearerToken, CsrfToken: csrfToken, Cookie: cookie}
		return auth, cfg, client, nil
	}))
	root.AddCommand(newResumeCmd(func() (config.Config, api.Auth, error) {
		cfg, err := config.FromEnvFull()
		if err != nil {
			return config.Config{}, api.Auth{}, err
		}
		auth := api.Auth{BearerToken: bearerToken, CsrfToken: csrfToken, Cookie: cookie}
		return cfg, auth, nil
	}))

	root.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Print version",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintln(cmd.OutOrStdout(), "dev")
			return nil
		},
	})

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

func newFetchOnceCmd(build func() (api.Auth, config.Config, *api.Client, error)) *cobra.Command {
	var (
		page           int
		pageSize       int
		orderBy        string
		filterOperator string
		filterValue    string
		verbose        bool
	)
	cmd := &cobra.Command{
		Use:   "fetch-once",
		Short: "Fetch a single page and print extracted keys",
		RunE: func(cmd *cobra.Command, args []string) error {
			auth, cfg, client, err := build()
			if err != nil {
				return err
			}

			// Flag overrides env/config defaults.
			ps := cfg.PageSize
			if pageSize > 0 {
				ps = pageSize
			}
			ob := "contactKey ASC"
			if orderBy != "" {
				ob = orderBy
			}
			fo := "Is"
			if filterOperator != "" {
				fo = filterOperator
			}
			fv := "MOBILE"
			if filterValue != "" {
				fv = filterValue
			}

			params := api.FetchPageParams{
				PageSize:                ps,
				Page:                    page,
				OrderBy:                 ob,
				FilterConditionOperator: fo,
				FilterConditionValue:    fv,
			}

			if verbose {
				fmt.Fprintf(cmd.OutOrStdout(), "fetching page=%d page_size=%d order_by=%q filter=%s:%s\n",
					page, ps, ob, fo, fv)
			}

			resp, httpResp, err := client.FetchPage(cmd.Context(), auth, params)
			if err != nil {
				if httpResp != nil {
					return fmt.Errorf("http %d: %w", httpResp.StatusCode, err)
				}
				return err
			}
			contacts, empty := api.ExtractContactInfo(resp)
			fmt.Fprintf(cmd.OutOrStdout(), "page=%d empty=%v total_count=%d keys=%d\n",
				page, empty, resp.TotalCount, len(contacts))
			for _, c := range contacts {
				fmt.Fprintln(cmd.OutOrStdout(), c)
			}
			return nil
		},
	}
	cmd.Flags().IntVar(&page, "page", 1, "Page number to fetch")
	cmd.Flags().IntVar(&pageSize, "page-size", 0, "Page size (default: PAGE_SIZE env or 25)")
	cmd.Flags().StringVar(&orderBy, "order-by", "", "Order-by clause (default: contactKey ASC)")
	cmd.Flags().StringVar(&filterOperator, "filter-operator", "", "Filter condition operator (default: Is)")
	cmd.Flags().StringVar(&filterValue, "filter-value", "", "Filter condition value (default: MOBILE)")
	cmd.Flags().BoolVar(&verbose, "verbose", false, "Print request details before fetching")
	return cmd
}

func newStartRunCmd(build func() (config.Config, api.Auth, error)) *cobra.Command {
	var startedPage int
	cmd := &cobra.Command{
		Use:   "start-run",
		Short: "Create a run in Postgres and pre-seed all pages as pending",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, auth, err := build()
			if err != nil {
				return err
			}
			if cfg.DBDSN == "" {
				return fmt.Errorf("DB_DSN is required")
			}
			if auth.BearerToken == "" || auth.CsrfToken == "" || auth.Cookie == "" {
				return fmt.Errorf("--bearer-token, --csrf-token, and --cookie are required for start-run")
			}

			pool, err := db.Open(cmd.Context(), cfg.DBDSN)
			if err != nil {
				return err
			}
			defer pool.Close()

			if err := db.ApplyMigrations(cmd.Context(), pool); err != nil {
				return err
			}

			apiClient := api.NewClient(cfg.APIBaseURL)
			if err := validateAuthPreflight(cmd.Context(), apiClient, auth, cfg); err != nil {
				return err
			}

			repo := db.NewRepo(pool)
			runID, totalCount, totalPages, seededRows, err := createRunAndSeedAllPages(cmd.Context(), repo, apiClient, auth, cfg, startedPage)
			if err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "run_id=%s total_count=%d total_pages=%d seeded_rows=%d started_page=%d\n", runID, totalCount, totalPages, seededRows, startedPage)
			return nil
		},
	}
	cmd.Flags().IntVar(&startedPage, "started-page", 1, "Starting page number")
	return cmd
}

func newWorkerCmd(build func() (config.Config, api.Auth, error)) *cobra.Command {
	var runID string
	cmd := &cobra.Command{
		Use:   "worker",
		Short: "Run distributed Postgres-backed workers to fetch pages",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, auth, err := build()
			if err != nil {
				return err
			}
			if cfg.DBDSN == "" {
				return fmt.Errorf("DB_DSN is required")
			}
			if auth.BearerToken == "" || auth.CsrfToken == "" || auth.Cookie == "" {
				return fmt.Errorf("--bearer-token, --csrf-token, and --cookie are required for worker")
			}

			pool, err := db.Open(cmd.Context(), cfg.DBDSN)
			if err != nil {
				return err
			}
			defer pool.Close()

			if err := db.ApplyMigrations(cmd.Context(), pool); err != nil {
				return err
			}

			repo := db.NewRepo(pool)
			apiClient := api.NewClient(cfg.APIBaseURL)
			if err := validateAuthPreflight(cmd.Context(), apiClient, auth, cfg); err != nil {
				return err
			}
			if runID == "" {
				var totalCount, totalPages int
				var seededRows int64
				runID, totalCount, totalPages, seededRows, err = createRunAndSeedAllPages(cmd.Context(), repo, apiClient, auth, cfg, 1)
				if err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "auto-created run_id=%s total_count=%d total_pages=%d seeded_rows=%d started_page=1\n", runID, totalCount, totalPages, seededRows)
			}
			workerID := fmt.Sprintf("%d-%d", os.Getpid(), rand.Int())
			proc := &runner.Processor{
				DB:       pool,
				Repo:     repo,
				API:      apiClient,
				Auth:     auth,
				WorkerID: workerID,
				Stdin:    cmd.InOrStdin(),
				Stdout:   cmd.OutOrStdout(),
				Options: runner.Options{
					MaxInFlight:  cfg.MaxInFlight,
					MaxAttempts:  cfg.MaxAttempts,
					RetryInitial: cfg.RetryInitial,
					RetryMax:     cfg.RetryMax,
					LockTimeout:  cfg.LockTimeout,
					IdleSleep:    cfg.IdleSleep,
					ReapInterval: cfg.ReapInterval,
				},
			}
			if err := proc.Validate(); err != nil {
				return err
			}

			// Graceful shutdown: capture signal, let the processor drain,
			// then persist the last-seen page and reap stale locks.
			workerCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()

			log.Printf("worker started run_id=%s worker_id=%s max_in_flight=%d", runID, workerID, cfg.MaxInFlight)
			runErr := proc.Run(workerCtx, runID)

			// ── Cleanup on exit (signal or natural completion) ──────────────
			log.Printf("worker stopping run_id=%s worker_id=%s — running shutdown cleanup", runID, workerID)
			cleanupCtx, cancel := context.WithTimeout(context.Background(), cfg.LockTimeout)
			defer cancel()

			// 1. Record the last page that was touched during this session so
			//    resume can continue from there.
			if lastPage, perr := repo.LastTouchedPage(cleanupCtx, runID); perr == nil && lastPage > 0 {
				if perr2 := repo.SetLastExitPage(cleanupCtx, runID, lastPage); perr2 != nil {
					log.Printf("warn: set last_exit_page: %v", perr2)
				} else {
					log.Printf("persisted last_exit_page=%d for run_id=%s", lastPage, runID)
				}
			}

			// 2. Reap any pages that were left in-progress by this process so
			//    they can be retried immediately on next start.
			reaped, rerr := repo.ReapStaleInProgress(cleanupCtx, runID, 0)
			if rerr != nil {
				log.Printf("warn: reap stale pages on exit: %v", rerr)
			} else if reaped > 0 {
				log.Printf("requeued %d in-progress page(s) back to pending", reaped)
			}

			log.Printf("worker exited run_id=%s worker_id=%s", runID, workerID)
			return runErr
		},
	}
	cmd.Flags().StringVar(&runID, "run-id", "", "Run ID (uuid)")
	return cmd
}

// newResumeCmd resets a previously interrupted run back to a clean state so
// workers can pick up exactly where they left off.  It:
//  1. Resets all in-progress pages → pending (handles crash leftovers).
//  2. Drops contact_keys rows for pages >= last_exit_page (partial writes).
//  3. Resets pages >= last_exit_page → pending so they are re-fetched.
func newResumeCmd(build func() (config.Config, api.Auth, error)) *cobra.Command {
	var runID string
	var fromPage int
	cmd := &cobra.Command{
		Use:   "resume",
		Short: "Resume an interrupted run from its last-exit page (resets partial progress)",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, auth, err := build()
			if err != nil {
				return err
			}
			if cfg.DBDSN == "" {
				return fmt.Errorf("DB_DSN is required")
			}
			if runID == "" {
				return fmt.Errorf("--run-id is required")
			}
			if auth.BearerToken == "" || auth.CsrfToken == "" || auth.Cookie == "" {
				return fmt.Errorf("--bearer-token, --csrf-token, and --cookie are required for resume")
			}

			pool, err := db.Open(cmd.Context(), cfg.DBDSN)
			if err != nil {
				return err
			}
			defer pool.Close()

			if err := db.ApplyMigrations(cmd.Context(), pool); err != nil {
				return err
			}

			repo := db.NewRepo(pool)

			// Determine resume page: explicit flag beats persisted value.
			resumePage := fromPage
			if resumePage <= 0 {
				run, rerr := repo.GetRun(cmd.Context(), runID)
				if rerr != nil {
					return rerr
				}
				if run.LastExitPage != nil {
					resumePage = *run.LastExitPage
				}
			}
			if resumePage <= 0 {
				return fmt.Errorf("no last_exit_page found for run %s and --from-page not set", runID)
			}

			dropped, reset, rerr := repo.ResumeFromPage(cmd.Context(), runID, resumePage)
			if rerr != nil {
				return rerr
			}

			// Re-open the run so workers can claim pages.
			if rerr2 := repo.ReopenRun(cmd.Context(), runID); rerr2 != nil {
				return rerr2
			}

			fmt.Fprintf(cmd.OutOrStdout(),
				"run_id=%s resume_page=%d contact_keys_dropped=%d pages_reset=%d state=running\n",
				runID, resumePage, dropped, reset)

			// Kick off workers immediately in the same process.
			workerID := fmt.Sprintf("%d-%d", os.Getpid(), rand.Int())
			apiClient := api.NewClient(cfg.APIBaseURL)
			proc := &runner.Processor{
				DB:       pool,
				Repo:     repo,
				API:      apiClient,
				Auth:     auth,
				WorkerID: workerID,
				Stdin:    cmd.InOrStdin(),
				Stdout:   cmd.OutOrStdout(),
				Options: runner.Options{
					MaxInFlight:  cfg.MaxInFlight,
					MaxAttempts:  cfg.MaxAttempts,
					RetryInitial: cfg.RetryInitial,
					RetryMax:     cfg.RetryMax,
					LockTimeout:  cfg.LockTimeout,
					IdleSleep:    cfg.IdleSleep,
					ReapInterval: cfg.ReapInterval,
				},
			}
			if err := proc.Validate(); err != nil {
				return err
			}

			workerCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()

			log.Printf("resume worker started run_id=%s worker_id=%s from_page=%d", runID, workerID, resumePage)
			runErr := proc.Run(workerCtx, runID)

			// Same shutdown cleanup as the worker command.
			cleanupCtx, cancel := context.WithTimeout(context.Background(), cfg.LockTimeout)
			defer cancel()

			if lastPage, perr := repo.LastTouchedPage(cleanupCtx, runID); perr == nil && lastPage > 0 {
				if perr2 := repo.SetLastExitPage(cleanupCtx, runID, lastPage); perr2 != nil {
					log.Printf("warn: set last_exit_page: %v", perr2)
				} else {
					log.Printf("persisted last_exit_page=%d for run_id=%s", lastPage, runID)
				}
			}
			reaped, rerr2 := repo.ReapStaleInProgress(cleanupCtx, runID, 0)
			if rerr2 != nil {
				log.Printf("warn: reap on exit: %v", rerr2)
			} else if reaped > 0 {
				log.Printf("requeued %d in-progress page(s) back to pending", reaped)
			}

			log.Printf("resume worker exited run_id=%s worker_id=%s", runID, workerID)
			return runErr
		},
	}
	cmd.Flags().StringVar(&runID, "run-id", "", "Run ID (uuid) — required")
	cmd.Flags().IntVar(&fromPage, "from-page", 0, "Override resume page (default: last_exit_page stored in DB)")
	return cmd
}

func createRunAndSeedAllPages(ctx context.Context, repo *db.Repo, client *api.Client, auth api.Auth, cfg config.Config, startedPage int) (string, int, int, int64, error) {
	countResp, _, err := client.FetchCount(ctx, auth, api.FetchCountParams{
		PageSize: cfg.PageSize,
		Page:     1,
		OrderBy:  "contactKey ASC",
	})
	if err != nil {
		return "", 0, 0, 0, fmt.Errorf("fetch total count: %w", err)
	}
	totalCount := countResp.TotalCount
	if totalCount < 0 {
		totalCount = 0
	}
	totalPages := totalPagesFor(totalCount, cfg.PageSize)
	runID, err := repo.CreateRun(ctx, cfg.PageSize, startedPage, cfg.APIBaseURL, "Is", "MOBILE", totalCount)
	if err != nil {
		return "", 0, 0, 0, err
	}
	seededRows, err := repo.SeedPendingPages(ctx, runID, startedPage, totalPages, 10000)
	if err != nil {
		return "", 0, 0, 0, err
	}
	return runID, totalCount, totalPages, seededRows, nil
}

func totalPagesFor(totalCount int, pageSize int) int {
	if pageSize <= 0 || totalCount <= 0 {
		return 0
	}
	return (totalCount + pageSize - 1) / pageSize
}

func validateAuthPreflight(ctx context.Context, client *api.Client, auth api.Auth, cfg config.Config) error {
	resp, err := client.PingAuth(ctx, auth, api.PingAuthParams{
		PageSize:                cfg.PageSize,
		FilterConditionOperator: "Is",
		FilterConditionValue:    "MOBILE",
		OrderBy:                 "contactKey ASC",
	})
	if err == nil {
		return nil
	}
	if api.IsAuthError(resp) {
		return fmt.Errorf("auth preflight failed with http %d (invalid bearer/csrf/cookie)", resp.StatusCode)
	}
	return fmt.Errorf("auth preflight failed: %w", err)
}

func newStatusCmd(build func() (config.Config, error)) *cobra.Command {
	var runID string
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show run status and page progress from Postgres",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := build()
			if err != nil {
				return err
			}
			if cfg.DBDSN == "" {
				return fmt.Errorf("DB_DSN is required")
			}
			if runID == "" {
				return fmt.Errorf("--run-id is required")
			}

			pool, err := db.Open(cmd.Context(), cfg.DBDSN)
			if err != nil {
				return err
			}
			defer pool.Close()

			if err := db.ApplyMigrations(cmd.Context(), pool); err != nil {
				return err
			}

			repo := db.NewRepo(pool)
			run, err := repo.GetRun(cmd.Context(), runID)
			if err != nil {
				return err
			}
			done, failed, empty, inProgress, pending, err := repo.RunProgress(cmd.Context(), runID)
			if err != nil {
				return err
			}
			stopPage := "null"
			if run.StopPage != nil {
				stopPage = fmt.Sprintf("%d", *run.StopPage)
			}
			lastExitPage := "null"
			if run.LastExitPage != nil {
				lastExitPage = fmt.Sprintf("%d", *run.LastExitPage)
			}

			fmt.Fprintf(cmd.OutOrStdout(),
				"run_id=%s state=%s stop_page=%s last_exit_page=%s page_size=%d base_url=%s filter=%s:%s\n",
				run.ID, run.State, stopPage, lastExitPage, run.PageSize, run.BaseURL, run.FilterOperator, run.FilterValue)
			fmt.Fprintf(cmd.OutOrStdout(),
				"pages_done=%d pages_failed=%d pages_empty=%d pages_in_progress=%d pages_pending=%d\n",
				done, failed, empty, inProgress, pending)
			return nil
		},
	}
	cmd.Flags().StringVar(&runID, "run-id", "", "Run ID (uuid)")
	return cmd
}
