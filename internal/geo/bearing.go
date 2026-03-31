package geo

import "math"

// Bearing calculates the compass bearing in degrees from start to end.
// start and end are [lon, lat] pairs matching ORS polyline format.
func Bearing(start, end [2]float64) float64 {
	lat1 := toRad(start[1])
	lon1 := toRad(start[0])
	lat2 := toRad(end[1])
	lon2 := toRad(end[0])

	dLon := lon2 - lon1
	y := math.Sin(dLon) * math.Cos(lat2)
	x := math.Cos(lat1)*math.Sin(lat2) - math.Sin(lat1)*math.Cos(lat2)*math.Cos(dLon)

	bearing := math.Atan2(y, x)
	degrees := math.Mod(toDeg(bearing)+360, 360)
	return math.Round(degrees)
}

// Distance returns the distance in km between two [lon, lat] coordinate pairs.
func Distance(coord1, coord2 [2]float64) float64 {
	return Haversine(coord1[1], coord1[0], coord2[1], coord2[0])
}
