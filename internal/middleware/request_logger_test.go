package middleware

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/sirupsen/logrus"
)

func TestRequestLoggerAddsRequestIDContextAndHeader(t *testing.T) {
	var buf bytes.Buffer
	log := logrus.New()
	log.SetOutput(&buf)
	log.SetFormatter(&logrus.JSONFormatter{})

	e := echo.New()
	e.Use(RequestLogger(Config{Logger: log}))
	e.GET("/jobs", func(c echo.Context) error {
		if RequestID(c) == "" {
			t.Fatal("request ID missing from context")
		}
		return c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/jobs", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if rec.Header().Get(RequestIDHeader) == "" {
		t.Fatalf("%s header missing", RequestIDHeader)
	}
	if requestID := req.Header.Get(RequestIDHeader); requestID != "" {
		t.Fatalf("request header should not be mutated, got %q", requestID)
	}

	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("decode log JSON: %v\nlog: %s", err, buf.String())
	}
	if got["method"] != http.MethodGet {
		t.Fatalf("method = %v, want GET", got["method"])
	}
	if got["path"] != "/jobs" {
		t.Fatalf("path = %v, want /jobs", got["path"])
	}
	if _, ok := got["request_id"]; !ok {
		t.Fatalf("request_id missing from log entry: %s", buf.String())
	}
	if _, ok := got["execution_ms"]; !ok {
		t.Fatalf("execution_ms missing from log entry: %s", buf.String())
	}
}

func TestRequestLoggerPreservesIncomingRequestID(t *testing.T) {
	e := echo.New()
	e.Use(RequestLogger(Config{Logger: logrus.New()}))
	e.GET("/jobs", func(c echo.Context) error {
		if got, want := RequestID(c), "incoming-request-id"; got != want {
			t.Fatalf("request ID = %q, want %q", got, want)
		}
		return c.NoContent(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/jobs", nil)
	req.Header.Set(RequestIDHeader, "incoming-request-id")
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if got := rec.Header().Get(RequestIDHeader); got != "incoming-request-id" {
		t.Fatalf("response request ID = %q, want incoming-request-id", got)
	}
}
