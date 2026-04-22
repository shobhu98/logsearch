package index

import (
	"backend/pkg/models"
	"sync"
)

// Index is the in-memory inverted index
type Index struct {
	mu          sync.RWMutex
	inverted    map[string][]posting
	docs        map[int]*models.Record
	sortedTerms []string // for O(log n) prefix search via binary search
	total       int
}

var stopwords = map[string]struct{}{
	"the": {}, "and": {}, "or": {}, "in": {},
	"at": {}, "to": {}, "a": {}, "an": {},
	"is": {}, "it": {}, "of": {}, "for": {},
	"on": {}, "with": {}, "as": {}, "by": {},
	"be": {}, "was": {}, "are": {}, "has": {},
}

// posting holds which document a term appeared in and which field
type posting struct {
	docID int
	field string
}

// fieldWeight defines relevance score contribution per field
var fieldWeight = map[string]int{
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
