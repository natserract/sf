package analytics

import (
	"context"
	"database/sql"
	"log"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Analytics struct {
	DB *pgxpool.Pool
}

func (a *Analytics) Run(ctx context.Context) error {
	r := gin.Default()
	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"GET", "POST", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: false,
		MaxAge:           12 * time.Hour,
	}))

	api := r.Group("/api/v1")
	{
		api.GET("/summary", a.getSummary)
		api.GET("/score-distribution", a.getScoreDistribution)
		api.GET("/status-breakdown", a.getStatusBreakdown)
		api.GET("/failure-reasons", a.getFailureReasons)
		api.GET("/stage-performance", a.getStagePerformance)
		api.GET("/results", a.getResults)
		api.GET("/trend", a.getTrend)
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("Server starting on :%s", port)
	r.Run(":" + port)

	return nil
}

// ─── Summary ────────────────────────────────────────────────────────────────
type Summary struct {
	Total          int     `json:"total"`
	Done           int     `json:"done"`
	Failed         int     `json:"failed"`
	Pending        int     `json:"pending"`
	InProgress     int     `json:"in_progress"`
	SuccessRate    float64 `json:"success_rate"`
	AvgScore       float64 `json:"avg_score"`
	MaxScore       int     `json:"max_score"`
	MinScore       int     `json:"min_score"`
	AvgLatencyMs   float64 `json:"avg_latency_ms"`
	UniqueContacts int     `json:"unique_contacts"`
	UniqueRuns     int     `json:"unique_runs"`
}

