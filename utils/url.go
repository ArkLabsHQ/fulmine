package utils

import (
	"net/url"
)

func IsValidURL(str string) bool {
	_, err := url.ParseRequestURI(str)
	return err == nil
}
