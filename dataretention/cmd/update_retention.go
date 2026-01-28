package main

import (
	"context"
	"fmt"
	"os"

	"github.com/natserract/sf/dataretention/schema/postgres"
	"github.com/natserract/sf/dataretention/services"
	"github.com/natserract/sf/pkg/config"
	salesforce "github.com/natserract/sf/pkg/salesforce/mce"
	"go.uber.org/zap"
)

func main() {
	// Get data extension ID from command line or use default
	dataExtensionID := "57ddcfc3-83f2-ea11-a2f5-48df370ed95c"
	if len(os.Args) > 1 {
		dataExtensionID = os.Args[1]
	}

	// Initialize logger
	logger, err := zap.NewProduction()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync()

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		logger.Error("Failed to load config", zap.Error(err))
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Initialize database connection
	dbCfg := postgres.NewConfig()
	db, err := postgres.New(dbCfg, logger)
	if err != nil {
		logger.Error("Failed to connect to database", zap.Error(err))
		fmt.Fprintf(os.Stderr, "Failed to connect to database: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()
	logger.Info("Database connection established")

	// Create Salesforce client
	client := salesforce.NewSalesforceWithLogger(cfg, logger)

	// Create data extension service
	dataExtSvc := services.NewDataExtensionService(db, logger)

	// Update data retention for the specified ID
	ctx := context.Background()
	fmt.Printf("Updating data retention for data extension: %s\n", dataExtensionID)

	err = dataExtSvc.UpdateDataRetentionViaAPI(ctx, client, dataExtensionID)
	if err != nil {
		logger.Error("Failed to update data retention",
			zap.String("data_extension_id", dataExtensionID),
			zap.Error(err))
		fmt.Fprintf(os.Stderr, "Error: Failed to update data retention: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Successfully updated data retention for data extension: %s\n", dataExtensionID)
	logger.Info("Successfully updated data retention",
		zap.String("data_extension_id", dataExtensionID))
}