func (a *Analytics) getSummary(c *gin.Context) {
	query := `
		SELECT
			COUNT(*) AS total,
			COUNT(*) FILTER (WHERE status = 'done') AS done,
			COUNT(*) FILTER (WHERE status = 'failed') AS failed,
			COUNT(*) FILTER (WHERE status = 'pending') AS pending,
			COUNT(*) FILTER (WHERE status = 'in_progress') AS in_progress,
			ROUND(AVG(total_score)::numeric, 2) AS avg_score,
			MAX(total_score) AS max_score,
			MIN(total_score) AS min_score,
			ROUND(AVG(syntax_latency_ms + domain_dns_latency_ms + mx_latency_ms + smtp_latency_ms)::numeric, 2) AS avg_latency_ms,
			COUNT(DISTINCT contact_id) AS unique_contacts,
			COUNT(DISTINCT run_id) AS unique_runs
		FROM validation_results
	`

	var s Summary
	var avgScore, avgLatency sql.NullFloat64
	var maxScore, minScore sql.NullInt64

	err := a.DB.QueryRow(c, query).Scan(
		&s.Total, &s.Done, &s.Failed, &s.Pending, &s.InProgress,
		&avgScore, &maxScore, &minScore, &avgLatency,
		&s.UniqueContacts, &s.UniqueRuns,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	s.AvgScore = avgScore.Float64
	s.AvgLatencyMs = avgLatency.Float64
	if maxScore.Valid {
		s.MaxScore = int(maxScore.Int64)
	}
	if minScore.Valid {
		s.MinScore = int(minScore.Int64)
	}
	if s.Total > 0 {
		s.SuccessRate = math.Round(float64(s.Done)/float64(s.Total)*10000) / 100
	}

	c.JSON(http.StatusOK, s)
}

// ─── Score Distribution (histogram) ─────────────────────────────────────────

type ScoreBucket struct {
	Range string `json:"range"`
	Count int    `json:"count"`
}

func (a *Analytics) getScoreDistribution(c *gin.Context) {
	query := `
		SELECT
			CASE
				WHEN total_score < 20  THEN '0-19'
				WHEN total_score < 40  THEN '20-39'
				WHEN total_score < 60  THEN '40-59'
				WHEN total_score < 80  THEN '60-79'
				WHEN total_score < 100 THEN '80-99'
				ELSE '100+'
			END AS bucket,
			COUNT(*) AS cnt
		FROM validation_results
		GROUP BY 1 ORDER BY 1
	`
	rows, err := a.DB.Query(c, query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	var result []ScoreBucket
	for rows.Next() {
		var b ScoreBucket
		rows.Scan(&b.Range, &b.Count)
		result = append(result, b)
	}
	c.JSON(http.StatusOK, result)
}

// ─── Status Breakdown ────────────────────────────────────────────────────────

type StatusItem struct {
	Status string `json:"status"`
	Count  int    `json:"count"`
}

func (a *Analytics) getStatusBreakdown(c *gin.Context) {
	query := `SELECT status, COUNT(*) FROM validation_results GROUP BY status ORDER BY COUNT(*) DESC`
	rows, err := a.DB.Query(c, query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	var result []StatusItem
	for rows.Next() {
		var item StatusItem
		rows.Scan(&item.Status, &item.Count)
		result = append(result, item)
	}
	c.JSON(http.StatusOK, result)
}

// ─── Failure Reasons ─────────────────────────────────────────────────────────

type FailureReason struct {
	Reason string `json:"reason"`
	Count  int    `json:"count"`
}

func (a *Analytics) getFailureReasons(c *gin.Context) {
	query := `
		SELECT COALESCE(failure_reason, 'unknown'), COUNT(*)
		FROM validation_results
		WHERE status = 'failed'
		GROUP BY 1 ORDER BY 2 DESC LIMIT 20
	`

	rows, err := a.DB.Query(c, query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	var result []FailureReason
	for rows.Next() {
		var r FailureReason
		rows.Scan(&r.Reason, &r.Count)
		result = append(result, r)
	}
	c.JSON(http.StatusOK, result)
}

// ─── Stage Performance (radar / bar) ─────────────────────────────────────────

type StagePerf struct {
	Stage      string  `json:"stage"`
	AvgScore   float64 `json:"avg_score"`
	AvgLatency float64 `json:"avg_latency_ms"`
	PassRate   float64 `json:"pass_rate"`
}

func (a *Analytics) getStagePerformance(c *gin.Context) {
	query := `
		SELECT
			'Syntax' AS stage,
			ROUND(AVG(syntax_score)::numeric,2),
			ROUND(AVG(syntax_latency_ms)::numeric,2),
			ROUND(100.0 * COUNT(*) FILTER (WHERE syntax_status = 'passed') / NULLIF(COUNT(*),0), 2)
		FROM validation_results
		UNION ALL
		SELECT 'Domain DNS',
			ROUND(AVG(domain_dns_score)::numeric,2),
			ROUND(AVG(domain_dns_latency_ms)::numeric,2),
			ROUND(100.0 * COUNT(*) FILTER (WHERE domain_dns_status = 'passed') / NULLIF(COUNT(*),0), 2)
		FROM validation_results
		UNION ALL
		SELECT 'MX',
			ROUND(AVG(mx_score)::numeric,2),
			ROUND(AVG(mx_latency_ms)::numeric,2),
			ROUND(100.0 * COUNT(*) FILTER (WHERE mx_status = 'passed') / NULLIF(COUNT(*),0), 2)
		FROM validation_results
		UNION ALL
		SELECT 'SMTP',
			ROUND(AVG(smtp_score)::numeric,2),
			ROUND(AVG(smtp_latency_ms)::numeric,2),
			ROUND(100.0 * COUNT(*) FILTER (WHERE smtp_status = 'passed') / NULLIF(COUNT(*),0), 2)
		FROM validation_results
		UNION ALL
		SELECT 'History',
			ROUND(AVG(history_score)::numeric,2),
			0,
			ROUND(100.0 * COUNT(*) FILTER (WHERE history_status = 'passed') / NULLIF(COUNT(*),0), 2)
		FROM validation_results`

	rows, err := a.DB.Query(c, query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	var result []StagePerf
	for rows.Next() {
		var sp StagePerf
		var avgScore, avgLatency, passRate sql.NullFloat64
		rows.Scan(&sp.Stage, &avgScore, &avgLatency, &passRate)
		sp.AvgScore = avgScore.Float64
		sp.AvgLatency = avgLatency.Float64
		sp.PassRate = passRate.Float64
		result = append(result, sp)
	}
	c.JSON(http.StatusOK, result)
}

// ─── Paginated Results List ───────────────────────────────────────────────────

type ValidationRow struct {
	ID               int64          `json:"id"`
	RunID            string         `json:"run_id"`
	RowNumber        int            `json:"row_number"`
	ContactID        string         `json:"contact_id"`
	RawContactKey    string         `json:"raw_contact_key"`
	CleanCandidate   sql.NullString `json:"-"`
	CleanCandidateV  string         `json:"clean_candidate"`
	NormalizedEmail  sql.NullString `json:"-"`
	NormalizedEmailV string         `json:"normalized_email"`
	Status           string         `json:"status"`
	FailureReason    sql.NullString `json:"-"`
	FailureReasonV   string         `json:"failure_reason"`
	SyntaxStatus     sql.NullString `json:"-"`
	SyntaxStatusV    string         `json:"syntax_status"`
	DomainDNSStatus  sql.NullString `json:"-"`
	DomainDNSStatusV string         `json:"domain_dns_status"`
	MXStatus         sql.NullString `json:"-"`
	MXStatusV        string         `json:"mx_status"`
	SMTPStatus       sql.NullString `json:"-"`
	SMTPStatusV      string         `json:"smtp_status"`
	HistoryStatus    sql.NullString `json:"-"`
	HistoryStatusV   string         `json:"history_status"`
	TotalScore       int            `json:"total_score"`
	SyntaxScore      int            `json:"syntax_score"`
	DomainScore      int            `json:"domain_dns_score"`
	MXScore          int            `json:"mx_score"`
	SMTPScore        int            `json:"smtp_score"`
	HistoryScore     int            `json:"history_score"`
	CreatedAt        time.Time      `json:"created_at"`
}

type PaginatedResults struct {
	Data       []map[string]interface{} `json:"data"`
	Total      int64                    `json:"total"`
	Page       int                      `json:"page"`
	PageSize   int                      `json:"page_size"`
	TotalPages int                      `json:"total_pages"`
}

func (a *Analytics) getResults(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "50"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 200 {
		pageSize = 50
	}
	offset := (page - 1) * pageSize

	status := c.Query("status")
	search := c.Query("search")
	sortBy := c.DefaultQuery("sort_by", "id")
	sortDir := strings.ToUpper(c.DefaultQuery("sort_dir", "DESC"))

	allowed := map[string]bool{
		"id": true, "total_score": true, "created_at": true,
		"row_number": true, "contact_id": true, "status": true,
	}
	if !allowed[sortBy] {
		sortBy = "id"
	}
	if sortDir != "ASC" {
		sortDir = "DESC"
	}

	conditions := []string{}
	args := []interface{}{}
	idx := 1

	if status != "" {
		conditions = append(conditions, "status = $"+strconv.Itoa(idx))
		args = append(args, status)
		idx++
	}
	if search != "" {
		conditions = append(conditions, "(contact_id ILIKE $"+strconv.Itoa(idx)+
			" OR raw_contact_key ILIKE $"+strconv.Itoa(idx)+
			" OR normalized_email ILIKE $"+strconv.Itoa(idx)+")")
		args = append(args, "%"+search+"%")
		idx++
	}

	where := ""
	if len(conditions) > 0 {
		where = "WHERE " + strings.Join(conditions, " AND ")
	}

	// count
	var total int64
	countQuery := "SELECT COUNT(*) FROM validation_results " + where
	a.DB.QueryRow(c, countQuery, args...).Scan(&total)

	// data
	dataQuery := `
		SELECT id, run_id, row_number, contact_id, raw_contact_key,
			clean_candidate, normalized_email, status, failure_reason,
			syntax_status, domain_dns_status, mx_status, smtp_status, history_status,
			total_score, syntax_score, domain_dns_score, mx_score, smtp_score, history_score,
			created_at
		FROM validation_results ` + where +
		" ORDER BY " + sortBy + " " + sortDir +
		" LIMIT $" + strconv.Itoa(idx) + " OFFSET $" + strconv.Itoa(idx+1)

	args = append(args, pageSize, offset)

	rows, err := a.DB.Query(c, dataQuery, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	data := []map[string]interface{}{}
	for rows.Next() {
		var r ValidationRow
		err := rows.Scan(
			&r.ID, &r.RunID, &r.RowNumber, &r.ContactID, &r.RawContactKey,
			&r.CleanCandidate, &r.NormalizedEmail, &r.Status, &r.FailureReason,
			&r.SyntaxStatus, &r.DomainDNSStatus, &r.MXStatus, &r.SMTPStatus, &r.HistoryStatus,
			&r.TotalScore, &r.SyntaxScore, &r.DomainScore, &r.MXScore, &r.SMTPScore, &r.HistoryScore,
			&r.CreatedAt,
		)
		if err != nil {
			continue
		}
		data = append(data, map[string]interface{}{
			"id":                r.ID,
			"run_id":            r.RunID,
			"row_number":        r.RowNumber,
			"contact_id":        r.ContactID,
			"raw_contact_key":   r.RawContactKey,
			"clean_candidate":   r.CleanCandidate.String,
			"normalized_email":  r.NormalizedEmail.String,
			"status":            r.Status,
			"failure_reason":    r.FailureReason.String,
			"syntax_status":     r.SyntaxStatus.String,
			"domain_dns_status": r.DomainDNSStatus.String,
			"mx_status":         r.MXStatus.String,
			"smtp_status":       r.SMTPStatus.String,
			"history_status":    r.HistoryStatus.String,
			"total_score":       r.TotalScore,
			"syntax_score":      r.SyntaxScore,
			"domain_dns_score":  r.DomainScore,
			"mx_score":          r.MXScore,
			"smtp_score":        r.SMTPScore,
			"history_score":     r.HistoryScore,
			"created_at":        r.CreatedAt,
		})
	}

	totalPages := int(math.Ceil(float64(total) / float64(pageSize)))
	c.JSON(http.StatusOK, PaginatedResults{
		Data:       data,
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
	})
}

// ─── Trend over time ─────────────────────────────────────────────────────────

type TrendPoint struct {
	Date     string  `json:"date"`
	Done     int     `json:"done"`
	Failed   int     `json:"failed"`
	Pending  int     `json:"pending"`
	AvgScore float64 `json:"avg_score"`
}

func (a *Analytics) getTrend(c *gin.Context) {
	runID := c.Query("run_id")
	query := `
		SELECT
			DATE_TRUNC('hour', created_at) AS ts,
			COUNT(*) FILTER (WHERE status = 'done') AS done,
			COUNT(*) FILTER (WHERE status = 'failed') AS failed,
			COUNT(*) FILTER (WHERE status = 'pending') AS pending,
			ROUND(AVG(total_score)::numeric, 2) AS avg_score
		FROM validation_results
	`
	args := []interface{}{}
	if runID != "" {
		query += " WHERE run_id = $1"
		args = append(args, runID)
	}
	query += " GROUP BY 1 ORDER BY 1 DESC LIMIT 48"

	rows, err := a.DB.Query(c, query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	var result []TrendPoint
	for rows.Next() {
		var tp TrendPoint
		var ts time.Time
		var avgScore sql.NullFloat64
		rows.Scan(&ts, &tp.Done, &tp.Failed, &tp.Pending, &avgScore)
		tp.Date = ts.Format("Jan 02 15:04")
		tp.AvgScore = avgScore.Float64
		result = append(result, tp)
	}
	// reverse for chronological display
	for i, j := 0, len(result)-1; i < j; i, j = i+1, j-1 {
		result[i], result[j] = result[j], result[i]
	}
	c.JSON(http.StatusOK, result)
}
