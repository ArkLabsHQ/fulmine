package web

import "time"

func prettyUnixTimestamp(unixTime int64) string {
	// return time.Unix(unixTime, 0).Format(time.RFC3339) // Adjust format as needed
	return time.Unix(unixTime, 0).Format("02/01/2006 15:04")
}

func prettyDay(unixTime int64) string {
	if unixTime == 0 {
		return "0"
	}
	return time.Unix(unixTime, 0).Format("02/01/2006")
}

func prettyHour(unixTime int64) string {
	if unixTime == 0 {
		return "0"
	}
	return time.Unix(unixTime, 0).Format("15:04")
}
