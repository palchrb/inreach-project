package command

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// TrainHandler handles "train" commands.
type TrainHandler struct{}

func NewTrainHandler() *TrainHandler { return &TrainHandler{} }
func (h *TrainHandler) Name() string { return "train" }

func (h *TrainHandler) Handle(cc *CommandContext) ([]string, error) {
	args := strings.TrimSpace(cc.Args)

	// Check for stationboard subcommand
	if strings.HasPrefix(strings.ToLower(args), "stationboard") {
		return h.handleStationBoard(cc, strings.TrimPrefix(args, "stationboard"))
	}

	return h.handleTrainRequest(cc, args)
}

func (h *TrainHandler) handleTrainRequest(cc *CommandContext, message string) ([]string, error) {
	parsed := parseTrainMessage(message)
	if parsed == nil {
		return []string{"Invalid format. Use: train origin - destination [Xh]"}, nil
	}

	var originStation, destStation *station

	// Find origin station
	if strings.EqualFold(parsed.originName, "location") && cc.Lat != nil && cc.Lon != nil {
		originStation = findNearestStation(*cc.Lat, *cc.Lon, parsed.allowBus)
	} else {
		originStation = findStationID(parsed.originName, "", parsed.allowBus)
	}

	// Find destination station
	destStation = findStationID(parsed.destinationName, parsed.destinationCity, parsed.allowBus)
	if destStation == nil {
		// Try geocoding fallback
		coords := findClosestRailStation(parsed.destinationName, parsed.destinationCity)
		if coords != nil {
			destStation = findNearestStation(coords[0], coords[1], parsed.allowBus)
		}
	}

	if originStation == nil {
		return []string{fmt.Sprintf("Startstasjon \"%s\" ikke funnet.", parsed.originName)}, nil
	}
	if destStation == nil {
		return []string{fmt.Sprintf("Destinasjon \"%s\" ikke funnet.", parsed.destinationName)}, nil
	}

	result := getTrainDepartures(originStation, destStation, parsed.hours, 5, parsed.allowBus)
	return []string{result}, nil
}

func (h *TrainHandler) handleStationBoard(cc *CommandContext, args string) ([]string, error) {
	args = strings.TrimSpace(args)

	var stationObj *station
	var filterDest string
	timeRange := 72000 // Default: 20 hours in seconds

	// Parse "to:DESTINATION"
	toMatch := regexp.MustCompile(`(?i)to:(.+)$`).FindStringSubmatch(args)
	if toMatch != nil {
		filterDest = strings.TrimSpace(toMatch[1])
		args = strings.TrimSpace(strings.Replace(args, toMatch[0], "", 1))
	}

	// Parse station name and time
	match := regexp.MustCompile(`(?i)^([^\d]+?)(\d+h)?$`).FindStringSubmatch(args)
	if match != nil && strings.TrimSpace(match[1]) != "" {
		stationObj = findStationID(strings.TrimSpace(match[1]), "", false)
		if match[2] != "" {
			hours, _ := strconv.Atoi(strings.TrimSuffix(match[2], "h"))
			timeRange = hours * 3600
		}
	} else if cc.Lat != nil && cc.Lon != nil {
		stationObj = findNearestStation(*cc.Lat, *cc.Lon, false)
	}

	if stationObj == nil {
		return []string{"Kunne ikke finne stasjonen."}, nil
	}

	result := fetchStationBoard(stationObj, filterDest, timeRange)
	return []string{result}, nil
}

// Data types

type station struct {
	ID   string
	Name string
}

type trainParsed struct {
	originName      string
	destinationName string
	destinationCity string
	hours           int
	allowBus        bool
}

