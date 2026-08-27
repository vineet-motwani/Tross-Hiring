package utils

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

var (
	publicIdentifierRegexp = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{1,149}$`)
	allowedHosts           = map[string]bool{
		"linkedin.com":     true,
		"www.linkedin.com": true,
		"in.linkedin.com":  true,
	}
)

const MaxProfileURLLength = 512

type LinkedInProfileURL struct {
	PublicIdentifier string
}

func (l LinkedInProfileURL) CanonicalURL() string {
	return fmt.Sprintf("https://www.linkedin.com/in/%s/", l.PublicIdentifier)
}

// InvalidProfileURLError defined here as it maps to the invalid request domain error
type InvalidProfileURLError struct {
	Message string
}

func (e *InvalidProfileURLError) Error() string {
	return e.Message
}

func ParseLinkedInProfileURL(value string) (*LinkedInProfileURL, error) {
	if len(value) > MaxProfileURLLength {
		return nil, &InvalidProfileURLError{"The LinkedIn profile URL is too long"}
	}
	candidate := strings.TrimSpace(value)
	if candidate == "" {
		return nil, &InvalidProfileURLError{"A LinkedIn profile URL is required"}
	}

	for _, ch := range candidate {
		if ch < 32 || ch == 127 {
			return nil, &InvalidProfileURLError{"LinkedIn profile URLs must not contain control characters"}
		}
	}

	if !strings.Contains(candidate, "://") {
		candidate = "https://" + candidate
	}

	parsed, err := url.Parse(candidate)
	if err != nil {
		return nil, &InvalidProfileURLError{"The LinkedIn profile URL is malformed"}
	}

	if parsed.Scheme != "https" {
		return nil, &InvalidProfileURLError{"LinkedIn profile URLs must use HTTPS"}
	}

	hostname := strings.ToLower(parsed.Hostname())
	if !allowedHosts[hostname] {
		return nil, &InvalidProfileURLError{"Only linkedin.com profile URLs are accepted"}
	}

	if parsed.Port() != "" {
		return nil, &InvalidProfileURLError{"LinkedIn profile URLs must not specify a port"}
	}

	if parsed.User != nil {
		return nil, &InvalidProfileURLError{"LinkedIn profile URLs must not contain credentials"}
	}

	pathParts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	var parts []string
	for _, p := range pathParts {
		if p != "" {
			parts = append(parts, p)
		}
	}

	if len(parts) != 2 || strings.ToLower(parts[0]) != "in" {
		return nil, &InvalidProfileURLError{"Expected a LinkedIn URL in the form https://linkedin.com/in/name"}
	}

	identifier := parts[1]
	
	if !publicIdentifierRegexp.MatchString(identifier) {
		return nil, &InvalidProfileURLError{"The LinkedIn public identifier is invalid"}
	}

	return &LinkedInProfileURL{PublicIdentifier: identifier}, nil
}
