package logger

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

const (
	// DefaultServiceName is used when no explicit service name is configured.
	DefaultServiceName = "[service-name]"
	// EnvLogLevel is the environment variable used to configure the logger level.
	EnvLogLevel = "LOG_LEVEL"
)

// Config controls logger initialization.
type Config struct {
	// ServiceName is written to every log entry as the service_name field.
	ServiceName string
	// LogLevel overrides LOG_LEVEL when non-empty. Supported values: debug, info, error.
	LogLevel string
	// Output overrides stdout. Tests can use io.Discard.
	Output io.Writer
	// Formatter overrides the default JSON formatter.
	Formatter logrus.Formatter
	// ReportCaller enables caller file/line/function fields.
	ReportCaller bool
}

// NewLogger returns a production-ready structured logger configured from LOG_LEVEL.
func NewLogger() *logrus.Logger {
	return NewLoggerWithConfig(Config{
		ServiceName: DefaultServiceName,
		LogLevel:    os.Getenv(EnvLogLevel),
		Output:      os.Stdout,
	})
}

// NewLoggerWithConfig returns a structured logger with explicit configuration.
func NewLoggerWithConfig(config Config) *logrus.Logger {
	log := logrus.New()

	if config.ServiceName == "" {
		config.ServiceName = DefaultServiceName
	}
	if config.Output == nil {
		config.Output = os.Stdout
	}

	log.SetOutput(config.Output)
	if config.Formatter != nil {
		log.SetFormatter(config.Formatter)
	} else {
		log.SetFormatter(&logrus.JSONFormatter{
			TimestampFormat: time.RFC3339Nano,
		})
	}
	log.SetReportCaller(config.ReportCaller)

	level, err := ParseLevel(config.LogLevel)
	if err != nil {
		log.Warnf("invalid log level %q from %s, defaulting to info: %v", config.LogLevel, EnvLogLevel, err)
		level = logrus.InfoLevel
	}
	log.SetLevel(level)

	log.AddHook(serviceNameHook{serviceName: config.ServiceName})

	return log
}

// ParseLevel parses the supported LOG_LEVEL values.
func ParseLevel(raw string) (logrus.Level, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "info":
		return logrus.InfoLevel, nil
	case "debug":
		return logrus.DebugLevel, nil
	case "error":
		return logrus.ErrorLevel, nil
	default:
		return logrus.InfoLevel, fmt.Errorf("unsupported log level %q; supported values are debug, info, error", raw)
	}
}

// LogWithLevel logs a message at an explicitly selected level.
func LogWithLevel(log *logrus.Logger, level logrus.Level, message string, fields logrus.Fields) {
	if log == nil {
		return
	}
	log.WithFields(fields).Log(level, message)
}

type serviceNameHook struct {
	serviceName string
}

func (h serviceNameHook) Levels() []logrus.Level {
	return logrus.AllLevels
}

func (h serviceNameHook) Fire(entry *logrus.Entry) error {
	if entry.Data == nil {
		entry.Data = logrus.Fields{}
	}
	entry.Data["service_name"] = h.serviceName
	return nil
}
