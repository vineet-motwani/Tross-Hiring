package linkedin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/vineet-motwani/Tross-Hiring/identity"
)

var (
	ErrAuthentication      = errors.New("authentication error")
	ErrProfileNotFound     = errors.New("profile not found")
	ErrUpstreamRateLimited = errors.New("upstream rate limited")
	ErrUpstreamResponse    = errors.New("upstream response error")
	ErrBudgetExhausted     = errors.New("request budget exhausted")
)

type Settings struct {
	LinkedInUserAgent             string
	LinkedInRequestTimeoutSeconds int
	LinkedInTotalTimeoutSeconds   int
	LinkedInMaxUpstreamRequests   int
	LinkedInFetchSectionFallbacks bool
	LinkedInMaxRetries            int
	LinkedInMaxResponseBytes      int
}

type Credentials struct {
	CSRFToken string
	Cookies   []*http.Cookie
}

type CredentialProvider interface {
	Get(ctx context.Context) (*Credentials, error)
	Clear()
}

type FetchResult struct {
	Documents []map[string]interface{}
	Warnings  []string
}

type AttemptBudget struct {
	remaining int
	lock      sync.Mutex
}

func NewAttemptBudget(maximum int) *AttemptBudget {
	return &AttemptBudget{remaining: maximum}
}

func (b *AttemptBudget) Charge() error {
	b.lock.Lock()
	defer b.lock.Unlock()
	if b.remaining <= 0 {
		return ErrBudgetExhausted
	}
	b.remaining--
	return nil
}

type Client struct {
	Settings           Settings
	CredentialProvider CredentialProvider
	HTTPClient         *http.Client
}

const BaseURL = "https://www.linkedin.com/voyager/api"

var ProfileDecorations = []string{
	"com.linkedin.voyager.dash.deco.identity.profile.FullProfileWithEntities-101",
	"com.linkedin.voyager.dash.deco.identity.profile.FullProfileWithEntities-91",
}

var SectionPaths = []string{
	"skills",
	"certifications",
	"languages",
	"projects",
	"publications",
	"courses",
	"honors",
	"volunteerExperiences",
}

