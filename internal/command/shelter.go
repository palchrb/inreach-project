package command

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/palchrb/inreach-project/internal/geo"
	"github.com/palchrb/inreach-project/internal/store"
)

// ShelterHandler handles "shelter" commands.
type ShelterHandler struct {
	state      *store.ShelterState
	elevation  *geo.ElevationClient
	cabinsFile string
}

// NewShelterHandler creates a new shelter handler.
func NewShelterHandler(state *store.ShelterState, cabinsFile string) *ShelterHandler {
	return &ShelterHandler{
		state:      state,
		elevation:  geo.NewElevationClient(),
		cabinsFile: cabinsFile,
	}
}

func (h *ShelterHandler) Name() string { return "shelter" }

func (h *ShelterHandler) Handle(cc *CommandContext) ([]string, error) {
	if cc.Lat == nil || cc.Lon == nil {
		return []string{"No coordinates found for nearest cabin."}, nil
	}

	lat, lon := *cc.Lat, *cc.Lon
	result, huts, err := h.findTop4Huts(cc, lat, lon)
	if err != nil {
		return nil, err
	}

	// Store shelter data for "route N" command
	h.state.Set(cc.Message.ConversationID.String(), &store.ShelterData{
		Lat:  lat,
		Lon:  lon,
		Huts: huts,
	})

	return []string{result}, nil
}

type hutCandidate struct {
	Name          string
	Lat           float64
	Lon           float64
	Distance      float64
	Elevation     float64
	ElevationDiff float64
	TotalScore    float64
	Source        string
	ServiceLevel  string
}

// boundingBoxFilter returns true if the cabin is within ~degRadius degrees of lat/lon.
// 0.5 degrees ≈ 55km at equator, ~35km in Norway. Fast pre-filter before haversine.
func inBoundingBox(lat, lon, cabinLat, cabinLon, degRadius float64) bool {
	return cabinLat >= lat-degRadius && cabinLat <= lat+degRadius &&
		cabinLon >= lon-degRadius && cabinLon <= lon+degRadius
}

func (h *ShelterHandler) findTop4Huts(cc *CommandContext, lat, lon float64) (string, []store.ShelterResult, error) {
	var allHuts []hutCandidate

	// 1. Load cached UT.no cabins (pre-filtered by bounding box)
	if h.cabinsFile != "" {
		cabins, err := LoadCachedCabins(h.cabinsFile)
		if err != nil {
			cc.Logger.Warn("No cached cabins file, using OSM only", "error", err)
		} else {
			var nearby int
			for _, c := range cabins {
				// Pre-filter: skip cabins far outside 50km radius (~0.6° at 60°N)
				if !inBoundingBox(lat, lon, c.Lat, c.Lon, 0.6) {
					continue
				}
				nearby++
				distance := geo.Haversine(lat, lon, c.Lat, c.Lon)
				sl := shortServiceLevel(c.ServiceLevel)
				allHuts = append(allHuts, hutCandidate{
					Name:         trimName(c.Name),
					Lat:          c.Lat,
					Lon:          c.Lon,
					Distance:     distance,
					Source:       "U",
					ServiceLevel: sl,
				})
			}
			cc.Logger.Debug("Loaded cached cabins", "total", len(cabins), "nearby", nearby)
		}
	}

	// 2. Fetch OSM huts + shelters + emergency (additional not in UT.no)
	osmHuts, err := fetchOSMHuts(lat, lon, 50000)
	if err != nil {
		cc.Logger.Warn("Failed to fetch OSM huts", "error", err)
	} else {
		allHuts = append(allHuts, osmHuts...)
	}

	// Remove duplicates
	allHuts = removeDuplicateHuts(allHuts)

	// Sort by distance and shortlist
	sort.Slice(allHuts, func(i, j int) bool { return allHuts[i].Distance < allHuts[j].Distance })
	if len(allHuts) > 20 {
		allHuts = allHuts[:20]
	}

	// Fetch elevations
	locations := make([][2]float64, 0, len(allHuts)+1)
	locations = append(locations, [2]float64{lat, lon}) // User position first
	for _, hut := range allHuts {
		locations = append(locations, [2]float64{hut.Lat, hut.Lon})
	}

	elevations, err := h.elevation.GetElevationBatch(cc.Ctx, locations)
	if err != nil {
		cc.Logger.Warn("Failed to fetch elevations", "error", err)
		elevations = make([]float64, len(locations))
	}

	userElevation := elevations[0]
	for i := range allHuts {
		allHuts[i].Elevation = elevations[i+1]
		allHuts[i].ElevationDiff = elevations[i+1] - userElevation
		elevationFactor := math.Log1p(math.Abs(allHuts[i].ElevationDiff)) * 0.5
		allHuts[i].TotalScore = allHuts[i].Distance*0.50 + elevationFactor
	}

	// Sort by total score and pick top 4
	sort.Slice(allHuts, func(i, j int) bool { return allHuts[i].TotalScore < allHuts[j].TotalScore })
	if len(allHuts) > 4 {
		allHuts = allHuts[:4]
	}

	// Format response and build store results
	var lines []string
	var storeHuts []store.ShelterResult
	for _, hut := range allHuts {
		elevSign := "+"
		if hut.ElevationDiff < 0 {
			elevSign = ""
		}
		svc := ""
		if hut.ServiceLevel != "" {
			svc = "|" + hut.ServiceLevel
		}
		line := fmt.Sprintf("%s|%.4f|%.4f|%.1fkm|Δ%s%.0fm|%s%s",
			hut.Name, hut.Lat, hut.Lon, hut.Distance, elevSign, hut.ElevationDiff, hut.Source, svc)
		lines = append(lines, line)
		storeHuts = append(storeHuts, store.ShelterResult{
			Name:          hut.Name,
			Lat:           hut.Lat,
			Lon:           hut.Lon,
			Distance:      hut.Distance,
			ElevationDiff: hut.ElevationDiff,
			Source:        hut.Source,
		})
	}

	return strings.Join(lines, "\n"), storeHuts, nil
}

