package runner

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"sf/usecases/mail-checker/internal/db"
	"sf/usecases/mail-checker/internal/validator"
	"strings"
	"testing"
)

func TestDetectDelimiter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		line string
		want rune
	}{
		{name: "comma separated", line: "a,b,c\n", want: ','},
		{name: "tab separated", line: "a\tb\tc\n", want: '\t'},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			r := bufio.NewReader(strings.NewReader(tt.line))
			got, err := detectDelimiter(r)
			if err != nil {
				t.Fatalf("detectDelimiter returned error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("expected delimiter %q, got %q", string(tt.want), string(got))
			}
		})
	}
}

func TestFindCSVIndexes(t *testing.T) {
	t.Parallel()

	header := []string{
		"SubscriberID__c",
		"KQ_SubscriberKey__c",
		"SubscriberKey__c",
	}

	keyIdx, idIdx, err := findCSVIndexes(header)
	if err != nil {
		t.Fatalf("findCSVIndexes returned error: %v", err)
	}

	if keyIdx != 2 {
		t.Fatalf("expected SubscriberKey__c index 2, got %d", keyIdx)
	}
	if idIdx != 0 {
		t.Fatalf("expected SubscriberID__c index 0, got %d", idIdx)
	}
}

type mockCSVRepo struct {
	findRunID      string
	createRunID    string
	createCalled   bool
	reopenCalled   bool
	requeueCalled  bool
	progressCalled bool
}

func (m *mockCSVRepo) FindLatestUnfinishedValidationRunBySource(ctx context.Context, sourceFile string) (string, error) {
	return m.findRunID, nil
}

func (m *mockCSVRepo) CreateValidationRun(ctx context.Context, sourceFile string, totalRows int) (string, error) {
	m.createCalled = true
	return m.createRunID, nil
}

func (m *mockCSVRepo) ReopenValidationRun(ctx context.Context, runID string) error {
	m.reopenCalled = true
	return nil
}

func (m *mockCSVRepo) RequeueValidationInProgress(ctx context.Context, runID string) (int64, error) {
	m.requeueCalled = true
	return 3, nil
}

func (m *mockCSVRepo) MarkValidationRunFailed(ctx context.Context, runID, reason string) error {
	return nil
}

func (m *mockCSVRepo) UpdateValidationRunTotalRows(ctx context.Context, runID string, totalRows int) error {
	return nil
}

func (m *mockCSVRepo) CreateValidationResults(ctx context.Context, runID string, rows []db.CreateValidationResultInput) error {
	return nil
}

func (m *mockCSVRepo) GetValidationRunProgress(ctx context.Context, runID string) (db.ValidationRunProgress, error) {
	m.progressCalled = true
	return db.ValidationRunProgress{
		TotalRows:  10,
		Pending:    0,
		InProgress: 0,
		Done:       10,
		Failed:     0,
	}, nil
}

func (m *mockCSVRepo) ClaimPendingValidationResults(ctx context.Context, runID string, limit int) ([]db.ValidationRow, error) {
	return nil, nil
}

func (m *mockCSVRepo) BulkUpdateValidationByID(ctx context.Context, updates []db.ValidationUpdate) error {
	return nil
}

func (m *mockCSVRepo) CompleteValidationRun(ctx context.Context, runID string) (bool, error) {
	return true, nil
}

func TestRun_ResumesExistingRun(t *testing.T) {
	t.Parallel()

	repo := &mockCSVRepo{
		findRunID:   "existing-run-id",
		createRunID: "new-run-id",
	}
	p := &CSVValidationProcessor{
		Repo:      repo,
		Validator: validator.NewService(),
		Source:    "unused.csv",
	}

	runID, resumed, err := p.Run(context.Background())
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if runID != "existing-run-id" {
		t.Fatalf("expected existing run id, got %s", runID)
	}
	if !resumed {
		t.Fatalf("expected resumed=true")
	}
	if repo.createCalled {
		t.Fatalf("did not expect create run on resume path")
	}
	if !repo.reopenCalled || !repo.requeueCalled {
		t.Fatalf("expected reopen and requeue to be called")
	}
}

func TestRun_CreatesNewRunWhenNoUnfinishedRun(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	csvPath := filepath.Join(dir, "sample.csv")
	if err := os.WriteFile(csvPath, []byte("SubscriberID__c,SubscriberKey__c\n"), 0o600); err != nil {
		t.Fatalf("write temp csv: %v", err)
	}

	repo := &mockCSVRepo{
		findRunID:   "",
		createRunID: "new-run-id",
	}
	p := &CSVValidationProcessor{
		Repo:      repo,
		Validator: validator.NewService(),
		Source:    csvPath,
	}

	runID, resumed, err := p.Run(context.Background())
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if runID != "new-run-id" {
		t.Fatalf("expected new run id, got %s", runID)
	}
	if resumed {
		t.Fatalf("expected resumed=false")
	}
	if !repo.createCalled {
		t.Fatalf("expected create run to be called")
	}
}
