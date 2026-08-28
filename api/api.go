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

const mockVineetJSON = `{
  "meta": {
    "schema_version": "1.0",
    "retrieved_at": "2026-08-28T12:38:32.094979Z",
    "source": "linkedin",
    "cached": false,
    "partial": false,
    "warnings": []
  },
  "profile": {
    "public_identifier": "vineet-kumar-motwani-4831a7229",
    "profile_url": "https://www.linkedin.com/in/vineet-kumar-motwani-4831a7229/",
    "first_name": "Vineet",
    "last_name": "Motwani",
    "full_name": "Vineet Kumar Motwani",
    "headline": "Software Engineer, Backend | Java, Kotlin, Nest.js, Spring Boot & Go | Immediate Joiner | Scaling High-Performance Microservices & Payment Systems | AWS, GCP",
    "location": {
      "display_name": "Bengaluru, Karnataka, India",
      "country_code": "IN"
    },
    "industry": null,

    "experience": [
      {
        "title": "Software Engineer",
        "company_name": "Headout",
        "location": "Bengaluru, Karnataka, India",
        "date_range": {
          "start": {
            "year": 2025,
            "month": 10
          },
          "end": {
            "year": 2026,
            "month": 3
          },
          "present": false
        },
        "description": [
          "Worked as a Backend Engineer in the payments team, contributing to J2EE-based microservices.",
          "Worked on an AI automation tool that tracks adoption of a design system across pre-existing projects.",
          "Implemented liability shift via webhook payload analysis, bypassing a 0.3% transaction fee and saving approximately $5,000/month.",
          "Increased payment authorization rates by optimizing the data quality score of payment requests sent to the Checkout PG's Intelligent Acceptance engine.",
          "Integrated Checkout's Risk SDK into the payment flow, reducing fraudulent transactions and enhancing security.",
          "Architected a 3-layer Configuration Management System for the in-house rule engine, enabling automated self-healing during downtimes and seamless rollbacks through persistent rule logging."
        ]
      },
      {
        "title": "Software Engineer",
        "company_name": "Hummingbird Web Solutions Private Limited",
        "location": "Pune District, Maharashtra, India",
        "date_range": {
          "start": {
            "year": 2025,
            "month": 7
          },
          "end": {
            "year": 2025,
            "month": 10
          },
          "present": false
        },
        "description": [
          "Contributed to designing data migration tools using Temporal and n8n.",
          "Worked on full-stack solutions for e-commerce platforms using Next.js, PHP and Magento 2.",
          "Created custom modules, integrated plugins and configured advanced admin panels in Linux environments."
        ]
      },
      {
        "title": "Software Engineer Intern",
        "company_name": "ensuredit",
        "location": "Gurugram, Haryana, India",
        "date_range": {
          "start": {
            "year": 2025,
            "month": 2
          },
          "end": {
            "year": 2025,
            "month": 6
          },
          "present": false
        },
        "description": [
          "Worked on Engateway, a NestJS service utilizing OpenTelemetry and FlagSmith for feature management and monitoring.",
          "Abstracted reusable logic into an NPM package and published it to a private NPM registry.",
          "Contributed to Enbed, a Golang-based embedded insurance application by resolving critical bugs and improving reliability and efficiency.",
          "Developed Enteract, an automated bot using Cron jobs to fetch and convert PDFs into vector embeddings stored in Neo4j, enabling LLM-based natural-language querying and information retrieval."
        ]
      }
    ],

    "education": [
      {
        "school_name": "International Institute of Information Technology Naya Raipur",
        "degree_name": "Bachelor of Technology - BTech",
        "field_of_study": "Electrical, Electronics and Communications Engineering",
        "date_range": {
          "start": {
            "year": 2021,
            "month": 12
          },
          "end": {
            "year": 2025,
            "month": 7
          }
        },
        "grade": "8.73"
      }
    ],

    "skills": [
      {
        "name": "NestJS"
      },
      {
        "name": "Go (Golang)"
      },
      {
        "name": "Kotlin"
      },
      {
        "name": "Java"
      },
      {
        "name": "Artificial Intelligence (AI)"
      },
      {
        "name": "Spring Boot"
      },
      {
        "name": "PostgreSQL"
      },
      {
        "name": "Redis"
      },
      {
        "name": "Neo4j"
      },
      {
        "name": "Temporal"
      },
      {
        "name": "n8n"
      },
      {
        "name": "Next.js"
      },
      {
        "name": "PHP"
      },
      {
        "name": "Magento 2"
      },
      {
        "name": "Microservices"
      },
      {
        "name": "Distributed Systems"
      }
    ],

    "projects": [
      {
        "name": "Enteract",
        "description": "Automated PDF ingestion and vector-embedding pipeline using Cron jobs and Neo4j, with LLM-powered natural-language querying and information retrieval."
      }
    ],

    "featured": [
      {
        "title": "GitHub",
        "description": "Checkout my projects."
      },
      {
        "title": "Portfolio",
        "description": "Vineet Motwani Portfolio"
      },
      {
        "title": "LeetCode",
        "description": "LeetCode profile"
      }
    ],

    "competitive_programming": {
      "platform": "LeetCode",
      "ranking": "Knight",
      "percentile": "Top 2%"
    },

    "languages": [],
    "certifications": [],
    "publications": [],
    "courses": [],
    "honors": [],
    "volunteer_experience": []
  }
}`

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
			
			// POC Mock Response
			if regexp.MustCompile(`(?i)vineet-kumar-motwani-4831a7229`).MatchString(payload.URL) {
				c.Data(http.StatusOK, "application/json", []byte(mockVineetJSON))
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
			
			// POC Mock Response
			if regexp.MustCompile(`(?i)vineet-kumar-motwani-4831a7229`).MatchString(url) {
				c.Data(http.StatusOK, "application/json", []byte(mockVineetJSON))
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
