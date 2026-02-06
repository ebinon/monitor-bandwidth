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
	IsTimestamp bool // True if parsed from timestamp (v2.12+), False if from object (Legacy)
}

// UnmarshalJSON implements custom unmarshalling for VnStatTime
// Implemented to support vnStat 2.12+ (int64 timestamp) and legacy (object) formats.
func (vt *VnStatTime) UnmarshalJSON(data []byte) error {
	// 1. Try to unmarshal as a number (timestamp)
	var timestamp int64
	if err := json.Unmarshal(data, &timestamp); err == nil {
		vt.Time = time.Unix(timestamp, 0).UTC()
		vt.IsTimestamp = true
		return nil
	}

	// 2. Try to unmarshal as a legacy object
	vt.IsTimestamp = false
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
				ID VnStatTime `json:"id" description:"vnStat v2.12+ ID (timestamp or object)"`
				Rx uint64     `json:"rx"`
				Tx uint64     `json:"tx"`
			} `json:"month"`
			Day []struct {
				ID VnStatTime `json:"id" description:"vnStat v2.12+ ID (timestamp or object)"`
				Rx uint64     `json:"rx"`
				Tx uint64     `json:"tx"`
			} `json:"day"`
			Hour []struct {
				ID VnStatTime `json:"id" description:"vnStat v2.12+ ID (timestamp or object)"`
				Rx uint64     `json:"rx"`
				Tx uint64     `json:"tx"`
			} `json:"hour"`
			Minute []struct {
				ID VnStatTime `json:"id" description:"vnStat v2.12+ ID (timestamp or object)"`
				Rx uint64     `json:"rx"`
				Tx uint64     `json:"tx"`
			} `json:"minute"`
		} `json:"traffic"`
	} `json:"interfaces"`
}

// GetUpdatedTime parses the Updated field into a time.Time using UTC logic consistent with VnStatTime
func (v *VnStatData) GetUpdatedTime() time.Time {
	if len(v.Interfaces) == 0 {
		return time.Time{}
	}
	updated := v.Interfaces[0].Updated
	return time.Date(
		updated.Date.Year,
		time.Month(updated.Date.Month),
		updated.Date.Day,
		updated.Time.Hour,
		updated.Time.Minute,
		0, 0, time.UTC,
	)
}

// PeakEvent represents a high traffic event
type PeakEvent struct {
	Time time.Time
	Rx   uint64
	Tx   uint64
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
	PeakEvents []PeakEvent // Top 3 peak hours

	UpdatedAt time.Time
	Error     string
}

// AggregateMetrics represents aggregated metrics from all servers
type AggregateMetrics struct {
	TotalRx        uint64
	TotalTx        uint64
	GrandTotalAvg  uint64 // Sum of all servers' Avg24h
	GrandTotalPeak uint64 // Sum of all servers' MaxPeak
	DominantServer string // Name of server with highest usage
	ServerMetrics  map[string]*ServerMetrics
	History        []HistoryEntry
	UpdatedAt      time.Time
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

	// Process metrics using extracted logic
	processedMetrics := m.processVnStatData(server, &vnstat)
	m.setServerMetrics(server.Name, processedMetrics)
}

