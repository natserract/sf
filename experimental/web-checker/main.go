package main

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
	"github.com/spf13/cobra"

	"sf/usecases/web-checker/fetch-all-contacts-go/cmd"
	"sf/usecases/web-checker/fetch-all-contacts-go/internal/config"
)

func main() {
	_ = godotenv.Load()

	root := &cobra.Command{
		Use:   "web-checker",
		Short: "Web checker UI",
	}

	var bearerToken string
	var csrfToken string
	var cookie string

	root.PersistentFlags().StringVar(&bearerToken, "bearer-token", "", "Bearer token (no 'Bearer ' prefix)")
	root.PersistentFlags().StringVar(&csrfToken, "csrf-token", "", "X-CSRF-Token header value")
	root.PersistentFlags().StringVar(&cookie, "cookie", "", "Cookie header value")

	root.AddCommand(cmd.NewWebCmd(func() (config.Config, error) {
		cfg, err := config.FromEnvFull()
		if err != nil {
			return config.Config{}, err
		}
		return cfg, nil
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
