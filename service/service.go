package service

import (
	"context"
	"fmt"

	"github.com/vineet-motwani/Tross-Hiring/cache"
	"github.com/vineet-motwani/Tross-Hiring/errors"
	"github.com/vineet-motwani/Tross-Hiring/linkedin"
	"github.com/vineet-motwani/Tross-Hiring/models"
	"github.com/vineet-motwani/Tross-Hiring/parser"
	"github.com/vineet-motwani/Tross-Hiring/utils"
	"golang.org/x/sync/singleflight"
)

type ProfileService struct {
	client           *linkedin.Client
	cache            *cache.ProfileCache
	allowContactInfo bool
	sg               singleflight.Group
}

func NewProfileService(client *linkedin.Client, cache *cache.ProfileCache, allowContactInfo bool) *ProfileService {
	return &ProfileService{
		client:           client,
		cache:            cache,
		allowContactInfo: allowContactInfo,
	}
}

func (s *ProfileService) GetProfile(ctx context.Context, url string, includeContactInfo bool) (*models.ProfileResponse, error) {
	if includeContactInfo && !s.allowContactInfo {
		return nil, errors.NewProfileAPIError("contact_info_disabled", "Contact information is disabled on this deployment")
	}

	parsedUrl, err := utils.ParseLinkedInProfileURL(url)
	if err != nil {
		return nil, err
	}

	// Use lowercase public_identifier
	cacheKey := fmt.Sprintf("%s:%t", parsedUrl.PublicIdentifier, includeContactInfo)

	cached, err := s.cache.Get(ctx, cacheKey)
	if err == nil && cached != nil {
		return cached, nil
	}

	// Use singleflight to prevent multiple concurrent requests for the same profile
	v, err, _ := s.sg.Do(cacheKey, func() (interface{}, error) {
		// Double check cache
		cached, err := s.cache.Get(ctx, cacheKey)
		if err == nil && cached != nil {
			return cached, nil
		}

		return s.fetchAndCache(ctx, cacheKey, parsedUrl.PublicIdentifier, includeContactInfo)
	})

	if err != nil {
		return nil, err
	}

	resp := v.(*models.ProfileResponse)
	return resp, nil
}

func (s *ProfileService) fetchAndCache(ctx context.Context, cacheKey string, publicIdentifier string, includeContactInfo bool) (*models.ProfileResponse, error) {
	fetched, err := s.client.FetchProfile(ctx, publicIdentifier, includeContactInfo)
	if err != nil {
		return nil, err
	}

	rawDocs := make([]interface{}, len(fetched.Documents))
	for i, v := range fetched.Documents {
		rawDocs[i] = v
	}

	voyagerParser, err := parser.NewVoyagerParser(rawDocs)
	if err != nil {
		return nil, err
	}
	
	profile, err := voyagerParser.Parse(publicIdentifier, includeContactInfo)
	if err != nil {
		return nil, err
	}

	// Deduplicate warnings
	warningMap := make(map[string]bool)
	var warnings []string
	for _, w := range fetched.Warnings {
		if !warningMap[w] {
			warningMap[w] = true
			warnings = append(warnings, w)
		}
	}

	response := &models.ProfileResponse{
		Meta: models.ResponseMeta{
			Partial:  len(warnings) > 0,
			Warnings: warnings,
		},
		Profile: *profile,
	}

	err = s.cache.Set(ctx, cacheKey, response)
	if err != nil {
		// Just log the error or ignore; cache error shouldn't fail the request entirely
	}

	return response, nil
}
