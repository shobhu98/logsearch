package api

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"backend/pkg/config"
	"backend/pkg/convert"
	"backend/pkg/index"
	"backend/pkg/models"
	"backend/pkg/parser"
)

// Handler holds all dependencies for the API
type Handler struct {
	mu         sync.RWMutex
	rebuildMu  sync.Mutex // serialises concurrent rebuild/upload requests
	idx        *index.Index
	dataDir    string
	ready      bool
	rebuilding bool // true while a background index rebuild is in progress
	numWorkers int
	ctx        context.Context
	search     config.SearchConfig
}

// New creates a new Handler and builds the index from dataDir
func New(ctx context.Context, dataDir string, numWorkers int, searchCfg config.SearchConfig) (*Handler, error) {
	h := &Handler{ctx: ctx, dataDir: dataDir, numWorkers: numWorkers, search: searchCfg}
	if err := h.buildIndex(); err != nil {
		return nil, err
	}
	return h, nil
}

// buildIndex loads records and rebuilds the index — used at startup and on reload
func (h *Handler) buildIndex() error {
	start := time.Now()
	log.Println("[index] Loading records from", h.dataDir)

	records, err := parser.LoadFromJSON(h.dataDir)
	if err != nil {
		return fmt.Errorf("failed to load records: %w", err)
	}
	log.Printf("[index] Loaded %d records in %v", len(records), time.Since(start))

	buildStart := time.Now()
	idx := index.New()
	if err := idx.Build(h.ctx, records, h.numWorkers); err != nil {
		return fmt.Errorf("index build cancelled: %w", err)
	}
	log.Printf("[index] Built inverted index in %v", time.Since(buildStart))

	stats := idx.Stats()
	log.Printf("[index] Stats: %d docs, %d unique terms", stats["total_docs"], stats["total_terms"])

	h.mu.Lock()
	oldIdx := h.idx
	h.idx = idx
	h.ready = true
	h.mu.Unlock()

	if oldIdx != nil {
		oldIdx.Stop()
	}

	return nil
}

// gzPool reuses gzip writers across requests to avoid per-request allocations.
var gzPool = sync.Pool{
	New: func() any {
		gz, _ := gzip.NewWriterLevel(io.Discard, gzip.BestSpeed)
		return gz
	},
}

type gzipWriter struct {
	http.ResponseWriter
	gz *gzip.Writer
}

func (g *gzipWriter) Write(b []byte) (int, error) { return g.gz.Write(b) }

// gzipMiddleware compresses responses for clients that advertise Accept-Encoding: gzip.
func gzipMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			next.ServeHTTP(w, r)
			return
		}
		gz := gzPool.Get().(*gzip.Writer)
		gz.Reset(w)
		defer func() {
			gz.Close()
			gzPool.Put(gz)
		}()
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Set("Vary", "Accept-Encoding")
		next.ServeHTTP(&gzipWriter{ResponseWriter: w, gz: gz}, r)
	})
}

// RegisterRoutes sets up all HTTP routes
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	gz := gzipMiddleware
	mux.Handle("/health", gz(http.HandlerFunc(h.handleHealth)))
	mux.Handle("/api/search", gz(http.HandlerFunc(h.handleSearch)))
	mux.Handle("/api/stats", gz(http.HandlerFunc(h.handleStats)))
	mux.Handle("/api/reload", gz(http.HandlerFunc(h.handleReload)))
	mux.Handle("/api/upload", gz(http.HandlerFunc(h.handleUpload)))
}

// handleHealth returns service status
func (h *Handler) handleHealth(w http.ResponseWriter, r *http.Request) {
	setCORSHeaders(w)
	h.mu.RLock()
	ready := h.ready
	rebuilding := h.rebuilding
	h.mu.RUnlock()

	status := "ok"
	code := http.StatusOK
	if rebuilding {
		status = "rebuilding"
	} else if !ready {
		status = "loading"
		code = http.StatusServiceUnavailable
	}

	writeJSON(w, code, map[string]any{"status": status, "rebuilding": rebuilding})
}

