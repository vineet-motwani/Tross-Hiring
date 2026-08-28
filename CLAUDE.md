# Coding Guidelines for this Repository

This repository is a high-performance Go application for querying LinkedIn profiles.
When contributing to this repository, adhere to the following best practices:

1. **Idiomatic Go**: Use idiomatic Go formatting (`gofmt`), naming conventions (e.g., camelCase for unexported variables, PascalCase for exported ones), and patterns.
2. **Error Handling**: Do not silently swallow errors. Wrap errors with context where appropriate using `fmt.Errorf("...: %w", err)`.
3. **Concurrency**: Ensure thread safety when manipulating shared resources (like the in-memory rate limiter or cache) using `sync.Mutex` or `sync.RWMutex`.
4. **Minimal Dependencies**: Rely on the Go standard library (`net/http`, `encoding/json`, `context`) where possible. We use `gin-gonic/gin` for the API and `aws-sdk-go-v2` for Secrets Manager, but otherwise keep the dependency tree light.
5. **No Panics**: Libraries and utilities should return `error` instead of using `panic`.
6. **Documentation**: Write GoDoc-style comments for exported functions and types.
7. **JSON Schemas**: Ensure all structs mapping to JSON have proper `json:"..."` tags, especially using `omitempty` for optional fields to keep responses clean.
