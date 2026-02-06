package monitor

import (
	"bandwidth-monitor/config"
	"bandwidth-monitor/sshclient"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"sync"
	"time"
)

// TrafficBucket represents a standard traffic data point in vnStat 2.12+
type TrafficBucket struct {
	ID        int    `json:"id"`
	Timestamp int64  `json:"timestamp"`
	Rx        uint64 `json:"rx"`
	Tx        uint64 `json:"tx"`
	Date struct {
		Year  int `json:"year"`
		Month int `json:"month"`
		Day   int `json:"day"`
	} `json:"date"`
	Time struct {
		Hour   int `json:"hour"`
		Minute int `json:"minute"`
	} `json:"time"`
}

// VnStatData represents vnStat JSON output structure (v2.12+)
type VnStatData struct {
	VnStatVersion string `json:"vnstatversion"`
	JsonVersion   string `json:"jsonversion"`
	Interfaces []struct {
		Name    string `json:"name"`
		Alias   string `json:"alias"`
		Created struct {
			Timestamp int64 `json:"timestamp"`
		} `json:"created"`
		Updated struct {
			Timestamp int64 `json:"timestamp"`
		} `json:"updated"`
		Traffic struct {
			Total struct {
				Rx uint64 `json:"rx"`
				Tx uint64 `json:"tx"`
			} `json:"total"`
			FiveMinute []TrafficBucket `json:"fiveminute"`
			Hour       []TrafficBucket `json:"hour"`
			Day        []TrafficBucket `json:"day"`
			Month      []TrafficBucket `json:"month"`
			Top        []TrafficBucket `json:"top"`
		} `json:"traffic"`
	} `json:"interfaces"`
}

// GetUpdatedTime parses the Updated field into a time.Time
func (v *VnStatData) GetUpdatedTime() time.Time {
	if len(v.Interfaces) == 0 {
		return time.Time{}
	}
	return time.Unix(v.Interfaces[0].Updated.Timestamp, 0).UTC()
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
	AvgRx12h   uint64
	AvgTx12h   uint64
	AvgRx24h   uint64
	AvgTx24h   uint64
	PeakRx     uint64 // Max observed speed in last 24h
	PeakTx     uint64 // Max observed speed in last 24h
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
	config       *config.Config
	privateKey   []byte
	metrics      *AggregateMetrics
	mu           sync.RWMutex
	stopChan     chan struct{}
	pollInterval time.Duration
	historyLimit int
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
		config:     cfg,
		privateKey: []byte(privateKeyStr),
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
func (m *Monitor) processVnStatData(server config.ServerConfig, vnstat *VnStatData) *ServerMetrics {
	metrics := &ServerMetrics{
		Name:      server.Name,
		IP:        server.IP,
		Online:    true,
		UpdatedAt: time.Now(),
	}

	if len(vnstat.Interfaces) > 0 {
		iface := vnstat.Interfaces[0]
		now := time.Now().UTC()

		// Get today's total
		// Find the day entry that matches today
		// (Assuming the last entry in Day array is usually today, but we can check timestamp if needed.
		// For simplicity and standard vnStat behavior, the list usually ends with current data,
		// but let's look for the one with highest timestamp just to be safe or just take the last one)
		if len(iface.Traffic.Day) > 0 {
			// Sort days by timestamp just in case
			sort.Slice(iface.Traffic.Day, func(i, j int) bool {
				return iface.Traffic.Day[i].Timestamp < iface.Traffic.Day[j].Timestamp
			})
			today := iface.Traffic.Day[len(iface.Traffic.Day)-1]
			metrics.TotalRx = today.Rx
			metrics.TotalTx = today.Tx
		}

		// Calculate real-time speed from FiveMinute data
		if len(iface.Traffic.FiveMinute) > 0 {
			// Sort by timestamp DESCENDING (newest first)
			// Copy slice to avoid modifying original if we needed it elsewhere, but here it's fine.
			// Actually let's just sort the slice in place.
			buckets := iface.Traffic.FiveMinute
			sort.Slice(buckets, func(i, j int) bool {
				return buckets[i].Timestamp > buckets[j].Timestamp
			})

			// Take the latest bucket
			latest := buckets[0]

			// Calculate speed: Volume / 300 seconds
			metrics.Rx = latest.Rx / 300
			metrics.Tx = latest.Tx / 300
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
			ts := time.Unix(h.Timestamp, 0).UTC()
			age := now.Sub(ts)

			// Calculate 24h average usage
			// Using 24h window
			if age <= 24*time.Hour && age >= 0 {
				sumRx24 += h.Rx
				sumTx24 += h.Tx
				count24++

				rateRx := h.Rx / 3600
				rateTx := h.Tx / 3600

				if rateRx > peakRx {
					peakRx = rateRx
				}
				if rateTx > peakTx {
					peakTx = rateTx
				}

				hours = append(hours, hourTraffic{
					t:     ts,
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
		sort.Slice(hours, func(i, j int) bool {
			return hours[i].total > hours[j].total
		})

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