func NewClient(settings Settings, credProvider CredentialProvider, transport http.RoundTripper) *Client {
	if transport == nil {
		transport = http.DefaultTransport
	}
	return &Client{
		Settings:           settings,
		CredentialProvider: credProvider,
		HTTPClient: &http.Client{
			Transport: transport,
			Timeout:   time.Duration(settings.LinkedInRequestTimeoutSeconds) * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

func (c *Client) FetchProfile(ctx context.Context, publicIdentifier string, includeContactInfo bool) (*FetchResult, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Duration(c.Settings.LinkedInTotalTimeoutSeconds)*time.Second)
	defer cancel()

	ctx = context.WithValue(ctx, "publicIdentifier", publicIdentifier)
	
	creds, err := c.CredentialProvider.Get(ctx)
	if err != nil {
		return nil, err
	}

	budget := NewAttemptBudget(c.Settings.LinkedInMaxUpstreamRequests)
	result := &FetchResult{
		Documents: make([]map[string]interface{}, 0),
		Warnings:  make([]string, 0),
	}

	primary, err := c.fetchPrimary(ctx, publicIdentifier, budget, creds)
	if err != nil {
		return nil, err
	}
	primary["__source"] = "primary"
	primary["__profile_identifier"] = publicIdentifier
	result.Documents = append(result.Documents, primary)

	if c.Settings.LinkedInFetchSectionFallbacks {
		docs, warns := c.fetchSections(ctx, publicIdentifier, budget, creds)
		result.Documents = append(result.Documents, docs...)
		result.Warnings = append(result.Warnings, warns...)
	}

	if includeContactInfo {
		path := fmt.Sprintf("/identity/profiles/%s/profileContactInfo", publicIdentifier)
		doc, warn := c.optionalGet(ctx, path, "contact information", budget, creds, nil)
		if doc != nil {
			doc["__source"] = "contact"
			doc["__profile_identifier"] = publicIdentifier
			result.Documents = append(result.Documents, doc)
		}
		if warn != "" {
			result.Warnings = append(result.Warnings, warn)
		}
	}

	return result, nil
}

func (c *Client) fetchPrimary(ctx context.Context, publicIdentifier string, budget *AttemptBudget, creds *Credentials) (map[string]interface{}, error) {
	var lastErr error
	for _, deco := range ProfileDecorations {
		params := map[string]string{
			"decorationId":   deco,
			"memberIdentity": publicIdentifier,
			"q":              "memberIdentity",
		}
		doc, err := c.getJSON(ctx, "/identity/dash/profiles", params, true, budget, creds)
		if err != nil {
			if errors.Is(err, ErrBudgetExhausted) {
				return nil, err
			}
			lastErr = err
			continue
		}
		if !matchesPrimaryIdentity(doc, publicIdentifier) {
			lastErr = fmt.Errorf("%w: unexpected identity", ErrUpstreamResponse)
			continue
		}
		return doc, nil
	}

	path := fmt.Sprintf("/identity/profiles/%s/profileView", publicIdentifier)
	doc, err := c.getJSON(ctx, path, nil, true, budget, creds)
	if err != nil {
		if lastErr != nil && !errors.Is(err, ErrBudgetExhausted) {
			return nil, lastErr
		}
		return nil, err
	}
	if !matchesPrimaryIdentity(doc, publicIdentifier) {
		return nil, fmt.Errorf("%w: unexpected identity", ErrUpstreamResponse)
	}
	return doc, nil
}

func (c *Client) fetchSections(ctx context.Context, publicIdentifier string, budget *AttemptBudget, creds *Credentials) ([]map[string]interface{}, []string) {
	var wg sync.WaitGroup
	sem := make(chan struct{}, 3)

	type res struct {
		idx  int
		doc  map[string]interface{}
		warn string
	}
	results := make([]res, len(SectionPaths))

	for i, section := range SectionPaths {
		wg.Add(1)
		go func(i int, section string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			params := map[string]string{"count": "100", "start": "0"}
			path := fmt.Sprintf("/identity/profiles/%s/%s", publicIdentifier, section)
			doc, warn := c.optionalGet(ctx, path, section, budget, creds, params)
			results[i] = res{idx: i, doc: doc, warn: warn}
		}(i, section)
	}
	wg.Wait()

	var docs []map[string]interface{}
	var warns []string

	for i, section := range SectionPaths {
		item := results[i]
		if item.doc != nil {
			item.doc["__source"] = "section"
			item.doc["__section"] = section
			item.doc["__profile_identifier"] = publicIdentifier
			docs = append(docs, item.doc)
		}
		if item.warn != "" {
			warns = append(warns, item.warn)
		}
	}

	return docs, warns
}

func (c *Client) optionalGet(ctx context.Context, path string, label string, budget *AttemptBudget, creds *Credentials, params map[string]string) (map[string]interface{}, string) {
	doc, err := c.getJSON(ctx, path, params, false, budget, creds)
	if err != nil {
		if errors.Is(err, ErrProfileNotFound) || errors.Is(err, ErrUpstreamResponse) {
			return nil, fmt.Sprintf("LinkedIn did not expose %s through this session", label)
		}
		return nil, err.Error()
	}
	return doc, ""
}

func (c *Client) getJSON(ctx context.Context, path string, params map[string]string, profileLookup bool, budget *AttemptBudget, creds *Credentials) (map[string]interface{}, error) {
	attempts := c.Settings.LinkedInMaxRetries + 1
	var lastErr error

	for attempt := 0; attempt < attempts; attempt++ {
		if err := budget.Charge(); err != nil {
			return nil, err
		}

		req, err := http.NewRequestWithContext(ctx, "GET", BaseURL+path, nil)
		if err != nil {
			return nil, err
		}

		q := req.URL.Query()
		for k, v := range params {
			q.Add(k, v)
		}
		req.URL.RawQuery = q.Encode()

		req.Header.Set("accept", "application/vnd.linkedin.normalized+json+2.1")
		req.Header.Set("accept-language", "en-US,en;q=0.9")
		req.Header.Set("csrf-token", creds.CSRFToken)
		
		// Add referer header matching the profile identifier if this is a profile lookup
		if publicIdentifier, ok := ctx.Value("publicIdentifier").(string); ok && publicIdentifier != "" {
			req.Header.Set("referer", fmt.Sprintf("https://www.linkedin.com/in/%s/", publicIdentifier))
		}
		
		req.Header.Set("sec-fetch-dest", "empty")
		req.Header.Set("sec-fetch-mode", "cors")
		req.Header.Set("sec-fetch-site", "same-origin")
		req.Header.Set("user-agent", c.Settings.LinkedInUserAgent)
		req.Header.Set("x-li-lang", "en_US")
		req.Header.Set("x-restli-protocol-version", "2.0.0")

		for _, cookie := range creds.Cookies {
			req.AddCookie(cookie)
		}

		resp, err := c.HTTPClient.Do(req)
		if err != nil {
			lastErr = err
			if attempt+1 < attempts {
				time.Sleep(time.Duration(min(0.4*float64(int(1)<<attempt), 2.0)*1000) * time.Millisecond)
				continue
			}
			return nil, fmt.Errorf("%w: %v", ErrUpstreamResponse, err)
		}

		switch resp.StatusCode {
		case 301, 302, 303, 307, 308, 401, 403:
			c.CredentialProvider.Clear()
			resp.Body.Close()
			return nil, ErrAuthentication
		case 404:
			if profileLookup {
				resp.Body.Close()
				return nil, ErrProfileNotFound
			}
			fallthrough
		case 410:
			resp.Body.Close()
			return nil, ErrProfileNotFound
		case 429:
			resp.Body.Close()
			return nil, ErrUpstreamRateLimited
		}

		if resp.StatusCode >= 500 && attempt+1 < attempts {
			resp.Body.Close()
			time.Sleep(time.Duration(min(0.4*float64(int(1)<<attempt), 2.0)*1000) * time.Millisecond)
			continue
		}
		if resp.StatusCode >= 400 {
			resp.Body.Close()
			return nil, fmt.Errorf("%w: HTTP %d", ErrUpstreamResponse, resp.StatusCode)
		}

		if !strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "json") {
			c.CredentialProvider.Clear()
			resp.Body.Close()
			return nil, ErrAuthentication
		}

		var buf bytes.Buffer
		_, err = buf.ReadFrom(http.MaxBytesReader(nil, resp.Body, int64(c.Settings.LinkedInMaxResponseBytes)))
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("%w: oversized or stream error", ErrUpstreamResponse)
		}

		var payload map[string]interface{}
		if err := json.Unmarshal(buf.Bytes(), &payload); err != nil {
			return nil, fmt.Errorf("%w: invalid JSON", ErrUpstreamResponse)
		}

		return payload, nil
	}

	if lastErr != nil {
		return nil, fmt.Errorf("%w: request failed %v", ErrUpstreamResponse, lastErr)
	}
	return nil, ErrUpstreamResponse
}

