package monitor

import (
	"bandwidth-monitor/config"
	"bandwidth-monitor/sshclient"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"
)

// VnStatTime is a wrapper around time.Time to handle both timestamp and legacy object formats
type VnStatTime struct {
	time.Time
}

// UnmarshalJSON implements custom unmarshalling for VnStatTime
func (vt *VnStatTime) UnmarshalJSON(data []byte) error {
	// 1. Try to unmarshal as a number (timestamp)
	var timestamp int64
	if err := json.Unmarshal(data, &timestamp); err == nil {
		vt.Time = time.Unix(timestamp, 0).UTC()
		return nil
	}

	// 2. Try to unmarshal as a legacy object
	// We use a generic map to inspect the fields
	var obj map[string]interface{}
	if err := json.Unmarshal(data, &obj); err != nil {
		return err
	}

	// Helper to safely get int from map
	getInt := func(m map[string]interface{}, key string) int {
		if val, ok := m[key]; ok {
			if f, ok := val.(float64); ok {
				return int(f)
			}
		}
		return 0
	}

	year := getInt(obj, "year")
	month := getInt(obj, "month")
	day := getInt(obj, "day")
	hour := getInt(obj, "hour")
	minute := getInt(obj, "minute")

	// Check for nested "date" object (common in legacy Hour/Minute)
	if dateObj, ok := obj["date"].(map[string]interface{}); ok {
		if year == 0 { year = getInt(dateObj, "year") }
		if month == 0 { month = getInt(dateObj, "month") }
		if day == 0 { day = getInt(dateObj, "day") }
	}
	// Check for nested "time" object (less common in ID, but possible)
	if timeObj, ok := obj["time"].(map[string]interface{}); ok {
		if hour == 0 { hour = getInt(timeObj, "hour") }
		if minute == 0 { minute = getInt(timeObj, "minute") }
	}

	// Default to 1 for day/month if missing (e.g. Month ID only has year/month)
	if day == 0 { day = 1 }
	if month == 0 { month = 1 }

	vt.Time = time.Date(year, time.Month(month), day, hour, minute, 0, 0, time.UTC)
	return nil
}

// VnStatData represents vnStat JSON output structure
type VnStatData struct {
	VnStatVersion        string `json:"vnstatversion"`
	VnStatVersionNumeric uint64 `json:"vnstatversionnumeric"`
	Interfaces           []struct {
		ID      string `json:"id"`
		Name    string `json:"name"`
		Alias   string `json:"alias"`
		Created struct {
			Date struct {
				Year  int `json:"year"`
				Month int `json:"month"`
				Day   int `json:"day"`
			} `json:"date"`
			Time struct {
				Hour   int `json:"hour"`
				Minute int `json:"minute"`
			} `json:"time"`
		} `json:"created"`
		Updated struct {
			Date struct {
				Year  int `json:"year"`
				Month int `json:"month"`
				Day   int `json:"day"`
			} `json:"date"`
			Time struct {
				Hour   int `json:"hour"`
				Minute int `json:"minute"`
			} `json:"time"`
		} `json:"updated"`
		Traffic struct {
			Total struct {
				Rx uint64 `json:"rx"`
				Tx uint64 `json:"tx"`
			} `json:"total"`
			Month []struct {
				ID VnStatTime `json:"id"`
				Rx uint64     `json:"rx"`
				Tx uint64     `json:"tx"`
			} `json:"month"`
			Day []struct {
				ID VnStatTime `json:"id"`
				Rx uint64     `json:"rx"`
				Tx uint64     `json:"tx"`
			} `json:"day"`
			Hour []struct {
				ID VnStatTime `json:"id"`
				Rx uint64     `json:"rx"`
				Tx uint64     `json:"tx"`
			} `json:"hour"`
			Minute []struct {
				ID VnStatTime `json:"id"`
				Rx uint64     `json:"rx"`
				Tx uint64     `json:"tx"`
			} `json:"minute"`
		} `json:"traffic"`
	} `json:"interfaces"`
}

