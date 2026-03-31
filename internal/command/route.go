package command

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/palchrb/inreach-project/internal/encoding"
	"github.com/palchrb/inreach-project/internal/geo"
	"github.com/palchrb/inreach-project/internal/store"
)

// RouteHandler handles "route" commands.
type RouteHandler struct {
	apiKey string // OpenRouteService API key
	state  *store.ShelterState
}

// NewRouteHandler creates a new route handler.
func NewRouteHandler(orsAPIKey string, state *store.ShelterState) *RouteHandler {
	return &RouteHandler{apiKey: orsAPIKey, state: state}
}

func (h *RouteHandler) Name() string { return "route" }

func (h *RouteHandler) Handle(cc *CommandContext) ([]string, error) {
	if cc.Lat == nil || cc.Lon == nil {
		return []string{"No start position found."}, nil
	}

	startLat, startLon := *cc.Lat, *cc.Lon
	args := strings.TrimSpace(cc.Args)

	var endLat, endLon float64

	// Check if args is a number (route to shelter #N)
	if n, err := strconv.Atoi(args); err == nil {
		shelterData := h.state.Get(cc.Message.ConversationID.String())
		if shelterData == nil {
			return []string{"No previous shelter data found."}, nil
		}
		idx := n - 1
		if idx < 0 || idx >= len(shelterData.Huts) {
			return []string{fmt.Sprintf("Invalid cabin selection: %d", n)}, nil
		}
		// Use current GPS position as start (not the shelter query position)
		endLat = shelterData.Huts[idx].Lat
		endLon = shelterData.Huts[idx].Lon
	} else {
		// Parse coordinates: "lat,lon"
		parts := strings.Split(args, ",")
		if len(parts) != 2 {
			return []string{"Invalid coordinate format. Use: route lat,lon"}, nil
		}
		var err1, err2 error
		endLat, err1 = strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
		endLon, err2 = strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
		if err1 != nil || err2 != nil {
			return []string{"Invalid coordinates."}, nil
		}
	}

	result, err := h.getSimplifiedHikingRoute(startLat, startLon, endLat, endLon)
	if err != nil {
		return nil, err
	}

	return []string{result}, nil
}

func (h *RouteHandler) getSimplifiedHikingRoute(startLat, startLon, endLat, endLon float64) (string, error) {
	if h.apiKey == "" {
		return "ORS API key missing.", nil
	}

	payload := map[string]interface{}{
		"coordinates":  [][2]float64{{startLon, startLat}, {endLon, endLat}},
		"instructions": true,
		"geometry":     true,
	}
	body, _ := json.Marshal(payload)

	client := &http.Client{Timeout: 30 * time.Second}
	req, _ := http.NewRequest("POST", "https://api.openrouteservice.org/v2/directions/foot-hiking", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", h.apiKey)

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("ORS API request: %w", err)
	}
	defer resp.Body.Close()

	var orsResp struct {
		Routes []struct {
			Geometry string `json:"geometry"`
		} `json:"routes"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&orsResp); err != nil {
		return "", fmt.Errorf("parsing ORS response: %w", err)
	}

	if len(orsResp.Routes) == 0 {
		return "Ingen rute.", nil
	}

	coords := encoding.DecodePolyline(orsResp.Routes[0].Geometry)
	simplified := adaptiveSimplifyRoute(coords, 400, 6)

	if len(simplified) < 2 {
		return "For få punkter.", nil
	}

	var lines []string
	for i := 1; i < len(simplified); i++ {
		bearing := geo.Bearing(simplified[i-1], simplified[i])
		distance := geo.Distance(simplified[i-1], simplified[i])
		lines = append(lines, fmt.Sprintf("%d.%.0f° %.2fkm;%.5f,%.5f",
			i, bearing, distance, simplified[i-1][1], simplified[i-1][0]))
	}

	// Last point without bearing/distance
	last := simplified[len(simplified)-1]
	lines = append(lines, fmt.Sprintf("%d.%.5f,%.5f", len(simplified), last[1], last[0]))

	return strings.Join(lines, "\n"), nil
}

// Douglas-Peucker simplification

func adaptiveSimplifyRoute(coords [][2]float64, tolerance float64, maxPoints int) [][2]float64 {
	simplified := simplifyRoute(coords, tolerance)
	for len(simplified) > maxPoints && tolerance < 800 {
		tolerance += 100
		simplified = simplifyRoute(coords, tolerance)
	}
	return simplified
}

func simplifyRoute(coords [][2]float64, tolerance float64) [][2]float64 {
	if len(coords) < 3 {
		return coords
	}

	maxDist := 0.0
	maxIdx := 0

	for i := 1; i < len(coords)-1; i++ {
		dist := perpendicularDistance(coords[i], coords[0], coords[len(coords)-1])
		if dist > maxDist {
			maxDist = dist
			maxIdx = i
		}
	}

	if maxDist > tolerance {
		left := simplifyRoute(coords[:maxIdx+1], tolerance)
		right := simplifyRoute(coords[maxIdx:], tolerance)
		return append(left[:len(left)-1], right...)
	}

	return [][2]float64{coords[0], coords[len(coords)-1]}
}

func perpendicularDistance(point, lineStart, lineEnd [2]float64) float64 {
	x, y := point[0], point[1]
	x1, y1 := lineStart[0], lineStart[1]
	x2, y2 := lineEnd[0], lineEnd[1]

	A := x - x1
	B := y - y1
	C := x2 - x1
	D := y2 - y1

	dot := A*C + B*D
	lenSq := C*C + D*D

	param := -1.0
	if lenSq != 0 {
		param = dot / lenSq
	}

	var xx, yy float64
	if param < 0 {
		xx, yy = x1, y1
	} else if param > 1 {
		xx, yy = x2, y2
	} else {
		xx = x1 + param*C
		yy = y1 + param*D
	}

	dx := x - xx
	dy := y - yy
	return math.Sqrt(dx*dx+dy*dy) * 100000
}
