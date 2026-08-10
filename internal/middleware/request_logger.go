package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/sirupsen/logrus"
)

const (
	// RequestIDContextKey is the Echo context key used by RequestID.
	RequestIDContextKey = "request_id"
	// RequestIDHeader is the response/request header used for request IDs.
	RequestIDHeader = echo.HeaderXRequestID
)

type requestIDContextKey struct{}

// Config configures HTTP request logging middleware.
type Config struct {
	Logger          *logrus.Logger
	RequestIDHeader string
	ContextKey      string
}

// RequestLogger logs each HTTP request and injects a request ID into Echo context
// and the X-Request-ID response header.
func RequestLogger(config Config) echo.MiddlewareFunc {
	logger := config.Logger
	if logger == nil {
		logger = logrus.New()
	}

	requestIDHeader := config.RequestIDHeader
	if requestIDHeader == "" {
		requestIDHeader = RequestIDHeader
	}

	contextKey := config.ContextKey
	if contextKey == "" {
		contextKey = RequestIDContextKey
	}

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			start := time.Now()

			requestID := c.Request().Header.Get(requestIDHeader)
			if requestID == "" {
				requestID = newRequestID()
			}

			c.Set(contextKey, requestID)
			c.Response().Header().Set(requestIDHeader, requestID)

			ctx := context.WithValue(c.Request().Context(), requestIDContextKey{}, requestID)
			c.SetRequest(c.Request().WithContext(ctx))

			err := next(c)

			fields := logrus.Fields{
				"event":        "http_request",
				"request_id":   requestID,
				"method":       c.Request().Method,
				"path":         c.Request().URL.Path,
				"execution_ms": time.Since(start).Milliseconds(),
				"status":       c.Response().Status,
				"remote_addr":  c.Request().RemoteAddr,
				"user_agent":   c.Request().UserAgent(),
			}
			if rawQuery := c.Request().URL.RawQuery; rawQuery != "" {
				fields["query"] = rawQuery
			}

			if err != nil {
				logger.WithFields(fields).WithError(err).Error("request failed")
				return err
			}

			logger.WithFields(fields).Info("request completed")
			return nil
		}
	}
}

// RequestID returns the request ID stored in Echo context or request context.
func RequestID(c echo.Context) string {
	if c == nil {
		return ""
	}
	if value := c.Get(RequestIDContextKey); value != nil {
		if requestID, ok := value.(string); ok {
			return requestID
		}
	}
	if value := c.Request().Context().Value(requestIDContextKey{}); value != nil {
		if requestID, ok := value.(string); ok {
			return requestID
		}
	}
	return ""
}

func newRequestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err == nil {
		return hex.EncodeToString(b[:])
	}
	return fmt.Sprintf("request-%d", time.Now().UnixNano())
}
