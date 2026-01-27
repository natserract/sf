package services

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/natserract/sforce/pkg/sforce"
	"github.com/natserract/sforce/schema/postgres"
	"github.com/natserract/sforce/schema/postgres/gen"
	"go.uber.org/zap"
)

// FolderService handles folder persistence operations
type FolderService struct {
	queries *gen.Queries
	db      *postgres.DB
	logger  *zap.Logger
}

// NewFolderService creates a new folder service
func NewFolderService(db *postgres.DB, logger *zap.Logger) *FolderService {
	return &FolderService{
		queries: gen.New(),
		db:      db,
		logger:  logger,
	}
}

// SaveFolder saves or updates a folder in the database
func (f *FolderService) SaveFolder(ctx context.Context, folder sforce.Folder) error {
	lastUpdated := pgtype.Timestamptz{Time: folder.LastUpdated, Valid: !folder.LastUpdated.IsZero()}
	// Treat "0" as empty/invalid parentId (it's a sentinel value meaning "no parent")
	parentIDValid := folder.ParentID != "" && folder.ParentID != "0"
	parentID := pgtype.Text{String: folder.ParentID, Valid: parentIDValid}
	description := pgtype.Text{String: folder.Description, Valid: folder.Description != ""}
	iconType := pgtype.Text{String: folder.IconType, Valid: folder.IconType != ""}

	params := gen.CreateFolderParams{
		ID:          folder.ID,
		Type:        folder.Type,
		LastUpdated: lastUpdated,
		CreatedBy:   int32(folder.CreatedBy),
		ParentID:    parentID,
		Name:        folder.Name,
		Description: description,
		IconType:    iconType,
	}

	_, err := f.queries.CreateFolder(ctx, f.db.Pool(), params)
	if err != nil {
		// Check if it's a unique constraint violation (record already exists)
		if isUniqueConstraintViolation(err) {
			// Try update if insert fails due to existing record
			updateParams := gen.UpdateFolderParams{
				ID:          folder.ID,
				Type:        folder.Type,
				LastUpdated: lastUpdated,
				Name:        folder.Name,
				Description: description,
				IconType:    iconType,
			}
			_, updateErr := f.queries.UpdateFolder(ctx, f.db.Pool(), updateParams)
			if updateErr != nil {
				f.logger.Error("Failed to update folder",
					zap.String("folder_id", folder.ID),
					zap.Error(updateErr))
				return fmt.Errorf("failed to update folder %s: %w", folder.ID, updateErr)
			}
			f.logger.Debug("Updated existing folder", zap.String("folder_id", folder.ID))
		} else {
			// Log the actual error for debugging
			f.logger.Error("Failed to create folder",
				zap.String("folder_id", folder.ID),
				zap.Error(err))
			return fmt.Errorf("failed to create folder %s: %w", folder.ID, err)
		}
	} else {
		f.logger.Debug("Created folder", zap.String("folder_id", folder.ID))
	}

	return nil
}

// SaveFoldersBatch saves multiple folders in a transaction
func (f *FolderService) SaveFoldersBatch(ctx context.Context, folders []sforce.Folder) error {
	tx, err := f.db.Pool().Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	for _, folder := range folders {
		if err := f.SaveFolder(ctx, folder); err != nil {
			return fmt.Errorf("failed to save folder in batch: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	f.logger.Info("Saved folders batch", zap.Int("count", len(folders)))
	return nil
}

// SaveFoldersInOrder saves folders ensuring parents are saved before children
func (f *FolderService) SaveFoldersInOrder(ctx context.Context, folders []sforce.Folder, folderMap map[string]sforce.Folder) error {
	// Create a map to track which folders have been saved
	saved := make(map[string]bool)
	maxRetries := 5

	// Keep trying until all folders are saved or we've exhausted retries
	for attempt := 0; attempt < maxRetries; attempt++ {
		allSaved := true
		for _, folder := range folders {
			if saved[folder.ID] {
				continue
			}

			// Check if parent exists (either in saved map or in folderMap)
			if folder.ParentID != "" && folder.ParentID != "0" {
				// Check if parent is in our folder list and not yet saved
				if _, parentInList := folderMap[folder.ParentID]; parentInList && !saved[folder.ParentID] {
					allSaved = false
					continue
				}
				// If parent is not in our list, we assume it exists in the database
				// (it might be a top-level folder or was saved in a previous run)
			}

			// Try to save the folder
			if err := f.SaveFolder(ctx, folder); err != nil {
				// Check if it's a foreign key violation (parent doesn't exist)
				if isForeignKeyViolation(err) {
					f.logger.Warn("Failed to save subfolder due to missing parent, will retry",
						zap.String("folder_id", folder.ID),
						zap.String("folder_name", folder.Name),
						zap.String("parent_id", folder.ParentID),
						zap.Int("attempt", attempt+1),
						zap.Error(err))
					allSaved = false
					continue
				}
				// For other errors, log but don't retry (might be a real issue)
				f.logger.Error("Failed to save subfolder with non-FK error",
					zap.String("folder_id", folder.ID),
					zap.String("folder_name", folder.Name),
					zap.Error(err))
				// Continue to next folder, but mark that not all were saved
				allSaved = false
				continue
			}

			saved[folder.ID] = true
			f.logger.Debug("Saved subfolder",
				zap.String("folder_id", folder.ID),
				zap.String("folder_name", folder.Name))
		}

		if allSaved {
			f.logger.Info("Successfully saved all subfolders from initial list",
				zap.Int("total", len(folders)))
			return nil
		}

		// If not all saved and we have retries left, continue
		if attempt < maxRetries-1 {
			f.logger.Debug("Retrying to save remaining subfolders",
				zap.Int("attempt", attempt+1),
				zap.Int("max_retries", maxRetries))
		}
	}

	// If we get here, some folders couldn't be saved
	unsaved := []string{}
	for _, folder := range folders {
		if !saved[folder.ID] {
			unsaved = append(unsaved, folder.ID+"("+folder.Name+")")
		}
	}
	f.logger.Warn("Failed to save some subfolders after all retries",
		zap.Int("unsaved_count", len(unsaved)),
		zap.Strings("unsaved_ids", unsaved))
	// Don't return error - log warning and continue, as these might be fetched later via GetSubFolders
	return nil
}

// isForeignKeyViolation checks if the error is a PostgreSQL foreign key constraint violation
func isForeignKeyViolation(err error) bool {
	if err == nil {
		return false
	}

	// Check for pgx error code 23503 (foreign_key_violation)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23503"
	}

	// Fallback: check error message
	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "foreign key") ||
		strings.Contains(errStr, "violates foreign key constraint")
}
