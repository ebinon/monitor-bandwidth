package dashboard

import (
	"bandwidth-monitor/monitor"
	"embed"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

//go:embed static/*
var staticFiles embed.FS

const staticIndexPath = "static/index.html"

// Dashboard represents the web dashboard server
type Dashboard struct {
	monitor   *monitor.Monitor
	server    *http.Server
	username  string
	password  string
	authEnabled bool
}

// APIResponse represents a standard API response
type APIResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
}

// MetricsAPIResponse represents the metrics API response
type MetricsAPIResponse struct {
	TotalRx        uint64                       `json:"totalRx"`
	TotalTx        uint64                       `json:"totalTx"`
	GrandTotalAvg  uint64                       `json:"grandTotalAvg"`
	GrandTotalPeak uint64                       `json:"grandTotalPeak"`
	DominantServer string                       `json:"dominantServer"`
	Servers        map[string]*ServerMetricData `json:"servers"`
	History        []HistoryEntryData           `json:"history"`
	UpdatedAt      time.Time                    `json:"updatedAt"`
}

// PeakEventData represents a peak event for API
type PeakEventData struct {
	Time string `json:"time"`
	Rx   uint64 `json:"rx"`
	Tx   uint64 `json:"tx"`
}

// ServerMetricData represents server metric data for API
type ServerMetricData struct {
	Name       string          `json:"name"`
	IP         string          `json:"ip"`
	Online     bool            `json:"online"`
	Rx         uint64          `json:"rx"`
	Tx         uint64          `json:"tx"`
	TotalRx    uint64          `json:"totalRx"`
	TotalTx    uint64          `json:"totalTx"`
	AvgRx24h   uint64          `json:"avgRx24h"`
	AvgTx24h   uint64          `json:"avgTx24h"`
	PeakRx     uint64          `json:"peakRx"`
	PeakTx     uint64          `json:"peakTx"`
	PeakEvents []PeakEventData `json:"peakEvents"`
	UpdatedAt  time.Time       `json:"updatedAt"`
	Error      string          `json:"error,omitempty"`
}

// HistoryEntryData represents history entry for API
type HistoryEntryData struct {
	Timestamp int64  `json:"timestamp"`
	TotalRx   uint64 `json:"totalRx"`
	TotalTx   uint64 `json:"totalTx"`
}

// NewDashboard creates a new dashboard instance
func NewDashboard(m *monitor.Monitor, port int, username, password string, authEnabled bool) *Dashboard {
	return &Dashboard{
		monitor:    m,
		username:   username,
		password:   password,
		authEnabled: authEnabled,
		server: &http.Server{
			Addr:         fmt.Sprintf(":%d", port),
			ReadTimeout:  15 * time.Second,
			WriteTimeout: 15 * time.Second,
			IdleTimeout:  60 * time.Second,
		},
	}
}

// Start starts the dashboard server
func (d *Dashboard) Start() error {
	// Setup routes
	mux := http.NewServeMux()
	
	// Apply caching middleware and basic auth to all routes
	mux.HandleFunc("/", d.noCache(d.basicAuth(d.indexHandler)))
	mux.HandleFunc("/api/metrics", d.noCache(d.basicAuth(d.metricsHandler)))
	mux.HandleFunc("/api/servers", d.noCache(d.basicAuth(d.serversHandler)))
	
	d.server.Handler = mux
	
	log.Printf("Dashboard starting on %s", d.server.Addr)
	if d.authEnabled {
		log.Printf("HTTP Basic Auth enabled (user: %s)", d.username)
	} else {
		log.Println("HTTP Basic Auth disabled")
	}
	
	return d.server.ListenAndServe()
}

// Stop stops the dashboard server
func (d *Dashboard) Stop() error {
	if d.server != nil {
		return d.server.Close()
	}
	return nil
}

// indexHandler serves the main dashboard page
func (d *Dashboard) indexHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	
	content, err := staticFiles.ReadFile(staticIndexPath)
	if err != nil {
		http.Error(w, "Failed to load dashboard", http.StatusInternalServerError)
		log.Printf("Error reading index.html: %v", err)
		return
	}
	
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(content)
}

