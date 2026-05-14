package minutes

import (
	"fmt"
	"net/url"
	"strings"

	apperrors "openminutes/internal/errors"
)

// NormalizeBaseURL validates and normalizes an API base URL.
func NormalizeBaseURL(fieldName, rawURL string) (string, error) {
	rawURL = strings.TrimSpace(rawURL)
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", invalidBaseURLError(fieldName, rawURL)
	}

	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
	default:
		return "", invalidBaseURLError(fieldName, rawURL)
	}

	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", invalidBaseURLError(fieldName, rawURL)
	}

	parsed.Path = strings.TrimRight(parsed.Path, "/")
	return parsed.String(), nil
}

// NormalizeBaseURLOrDefault applies defaultURL when rawURL is empty, then
// validates and normalizes the result.
func NormalizeBaseURLOrDefault(fieldName, rawURL, defaultURL string) (string, bool, error) {
	rawURL = strings.TrimSpace(rawURL)
	defaulted := rawURL == ""
	if defaulted {
		rawURL = defaultURL
	}

	normalized, err := NormalizeBaseURL(fieldName, rawURL)
	return normalized, defaulted, err
}

func invalidBaseURLError(fieldName, rawURL string) error {
	return apperrors.Wrap(apperrors.KindConfig, fmt.Errorf("invalid %s %q: must be an absolute http or https URL with a host", fieldName, rawURL))
}
