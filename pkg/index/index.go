package index

import (
	"context"

	"math"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"backend/pkg/models"
)

func New() *Index {
	return &Index{
		inverted:    make(map[string][]posting),
		docs:        make(map[int]*models.Record),
		docFreq:     make(map[string]int),
		fieldLens:   make(map[int]map[string]int),
		avgFieldLen: make(map[string]float64),
		cache:       newQueryCache(cacheTTL),
	}
}

// Build constructs the inverted index using a concurrent worker pool.
// Each worker accumulates term frequencies per (term, docID, field) triple and
// field token counts per docID. The main goroutine merges these partials, then
// computes per-term document frequencies and per-field average lengths needed
// for BM25 scoring at query time.
func (idx *Index) Build(ctx context.Context, records []models.Record, numWorkers int) error {
	// postingKey uniquely identifies a (term, document, field) triple.
	type postingKey struct {
		term  string
		docID int
		field string
	}
	type partialResult struct {
		postings  map[postingKey]int     // (term, docID, field) -> termFreq
		fieldLens map[int]map[string]int // docID -> field -> token count
	}

	jobs := make(chan *models.Record, len(records))
	results := make(chan partialResult, numWorkers)
	errs := make(chan error, numWorkers)

	var wg sync.WaitGroup

	for range numWorkers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			local := partialResult{
				postings:  make(map[postingKey]int),
				fieldLens: make(map[int]map[string]int),
			}
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
					local.fieldLens[rec.ID] = make(map[string]int)
					for field, text := range extractFields(rec) {
						tokens := tokenize(text)
						local.fieldLens[rec.ID][field] = len(tokens)
						for _, term := range tokens {
							local.postings[postingKey{term, rec.ID, field}]++
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

	// Merge partial results from all workers.
	mergedPostings := make(map[postingKey]int)
	for partial := range results {
		for key, freq := range partial.postings {
			mergedPostings[key] += freq
		}
		for docID, fields := range partial.fieldLens {
			if idx.fieldLens[docID] == nil {
				idx.fieldLens[docID] = make(map[string]int)
			}
			for field, l := range fields {
				idx.fieldLens[docID][field] = l
			}
		}
	}

	for err := range errs {
		if err != nil {
			return err
		}
	}

	// Build the inverted index from merged postings.
	for key, freq := range mergedPostings {
		idx.inverted[key.term] = append(idx.inverted[key.term], posting{
			docID:    key.docID,
			field:    key.field,
			termFreq: freq,
		})
	}

	// Compute per-term document frequency (number of distinct docs containing the term).
	for term, posts := range idx.inverted {
		seen := make(map[int]struct{}, len(posts))
		for _, p := range posts {
			seen[p.docID] = struct{}{}
		}
		idx.docFreq[term] = len(seen)
	}

	// Compute average field token length across all documents.
	fieldLenSums := make(map[string]int)
	fieldLenCounts := make(map[string]int)
	for _, fields := range idx.fieldLens {
		for field, l := range fields {
			fieldLenSums[field] += l
			fieldLenCounts[field]++
		}
	}
	for field, sum := range fieldLenSums {
		if fieldLenCounts[field] > 0 {
			idx.avgFieldLen[field] = float64(sum) / float64(fieldLenCounts[field])
		}
	}

	// Build sorted term slice — enables O(log n) prefix search.
	idx.sortedTerms = make([]string, 0, len(idx.inverted))
	for term := range idx.inverted {
		idx.sortedTerms = append(idx.sortedTerms, term)
	}
	sort.Strings(idx.sortedTerms)
	idx.total = len(records)

	return nil
}

// Search returns BM25-ranked results for the given query.
// Results are served from a 30s in-memory cache on repeated identical queries.
func (idx *Index) Search(query string) ([]models.SearchResult, float64) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	start := time.Now()
	terms := tokenize(query)
	if len(terms) == 0 {
		return []models.SearchResult{}, 0
	}

	// Cache lookup — key is the sorted, deduplicated token list.
	cacheKey := strings.Join(terms, " ")
	if cached, ok := idx.cache.get(cacheKey); ok {
		elapsed := float64(time.Since(start).Microseconds()) / 1000.0
		return cached, elapsed
	}

	N := idx.total
	scores := make(map[int]float64)

	for _, term := range terms {
		for _, t := range idx.prefixSearch(term) {
			df := idx.docFreq[t]
			if df == 0 {
				continue
			}
			// Robertson-Sparck Jones IDF with smoothing to avoid negatives.
			idf := math.Log((float64(N)-float64(df)+0.5)/(float64(df)+0.5) + 1)

			for _, p := range idx.inverted[t] {
				fw := fieldWeight[p.field]
				if fw == 0 {
					fw = 1
				}
				docLen := float64(idx.fieldLens[p.docID][p.field])
				avgLen := idx.avgFieldLen[p.field]
				if avgLen == 0 {
					avgLen = 1
				}
				tf := float64(p.termFreq)
				// BM25 term score for this (term, doc, field) triple.
				norm := tf * (bm25K1 + 1) / (tf + bm25K1*(1-bm25B+bm25B*docLen/avgLen))
				scores[p.docID] += fw * idf * norm
			}
		}
	}

	results := make([]models.SearchResult, 0, len(scores))

	for docID, score := range scores {
		if rec := idx.docs[docID]; rec != nil {
			results = append(results, models.SearchResult{Record: rec, Score: score})
		}
	}

	// Rank by BM25 score desc; break ties by timestamp desc.
	sort.Slice(results, func(i, j int) bool {
		if results[i].Score != results[j].Score {
			return results[i].Score > results[j].Score
		}
		return results[i].Record.NanoTimeStamp > results[j].Record.NanoTimeStamp
	})

	idx.cache.set(cacheKey, results)

	elapsed := float64(time.Since(start).Microseconds()) / 1000.0
	return results, elapsed
}

// prefixSearch uses binary search on sortedTerms — O(log n + k), not O(n).
func (idx *Index) prefixSearch(prefix string) []string {
	lo := sort.SearchStrings(idx.sortedTerms, prefix)
	terms := []string{}
	for i := lo; i < len(idx.sortedTerms); i++ {
		t := idx.sortedTerms[i]
		if !strings.HasPrefix(t, prefix) {
			break
		}
		terms = append(terms, t)
		if len(terms) >= maxPrefixTerms {
			break
		}
	}
	return terms
}

func (idx *Index) Stop() {
	idx.cache.stop()
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
	// Split on whitespace only so any special-char token ("prod-web-01", "192.168.1.1", etc.) is preserved whole.
	words := strings.Fields(text)
	seen := make(map[string]struct{}, len(words)*2)
	result := make([]string, 0, len(words)*2)
	add := func(t string) {
		if t == "" {
			return
		}
		if _, ok := seen[t]; !ok {
			seen[t] = struct{}{}
			result = append(result, t)
		}
	}
	for _, w := range words {
		add(w) // keep full token: "prod-web-01", "10.0.0.1", "some_key:value", etc.
		// Also emit parts split on all non-alphanumeric chars so individual pieces are searchable too.
		parts := strings.FieldsFunc(w, func(r rune) bool {
			return !unicode.IsLetter(r) && !unicode.IsDigit(r)
		})
		if len(parts) > 1 {
			for _, p := range parts {
				add(p)
			}
		}
	}
	return result
}

func isStopword(w string) bool { _, ok := stopwords[w]; return ok }
