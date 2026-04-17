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

			// 2. Fetch History Concurrently
			var withHistory []string
			maxRetries := 3

			for attempt := 0; attempt < maxRetries; attempt++ {
				currentAuth := p.AuthMgr.GetAuth()

				results, response, err := p.API.FetchMessageHistoryConcurrent(ctx, currentAuth, contacts, p.MaxWorkers)

				if err == nil {
					withHistory = results
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

				// Handle other retriable errors (e.g. timeout)
				log.Printf("Batch failed: %v. Retrying in 2s...", err)
				time.Sleep(2 * time.Second)
			}

			// 3. Update DB
			if err := p.Repo.UpdateHistoryStatus(ctx, withHistory); err != nil {
				log.Printf("Failed to update DB: %v", err)
			}

			log.Printf("Successfully processed batch: %d found history", len(withHistory))
		}
	}
}
