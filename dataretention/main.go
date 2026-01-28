package main

import (
	"context"
	"fmt"
	"os"

	"github.com/natserract/sf/dataretention/schema/postgres"
	"github.com/natserract/sf/dataretention/services"
	sfmce "github.com/natserract/sf/pkg/salesforce/mce"
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
	cfg, err := sfmce.LoadConfig()
	if err != nil {
		logger.Error("Failed to load config", zap.Error(err))
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Initialize database connection (optional - graceful degradation if DB not available)
	var db *postgres.DB
	dbCfg := postgres.NewConfig()
	db, err = postgres.New(dbCfg, logger)
	if err != nil {
		logger.Warn("Failed to connect to database, continuing without database", zap.Error(err))
		fmt.Fprintf(os.Stderr, "Warning: Failed to connect to database: %v\n", err)
		fmt.Fprintf(os.Stderr, "Continuing without database connection...\n")
		fmt.Fprintf(os.Stderr, "Note: Database persistence will not be available\n")
		os.Exit(1) // Exit if DB is not available since we need it for this operation
	}
	defer db.Close()
	logger.Info("Database connection established")
	fmt.Println("Database connection established")

	// Create Salesforce client
	client := sfmce.NewSalesforceWithLogger(cfg, logger)

	// Create folder service
	folderSvc := services.NewFolderService(db, logger)

	// Create data extension service
	dataExtSvc := services.NewDataExtensionService(db, logger)

	// Create sync service
	syncSvc := services.NewSyncService(client, dataExtSvc, folderSvc, db, logger)

	// Fetch and process folders, subfolders, and data extensions
	ctx := context.Background()
	metrics, err := syncSvc.SyncAll(ctx)
	if err != nil {
		logger.Error("Failed to sync data", zap.Error(err))
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Log and print final metrics
	logger.Info("Successfully completed fetching and storing folders, subfolders, and data extensions",
		zap.Int("folders_succeeded", metrics.FoldersSucceeded),
		zap.Int("folders_failed", metrics.FoldersFailed),
		zap.Int("subfolders_succeeded", metrics.SubfoldersSucceeded),
		zap.Int("subfolders_failed", metrics.SubfoldersFailed),
		zap.Int("data_extensions_succeeded", metrics.DataExtensionsSucceeded),
		zap.Int("data_extensions_failed", metrics.DataExtensionsFailed),
		zap.Int("total_succeeded", metrics.TotalSucceeded()),
		zap.Int("total_failed", metrics.TotalFailed()))

	fmt.Println("Successfully completed fetching and storing folders, subfolders, and data extensions")
	fmt.Printf("Sync Metrics:\n")
	fmt.Printf("  Folders: %d succeeded, %d failed\n", metrics.FoldersSucceeded, metrics.FoldersFailed)
	fmt.Printf("  Subfolders: %d succeeded, %d failed\n", metrics.SubfoldersSucceeded, metrics.SubfoldersFailed)
	fmt.Printf("  Data Extensions: %d succeeded, %d failed\n", metrics.DataExtensionsSucceeded, metrics.DataExtensionsFailed)
	fmt.Printf("  Total: %d succeeded, %d failed\n", metrics.TotalSucceeded(), metrics.TotalFailed())
}
