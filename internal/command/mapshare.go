package command

import (
	"encoding/xml"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

// MapShareHandler handles "locate" commands.
type MapShareHandler struct{}

func NewMapShareHandler() *MapShareHandler { return &MapShareHandler{} }
func (h *MapShareHandler) Name() string    { return "mapshare" }

func (h *MapShareHandler) Handle(cc *CommandContext) ([]string, error) {
	identifier := strings.TrimSpace(strings.ToUpper(cc.Args))
	if identifier == "" {
		return []string{"Use: locate IDENTIFIER"}, nil
	}

	result, err := formatMapShareOutput(identifier)
	if err != nil {
		return nil, err
	}

	return []string{result}, nil
}

func formatMapShareOutput(identifier string) (string, error) {
	data, err := fetchAndParseMapShareKML(identifier)
	if err != nil {
		return "Ingen data tilgjengelig.", nil
	}

	name := data.Name
	if name == "" {
		name = "Ukjent"
	}
	updated := data.LocalTime
	if updated == "" {
		updated = data.Timestamp
	}
	if updated == "" {
		updated = "Ukjent tid"
	}

	elevation := "Elevation: " + strings.Split(data.ExtendedData["Elevation"], " ")[0] + "m"

	courseRaw := data.ExtendedData["Course"]
	courseStr := strings.Split(courseRaw, " ")[0]
	courseVal, _ := strconv.ParseFloat(courseStr, 64)
	course := fmt.Sprintf("Course: %.0f°", courseVal)

	inEmergency := strings.ToLower(data.ExtendedData["In Emergency"])
	status := "OK"
	if inEmergency == "true" {
		status = "Emergency"
	}

	output := fmt.Sprintf("%s\n%s\n%.4f, %.4f\n%s\n%s\nStatus: %s",
		name, updated, data.Lat, data.Lon, elevation, course, status)

	text := strings.TrimSpace(data.ExtendedData["Text"])
	if text != "" {
		output += "\n" + text
	}

	return output, nil
}

type mapShareData struct {
	Name         string
	Timestamp    string
	LocalTime    string
	Lat          float64
	Lon          float64
	Alt          float64
	ExtendedData map[string]string
}

func fetchAndParseMapShareKML(identifier string) (*mapShareData, error) {
	url := "https://share.garmin.com/Feed/Share/" + identifier

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("fetching KML: %w", err)
	}
	defer resp.Body.Close()

	// Parse KML XML
	var kml struct {
		XMLName  xml.Name `xml:"kml"`
		Document struct {
			Folder struct {
				Placemarks []struct {
					Name      string `xml:"name"`
					TimeStamp struct {
						When string `xml:"when"`
					} `xml:"TimeStamp"`
					Point struct {
						Coordinates string `xml:"coordinates"`
					} `xml:"Point"`
					ExtendedData struct {
						Data []struct {
							Name  string `xml:"name,attr"`
							Value string `xml:"value"`
						} `xml:"Data"`
					} `xml:"ExtendedData"`
				} `xml:"Placemark"`
			} `xml:"Folder"`
		} `xml:"Document"`
	}

	if err := xml.NewDecoder(resp.Body).Decode(&kml); err != nil {
		return nil, fmt.Errorf("parsing KML: %w", err)
	}

	placemarks := kml.Document.Folder.Placemarks
	if len(placemarks) == 0 {
		return nil, fmt.Errorf("no placemarks found")
	}

	// Find latest placemark by timestamp
	sort.Slice(placemarks, func(i, j int) bool {
		ti, _ := time.Parse(time.RFC3339, placemarks[i].TimeStamp.When)
		tj, _ := time.Parse(time.RFC3339, placemarks[j].TimeStamp.When)
		return ti.After(tj)
	})

	pm := placemarks[0]

	// Parse coordinates (lon,lat,alt)
	parts := strings.Split(strings.TrimSpace(pm.Point.Coordinates), ",")
	lon, _ := strconv.ParseFloat(parts[0], 64)
	lat := 0.0
	alt := 0.0
	if len(parts) > 1 {
		lat, _ = strconv.ParseFloat(parts[1], 64)
	}
	if len(parts) > 2 {
		alt, _ = strconv.ParseFloat(parts[2], 64)
	}

	// Parse extended data
	extData := make(map[string]string)
	for _, d := range pm.ExtendedData.Data {
		extData[d.Name] = d.Value
	}

	return &mapShareData{
		Name:         pm.Name,
		Timestamp:    pm.TimeStamp.When,
		LocalTime:    extData["Time"],
		Lat:          lat,
		Lon:          lon,
		Alt:          alt,
		ExtendedData: extData,
	}, nil
}
