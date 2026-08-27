package config

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/vineet-motwani/Tross-Hiring/session" // Replace with actual module path if different

	"github.com/aws/aws-sdk-go-v2/aws"
	awscfg "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
)

type Settings struct {
	AppName                       string
	AppVersion                    string
	LogLevel                      string
	LinkedinLiAt                  *string
	LinkedinJsessionid            *string
	LinkedinCookieHeader          *string
	LinkedinSecretArn             *string
	LinkedinSecretCacheTtlSeconds int
	LinkedinUserAgent             string
	LinkedinRequestTimeoutSeconds float64
	LinkedinTotalTimeoutSeconds   float64
	LinkedinMaxRetries            int
	LinkedinMaxUpstreamRequests   int
	LinkedinMaxResponseBytes      int
	LinkedinFetchSectionFallbacks bool
	AllowContactInfo              bool
	AwsRegion                     string
	CacheTtlSeconds               int
	CacheMaxEntries               int
	RateLimitPerMinute            int
	RateLimitMaxClients           int
	MaxRequestBodyBytes           int
}

func getEnvStr(key, defaultVal string) string {
	if val, ok := os.LookupEnv(key); ok {
		return val
	}
	return defaultVal
}

func getEnvStrPtr(key string) *string {
	if val, ok := os.LookupEnv(key); ok {
		return &val
	}
	return nil
}

func getEnvInt(key string, defaultVal int) int {
	if val, ok := os.LookupEnv(key); ok {
		if intVal, err := strconv.Atoi(val); err == nil {
			return intVal
		}
	}
	return defaultVal
}

func getEnvFloat64(key string, defaultVal float64) float64 {
	if val, ok := os.LookupEnv(key); ok {
		if floatVal, err := strconv.ParseFloat(val, 64); err == nil {
			return floatVal
		}
	}
	return defaultVal
}

func getEnvBool(key string, defaultVal bool) bool {
	if val, ok := os.LookupEnv(key); ok {
		if boolVal, err := strconv.ParseBool(val); err == nil {
			return boolVal
		}
	}
	return defaultVal
}

func LoadSettings() *Settings {
	return &Settings{
		AppName:                       getEnvStr("APP_NAME", "LinkedIn Profile API"),
		AppVersion:                    getEnvStr("APP_VERSION", "1.0.0"),
		LogLevel:                      getEnvStr("LOG_LEVEL", "INFO"),
		LinkedinLiAt:                  getEnvStrPtr("LINKEDIN_LI_AT"),
		LinkedinJsessionid:            getEnvStrPtr("LINKEDIN_JSESSIONID"),
		LinkedinCookieHeader:          getEnvStrPtr("LINKEDIN_COOKIE_HEADER"),
		LinkedinSecretArn:             getEnvStrPtr("LINKEDIN_SECRET_ARN"),
		LinkedinSecretCacheTtlSeconds: getEnvInt("LINKEDIN_SECRET_CACHE_TTL_SECONDS", 300),
		LinkedinUserAgent:             getEnvStr("LINKEDIN_USER_AGENT", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/151.0.0.0 Safari/537.36"),
		LinkedinRequestTimeoutSeconds: getEnvFloat64("LINKEDIN_REQUEST_TIMEOUT_SECONDS", 6.0),
		LinkedinTotalTimeoutSeconds:   getEnvFloat64("LINKEDIN_TOTAL_TIMEOUT_SECONDS", 10.0),
		LinkedinMaxRetries:            getEnvInt("LINKEDIN_MAX_RETRIES", 1),
		LinkedinMaxUpstreamRequests:   getEnvInt("LINKEDIN_MAX_UPSTREAM_REQUESTS", 8),
		LinkedinMaxResponseBytes:      getEnvInt("LINKEDIN_MAX_RESPONSE_BYTES", 2000000),
		LinkedinFetchSectionFallbacks: getEnvBool("LINKEDIN_FETCH_SECTION_FALLBACKS", false),
		AllowContactInfo:              getEnvBool("ALLOW_CONTACT_INFO", false),
		AwsRegion:                     getEnvStr("AWS_REGION", "ap-south-1"),
		CacheTtlSeconds:               getEnvInt("CACHE_TTL_SECONDS", 300),
		CacheMaxEntries:               getEnvInt("CACHE_MAX_ENTRIES", 128),
		RateLimitPerMinute:            getEnvInt("RATE_LIMIT_PER_MINUTE", 10),
		RateLimitMaxClients:           getEnvInt("RATE_LIMIT_MAX_CLIENTS", 2048),
		MaxRequestBodyBytes:           getEnvInt("MAX_REQUEST_BODY_BYTES", 4096),
	}
}

type LinkedInCredentials struct {
	LiAt           string
	Jsessionid     string
	SessionCookies []struct{ Key, Value string }
}

func (c *LinkedInCredentials) CsrfToken() string {
	return strings.Trim(c.Jsessionid, `"`)
}

func (c *LinkedInCredentials) Cookies() map[string]string {
	if len(c.SessionCookies) > 0 {
		m := make(map[string]string)
		for _, pair := range c.SessionCookies {
			m[pair.Key] = pair.Value
		}
		return m
	}
	quotedJsessionid := c.Jsessionid
	if !strings.HasPrefix(quotedJsessionid, `"`) {
		quotedJsessionid = `"` + quotedJsessionid + `"`
	}
	return map[string]string{
		"li_at":      c.LiAt,
		"JSESSIONID": quotedJsessionid,
	}
}

type CredentialsUnavailableError struct {
	Message string
	Err     error
}

func (e *CredentialsUnavailableError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Err)
	}
	return e.Message
}

