package command

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// AvalancheHandler handles "skred" commands.
type AvalancheHandler struct{}

func NewAvalancheHandler() *AvalancheHandler { return &AvalancheHandler{} }
func (h *AvalancheHandler) Name() string     { return "avalanche" }

func (h *AvalancheHandler) Handle(cc *CommandContext) ([]string, error) {
	if cc.Lat == nil || cc.Lon == nil {
		return []string{"No coordinates found for avalanche request."}, nil
	}

	result, err := processAvalancheData(*cc.Lat, *cc.Lon)
	if err != nil {
		return nil, err
	}

	return []string{result}, nil
}

type avalancheWarning struct {
	DangerLevel       string `json:"DangerLevel"`
	RegionName        string `json:"RegionName"`
	MainText          string `json:"MainText"`
	AvalancheDanger   string `json:"AvalancheDanger"`
	AvalancheProblems []struct {
		AvalancheProblemTypeId   int    `json:"AvalancheProblemTypeId"`
		AvalCauseId              int    `json:"AvalCauseId"`
		AvalPropagationId        int    `json:"AvalPropagationId"`
		AvalTriggerSensitivityId int    `json:"AvalTriggerSensitivityId"`
		DestructiveSizeExtId     int    `json:"DestructiveSizeExtId"`
		ExposedHeight1           int    `json:"ExposedHeight1"`
		ExposedHeightFill        string `json:"ExposedHeightFill"`
		ValidExpositions         string `json:"ValidExpositions"`
	} `json:"AvalancheProblems"`
}

func processAvalancheData(lat, lon float64) (string, error) {
	today := time.Now()
	startDate := formatAvalancheDate(today)
	endDate := formatAvalancheDate(today.Add(2 * 24 * time.Hour))

	url := fmt.Sprintf("https://api01.nve.no/hydrology/forecast/avalanche/v6.3.0/api/AvalancheWarningByCoordinates/Detail/%.4f/%.4f/en/%s/%s",
		lat, lon, startDate, endDate)

	client := &http.Client{Timeout: 15 * time.Second}
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("User-Agent", "InReachAvalancheApp/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("avalanche API request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("avalanche API error: HTTP %d", resp.StatusCode)
	}

	var warnings []avalancheWarning
	if err := json.NewDecoder(resp.Body).Decode(&warnings); err != nil {
		return "", fmt.Errorf("parsing avalanche data: %w", err)
	}

	if len(warnings) == 0 {
		return "Ingen data funnet for de angitte koordinatene.", nil
	}

	// Combine danger levels
	var dangerLevels string
	for _, w := range warnings {
		if w.DangerLevel == "" {
			dangerLevels += "0"
		} else {
			dangerLevels += w.DangerLevel
		}
	}

	// Encode avalanche problems from today's warning
	todayWarning := warnings[0]
	var encodedProblems string

	for _, p := range todayWarning.AvalancheProblems {
		problemType := recodeAvalancheProblemType(p.AvalancheProblemTypeId)
		cause := recodeAvalCause(p.AvalCauseId)
		propagation := encodeBase36(p.AvalPropagationId)
		sensitivity := recodeAvalTriggerSensitivity(p.AvalTriggerSensitivityId)
		destructiveSize := encodeBase36(p.DestructiveSizeExtId)
		height := encodeBase36(p.ExposedHeight1 / 100)
		heightQualifier := p.ExposedHeightFill
		if heightQualifier == "" {
			heightQualifier = "0"
		}
		directions := encodeDirections(p.ValidExpositions)

		encodedProblems += fmt.Sprintf("%s%s%s%s%s%s%s%s",
			problemType, cause, propagation, sensitivity,
			destructiveSize, height, heightQualifier, directions)
	}

	mainText := todayWarning.MainText
	if mainText == "" {
		mainText = "Ingen hovedmelding tilgjengelig."
	}

	return fmt.Sprintf("%s%s;%s", dangerLevels, encodedProblems, mainText), nil
}

func formatAvalancheDate(t time.Time) string {
	return fmt.Sprintf("%02d%02d%04d", t.Day(), t.Month(), t.Year())
}

func encodeBase36(value int) string {
	return strings.ToLower(strconv.FormatInt(int64(value), 36))
}

func recodeAvalancheProblemType(apiValue int) string {
	mapping := map[int]int{0: 0, 3: 1, 5: 2, 7: 3, 10: 4, 30: 5, 45: 6, 50: 7}
	v, ok := mapping[apiValue]
	if !ok {
		v = 0
	}
	return strconv.FormatInt(int64(v), 36)
}

func recodeAvalCause(apiValue int) string {
	mapping := map[int]string{
		0: "0", 10: "1", 11: "2", 13: "3", 14: "4", 15: "5",
		16: "6", 18: "7", 19: "8", 20: "9", 22: "a", 24: "b",
	}
	v, ok := mapping[apiValue]
	if !ok {
		v = "0"
	}
	return v
}

func recodeAvalTriggerSensitivity(apiValue int) string {
	mapping := map[int]int{0: 0, 10: 1, 20: 2, 30: 3, 40: 4, 45: 5}
	v, ok := mapping[apiValue]
	if !ok {
		v = 0
	}
	return strconv.FormatInt(int64(v), 36)
}

func encodeDirections(binaryStr string) string {
	if len(binaryStr) != 8 {
		return "00"
	}
	val, err := strconv.ParseInt(binaryStr, 2, 64)
	if err != nil {
		return "00"
	}
	encoded := strconv.FormatInt(val, 36)
	for len(encoded) < 2 {
		encoded = "0" + encoded
	}
	return encoded
}
