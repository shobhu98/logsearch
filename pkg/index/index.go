package index

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"backend/pkg/models"
)

func New() *Index {
	return &Index{
		inverted: make(map[string][]posting),
		docs:     make(map[int]*models.Record),
	}
}

// Build constructs the inverted index using a concurrent worker pool
func (idx *Index) Build(ctx context.Context, records []models.Record, numWorkers int) error {
	jobs := make(chan *models.Record, len(records))
	results := make(chan map[string][]posting, numWorkers)
	errs := make(chan error, numWorkers)

	var wg sync.WaitGroup

	for range numWorkers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			local := make(map[string][]posting)
			for {
				select {
				case <-ctx.Done():
					results <- local
					errs <- ctx.Err()
					return
				case rec, ok := <-jobs:
					if !ok {
						results <- local
						errs <- nil
						return
					}
					for field, text := range extractFields(rec) {
						for _, term := range tokenize(text) {
							local[term] = append(local[term], posting{
								docID: rec.ID,
								field: field,
							})
						}
					}
				}
			}
		}()
	}

	for i := range records {
		idx.docs[records[i].ID] = &records[i]
		jobs <- &records[i]
	}
	close(jobs)

	go func() {
		wg.Wait()
		close(results)
		close(errs)
	}()

	for partial := range results {
		for term, postings := range partial {
			idx.inverted[term] = append(idx.inverted[term], postings...)
		}
	}

	for err := range errs {
		if err != nil {
			return err
		}
	}

	// build sorted term slice — enables O(log n) prefix search
	idx.sortedTerms = make([]string, 0, len(idx.inverted))
	for term := range idx.inverted {
		idx.sortedTerms = append(idx.sortedTerms, term)
	}
	sort.Strings(idx.sortedTerms)
	idx.total = len(records)
	return nil
}

// Search returns ranked results for the given query
func (idx *Index) Search(query string) ([]models.SearchResult, float64) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	start := time.Now()
	terms := tokenize(query)
	if len(terms) == 0 {
		return []models.SearchResult{}, 0
	}

	// use scoreKey to prevent double-counting same (doc, field) pair
	type scoreKey struct {
		docID int
		field string
	}
	seen := make(map[scoreKey]bool)
	scores := make(map[int]int)

	for _, term := range terms {
		for _, t := range idx.prefixSearch(term) {
			for _, p := range idx.inverted[t] {
				key := scoreKey{p.docID, p.field}
				if !seen[key] {
					seen[key] = true
					w := fieldWeight[p.field]
					// if field is unknown, assign a default weight of 1 instead of 0
					if w == 0 {
						w = 1
					}
					scores[p.docID] += w
				}
			}
		}
	}

	results := make([]models.SearchResult, 0, len(scores))
	for docID, score := range scores {
		if rec := idx.docs[docID]; rec != nil {
			results = append(results, models.SearchResult{Record: rec, Score: score})
		}
	}

	// rank by score desc, break ties by timestamp desc
	sort.Slice(results, func(i, j int) bool {
		if results[i].Score != results[j].Score {
			return results[i].Score > results[j].Score
		}
		return results[i].Record.NanoTimeStamp > results[j].Record.NanoTimeStamp
	})

	elapsed := float64(time.Since(start).Microseconds()) / 1000.0
	return results, elapsed
}

// prefixSearch uses binary search on sortedTerms — O(log n + k) and not O(n)
func (idx *Index) prefixSearch(prefix string) []string {
	lo := sort.SearchStrings(idx.sortedTerms, prefix)
	terms := []string{}
	for i := lo; i < len(idx.sortedTerms); i++ {
		t := idx.sortedTerms[i]
		if !strings.HasPrefix(t, prefix) {
			break
		}
		terms = append(terms, t)
		if len(terms) >= 50 { // cap expansion
			break
		}
	}
	return terms
}

func (idx *Index) Stats() map[string]int {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return map[string]int{
		"total_docs":  idx.total,
		"total_terms": len(idx.inverted),
	}
}

func extractFields(r *models.Record) map[string]string {
	return map[string]string{
		"Message":        r.Message,
		"MessageRaw":     r.MessageRaw,
		"StructuredData": r.StructuredData,
		"AppName":        r.AppName,
		"Hostname":       r.Hostname,
		"Tag":            r.Tag,
		"SeverityString": r.SeverityString,
		"FacilityString": r.FacilityString,
		"Sender":         r.Sender,
		"Groupings":      r.Groupings,
		"Event":          r.Event,
		"Namespace":      r.Namespace,
		"ProcId":         r.ProcId,
	}
}

func tokenize(text string) []string {
	text = strings.ToLower(text)
	tokens := strings.FieldsFunc(text, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	result := tokens[:0]
	for _, t := range tokens {
		if len(t) >= 2 && !isStopword(t) {
			result = append(result, t)
		}
	}
	return result
}

func isStopword(w string) bool { _, ok := stopwords[w]; return ok }
