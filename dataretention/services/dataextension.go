package services

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/natserract/sf/dataretention/schema/postgres"
	"github.com/natserract/sf/dataretention/schema/postgres/gen"
	salesforce "github.com/natserract/sf/pkg/salesforce/mce"
	"go.uber.org/zap"
)

// DataExtensionService handles data extension persistence operations
type DataExtensionService struct {
	queries *gen.Queries
	db      *postgres.DB
	logger  *zap.Logger
}

// NewDataExtensionService creates a new data extension service
func NewDataExtensionService(db *postgres.DB, logger *zap.Logger) *DataExtensionService {
	return &DataExtensionService{
		queries: gen.New(),
		db:      db,
		logger:  logger,
	}
}

// SaveDataExtension saves or updates a data extension in the database
func (d *DataExtensionService) SaveDataExtension(ctx context.Context, de salesforce.DataExtension) error {
	createdDate := pgtype.Timestamptz{Time: de.CreatedDate.Time, Valid: !de.CreatedDate.Time.IsZero()}
	modifiedDate := pgtype.Timestamptz{Time: de.ModifiedDate.Time, Valid: !de.ModifiedDate.Time.IsZero()}

	description := pgtype.Text{String: de.Description, Valid: de.Description != ""}
	sendableCustomObjectField := pgtype.Text{String: de.SendableCustomObjectField, Valid: de.SendableCustomObjectField != ""}
	sendableSubscriberField := pgtype.Text{String: de.SendableSubscriberField, Valid: de.SendableSubscriberField != ""}
	createdByName := pgtype.Text{String: de.CreatedByName, Valid: de.CreatedByName != ""}
	modifiedByID := pgtype.Int4{Int32: int32(de.ModifiedByID), Valid: de.ModifiedByID != 0}
	modifiedByName := pgtype.Text{String: de.ModifiedByName, Valid: de.ModifiedByName != ""}
	ownerName := pgtype.Text{String: de.OwnerName, Valid: de.OwnerName != ""}
	partnerAPIObjectTypeID := pgtype.Int4{Int32: int32(de.PartnerAPIObjectTypeID), Valid: de.PartnerAPIObjectTypeID != 0}
	partnerAPIObjectTypeName := pgtype.Text{String: de.PartnerAPIObjectTypeName, Valid: de.PartnerAPIObjectTypeName != ""}

	params := gen.CreateDataExtensionParams{
		ID:                         de.ID,
		Name:                       de.Name,
		Key:                        de.Key,
		Description:                description,
		IsActive:                   de.IsActive,
		IsSendable:                 de.IsSendable,
		SendableCustomObjectField:  sendableCustomObjectField,
		SendableSubscriberField:    sendableSubscriberField,
		IsTestable:                 de.IsTestable,
		CategoryID:                 fmt.Sprintf("%d", de.CategoryID),
		OwnerID:                    int32(de.OwnerID),
		IsObjectDeletable:          de.IsObjectDeletable,
		IsFieldAdditionAllowed:     de.IsFieldAdditionAllowed,
		IsFieldModificationAllowed: de.IsFieldModificationAllowed,
		CreatedDate:                createdDate,
		CreatedByID:                int32(de.CreatedByID),
		CreatedByName:              createdByName,
		ModifiedDate:               modifiedDate,
		ModifiedByID:               modifiedByID,
		ModifiedByName:             modifiedByName,
		OwnerName:                  ownerName,
		PartnerApiObjectTypeID:     partnerAPIObjectTypeID,
		PartnerApiObjectTypeName:   partnerAPIObjectTypeName,
		RowCount:                   int32(de.RowCount),
		FieldCount:                 int32(de.FieldCount),
	}

	_, err := d.queries.CreateDataExtension(ctx, d.db.Pool(), params)
	if err != nil {
		// Check if it's a unique constraint violation (record already exists)
		if isUniqueConstraintViolation(err) {
			// Try update if insert fails due to existing record
			updateParams := gen.UpdateDataExtensionParams{
				ID:             de.ID,
				Name:           de.Name,
				Description:    description,
				IsActive:       de.IsActive,
				ModifiedDate:   modifiedDate,
				ModifiedByID:   modifiedByID,
				ModifiedByName: modifiedByName,
				RowCount:       int32(de.RowCount),
				FieldCount:     int32(de.FieldCount),
			}
			_, updateErr := d.queries.UpdateDataExtension(ctx, d.db.Pool(), updateParams)
			if updateErr != nil {
				d.logger.Error("Failed to update data extension",
					zap.String("data_extension_id", de.ID),
					zap.Error(updateErr))
				return fmt.Errorf("failed to update data extension %s: %w", de.ID, updateErr)
			}
			d.logger.Debug("Updated existing data extension", zap.String("data_extension_id", de.ID))
		} else {
			// Log the actual error for debugging
			d.logger.Error("Failed to create data extension",
				zap.String("data_extension_id", de.ID),
				zap.Error(err))
			return fmt.Errorf("failed to create data extension %s: %w", de.ID, err)
		}
	} else {
		d.logger.Debug("Created data extension", zap.String("data_extension_id", de.ID))
	}

	// Save data retention properties if present
	if de.DataRetentionProperties != nil {
		retentionParams := gen.CreateDataRetentionPropertiesParams{
			DataExtensionID:                  de.ID,
			DataRetentionPeriodLength:        int32(de.DataRetentionProperties.DataRetentionPeriodLength),
			DataRetentionPeriodUnitOfMeasure: int32(de.DataRetentionProperties.DataRetentionPeriodUnitOfMeasure),
			IsDeleteAtEndOfRetentionPeriod:   de.DataRetentionProperties.IsDeleteAtEndOfRetentionPeriod,
			IsRowBasedRetention:              de.DataRetentionProperties.IsRowBasedRetention,
			IsResetRetentionPeriodOnImport:   de.DataRetentionProperties.IsResetRetentionPeriodOnImport,
		}

		_, err = d.queries.CreateDataRetentionProperties(ctx, d.db.Pool(), retentionParams)
		if err != nil {
			// Try update if insert fails
			updateRetentionParams := gen.UpdateDataRetentionPropertiesParams{
				DataExtensionID:                  de.ID,
				DataRetentionPeriodLength:        int32(de.DataRetentionProperties.DataRetentionPeriodLength),
				DataRetentionPeriodUnitOfMeasure: int32(de.DataRetentionProperties.DataRetentionPeriodUnitOfMeasure),
				IsDeleteAtEndOfRetentionPeriod:   de.DataRetentionProperties.IsDeleteAtEndOfRetentionPeriod,
				IsRowBasedRetention:              de.DataRetentionProperties.IsRowBasedRetention,
				IsResetRetentionPeriodOnImport:   de.DataRetentionProperties.IsResetRetentionPeriodOnImport,
			}
			_, err = d.queries.UpdateDataRetentionProperties(ctx, d.db.Pool(), updateRetentionParams)
			if err != nil {
				d.logger.Warn("Failed to save retention properties",
					zap.String("data_extension_id", de.ID),
					zap.Error(err))
			}
		}
	}

	return nil
}