func parseTrainMessage(message string) *trainParsed {
	allowBus := false
	cleaned := strings.TrimSpace(message)

	if regexp.MustCompile(`(?i)^bus\b`).MatchString(cleaned) {
		allowBus = true
		cleaned = regexp.MustCompile(`(?i)^bus\s*`).ReplaceAllString(cleaned, "")
	}

	parts := regexp.MustCompile(`\s*-\s*`).Split(cleaned, 2)
	if len(parts) < 2 {
		return nil
	}

	originName := strings.TrimSpace(parts[0])
	destAndExtras := strings.TrimSpace(parts[1])

	match := regexp.MustCompile(`(?i)^(.+?)(?:\s+(\d+)h)?$`).FindStringSubmatch(destAndExtras)
	if match == nil {
		return nil
	}

	var destName, destCity string
	rawDest := strings.TrimSpace(match[1])
	if strings.Contains(rawDest, ",") {
		dParts := strings.SplitN(rawDest, ",", 2)
		destName = strings.TrimSpace(dParts[0])
		destCity = strings.TrimSpace(dParts[1])
	} else {
		destName = rawDest
	}

	hours := 0
	if match[2] != "" {
		hours, _ = strconv.Atoi(match[2])
	}

	return &trainParsed{
		originName:      originName,
		destinationName: destName,
		destinationCity: destCity,
		hours:           hours,
		allowBus:        allowBus,
	}
}

func findStationID(name, city string, allowBus bool) *station {
	categories := "railStation"
	if allowBus {
		categories = "railStation,busStation,onstreetBus"
	}

	apiURL := fmt.Sprintf("https://api.entur.io/geocoder/v1/autocomplete?text=%s&categories=%s&size=10",
		url.QueryEscape(name), categories)

	client := &http.Client{Timeout: 10 * time.Second}
	req, _ := http.NewRequest("GET", apiURL, nil)
	req.Header.Set("ET-Client-Name", "inreach-togsjekk")

	resp, err := client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	var result struct {
		Features []struct {
			Properties struct {
				ID       string `json:"id"`
				Name     string `json:"name"`
				Locality string `json:"locality"`
				Category []string `json:"category"`
			} `json:"properties"`
		} `json:"features"`
	}
	json.NewDecoder(resp.Body).Decode(&result)

	if len(result.Features) == 0 {
		return nil
	}

	matches := result.Features
	if city != "" {
		var filtered []struct {
			Properties struct {
				ID       string `json:"id"`
				Name     string `json:"name"`
				Locality string `json:"locality"`
				Category []string `json:"category"`
			} `json:"properties"`
		}
		for _, f := range matches {
			if strings.EqualFold(f.Properties.Locality, city) {
				filtered = append(filtered, f)
			}
		}
		if len(filtered) > 0 {
			matches = filtered
		}
	}

	return &station{
		ID:   matches[0].Properties.ID,
		Name: fmt.Sprintf("%s, %s", matches[0].Properties.Name, matches[0].Properties.Locality),
	}
}

func findNearestStation(lat, lon float64, allowBus bool) *station {
	categories := "railStation"
	if allowBus {
		categories = "railStation,busStation,onstreetBus"
	}

	apiURL := fmt.Sprintf("https://api.entur.io/geocoder/v1/reverse?point.lat=%.4f&point.lon=%.4f&categories=%s&boundary.circle.radius=10000&multiModal=all&size=5",
		lat, lon, categories)

	client := &http.Client{Timeout: 10 * time.Second}
	req, _ := http.NewRequest("GET", apiURL, nil)
	req.Header.Set("ET-Client-Name", "inreach-togsjekk")

	resp, err := client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	var result struct {
		Features []struct {
			Properties struct {
				ID       string   `json:"id"`
				Name     string   `json:"name"`
				Category []string `json:"category"`
			} `json:"properties"`
		} `json:"features"`
	}
	json.NewDecoder(resp.Body).Decode(&result)

	if len(result.Features) == 0 {
		return nil
	}

	return &station{
		ID:   result.Features[0].Properties.ID,
		Name: result.Features[0].Properties.Name,
	}
}

