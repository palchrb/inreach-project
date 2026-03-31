package store

import "sync"

// ShelterResult represents a single hut/cabin result.
type ShelterResult struct {
	Name          string
	Lat           float64
	Lon           float64
	Distance      float64
	ElevationDiff float64
	Source        string
}

// ShelterState stores the last shelter query results per conversation.
type ShelterState struct {
	mu   sync.RWMutex
	data map[string]*ShelterData
}

// ShelterData holds the user's position and the hut results.
type ShelterData struct {
	Lat  float64
	Lon  float64
	Huts []ShelterResult
}

// NewShelterState creates a new shelter state store.
func NewShelterState() *ShelterState {
	return &ShelterState{
		data: make(map[string]*ShelterData),
	}
}

// Set stores shelter data for a conversation.
func (s *ShelterState) Set(conversationID string, data *ShelterData) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[conversationID] = data
}

// Get retrieves shelter data for a conversation.
func (s *ShelterState) Get(conversationID string) *ShelterData {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.data[conversationID]
}
