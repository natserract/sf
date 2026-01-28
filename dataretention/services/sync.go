package services

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/natserract/sf/dataretention/schema/postgres"
	"github.com/natserract/sf/dataretention/schema/postgres/gen"
	salesforce "github.com/natserract/sf/pkg/salesforce/mce"
	"github.com/sourcegraph/conc/pool"
	"go.uber.org/zap"
)

// SyncMetrics tracks the overall sync operation metrics
type SyncMetrics struct {
	FoldersSucceeded        int
	FoldersFailed           int
	SubfoldersSucceeded     int
	SubfoldersFailed        int
	DataExtensionsSucceeded int
	DataExtensionsFailed    int
	mu                      sync.Mutex
}

// AddFolderSuccess increments the folders succeeded count
func (m *SyncMetrics) AddFolderSuccess() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.FoldersSucceeded++
}

// AddFolderFailure increments the folders failed count
func (m *SyncMetrics) AddFolderFailure() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.FoldersFailed++
}

// AddSubfolderSuccess increments the subfolders succeeded count
func (m *SyncMetrics) AddSubfolderSuccess() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.SubfoldersSucceeded++
}

// AddSubfolderFailure increments the subfolders failed count
func (m *SyncMetrics) AddSubfolderFailure() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.SubfoldersFailed++
}

// AddDataExtensionSuccess increments the data extensions succeeded count
func (m *SyncMetrics) AddDataExtensionSuccess() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.DataExtensionsSucceeded++
}

// AddDataExtensionFailure increments the data extensions failed count
func (m *SyncMetrics) AddDataExtensionFailure() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.DataExtensionsFailed++
}

// AddDataExtensions adds multiple data extension results
func (m *SyncMetrics) AddDataExtensions(succeeded, failed int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.DataExtensionsSucceeded += succeeded
	m.DataExtensionsFailed += failed
}

// TotalSucceeded returns the total number of succeeded operations
func (m *SyncMetrics) TotalSucceeded() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.FoldersSucceeded + m.SubfoldersSucceeded + m.DataExtensionsSucceeded
}

// TotalFailed returns the total number of failed operations
func (m *SyncMetrics) TotalFailed() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.FoldersFailed + m.SubfoldersFailed + m.DataExtensionsFailed
}

// SyncService handles direct synchronization of folders and data extensions
// with durable tracking via sync jobs
type SyncService struct {
	client     salesforce.SalesforceClient
	dataExtSvc *DataExtensionService
	folderSvc  *FolderService
	queries    *gen.Queries
	db         *postgres.DB
	logger     *zap.Logger
}

// NewSyncService creates a new sync service
func NewSyncService(client salesforce.SalesforceClient, dataExtSvc *DataExtensionService, folderSvc *FolderService, db *postgres.DB, logger *zap.Logger) *SyncService {
	return &SyncService{
		client:     client,
		dataExtSvc: dataExtSvc,
		folderSvc:  folderSvc,
		queries:    gen.New(),
		db:         db,
		logger:     logger,
	}
}

// SyncAll performs a full sync of all folders, subfolders, and data extensions
// Returns the sync metrics and any error that occurred
func (s *SyncService) SyncAll(ctx context.Context) (*SyncMetrics, error) {
	startTime := time.Now()
	s.logger.Info("Starting full sync operation")

	// Initialize metrics accumulator
	metrics := &SyncMetrics{}

	// Sync folders
	if err := s.SyncFolders(ctx, metrics); err != nil {
		return metrics, fmt.Errorf("failed to sync folders: %w", err)
	}

	duration := time.Since(startTime)

	// Log final metrics
	s.logger.Info("Completed full sync operation",
		zap.Duration("duration", duration),
		zap.Int("folders_succeeded", metrics.FoldersSucceeded),
		zap.Int("folders_failed", metrics.FoldersFailed),
		zap.Int("subfolders_succeeded", metrics.SubfoldersSucceeded),
		zap.Int("subfolders_failed", metrics.SubfoldersFailed),
		zap.Int("data_extensions_succeeded", metrics.DataExtensionsSucceeded),
		zap.Int("data_extensions_failed", metrics.DataExtensionsFailed),
		zap.Int("total_succeeded", metrics.TotalSucceeded()),
		zap.Int("total_failed", metrics.TotalFailed()))

	return metrics, nil
}

