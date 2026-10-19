package reports

import "strings"

// zonePlace is where a time zone puts a client until someone sets the real
// place: the zone's representative city.
type zonePlace struct {
	City    string
	Country string // ISO 3166-1 alpha-2
	Lat     float64
	Lng     float64
	IANA    string
}

// zonePlaces are the Rails time zones (ActiveSupport::TimeZone::MAPPING),
// each at the city it's named for or, for a region's zone, its best-known
// city ("Eastern Time (US & Canada)" is New York).
var zonePlaces = map[string]zonePlace{
	"Abu Dhabi":                    {"Abu Dhabi", "AE", 24.45, 54.38, "Asia/Muscat"},
	"Adelaide":                     {"Adelaide", "AU", -34.93, 138.60, "Australia/Adelaide"},
	"Alaska":                       {"Juneau", "US", 58.30, -134.42, "America/Juneau"},
	"Almaty":                       {"Almaty", "KZ", 43.24, 76.89, "Asia/Almaty"},
	"American Samoa":               {"Pago Pago", "AS", -14.28, -170.70, "Pacific/Pago_Pago"},
	"Amsterdam":                    {"Amsterdam", "NL", 52.37, 4.90, "Europe/Amsterdam"},
	"Arizona":                      {"Phoenix", "US", 33.45, -112.07, "America/Phoenix"},
	"Astana":                       {"Astana", "KZ", 51.17, 71.45, "Asia/Almaty"},
	"Asuncion":                     {"Asunción", "PY", -25.26, -57.58, "America/Asuncion"},
	"Athens":                       {"Athens", "GR", 37.98, 23.73, "Europe/Athens"},
	"Atlantic Time (Canada)":       {"Halifax", "CA", 44.65, -63.57, "America/Halifax"},
	"Auckland":                     {"Auckland", "NZ", -36.85, 174.76, "Pacific/Auckland"},
	"Azores":                       {"Ponta Delgada", "PT", 37.74, -25.67, "Atlantic/Azores"},
	"Baghdad":                      {"Baghdad", "IQ", 33.31, 44.36, "Asia/Baghdad"},
	"Baku":                         {"Baku", "AZ", 40.41, 49.87, "Asia/Baku"},
	"Bangkok":                      {"Bangkok", "TH", 13.76, 100.50, "Asia/Bangkok"},
	"Beijing":                      {"Beijing", "CN", 39.90, 116.41, "Asia/Shanghai"},
	"Belgrade":                     {"Belgrade", "RS", 44.79, 20.45, "Europe/Belgrade"},
	"Berlin":                       {"Berlin", "DE", 52.52, 13.40, "Europe/Berlin"},
	"Bern":                         {"Bern", "CH", 46.95, 7.45, "Europe/Zurich"},
	"Bogota":                       {"Bogotá", "CO", 4.71, -74.07, "America/Bogota"},
	"Brasilia":                     {"Brasília", "BR", -15.79, -47.88, "America/Sao_Paulo"},
	"Bratislava":                   {"Bratislava", "SK", 48.15, 17.11, "Europe/Bratislava"},
	"Brisbane":                     {"Brisbane", "AU", -27.47, 153.03, "Australia/Brisbane"},
	"Brussels":                     {"Brussels", "BE", 50.85, 4.35, "Europe/Brussels"},
	"Bucharest":                    {"Bucharest", "RO", 44.43, 26.10, "Europe/Bucharest"},
	"Budapest":                     {"Budapest", "HU", 47.50, 19.04, "Europe/Budapest"},
	"Buenos Aires":                 {"Buenos Aires", "AR", -34.60, -58.38, "America/Argentina/Buenos_Aires"},
	"Cairo":                        {"Cairo", "EG", 30.04, 31.24, "Africa/Cairo"},
	"Canberra":                     {"Canberra", "AU", -35.28, 149.13, "Australia/Canberra"},
	"Cape Verde Is.":               {"Praia", "CV", 14.93, -23.51, "Atlantic/Cape_Verde"},
	"Caracas":                      {"Caracas", "VE", 10.48, -66.90, "America/Caracas"},
	"Casablanca":                   {"Casablanca", "MA", 33.57, -7.59, "Africa/Casablanca"},
	"Central America":              {"Guatemala City", "GT", 14.63, -90.51, "America/Guatemala"},
	"Central Time (US & Canada)":   {"Chicago", "US", 41.88, -87.63, "America/Chicago"},
	"Chatham Is.":                  {"Waitangi", "NZ", -43.95, -176.56, "Pacific/Chatham"},
	"Chennai":                      {"Chennai", "IN", 13.08, 80.27, "Asia/Kolkata"},
	"Chihuahua":                    {"Chihuahua", "MX", 28.63, -106.09, "America/Chihuahua"},
	"Chongqing":                    {"Chongqing", "CN", 29.56, 106.55, "Asia/Chongqing"},
	"Copenhagen":                   {"Copenhagen", "DK", 55.68, 12.57, "Europe/Copenhagen"},
	"Darwin":                       {"Darwin", "AU", -12.46, 130.84, "Australia/Darwin"},
	"Dhaka":                        {"Dhaka", "BD", 23.81, 90.41, "Asia/Dhaka"},
	"Dublin":                       {"Dublin", "IE", 53.35, -6.26, "Europe/Dublin"},
	"Eastern Time (US & Canada)":   {"New York", "US", 40.71, -74.01, "America/New_York"},
	"Edinburgh":                    {"Edinburgh", "GB", 55.95, -3.19, "Europe/London"},
	"Ekaterinburg":                 {"Yekaterinburg", "RU", 56.84, 60.61, "Asia/Yekaterinburg"},
	"Fiji":                         {"Suva", "FJ", -18.14, 178.44, "Pacific/Fiji"},
	"Georgetown":                   {"Georgetown", "GY", 6.80, -58.16, "America/Guyana"},
	"Greenland":                    {"Nuuk", "GL", 64.18, -51.72, "America/Nuuk"},
	"Guadalajara":                  {"Guadalajara", "MX", 20.66, -103.35, "America/Mexico_City"},
	"Guam":                         {"Hagåtña", "GU", 13.48, 144.75, "Pacific/Guam"},
	"Hanoi":                        {"Hanoi", "VN", 21.03, 105.85, "Asia/Bangkok"},
	"Harare":                       {"Harare", "ZW", -17.83, 31.05, "Africa/Harare"},
	"Hawaii":                       {"Honolulu", "US", 21.31, -157.86, "Pacific/Honolulu"},
	"Helsinki":                     {"Helsinki", "FI", 60.17, 24.94, "Europe/Helsinki"},
	"Hobart":                       {"Hobart", "AU", -42.88, 147.33, "Australia/Hobart"},
	"Hong Kong":                    {"Hong Kong", "HK", 22.32, 114.17, "Asia/Hong_Kong"},
	"Indiana (East)":               {"Indianapolis", "US", 39.77, -86.16, "America/Indiana/Indianapolis"},
	"International Date Line West": {"Baker Island", "UM", 0.19, -176.48, "Etc/GMT+12"},
	"Irkutsk":                      {"Irkutsk", "RU", 52.29, 104.28, "Asia/Irkutsk"},
	"Islamabad":                    {"Islamabad", "PK", 33.68, 73.05, "Asia/Karachi"},
	"Istanbul":                     {"Istanbul", "TR", 41.01, 28.98, "Europe/Istanbul"},
	"Jakarta":                      {"Jakarta", "ID", -6.21, 106.85, "Asia/Jakarta"},
	"Jerusalem":                    {"Jerusalem", "IL", 31.77, 35.21, "Asia/Jerusalem"},
	"Kabul":                        {"Kabul", "AF", 34.56, 69.21, "Asia/Kabul"},
	"Kaliningrad":                  {"Kaliningrad", "RU", 54.71, 20.51, "Europe/Kaliningrad"},
	"Kamchatka":                    {"Petropavlovsk-Kamchatsky", "RU", 53.04, 158.65, "Asia/Kamchatka"},
	"Karachi":                      {"Karachi", "PK", 24.86, 67.01, "Asia/Karachi"},
	"Kathmandu":                    {"Kathmandu", "NP", 27.72, 85.32, "Asia/Kathmandu"},
	"Kolkata":                      {"Kolkata", "IN", 22.57, 88.36, "Asia/Kolkata"},
	"Krasnoyarsk":                  {"Krasnoyarsk", "RU", 56.01, 92.89, "Asia/Krasnoyarsk"},
	"Kuala Lumpur":                 {"Kuala Lumpur", "MY", 3.14, 101.69, "Asia/Kuala_Lumpur"},
	"Kuwait":                       {"Kuwait City", "KW", 29.38, 47.99, "Asia/Kuwait"},
	"Kyiv":                         {"Kyiv", "UA", 50.45, 30.52, "Europe/Kyiv"},
	"La Paz":                       {"La Paz", "BO", -16.49, -68.12, "America/La_Paz"},
	"Lima":                         {"Lima", "PE", -12.05, -77.04, "America/Lima"},
	"Lisbon":                       {"Lisbon", "PT", 38.72, -9.14, "Europe/Lisbon"},
	"Ljubljana":                    {"Ljubljana", "SI", 46.06, 14.51, "Europe/Ljubljana"},
	"London":                       {"London", "GB", 51.51, -0.13, "Europe/London"},
	"Madrid":                       {"Madrid", "ES", 40.42, -3.70, "Europe/Madrid"},
	"Magadan":                      {"Magadan", "RU", 59.56, 150.80, "Asia/Magadan"},
	"Marshall Is.":                 {"Majuro", "MH", 7.09, 171.38, "Pacific/Majuro"},
	"Mazatlan":                     {"Mazatlán", "MX", 23.25, -106.41, "America/Mazatlan"},
	"Melbourne":                    {"Melbourne", "AU", -37.81, 144.96, "Australia/Melbourne"},
	"Mexico City":                  {"Mexico City", "MX", 19.43, -99.13, "America/Mexico_City"},
	"Mid-Atlantic":                 {"King Edward Point", "GS", -54.28, -36.51, "Atlantic/South_Georgia"},
	"Midway Island":                {"Midway", "UM", 28.21, -177.38, "Pacific/Midway"},
	"Minsk":                        {"Minsk", "BY", 53.90, 27.56, "Europe/Minsk"},
	"Monrovia":                     {"Monrovia", "LR", 6.30, -10.80, "Africa/Monrovia"},
	"Monterrey":                    {"Monterrey", "MX", 25.69, -100.32, "America/Monterrey"},
	"Montevideo":                   {"Montevideo", "UY", -34.90, -56.16, "America/Montevideo"},
	"Moscow":                       {"Moscow", "RU", 55.76, 37.62, "Europe/Moscow"},
	"Mountain Time (US & Canada)":  {"Denver", "US", 39.74, -104.99, "America/Denver"},
	"Mumbai":                       {"Mumbai", "IN", 19.08, 72.88, "Asia/Kolkata"},
	"Muscat":                       {"Muscat", "OM", 23.59, 58.41, "Asia/Muscat"},
	"Nairobi":                      {"Nairobi", "KE", -1.29, 36.82, "Africa/Nairobi"},
	"New Caledonia":                {"Nouméa", "NC", -22.28, 166.46, "Pacific/Noumea"},
	"New Delhi":                    {"New Delhi", "IN", 28.61, 77.21, "Asia/Kolkata"},
	"Newfoundland":                 {"St. John's", "CA", 47.56, -52.71, "America/St_Johns"},
	"Novosibirsk":                  {"Novosibirsk", "RU", 55.01, 82.93, "Asia/Novosibirsk"},
	"Nuku'alofa":                   {"Nukuʻalofa", "TO", -21.14, -175.20, "Pacific/Tongatapu"},
	"Osaka":                        {"Osaka", "JP", 34.69, 135.50, "Asia/Tokyo"},
	"Pacific Time (US & Canada)":   {"Los Angeles", "US", 34.05, -118.24, "America/Los_Angeles"},
	"Paris":                        {"Paris", "FR", 48.86, 2.35, "Europe/Paris"},
	"Perth":                        {"Perth", "AU", -31.95, 115.86, "Australia/Perth"},
	"Port Moresby":                 {"Port Moresby", "PG", -9.44, 147.18, "Pacific/Port_Moresby"},
	"Prague":                       {"Prague", "CZ", 50.08, 14.44, "Europe/Prague"},
	"Pretoria":                     {"Pretoria", "ZA", -25.75, 28.19, "Africa/Johannesburg"},
	"Puerto Rico":                  {"San Juan", "PR", 18.47, -66.11, "America/Puerto_Rico"},
	"Quito":                        {"Quito", "EC", -0.18, -78.47, "America/Lima"},
	"Rangoon":                      {"Yangon", "MM", 16.84, 96.17, "Asia/Yangon"},
	"Riga":                         {"Riga", "LV", 56.95, 24.11, "Europe/Riga"},
	"Riyadh":                       {"Riyadh", "SA", 24.71, 46.68, "Asia/Riyadh"},
	"Rome":                         {"Rome", "IT", 41.90, 12.50, "Europe/Rome"},
	"Samara":                       {"Samara", "RU", 53.20, 50.15, "Europe/Samara"},
	"Samoa":                        {"Apia", "WS", -13.83, -171.76, "Pacific/Apia"},
	"Santiago":                     {"Santiago", "CL", -33.45, -70.67, "America/Santiago"},
	"Sapporo":                      {"Sapporo", "JP", 43.06, 141.35, "Asia/Tokyo"},
	"Sarajevo":                     {"Sarajevo", "BA", 43.86, 18.41, "Europe/Sarajevo"},
	"Saskatchewan":                 {"Regina", "CA", 50.45, -104.62, "America/Regina"},
	"Seoul":                        {"Seoul", "KR", 37.57, 126.98, "Asia/Seoul"},
	"Singapore":                    {"Singapore", "SG", 1.35, 103.82, "Asia/Singapore"},
	"Skopje":                       {"Skopje", "MK", 42.00, 21.43, "Europe/Skopje"},
	"Sofia":                        {"Sofia", "BG", 42.70, 23.32, "Europe/Sofia"},
	"Solomon Is.":                  {"Honiara", "SB", -9.43, 159.95, "Pacific/Guadalcanal"},
	"Srednekolymsk":                {"Srednekolymsk", "RU", 67.45, 153.68, "Asia/Srednekolymsk"},
	"Sri Jayawardenepura":          {"Colombo", "LK", 6.89, 79.90, "Asia/Colombo"},
	"St. Petersburg":               {"St. Petersburg", "RU", 59.93, 30.34, "Europe/Moscow"},
	"Stockholm":                    {"Stockholm", "SE", 59.33, 18.07, "Europe/Stockholm"},
	"Sydney":                       {"Sydney", "AU", -33.87, 151.21, "Australia/Sydney"},
	"Taipei":                       {"Taipei", "TW", 25.03, 121.57, "Asia/Taipei"},
	"Tallinn":                      {"Tallinn", "EE", 59.44, 24.75, "Europe/Tallinn"},
	"Tashkent":                     {"Tashkent", "UZ", 41.30, 69.24, "Asia/Tashkent"},
	"Tbilisi":                      {"Tbilisi", "GE", 41.72, 44.79, "Asia/Tbilisi"},
	"Tehran":                       {"Tehran", "IR", 35.69, 51.39, "Asia/Tehran"},
	"Tijuana":                      {"Tijuana", "MX", 32.51, -117.04, "America/Tijuana"},
	"Tokelau Is.":                  {"Fakaofo", "TK", -9.38, -171.22, "Pacific/Fakaofo"},
	"Tokyo":                        {"Tokyo", "JP", 35.68, 139.69, "Asia/Tokyo"},
	"UTC":                          {"Greenwich", "GB", 51.48, 0.00, "Etc/UTC"},
	"Ulaanbaatar":                  {"Ulaanbaatar", "MN", 47.89, 106.91, "Asia/Ulaanbaatar"},
	"Urumqi":                       {"Ürümqi", "CN", 43.83, 87.62, "Asia/Urumqi"},
	"Vienna":                       {"Vienna", "AT", 48.21, 16.37, "Europe/Vienna"},
	"Vilnius":                      {"Vilnius", "LT", 54.69, 25.28, "Europe/Vilnius"},
	"Vladivostok":                  {"Vladivostok", "RU", 43.12, 131.89, "Asia/Vladivostok"},
	"Volgograd":                    {"Volgograd", "RU", 48.71, 44.51, "Europe/Volgograd"},
	"Warsaw":                       {"Warsaw", "PL", 52.23, 21.01, "Europe/Warsaw"},
	"Wellington":                   {"Wellington", "NZ", -41.29, 174.78, "Pacific/Auckland"},
	"West Central Africa":          {"Lagos", "NG", 6.52, 3.38, "Africa/Algiers"},
	"Yakutsk":                      {"Yakutsk", "RU", 62.03, 129.73, "Asia/Yakutsk"},
	"Yerevan":                      {"Yerevan", "AM", 40.18, 44.51, "Asia/Yerevan"},
	"Zagreb":                       {"Zagreb", "HR", 45.81, 15.98, "Europe/Zagreb"},
	"Zurich":                       {"Zurich", "CH", 47.38, 8.54, "Europe/Zurich"},
}

