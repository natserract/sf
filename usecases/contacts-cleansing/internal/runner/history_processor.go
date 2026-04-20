package runner

import (
	"context"
	"log"
	"sf/usecases/mail-checker/internal/api"
	"sf/usecases/mail-checker/internal/db"
	"sf/usecases/mail-checker/internal/validator"
	"strings"
	"time"
)

type HistoryProcessor struct {
	Repo       *db.Repo
	API        *api.Client
	AuthMgr    *AuthManager
	Validator  *validator.Service
	MaxWorkers int
	BatchSize  int
}

func (p *HistoryProcessor) Run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			// ── 1. Get a batch from DB ─────────────────────────────────────────
			// Fetches contact_id + contact_key for contacts not yet history-checked.
			contacts, err := p.Repo.GetContactsForHistory(ctx, p.BatchSize)
			if err != nil {
				return err
			}
			if len(contacts) == 0 {
				log.Println("No more contacts to process. Sleeping...")
				time.Sleep(30 * time.Second)
				continue
			}

			allProcessed := make([]string, len(contacts))
			for i, c := range contacts {
				allProcessed[i] = c.ContactID
			}

			log.Printf("[history] processing batch of %d contacts", len(contacts))

			runID, err := p.Repo.CreateValidationRun(ctx, "default", len(contacts))
			if err != nil {
				return err
			}
			contactRows := make([]db.CreateValidationResultInput, 0, len(contacts))
			for i, rec := range contacts {
				contactRows = append(contactRows, db.CreateValidationResultInput{
					RowNumber:     i,
					RawContactKey: rec.ContactKey,
					ContactID:     strings.TrimSpace(rec.ContactID),
				})
			}
			if err := p.Repo.CreateValidationResults(ctx, runID, contactRows); err != nil {
				return err
			}

			validContactIDs := make([]string, 0, len(contacts))
			for _, c := range contacts {
				row, err := p.Repo.GetPendingValidationResult(ctx, runID)
				if err != nil || row == nil {
					return err
				}

				result := p.Validator.Validate(ctx, c.ContactKey)

				// Validate non email type contact key
				isPhoneNumber := validator.ValidatePhoneNumber(c.ContactKey, "ID")
				isNumber := validator.IsNumber(c.ContactKey)

				overallStatus := "done"
				if result.Status == "failed" {
					overallStatus = "failed"
				}

				if isPhoneNumber || isNumber {
					_ = p.Repo.UpdateValidation(ctx, db.ValidationUpdate{
						ID:              row.ID,
						Status:          "done", // skipped
						FailureReason:   "Skipped is phone or number",
						CleanCandidate:  c.ContactKey,
						NormalizedEmail: "",
						SyntaxStatus:    "skipped",
						SyntaxReason:    "Skipped is phone or number",
						SyntaxLatencyMS: 0,
						SyntaxScore:     0,
						DomainDNSStatus: "skipped",
						DomainDNSReason: "skipped",
						DomainLatencyMS: 0,
						DomainScore:     0,
						MXStatus:        "skipped",
						MXReason:        "skipped",
						MXLatencyMS:     0,
						MXScore:         0,
						SMTPStatus:      "skipped",
						SMTPReason:      "skipped",
						SMTPLatencyMS:   0,
						SMTPScore:       0,
						HistoryStatus:   "done",
						HistoryReason:   "fetch on demand",
						HistoryScore:    0,
						TotalScore:      100, // let it pass, we don't check phone number yet
					})
				} else {
					_ = p.Repo.UpdateValidation(ctx, db.ValidationUpdate{
						ID:              row.ID,
						Status:          overallStatus,
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

				if overallStatus == "done" {
					validContactIDs = append(validContactIDs, c.ContactID)
				}
			}

			if _, err := p.Repo.CompleteValidationRun(ctx, runID); err != nil {
				log.Printf("[history] failed to persist validation results: %v", err)
				// Non-fatal: history fetch still runs for contacts that passed
				// in memory even if the DB write failed.
			}

			log.Printf("[history] validation done — %d/%d passed", len(validContactIDs), len(contacts))

			// ── 3. Fetch history (valid contacts only) ─────────────────────────
			// If every contact failed validation fall back to the full batch so
			// we don't silently skip contacts on a transient validator error.
			historyTargets := contacts
			if len(validContactIDs) > 0 && len(validContactIDs) < len(contacts) {
				historyTargets = filterContacts(contacts, validContactIDs)
			}

			var withHistory []string
			const maxRetries = 3

			for attempt := 0; attempt < maxRetries; attempt++ {
				currentAuth := p.AuthMgr.GetAuth()

				results, response, err := p.API.FetchMessageHistoryConcurrent(ctx, currentAuth, historyTargets, p.MaxWorkers)
				if err == nil {
					withHistory = results
					log.Printf("[history] batch fetched OK on attempt %d/%d — %d/%d had history",
						attempt+1, maxRetries, len(withHistory), len(historyTargets))
					break
				}

				if api.IsAuthError(response) {
					log.Printf("[history] auth expired (attempt %d/%d), re-authenticating...", attempt+1, maxRetries)
					reauthErr := p.AuthMgr.ReauthenticateIfUnchanged(ctx, currentAuth, func(c context.Context, a api.Auth) error {
						_, pingErr := p.API.PingAuth(c, a, api.PingAuthParams{})
						return pingErr
					})
					if reauthErr != nil {
						return reauthErr
					}
					continue
				}

				if attempt < maxRetries-1 {
					log.Printf("[history] batch error on attempt %d/%d: %v — retrying in 2s", attempt+1, maxRetries, err)
					time.Sleep(2 * time.Second)
				} else {
					log.Printf("[history] batch error on attempt %d/%d: %v — giving up on this batch", attempt+1, maxRetries, err)
				}
			}

			// ── 4. Persist history results ─────────────────────────────────────
			// Always marks the full batch as checked so contacts are never
			// re-visited, even when the fetch failed entirely.
			if err := p.Repo.UpdateHistoryStatus(ctx, withHistory, allProcessed); err != nil {
				log.Printf("[history] failed to update history status for batch: %v", err)
			} else {
				log.Printf("[history] batch committed — %d/%d with history, %d marked checked",
					len(withHistory), len(contacts), len(allProcessed))
			}
		}
	}
}

// filterContacts returns only the contacts whose ContactID is in keepIDs.
func filterContacts(contacts []api.ContactInfo, keepIDs []string) []api.ContactInfo {
	keep := make(map[string]struct{}, len(keepIDs))
	for _, id := range keepIDs {
		keep[id] = struct{}{}
	}
	out := make([]api.ContactInfo, 0, len(keepIDs))
	for _, c := range contacts {
		if _, ok := keep[c.ContactID]; ok {
			out = append(out, c)
		}
	}
	return out
}