// SyncFolders syncs all folders with proper hierarchy handling
func (s *SyncService) SyncFolders(ctx context.Context, metrics *SyncMetrics) error {
	// Fetch all folders
	s.logger.Info("Fetching folders...")
	foldersResp, err := s.client.GetFolders()
	if err != nil {
		return fmt.Errorf("failed to fetch folders: %w", err)
	}

	s.logger.Info("Fetched folders",
		zap.Int("total_folders", foldersResp.TotalResults),
		zap.Int("items_count", len(foldersResp.Entry)))

	// Separate top-level folders (parentId is "0" or empty) from subfolders
	var topLevelFolders []salesforce.Folder
	var subfolders []salesforce.Folder
	folderMap := make(map[string]salesforce.Folder) // Map to track all folders by ID

	for _, folder := range foldersResp.Entry {
		folderMap[folder.ID] = folder
		if folder.ParentID == "" || folder.ParentID == "0" {
			topLevelFolders = append(topLevelFolders, folder)
		} else {
			subfolders = append(subfolders, folder)
		}
	}

	s.logger.Info("Separated folders",
		zap.Int("top_level_count", len(topLevelFolders)),
		zap.Int("subfolder_count", len(subfolders)))

	// Step 1: Save all top-level folders first (concurrently)
	s.logger.Info("Saving top-level folders...")
	topLevelPool := pool.New().WithMaxGoroutines(10).WithErrors()
	for _, folder := range topLevelFolders {
		folder := folder // capture loop variable
		topLevelPool.Go(func() error {
			if err := s.folderSvc.SaveFolder(ctx, folder); err != nil {
				metrics.AddFolderFailure()
				s.logger.Error("Failed to save top-level folder",
					zap.String("folder_id", folder.ID),
					zap.String("folder_name", folder.Name),
					zap.Error(err))
				return fmt.Errorf("failed to save top-level folder %s: %w", folder.ID, err)
			}
			metrics.AddFolderSuccess()
			s.logger.Info("Saved top-level folder",
				zap.String("folder_id", folder.ID),
				zap.String("folder_name", folder.Name))
			return nil
		})
	}

	if err := topLevelPool.Wait(); err != nil {
		return fmt.Errorf("error saving top-level folders: %w", err)
	}

	// Step 2: Save subfolders that were in the initial list (in dependency order)
	if len(subfolders) > 0 {
		s.logger.Info("Saving subfolders from initial list...")
		if err := s.folderSvc.SaveFoldersInOrder(ctx, subfolders, folderMap); err != nil {
			s.logger.Warn("Failed to save some subfolders from initial list", zap.Error(err))
			// Continue processing even if some subfolders fail
		}
	}

	// Step 3: Process all folders (top-level and subfolders) to fetch their subfolders and data extensions
	s.logger.Info("Processing folders to fetch subfolders and data extensions...")
	folderPool := pool.New().WithMaxGoroutines(10).WithErrors()

	// Process all folders
	for _, folder := range foldersResp.Entry {
		folder := folder // capture loop variable
		folderPool.Go(func() error {
			return s.SyncFolder(ctx, folder, true, metrics)
		})
	}

	// Wait for all folder processing to complete
	if err := folderPool.Wait(); err != nil {
		return fmt.Errorf("error processing folders: %w", err)
	}

	return nil
}

