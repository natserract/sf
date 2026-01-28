package main

import (
	"fmt"
	"os"

	sfmcn "github.com/natserract/sf/pkg/salesforce/mcn"
	"go.uber.org/zap"
)

func main() {
	// Initialize logger
	logger, err := zap.NewProduction()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync()

	// Load configuration
	cfg, err := sfmcn.LoadConfig()
	if err != nil {
		logger.Error("Failed to load config", zap.Error(err))
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Create Salesforce client
	client := sfmcn.NewSalesforceWithLogger(cfg, logger)

	// SQL
	joinRecords(client)
}
