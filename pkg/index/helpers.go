package index

import (
	"backend/pkg/models"
	"sync"
	"time"
)

const (
	bm25K1         = 1.2             // term-frequency saturation
	bm25B          = 0.75            // length normalisation
	cacheTTL       = 30 * time.Second
	maxPrefixTerms = 50
)

// Index is the in-memory inverted index
type Index struct {
	mu          sync.RWMutex
	inverted    map[string][]posting
	docs        map[int]*models.Record
	sortedTerms []string // for O(log n) prefix search via binary search
	total       int

	// BM25 data
	docFreq     map[string]int         // number of docs that contain each term (IDF input)
	fieldLens   map[int]map[string]int // docID -> field -> token count
	avgFieldLen map[string]float64     // average token count per field across all docs

	cache *queryCache
}

var stopwords = map[string]struct{}{
	"the": {}, "and": {}, "or": {}, "in": {},
	"at": {}, "to": {}, "a": {}, "an": {},
	"is": {}, "it": {}, "of": {}, "for": {},
	"on": {}, "with": {}, "as": {}, "by": {},
	"be": {}, "was": {}, "are": {}, "has": {},
}

type posting struct {
	docID    int
	field    string
	termFreq int
}

// fieldWeight defines per-field BM25 multipliers.
// Higher weight = matches in that field contribute more to the final score.
var fieldWeight = map[string]float64{
	"Message":        5,
	"AppName":        4,
	"Hostname":       4,
	"Tag":            4,
	"SeverityString": 3,
	"FacilityString": 3,
	"Sender":         3,
	"StructuredData": 2,
	"MessageRaw":     2,
	"Namespace":      2,
	"ProcId":         1,
	"Groupings":      1,
	"Event":          1,
}

// --- query result cache ---

type cacheEntry struct {
	results []models.SearchResult
	created time.Time
}

type queryCache struct {
	mu      sync.RWMutex
	entries map[string]cacheEntry
	ttl     time.Duration
	done    chan struct{}
}

func newQueryCache(ttl time.Duration) *queryCache {
	c := &queryCache{
		entries: make(map[string]cacheEntry),
		ttl:     ttl,
		done:    make(chan struct{}),
	}
	go c.clean()
	return c
}

// clean runs in a goroutine and removes expired entries every ttl interval.
func (c *queryCache) clean() {
	ticker := time.NewTicker(c.ttl)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			c.mu.Lock()
			for key, e := range c.entries {
				if time.Since(e.created) > c.ttl {
					delete(c.entries, key)
				}
			}
			c.mu.Unlock()
		case <-c.done:
			return
		}
	}
}

// stop shuts down the background cleaner goroutine.
func (c *queryCache) stop() {
	close(c.done)
}

func (c *queryCache) get(key string) ([]models.SearchResult, bool) {
	c.mu.RLock()
	e, ok := c.entries[key]
	c.mu.RUnlock()
	if !ok || time.Since(e.created) > c.ttl {
		return nil, false
	}
	return e.results, true
}

func (c *queryCache) set(key string, results []models.SearchResult) {
	c.mu.Lock()
	c.entries[key] = cacheEntry{results: results, created: time.Now()}
	c.mu.Unlock()
}
