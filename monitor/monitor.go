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
				ID struct {
					Year  int `json:"year"`
					Month int `json:"month"`
				} `json:"id"`
				Rx uint64 `json:"rx"`
				Tx uint64 `json:"tx"`
			} `json:"month"`
			Day []struct {
				ID struct {
					Year  int `json:"year"`
					Month int `json:"month"`
					Day   int `json:"day"`
				} `json:"id"`
				Rx uint64 `json:"rx"`
				Tx uint64 `json:"tx"`
			} `json:"day"`
			Hour []struct {
				ID struct {
					Date struct {
						Year  int `json:"year"`
						Month int `json:"month"`
						Day   int `json:"day"`
					} `json:"date"`
					Hour int `json:"hour"`
				} `json:"id"`
				Rx uint64 `json:"rx"`
				Tx uint64 `json:"tx"`
			} `json:"hour"`
			Minute []struct {
				ID struct {
					Date struct {
						Year  int `json:"year"`
						Month int `json:"month"`
						Day   int `json:"day"`
					} `json:"date"`
					Hour   int `json:"hour"`
					Minute int `json:"minute"`
				} `json:"id"`
				Rx uint64 `json:"rx"`
				Tx uint64 `json:"tx"`
			} `json:"minute"`
		} `json:"traffic"`
	} `json:"interfaces"`
}

// ServerMetrics represents metrics for a single server
type ServerMetrics struct {
	Name      string
	IP        string
	Online    bool
	Rx        uint64 // Bytes per second
	Tx        uint64 // Bytes per second
	TotalRx   uint64 // Total bytes today
	TotalTx   uint64 // Total bytes today
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
func NewMonitor(cfg *config.Config) (*Monitor, error) {
	// Load SSH private key
	privateKeyStr, err := sshclient.LoadPrivateKey()
	if err != nil {
		return nil, fmt.Errorf("failed to load SSH private key: %w", err)
	}

	return &Monitor{
		config:       cfg,
		privateKey:   []byte(privateKeyStr),
		metrics: &AggregateMetrics{
			ServerMetrics: make(map[string]*ServerMetrics),
			History:       make([]HistoryEntry, 0),
		},
		stopChan:     make(chan struct{}),
		pollInterval: 5 * time.Second,
		historyLimit: 60, // Keep last 5 minutes (60 entries * 5 seconds)
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
		Name:    server.Name,
		IP:      server.IP,
		Online:  false,
		Rx:      0,
		Tx:      0,
		TotalRx: 0,
		TotalTx: 0,
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
			today := iface.Traffic.Day[0]
			metrics.TotalRx = today.Rx
			metrics.TotalTx = today.Tx
		}

		// Calculate real-time speed from minute data
		if len(iface.Traffic.Minute) >= 2 {
			// Get last two minutes to calculate speed
			latest := iface.Traffic.Minute[0]
			previous := iface.Traffic.Minute[1]

			// Calculate bytes per second for the last minute
			// (latest - previous) / 60 seconds
			if latest.Rx > previous.Rx {
				metrics.Rx = (latest.Rx - previous.Rx) / 60
			}
			if latest.Tx > previous.Tx {
				metrics.Tx = (latest.Tx - previous.Tx) / 60
			}
		} else if len(iface.Traffic.Minute) > 0 {
			// Only one minute available, assume it's the current minute
			latest := iface.Traffic.Minute[0]
			metrics.Rx = latest.Rx / 60
			metrics.Tx = latest.Tx / 60
		}
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