// metricsHandler handles the /api/metrics endpoint
func (d *Dashboard) metricsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		d.writeJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	
	metrics := d.monitor.GetMetrics()
	
	// Convert to API response
	response := MetricsAPIResponse{
		TotalRx:        metrics.TotalRx,
		TotalTx:        metrics.TotalTx,
		GrandTotalAvg:  metrics.GrandTotalAvg,
		GrandTotalPeak: metrics.GrandTotalPeak,
		DominantServer: metrics.DominantServer,
		Servers:        make(map[string]*ServerMetricData),
		History:        make([]HistoryEntryData, len(metrics.History)),
		UpdatedAt:      metrics.UpdatedAt,
	}
	
	// Convert server metrics
	for name, sm := range metrics.ServerMetrics {
		peakEvents := make([]PeakEventData, len(sm.PeakEvents))
		for i, pe := range sm.PeakEvents {
			peakEvents[i] = PeakEventData{
				Time: pe.Time.Format("15:04"), // Format HH:MM
				Rx:   pe.Rx,
				Tx:   pe.Tx,
			}
		}

		response.Servers[name] = &ServerMetricData{
			Name:       sm.Name,
			IP:         sm.IP,
			Online:     sm.Online,
			Rx:         sm.Rx,
			Tx:         sm.Tx,
			TotalRx:    sm.TotalRx,
			TotalTx:    sm.TotalTx,
			AvgRx24h:   sm.AvgRx24h,
			AvgTx24h:   sm.AvgTx24h,
			PeakRx:     sm.PeakRx,
			PeakTx:     sm.PeakTx,
			PeakEvents: peakEvents,
			UpdatedAt:  sm.UpdatedAt,
			Error:      sm.Error,
		}
	}
	
	// Convert history
	for i, h := range metrics.History {
		response.History[i] = HistoryEntryData{
			Timestamp: h.Timestamp.Unix(),
			TotalRx:   h.TotalRx,
			TotalTx:   h.TotalTx,
		}
	}
	
	d.writeJSONResponse(w, response)
}

// serversHandler handles the /api/servers endpoint
func (d *Dashboard) serversHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		d.writeJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	
	metrics := d.monitor.GetMetrics()
	
	servers := make([]map[string]interface{}, 0)
	for _, sm := range metrics.ServerMetrics {
		server := map[string]interface{}{
			"name":      sm.Name,
			"ip":        sm.IP,
			"online":    sm.Online,
			"rx":        sm.Rx,
			"tx":        sm.Tx,
			"totalRx":   sm.TotalRx,
			"totalTx":   sm.TotalTx,
			"avgRx24h":  sm.AvgRx24h,
			"avgTx24h":  sm.AvgTx24h,
			"peakRx":    sm.PeakRx,
			"peakTx":    sm.PeakTx,
			"updatedAt": sm.UpdatedAt,
			"error":     sm.Error,
		}
		servers = append(servers, server)
	}
	
	d.writeJSONResponse(w, APIResponse{
		Success: true,
		Data:    servers,
	})
}

// noCache is a middleware that disables caching
func (d *Dashboard) noCache(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")
		next(w, r)
	}
}

// basicAuth wraps a handler with HTTP Basic Auth
func (d *Dashboard) basicAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !d.authEnabled {
			next(w, r)
			return
		}
		
		user, pass, ok := r.BasicAuth()
		if !ok || user != d.username || pass != d.password {
			w.Header().Set("WWW-Authenticate", `Basic realm="Bandwidth Monitor"`)
			d.writeJSONError(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		
		next(w, r)
	}
}

// writeJSONResponse writes a JSON response
func (d *Dashboard) writeJSONResponse(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)
	
	if err := encoder.Encode(APIResponse{
		Success: true,
		Data:    data,
	}); err != nil {
		log.Printf("Error encoding JSON response: %v", err)
	}
}

// writeJSONError writes a JSON error response
func (d *Dashboard) writeJSONError(w http.ResponseWriter, message string, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	
	json.NewEncoder(w).Encode(APIResponse{
		Success: false,
		Error:   message,
	})
}

// FormatBytes formats bytes to human-readable string
func FormatBytes(bytes uint64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	
	div, exp := uint64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	
	return fmt.Sprintf("%.2f %ciB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// FormatSpeed formats bytes per second to human-readable string
func FormatSpeed(bytesPerSec uint64) string {
	return FormatBytes(bytesPerSec) + "/s"
}
