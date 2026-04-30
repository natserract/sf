package exporter

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strconv"
	"time"

	"sfmc-retention/internal/models"
)

const (
	timeFormatRFC3339 = time.RFC3339
)

// ExportJourneyRefs writes step1 output (journey->eventDefinition refs).
func ExportJourneyRefs(refs []models.JourneyEventRef, path string) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create file %s: %w", path, err)
	}
	defer f.Close()

	w := csv.NewWriter(f)
	defer w.Flush()

	headers := []string{"business_unit_id", "journey_id", "journey_name", "event_def_id"}
	if err := w.Write(headers); err != nil {
		return err
	}
	for _, r := range refs {
		if err := w.Write([]string{r.BusinessUnitID, r.JourneyID, r.JourneyName, r.EventDefID}); err != nil {
			return err
		}
	}
	return nil
}

func ReadJourneyRefs(path string) ([]models.JourneyEventRef, error) {
	rows, err := readCSVRows(path)
	if err != nil {
		return nil, err
	}

	out := make([]models.JourneyEventRef, 0, len(rows))
	for _, row := range rows {
		out = append(out, models.JourneyEventRef{
			BusinessUnitID: row["business_unit_id"],
			JourneyID:      row["journey_id"],
			JourneyName:    row["journey_name"],
			EventDefID:     row["event_def_id"],
		})
	}
	return out, nil
}

// ExportEventDefRefs writes step2 output (eventDefinition->dataExtension refs).
func ExportEventDefRefs(refs []models.EventDefinitionRef, path string) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create file %s: %w", path, err)
	}
	defer f.Close()

	w := csv.NewWriter(f)
	defer w.Flush()

	headers := []string{"business_unit_id", "journey_id", "journey_name", "event_def_id", "de_id"}
	if err := w.Write(headers); err != nil {
		return err
	}
	for _, r := range refs {
		if err := w.Write([]string{r.BusinessUnitID, r.JourneyID, r.JourneyName, r.EventDefID, r.DataExtID}); err != nil {
			return err
		}
	}
	return nil
}

func ReadEventDefRefs(path string) ([]models.EventDefinitionRef, error) {
	rows, err := readCSVRows(path)
	if err != nil {
		return nil, err
	}

	out := make([]models.EventDefinitionRef, 0, len(rows))
	for _, row := range rows {
		out = append(out, models.EventDefinitionRef{
			BusinessUnitID: row["business_unit_id"],
			JourneyID:      row["journey_id"],
			JourneyName:    row["journey_name"],
			EventDefID:     row["event_def_id"],
			DataExtID:      row["de_id"],
		})
	}
	return out, nil
}

// ExportBefore writes a CSV snapshot of data extensions before any update.
func ExportBefore(des []models.DataExtension, path string) error {
	return writeCSV(path, des, "before")
}

// ExportAfter writes a CSV snapshot after update (re-fetched data).
func ExportAfter(des []models.DataExtension, path string) error {
	return writeCSV(path, des, "after")
}

func ReadDataExtensions(path string) ([]models.DataExtension, error) {
	rows, err := readCSVRows(path)
	if err != nil {
		return nil, err
	}
	out := make([]models.DataExtension, 0, len(rows))
	for _, row := range rows {
		d := models.DataExtension{
			BusinessUnitID: row["business_unit_id"],
			ID:             row["de_id"],
			Name:           row["de_name"],
			CustomerKey:    row["customer_key"],
			IsSendable:     parseBool(row["is_sendable"]),
			IsActive:       parseBool(row["is_active"]),
			FieldCount:     parseInt(row["field_count"]),
			TotalRowCount:  parseInt(row["total_row_count"]),
			CreatedDate:    row["created_date"],
			ModifiedDate:   row["modified_date"],
			CreatedByName:  row["created_by"],
			ModifiedByName: row["modified_by"],
			Description:    row["description"],
			JourneyID:      row["journey_id"],
			JourneyName:    row["journey_name"],
			SourceType:     row["source_type"],
			DataRetentionProperties: models.DataRetentionProps{
				DataRetentionPeriodLength:        parseInt(row["retention_period_length"]),
				DataRetentionPeriodUnitOfMeasure: parseInt(row["retention_period_unit"]),
				IsDeleteAtEndOfRetentionPeriod:   parseBool(row["is_delete_at_end"]),
				IsRowBasedRetention:              parseBool(row["is_row_based"]),
				IsResetRetentionPeriodOnImport:   parseBool(row["is_reset_on_import"]),
			},
		}
		out = append(out, d)
	}
	return out, nil
}