// SyncFolder syncs a single folder: saves it, fetches subfolders recursively, and data extensions
func (s *SyncService) SyncFolder(ctx context.Context, folder salesforce.Folder, recursive bool, metrics *SyncMetrics) error {
	// Save the folder
	if err := s.folderSvc.SaveFolder(ctx, folder); err != nil {
		metrics.AddFolderFailure()
		s.logger.Error("Failed to save folder",
			zap.String("folder_id", folder.ID),
			zap.String("folder_name", folder.Name),
			zap.Error(err))
		return fmt.Errorf("failed to save folder %s: %w", folder.ID, err)
	}
	metrics.AddFolderSuccess()
	s.logger.Info("Saved folder",
		zap.String("folder_id", folder.ID),
		zap.String("folder_name", folder.Name))

	// Fetch subfolders
	subfoldersResp, err := s.client.GetSubFolders(folder.ID)
	if err != nil {
		s.logger.Warn("Failed to fetch subfolders",
			zap.String("folder_id", folder.ID),
			zap.Error(err))
		// Continue processing even if subfolders fail
	} else {
		s.logger.Info("Fetched subfolders",
			zap.String("folder_id", folder.ID),
			zap.Int("subfolder_count", len(subfoldersResp.Entry)))

		// Create a worker pool for processing subfolders (max 5 concurrent per folder)
		subfolderPool := pool.New().WithMaxGoroutines(5).WithErrors()

		// Process each subfolder concurrently
		for _, subfolder := range subfoldersResp.Entry {
			subfolder := subfolder // capture loop variable
			subfolderPool.Go(func() error {
				// Save the subfolder
				if err := s.folderSvc.SaveFolder(ctx, subfolder); err != nil {
					metrics.AddSubfolderFailure()
					s.logger.Error("Failed to save subfolder",
						zap.String("subfolder_id", subfolder.ID),
						zap.String("subfolder_name", subfolder.Name),
						zap.Error(err))
					return fmt.Errorf("failed to save subfolder %s: %w", subfolder.ID, err)
				}
				metrics.AddSubfolderSuccess()

				// Recursively sync subfolder if recursive is true
				if recursive {
					if err := s.SyncFolder(ctx, subfolder, true, metrics); err != nil {
						s.logger.Warn("Failed to recursively sync subfolder",
							zap.String("subfolder_id", subfolder.ID),
							zap.Error(err))
						// Continue processing data extensions even if recursive sync fails
					}
				} else {
					// Just sync data extensions for this subfolder
					if err := s.SyncDataExtensions(ctx, subfolder.ID, subfolder.Name, metrics); err != nil {
						s.logger.Warn("Failed to sync data extensions for subfolder",
							zap.String("subfolder_id", subfolder.ID),
							zap.Error(err))
					}
				}
				return nil
			})
		}

		// Wait for all subfolder processing to complete
		if err := subfolderPool.Wait(); err != nil {
			s.logger.Warn("Error processing subfolders",
				zap.String("folder_id", folder.ID),
				zap.Error(err))
			// Continue processing folder's data extensions even if subfolders fail
		}
	}

	// Fetch and save data extensions for the folder itself (last 3 months)
	if err := s.SyncDataExtensions(ctx, folder.ID, folder.Name, metrics); err != nil {
		s.logger.Warn("Failed to fetch data extensions for folder",
			zap.String("folder_id", folder.ID),
			zap.Error(err))
		// Don't return error, just log it
	}

	return nil
}