func (e *CredentialsUnavailableError) Unwrap() error {
	return e.Err
}

type CredentialProvider struct {
	settings       *Settings
	cached         *LinkedInCredentials
	cacheExpiresAt time.Time
	mu             sync.Mutex
}

func NewCredentialProvider(settings *Settings) *CredentialProvider {
	return &CredentialProvider{
		settings: settings,
	}
}

func (p *CredentialProvider) Get(ctx context.Context) (*LinkedInCredentials, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.cached != nil && time.Now().Before(p.cacheExpiresAt) {
		return p.cached, nil
	}

	creds, err := p.fromEnvironment()
	var cacheExpiresAt time.Time

	if creds != nil {
		cacheExpiresAt = time.Now().Add(876000 * time.Hour) // practically infinite
	} else if p.settings.LinkedinSecretArn != nil {
		creds, err = p.fromSecretsManager(ctx)
		if err != nil {
			return nil, err
		}
		if creds != nil {
			cacheExpiresAt = time.Now().Add(time.Duration(p.settings.LinkedinSecretCacheTtlSeconds) * time.Second)
		}
	} else {
		cacheExpiresAt = time.Time{}
	}

	if creds == nil && err == nil {
		return nil, &CredentialsUnavailableError{Message: "LinkedIn credentials have not been configured"}
	} else if err != nil {
		return nil, err
	}

	p.cached = creds
	p.cacheExpiresAt = cacheExpiresAt
	return creds, nil
}

func (p *CredentialProvider) Clear() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cached = nil
	p.cacheExpiresAt = time.Time{}
}

func (p *CredentialProvider) fromEnvironment() (*LinkedInCredentials, error) {
	liAt := p.settings.LinkedinLiAt
	jsessionid := p.settings.LinkedinJsessionid

	if liAt == nil || jsessionid == nil {
		return nil, nil
	}

	liAtValue := strings.TrimSpace(*liAt)
	jsessionidValue := strings.TrimSpace(*jsessionid)

	if liAtValue == "" && jsessionidValue == "" {
		return nil, nil
	}

	var cookieHeaderValue *string
	if p.settings.LinkedinCookieHeader != nil {
		cookieHeaderValue = p.settings.LinkedinCookieHeader
	}

	return p.validate(liAtValue, jsessionidValue, nil, cookieHeaderValue)
}