// placeOfZone is where a stored zone (a Rails name, or an IANA one) puts a
// client, and whether it's one we know. An IANA zone we have no Rails
// name for is placed by its city's name when it's one of ours.
func placeOfZone(zone string) (zonePlace, bool) {
	if p, ok := zonePlaces[zone]; ok {
		return p, true
	}
	if zone == "" {
		return zonePlace{}, false
	}
	// An IANA name: the Rails zone named for its city first ("Europe/Paris"
	// is Paris), then any that maps to it.
	city := strings.ReplaceAll(zone[strings.LastIndex(zone, "/")+1:], "_", " ")
	if p, ok := zonePlaces[city]; ok && p.IANA == zone {
		return p, true
	}
	best, found := zonePlace{}, false
	for name, p := range zonePlaces {
		if p.IANA == zone && (!found || name < best.City) {
			best, found = p, true
		}
	}
	return best, found
}

// ianaOf is a stored zone's IANA name, for the app to tell the time there.
func ianaOf(zone string) string {
	if p, ok := zonePlaces[zone]; ok {
		return p.IANA
	}
	return zone
}

// countries are the names of the countries the zones are in, and the
// usual others, by ISO code; a place found by search brings its own.
var countries = map[string]string{
	"AE": "United Arab Emirates", "AF": "Afghanistan", "AM": "Armenia", "AR": "Argentina", "AS": "American Samoa", "AT": "Austria",
	"AU": "Australia", "AZ": "Azerbaijan", "BA": "Bosnia and Herzegovina", "BD": "Bangladesh", "BE": "Belgium", "BG": "Bulgaria",
	"BO": "Bolivia", "BR": "Brazil", "BY": "Belarus", "CA": "Canada", "CH": "Switzerland", "CL": "Chile", "CN": "China",
	"CO": "Colombia", "CV": "Cape Verde", "CZ": "Czechia", "DE": "Germany", "DK": "Denmark", "DZ": "Algeria", "EC": "Ecuador",
	"EE": "Estonia", "EG": "Egypt", "ES": "Spain", "FI": "Finland", "FJ": "Fiji", "FR": "France", "GB": "United Kingdom",
	"GE": "Georgia", "GL": "Greenland", "GR": "Greece", "GS": "South Georgia", "GT": "Guatemala", "GU": "Guam", "GY": "Guyana",
	"HK": "Hong Kong", "HR": "Croatia", "HU": "Hungary", "ID": "Indonesia", "IE": "Ireland", "IL": "Israel", "IN": "India",
	"IQ": "Iraq", "IR": "Iran", "IT": "Italy", "JP": "Japan", "KE": "Kenya", "KR": "South Korea", "KW": "Kuwait",
	"KZ": "Kazakhstan", "LK": "Sri Lanka", "LR": "Liberia", "LT": "Lithuania", "LV": "Latvia", "MA": "Morocco", "MH": "Marshall Islands",
	"MK": "North Macedonia", "MM": "Myanmar", "MN": "Mongolia", "MX": "Mexico", "MY": "Malaysia", "NC": "New Caledonia", "NG": "Nigeria",
	"NL": "Netherlands", "NO": "Norway", "NP": "Nepal", "NZ": "New Zealand", "OM": "Oman", "PE": "Peru", "PG": "Papua New Guinea",
	"PK": "Pakistan", "PL": "Poland", "PR": "Puerto Rico", "PT": "Portugal", "PY": "Paraguay", "RO": "Romania", "RS": "Serbia",
	"RU": "Russia", "SA": "Saudi Arabia", "SB": "Solomon Islands", "SE": "Sweden", "SG": "Singapore", "SI": "Slovenia", "SK": "Slovakia",
	"TH": "Thailand", "TK": "Tokelau", "TO": "Tonga", "TR": "Türkiye", "TW": "Taiwan", "UA": "Ukraine", "UM": "U.S. Outlying Islands",
	"US": "United States", "UY": "Uruguay", "UZ": "Uzbekistan", "VE": "Venezuela", "VN": "Vietnam", "WS": "Samoa", "ZA": "South Africa",
	"ZW": "Zimbabwe",
}

// countryName is a country's name by its code, else the code.
func countryName(code string) string {
	if n, ok := countries[strings.ToUpper(code)]; ok {
		return n
	}
	return strings.ToUpper(code)
}
