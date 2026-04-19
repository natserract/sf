package web

import (
	"context"
	"embed"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"sf/usecases/web-checker/fetch-all-contacts-go/internal/history"
	"sf/usecases/web-checker/fetch-all-contacts-go/internal/store"
	"sf/usecases/web-checker/fetch-all-contacts-go/internal/validator"
)

//go:embed static/*
var staticFS embed.FS

type Server struct {
	repo       *store.Repo
	validator  *validator.Service
	history    *history.Client
	httpServer *http.Server
	workers    int
	mu         sync.Mutex
	activeRuns map[string]bool
}

func NewServer(repo *store.Repo, validatorService *validator.Service, historyClient *history.Client, workers int) *Server {
	if workers <= 0 {
		workers = 20
	}
	s := &Server{
		repo:       repo,
		validator:  validatorService,
		history:    historyClient,
		workers:    workers,
		activeRuns: map[string]bool{},
	}

	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.FS(staticFS)))
	mux.HandleFunc("/api/runs", s.createRun)
	mux.HandleFunc("/api/runs/", s.handleRunRoutes)
	mux.HandleFunc("/api/history/fetch", s.fetchHistory)

	s.httpServer = &http.Server{
		Handler:      withCORS(mux),
		ReadTimeout:  60 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}
	return s
}

func (s *Server) ListenAndServe(addr string) error {
	s.httpServer.Addr = addr
	return s.httpServer.ListenAndServe()
}

func (s *Server) createRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "file is required", http.StatusBadRequest)
		return
	}
	defer file.Close()

	rows, err := parseCSV(file)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if len(rows) == 0 {
		http.Error(w, "csv has no rows", http.StatusBadRequest)
		return
	}

	runID, err := s.repo.CreateValidationRun(r.Context(), header.Filename, len(rows))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := s.repo.InsertRows(r.Context(), runID, rows); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	go s.processRun(context.Background(), runID)
	writeJSON(w, http.StatusCreated, map[string]any{
		"runId": runID,
		"rows":  len(rows),
	})
}

func parseCSV(reader io.Reader) ([]store.CreateInput, error) {
	c := csv.NewReader(reader)
	c.FieldsPerRecord = -1
	records, err := c.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("read csv: %w", err)
	}
	if len(records) < 2 {
		return nil, nil
	}
	header := records[0]
	var contactIDIdx = -1
	var contactKeyIdx = -1
	for i, col := range header {
		switch strings.TrimSpace(col) {
		case "contactID.value":
			contactIDIdx = i
		case "contactKey.value":
			contactKeyIdx = i
		}
	}
	if contactIDIdx < 0 || contactKeyIdx < 0 {
		return nil, fmt.Errorf("csv must include contactID.value and contactKey.value columns")
	}

	out := make([]store.CreateInput, 0, len(records)-1)
	for i, rec := range records[1:] {
		if contactIDIdx >= len(rec) || contactKeyIdx >= len(rec) {
			continue
		}
		out = append(out, store.CreateInput{
			RowNumber:     i + 2,
			ContactID:     strings.TrimSpace(rec[contactIDIdx]),
			RawContactKey: rec[contactKeyIdx],
		})
	}
	return out, nil
}

