package convert

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"time"

	"backend/pkg/config"
	"github.com/xitongsys/parquet-go-source/local"
	"github.com/xitongsys/parquet-go/reader"
)

type Record map[string]interface{}

func Run(cfg config.ConvertConfig) error {
	files, err := filepath.Glob(cfg.Pattern)
	if err != nil {
		return fmt.Errorf("failed to glob files: %w", err)
	}

	if len(files) == 0 {
		return fmt.Errorf("no files matching %s — place Parquet files (File 1, File 2, …) in data/ and re-run", cfg.Pattern)
	}

	sort.Strings(files)
	log.Printf("found %d Parquet file(s)", len(files))

	start := time.Now()
	var records []Record

	for _, path := range files {
		fileRecords, err := processParquetFile(path)
		if err != nil {
			return fmt.Errorf("failed to process %s: %w", path, err)
		}
		records = append(records, fileRecords...)
		log.Printf("%s: %d rows (total: %d)", filepath.Base(path), len(fileRecords), len(records))
	}

	if err := os.MkdirAll(cfg.DataDir, 0755); err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}

	jsonData, err := json.Marshal(records)
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}

	if err := os.WriteFile(cfg.OutFile, jsonData, 0644); err != nil {
		return fmt.Errorf("failed to write output file: %w", err)
	}

	elapsed := time.Since(start)
	sizeMB := float64(len(jsonData)) / 1024 / 1024
	log.Printf("done — %d records → %s (%.1f MB, %.2fs)", len(records), cfg.OutFile, sizeMB, elapsed.Seconds())
	return nil
}

// AppendFile reads a single Parquet file and appends its records to the
// existing records.json in dataDir. Called when a file is uploaded at runtime.
func AppendFile(dataDir, filePath string) error {
	start := time.Now()
	newRecords, err := processParquetFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read %s: %w", filepath.Base(filePath), err)
	}
	if len(newRecords) == 0 {
		return fmt.Errorf("no records found in %s", filepath.Base(filePath))
	}

	outFile := filepath.Join(dataDir, "records.json")

	var existing []Record
	if raw, err := os.ReadFile(outFile); err == nil {
		if err := json.Unmarshal(raw, &existing); err != nil {
			return fmt.Errorf("existing records.json is corrupted: %w", err)
		}
	}

	combined := append(existing, newRecords...)
	jsonData, err := json.Marshal(combined)
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}
	if err := os.WriteFile(outFile, jsonData, 0644); err != nil {
		return fmt.Errorf("failed to write %s: %w", outFile, err)
	}

	elapsed := time.Since(start)
	log.Printf("appended %d records from %s (total: %d, %.2fs)",
		len(newRecords), filepath.Base(filePath), len(combined), elapsed.Seconds())
	return nil
}

func processParquetFile(path string) ([]Record, error) {
	fr, err := local.NewLocalFileReader(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer fr.Close()

	pr, err := reader.NewParquetReader(fr, nil, 4)
	if err != nil {
		return nil, fmt.Errorf("failed to create parquet reader: %w", err)
	}
	defer pr.ReadStop()

	numRows := int(pr.GetNumRows())

	res, err := pr.ReadByNumber(numRows)
	if err != nil {
		return nil, fmt.Errorf("failed to read rows: %w", err)
	}

	records := make([]Record, 0, len(res))
	for _, row := range res {
		// parquet-go returns anonymous structs, not map[string]interface{},
		// so use a JSON round-trip to get a plain map regardless of the row type.
		b, err := json.Marshal(row)
		if err != nil {
			continue
		}
		record := make(Record)
		if err := json.Unmarshal(b, &record); err != nil {
			continue
		}
		records = append(records, record)
	}

	return records, nil
}
