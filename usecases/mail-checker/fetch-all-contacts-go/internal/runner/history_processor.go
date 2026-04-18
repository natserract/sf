package runner

import (
	"context"
	"log"
	"sf/usecases/mail-checker/fetch-all-contacts-go/internal/api"
	"sf/usecases/mail-checker/fetch-all-contacts-go/internal/db"
	"time"
)

type HistoryProcessor struct {
	Repo       *db.Repo
	API        *api.Client
	AuthMgr    *AuthManager
	MaxWorkers int
	BatchSize  int
}

func (p *HistoryProcessor) Run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			// 1. Get a batch from DB
			contacts, err := p.Repo.GetContactsForHistory(ctx, p.BatchSize)
			if err != nil {
				return err
			}
			if len(contacts) == 0 {
				log.Println("No more contacts to process. Sleeping...")
				time.Sleep(30 * time.Second)
				continue
			}

			// Build a flat list of all contact IDs in this batch so we can mark
			// every one of them as checked regardless of the outcome.
			allProcessed := make([]string, len(contacts))
			for i, c := range contacts {
				allProcessed[i] = c.ContactID
			}

			log.Printf("[history] processing batch of %d contacts", len(contacts))

			// 2. Fetch History Concurrently
			var withHistory []string
			maxRetries := 3

			for attempt := 0; attempt < maxRetries; attempt++ {
				currentAuth := p.AuthMgr.GetAuth()

				results, response, err := p.API.FetchMessageHistoryConcurrent(ctx, currentAuth, contacts, p.MaxWorkers)

				if err == nil {
					withHistory = results
					log.Printf("[history] batch fetched OK on attempt %d/%d — %d/%d had history",
						attempt, maxRetries, len(withHistory), len(contacts))
					break
				}

				// Check if error is Auth related (401/403)
				// Note: You may need a helper to check if the error contains a 401 response
				if api.IsAuthError(response) {
					log.Printf("Auth expired (Attempt %d/%d). Requesting new token...", attempt+1, maxRetries)

					// This blocks and asks the user for input
					reauthErr := p.AuthMgr.ReauthenticateIfUnchanged(ctx, currentAuth, func(c context.Context, a api.Auth) error {
						_, pingErr := p.API.PingAuth(c, a, api.PingAuthParams{})
						return pingErr
					})

					if reauthErr != nil {
						return reauthErr
					}
					continue // Retry with new credentials
				}

				// Transient error (timeout, network blip, etc.)
				if attempt < maxRetries {
					log.Printf("[history] batch error on attempt %d/%d: %v — retrying in 2s", attempt, maxRetries, err)
					time.Sleep(2 * time.Second)
				} else {
					log.Printf("[history] batch error on attempt %d/%d: %v — giving up on this batch", attempt, maxRetries, err)
				}
			}

			// ── 3. Persist results ─────────────────────────────────────────────
			// Always mark the full batch as checked so we never re-visit the same
			// contacts on the next iteration, even if the fetch failed.
			if err := p.Repo.UpdateHistoryStatus(ctx, withHistory, allProcessed); err != nil {
				log.Printf("[history] failed to update DB for batch: %v", err)
			} else {
				log.Printf("[history] batch committed — %d/%d with history, %d marked checked",
					len(withHistory), len(contacts), len(allProcessed))
			}
		}
	}
}
