package utils

import (
	"fmt"
	"net/url"
	"strings"
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

func ValidateURL(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", fmt.Errorf("url is empty")
	}

	// if no scheme, assume http
	if !strings.Contains(s, "://") {
		s = "http://" + s
	}

	u, err := url.ParseRequestURI(s)
	if err != nil {
		return "", fmt.Errorf("invalid url: %w", err)
	}

	// only allow http or https
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("unsupported scheme %q; only http and https are allowed", u.Scheme)
	}
	if u.Host == "" {
		return "", fmt.Errorf("url missing host")
	}

	return strings.TrimSuffix(s, "/"), nil
}