func fetchOSMHuts(lat, lon float64, radius int) ([]hutCandidate, error) {
	// Include alpine huts, wilderness huts, cabins, lean-tos, and emergency shelters
	query := fmt.Sprintf(`[out:json];(
		node(around:%d,%.4f,%.4f)["tourism"~"alpine_hut|wilderness_hut|chalet|cabin"];
		node(around:%d,%.4f,%.4f)["shelter_type"="lean_to"];
		node(around:%d,%.4f,%.4f)["emergency"="yes"]["amenity"="shelter"];
	);out body;`, radius, lat, lon, radius, lat, lon, radius, lat, lon)

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Post("https://overpass-api.de/api/interpreter", "application/x-www-form-urlencoded",
		strings.NewReader("data="+query))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Elements []struct {
			Lat  float64           `json:"lat"`
			Lon  float64           `json:"lon"`
			Tags map[string]string `json:"tags"`
		} `json:"elements"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	var huts []hutCandidate
	for _, el := range result.Elements {
		name := el.Tags["name"]
		if name == "" {
			name = "Ukjent hytte"
		}
		name = trimName(name)
		distance := geo.Haversine(lat, lon, el.Lat, el.Lon)

		// Determine source label based on type
		source := "O"
		if el.Tags["emergency"] == "yes" {
			source = "E"
		} else if el.Tags["shelter_type"] == "lean_to" {
			source = "G" // Gapahuk
		}

		huts = append(huts, hutCandidate{
			Name:     name,
			Lat:      el.Lat,
			Lon:      el.Lon,
			Distance: distance,
			Source:   source,
		})
	}

	return huts, nil
}

// shortServiceLevel abbreviates UT.no service levels for compact display.
func shortServiceLevel(sl string) string {
	sl = strings.ToLower(sl)
	switch {
	case strings.Contains(sl, "staffed") || strings.Contains(sl, "betjent"):
		return "B"
	case strings.Contains(sl, "self-service") || strings.Contains(sl, "selvbetjent"):
		return "S"
	case strings.Contains(sl, "no-service") && strings.Contains(sl, "no beds"):
		return "X"
	case strings.Contains(sl, "no-service") || strings.Contains(sl, "ubetjent"):
		return "U"
	default:
		return ""
	}
}

func removeDuplicateHuts(huts []hutCandidate) []hutCandidate {
	var unique []hutCandidate
	for _, hut := range huts {
		isDuplicate := false
		for _, existing := range unique {
			dist := geo.Haversine(hut.Lat, hut.Lon, existing.Lat, existing.Lon)
			if strings.EqualFold(hut.Name, existing.Name) || dist < 0.5 {
				isDuplicate = true
				break
			}
		}
		if !isDuplicate {
			unique = append(unique, hut)
		}
	}
	return unique
}

func trimName(text string) string {
	// Remove special characters, keep Norwegian letters
	var result []rune
	for _, r := range text {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') ||
			r == 'æ' || r == 'ø' || r == 'å' || r == 'Æ' || r == 'Ø' || r == 'Å' ||
			r == ' ' || r == '-' || r == '|' {
			result = append(result, r)
		}
	}
	s := strings.TrimSpace(string(result))
	if len(s) > 20 {
		s = s[:20]
	}
	return s
}
