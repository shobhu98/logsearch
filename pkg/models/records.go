package models

// SearchResult wraps a record with a relevance score
type SearchResult struct {
	Record *Record `json:"record"`
	Score  int     `json:"score"` // how many fields matched
}

// SearchResponse is what the API returns
type SearchResponse struct {
	Query       string         `json:"query"`
	Total       int            `json:"total"`
	TimeTakenMs float64        `json:"time_taken_ms"`
	Results     []SearchResult `json:"results"`
}

// Record represents a single log entry from the parquet files
type Record struct {
	ID             int    `json:"id"` // internal doc ID for the index
	MsgId          string `json:"MsgId"`
	PartitionId    string `json:"PartitionId"`
	Timestamp      string `json:"Timestamp"`
	Hostname       string `json:"Hostname"`
	Priority       string `json:"Priority"`
	Facility       string `json:"Facility"`
	FacilityString string `json:"FacilityString"`
	Severity       string `json:"Severity"`
	SeverityString string `json:"SeverityString"`
	AppName        string `json:"AppName"`
	ProcId         string `json:"ProcId"`
	Message        string `json:"Message"`
	MessageRaw     string `json:"MessageRaw"`
	StructuredData string `json:"StructuredData"`
	Tag            string `json:"Tag"`
	Sender         string `json:"Sender"`
	Groupings      string `json:"Groupings"`
	Event          string `json:"Event"`
	EventId        string `json:"EventId"`
	NanoTimeStamp  string `json:"NanoTimeStamp"`
	Namespace      string `json:"namespace"`
}
