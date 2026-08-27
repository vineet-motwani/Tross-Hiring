package api

import (
	"bytes"
	"io"
	"net/http"
	"regexp"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/vineet-motwani/Tross-Hiring/config"
	"github.com/vineet-motwani/Tross-Hiring/errors"
	"github.com/vineet-motwani/Tross-Hiring/models"
	"github.com/vineet-motwani/Tross-Hiring/ratelimit"
	"github.com/vineet-motwani/Tross-Hiring/service"
	"github.com/vineet-motwani/Tross-Hiring/utils"
)

var safeRequestID = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,100}$`)

func errorResponse(c *gin.Context, statusCode int, code string, message string, requestID string) {
	c.JSON(statusCode, models.ErrorResponse{
		Error: models.ErrorDetail{
			Code:      code,
			Message:   message,
			RequestID: &requestID,
		},
	})
	c.Abort()
}

func getRequestID(c *gin.Context) string {
	if reqID, exists := c.Get("request_id"); exists {
		if str, ok := reqID.(string); ok {
			return str
		}
	}
	return ""
}

func clientKey(c *gin.Context) string {
	if c.ClientIP() != "" {
		return c.ClientIP()
	}
	return "unknown"
}

func BodyLimitMiddleware(maxBytes int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method == "POST" && c.Request.URL.Path == "/v1/profiles" {
			if c.Request.ContentLength > maxBytes {
				reqID := getRequestID(c)
				errorResponse(c, http.StatusRequestEntityTooLarge, "request_too_large", "The request body is too large", reqID)
				return
			}
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
			
			bodyBytes, err := io.ReadAll(c.Request.Body)
			if err != nil {
				reqID := getRequestID(c)
				errorResponse(c, http.StatusRequestEntityTooLarge, "request_too_large", "The request body is too large", reqID)
				return
			}
			
			c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
		}
		c.Next()
	}
}

func RequestContextMiddleware(limiter *ratelimit.InMemoryRateLimiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		supplied := c.GetHeader("x-request-id")
		reqID := uuid.New().String()
		if safeRequestID.MatchString(supplied) {
			reqID = supplied
		}
		c.Set("request_id", reqID)

		if c.Request.URL.Path == "/v1/profiles" && !limiter.Allow(clientKey(c)) {
			c.Header("Retry-After", "60")
			errorResponse(c, http.StatusTooManyRequests, "client_rate_limited", "Too many requests; try again in one minute", reqID)
			return
		}

		c.Next()

		c.Header("X-Request-ID", reqID)
		c.Header("Cache-Control", "no-store")
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("Referrer-Policy", "no-referrer")
		c.Header("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
	}
}

func ErrorHandlerMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()

		if len(c.Errors) > 0 {
			err := c.Errors.Last().Err
			reqID := getRequestID(c)
			
			if apiErr, ok := err.(*errors.ProfileAPIError); ok {
				statusCode := http.StatusInternalServerError
				switch apiErr.Code {
				case "contact_info_disabled":
					statusCode = http.StatusForbidden
				}
				errorResponse(c, statusCode, apiErr.Code, apiErr.Error(), reqID)
				return
			}
			if apiErr, ok := err.(*utils.InvalidProfileURLError); ok {
				errorResponse(c, http.StatusUnprocessableEntity, "invalid_request", apiErr.Error(), reqID)
				return
			}
			
			errStr := err.Error()
			if errStr == "profile not found" {
				errorResponse(c, http.StatusNotFound, "profile_not_found", "Profile not found", reqID)
				return
			}
			if errStr == "upstream rate limited" {
				c.Header("Retry-After", "60")
				errorResponse(c, http.StatusTooManyRequests, "upstream_rate_limited", "Upstream rate limited", reqID)
				return
			}
			if errStr == "authentication error" || errStr == "request budget exhausted" {
				errorResponse(c, http.StatusServiceUnavailable, "service_unavailable", "Service unavailable", reqID)
				return
			}
			
			errorResponse(c, http.StatusInternalServerError, "internal_error", err.Error(), reqID)
		}
	}
}

func CreateApp(runtimeSettings *config.Settings, svc *service.ProfileService, limiter *ratelimit.InMemoryRateLimiter) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	app := gin.New()
	
	app.Use(gin.Recovery())
	app.Use(RequestContextMiddleware(limiter))
	app.Use(BodyLimitMiddleware(int64(runtimeSettings.MaxRequestBodyBytes)))
	app.Use(ErrorHandlerMiddleware())

	app.GET("/", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"name":          runtimeSettings.AppName,
			"version":       runtimeSettings.AppVersion,
			"health":        "/health",
			"documentation": "/docs",
			"endpoint":      "/v1/profiles",
		})
	})

	app.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":  "ok",
			"version": runtimeSettings.AppVersion,
		})
	})

	v1 := app.Group("/v1")
	{
		v1.POST("/profiles", func(c *gin.Context) {
			var payload models.ProfileRequest
			if err := c.ShouldBindJSON(&payload); err != nil {
				reqID := getRequestID(c)
				errorResponse(c, http.StatusUnprocessableEntity, "invalid_request", "Supply a valid LinkedIn member profile URL", reqID)
				return
			}
			
			resp, err := svc.GetProfile(c.Request.Context(), payload.URL, payload.IncludeContactInfo)
			if err != nil {
				c.Error(err)
				return
			}
			c.JSON(http.StatusOK, resp)
		})

		v1.GET("/profiles", func(c *gin.Context) {
			url := c.Query("url")
			if len(url) > utils.MaxProfileURLLength || url == "" {
				reqID := getRequestID(c)
				errorResponse(c, http.StatusUnprocessableEntity, "invalid_request", "Supply a valid LinkedIn member profile URL", reqID)
				return
			}
			
			includeContactInfo := c.Query("include_contact_info") == "true"
			
			resp, err := svc.GetProfile(c.Request.Context(), url, includeContactInfo)
			if err != nil {
				c.Error(err)
				return
			}
			c.JSON(http.StatusOK, resp)
		})
	}

	return app
}
