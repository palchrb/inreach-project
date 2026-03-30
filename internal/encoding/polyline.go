package encoding

// DecodePolyline decodes a Google encoded polyline string into [lon, lat] coordinates.
func DecodePolyline(encoded string) [][2]float64 {
	var coords [][2]float64
	index := 0
	lat := 0
	lon := 0

	for index < len(encoded) {
		// Decode latitude
		shift := 0
		result := 0
		for {
			b := int(encoded[index]) - 63
			index++
			result |= (b & 0x1f) << shift
			shift += 5
			if b < 0x20 {
				break
			}
		}
		if result&1 != 0 {
			lat += ^(result >> 1)
		} else {
			lat += result >> 1
		}

		// Decode longitude
		shift = 0
		result = 0
		for {
			b := int(encoded[index]) - 63
			index++
			result |= (b & 0x1f) << shift
			shift += 5
			if b < 0x20 {
				break
			}
		}
		if result&1 != 0 {
			lon += ^(result >> 1)
		} else {
			lon += result >> 1
		}

		coords = append(coords, [2]float64{float64(lon) / 1e5, float64(lat) / 1e5})
	}

	return coords
}
