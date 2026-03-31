package command

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/palchrb/inreach-project/internal/encoding"
	"github.com/palchrb/inreach-project/internal/geo"
)

// WeatherHandler handles "vær" commands.
type WeatherHandler struct {
	apiKey      string // OpenAI API key for qualitative assessment
	model       string // OpenAI model
	tzAPIKey    string // TimezoneDB API key
}

// NewWeatherHandler creates a new weather handler.
func NewWeatherHandler(openAIKey, model, tzAPIKey string) *WeatherHandler {
	return &WeatherHandler{apiKey: openAIKey, model: model, tzAPIKey: tzAPIKey}
}

func (h *WeatherHandler) Name() string { return "weather" }

func (h *WeatherHandler) Handle(cc *CommandContext) ([]string, error) {
	if cc.Lat == nil || cc.Lon == nil {
		return []string{"No coordinates found for weather forecast."}, nil
	}

	lat, lon := *cc.Lat, *cc.Lon
	argsLower := strings.ToLower(cc.Args)

	// Determine day (Norwegian + English)
	day := 1
	if strings.Contains(argsLower, "i morgen") || strings.Contains(argsLower, "tomorrow") {
		day = 2
	} else if strings.Contains(argsLower, "i overimorgen") || strings.Contains(argsLower, "day after") {
		day = 3
	}

	// Get timezone
	tzData, err := geo.GetTimezone(cc.Ctx, h.tzAPIKey, lat, lon)
	if err != nil {
		cc.Logger.Warn("Failed to get timezone, using UTC", "error", err)
		tzData = &geo.TimezoneData{CityName: "UTC"}
	}

	// Fetch weather data
	weatherData, err := fetchYrWeather(lat, lon, day, tzData)
	if err != nil {
		return nil, fmt.Errorf("fetching weather: %w", err)
	}

	if len(weatherData) == 0 {
		return []string{"Ingen værdata tilgjengelig."}, nil
	}

	// Adjust to local timezone
	adjustWeatherToTimezone(weatherData, tzData)

	if strings.Contains(argsLower, "detaljert") || strings.Contains(argsLower, "detailed") {
		// Detailed weather: ultra-compact Base85/Base36 encoding
		msg := generateUltraCompactWeather(weatherData, tzData.CityName)
		return []string{msg}, nil
	}

	// Standard weather: M/E/K summary
	weatherMsg := generateWeatherMessage(weatherData)

	// Qualitative assessment from ChatGPT
	if h.apiKey != "" {
		remainingLen := cc.CharLimit - len(weatherMsg) - 1
		if remainingLen > 20 {
			qualitative := h.generateQualitativeAssessment(weatherData, remainingLen)
			if qualitative != "" {
				weatherMsg += "\n" + qualitative
			}
		}
	}

	return []string{weatherMsg}, nil
}

// yrTimeseries is the relevant subset of yr.no API response.
type yrTimeseries struct {
	Time string
	Data struct {
		Instant struct {
			Details struct {
				AirTemperature   float64 `json:"air_temperature"`
				WindSpeed        float64 `json:"wind_speed"`
				WindSpeedOfGust  float64 `json:"wind_speed_of_gust"`
				CloudAreaFraction float64 `json:"cloud_area_fraction"`
				WindFromDirection float64 `json:"wind_from_direction"`
			} `json:"details"`
		} `json:"instant"`
		Next1Hours *struct {
			Summary struct {
				SymbolCode string `json:"symbol_code"`
			} `json:"summary"`
			Details struct {
				PrecipitationAmount float64 `json:"precipitation_amount"`
			} `json:"details"`
		} `json:"next_1_hours"`
	} `json:"data"`
}

