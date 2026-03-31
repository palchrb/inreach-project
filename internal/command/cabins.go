package command

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"
)

// CabinEntry represents a cached cabin from UT.no.
type CabinEntry struct {
	ID           int     `json:"id"`
	Name         string  `json:"name"`
	Lat          float64 `json:"lat"`
	Lon          float64 `json:"lon"`
	ServiceLevel string  `json:"serviceLevel"`
	Owner        string  `json:"owner"`
}

// FetchAndCacheCabins fetches all cabins from UT.no GraphQL API and saves to a JSON file.
// This should be run periodically (e.g. weekly) to keep the cache fresh.
func FetchAndCacheCabins(logger *slog.Logger, outputPath string) error {
	const apiURL = "https://api.ut.no/"
	const query = `query FindCabins($input: NTB_FindCabinsInput) {
		ntb_findCabins(input: $input) {
			totalCount
			pageInfo {
				hasNextPage
				endCursor
			}
			edges {
				node {
					id
					name
					serviceLevel
					dntCabin
					owner {
						name
					}
					geometry
				}
			}
		}
	}`

	client := &http.Client{Timeout: 30 * time.Second}
	var allCabins []CabinEntry
	var afterCursor *string

	for {
		variables := map[string]interface{}{
			"input": map[string]interface{}{
				"pageOptions": map[string]interface{}{
					"limit":            500,
					"afterCursor":      afterCursor,
					"orderByDirection": "DESC",
					"orderBy":          "ID",
				},
			},
		}

		payload := map[string]interface{}{
			"query":     query,
			"variables": variables,
		}

		body, _ := json.Marshal(payload)
		req, err := http.NewRequest("POST", apiURL, bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("creating request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("UT.no API request: %w", err)
		}

		var result struct {
			Data struct {
				NTBFindCabins struct {
					TotalCount int `json:"totalCount"`
					PageInfo   struct {
						HasNextPage bool    `json:"hasNextPage"`
						EndCursor   *string `json:"endCursor"`
					} `json:"pageInfo"`
					Edges []struct {
						Node struct {
							ID           int    `json:"id"`
							Name         string `json:"name"`
							ServiceLevel string `json:"serviceLevel"`
							Owner        *struct {
								Name string `json:"name"`
							} `json:"owner"`
							Geometry *struct {
								Coordinates []float64 `json:"coordinates"`
							} `json:"geometry"`
						} `json:"node"`
					} `json:"edges"`
				} `json:"ntb_findCabins"`
			} `json:"data"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			resp.Body.Close()
			return fmt.Errorf("parsing UT.no response: %w", err)
		}
		resp.Body.Close()

		data := result.Data.NTBFindCabins

		for _, edge := range data.Edges {
			node := edge.Node

			// GeoJSON coordinates: [lon, lat]
			var lat, lon float64
			if node.Geometry != nil && len(node.Geometry.Coordinates) >= 2 {
				lon = node.Geometry.Coordinates[0]
				lat = node.Geometry.Coordinates[1]
			} else {
				continue // Skip cabins without coordinates
			}

			owner := "Unknown"
			if node.Owner != nil {
				owner = node.Owner.Name
			}

			allCabins = append(allCabins, CabinEntry{
				ID:           node.ID,
				Name:         node.Name,
				Lat:          lat,
				Lon:          lon,
				ServiceLevel: node.ServiceLevel,
				Owner:        owner,
			})
		}

		logger.Info("Fetched cabins page",
			"count", len(data.Edges),
			"total", len(allCabins),
			"totalAvailable", data.TotalCount,
		)

		if !data.PageInfo.HasNextPage {
			break
		}
		afterCursor = data.PageInfo.EndCursor
	}

	// Write to file
	out, err := json.MarshalIndent(allCabins, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling cabins: %w", err)
	}

	if err := os.WriteFile(outputPath, out, 0o644); err != nil {
		return fmt.Errorf("writing cabins file: %w", err)
	}

	logger.Info("Cabins cached", "count", len(allCabins), "path", outputPath)
	return nil
}

// LoadCachedCabins loads cabins from the cache file.
func LoadCachedCabins(path string) ([]CabinEntry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cabins []CabinEntry
	if err := json.Unmarshal(data, &cabins); err != nil {
		return nil, fmt.Errorf("parsing cabins cache: %w", err)
	}
	return cabins, nil
}
