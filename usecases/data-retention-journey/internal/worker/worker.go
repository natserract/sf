package worker

import (
	"fmt"
	"log"
	"sync"

	"sfmc-retention/internal/client"
	"sfmc-retention/internal/models"
)

const (
	journeyDetailConcurrency = 10
	eventDefConcurrency      = 10
	dataExtConcurrency       = 10
	updateConcurrency        = 5
)

type Pipeline struct {
	client  *client.Client
	dryRun  bool
	verbose bool
}

func New(c *client.Client, dryRun, verbose bool) *Pipeline {
	return &Pipeline{client: c, dryRun: dryRun, verbose: verbose}
}

func (p *Pipeline) FetchJourneys() ([]models.Journey, error) {
	log.Println("→ Fetching all journeys (paginated)...")
	journeys, err := p.client.GetAllJourneys()
	if err != nil {
		return nil, fmt.Errorf("fetch all journeys: %w", err)
	}
	log.Printf("  Found %d journeys\n", len(journeys))
	return journeys, nil
}

func ExtractJourneyEventRefs(journeys []models.Journey, businessUnitID string) []models.JourneyEventRef {
	var refs []models.JourneyEventRef
	for _, j := range journeys {
		for _, t := range j.Triggers {
			if t.MetaData.EventDefinitionID != "" {
				refs = append(refs, models.JourneyEventRef{
					BusinessUnitID: businessUnitID,
					JourneyID:      j.ID,
					JourneyName:    j.Name,
					EventDefID:     t.MetaData.EventDefinitionID,
				})
			}
		}
	}
	return refs
}

func (p *Pipeline) ResolveEventDefinitions(refs []models.JourneyEventRef) []models.EventDefinitionRef {
	log.Printf("→ Resolving %d event definitions...\n", len(refs))

	// Fan-out: fetch event definitions concurrently
	type edResult struct {
		ref models.JourneyEventRef
		ed  *models.EventDefinition
		err error
	}
	edCh := make(chan edResult, len(refs))
	sem := make(chan struct{}, eventDefConcurrency)
	var wg sync.WaitGroup
	for _, ref := range refs {
		wg.Add(1)
		go func(r models.JourneyEventRef) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			ed, err := p.client.GetEventDefinition(r.EventDefID)
			edCh <- edResult{r, ed, err}
		}(ref)
	}
	go func() { wg.Wait(); close(edCh) }()

	// Keep journey-to-DE mapping per journey (no dedupe), so each DE record
	// preserves which journey it came from.
	var deRefs []models.EventDefinitionRef
	for res := range edCh {
		if res.err != nil {
			log.Printf("  [warn] event def %s: %v\n", res.ref.EventDefID, res.err)
			continue
		}
		if res.ed.DataExtensionID == "" {
			continue
		}
		deRefs = append(deRefs, models.EventDefinitionRef{
			BusinessUnitID: res.ref.BusinessUnitID,
			JourneyID:      res.ref.JourneyID,
			JourneyName:    res.ref.JourneyName,
			EventDefID:     res.ref.EventDefID,
			DataExtID:      res.ed.DataExtensionID,
		})
	}
	return deRefs
}

func (p *Pipeline) FetchDataExtensions(deRefs []models.EventDefinitionRef) []models.DataExtension {
	unique := map[string]struct{}{}
	for _, ref := range deRefs {
		unique[ref.DataExtID] = struct{}{}
	}
	log.Printf("→ Fetching %d data extension details (%d unique IDs)...\n", len(deRefs), len(unique))

	// Fetch each unique DE once.
	type deLookup struct {
		de  *models.DataExtension
		err error
	}
	cache := make(map[string]deLookup, len(unique))
	var mu sync.Mutex
	sem2 := make(chan struct{}, dataExtConcurrency)
	var wg2 sync.WaitGroup
	for deID := range unique {
		wg2.Add(1)
		go func(id string) {
			defer wg2.Done()
			sem2 <- struct{}{}
			defer func() { <-sem2 }()
			de, err := p.client.GetDataExtension(id)
			mu.Lock()
			cache[id] = deLookup{de: de, err: err}
			mu.Unlock()
		}(deID)
	}
	wg2.Wait()

	var des []models.DataExtension
	for _, ref := range deRefs {
		found, ok := cache[ref.DataExtID]
		if !ok || found.err != nil {
			if ok {
				log.Printf("  [warn] data extension %s: %v\n", ref.DataExtID, found.err)
			}
			continue
		}
		de := *found.de
		de.BusinessUnitID = ref.BusinessUnitID
		de.JourneyID = ref.JourneyID
		de.JourneyName = ref.JourneyName
		de.SourceType = "journey"
		des = append(des, de)
	}

	return des
}

// Discover returns all unique data extensions referenced by journeys.
// It runs the full chain: journeys → event definitions → data extensions.
func (p *Pipeline) Discover() ([]models.DataExtension, error) {
	journeys, err := p.FetchJourneys()
	if err != nil {
		return nil, err
	}
	refs := ExtractJourneyEventRefs(journeys, "")
	deRefs := p.ResolveEventDefinitions(refs)
	return p.FetchDataExtensions(deRefs), nil
}

// Update applies 7-day retention to each data extension concurrently.
// Returns results (updated + errors).
func (p *Pipeline) Update(des []models.DataExtension) []models.ProcessResult {
	results := make([]models.ProcessResult, len(des))
	sem := make(chan struct{}, updateConcurrency)
	var wg sync.WaitGroup

	for i, de := range des {
		wg.Add(1)
		go func(idx int, d models.DataExtension) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			res := models.ProcessResult{DataExtension: d}

			if p.dryRun {
				if p.verbose {
					log.Printf("  [dry-run] would update DE: %s (%s)\n", d.Name, d.ID)
				}
				res.Updated = false
				results[idx] = res
				return
			}

			if err := p.client.UpdateDataRetention(d.ID); err != nil {
				log.Printf("  [error] updating DE %s: %v\n", d.Name, err)
				res.Error = err
				results[idx] = res
				return
			}

			if p.verbose {
				log.Printf("  [ok] updated DE: %s (%s)\n", d.Name, d.ID)
			}
			res.Updated = true
			results[idx] = res
		}(i, de)
	}
	wg.Wait()
	return results
}

// FetchUpdated re-fetches DE details after update to capture new state.
func (p *Pipeline) FetchUpdated(results []models.ProcessResult) []models.DataExtension {
	var updated []models.DataExtension
	sem := make(chan struct{}, dataExtConcurrency)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, r := range results {
		if r.Error != nil || !r.Updated {
			continue
		}
		wg.Add(1)
		go func(res models.ProcessResult) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			de, err := p.client.GetDataExtension(res.DataExtension.ID)
			if err != nil {
				log.Printf("  [warn] re-fetch DE %s: %v\n", res.DataExtension.ID, err)
				return
			}
			de.JourneyID = res.DataExtension.JourneyID
			de.JourneyName = res.DataExtension.JourneyName
			de.SourceType = res.DataExtension.SourceType
			mu.Lock()
			updated = append(updated, *de)
			mu.Unlock()
		}(r)
	}
	wg.Wait()
	return updated
}