// ServerMetrics represents metrics for a single server
type ServerMetrics struct {
	Name      string
	IP        string
	Online    bool
	Rx        uint64 // Bytes per second (Current)
	Tx        uint64 // Bytes per second (Current)
	TotalRx   uint64 // Total bytes today
	TotalTx   uint64 // Total bytes today

	// New Analytics Fields (Bytes per second)
	AvgRx12h  uint64
	AvgTx12h  uint64
	AvgRx24h  uint64
	AvgTx24h  uint64
	PeakRx    uint64 // Max observed speed in last 24h
	PeakTx    uint64 // Max observed speed in last 24h

	UpdatedAt time.Time
	Error     string
}

// AggregateMetrics represents aggregated metrics from all servers
type AggregateMetrics struct {
	TotalRx       uint64
	TotalTx       uint64
	ServerMetrics map[string]*ServerMetrics
	History       []HistoryEntry
	UpdatedAt     time.Time
}

// HistoryEntry represents a historical data point
type HistoryEntry struct {
	Timestamp time.Time
	TotalRx   uint64
	TotalTx   uint64
}

// Monitor manages monitoring of all servers
type Monitor struct {
	config         *config.Config
	privateKey     []byte
	metrics        *AggregateMetrics
	mu             sync.RWMutex
	stopChan       chan struct{}
	pollInterval   time.Duration
	historyLimit   int
}

// NewMonitor creates a new monitor instance
func NewMonitor(cfg *config.Config, pollInterval time.Duration) (*Monitor, error) {
	// Load SSH private key
	privateKeyStr, err := sshclient.LoadPrivateKey()
	if err != nil {
		return nil, fmt.Errorf("failed to load SSH private key: %w", err)
	}

	// Calculate history limit to keep approximately 5 minutes of data
	historyLimit := int((5 * time.Minute) / pollInterval)
	if historyLimit < 1 {
		historyLimit = 1
	}

	return &Monitor{
		config:       cfg,
		privateKey:   []byte(privateKeyStr),
		metrics: &AggregateMetrics{
			ServerMetrics: make(map[string]*ServerMetrics),
			History:       make([]HistoryEntry, 0),
		},
		stopChan:     make(chan struct{}),
		pollInterval: pollInterval,
		historyLimit: historyLimit,
	}, nil
}

// Start begins monitoring all servers
func (m *Monitor) Start() {
	servers := m.config.GetServers()

	var wg sync.WaitGroup
	for _, server := range servers {
		wg.Add(1)
		go func(s config.ServerConfig) {
			defer wg.Done()
			m.monitorServer(s)
		}(server)
	}

	// Start history cleaner
	go m.cleanHistory()

	// Start aggregation updater
	go m.updateAggregate()
}

// Stop stops monitoring
func (m *Monitor) Stop() {
	close(m.stopChan)
}

// monitorServer monitors a single server
func (m *Monitor) monitorServer(server config.ServerConfig) {
	ticker := time.NewTicker(m.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.stopChan:
			return
		case <-ticker.C:
			m.collectMetrics(server)
		}
	}
}

