package store

import (
	"database/sql"
	"fmt"
	"time"

	"sfmc-retention/internal/models"
)

type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) SaveStep1(runID string, refs []models.JourneyEventRef) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`INSERT INTO journey_refs (run_id, business_unit_id, journey_id, journey_name, event_def_id) VALUES ($1,$2,$3,$4,$5)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, v := range refs {
		if _, err := stmt.Exec(runID, v.BusinessUnitID, v.JourneyID, v.JourneyName, v.EventDefID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *Repository) LoadStep1(runID, buID string) (string, []models.JourneyEventRef, error) {
	returnedRunID := runID
	if returnedRunID == "" {
		var err error
		returnedRunID, err = r.latestRunID("journey_refs", buID)
		if err != nil {
			return "", nil, err
		}
	}

	rows, err := r.db.Query(`SELECT business_unit_id, journey_id, journey_name, event_def_id FROM journey_refs WHERE run_id = $1 AND business_unit_id = $2 ORDER BY created_at ASC`, returnedRunID, buID)
	if err != nil {
		return "", nil, err
	}
	defer rows.Close()

	var out []models.JourneyEventRef
	for rows.Next() {
		var v models.JourneyEventRef
		if err := rows.Scan(&v.BusinessUnitID, &v.JourneyID, &v.JourneyName, &v.EventDefID); err != nil {
			return "", nil, err
		}
		out = append(out, v)
	}
	return returnedRunID, out, rows.Err()
}

func (r *Repository) SaveStep2(runID string, refs []models.EventDefinitionRef) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(`INSERT INTO event_defs (run_id, business_unit_id, journey_id, journey_name, event_def_id, de_id) VALUES ($1,$2,$3,$4,$5,$6)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, v := range refs {
		if _, err := stmt.Exec(runID, v.BusinessUnitID, v.JourneyID, v.JourneyName, v.EventDefID, v.DataExtID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *Repository) LoadStep2(runID, buID string) (string, []models.EventDefinitionRef, error) {
	returnedRunID := runID
	if returnedRunID == "" {
		var err error
		returnedRunID, err = r.latestRunID("event_defs", buID)
		if err != nil {
			return "", nil, err
		}
	}

	rows, err := r.db.Query(`SELECT business_unit_id, journey_id, journey_name, event_def_id, de_id FROM event_defs WHERE run_id = $1 AND business_unit_id = $2 ORDER BY created_at ASC`, returnedRunID, buID)
	if err != nil {
		return "", nil, err
	}
	defer rows.Close()

	var out []models.EventDefinitionRef
	for rows.Next() {
		var v models.EventDefinitionRef
		if err := rows.Scan(&v.BusinessUnitID, &v.JourneyID, &v.JourneyName, &v.EventDefID, &v.DataExtID); err != nil {
			return "", nil, err
		}
		out = append(out, v)
	}
	return returnedRunID, out, rows.Err()
}

func (r *Repository) SaveStep3(runID string, des []models.DataExtension) error {
	return r.saveDataExtensions("data_extensions", runID, des)
}

func (r *Repository) SaveStep5(runID string, des []models.DataExtension) error {
	return r.saveDataExtensions("after_data_extensions", runID, des)
}

func (r *Repository) LoadStep3(runID, buID string) (string, []models.DataExtension, error) {
	return r.loadDataExtensions("data_extensions", runID, buID)
}

func (r *Repository) LoadStep5(runID, buID string) (string, []models.DataExtension, error) {
	return r.loadDataExtensions("after_data_extensions", runID, buID)
}

func (r *Repository) SaveStep4(runID string, results []models.ProcessResult) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`INSERT INTO update_results (run_id, business_unit_id, de_id, de_name, customer_key, journey_id, journey_name, source_type, updated, error_text) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, r := range results {
		errText := ""
		if r.Error != nil {
			errText = r.Error.Error()
		}
		de := r.DataExtension
		if _, err := stmt.Exec(runID, de.BusinessUnitID, de.ID, de.Name, de.CustomerKey, de.JourneyID, de.JourneyName, de.SourceType, r.Updated, errText); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *Repository) LoadStep4(runID, buID string) (string, []models.ProcessResult, error) {
	returnedRunID := runID
	if returnedRunID == "" {
		var err error
		returnedRunID, err = r.latestRunID("update_results", buID)
		if err != nil {
			return "", nil, err
		}
	}

	rows, err := r.db.Query(`SELECT business_unit_id, de_id, de_name, customer_key, journey_id, journey_name, source_type, updated, COALESCE(error_text, '') FROM update_results WHERE run_id = $1 AND business_unit_id = $2 ORDER BY created_at ASC`, returnedRunID, buID)
	if err != nil {
		return "", nil, err
	}
	defer rows.Close()

	var out []models.ProcessResult
	for rows.Next() {
		var (
			de      models.DataExtension
			updated bool
			errText string
		)
		if err := rows.Scan(&de.BusinessUnitID, &de.ID, &de.Name, &de.CustomerKey, &de.JourneyID, &de.JourneyName, &de.SourceType, &updated, &errText); err != nil {
			return "", nil, err
		}
		pr := models.ProcessResult{DataExtension: de, Updated: updated}
		if errText != "" {
			pr.Error = fmt.Errorf("%s", errText)
		}
		out = append(out, pr)
	}
	return returnedRunID, out, rows.Err()
}

func (r *Repository) saveDataExtensions(table, runID string, des []models.DataExtension) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	query := fmt.Sprintf(`INSERT INTO %s (run_id, business_unit_id, de_id, de_name, customer_key, journey_id, journey_name, source_type, is_sendable, is_active, field_count, total_row_count, created_date, modified_date, created_by, modified_by, description, retention_period_length, retention_period_unit, is_delete_at_end, is_row_based, is_reset_on_import, created_at, snapshot_time) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24)`, table)
	stmt, err := tx.Prepare(query)
	if err != nil {
		return err
	}
	defer stmt.Close()

	now := time.Now().UTC()
	for _, d := range des {
		rp := d.DataRetentionProperties
		if _, err := stmt.Exec(
			runID, d.BusinessUnitID, d.ID, d.Name, d.CustomerKey, d.JourneyID, d.JourneyName, d.SourceType,
			d.IsSendable, d.IsActive, d.FieldCount, d.TotalRowCount, d.CreatedDate, d.ModifiedDate, d.CreatedByName,
			d.ModifiedByName, d.Description, rp.DataRetentionPeriodLength, rp.DataRetentionPeriodUnitOfMeasure,
			rp.IsDeleteAtEndOfRetentionPeriod, rp.IsRowBasedRetention, rp.IsResetRetentionPeriodOnImport, now, now,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *Repository) loadDataExtensions(table, runID, buID string) (string, []models.DataExtension, error) {
	returnedRunID := runID
	if returnedRunID == "" {
		var err error
		returnedRunID, err = r.latestRunID(table, buID)
		if err != nil {
			return "", nil, err
		}
	}

	query := fmt.Sprintf(`SELECT business_unit_id, de_id, de_name, customer_key, journey_id, journey_name, source_type, is_sendable, is_active, field_count, total_row_count, created_date, modified_date, created_by, modified_by, description, retention_period_length, retention_period_unit, is_delete_at_end, is_row_based, is_reset_on_import FROM %s WHERE run_id = $1 AND business_unit_id = $2 ORDER BY created_at ASC`, table)
	rows, err := r.db.Query(query, returnedRunID, buID)
	if err != nil {
		return "", nil, err
	}
	defer rows.Close()

	var out []models.DataExtension
	for rows.Next() {
		var d models.DataExtension
		if err := rows.Scan(
			&d.BusinessUnitID, &d.ID, &d.Name, &d.CustomerKey, &d.JourneyID, &d.JourneyName, &d.SourceType,
			&d.IsSendable, &d.IsActive, &d.FieldCount, &d.TotalRowCount, &d.CreatedDate, &d.ModifiedDate, &d.CreatedByName,
			&d.ModifiedByName, &d.Description, &d.DataRetentionProperties.DataRetentionPeriodLength,
			&d.DataRetentionProperties.DataRetentionPeriodUnitOfMeasure, &d.DataRetentionProperties.IsDeleteAtEndOfRetentionPeriod,
			&d.DataRetentionProperties.IsRowBasedRetention, &d.DataRetentionProperties.IsResetRetentionPeriodOnImport,
		); err != nil {
			return "", nil, err
		}
		out = append(out, d)
	}
	return returnedRunID, out, rows.Err()
}

func (r *Repository) latestRunID(table, buID string) (string, error) {
	query := fmt.Sprintf(`SELECT run_id FROM %s WHERE business_unit_id = $1 ORDER BY created_at DESC LIMIT 1`, table)
	var runID string
	if err := r.db.QueryRow(query, buID).Scan(&runID); err != nil {
		if err == sql.ErrNoRows {
			return "", fmt.Errorf("no rows found in %s for business unit %s", table, buID)
		}
		return "", err
	}
	return runID, nil
}
