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
		cc.Logger.Error("MapShare error", "identifier", identifier, "error", err)
		return []string{fmt.Sprintf("MapShare error: %v", err)}, nil
	}

	return []string{result}, nil
}

func formatMapShareOutput(identifier string) (string, error) {
	data, err := fetchAndParseMapShareKML(identifier)
	if err != nil {
		return "", err
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
	kmlURL := "https://share.garmin.com/Feed/Share/" + identifier

	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequest("GET", kmlURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("User-Agent", "InReachAssistant/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching KML: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("MapShare returned HTTP %d for %s", resp.StatusCode, identifier)
	}

	// KML uses default namespace http://www.opengis.net/kml/2.2
	// Go's encoding/xml requires fully qualified names when a namespace is present.
	type kmlData struct {
		Name  string `xml:"name,attr"`
		Value string `xml:"value"`
	}
	var kml struct {
		Document struct {
			Folder struct {
				Placemarks []struct {
					Name      string `xml:"http://www.opengis.net/kml/2.2 name"`
					TimeStamp struct {
						When string `xml:"http://www.opengis.net/kml/2.2 when"`
					} `xml:"http://www.opengis.net/kml/2.2 TimeStamp"`
					Point struct {
						Coordinates string `xml:"http://www.opengis.net/kml/2.2 coordinates"`
					} `xml:"http://www.opengis.net/kml/2.2 Point"`
					ExtendedData struct {
						Data []kmlData `xml:"http://www.opengis.net/kml/2.2 Data"`
					} `xml:"http://www.opengis.net/kml/2.2 ExtendedData"`
				} `xml:"http://www.opengis.net/kml/2.2 Placemark"`
			} `xml:"http://www.opengis.net/kml/2.2 Folder"`
		} `xml:"http://www.opengis.net/kml/2.2 Document"`
	}
	if err := xml.NewDecoder(resp.Body).Decode(&kml); err != nil {
		return nil, fmt.Errorf("parsing KML: %w", err)
	}

	// Filter to placemarks that have a Point (skip LineString track logs)
	var placemarks []struct {
		Name      string `xml:"http://www.opengis.net/kml/2.2 name"`
		TimeStamp struct {
			When string `xml:"http://www.opengis.net/kml/2.2 when"`
		} `xml:"http://www.opengis.net/kml/2.2 TimeStamp"`
		Point struct {
			Coordinates string `xml:"http://www.opengis.net/kml/2.2 coordinates"`
		} `xml:"http://www.opengis.net/kml/2.2 Point"`
		ExtendedData struct {
			Data []kmlData `xml:"http://www.opengis.net/kml/2.2 Data"`
		} `xml:"http://www.opengis.net/kml/2.2 ExtendedData"`
	}
	for _, pm := range kml.Document.Folder.Placemarks {
		if strings.TrimSpace(pm.Point.Coordinates) != "" {
			placemarks = append(placemarks, pm)
		}
	}
	if len(placemarks) == 0 {
		return nil, fmt.Errorf("no placemarks with coordinates found")
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
