package logger

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"

	"github.com/sirupsen/logrus"
)

func TestNewLoggerWithConfigJSONServiceNameAndLevel(t *testing.T) {
	var buf bytes.Buffer
	log := NewLoggerWithConfig(Config{
		ServiceName: "scheduler-test",
		LogLevel:    "debug",
		Output:      &buf,
	})

	log.Debug("debug message")

	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("decode log JSON: %v\nlog: %s", err, buf.String())
	}
	if got["level"] != "debug" {
		t.Fatalf("level = %v, want debug", got["level"])
	}
	if got["service_name"] != "scheduler-test" {
		t.Fatalf("service_name = %v, want scheduler-test", got["service_name"])
	}
	if _, ok := got["time"]; !ok {
		t.Fatalf("time field missing from log entry: %s", buf.String())
	}
	if got["msg"] != "debug message" {
		t.Fatalf("msg = %v, want debug message", got["msg"])
	}
}

func TestParseLevel(t *testing.T) {
	tests := []struct {
		raw     string
		want    logrus.Level
		wantErr bool
	}{
		{raw: "debug", want: logrus.DebugLevel},
		{raw: "info", want: logrus.InfoLevel},
		{raw: "error", want: logrus.ErrorLevel},
		{raw: "INFO", want: logrus.InfoLevel},
		{raw: "", want: logrus.InfoLevel},
		{raw: "trace", wantErr: true},
	}

	for _, tt := range tests {
		got, err := ParseLevel(tt.raw)
		if tt.wantErr {
			if err == nil {
				t.Fatalf("ParseLevel(%q) error = nil, want error", tt.raw)
			}
			continue
		}
		if err != nil {
			t.Fatalf("ParseLevel(%q) error = %v, want nil", tt.raw, err)
		}
		if got != tt.want {
			t.Fatalf("ParseLevel(%q) = %v, want %v", tt.raw, got, tt.want)
		}
	}
}

func TestNewLoggerUsesEnvLogLevel(t *testing.T) {
	t.Setenv("LOG_LEVEL", "error")
	var buf bytes.Buffer
	log := NewLogger()
	log.SetOutput(&buf)
	if log.GetLevel() != logrus.ErrorLevel {
		t.Fatalf("level = %v, want error", log.GetLevel())
	}
}

func TestDefaultServiceName(t *testing.T) {
	var buf bytes.Buffer
	log := NewLoggerWithConfig(Config{Output: &buf})
	log.Info("message")

	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("decode log JSON: %v\nlog: %s", err, buf.String())
	}
	if got["service_name"] != DefaultServiceName {
		t.Fatalf("service_name = %v, want %s", got["service_name"], DefaultServiceName)
	}
}

func TestLogWithLevel(t *testing.T) {
	var buf bytes.Buffer
	log := NewLoggerWithConfig(Config{
		LogLevel: "debug",
		Output:   &buf,
	})

	LogWithLevel(log, logrus.ErrorLevel, "error message", logrus.Fields{"field": "value"})

	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("decode log JSON: %v\nlog: %s", err, buf.String())
	}
	if got["level"] != "error" {
		t.Fatalf("level = %v, want error", got["level"])
	}
	if got["field"] != "value" {
		t.Fatalf("field = %v, want value", got["field"])
	}
}

func TestNewLoggerInvalidLevelWarnsAndDefaultsToInfo(t *testing.T) {
	var buf bytes.Buffer
	log := NewLoggerWithConfig(Config{
		LogLevel: "invalid",
		Output:   &buf,
	})

	if log.GetLevel() != logrus.InfoLevel {
		t.Fatalf("level = %v, want info", log.GetLevel())
	}
	if !bytes.Contains(buf.Bytes(), []byte("invalid log level")) {
		t.Fatalf("expected warning about invalid level, got: %s", buf.String())
	}
}

func BenchmarkNewLogger(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = NewLoggerWithConfig(Config{
			ServiceName: "scheduler",
			LogLevel:    "info",
			Output:      os.Stdout,
		})
	}
}
