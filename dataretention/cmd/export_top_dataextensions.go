package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"github.com/natserract/sf/pkg/config"
	salesforce "github.com/natserract/sf/pkg/salesforce/mce"
	"go.uber.org/zap"
)

const (
	pageSize     = 96
	topCount     = 20
	defaultFname = "export.json"
)

func main() {
	logger, err := zap.NewProduction()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync()

	cfg, err := config.Load()
	if err != nil {
		logger.Error("Failed to load config", zap.Error(err))
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	client := salesforce.NewSalesforceWithLogger(cfg, logger)

	// Phase 1 – full folder set
	folderIDs, err := collectAllFolderIDs(client, logger)
	if err != nil {
		logger.Error("Phase 1 failed", zap.Error(err))
		fmt.Fprintf(os.Stderr, "Phase 1 (folders) failed: %v\n", err)
		os.Exit(1)
	}
	logger.Info("Phase 1 done", zap.Int("folder_count", len(folderIDs)))

	// Phase 2 – all data extensions
	allDE, err := fetchAllDataExtensions(client, folderIDs, logger)
	if err != nil {
		logger.Error("Phase 2 failed", zap.Error(err))
		fmt.Fprintf(os.Stderr, "Phase 2 (data extensions) failed: %v\n", err)
		os.Exit(1)
	}
	logger.Info("Phase 2 done", zap.Int("data_extension_count", len(allDE)))

	// Phase 3 – sort by RowCount desc, take top 20
	sort.Slice(allDE, func(i, j int) bool {
		return allDE[i].RowCount > allDE[j].RowCount
	})
	top := allDE
	if len(top) > topCount {
		top = top[:topCount]
	}

	// Phase 4 – export
	if err := os.MkdirAll("exports", 0755); err != nil {
		logger.Error("Failed to create exports dir", zap.Error(err))
		fmt.Fprintf(os.Stderr, "Failed to create exports dir: %v\n", err)
		os.Exit(1)
	}
	fname := defaultFname
	if cfg.AccountID != "" {
		fname = cfg.AccountID + ".json"
	}
	path := "exports/" + fname
	payload, err := json.MarshalIndent(top, "", "  ")
	if err != nil {
		logger.Error("Failed to marshal JSON", zap.Error(err))
		fmt.Fprintf(os.Stderr, "Failed to marshal JSON: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(path, payload, 0644); err != nil {
		logger.Error("Failed to write export file", zap.String("path", path), zap.Error(err))
		fmt.Fprintf(os.Stderr, "Failed to write %s: %v\n", path, err)
		os.Exit(1)
	}
	logger.Info("Export written", zap.String("path", path), zap.Int("count", len(top)))
	fmt.Printf("Exported top %d data extensions to %s\n", len(top), path)
}

// collectAllFolderIDs returns a unique slice of folder IDs by traversing
// GetFolders() and recursively GetSubFolders until no new IDs are found.
func collectAllFolderIDs(client salesforce.SalesforceClient, logger *zap.Logger) ([]string, error) {
	seen := make(map[string]bool)
	var queue []string

	resp, err := client.GetFolders()
	if err != nil {
		return nil, fmt.Errorf("GetFolders: %w", err)
	}
	for _, f := range resp.Entry {
		if !seen[f.ID] {
			seen[f.ID] = true
			queue = append(queue, f.ID)
		}
	}

	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		sub, err := client.GetSubFolders(id)
		if err != nil {
			logger.Warn("GetSubFolders failed", zap.String("folder_id", id), zap.Error(err))
			continue
		}
		for _, f := range sub.Entry {
			if !seen[f.ID] {
				seen[f.ID] = true
				queue = append(queue, f.ID)
			}
		}
	}

	ids := make([]string, 0, len(seen))
	for k := range seen {
		ids = append(ids, k)
	}
	return ids, nil
}

// fetchAllDataExtensions calls GetDataExtensions for each folder ID with
// pagination (loop until len(resp.Items) < pageSize) and returns one slice.
func fetchAllDataExtensions(client salesforce.SalesforceClient, folderIDs []string, logger *zap.Logger) ([]salesforce.DataExtension, error) {
	var all []salesforce.DataExtension
	for _, folderID := range folderIDs {
		page := 1
		for {
			resp, err := client.GetDataExtensions(folderID, page, pageSize)
			if err != nil {
				return nil, fmt.Errorf("GetDataExtensions folder=%s page=%d: %w", folderID, page, err)
			}
			if len(resp.Items) == 0 {
				break
			}
			for _, de := range resp.Items {
				if de.CategoryFullPathForRecycleBin == nil || *de.CategoryFullPathForRecycleBin == "" {
					all = append(all, de)
				}
			}
			if len(resp.Items) < pageSize {
				break
			}
			page++
		}
	}
	return all, nil
}
