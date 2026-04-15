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
	root.AddCommand(newFetchOnceCmd(func() (api.Auth, api.FetchPageParams, *api.Client, error) {
		cfg, err := config.FromEnv()
		if err != nil {
			return api.Auth{}, api.FetchPageParams{}, nil, err
		}
		client := api.NewClient(cfg.APIBaseURL)
		auth := api.Auth{BearerToken: bearerToken, CsrfToken: csrfToken, Cookie: cookie}
		params := api.FetchPageParams{
			PageSize:                cfg.PageSize,
			OrderBy:                 "contactKey ASC",
			FilterConditionOperator: "Is",
			FilterConditionValue:    "MOBILE",
		}
		return auth, params, client, nil
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

func newFetchOnceCmd(build func() (api.Auth, api.FetchPageParams, *api.Client, error)) *cobra.Command {
	var page int
	cmd := &cobra.Command{
		Use:   "fetch-once",
		Short: "Fetch a single page and print extracted keys",
		RunE: func(cmd *cobra.Command, args []string) error {
			auth, params, client, err := build()
			if err != nil {
				return err
			}
			params.Page = page
			resp, _, err := client.FetchPage(cmd.Context(), auth, params)
			if err != nil {
				return err
			}
			contacts, empty := api.ExtractContactInfo(resp)
			fmt.Fprintf(cmd.OutOrStdout(), "page=%d empty=%v keys=%d\n", page, empty, len(contacts))
			for _, c := range contacts {
				fmt.Fprintln(cmd.OutOrStdout(), c)
			}
			return nil
		},
	}
	cmd.Flags().IntVar(&page, "page", 1, "Page number to fetch")
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

			workerCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			log.Printf("worker started run_id=%s worker_id=%s max_in_flight=%d", runID, workerID, cfg.MaxInFlight)
			if err := proc.Run(workerCtx, runID); err != nil {
				return err
			}
			log.Printf("worker exited run_id=%s worker_id=%s", runID, workerID)
			return nil
		},
	}
	cmd.Flags().StringVar(&runID, "run-id", "", "Run ID (uuid)")
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

			fmt.Fprintf(cmd.OutOrStdout(), "run_id=%s state=%s stop_page=%s page_size=%d base_url=%s filter=%s:%s\n", run.ID, run.State, stopPage, run.PageSize, run.BaseURL, run.FilterOperator, run.FilterValue)
			fmt.Fprintf(cmd.OutOrStdout(), "pages_done=%d pages_failed=%d pages_empty=%d pages_in_progress=%d pages_pending=%d\n", done, failed, empty, inProgress, pending)
			return nil
		},
	}
	cmd.Flags().StringVar(&runID, "run-id", "", "Run ID (uuid)")
	return cmd
}