func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func primaryProfileCandidates(document map[string]interface{}) []map[string]interface{} {
	included, ok := document["included"].([]interface{})
	if !ok {
		return nil
	}
	var profiles []map[string]interface{}
	for _, item := range included {
		if entity, ok := item.(map[string]interface{}); ok {
			if entity["$type"] == "com.linkedin.voyager.dash.identity.profile.Profile" {
				profiles = append(profiles, entity)
			}
		}
	}
	data, ok := document["data"].(map[string]interface{})
	if !ok {
		return profiles
	}
	roots, ok := data["*elements"].([]interface{})
	if !ok {
		return profiles
	}
	rootUrns := make(map[string]bool)
	for _, r := range roots {
		if s, ok := r.(string); ok {
			rootUrns[s] = true
		}
	}
	var filtered []map[string]interface{}
	for _, p := range profiles {
		if urn, ok := p["entityUrn"].(string); ok && rootUrns[urn] {
			filtered = append(filtered, p)
		}
	}
	return filtered
}

// matchesPrimaryIdentity validates that the returned document belongs to the requested profile.
func matchesPrimaryIdentity(doc map[string]interface{}, publicIdentifier string) bool {
	expected := strings.ToLower(publicIdentifier)
	candidates := primaryProfileCandidates(doc)
	if legacy, ok := doc["profile"]; ok && legacy != nil {
		if identity.HasMalformedPublicIdentifier(legacy) {
			return false
		}
		legacyIdentifiers := identity.ProfilePublicIdentifiers(legacy)
		if len(legacyIdentifiers) > 0 {
			for _, item := range legacyIdentifiers {
				if strings.ToLower(item) == expected {
					return true
				}
			}
			return false
		}
	}
	for _, candidate := range candidates {
		if identity.HasMalformedPublicIdentifier(candidate) {
			return false
		}
		identifiers := identity.ProfilePublicIdentifiers(candidate)
		for _, item := range identifiers {
			if strings.ToLower(item) == expected {
				return true
			}
		}
	}
	return false
}
