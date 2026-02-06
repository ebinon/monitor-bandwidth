package monitor

import (
	"bandwidth-monitor/config"
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

// TestProcessVnStatData_TimezoneCompatibility verifies that the adaptive logic
// correctly handles data that appears to be in the future (due to timezone differences).
func TestProcessVnStatData_TimezoneCompatibility(t *testing.T) {
	// 1. Setup Mock Data
	// Scenario: Server in UTC+3.
	// Real Time (Monitor): T (UTC)
	// Server Time (Updated): T + 3h (Local)
	// Data Time: T + 3h (Local)

	// We align relative to actual time.Now() because the code uses time.Now() internally.
	realNow := time.Now().UTC()

	// "Future" time (+3 hours)
	futureTime := realNow.Add(3 * time.Hour)

	// Construct JSON using Legacy Object Format (Local Time)
	// This forces ID.IsTimestamp = false
	// And creates ID.Time = futureTime (interpreted as UTC by Unmarshal)
	vnStatJSON := createLegacyVnStatJSON(futureTime, futureTime)

	var vnstat VnStatData
	if err := json.Unmarshal([]byte(vnStatJSON), &vnstat); err != nil {
		t.Fatalf("Failed to unmarshal JSON: %v", err)
	}

	// Verify IsTimestamp is false
	if len(vnstat.Interfaces) > 0 && len(vnstat.Interfaces[0].Traffic.Minute) > 0 {
		if vnstat.Interfaces[0].Traffic.Minute[0].ID.IsTimestamp {
			t.Fatal("Expected IsTimestamp=false for legacy object format")
		}
	} else {
		t.Fatal("Failed to parse minute data structure")
	}

	// 2. Run Process
	m := &Monitor{}
	serverConfig := config.ServerConfig{
		Name: "Test Server",
		IP:   "1.2.3.4",
	}

	// Calling the method under test
	metrics := m.processVnStatData(serverConfig, &vnstat)

	// 3. Assertions

	// Check Live RX/TX
	// We put 6000 bytes in 1 minute -> 100 B/s
	if metrics.Rx != 100 {
		t.Errorf("Rx mismatch. Got %d, want 100. (Zero means data was rejected)", metrics.Rx)
	}

	// Check AvgRx24h
	// We put 360000 bytes in 1 hour -> 100 B/s
	if metrics.AvgRx24h != 100 {
		t.Errorf("AvgRx24h mismatch. Got %d, want 100. (Zero means data was rejected)", metrics.AvgRx24h)
	}

	if metrics.Rx > 0 && metrics.AvgRx24h > 0 {
		t.Log("Success: Future data was accepted due to adaptive age calculation.")
	}
}

// createLegacyVnStatJSON creates a JSON string mimicking vnStat legacy output
func createLegacyVnStatJSON(updated time.Time, data time.Time) string {
	jsonTmpl := `{
		"vnstatversion": "2.6",
		"interfaces": [{
			"id": "eth0",
			"updated": {
				"date": {"year": %d, "month": %d, "day": %d},
				"time": {"hour": %d, "minute": %d}
			},
			"traffic": {
				"hour": [
					{
						"id": {"year": %d, "month": %d, "day": %d, "hour": %d},
						"rx": 360000,
						"tx": 360000
					}
				],
				"minute": [
					{
						"id": {"year": %d, "month": %d, "day": %d, "hour": %d, "minute": %d},
						"rx": 6000,
						"tx": 6000
					}
				]
			}
		}]
	}`

	return fmt.Sprintf(jsonTmpl,
		updated.Year(), updated.Month(), updated.Day(), updated.Hour(), updated.Minute(),
		data.Year(), data.Month(), data.Day(), data.Hour(),
		data.Year(), data.Month(), data.Day(), data.Hour(), data.Minute(),
	)
}