func (s *Server) processRun(ctx context.Context, runID string) {
	s.mu.Lock()
	if s.activeRuns[runID] {
		s.mu.Unlock()
		return
	}
	s.activeRuns[runID] = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.activeRuns, runID)
		s.mu.Unlock()
	}()

	var wg sync.WaitGroup
	for i := 0; i < s.workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				row, err := s.repo.ClaimNextPending(ctx, runID)
				if err != nil || row == nil {
					return
				}
				result := s.validator.Validate(ctx, row.RawContactKey)
				status := "done"
				if result.Status == "failed" {
					status = "failed"
				}
				_ = s.repo.UpdateValidation(ctx, store.ValidationUpdate{
					ID:              row.ID,
					Status:          status,
					FailureReason:   result.Reason,
					CleanCandidate:  result.Clean.Cleaned,
					NormalizedEmail: result.Clean.Normalized,
					SyntaxStatus:    result.Syntax.Status,
					SyntaxReason:    result.Syntax.Reason,
					SyntaxLatencyMS: result.Syntax.LatencyMS,
					SyntaxScore:     result.Syntax.Score,
					DomainDNSStatus: result.Domain.Status,
					DomainDNSReason: result.Domain.Reason,
					DomainLatencyMS: result.Domain.LatencyMS,
					DomainScore:     result.Domain.Score,
					MXStatus:        result.MX.Status,
					MXReason:        result.MX.Reason,
					MXLatencyMS:     result.MX.LatencyMS,
					MXScore:         result.MX.Score,
					SMTPStatus:      result.SMTP.Status,
					SMTPReason:      result.SMTP.Reason,
					SMTPLatencyMS:   result.SMTP.LatencyMS,
					SMTPScore:       result.SMTP.Score,
					HistoryStatus:   "pending",
					HistoryReason:   "fetch on demand",
					HistoryScore:    0,
					TotalScore:      result.Total,
				})
			}
		}()
	}
	wg.Wait()
	_, _ = s.repo.CompleteRunIfDone(ctx, runID)
}

func (s *Server) handleRunRoutes(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/runs/")
	parts := strings.Split(path, "/")
	if len(parts) == 0 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	runID := parts[0]
	if len(parts) == 1 && r.Method == http.MethodGet {
		progress, err := s.repo.RunProgress(r.Context(), runID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, progress)
		return
	}
	if len(parts) >= 2 && parts[1] == "results" {
		if len(parts) == 2 && r.Method == http.MethodGet {
			offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
			limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
			query := r.URL.Query().Get("q")
			rows, total, err := s.repo.ListResults(r.Context(), runID, offset, limit, query)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"items": rows, "total": total})
			return
		}
		if len(parts) == 3 && r.Method == http.MethodGet {
			rowID, err := strconv.ParseInt(parts[2], 10, 64)
			if err != nil {
				http.Error(w, "invalid row id", http.StatusBadRequest)
				return
			}
			row, err := s.repo.GetResult(r.Context(), runID, rowID)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			writeJSON(w, http.StatusOK, row)
			return
		}
	}
	http.NotFound(w, r)
}

type historyReq struct {
	RunID       string            `json:"runId"`
	RowID       int64             `json:"rowId"`
	ContactID   string            `json:"contactId"`
	BearerToken string            `json:"bearerToken"`
	CsrfToken   string            `json:"csrfToken"`
	Cookie      string            `json:"cookie"`
	Headers     map[string]string `json:"headers"`
}

func (s *Server) fetchHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req historyReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json body", http.StatusBadRequest)
		return
	}
	if req.ContactID == "" || req.RowID == 0 {
		http.Error(w, "contactId and rowId are required", http.StatusBadRequest)
		return
	}

	auth := history.AuthInput{
		BearerToken: req.BearerToken,
		CsrfToken:   req.CsrfToken,
		Cookie:      req.Cookie,
	}
	if auth.BearerToken == "" {
		auth.BearerToken = req.Headers["authorization"]
	}
	if auth.CsrfToken == "" {
		auth.CsrfToken = req.Headers["x-csrf-token"]
	}
	if auth.Cookie == "" {
		auth.Cookie = req.Headers["cookie"]
	}

	body, err := s.history.FetchMessageHistory(r.Context(), auth, req.ContactID)
	if err != nil {
		_ = s.repo.SaveHistory(r.Context(), req.RowID, "failed", err.Error(), 0, []byte(`{}`))
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	_ = s.repo.SaveHistory(r.Context(), req.RowID, "passed", "history fetched", 0, body)
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"payload": json.RawMessage(body),
	})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-CSRF-Token, Cookie")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