// SyncDataExtensions fetches all data extensions for a folder (with pagination) and saves them
// Only fetches data extensions modified in the last 3 months
// After saving, updates data retention properties via API
// Creates and tracks a sync job for durability
func (s *SyncService) SyncDataExtensions(ctx context.Context, folderID string, folderName string, metrics *SyncMetrics) error {
	startTime := time.Now()
	totalSucceeded := 0
	totalFailed := 0
	retentionUpdateSucceeded := 0
	retentionUpdateFailed := 0

	s.logger.Info("Fetching data extensions with date filter",
		zap.String("folder_id", folderID),
		zap.String("folder_name", folderName))

	// Fetch all data extensions (handles pagination internally)
	dataExtensions, err := s.dataExtSvc.GetDataExtensions(ctx, s.client, folderID)
	if err != nil {
		return fmt.Errorf("failed to fetch data extensions for folder %s: %w", folderID, err)
	}

	s.logger.Info("Fetched all data extensions",
		zap.String("folder_id", folderID),
		zap.String("folder_name", folderName),
		zap.Int("total_items", len(dataExtensions)))

	// Create sync job for tracking retention updates
	var syncJobID uuid.UUID
	if len(dataExtensions) > 0 {
		metadata, _ := json.Marshal(map[string]interface{}{
			"folder_id":   folderID,
			"folder_name": folderName,
			"operation":   "data_retention_update",
		})
		job, err := s.queries.CreateSyncJob(ctx, s.db.Pool(), gen.CreateSyncJobParams{
			JobType:    "data_retention_update",
			Status:     "running",
			TotalItems: int32(len(dataExtensions)),
			Metadata:   metadata,
		})
		if err != nil {
			s.logger.Warn("Failed to create sync job for retention updates",
				zap.String("folder_id", folderID),
				zap.Error(err))
		} else {
			syncJobID = job.ID
			s.logger.Info("Created sync job for retention updates",
				zap.String("job_id", syncJobID.String()),
				zap.String("folder_id", folderID),
				zap.Int("total_items", len(dataExtensions)))
		}
	}

	// Save all data extensions and update retention using worker pool
	// Items are already filtered by GetDataExtensions to only include those modified in last 3 months
	dataExtPool := pool.New().WithMaxGoroutines(10).WithErrors()
	saveResults := make([]error, len(dataExtensions))
	retentionResults := make([]error, len(dataExtensions))

	for idx, de := range dataExtensions {
		de := de // capture loop variable
		i := idx // capture index
		dataExtPool.Go(func() error {
			// First, save the data extension
			err := s.dataExtSvc.SaveDataExtension(ctx, de)
			saveResults[i] = err
			if err != nil {
				s.logger.Error("Failed to save data extension",
					zap.String("data_extension_id", de.ID),
					zap.String("data_extension_name", de.Name),
					zap.String("folder_id", folderID),
					zap.Error(err))
				return err
			}

			// After successful save, update data retention via API
			retentionErr := s.dataExtSvc.UpdateDataRetentionViaAPI(ctx, s.client, de.ID)
			retentionResults[i] = retentionErr
			if retentionErr != nil {
				s.logger.Error("Failed to update data retention via API",
					zap.String("data_extension_id", de.ID),
					zap.String("data_extension_name", de.Name),
					zap.String("folder_id", folderID),
					zap.Error(retentionErr))
			} else {
				s.logger.Debug("Successfully updated data retention via API",
					zap.String("data_extension_id", de.ID),
					zap.String("data_extension_name", de.Name))
			}

			return retentionErr
		})
	}

	// Wait for all operations to complete
	_ = dataExtPool.Wait()

	// Count save successes and failures
	succeeded := 0
	failed := 0
	for _, err := range saveResults {
		if err != nil {
			failed++
		} else {
			succeeded++
		}
	}

	// Count retention update successes and failures
	for _, err := range retentionResults {
		if err != nil {
			retentionUpdateFailed++
		} else {
			retentionUpdateSucceeded++
		}
	}

	totalSucceeded += succeeded
	totalFailed += failed

	// Update global metrics
	metrics.AddDataExtensions(succeeded, failed)

	// Update sync job progress and completion
	if syncJobID != uuid.Nil {
		// Update job with retention update progress
		err := s.queries.UpdateSyncJobProgress(ctx, s.db.Pool(), gen.UpdateSyncJobProgressParams{
			ProcessedItems: int32(len(dataExtensions)),
			SucceededItems: int32(retentionUpdateSucceeded),
			FailedItems:    int32(retentionUpdateFailed),
			ID:             syncJobID,
		})
		if err != nil {
			s.logger.Warn("Failed to update sync job progress",
				zap.String("job_id", syncJobID.String()),
				zap.Error(err))
		}

		// Mark job as completed
		duration := time.Since(startTime).Milliseconds()
		avgProcessingTime := int32(duration)
		if len(dataExtensions) > 0 {
			avgProcessingTime = int32(duration / int64(len(dataExtensions)))
		}
		err = s.queries.CompleteSyncJob(ctx, s.db.Pool(), gen.CompleteSyncJobParams{
			Status:              "completed",
			DurationMs:          pgtype.Int4{Int32: int32(duration), Valid: true},
			AvgProcessingTimeMs: pgtype.Int4{Int32: avgProcessingTime, Valid: true},
			ID:                  syncJobID,
		})
		if err != nil {
			s.logger.Warn("Failed to complete sync job",
				zap.String("job_id", syncJobID.String()),
				zap.Error(err))
		} else {
			s.logger.Info("Completed sync job for retention updates",
				zap.String("job_id", syncJobID.String()),
				zap.Int64("duration_ms", duration))
		}
	}

	s.logger.Info("Completed fetching and updating data extensions for folder",
		zap.String("folder_id", folderID),
		zap.String("folder_name", folderName),
		zap.Int("total_succeeded", totalSucceeded),
		zap.Int("total_failed", totalFailed),
		zap.Int("retention_updates_succeeded", retentionUpdateSucceeded),
		zap.Int("retention_updates_failed", retentionUpdateFailed))

	return nil
}
