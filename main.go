package main

import (
	"context"
	"log"
	"net/http"

	"github.com/vineet-motwani/Tross-Hiring/api"
	"github.com/vineet-motwani/Tross-Hiring/cache"
	"github.com/vineet-motwani/Tross-Hiring/config"
	"github.com/vineet-motwani/Tross-Hiring/linkedin"
	"github.com/vineet-motwani/Tross-Hiring/ratelimit"
	"github.com/vineet-motwani/Tross-Hiring/service"
)

func main() {
	runtimeSettings := config.LoadSettings()
	credentialProvider := config.NewCredentialProvider(runtimeSettings)
	
	// Convert config.Settings to linkedin.Settings
	linkedinSettings := linkedin.Settings{
		LinkedInUserAgent:             runtimeSettings.LinkedinUserAgent,
		LinkedInRequestTimeoutSeconds: int(runtimeSettings.LinkedinRequestTimeoutSeconds),
		LinkedInTotalTimeoutSeconds:   int(runtimeSettings.LinkedinTotalTimeoutSeconds),
		LinkedInMaxUpstreamRequests:   runtimeSettings.LinkedinMaxUpstreamRequests,
		LinkedInFetchSectionFallbacks: runtimeSettings.LinkedinFetchSectionFallbacks,
		LinkedInMaxRetries:            runtimeSettings.LinkedinMaxRetries,
		LinkedInMaxResponseBytes:      runtimeSettings.LinkedinMaxResponseBytes,
	}
	
	linkedinClient := linkedin.NewClient(linkedinSettings, credentialProviderAdapter{credentialProvider}, nil)
	
	profileCache := cache.NewProfileCache(
		runtimeSettings.CacheTtlSeconds,
		runtimeSettings.CacheMaxEntries,
	)
	
	svc := service.NewProfileService(
		linkedinClient,
		profileCache,
		runtimeSettings.AllowContactInfo,
	)
	
	limiter := ratelimit.NewInMemoryRateLimiter(
		runtimeSettings.RateLimitPerMinute,
		runtimeSettings.RateLimitMaxClients,
	)

	app := api.CreateApp(runtimeSettings, svc, limiter)

	log.Printf("Starting application on port 8000...")
	if err := app.Run(":8000"); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

type credentialProviderAdapter struct {
	inner *config.CredentialProvider
}

func (a credentialProviderAdapter) Get(ctx context.Context) (*linkedin.Credentials, error) {
	creds, err := a.inner.Get(ctx)
	if err != nil {
		return nil, err
	}
	if creds == nil {
		return nil, nil
	}
	var cookies []*http.Cookie
	for _, c := range creds.SessionCookies {
		cookies = append(cookies, &http.Cookie{Name: c.Key, Value: c.Value})
	}
	return &linkedin.Credentials{
		CSRFToken: creds.CsrfToken(),
		Cookies:   cookies,
	}, nil
}

func (a credentialProviderAdapter) Clear() {
	a.inner.Clear()
}
