package session

import (
	"fmt"
	"strings"
)

const (
	MaxCookieHeaderLength = 32768
	MaxCookieValueLength  = 8192
)

var SessionCookieNames = []string{
	"bcookie",
	"bscookie",
	"li_rm",
	"li_sugr",
	"timezone",
	"JSESSIONID",
	"lang",
	"li_at",
	"liap",
	"lidc",
}

var sessionCookieNameSet = make(map[string]bool)

func init() {
	for _, name := range SessionCookieNames {
		sessionCookieNameSet[name] = true
	}
}

type SessionCookieError struct {
	Message string
}

func (e *SessionCookieError) Error() string {
	return e.Message
}

func validateText(value string, label string, maximum int) (string, error) {
	normalized := strings.TrimSpace(value)
	if normalized == "" {
		return "", &SessionCookieError{Message: fmt.Sprintf("The %s is empty", label)}
	}
	if len(normalized) > maximum {
		return "", &SessionCookieError{Message: fmt.Sprintf("The %s is too long", label)}
	}
	for _, r := range normalized {
		if r < 0x20 || r > 0x7E {
			return "", &SessionCookieError{Message: fmt.Sprintf("The %s contains unsafe characters", label)}
		}
	}
	return normalized, nil
}

func validateRequiredValues(cookies map[string]string, liAt *string, jsessionid *string) error {
	cookieLiAt, ok := cookies["li_at"]
	if !ok || len(cookieLiAt) < 20 {
		return &SessionCookieError{Message: "The imported cookies do not contain a valid li_at"}
	}
	cookieJsessionid, ok := cookies["JSESSIONID"]
	if !ok || !strings.HasPrefix(strings.Trim(cookieJsessionid, `"`), "ajax:") {
		return &SessionCookieError{Message: "The imported cookies do not contain a valid JSESSIONID"}
	}
	if liAt != nil && cookieLiAt != *liAt {
		return &SessionCookieError{Message: "The imported cookies do not match li_at"}
	}
	if jsessionid != nil && strings.Trim(cookieJsessionid, `"`) != strings.Trim(*jsessionid, `"`) {
		return &SessionCookieError{Message: "The imported cookies do not match JSESSIONID"}
	}
	return nil
}

func ImportCookieHeader(value string, liAt *string, jsessionid *string) (map[string]string, error) {
	header := strings.TrimSpace(value)
	if strings.HasPrefix(strings.ToLower(header), "cookie:") {
		parts := strings.SplitN(header, ":", 2)
		if len(parts) == 2 {
			header = strings.TrimSpace(parts[1])
		}
	}

	header, err := validateText(header, "Cookie header", MaxCookieHeaderLength)
	if err != nil {
		return nil, err
	}

	cookies := make(map[string]string)
	for _, segment := range strings.Split(header, ";") {
		pair := strings.TrimSpace(segment)
		if !strings.Contains(pair, "=") {
			continue
		}
		parts := strings.SplitN(pair, "=", 2)
		name := strings.TrimSpace(parts[0])
		cookieValue := parts[1]

		if !sessionCookieNameSet[name] {
			continue
		}
		if _, exists := cookies[name]; exists {
			return nil, &SessionCookieError{Message: "The Cookie header repeats a session cookie"}
		}

		validatedVal, err := validateText(cookieValue, "session cookie value", MaxCookieValueLength)
		if err != nil {
			return nil, err
		}
		cookies[name] = validatedVal
	}

	err = validateRequiredValues(cookies, liAt, jsessionid)
	if err != nil {
		return nil, err
	}
	return cookies, nil
}

func ValidateCookieMap(value map[string]interface{}, liAt string, jsessionid string) (map[string]string, error) {
	cookies := make(map[string]string)
	for name, cookieValueAny := range value {
		cookieValue, ok := cookieValueAny.(string)
		if !sessionCookieNameSet[name] || !ok {
			return nil, &SessionCookieError{Message: "The LinkedIn session cookies contain an invalid entry"}
		}

		validatedVal, err := validateText(cookieValue, "session cookie value", MaxCookieValueLength)
		if err != nil {
			return nil, err
		}
		cookies[name] = validatedVal
	}
	err := validateRequiredValues(cookies, &liAt, &jsessionid)
	if err != nil {
		return nil, err
	}
	return cookies, nil
}
