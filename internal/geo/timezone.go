package geo

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// TimezoneData contains timezone information for a location.
type TimezoneData struct {
	CityName  string `json:"cityName"`
	// GMTOffset is the total offset from UTC in seconds, including DST
	// when active. TimezoneDB returns this as the complete current offset,
	// so no additional DST adjustment is needed.
	GMTOffset int `json:"gmtOffset"`
	// DST indicates whether DST is currently active (1) or not (0).
	// This is informational only — GMTOffset already accounts for it.
	DST int `json:"dst"`
}

// GetTimezone fetches timezone data for a lat/lon using the TimezoneDB API.
func GetTimezone(ctx context.Context, apiKey string, lat, lon float64) (*TimezoneData, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("timezonedb API key is required for correct weather forecasts")
	}

	url := fmt.Sprintf("http://api.timezonedb.com/v2.1/get-time-zone?key=%s&format=json&by=position&lat=%.4f&lng=%.4f",
		apiKey, lat, lon)

	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("timezone API request: %w", err)
	}
	defer resp.Body.Close()

	// TimezoneDB returns dst as a string "0" or "1", not a number.
	var result struct {
		Status    string `json:"status"`
		CityName  string `json:"cityName"`
		GMTOffset int    `json:"gmtOffset"`
		DST       string `json:"dst"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("parsing timezone response: %w", err)
	}

	if result.Status != "OK" {
		return &TimezoneData{CityName: "UTC", GMTOffset: 0, DST: 0}, nil
	}

	dst := 0
	if result.DST == "1" {
		dst = 1
	}

	return &TimezoneData{
		CityName:  result.CityName,
		GMTOffset: result.GMTOffset,
		DST:       dst,
	}, nil
}
