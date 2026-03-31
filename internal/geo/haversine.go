package geo

import "math"

// Haversine returns the distance in km between two lat/lon points.
func Haversine(lat1, lon1, lat2, lon2 float64) float64 {
	const R = 6371.0 // Earth's radius in km
	dLat := toRad(lat2 - lat1)
	dLon := toRad(lon2 - lon1)
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(toRad(lat1))*math.Cos(toRad(lat2))*
			math.Sin(dLon/2)*math.Sin(dLon/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	return R * c
}

func toRad(deg float64) float64 {
	return deg * math.Pi / 180.0
}

func toDeg(rad float64) float64 {
	return rad * 180.0 / math.Pi
}
