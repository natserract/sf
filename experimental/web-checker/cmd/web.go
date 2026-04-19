package cmd

import (
	"fmt"
	"log"
	"net/http"

	"github.com/spf13/cobra"

	"sf/usecases/web-checker/fetch-all-contacts-go/internal/config"
	"sf/usecases/web-checker/fetch-all-contacts-go/internal/db"
	"sf/usecases/web-checker/fetch-all-contacts-go/internal/history"
	"sf/usecases/web-checker/fetch-all-contacts-go/internal/store"
	"sf/usecases/web-checker/fetch-all-contacts-go/internal/validator"
	"sf/usecases/web-checker/fetch-all-contacts-go/internal/web"
)

func NewWebCmd(build func() (config.Config, error)) *cobra.Command {
	var addr string
	var workers int
	cmd := &cobra.Command{
		Use:   "web",
		Short: "Run local web app for CSV email validation",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := build()
			if err != nil {
				return err
			}
			if cfg.DBDSN == "" {
				return fmt.Errorf("DB_DSN is required")
			}

			pool, err := db.Open(cmd.Context(), cfg.DBDSN)
			if err != nil {
				return err
			}
			defer pool.Close()
			if err := db.ApplyMigrations(cmd.Context(), pool); err != nil {
				return err
			}

			repo := store.NewRepo(pool)
			validation := validator.NewService()
			historyClient := history.NewClient(cfg.APIBaseURL)
			srv := web.NewServer(repo, validation, historyClient, workers)
			log.Printf("web server listening on %s", addr)
			err = srv.ListenAndServe(addr)
			if err != nil && err != http.ErrServerClosed {
				return err
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&addr, "addr", ":8080", "HTTP listen address")
	cmd.Flags().IntVar(&workers, "workers", 20, "Validation worker count")
	return cmd
}