func findClosestRailStation(name, locality string) []float64 {
	searchQuery := name
	if locality != "" {
		searchQuery += ", " + locality
	}

	apiURL := fmt.Sprintf("https://api.entur.io/geocoder/v1/autocomplete?text=%s&size=1", url.QueryEscape(searchQuery))

	client := &http.Client{Timeout: 10 * time.Second}
	req, _ := http.NewRequest("GET", apiURL, nil)
	req.Header.Set("ET-Client-Name", "inreach-togsjekk")

	resp, err := client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	var result struct {
		Features []struct {
			Geometry struct {
				Coordinates []float64 `json:"coordinates"`
			} `json:"geometry"`
		} `json:"features"`
	}
	json.NewDecoder(resp.Body).Decode(&result)

	if len(result.Features) == 0 || len(result.Features[0].Geometry.Coordinates) < 2 {
		return nil
	}

	coords := result.Features[0].Geometry.Coordinates
	return []float64{coords[1], coords[0]} // lat, lon
}

func getTrainDepartures(origin, dest *station, hours, numTrips int, allowBus bool) string {
	dateTime := time.Now().Add(time.Duration(hours) * time.Hour)

	filters := `{not: {transportModes: [{transportMode: air}, {transportMode: bus}, {transportMode: coach}]}}`
	if allowBus {
		filters = `{not: {transportModes: {transportMode: air}}}`
	}

	query := fmt.Sprintf(`{
		trip(
			from: { place: "%s" }
			to: { place: "%s" }
			numTripPatterns: %d
			dateTime: "%s"
			filters: %s
		) {
			tripPatterns {
				legs {
					mode
					line { publicCode }
					fromEstimatedCall {
						aimedDepartureTime
						expectedDepartureTime
						cancellation
					}
				}
			}
		}
	}`, origin.ID, dest.ID, numTrips, dateTime.Format(time.RFC3339), filters)

	client := &http.Client{Timeout: 15 * time.Second}
	body, _ := json.Marshal(map[string]string{"query": query})
	req, _ := http.NewRequest("POST", "https://api.entur.io/journey-planner/v3/graphql", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("ET-Client-Name", "inreach-togsjekk")

	resp, err := client.Do(req)
	if err != nil {
		return "Feil ved henting av togdata."
	}
	defer resp.Body.Close()

	var data struct {
		Data struct {
			Trip struct {
				TripPatterns []struct {
					Legs []struct {
						Mode string `json:"mode"`
						Line *struct {
							PublicCode string `json:"publicCode"`
						} `json:"line"`
						FromEstimatedCall *struct {
							AimedDepartureTime    string `json:"aimedDepartureTime"`
							ExpectedDepartureTime string `json:"expectedDepartureTime"`
							Cancellation          bool   `json:"cancellation"`
						} `json:"fromEstimatedCall"`
					} `json:"legs"`
				} `json:"tripPatterns"`
			} `json:"trip"`
		} `json:"data"`
	}
	json.NewDecoder(resp.Body).Decode(&data)

	trips := data.Data.Trip.TripPatterns
	if len(trips) == 0 {
		return "Ingen avganger funnet."
	}

	originShort := shortenStationName(origin.Name, 10)
	destShort := shortenStationName(dest.Name, 10)
	msg := fmt.Sprintf("%s-%s\n", originShort, destShort)

	for _, trip := range trips {
		var legParts []string
		hasAir := false
		for _, leg := range trip.Legs {
			if leg.Mode == "foot" {
				continue
			}
			if leg.Mode == "air" {
				hasAir = true
				break
			}
			lineCode := "N/A"
			if leg.Line != nil && leg.Line.PublicCode != "" {
				lineCode = leg.Line.PublicCode
			}
			if leg.FromEstimatedCall == nil {
				continue
			}
			aimed := formatTime(leg.FromEstimatedCall.AimedDepartureTime)
			expected := formatTime(leg.FromEstimatedCall.ExpectedDepartureTime)

			if leg.FromEstimatedCall.Cancellation {
				legParts = append(legParts, lineCode+" KANSELLERT")
			} else {
				delay := ""
				if aimed != expected {
					delay = " (" + expected + ")"
				}
				legParts = append(legParts, lineCode+" "+aimed+delay)
			}
		}
		if hasAir || len(legParts) == 0 {
			continue
		}
		msg += strings.Join(legParts, " - ") + "\n"
	}

	return strings.TrimSpace(msg)
}

func fetchStationBoard(s *station, filterDest string, timeRange int) string {
	query := fmt.Sprintf(`{
		stopPlace(id: "%s") {
			name
			estimatedCalls(timeRange: %d, numberOfDepartures: 50, includeCancelledTrips: true) {
				aimedDepartureTime
				expectedDepartureTime
				cancellation
				destinationDisplay { frontText }
				serviceJourney {
					journeyPattern {
						line { publicCode transportMode }
					}
				}
			}
		}
	}`, s.ID, timeRange)

	client := &http.Client{Timeout: 15 * time.Second}
	body, _ := json.Marshal(map[string]string{"query": query})
	req, _ := http.NewRequest("POST", "https://api.entur.io/journey-planner/v3/graphql", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("ET-Client-Name", "inreach-togsjekk")

	resp, err := client.Do(req)
	if err != nil {
		return s.Name + "\nFeil ved henting av togdata."
	}
	defer resp.Body.Close()

	var data struct {
		Data struct {
			StopPlace struct {
				Name           string `json:"name"`
				EstimatedCalls []struct {
					AimedDepartureTime    string `json:"aimedDepartureTime"`
					ExpectedDepartureTime string `json:"expectedDepartureTime"`
					Cancellation          bool   `json:"cancellation"`
					DestinationDisplay    struct {
						FrontText string `json:"frontText"`
					} `json:"destinationDisplay"`
					ServiceJourney struct {
						JourneyPattern struct {
							Line struct {
								PublicCode    string `json:"publicCode"`
								TransportMode string `json:"transportMode"`
							} `json:"line"`
						} `json:"journeyPattern"`
					} `json:"serviceJourney"`
				} `json:"estimatedCalls"`
			} `json:"stopPlace"`
		} `json:"data"`
	}
	json.NewDecoder(resp.Body).Decode(&data)

	calls := data.Data.StopPlace.EstimatedCalls

	// Filter to rail only
	var filtered []struct {
		AimedDepartureTime    string `json:"aimedDepartureTime"`
		ExpectedDepartureTime string `json:"expectedDepartureTime"`
		Cancellation          bool   `json:"cancellation"`
		DestinationDisplay    struct {
			FrontText string `json:"frontText"`
		} `json:"destinationDisplay"`
		ServiceJourney struct {
			JourneyPattern struct {
				Line struct {
					PublicCode    string `json:"publicCode"`
					TransportMode string `json:"transportMode"`
				} `json:"line"`
			} `json:"journeyPattern"`
		} `json:"serviceJourney"`
	}
	for _, c := range calls {
		if c.ServiceJourney.JourneyPattern.Line.TransportMode == "rail" {
			if filterDest == "" || strings.EqualFold(c.DestinationDisplay.FrontText, filterDest) {
				filtered = append(filtered, c)
			}
		}
	}

	if len(filtered) > 10 {
		filtered = filtered[:10]
	}

	if len(filtered) == 0 {
		return s.Name + "\nIngen togavganger funnet."
	}

	msg := s.Name + "\n"
	for _, c := range filtered {
		lineCode := c.ServiceJourney.JourneyPattern.Line.PublicCode
		if lineCode == "" {
			lineCode = "N/A"
		}
		dest := c.DestinationDisplay.FrontText
		aimed := formatTime(c.AimedDepartureTime)
		expected := formatTime(c.ExpectedDepartureTime)

		cancelled := ""
		if c.Cancellation {
			cancelled = " CANCELLED"
		}

		depTime := aimed
		if aimed != expected {
			depTime = aimed + " (" + expected + ")"
		}

		msg += fmt.Sprintf("%s %s to %s%s\n", lineCode, depTime, dest, cancelled)
	}

	return strings.TrimSpace(msg)
}

func formatTime(isoTime string) string {
	t, err := time.Parse(time.RFC3339, isoTime)
	if err != nil {
		return "N/A"
	}
	return fmt.Sprintf("%02d%02d", t.Hour(), t.Minute())
}

func shortenStationName(name string, maxLen int) string {
	if len(name) > maxLen {
		return name[:maxLen]
	}
	return name
}