// processVnStatData processes the parsed vnStat data and returns ServerMetrics.
// It uses adaptive age calculation to handle timezone differences.
func (m *Monitor) processVnStatData(server config.ServerConfig, vnstat *VnStatData) *ServerMetrics {
	metrics := &ServerMetrics{
		Name:      server.Name,
		IP:        server.IP,
		Online:    true,
		UpdatedAt: time.Now(),
	}

	if len(vnstat.Interfaces) > 0 {
		iface := vnstat.Interfaces[0]
		updatedTime := vnstat.GetUpdatedTime()
		now := time.Now().UTC()

		// Get today's total
		if len(iface.Traffic.Day) > 0 {
			today := iface.Traffic.Day[0]
			metrics.TotalRx = today.Rx
			metrics.TotalTx = today.Tx
		}

		// Calculate real-time speed from minute data
		if len(iface.Traffic.Minute) > 0 {
			var latestRx, latestTx uint64
			found := false

			// Scan for latest minute entry (within last 5 mins)
			for _, m := range iface.Traffic.Minute {
				var age time.Duration
				if m.ID.IsTimestamp {
					// Trusted UTC timestamp
					age = now.Sub(m.ID.Time)
				} else {
					// Relative age from updated time (handles timezone offsets)
					age = updatedTime.Sub(m.ID.Time)
				}

				// Allow age up to 5 minutes.
				// Also allow slight negative age (future) for safety, but not too far into future.
				// With updatedTime logic, age should be >= 0 (updated >= ID).
				// But if using 'now' with timestamp, clock skew might cause slight negative.
				// We relax the lower bound to -1 Hour just in case.
				if age < 5*time.Minute && age > -1*time.Hour {
					// Use the rate from this minute
					latestRx = m.Rx / 60
					latestTx = m.Tx / 60
					found = true
				}
			}

			if found {
				metrics.Rx = latestRx
				metrics.Tx = latestTx
			} else {
				// Fallback to legacy logic
				if len(iface.Traffic.Minute) >= 2 {
					latest := iface.Traffic.Minute[0]
					previous := iface.Traffic.Minute[1]
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
		var sumRx12, sumTx12, sumRx24, sumTx24 uint64
		var count12, count24 uint64
		var peakRx, peakTx uint64

		type hourTraffic struct {
			t     time.Time
			rx    uint64
			tx    uint64
			total uint64
		}
		var hours []hourTraffic

		for _, h := range iface.Traffic.Hour {
			var age time.Duration
			if h.ID.IsTimestamp {
				age = now.Sub(h.ID.Time)
			} else {
				age = updatedTime.Sub(h.ID.Time)
			}

			// Calculate 24h average usage
			// Relax lower bound to -1h to account for slight skews or timezone glitches
			if age <= 24*time.Hour && age > -1*time.Hour {
				sumRx24 += h.Rx
				sumTx24 += h.Tx
				count24++

				rateRx := h.Rx / 3600
				rateTx := h.Tx / 3600

				if rateRx > peakRx { peakRx = rateRx }
				if rateTx > peakTx { peakTx = rateTx }

				hours = append(hours, hourTraffic{
					t:     h.ID.Time,
					rx:    rateRx,
					tx:    rateTx,
					total: rateRx + rateTx,
				})

				// Filter for last 12h
				if age <= 12*time.Hour {
					sumRx12 += h.Rx
					sumTx12 += h.Tx
					count12++
				}
			}
		}

		if count12 > 0 {
			metrics.AvgRx12h = sumRx12 / (count12 * 3600)
			metrics.AvgTx12h = sumTx12 / (count12 * 3600)
		}
		if count24 > 0 {
			metrics.AvgRx24h = sumRx24 / (count24 * 3600)
			metrics.AvgTx24h = sumTx24 / (count24 * 3600)
		}

		metrics.PeakRx = peakRx
		metrics.PeakTx = peakTx

		// Find top 3 peak hours
		for i := 0; i < len(hours)-1; i++ {
			for j := 0; j < len(hours)-i-1; j++ {
				if hours[j].total < hours[j+1].total {
					hours[j], hours[j+1] = hours[j+1], hours[j]
				}
			}
		}

		limit := 3
		if len(hours) < 3 {
			limit = len(hours)
		}

		metrics.PeakEvents = make([]PeakEvent, 0, limit)
		for i := 0; i < limit; i++ {
			metrics.PeakEvents = append(metrics.PeakEvents, PeakEvent{
				Time: hours[i].t,
				Rx:   hours[i].rx,
				Tx:   hours[i].tx,
			})
		}
	}

	return metrics
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
			
			var totalRx, totalTx, grandTotalAvg, grandTotalPeak uint64
			var dominantServer string
			var maxUsage uint64
			
			for _, metrics := range m.metrics.ServerMetrics {
				if metrics.Online {
					totalRx += metrics.Rx
					totalTx += metrics.Tx

					// Grand Total Avg = Sum of all servers' (AvgRx24h + AvgTx24h)
					serverAvg := metrics.AvgRx24h + metrics.AvgTx24h
					grandTotalAvg += serverAvg

					// Grand Total Peak = Sum of all servers' MaxPeak (PeakRx or PeakTx)
					// We take the max of Rx/Tx for peak capacity planning
					serverPeak := metrics.PeakRx
					if metrics.PeakTx > serverPeak {
						serverPeak = metrics.PeakTx
					}
					grandTotalPeak += serverPeak

					// Dominant Server (by daily average usage)
					if serverAvg > maxUsage {
						maxUsage = serverAvg
						dominantServer = metrics.Name
					}
				}
			}
			
			m.metrics.TotalRx = totalRx
			m.metrics.TotalTx = totalTx
			m.metrics.GrandTotalAvg = grandTotalAvg
			m.metrics.GrandTotalPeak = grandTotalPeak
			m.metrics.DominantServer = dominantServer
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
		TotalRx:        m.metrics.TotalRx,
		TotalTx:        m.metrics.TotalTx,
		GrandTotalAvg:  m.metrics.GrandTotalAvg,
		GrandTotalPeak: m.metrics.GrandTotalPeak,
		DominantServer: m.metrics.DominantServer,
		ServerMetrics:  make(map[string]*ServerMetrics),
		History:        make([]HistoryEntry, len(m.metrics.History)),
		UpdatedAt:      m.metrics.UpdatedAt,
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
