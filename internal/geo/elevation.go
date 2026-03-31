package geo

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const defaultElevationURL = "https://api.opentopodata.org/v1/mapzen"

// ElevationClient fetches elevation data from Open Topo Data.
type ElevationClient struct {
	httpClient *http.Client
	baseURL    string
}

// NewElevationClient creates a new elevation client.
func NewElevationClient() *ElevationClient {
	return &ElevationClient{
		httpClient: &http.Client{Timeout: 15 * time.Second},
		baseURL:    defaultElevationURL,
	}
}

type elevationResponse struct {
	Status  string `json:"status"`
	Results []struct {
		Elevation float64 `json:"elevation"`
	} `json:"results"`
}

// GetElevationBatch fetches elevations for a batch of locations.
// Each location is {lat, lon}. Returns elevations in the same order.
func (c *ElevationClient) GetElevationBatch(ctx context.Context, locations [][2]float64) ([]float64, error) {
	const batchSize = 10
	var elevations []float64

	for i := 0; i < len(locations); i += batchSize {
		end := i + batchSize
		if end > len(locations) {
			end = len(locations)
		}
		batch := locations[i:end]

		var parts []string
		for _, loc := range batch {
			parts = append(parts, fmt.Sprintf("%.4f,%.4f", loc[0], loc[1]))
		}
		queryString := strings.Join(parts, "|")

		url := fmt.Sprintf("%s?locations=%s&interpolation=cubic", c.baseURL, queryString)

		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return nil, err
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			// On error, fill with zeros
			for range batch {
				elevations = append(elevations, 0)
			}
			continue
		}

		var result elevationResponse
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			resp.Body.Close()
			for range batch {
				elevations = append(elevations, 0)
			}
			continue
		}
		resp.Body.Close()

		if result.Status == "OK" {
			for _, r := range result.Results {
				elevations = append(elevations, r.Elevation)
			}
		} else {
			for range batch {
				elevations = append(elevations, 0)
			}
		}
	}

	return elevations, nil
}
