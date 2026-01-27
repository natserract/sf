package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	httpclient "github.com/natserract/sf/pkg/http"
	"go.uber.org/zap"
)

// GetDataExtensions retrieves data extensions for a given category ID with pagination
func (s *Salesforce) GetDataExtensions(folderID string, page, pageSize int) (*DataExtensionsResponse, error) {
	s.logger.Info("Getting data extensions",
		zap.String("folder_id", folderID),
		zap.Int("page", page),
		zap.Int("page_size", pageSize))
	token, err := s.getAccessToken(context.Background())
	if err != nil {
		s.logger.Error("Failed to get access token", zap.Error(err))
		return nil, err
	}

	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 25
	}

	queryParams := map[string]string{
		"retrievalType": "1",
		"$page":         strconv.Itoa(page),
		"$pagesize":     strconv.Itoa(pageSize),
		"$orderBy":      "modifiedDate DESC",
		"_":             strconv.FormatInt(time.Now().Unix(), 10),
	}

	endpoint, err := httpclient.BuildURL(s.config.RestBaseURI, fmt.Sprintf("/data/v1/customobjects/category/%s", folderID), queryParams)
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
		s.logger.Error("Get data extensions request failed", zap.Error(err), zap.String("endpoint", endpoint))
		return nil, fmt.Errorf("get data extensions request failed: %w", err)
	}

	if resp.StatusCode != 200 {
		s.logger.Error("Get data extensions failed",
			zap.Int("status_code", resp.StatusCode),
			zap.String("response", string(resp.Body)))
		return nil, fmt.Errorf("get data extensions failed with status %d: %s", resp.StatusCode, string(resp.Body))
	}

	var dataExtResp DataExtensionsResponse
	if err := json.Unmarshal(resp.Body, &dataExtResp); err != nil {
		s.logger.Error("Failed to parse data extensions response", zap.Error(err))
		return nil, fmt.Errorf("failed to parse data extensions response: %w", err)
	}

	s.logger.Info("Successfully retrieved data extensions",
		zap.String("folder_id", folderID),
		zap.Int("items_count", len(dataExtResp.Items)))

	return &dataExtResp, nil
}

// UpdateDataRetention updates the data retention properties for a data extension
func (s *Salesforce) UpdateDataRetention(dataExtensionID string, retention *DataRetentionProperties) error {
	s.logger.Info("Updating data retention",
		zap.String("data_extension_id", dataExtensionID),
		zap.Int("retention_period_length", retention.DataRetentionPeriodLength),
		zap.Int("retention_period_unit", retention.DataRetentionPeriodUnitOfMeasure),
		zap.Bool("row_based_retention", retention.IsRowBasedRetention))
	token, err := s.getAccessToken(context.Background())
	if err != nil {
		s.logger.Error("Failed to get access token", zap.Error(err))
		return err
	}

	endpoint := fmt.Sprintf("%s/data/v1/customobjects/%s", s.config.RestBaseURI, dataExtensionID)

	headers := map[string]string{
		"Authorization": fmt.Sprintf("Bearer %s", token),
	}

	requestBody := UpdateDataRetentionRequest{
		DataRetentionProperties: retention,
	}

	s.logger.Debug("Making PATCH request", zap.String("endpoint", endpoint))
	resp, err := s.httpClient.Patch(context.Background(), endpoint, headers, requestBody)
	if err != nil {
		s.logger.Error("Update data retention request failed", zap.Error(err), zap.String("endpoint", endpoint))
		return fmt.Errorf("update data retention request failed: %w", err)
	}

	if resp.StatusCode != 200 && resp.StatusCode != 204 {
		s.logger.Error("Update data retention failed",
			zap.Int("status_code", resp.StatusCode),
			zap.String("response", string(resp.Body)))
		return fmt.Errorf("update data retention failed with status %d: %s", resp.StatusCode, string(resp.Body))
	}

	s.logger.Info("Successfully updated data retention", zap.String("data_extension_id", dataExtensionID))
	return nil
}