func (p *CredentialProvider) fromSecretsManager(ctx context.Context) (*LinkedInCredentials, error) {
	secretArn := p.settings.LinkedinSecretArn
	if secretArn == nil {
		return nil, &CredentialsUnavailableError{Message: "No LinkedIn secret ARN is configured"}
	}

	cfg, err := awscfg.LoadDefaultConfig(ctx, awscfg.WithRegion(p.settings.AwsRegion))
	if err != nil {
		return nil, &CredentialsUnavailableError{Message: "The configured LinkedIn secret could not be loaded", Err: err}
	}

	client := secretsmanager.NewFromConfig(cfg)
	input := &secretsmanager.GetSecretValueInput{
		SecretId: aws.String(*secretArn),
	}

	result, err := client.GetSecretValue(ctx, input)
	if err != nil {
		return nil, &CredentialsUnavailableError{Message: "The configured LinkedIn secret could not be loaded", Err: err}
	}

	if result.SecretString == nil || *result.SecretString == "" {
		return nil, &CredentialsUnavailableError{Message: "The configured LinkedIn secret is empty"}
	}

	var secretMap map[string]interface{}
	err = json.Unmarshal([]byte(*result.SecretString), &secretMap)
	if err != nil {
		return nil, &CredentialsUnavailableError{Message: "The configured LinkedIn secret is not valid JSON", Err: err}
	}

	liAtAny, _ := secretMap["li_at"]
	jsessionidAny, _ := secretMap["jsessionid"]
	cookiesAny, _ := secretMap["cookies"]
	cookieHeaderAny, _ := secretMap["cookie_header"]

	var liAt, jsessionid, cookieHeader *string
	var cookies map[string]interface{}

	if val, ok := liAtAny.(string); ok {
		liAt = &val
	}
	if val, ok := jsessionidAny.(string); ok {
		jsessionid = &val
	}
	if val, ok := cookieHeaderAny.(string); ok {
		cookieHeader = &val
	}
	if val, ok := cookiesAny.(map[string]interface{}); ok {
		cookies = val
	}

	if liAt == nil || jsessionid == nil {
		return nil, &CredentialsUnavailableError{Message: "The LinkedIn li_at value or JSESSIONID value is invalid"}
	}

	return p.validate(*liAt, *jsessionid, cookies, cookieHeader)
}

func (p *CredentialProvider) validate(liAt string, jsessionid string, cookies map[string]interface{}, cookieHeader *string) (*LinkedInCredentials, error) {
	if len(strings.TrimSpace(liAt)) < 20 {
		return nil, &CredentialsUnavailableError{Message: "The LinkedIn li_at value is invalid"}
	}
	if !strings.HasPrefix(strings.Trim(jsessionid, `"`), "ajax:") {
		return nil, &CredentialsUnavailableError{Message: "The LinkedIn JSESSIONID value is invalid"}
	}

	normalizedLiAt := strings.TrimSpace(liAt)
	normalizedJsessionid := strings.TrimSpace(jsessionid)

	var normalizedCookies map[string]string
	var err error

	if cookies != nil {
		if cookieHeader != nil {
			return nil, &CredentialsUnavailableError{Message: "The LinkedIn secret contains conflicting cookie formats"}
		}
		normalizedCookies, err = session.ValidateCookieMap(cookies, normalizedLiAt, normalizedJsessionid)
	} else if cookieHeader != nil {
		normalizedCookies, err = session.ImportCookieHeader(*cookieHeader, &normalizedLiAt, &normalizedJsessionid)
	} else {
		normalizedCookies = make(map[string]string)
	}

	if err != nil {
		return nil, &CredentialsUnavailableError{Message: err.Error(), Err: err}
	}

	var sessionCookies []struct{ Key, Value string }
	for k, v := range normalizedCookies {
		sessionCookies = append(sessionCookies, struct{ Key, Value string }{k, v})
	}

	return &LinkedInCredentials{
		LiAt:           normalizedLiAt,
		Jsessionid:     normalizedJsessionid,
		SessionCookies: sessionCookies,
	}, nil
}