// SaveDataExtensionsBatch saves multiple data extensions in a transaction
func (d *DataExtensionService) SaveDataExtensionsBatch(ctx context.Context, dataExtensions []salesforce.DataExtension) error {
	tx, err := d.db.Pool().Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	for _, de := range dataExtensions {
		if err := d.SaveDataExtension(ctx, de); err != nil {
			return fmt.Errorf("failed to save data extension in batch: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	d.logger.Info("Saved data extensions batch", zap.Int("count", len(dataExtensions)))
	return nil
}

// GetDataExtensions fetches all data extensions for a folder with pagination
// Handles pagination internally and returns all matching data extensions as a single slice
func (d *DataExtensionService) GetDataExtensions(ctx context.Context, client salesforce.SalesforceClient, folderID string) ([]salesforce.DataExtension, error) {
	page := 1
	pageSize := 96

	d.logger.Info("Fetching data extensions",
		zap.String("folder_id", folderID))

	var allDataExtensions []salesforce.DataExtension

	for {
		// Fetch data extensions for current page
		resp, err := client.GetDataExtensions(folderID, page, pageSize)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch data extensions for folder %s (page %d): %w", folderID, page, err)
		}

		if len(resp.Items) == 0 {
			// No more items, break the loop
			break
		}

		d.logger.Info("Fetched data extensions page",
			zap.String("folder_id", folderID),
			zap.Int("page", page),
			zap.Int("items_in_page", len(resp.Items)))

		// Add all items to the result
		allDataExtensions = append(allDataExtensions, resp.Items...)

		// Check if there are more pages
		// If we get fewer items than pageSize, we've reached the last page
		if len(resp.Items) < pageSize {
			d.logger.Info("Reached end of data extensions",
				zap.String("folder_id", folderID),
				zap.Int("items_in_page", len(resp.Items)),
				zap.Int("page_size", pageSize))
			break
		}

		// Move to next page
		page++
	}

	d.logger.Info("Completed fetching data extensions for folder",
		zap.String("folder_id", folderID),
		zap.Int("total_items", len(allDataExtensions)))

	return allDataExtensions, nil
}

// UpdateDataRetentionViaAPI updates data retention properties via Salesforce API
// Uses the standard payload: 3 months retention, row-based, no reset on import, no delete at end
func (d *DataExtensionService) UpdateDataRetentionViaAPI(ctx context.Context, client salesforce.SalesforceClient, dataExtensionID string) error {
	// Create the retention properties payload as specified
	retention := &salesforce.DataRetentionProperties{
		DataRetentionPeriodLength:        1,
		DataRetentionPeriodUnitOfMeasure: 5, // 5 = months
		IsDeleteAtEndOfRetentionPeriod:   false,
		IsRowBasedRetention:              true,
		IsResetRetentionPeriodOnImport:   false,
	}

	// First, mark as pending in the database
	_, err := d.queries.UpdateDataRetentionAPIUpdateStatus(ctx, d.db.Pool(), gen.UpdateDataRetentionAPIUpdateStatusParams{
		DataExtensionID:                  dataExtensionID,
		LastApiUpdateStatus:              "pending",
		LastApiUpdateError:               pgtype.Text{Valid: false},
		DataRetentionPeriodLength:        int32(retention.DataRetentionPeriodLength),
		DataRetentionPeriodUnitOfMeasure: int32(retention.DataRetentionPeriodUnitOfMeasure),
		IsRowBasedRetention:              retention.IsRowBasedRetention,
	})
	if err != nil {
		d.logger.Warn("Failed to update retention status to pending",
			zap.String("data_extension_id", dataExtensionID),
			zap.Error(err))
		// Continue with API call even if DB update fails
	}

	// Call the Salesforce API to update retention
	err = client.UpdateDataRetention(dataExtensionID, retention)
	if err != nil {
		// Update database with failed status
		errorMsg := err.Error()
		if len(errorMsg) > 1000 {
			errorMsg = errorMsg[:1000] // Truncate if too long
		}
		_, updateErr := d.queries.UpdateDataRetentionAPIUpdateStatus(ctx, d.db.Pool(), gen.UpdateDataRetentionAPIUpdateStatusParams{
			DataExtensionID:                  dataExtensionID,
			LastApiUpdateStatus:              "failed",
			LastApiUpdateError:               pgtype.Text{String: errorMsg, Valid: true},
			DataRetentionPeriodLength:        int32(retention.DataRetentionPeriodLength),
			DataRetentionPeriodUnitOfMeasure: int32(retention.DataRetentionPeriodUnitOfMeasure),
			IsRowBasedRetention:              retention.IsRowBasedRetention,
		})
		if updateErr != nil {
			d.logger.Error("Failed to update retention status to failed",
				zap.String("data_extension_id", dataExtensionID),
				zap.Error(updateErr))
		}
		return fmt.Errorf("failed to update data retention via API for %s: %w", dataExtensionID, err)
	}

	// Update database with succeeded status and retention properties
	_, err = d.queries.UpdateDataRetentionAPIUpdateStatus(ctx, d.db.Pool(), gen.UpdateDataRetentionAPIUpdateStatusParams{
		DataExtensionID:                  dataExtensionID,
		LastApiUpdateStatus:              "succeeded",
		LastApiUpdateError:               pgtype.Text{Valid: false},
		DataRetentionPeriodLength:        int32(retention.DataRetentionPeriodLength),
		DataRetentionPeriodUnitOfMeasure: int32(retention.DataRetentionPeriodUnitOfMeasure),
		IsRowBasedRetention:              retention.IsRowBasedRetention,
	})
	if err != nil {
		d.logger.Error("Failed to update retention status to succeeded",
			zap.String("data_extension_id", dataExtensionID),
			zap.Error(err))
		// Don't return error since API call succeeded
	}

	d.logger.Info("Successfully updated data retention via API",
		zap.String("data_extension_id", dataExtensionID))

	return nil
}

// isUniqueConstraintViolation checks if the error is a PostgreSQL unique constraint violation
func isUniqueConstraintViolation(err error) bool {
	if err == nil {
		return false
	}

	// Check for pgx error code 23505 (unique_violation)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505"
	}

	// Fallback: check error message
	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "duplicate key") ||
		strings.Contains(errStr, "unique constraint") ||
		strings.Contains(errStr, "already exists")
}