func fetchYrWeather(lat, lon float64, day int, tz *geo.TimezoneData) ([]yrTimeseries, error) {
	url := fmt.Sprintf("https://api.met.no/weatherapi/locationforecast/2.0/complete?lat=%.4f&lon=%.4f", lat, lon)

	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "InReachWeatherApp/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var yrResp struct {
		Properties struct {
			Timeseries []struct {
				Time string          `json:"time"`
				Data json.RawMessage `json:"data"`
			} `json:"timeseries"`
		} `json:"properties"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&yrResp); err != nil {
		return nil, err
	}

	// Calculate UTC time range for the desired local day
	now := time.Now().UTC()
	targetDate := now.Add(time.Duration(day-1) * 24 * time.Hour)
	targetDate = time.Date(targetDate.Year(), targetDate.Month(), targetDate.Day(), 0, 0, 0, 0, time.UTC)

	gmtOffsetDur := time.Duration(tz.GMTOffset) * time.Second
	localMidnightUTC := targetDate.Add(-gmtOffsetDur)
	localEndUTC := localMidnightUTC.Add(24 * time.Hour)

	var filtered []yrTimeseries
	for _, ts := range yrResp.Properties.Timeseries {
		entryTime, err := time.Parse(time.RFC3339, ts.Time)
		if err != nil {
			continue
		}
		if entryTime.Before(localMidnightUTC) || !entryTime.Before(localEndUTC) {
			continue
		}
		var entry yrTimeseries
		entry.Time = ts.Time
		if err := json.Unmarshal(ts.Data, &entry.Data); err != nil {
			continue
		}
		filtered = append(filtered, entry)
	}

	return filtered, nil
}

func adjustWeatherToTimezone(data []yrTimeseries, tz *geo.TimezoneData) {
	// GMTOffset from TimezoneDB already includes DST when active,
	// so we use it directly without adding extra DST offset.
	offsetMs := time.Duration(tz.GMTOffset) * time.Second
	for i := range data {
		t, err := time.Parse(time.RFC3339, data[i].Time)
		if err != nil {
			continue
		}
		adjusted := t.Add(offsetMs)
		data[i].Time = adjusted.Format(time.RFC3339)
	}
}

func generateWeatherMessage(data []yrTimeseries) string {
	if len(data) == 0 {
		return "Ingen værdata tilgjengelig."
	}

	// Get day number from first entry
	datePart := strings.Split(data[0].Time, "T")[0]
	parts := strings.Split(datePart, "-")
	localDay := parts[2]

	// Filter by time period
	var morning, afternoon, evening []yrTimeseries
	for _, entry := range data {
		hour := getHour(entry.Time)
		switch {
		case hour >= 6 && hour < 12:
			morning = append(morning, entry)
		case hour >= 12 && hour < 18:
			afternoon = append(afternoon, entry)
		case hour >= 18 && hour <= 23:
			evening = append(evening, entry)
		}
	}

	mSummary := summarizePeriod(morning, true)
	eSummary := summarizePeriod(afternoon, false)
	kSummary := summarizePeriod(evening, false)

	return fmt.Sprintf("%s\nM %s\nE %s\nK %s", localDay, mSummary, eSummary, kSummary)
}

func summarizePeriod(data []yrTimeseries, withUnits bool) string {
	if len(data) == 0 {
		return "- - - -"
	}

	first := data[0]
	temp := int(math.Round(first.Data.Instant.Details.AirTemperature))
	wind := int(math.Round(first.Data.Instant.Details.WindSpeed))
	cloudCover := int(math.Round(first.Data.Instant.Details.CloudAreaFraction))

	var totalPrecip float64
	for _, entry := range data {
		if entry.Data.Next1Hours != nil {
			totalPrecip += entry.Data.Next1Hours.Details.PrecipitationAmount
		}
	}
	totalPrecip = math.Round(totalPrecip*10) / 10

	if withUnits {
		return fmt.Sprintf("%d° %dm/s %.1fmm %d%%", temp, wind, totalPrecip, cloudCover)
	}
	return fmt.Sprintf("%d %d %.1f %d", temp, wind, totalPrecip, cloudCover)
}

func generateUltraCompactWeather(data []yrTimeseries, cityName string) string {
	if len(data) == 0 {
		return "Ingen data."
	}

	// Truncate city name to 12 chars, uppercase
	truncatedCity := strings.ToUpper(cityName)
	if len(truncatedCity) > 12 {
		truncatedCity = truncatedCity[:12]
	}

	// Extract date from first entry
	datePart := strings.Split(data[0].Time, "T")[0]
	dateParts := strings.Split(datePart, "-")
	year, _ := parseInt(dateParts[0])
	month, _ := parseInt(dateParts[1])
	day, _ := parseInt(dateParts[2])
	dateValue := (year%100)*10000 + month*100 + day
	dateBase36 := encoding.EncodeBase36Pad(dateValue, 4)

	var encoded []string
	for _, hour := range data {
		h := getHour(hour.Time)
		hourB36 := strings.ToUpper(fmt.Sprintf("%s", encoding.EncodeBase36(h)))
		tempB36 := encoding.EncodeBase36Pad(int(math.Round(hour.Data.Instant.Details.AirTemperature))+50, 2)
		windB36 := encoding.EncodeBase36(int(math.Round(hour.Data.Instant.Details.WindSpeed)))
		gustB36 := encoding.EncodeBase36Pad(int(math.Round(hour.Data.Instant.Details.WindSpeedOfGust)), 2)

		symbolCode := "unknown"
		precipAmount := 0.0
		if hour.Data.Next1Hours != nil {
			symbolCode = hour.Data.Next1Hours.Summary.SymbolCode
			precipAmount = hour.Data.Next1Hours.Details.PrecipitationAmount
		}
		iconB85 := encoding.MapWeatherSymbolToBase85(symbolCode)
		precipB36 := encoding.EncodeBase36Pad(int(math.Round(precipAmount*10)), 2)
		direction := int(math.Round(hour.Data.Instant.Details.WindFromDirection/45)) % 8

		encoded = append(encoded, fmt.Sprintf("%s%s%s%s%s%s%d", hourB36, tempB36, windB36, gustB36, iconB85, precipB36, direction))
	}

	return fmt.Sprintf("%s;%s;%s", truncatedCity, dateBase36, strings.Join(encoded, ";"))
}

func (h *WeatherHandler) generateQualitativeAssessment(data []yrTimeseries, maxLen int) string {
	if len(data) == 0 || h.apiKey == "" {
		return ""
	}

	var formatted []string
	for _, entry := range data {
		t := strings.Split(entry.Time, "T")[1][:5]
		temp := entry.Data.Instant.Details.AirTemperature
		wind := entry.Data.Instant.Details.WindSpeed
		precip := 0.0
		cloudCover := entry.Data.Instant.Details.CloudAreaFraction
		if entry.Data.Next1Hours != nil {
			precip = entry.Data.Next1Hours.Details.PrecipitationAmount
		}
		formatted = append(formatted, fmt.Sprintf("%s: %.0f°C, %.0fm/s, %.1fmm, %.0f%%skydekke", t, temp, wind, precip, cloudCover))
	}

	prompt := fmt.Sprintf("Du er en meteorologisk ekspert som skal svare konsist innen en streng grense på %d tegn. Analyser følgende time-for-time værvarsel: %s. Fokuser på sterke vindforhold (>8m/s), betydelig nedbør (>0.5mm per time over flere timer, med >10mm iløpet av dagen), eller perioder med klart vær. Gi en kort vurdering som inkluderer det beste værvinduet for utendørs aktivitet på høyfjellet i Norge om vinteren - på dagtid, altså mellom 0600 og 2000. Bruk fornuftige forkortelser og hold deg innenfor tegnbegrensningen.",
		maxLen, strings.Join(formatted, ", "))

	reply, err := CallOpenAIWithPrompt(h.apiKey, h.model, "Analyser værmelding og lag korte vurderinger av store endringer.", prompt, maxLen)
	if err != nil {
		return ""
	}

	return reply
}

func getHour(timeStr string) int {
	parts := strings.Split(timeStr, "T")
	if len(parts) < 2 {
		return 0
	}
	hourStr := parts[1][:2]
	h, _ := parseInt(hourStr)
	return h
}

func parseInt(s string) (int, error) {
	var n int
	for _, c := range s {
		if c >= '0' && c <= '9' {
			n = n*10 + int(c-'0')
		}
	}
	return n, nil
}