// ExportResults writes a summary CSV of the update run.
func ExportResults(results []models.ProcessResult, path string) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create file %s: %w", path, err)
	}
	defer f.Close()

	w := csv.NewWriter(f)
	defer w.Flush()

	headers := []string{
		"business_unit_id", "de_id", "de_name", "customer_key", "journey_id", "journey_name",
		"source_type", "updated", "error",
	}
	if err := w.Write(headers); err != nil {
		return err
	}

	for _, r := range results {
		d := r.DataExtension
		errStr := ""
		if r.Error != nil {
			errStr = r.Error.Error()
		}
		row := []string{
			d.BusinessUnitID,
			d.ID,
			d.Name,
			d.CustomerKey,
			d.JourneyID,
			d.JourneyName,
			d.SourceType,
			strconv.FormatBool(r.Updated),
			errStr,
		}
		if err := w.Write(row); err != nil {
			return err
		}
	}
	return nil
}

func ReadResults(path string) ([]models.ProcessResult, error) {
	rows, err := readCSVRows(path)
	if err != nil {
		return nil, err
	}
	out := make([]models.ProcessResult, 0, len(rows))
	for _, row := range rows {
		de := models.DataExtension{
			BusinessUnitID: row["business_unit_id"],
			ID:             row["de_id"],
			Name:           row["de_name"],
			CustomerKey:    row["customer_key"],
			JourneyID:      row["journey_id"],
			JourneyName:    row["journey_name"],
			SourceType:     row["source_type"],
		}
		res := models.ProcessResult{
			DataExtension: de,
			Updated:       parseBool(row["updated"]),
		}
		if row["error"] != "" {
			res.Error = fmt.Errorf("%s", row["error"])
		}
		out = append(out, res)
	}
	return out, nil
}

func writeCSV(path string, des []models.DataExtension, label string) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create file %s: %w", path, err)
	}
	defer f.Close()

	w := csv.NewWriter(f)
	defer w.Flush()

	headers := []string{
		"de_id",
		"business_unit_id",
		"de_name",
		"customer_key",
		"is_sendable",
		"is_active",
		"field_count",
		"total_row_count",
		"created_date",
		"modified_date",
		"created_by",
		"modified_by",
		"description",
		"journey_id",
		"journey_name",
		"source_type",
		"retention_period_length",
		"retention_period_unit",
		"retention_period_unit_label",
		"is_delete_at_end",
		"is_row_based",
		"is_reset_on_import",
		"snapshot_type",
		"snapshot_time",
	}
	if err := w.Write(headers); err != nil {
		return err
	}

	now := time.Now().UTC().Format(timeFormatRFC3339)
	for _, d := range des {
		rp := d.DataRetentionProperties
		row := []string{
			d.ID,
			d.BusinessUnitID,
			d.Name,
			d.CustomerKey,
			strconv.FormatBool(d.IsSendable),
			strconv.FormatBool(d.IsActive),
			strconv.Itoa(d.FieldCount),
			strconv.Itoa(d.TotalRowCount),
			d.CreatedDate,
			d.ModifiedDate,
			d.CreatedByName,
			d.ModifiedByName,
			d.Description,
			d.JourneyID,
			d.JourneyName,
			d.SourceType,
			strconv.Itoa(rp.DataRetentionPeriodLength),
			strconv.Itoa(rp.DataRetentionPeriodUnitOfMeasure),
			models.UnitOfMeasureLabel(rp.DataRetentionPeriodUnitOfMeasure),
			strconv.FormatBool(rp.IsDeleteAtEndOfRetentionPeriod),
			strconv.FormatBool(rp.IsRowBasedRetention),
			strconv.FormatBool(rp.IsResetRetentionPeriodOnImport),
			label,
			now,
		}
		if err := w.Write(row); err != nil {
			return err
		}
	}
	return nil
}

func readCSVRows(path string) ([]map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open file %s: %w", path, err)
	}
	defer f.Close()

	r := csv.NewReader(f)
	headers, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("read headers %s: %w", path, err)
	}

	var rows []map[string]string
	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read row %s: %w", path, err)
		}

		row := make(map[string]string, len(headers))
		for i, h := range headers {
			if i < len(record) {
				row[h] = record[i]
			} else {
				row[h] = ""
			}
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func parseBool(v string) bool {
	b, err := strconv.ParseBool(v)
	if err != nil {
		return false
	}
	return b
}

func parseInt(v string) int {
	i, err := strconv.Atoi(v)
	if err != nil {
		return 0
	}
	return i
}