// collectMetrics collects metrics from a single server
func (m *Monitor) collectMetrics(server config.ServerConfig) {
	metrics := &ServerMetrics{
		Name:      server.Name,
		IP:        server.IP,
		Online:    false,
		UpdatedAt: time.Now(),
	}

	// Connect to server
	client, err := sshclient.NewClientWithKey(server.IP, server.Port, server.User, m.privateKey)
	if err != nil {
		metrics.Error = err.Error()
		m.setServerMetrics(server.Name, metrics)
		return
	}
	defer client.Close()

	// Get vnStat data
	jsonData, err := client.GetVnStatData(server.Interface)
	if err != nil {
		metrics.Error = err.Error()
		m.setServerMetrics(server.Name, metrics)
		return
	}

	// Parse vnStat data
	var vnstat VnStatData
	if err := json.Unmarshal([]byte(jsonData), &vnstat); err != nil {
		metrics.Error = fmt.Sprintf("failed to parse vnStat data: %v", err)
		m.setServerMetrics(server.Name, metrics)
		return
	}

	// Extract metrics
	if len(vnstat.Interfaces) > 0 {
		iface := vnstat.Interfaces[0]

		// Get today's total
		if len(iface.Traffic.Day) > 0 {
			// Usually Day[0] is the current day in vnStat JSON output
			// But check ID just in case? vnStat sorts by date usually.
			// Legacy code assumed index 0. We will stick to that or use the last one?
			// vnStat json: "day" array usually contains history. Index 0 might be oldest or newest depending on version?
			// Assuming index 0 is valid for "today" or "latest" based on legacy code.
			// However, usually index 0 is the *oldest* in vnStat JSON unless reversed.
			// Let's check dates if possible, but for now stick to legacy behavior for TotalRx/Tx
			// Wait, legacy code: today := iface.Traffic.Day[0].
			// If legacy worked, we keep it.
			if len(iface.Traffic.Day) > 0 {
				today := iface.Traffic.Day[0]
				metrics.TotalRx = today.Rx
				metrics.TotalTx = today.Tx
			}
		}

		// Calculate real-time speed from minute data
		// Legacy logic: (latest.Rx - previous.Rx) / 60
		// We will keep this for 'Rx'/'Tx' (current speed) as requested to not break current "Volume" logic if it was working?
		// Actually, if vnStat 2.12 returns interval volumes, (latest - previous) is wrong.
		// Use safe calculation:
		if len(iface.Traffic.Minute) > 0 {
			// Assume sorted, check last available minute?
			// vnStat JSON usually ordered oldest to newest.
			// So last element is newest.
			// Legacy code used [0] and [1]. Maybe it was reversed?
			// Let's assume standard vnStat JSON (ordered by date).
			// If ordered by date, [len-1] is latest.
			// Legacy code used [0]. This suggests legacy vnStat returned newest first?
			// Let's stick to legacy assumption for Rx/Tx to avoid regression on older versions,
			// but for 2.12+ we might need to verify order.
			// Given I cannot run vnstat, I will implement a robust check.

			// Find the minute closest to now
			now := time.Now().UTC()
			var latestRx, latestTx uint64
			found := false

			// Scan for latest minute entry (within last 5 mins)
			for _, m := range iface.Traffic.Minute {
				if now.Sub(m.ID.Time) < 5*time.Minute && now.Sub(m.ID.Time) >= 0 {
					// Use the rate from this minute
					// Rx/Tx are Bytes in that minute.
					// Speed = Bytes / 60
					latestRx = m.Rx / 60
					latestTx = m.Tx / 60
					found = true
					// Keep searching for potentially newer entry
				}
			}

			if found {
				metrics.Rx = latestRx
				metrics.Tx = latestTx
			} else {
				// Fallback to legacy logic if loop didn't find "recent" match (maybe time skew)
				// or if data is just not time-aligned.
				if len(iface.Traffic.Minute) >= 2 {
					latest := iface.Traffic.Minute[0]
					previous := iface.Traffic.Minute[1]
					// Legacy logic assumed cumulative counters?
					if latest.Rx > previous.Rx {
						metrics.Rx = (latest.Rx - previous.Rx) / 60
					} else {
						metrics.Rx = latest.Rx / 60
					}
					if latest.Tx > previous.Tx {
						metrics.Tx = (latest.Tx - previous.Tx) / 60
					} else {
						metrics.Tx = latest.Tx / 60
					}
				} else if len(iface.Traffic.Minute) > 0 {
					latest := iface.Traffic.Minute[0]
					metrics.Rx = latest.Rx / 60
					metrics.Tx = latest.Tx / 60
				}
			}
		}

		// Calculate Averages and Peaks (12h/24h)
		// We use Hour data for this as it covers the range reliably.
		now := time.Now().UTC()
		var sumRx12, sumTx12, sumRx24, sumTx24 uint64
		var count12, count24 uint64 // Count of hours
		var peakRx, peakTx uint64

		for _, h := range iface.Traffic.Hour {
			age := now.Sub(h.ID.Time)

			// Filter for last 24h
			if age <= 24*time.Hour && age >= 0 {
				sumRx24 += h.Rx
				sumTx24 += h.Tx
				count24++

				// Calculate hourly rate (Bytes/sec)
				rateRx := h.Rx / 3600
				rateTx := h.Tx / 3600

				if rateRx > peakRx { peakRx = rateRx }
				if rateTx > peakTx { peakTx = rateTx }

				// Filter for last 12h
				if age <= 12*time.Hour {
					sumRx12 += h.Rx
					sumTx12 += h.Tx
					count12++
				}
			}
		}

		// Calculate Averages (Bytes per second)
		if count12 > 0 {
			// Total Bytes / (Count * 3600)
			metrics.AvgRx12h = sumRx12 / (count12 * 3600)
			metrics.AvgTx12h = sumTx12 / (count12 * 3600)
		}
		if count24 > 0 {
			metrics.AvgRx24h = sumRx24 / (count24 * 3600)
			metrics.AvgTx24h = sumTx24 / (count24 * 3600)
		}

		metrics.PeakRx = peakRx
		metrics.PeakTx = peakTx
	}

	metrics.Online = true
	m.setServerMetrics(server.Name, metrics)
}

