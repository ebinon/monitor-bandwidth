package monitor

import (
	"encoding/json"
	"testing"
	"time"
)

// TestVnStatParsing verifies that both legacy (Object) and new (Timestamp) ID formats are parsed correctly.
func TestVnStatParsing(t *testing.T) {
	// 1. Legacy JSON Format (Object IDs)
	// We simulate the structure found in older vnStat versions
	legacyJSON := `{
		"vnstatversion": "2.10",
		"interfaces": [{
			"id": "eth0",
			"traffic": {
				"hour": [
					{
						"id": {
							"date": { "year": 2023, "month": 10, "day": 27 },
							"hour": 14
						},
						"rx": 1000,
						"tx": 2000
					}
				],
				"day": [
					{
						"id": { "year": 2023, "month": 10, "day": 27 },
						"rx": 24000,
						"tx": 48000
					}
				],
				"month": [
					{
						"id": { "year": 2023, "month": 10 },
						"rx": 720000,
						"tx": 1440000
					}
				]
			}
		}]
	}`

	// 2. New JSON Format (Timestamp IDs - vnStat 2.12+)
	// Timestamps:
	// 2023-10-27 14:00:00 UTC = 1698415200
	// 2023-10-27 00:00:00 UTC = 1698364800 (for Day)
	// 2023-10-01 00:00:00 UTC = 1696118400 (for Month)
	newJSON := `{
		"vnstatversion": "2.12",
		"interfaces": [{
			"id": "eth0",
			"traffic": {
				"hour": [
					{
						"id": 1698415200,
						"rx": 1000,
						"tx": 2000
					}
				],
				"day": [
					{
						"id": 1698364800,
						"rx": 24000,
						"tx": 48000
					}
				],
				"month": [
					{
						"id": 1696118400,
						"rx": 720000,
						"tx": 1440000
					}
				]
			}
		}]
	}`

	tests := []struct {
		name string
		json string
	}{
		{"Legacy Format", legacyJSON},
		{"New Format", newJSON},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var data VnStatData
			if err := json.Unmarshal([]byte(tt.json), &data); err != nil {
				t.Fatalf("Failed to unmarshal %s: %v", tt.name, err)
			}

			if len(data.Interfaces) == 0 {
				t.Fatal("No interfaces found")
			}
			iface := data.Interfaces[0]

			// Verify Hour
			if len(iface.Traffic.Hour) == 0 {
				t.Fatal("No hour data found")
			}
			hourID := iface.Traffic.Hour[0].ID
			expectedHour := time.Date(2023, 10, 27, 14, 0, 0, 0, time.UTC)

			if !hourID.Time.Equal(expectedHour) && !hourID.Time.Local().Equal(expectedHour.Local()) {
				t.Errorf("Hour ID mismatch. Got %v, want %v", hourID.Time, expectedHour)
			}

			// Verify Day
			if len(iface.Traffic.Day) == 0 {
				t.Fatal("No day data found")
			}
			dayID := iface.Traffic.Day[0].ID
			expectedDay := time.Date(2023, 10, 27, 0, 0, 0, 0, time.UTC)
			// Using Year/Month/Day resolution, time should be midnight
			y, m, d := dayID.Time.Date()
			ey, em, ed := expectedDay.Date()
			if y != ey || m != em || d != ed {
				t.Errorf("Day ID mismatch. Got %v, want %v", dayID.Time, expectedDay)
			}

			// Verify Month
			if len(iface.Traffic.Month) == 0 {
				t.Fatal("No month data found")
			}
			monthID := iface.Traffic.Month[0].ID
			expectedMonth := time.Date(2023, 10, 1, 0, 0, 0, 0, time.UTC)
			my, mm, _ := monthID.Time.Date()
			emy, emm, _ := expectedMonth.Date()
			if my != emy || mm != emm {
				t.Errorf("Month ID mismatch. Got %v, want %v", monthID.Time, expectedMonth)
			}
		})
	}
}

// TestMetricCalculation verifies the logic used for calculating averages and peaks
func TestMetricCalculation(t *testing.T) {
	// Construct sample data
	now := time.Now().UTC()

	// Create data for last 25 hours
	var hours []struct {
		ID VnStatTime `json:"id"`
		Rx uint64     `json:"rx"`
		Tx uint64     `json:"tx"`
	}

	// 1000 Bytes/sec base rate
	rxRate := uint64(1000)
	txRate := uint64(2000)

	rxPerHour := rxRate * 3600
	txPerHour := txRate * 3600

	// Add a peak hour: 2x traffic 2 hours ago
	peakRxRate := uint64(5000)
	peakTxRate := uint64(8000)

	for i := 0; i < 30; i++ {
		ts := now.Add(time.Duration(-i) * time.Hour)

		rx := rxPerHour
		tx := txPerHour

		// Inject peak at 2 hours ago
		if i == 2 {
			rx = peakRxRate * 3600
			tx = peakTxRate * 3600
		}

		hours = append(hours, struct {
			ID VnStatTime `json:"id"`
			Rx uint64     `json:"rx"`
			Tx uint64     `json:"tx"`
		}{
			ID: VnStatTime{Time: ts},
			Rx: rx,
			Tx: tx,
		})
	}

	// Replicate the logic from collectMetrics
	var sumRx12, sumTx12, sumRx24, sumTx24 uint64
	var count12, count24 uint64
	var peakRx, peakTx uint64

	for _, h := range hours {
		age := now.Sub(h.ID.Time)

		if age <= 24*time.Hour && age >= 0 {
			sumRx24 += h.Rx
			sumTx24 += h.Tx
			count24++

			rateRx := h.Rx / 3600
			rateTx := h.Tx / 3600

			if rateRx > peakRx { peakRx = rateRx }
			if rateTx > peakTx { peakTx = rateTx }

			if age <= 12*time.Hour {
				sumRx12 += h.Rx
				sumTx12 += h.Tx
				count12++
			}
		}
	}

	var avgRx12, avgTx12, avgRx24, avgTx24 uint64
	if count12 > 0 {
		avgRx12 = sumRx12 / (count12 * 3600)
		avgTx12 = sumTx12 / (count12 * 3600)
	}
	if count24 > 0 {
		avgRx24 = sumRx24 / (count24 * 3600)
		avgTx24 = sumTx24 / (count24 * 3600)
	}

	// Assertions

	// Peak should match the injected peak
	if peakRx != peakRxRate {
		t.Errorf("PeakRx mismatch. Got %d, want %d", peakRx, peakRxRate)
	}
	if peakTx != peakTxRate {
		t.Errorf("PeakTx mismatch. Got %d, want %d", peakTx, peakTxRate)
	}

	// Averages check
	expectedAvgRx12 := (peakRxRate*3600 + (count12-1)*rxPerHour) / (count12 * 3600)

	if avgRx12 != expectedAvgRx12 {
		t.Errorf("AvgRx12 mismatch. Got %d, want %d (Count: %d)", avgRx12, expectedAvgRx12, count12)
	}

	// Use other variables to satisfy compiler and basic sanity check
	if avgTx12 == 0 { t.Error("AvgTx12 should not be 0") }
	if avgRx24 == 0 { t.Error("AvgRx24 should not be 0") }
	if avgTx24 == 0 { t.Error("AvgTx24 should not be 0") }
}
