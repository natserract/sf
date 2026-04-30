package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/joho/godotenv"
	"github.com/spf13/cobra"

	"sfmc-retention/internal/client"
	"sfmc-retention/internal/exporter"
	"sfmc-retention/internal/models"
	"sfmc-retention/internal/ping"
	"sfmc-retention/internal/store"
	"sfmc-retention/internal/worker"
)

var (
	// JB (Journey Builder) flags
	jbHost      string
	jbCookie    string
	jbCSRFToken string

	// MC flags
	mcHost      string
	mcCookie    string
	mcCSRFToken string
	bearerToken string

	// Behaviour flags
	dryRun    bool
	verbose   bool
	debug     bool
	outputDir string
	runID     string

	businessUnitID string
)

func main() {
	// Load local .env if present; real environment vars still take precedence.
	_ = godotenv.Load()

	root := &cobra.Command{
		Use:   "sfmc-retention",
		Short: "Set 7-day data retention on all SFMC journey data extensions",
		Long:  "Run SFMC retention pipeline as isolated steps with CSV and Postgres persistence.",
	}

	runCmd := &cobra.Command{
		Use:   "run",
		Short: "Run step1 through step5 sequentially",
		RunE: func(cmd *cobra.Command, args []string) error {
			if _, _, _, err := runStep1(); err != nil {
				return err
			}
			if _, _, _, err := runStep2(); err != nil {
				return err
			}
			if _, _, _, err := runStep3(); err != nil {
				return err
			}
			if _, _, _, err := runStep4(); err != nil {
				return err
			}
			_, _, _, err := runStep5()
			return err
		},
	}
	step1Cmd := &cobra.Command{Use: "step-fetch-journeys", Short: "Fetch journeys and export refs", RunE: func(cmd *cobra.Command, args []string) error {
		_, _, _, err := runStep1()
		return err
	}}
	step2Cmd := &cobra.Command{Use: "step-resolve-event-defs", Short: "Resolve event defs from journeys DB data", RunE: func(cmd *cobra.Command, args []string) error {
		_, _, _, err := runStep2()
		return err
	}}
	step3Cmd := &cobra.Command{Use: "step-fetch-data-extensions", Short: "Fetch DE details from event defs DB data", RunE: func(cmd *cobra.Command, args []string) error {
		_, _, _, err := runStep3()
		return err
	}}
	step4Cmd := &cobra.Command{Use: "step-update-retention", Short: "Update retention from data extensions DB data", RunE: func(cmd *cobra.Command, args []string) error {
		_, _, _, err := runStep4()
		return err
	}}
	step5Cmd := &cobra.Command{Use: "step-refetch-updated", Short: "Refetch updated DEs from update results DB data", RunE: func(cmd *cobra.Command, args []string) error {
		_, _, _, err := runStep5()
		return err
	}}
	migrateCmd := &cobra.Command{Use: "migrate", Short: "Create/update PostgreSQL schema", RunE: func(cmd *cobra.Command, args []string) error {
		db, err := store.Open(store.ConfigFromEnv())
		if err != nil {
			return err
		}
		defer db.Close()
		if err := store.EnsureSchema(db); err != nil {
			return err
		}
		log.Println("database migration completed")
		return nil
	}}
	pingCmd := &cobra.Command{Use: "ping", Short: "Verify credentials only", RunE: func(cmd *cobra.Command, args []string) error {
		_, err := ping.VerifyOrPrompt(buildCreds(), debug)
		return err
	}}

	for _, cmd := range []*cobra.Command{runCmd, step1Cmd, step2Cmd, step3Cmd, step4Cmd, step5Cmd, pingCmd} {
		cmd.Flags().StringVar(&jbHost, "jb-host", envOr("SFMC_JB_HOST", "jbinteractions.s12.marketingcloudapps.com"), "Journey Builder host")
		cmd.Flags().StringVar(&jbCookie, "jb-cookie", envOr("SFMC_JB_COOKIE", ""), "Journey Builder session cookie")
		cmd.Flags().StringVar(&jbCSRFToken, "jb-csrf", envOr("SFMC_JB_CSRF", ""), "Journey Builder X-CSRF-Token")
		cmd.Flags().StringVar(&mcHost, "mc-host", envOr("SFMC_MC_HOST", "mc.s12.marketingcloudapps.com"), "Marketing Cloud host")
		cmd.Flags().StringVar(&mcCookie, "mc-cookie", envOr("SFMC_MC_COOKIE", ""), "MC session cookie")
		cmd.Flags().StringVar(&mcCSRFToken, "mc-csrf", envOr("SFMC_MC_CSRF", ""), "MC X-CSRF-Token")
		cmd.Flags().StringVar(&bearerToken, "bearer", envOr("SFMC_BEARER", ""), "Bearer token")
		cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Skip updates")
		cmd.Flags().BoolVar(&verbose, "verbose", false, "Verbose output")
		cmd.Flags().BoolVar(&debug, "debug", false, "Debug HTTP logs")
		cmd.Flags().StringVar(&outputDir, "output-dir", "./output", "CSV output directory")
		cmd.Flags().StringVar(&runID, "run-id", "", "Run identifier to read/write DB rows")
		cmd.Flags().StringVar(&businessUnitID, "business-unit-id", envOr("SFMC_BUSINESS_UNIT_ID", ""), "Business unit identifier")
	}

	root.AddCommand(runCmd, step1Cmd, step2Cmd, step3Cmd, step4Cmd, step5Cmd, migrateCmd, pingCmd)

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

// ─── Step runners ─────────────────────────────────────────────────────────────

func runStep1() (string, string, []models.JourneyEventRef, error) {
	runID, buID, pipe, repo, err := initRuntime(true, true)
	if err != nil {
		return "", "", nil, err
	}
	journeys, err := pipe.FetchJourneys()
	if err != nil {
		return "", "", nil, err
	}
	refs := worker.ExtractJourneyEventRefs(journeys, buID)
	path := stepPath("step1_journey_refs", runID)
	if err := exporter.ExportJourneyRefs(refs, path); err != nil {
		return "", "", nil, err
	}
	if err := repo.SaveStep1(runID, refs); err != nil {
		return "", "", nil, err
	}
	return runID, path, refs, nil
}

func runStep2() (string, string, []models.EventDefinitionRef, error) {
	runID, buID, pipe, repo, err := initRuntime(true, false)
	if err != nil {
		return "", "", nil, err
	}
	runID, refs, err := repo.LoadStep1(runID, buID)
	if err != nil {
		return "", "", nil, err
	}
	out := pipe.ResolveEventDefinitions(refs)
	path := stepPath("step2_event_defs", runID)
	if err := exporter.ExportEventDefRefs(out, path); err != nil {
		return "", "", nil, err
	}
	if err := repo.SaveStep2(runID, out); err != nil {
		return "", "", nil, err
	}
	return runID, path, out, nil
}

func runStep3() (string, string, []models.DataExtension, error) {
	runID, buID, pipe, repo, err := initRuntime(true, false)
	if err != nil {
		return "", "", nil, err
	}
	runID, refs, err := repo.LoadStep2(runID, buID)
	if err != nil {
		return "", "", nil, err
	}
	out := pipe.FetchDataExtensions(refs)
	path := stepPath("step3_data_extensions", runID)
	if err := exporter.ExportBefore(out, path); err != nil {
		return "", "", nil, err
	}
	if err := repo.SaveStep3(runID, out); err != nil {
		return "", "", nil, err
	}
	return runID, path, out, nil
}

func runStep4() (string, string, []models.ProcessResult, error) {
	runID, buID, pipe, repo, err := initRuntime(true, false)
	if err != nil {
		return "", "", nil, err
	}
	runID, des, err := repo.LoadStep3(runID, buID)
	if err != nil {
		return "", "", nil, err
	}
	results := pipe.Update(des)
	path := stepPath("step4_update_results", runID)
	if err := exporter.ExportResults(results, path); err != nil {
		return "", "", nil, err
	}
	if err := repo.SaveStep4(runID, results); err != nil {
		return "", "", nil, err
	}
	return runID, path, results, nil
}

func runStep5() (string, string, []models.DataExtension, error) {
	runID, buID, pipe, repo, err := initRuntime(true, false)
	if err != nil {
		return "", "", nil, err
	}
	runID, results, err := repo.LoadStep4(runID, buID)
	if err != nil {
		return "", "", nil, err
	}
	updated := pipe.FetchUpdated(results)
	path := stepPath("step5_after", runID)
	if err := exporter.ExportAfter(updated, path); err != nil {
		return "", "", nil, err
	}
	if err := repo.SaveStep5(runID, updated); err != nil {
		return "", "", nil, err
	}
	return runID, path, updated, nil
}

func initRuntime(verifyAuth bool, ensureRunID bool) (string, string, *worker.Pipeline, *store.Repository, error) {
	log.SetFlags(log.Ltime)
	if businessUnitID == "" {
		return "", "", nil, nil, fmt.Errorf("business unit is required: set --business-unit-id or SFMC_BUSINESS_UNIT_ID")
	}
	if ensureRunID && runID == "" {
		runID = time.Now().Format("20060102_150405")
	}
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return "", "", nil, nil, fmt.Errorf("create output dir: %w", err)
	}

	creds := buildCreds()
	if verifyAuth {
		verifiedCreds, err := ping.VerifyOrPrompt(creds, debug)
		if err != nil {
			return "", "", nil, nil, err
		}
		creds = verifiedCreds
	}

	db, err := store.Open(store.ConfigFromEnv())
	if err != nil {
		return "", "", nil, nil, err
	}
	if err := store.EnsureSchema(db); err != nil {
		return "", "", nil, nil, err
	}
	repo := store.NewRepository(db)
	return runID, businessUnitID, worker.New(client.New(creds, debug), dryRun, verbose), repo, nil
}

func stepPath(prefix, runID string) string {
	return fmt.Sprintf("%s/%s_%s.csv", outputDir, prefix, runID)
}

func buildCreds() client.Credentials {
	return client.Credentials{
		JBHost:      jbHost,
		MCHost:      mcHost,
		JBCookie:    jbCookie,
		MCCookie:    mcCookie,
		JBCSRFToken: jbCSRFToken,
		MCCSRFToken: mcCSRFToken,
		BearerToken: bearerToken,
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
