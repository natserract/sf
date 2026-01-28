package sfmce

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	httpclient "github.com/natserract/sf/pkg/http"
	"go.uber.org/zap"
)

// GetFolders retrieves all folders matching the allowed types
func (s *Salesforce) GetFolders() (*FoldersResponse, error) {
	s.logger.Info("Getting folders")
	token, err := s.getAccessToken(context.Background())
	if err != nil {
		s.logger.Error("Failed to get access token", zap.Error(err))
		return nil, err
	}

	endpoint, err := httpclient.BuildURL(s.config.RestBaseURI, "/legacy/v1/beta/folder", map[string]string{
		"$where":       "allowedtypes in ('synchronizeddataextension', 'dataextension', 'shared_data', 'recyclebin')",
		"Localization": "true",
		"_":            strconv.FormatInt(time.Now().Unix(), 10),
	})
	if err != nil {
		s.logger.Error("Failed to build URL", zap.Error(err))
		return nil, fmt.Errorf("failed to build URL: %w", err)
	}

	headers := map[string]string{
		"Authorization": fmt.Sprintf("Bearer %s", token),
	}

	s.logger.Debug("Making GET request", zap.String("endpoint", endpoint))
	resp, err := s.httpClient.Get(context.Background(), endpoint, headers)
	if err != nil {
		s.logger.Error("Get folders request failed", zap.Error(err), zap.String("endpoint", endpoint))
		return nil, fmt.Errorf("get folders request failed: %w", err)
	}

	if resp.StatusCode != 200 {
		s.logger.Error("Get folders failed",
			zap.Int("status_code", resp.StatusCode),
			zap.String("response", string(resp.Body)))
		return nil, fmt.Errorf("get folders failed with status %d: %s", resp.StatusCode, string(resp.Body))
	}

	var foldersResp FoldersResponse
	if err := json.Unmarshal(resp.Body, &foldersResp); err != nil {
		s.logger.Error("Failed to parse folders response", zap.Error(err))
		return nil, fmt.Errorf("failed to parse folders response: %w", err)
	}

	s.logger.Info("Successfully retrieved folders",
		zap.Int("total_results", foldersResp.TotalResults),
		zap.Int("items_count", len(foldersResp.Entry)))

	return &foldersResp, nil
}

// GetSubFolders retrieves subfolders for a given category ID
func (s *Salesforce) GetSubFolders(parentFolderID string) (*FoldersResponse, error) {
	s.logger.Info("Getting subfolders", zap.String("parent_folder_id", parentFolderID))
	token, err := s.getAccessToken(context.Background())
	if err != nil {
		s.logger.Error("Failed to get access token", zap.Error(err))
		return nil, err
	}

	endpoint := fmt.Sprintf("%s/legacy/v1/beta/folder/%s/children?Localization=true&$top=1000&$skip=0", s.config.RestBaseURI, parentFolderID)

	headers := map[string]string{
		"Authorization": fmt.Sprintf("Bearer %s", token),
	}

	s.logger.Debug("Making GET request", zap.String("endpoint", endpoint))
	resp, err := s.httpClient.Get(context.Background(), endpoint, headers)
	if err != nil {
		s.logger.Error("Get subfolders request failed", zap.Error(err), zap.String("endpoint", endpoint))
		return nil, fmt.Errorf("get subfolders request failed: %w", err)
	}

	if resp.StatusCode != 200 {
		s.logger.Error("Get subfolders failed",
			zap.Int("status_code", resp.StatusCode),
			zap.String("response", string(resp.Body)))
		return nil, fmt.Errorf("get subfolders failed with status %d: %s", resp.StatusCode, string(resp.Body))
	}

	var foldersResp FoldersResponse
	if err := json.Unmarshal(resp.Body, &foldersResp); err != nil {
		s.logger.Error("Failed to parse subfolders response", zap.Error(err))
		return nil, fmt.Errorf("failed to parse subfolders response: %w", err)
	}

	s.logger.Info("Successfully retrieved subfolders",
		zap.String("parent_folder_id", parentFolderID),
		zap.Int("total_results", foldersResp.TotalResults),
		zap.Int("items_count", len(foldersResp.Entry)))

	return &foldersResp, nil
}
