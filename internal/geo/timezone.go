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
	GMTOffset int    `json:"gmtOffset"` // Offset from UTC in seconds
	DST       int    `json:"dst"`       // 1 if DST is active
}

// TotalOffsetSeconds returns the total offset including DST.
func (t *TimezoneData) TotalOffsetSeconds() int {
	offset := t.GMTOffset
	if t.DST == 1 {
		offset += 3600
	}
	return offset
}

// GetTimezone fetches timezone data for a lat/lon using the TimezoneDB API.
func GetTimezone(ctx context.Context, apiKey string, lat, lon float64) (*TimezoneData, error) {
	if apiKey == "" {
		// Return UTC if no API key
		return &TimezoneData{CityName: "UTC", GMTOffset: 0, DST: 0}, nil
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

	var result struct {
		Status    string `json:"status"`
		CityName  string `json:"cityName"`
		GMTOffset int    `json:"gmtOffset"`
		DST       int    `json:"dst"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("parsing timezone response: %w", err)
	}

	if result.Status != "OK" {
		return &TimezoneData{CityName: "UTC", GMTOffset: 0, DST: 0}, nil
	}

	return &TimezoneData{
		CityName:  result.CityName,
		GMTOffset: result.GMTOffset,
		DST:       result.DST,
	}, nil
}
