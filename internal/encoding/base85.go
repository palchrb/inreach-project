package encoding

// Base85Chars is the 85-character alphabet used for weather symbol encoding.
const Base85Chars = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz!#$%&()*+-;<=>?@^_`{|}~"

// WeatherIconMap maps yr.no weather symbol codes to their index in the Base85 alphabet.
var WeatherIconMap = []string{
	"clearsky_day", "clearsky_night", "clearsky_polartwilight",
	"fair_day", "fair_night", "fair_polartwilight",
	"partlycloudy_day", "partlycloudy_night", "partlycloudy_polartwilight",
	"cloudy",
	"rainshowers_day", "rainshowers_night", "rainshowers_polartwilight",
	"rainshowersandthunder_day", "rainshowersandthunder_night", "rainshowersandthunder_polartwilight",
	"sleetshowers_day", "sleetshowers_night", "sleetshowers_polartwilight",
	"snowshowers_day", "snowshowers_night", "snowshowers_polartwilight",
	"rain", "heavyrain", "heavyrainandthunder",
	"sleet", "snow", "snowandthunder",
	"fog",
	"sleetshowersandthunder_day", "sleetshowersandthunder_night", "sleetshowersandthunder_polartwilight",
	"snowshowersandthunder_day", "snowshowersandthunder_night", "snowshowersandthunder_polartwilight",
	"rainandthunder", "sleetandthunder",
	"lightrainshowersandthunder_day", "lightrainshowersandthunder_night", "lightrainshowersandthunder_polartwilight",
	"heavyrainshowersandthunder_day", "heavyrainshowersandthunder_night", "heavyrainshowersandthunder_polartwilight",
	"lightsleetshowersandthunder_day", "lightsleetshowersandthunder_night", "lightsleetshowersandthunder_polartwilight",
	"heavysleetshowersandthunder_day", "heavysleetshowersandthunder_night", "heavysleetshowersandthunder_polartwilight",
	"lightsnowshowersandthunder_day", "lightsnowshowersandthunder_night", "lightsnowshowersandthunder_polartwilight",
	"heavysnowshowersandthunder_day", "heavysnowshowersandthunder_night", "heavysnowshowersandthunder_polartwilight",
	"lightrainandthunder", "lightsleetandthunder", "heavysleetandthunder",
	"lightsnowandthunder", "heavysnowandthunder",
	"lightrainshowers_day", "lightrainshowers_night", "lightrainshowers_polartwilight",
	"heavyrainshowers_day", "heavyrainshowers_night", "heavyrainshowers_polartwilight",
	"lightsleetshowers_day", "lightsleetshowers_night", "lightsleetshowers_polartwilight",
	"heavysleetshowers_day", "heavysleetshowers_night", "heavysleetshowers_polartwilight",
	"lightsnowshowers_day", "lightsnowshowers_night", "lightsnowshowers_polartwilight",
	"heavysnowshowers_day", "heavysnowshowers_night", "heavysnowshowers_polartwilight",
	"lightrain", "lightsleet", "heavysleet",
	"lightsnow", "heavysnow",
}

// MapWeatherSymbolToBase85 maps a yr.no weather symbol code to a single Base85 character.
func MapWeatherSymbolToBase85(symbol string) string {
	for i, s := range WeatherIconMap {
		if s == symbol {
			if i < len(Base85Chars) {
				return string(Base85Chars[i])
			}
		}
	}
	return "?"
}
