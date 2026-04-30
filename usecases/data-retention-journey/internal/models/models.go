package models

// ─── Journey List ─────────────────────────────────────────────────────────────

type JourneyListResponse struct {
	Count    int       `json:"count"`
	Page     int       `json:"page"`
	PageSize int       `json:"pageSize"`
	Items    []Journey `json:"items"`
}

type Journey struct {
	ID         string     `json:"id"`
	Key        string     `json:"key"`
	Name       string     `json:"name"`
	Status     string     `json:"status"`
	Version    int        `json:"version"`
	Triggers   []Trigger  `json:"triggers"`
	Activities []Activity `json:"activities"`
}

type Trigger struct {
	ID       string          `json:"id"`
	Key      string          `json:"key"`
	Type     string          `json:"type"`
	MetaData TriggerMetaData `json:"metaData"`
}

type TriggerMetaData struct {
	EventDefinitionID  string `json:"eventDefinitionId"`
	EventDefinitionKey string `json:"eventDefinitionKey"`
}

type Activity struct {
	ID   string `json:"id"`
	Key  string `json:"key"`
	Type string `json:"type"`
}

// ─── Event Definition ─────────────────────────────────────────────────────────

type EventDefinition struct {
	ID                string `json:"id"`
	Type              string `json:"type"`
	Name              string `json:"name"`
	DataExtensionID   string `json:"dataExtensionId"`
	DataExtensionName string `json:"dataExtensionName"`
}

// JourneyEventRef is the minimal payload needed between step1 and step2.
type JourneyEventRef struct {
	BusinessUnitID string
	JourneyID      string
	JourneyName    string
	EventDefID     string
}

// EventDefinitionRef is the minimal payload needed between step2 and step3.
type EventDefinitionRef struct {
	BusinessUnitID string
	JourneyID      string
	JourneyName    string
	EventDefID     string
	DataExtID      string
}

// ─── Data Extension ───────────────────────────────────────────────────────────

type DataExtension struct {
	ID                      string             `json:"id"`
	Name                    string             `json:"name"`
	CustomerKey             string             `json:"customerKey"`
	IsSendable              bool               `json:"isSendable"`
	IsActive                bool               `json:"isActive"`
	FieldCount              int                `json:"fieldCount"`
	TotalRowCount           int                `json:"totalRowCount"`
	CreatedDate             string             `json:"createdDate"`
	ModifiedDate            string             `json:"modifiedDate"`
	CreatedByName           string             `json:"createdByName"`
	ModifiedByName          string             `json:"modifiedByName"`
	Description             string             `json:"description"`
	DataRetentionProperties DataRetentionProps `json:"dataRetentionProperties"`
	// Enriched during processing
	BusinessUnitID string `json:"-"`
	JourneyName    string `json:"-"`
	JourneyID      string `json:"-"`
	SourceType     string `json:"-"` // "journey" or "sendable"
}

type DataRetentionProps struct {
	DataRetentionPeriodLength        int   `json:"dataRetentionPeriodLength"`
	DataRetentionPeriodUnitOfMeasure int   `json:"dataRetentionPeriodUnitOfMeasure"`
	IsDeleteAtEndOfRetentionPeriod   bool  `json:"isDeleteAtEndOfRetentionPeriod"`
	IsRowBasedRetention              bool  `json:"isRowBasedRetention"`
	IsResetRetentionPeriodOnImport   bool  `json:"isResetRetentionPeriodOnImport"`
	RowBasedThreshold                int64 `json:"rowBasedThreshold,omitempty"`
}

// UnitOfMeasureLabel returns a human-readable label for retention period units.
// SFMC values: 1=Days, 2=Weeks, 3=Months(?), 4=Years(?), 5=Years(confirmed)
func UnitOfMeasureLabel(unit int) string {
	switch unit {
	case 1:
		return "Days"
	case 2:
		return "Weeks"
	case 3:
		return "Months"
	case 4:
		return "Quarters"
	case 5:
		return "Years"
	default:
		return "Unknown"
	}
}

// ─── Processing result ────────────────────────────────────────────────────────

type ProcessResult struct {
	DataExtension DataExtension
	Updated       bool
	Error         error
}
