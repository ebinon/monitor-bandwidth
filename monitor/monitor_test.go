package monitor

import (
	"bandwidth-monitor/config"
	"encoding/json"
	"testing"
	"time"
)

// TestVnStatParsing verifies that vnStat 2.12+ JSON is parsed correctly.
func TestVnStatParsing(t *testing.T) {
	// Sample JSON from vnStat 2.12 (provided in prompt)
	jsonData := `{
	  "vnstatversion": "2.12",
	  "jsonversion": "2",
	  "interfaces": [
	    {
	      "name": "eth0",
	      "alias": "",
	      "created": {
	        "date": { "year": 2026, "month": 2, "day": 6 },
	        "timestamp": 1770387362
	      },
	      "updated": {
	        "date": { "year": 2026, "month": 2, "day": 6 },
	        "time": { "hour": 20, "minute": 30 },
	        "timestamp": 1770409800
	      },
	      "traffic": {
	        "total": { "rx": 60747498442, "tx": 70868773957 },
	        "fiveminute": [
	          {
	            "id": 4,
	            "date": { "year": 2026, "month": 2, "day": 6 },
	            "time": { "hour": 14, "minute": 15 },
	            "timestamp": 1770387300,
	            "rx": 638411858,
	            "tx": 723348641
	          },
	          {
	            "id": 3,
	            "date": { "year": 2026, "month": 2, "day": 6 },
	            "time": { "hour": 14, "minute": 20 },
	            "timestamp": 1770387600,
	            "rx": 699685918,
	            "tx": 720640368
	          }
	        ],
	        "hour": [
	          {
	            "id": 2,
	            "date": { "year": 2026, "month": 2, "day": 6 },
	            "time": { "hour": 14, "minute": 0 },
	            "timestamp": 1770386400,
	            "rx": 7274739075,
	            "tx": 8253464512
	          }
	        ],
	        "day": [
	          {
	            "id": 2,
	            "date": { "year": 2026, "month": 2, "day": 6 },
	            "timestamp": 1770336000,
	            "rx": 60747498442,
	            "tx": 70868773957
	          }
	        ]
	      }
	    }
	  ]
	}`

	var data VnStatData
	if err := json.Unmarshal([]byte(jsonData), &data); err != nil {
		t.Fatalf("Failed to unmarshal JSON: %v", err)
	}

	if len(data.Interfaces) == 0 {
		t.Fatal("No interfaces found")
	}
	iface := data.Interfaces[0]

	// Verify FiveMinute
	if len(iface.Traffic.FiveMinute) != 2 {
		t.Errorf("Expected 2 FiveMinute entries, got %d", len(iface.Traffic.FiveMinute))
	}
	// Verify one bucket details
	bucket := iface.Traffic.FiveMinute[1] // The second one in the list (ID 3, Timestamp 1770387600)
	if bucket.ID != 3 {
		t.Errorf("Expected bucket ID 3, got %d", bucket.ID)
	}
	if bucket.Timestamp != 1770387600 {
		t.Errorf("Expected timestamp 1770387600, got %d", bucket.Timestamp)
	}

	// Verify logic to process this data
	m := &Monitor{}

	// Test Live Speed calculation
	metrics := m.processVnStatData(config.ServerConfig{}, &data)

	// Expectation:
	// Latest bucket is ID 3 (Timestamp 1770387600) vs ID 4 (Timestamp 1770387300).
	// 1770387600 > 1770387300. So ID 3 is latest.
	// Rx = 699685918 / 300 = 2332286
	// Tx = 720640368 / 300 = 2402134

	if metrics.Rx != 2332286 {
		t.Errorf("Live Rx calculation failed. Got %d, want 2332286", metrics.Rx)
	}
	if metrics.Tx != 2402134 {
		t.Errorf("Live Tx calculation failed. Got %d, want 2402134", metrics.Tx)
	}
}

// TestMetricCalculation verifies the logic used for calculating averages and peaks
func TestMetricCalculation(t *testing.T) {
	// Construct sample data relative to NOW so it passes the age check
	now := time.Now().UTC()

	var hourBuckets []TrafficBucket

	// 1. Generate 25 hours of data
	// Base rate: 3600 bytes/hour = 1 byte/sec
	baseRx := uint64(3600)
	baseTx := uint64(7200) // 2 bytes/sec

	// Peak rate: 36000 bytes/hour = 10 bytes/sec
	peakRx := uint64(36000)
	peakTx := uint64(72000) // 20 bytes/sec

	for i := 0; i < 25; i++ {
		ts := now.Add(time.Duration(-i) * time.Hour).Unix()

		rx := baseRx
		tx := baseTx

		// Inject peak at 2 hours ago
		if i == 2 {
			rx = peakRx
			tx = peakTx
		}

		hourBuckets = append(hourBuckets, TrafficBucket{
			ID: i,
			Timestamp: ts,
			Rx: rx,
			Tx: tx,
		})
	}

	// Create VnStatData
	data := &VnStatData{
		Interfaces: []struct {
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
		}{
			{
				Name: "eth0",
				Traffic: struct {
					Total struct {
						Rx uint64 `json:"rx"`
						Tx uint64 `json:"tx"`
					} `json:"total"`
					FiveMinute []TrafficBucket `json:"fiveminute"`
					Hour       []TrafficBucket `json:"hour"`
					Day        []TrafficBucket `json:"day"`
					Month      []TrafficBucket `json:"month"`
					Top        []TrafficBucket `json:"top"`
				}{
					Hour: hourBuckets,
				},
			},
		},
	}

	m := &Monitor{}
	metrics := m.processVnStatData(config.ServerConfig{}, data)

	// Assertions

	// 1. Peak Check
	// Expected Peak Rx = 10 (36000 / 3600)
	// Expected Peak Tx = 20 (72000 / 3600)
	if metrics.PeakRx != 10 {
		t.Errorf("PeakRx mismatch. Got %d, want 10", metrics.PeakRx)
	}
	if metrics.PeakTx != 20 {
		t.Errorf("PeakTx mismatch. Got %d, want 20", metrics.PeakTx)
	}

	// 2. Average Check (24h)
	// Data points: 0 to 24 (25 points).
	// Point 0 (Now) -> Age 0. Included.
	// Point 24 (Now - 24h) -> Age 24h. Included.
	// Total 25 points.
	// Volume Sum = 24 * 3600 + 1 * 36000 = 86400 + 36000 = 122400
	// Count = 25
	// Avg = 122400 / (25 * 3600) = 122400 / 90000 = 1.36
	// Integer math: 1

	if metrics.AvgRx24h != 1 {
		t.Errorf("AvgRx24h mismatch. Got %d, want 1", metrics.AvgRx24h)
	}

	// Top Peaks List
	if len(metrics.PeakEvents) != 3 {
		t.Errorf("Expected 3 peak events, got %d", len(metrics.PeakEvents))
	}
	// The top one should be the peak we injected (index 2, 2 hours ago)
	top := metrics.PeakEvents[0]
	// Check rate
	if top.Rx != 10 {
		t.Errorf("Top peak event Rx mismatch. Got %d, want 10", top.Rx)
	}
}
