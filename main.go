package main

import (
	"context"
	"log"
	"net/http"
	"os"

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

	port := os.Getenv("PORT")
	if port == "" {
		port = "8000"
	}

	log.Printf("Starting application on port %s...", port)
	if err := app.Run(":" + port); err != nil {
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
	for k, v := range creds.Cookies() {
		cookies = append(cookies, &http.Cookie{Name: k, Value: v})
	}
	return &linkedin.Credentials{
		CSRFToken: creds.CsrfToken(),
		Cookies:   cookies,
	}, nil
}

func (a credentialProviderAdapter) Clear() {
	a.inner.Clear()
}
