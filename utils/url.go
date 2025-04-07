package utils

import (
	"net/url"
)

func IsValidURL(str string) bool {
	_, err := url.ParseRequestURI(str)
	return err == nil
}

func IsValidLnUrl(str string) bool {
	u, err := url.Parse(str)
	if err != nil {
		return false
	}
	if u.Scheme == "lndconnect" || u.Scheme == "clnconnect" {
		return true
	}
	return false
}