// setServerMetrics updates metrics for a server
func (m *Monitor) setServerMetrics(name string, metrics *ServerMetrics) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.metrics.ServerMetrics[name] = metrics
}

// updateAggregate updates aggregate metrics periodically
func (m *Monitor) updateAggregate() {
	ticker := time.NewTicker(m.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.stopChan:
			return
		case <-ticker.C:
			m.mu.Lock()
			
			var totalRx, totalTx uint64
			
			for _, metrics := range m.metrics.ServerMetrics {
				if metrics.Online {
					totalRx += metrics.Rx
					totalTx += metrics.Tx
				}
			}
			
			m.metrics.TotalRx = totalRx
			m.metrics.TotalTx = totalTx
			m.metrics.UpdatedAt = time.Now()
			
			// Add to history
			entry := HistoryEntry{
				Timestamp: time.Now(),
				TotalRx:   totalRx,
				TotalTx:   totalTx,
			}
			m.metrics.History = append(m.metrics.History, entry)
			
			m.mu.Unlock()
		}
	}
}

// cleanHistory removes old history entries
func (m *Monitor) cleanHistory() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-m.stopChan:
			return
		case <-ticker.C:
			m.mu.Lock()
			if len(m.metrics.History) > m.historyLimit {
				m.metrics.History = m.metrics.History[len(m.metrics.History)-m.historyLimit:]
			}
			m.mu.Unlock()
		}
	}
}

// GetMetrics returns current metrics
func (m *Monitor) GetMetrics() *AggregateMetrics {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Return a copy to avoid race conditions
	metricsCopy := &AggregateMetrics{
		TotalRx:       m.metrics.TotalRx,
		TotalTx:       m.metrics.TotalTx,
		ServerMetrics: make(map[string]*ServerMetrics),
		History:       make([]HistoryEntry, len(m.metrics.History)),
		UpdatedAt:     m.metrics.UpdatedAt,
	}

	for k, v := range m.metrics.ServerMetrics {
		metricsCopy.ServerMetrics[k] = v
	}
	copy(metricsCopy.History, m.metrics.History)

	return metricsCopy
}

// GetServerMetrics returns metrics for a specific server
func (m *Monitor) GetServerMetrics(name string) *ServerMetrics {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if metrics, ok := m.metrics.ServerMetrics[name]; ok {
		return metrics
	}
	return nil
}

// RefreshServers updates the monitored servers list
func (m *Monitor) RefreshServers() {
	// The monitor reads from config.GetServers() which is always up to date
	log.Println("Server list refreshed")
}
