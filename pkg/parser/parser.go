package parser

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"backend/pkg/models"
)

// LoadFromJSON reads the pre-converted records.json file
func LoadFromJSON(dataDir string) ([]models.Record, error) {
	jsonPath := filepath.Join(dataDir, "records.json")

	data, err := os.ReadFile(jsonPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read records.json: %w", err)
	}

	// raw map slice so we can handle mixed types from parquet (int fields stored as strings)
	var raw []map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("failed to parse records.json: %w", err)
	}

	records := make([]models.Record, 0, len(raw))
	for i, r := range raw {
		rec := models.Record{
			ID:             i,
			MsgId:          toString(r["MsgId"]),
			PartitionId:    toString(r["PartitionId"]),
			Timestamp:      toString(r["Timestamp"]),
			Hostname:       toString(r["Hostname"]),
			Priority:       toString(r["Priority"]),
			Facility:       toString(r["Facility"]),
			FacilityString: toString(r["FacilityString"]),
			Severity:       toString(r["Severity"]),
			SeverityString: toString(r["SeverityString"]),
			AppName:        toString(r["AppName"]),
			ProcId:         toString(r["ProcId"]),
			Message:        toString(r["Message"]),
			MessageRaw:     toString(r["MessageRaw"]),
			StructuredData: toString(r["StructuredData"]),
			Tag:            toString(r["Tag"]),
			Sender:         toString(r["Sender"]),
			Groupings:      toString(r["Groupings"]),
			Event:          toString(r["Event"]),
			EventId:        toString(r["EventId"]),
			NanoTimeStamp:  toString(r["NanoTimeStamp"]),
			Namespace:      toString(r["Namespace"]),
		}
		records = append(records, rec)
	}

	return records, nil
}

// toString safely converts interface{} to string
func toString(v interface{}) string {
	if v == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprintf("%v", v))
}