// handleSearch is the main search endpoint
// GET /api/search?q=<query>&limit=<n>&offset=<n>
func (h *Handler) handleSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, errorResponse("method not allowed"))
		return
	}

	// CORS for React dev server
	setCORSHeaders(w)

	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse("query parameter 'q' is required"))
		return
	}

	limit := h.search.DefaultLimit
	offset := h.search.DefaultOffset
	if l := r.URL.Query().Get("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 {
			limit = min(v, h.search.MaxLimit)
		}
	}
	if o := r.URL.Query().Get("offset"); o != "" {
		if v, err := strconv.Atoi(o); err == nil && v >= 0 {
			offset = v
		}
	}

	h.mu.RLock()
	idx := h.idx
	h.mu.RUnlock()

	results, timeTakenMs := idx.Search(query)

	// apply pagination
	total := len(results)
	if offset >= total {
		results = []models.SearchResult{}
	} else {
		end := offset + limit
		if end > total {
			end = total
		}
		results = results[offset:end]
	}

	writeJSON(w, http.StatusOK, models.SearchResponse{
		Query:       query,
		Total:       total,
		TimeTakenMs: timeTakenMs,
		Results:     results,
	})
}

// handleStats returns index statistics
func (h *Handler) handleStats(w http.ResponseWriter, r *http.Request) {
	setCORSHeaders(w)
	h.mu.RLock()
	idx := h.idx
	h.mu.RUnlock()

	writeJSON(w, http.StatusOK, idx.Stats())
}

// handleReload reloads the index from disk.
func (h *Handler) handleReload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, errorResponse("method not allowed"))
		return
	}
	setCORSHeaders(w)

	log.Println("[reload] Triggered index reload")
	h.mu.Lock()
	h.rebuilding = true
	h.mu.Unlock()

	go func() {
		h.rebuildMu.Lock()
		defer h.rebuildMu.Unlock()
		if err := h.buildIndex(); err != nil {
			log.Printf("[reload] Failed: %v", err)
		} else {
			log.Println("[reload] Index reloaded successfully")
		}
		h.mu.Lock()
		h.rebuilding = false
		h.mu.Unlock()
	}()

	writeJSON(w, http.StatusAccepted, map[string]string{"status": "reload started"})
}

// handleUpload accepts a Parquet file, saves it to dataDir, appends its
// records to records.json, and rebuilds the index — all without restarting.
// POST /api/upload  (multipart/form-data, field name "file")
func (h *Handler) handleUpload(w http.ResponseWriter, r *http.Request) {
	setCORSHeaders(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, errorResponse("method not allowed"))
		return
	}

	if err := r.ParseMultipartForm(100 << 20); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse("failed to parse form: "+err.Error()))
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse("field 'file' missing"))
		return
	}
	defer file.Close()

	// sanitise filename — no path traversal, null bytes, or non-parquet uploads
	if strings.ContainsRune(header.Filename, 0) {
		writeJSON(w, http.StatusBadRequest, errorResponse("invalid filename"))
		return
	}
	safeName := filepath.Base(filepath.Clean(header.Filename))
	if safeName == "." || !strings.HasSuffix(strings.ToLower(safeName), ".parquet") {
		writeJSON(w, http.StatusBadRequest, errorResponse("only .parquet files are accepted"))
		return
	}
	destPath := filepath.Join(h.dataDir, safeName)

	dest, err := os.Create(destPath)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse("failed to save file"))
		return
	}
	if _, err := io.Copy(dest, file); err != nil {
		dest.Close()
		writeJSON(w, http.StatusInternalServerError, errorResponse("failed to write file"))
		return
	}
	dest.Close()

	log.Printf("[upload] Saved %s (%d bytes)", safeName, header.Size)

	h.mu.Lock()
	h.rebuilding = true
	h.mu.Unlock()

	go func() {
		h.rebuildMu.Lock()
		defer h.rebuildMu.Unlock()

		if err := convert.AppendFile(h.dataDir, destPath); err != nil {
			log.Printf("[upload] AppendFile failed: %v", err)
		} else if err := h.buildIndex(); err != nil {
			log.Printf("[upload] Index rebuild failed: %v", err)
		} else {
			log.Printf("[upload] Index rebuilt after adding %s", safeName)
		}
		h.mu.Lock()
		h.rebuilding = false
		h.mu.Unlock()
	}()

	writeJSON(w, http.StatusAccepted, map[string]string{
		"status": "processing",
		"file":   safeName,
	})
}

// --- helpers ---

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("failed to write response: %v", err)
	}
}

func errorResponse(msg string) map[string]string {
	return map[string]string{"error": msg}
}

func setCORSHeaders(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
}
