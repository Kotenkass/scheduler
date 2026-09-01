package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/Kotenkass/scheduler/internal/logger"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
	"github.com/robfig/cron/v3"
	"github.com/sirupsen/logrus"
)

const (
	dailyCronJobName   = "daily-message"
	weeklyCronJobName  = "weekly-reco"
	redisChannel       = "send_message"
	weeklyRedisChannel = "weekly_reco"
	httpAddr           = ":8080"
)

var (
	cronJobRunsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "scheduler",
			Name:      "cron_job_runs_total",
			Help:      "Total number of scheduler cron job executions.",
		},
		[]string{"job"},
	)
	cronJobErrorsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "scheduler",
			Name:      "cron_job_errors_total",
			Help:      "Total number of failed scheduler cron job executions.",
		},
		[]string{"job"},
	)
	cronJobDurationSeconds = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "scheduler",
			Name:      "cron_job_duration_seconds",
			Help:      "Scheduler cron job execution duration in seconds.",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"job"},
	)
	cronJobLastSuccessTimestampSeconds = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "scheduler",
			Name:      "cron_job_last_success_timestamp_seconds",
			Help:      "Unix timestamp of the last successful scheduler cron job execution.",
		},
		[]string{"job"},
	)
)

func init() {
	prometheus.MustRegister(
		cronJobRunsTotal,
		cronJobErrorsTotal,
		cronJobDurationSeconds,
		cronJobLastSuccessTimestampSeconds,
	)
}

type Payload struct {
	Message   string    `json:"message"`
	SentAt    time.Time `json:"sent_at"`
	Recipient string    `json:"recipient,omitempty"`
}

func main() {
	log := logger.NewLogger()

	redisOptions, err := redisOptionsFromEnv()
	if err != nil {
		log.WithError(err).Fatal("parse redis options")
	}
	log.WithFields(logrus.Fields{
		"redis_addr": redisOptions.Addr,
		"redis_db":   redisOptions.DB,
	}).Info("redis options loaded")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	rdb := redis.NewClient(redisOptions)
	defer rdb.Close()

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	if err := rdb.Ping(pingCtx).Err(); err != nil {
		cancel()
		log.WithError(err).Fatal("connect to redis")
	}
	cancel()

	c := cron.New(cron.WithSeconds())

	_, err = c.AddFunc("0 0 10 * * *", func() {
		start := time.Now()
		cronJobRunsTotal.WithLabelValues(dailyCronJobName).Inc()

		if err := publishMessage(ctx, rdb); err != nil {
			cronJobErrorsTotal.WithLabelValues(dailyCronJobName).Inc()
			log.WithError(err).WithField("redis_channel", redisChannel).Error("publish failed")
			return
		}

		cronJobDurationSeconds.WithLabelValues(dailyCronJobName).Observe(time.Since(start).Seconds())
		cronJobLastSuccessTimestampSeconds.WithLabelValues(dailyCronJobName).Set(float64(time.Now().Unix()))
		log.WithFields(logrus.Fields{
			"event":         "message_published",
			"redis_channel": redisChannel,
			"sent_at":       time.Now().UTC().Format(time.RFC3339Nano),
		}).Info("message published")
	})
	if err != nil {
		log.WithError(err).Fatal("schedule cron job")
	}

	_, err = c.AddFunc("0 0 9 * * MON", func() {
		start := time.Now()
		cronJobRunsTotal.WithLabelValues(weeklyCronJobName).Inc()

		if err := publishWeeklyRecommendation(ctx, rdb); err != nil {
			cronJobErrorsTotal.WithLabelValues(weeklyCronJobName).Inc()
			log.WithError(err).WithField("redis_channel", weeklyRedisChannel).Error("publish failed")
			return
		}

		cronJobDurationSeconds.WithLabelValues(weeklyCronJobName).Observe(time.Since(start).Seconds())
		cronJobLastSuccessTimestampSeconds.WithLabelValues(weeklyCronJobName).Set(float64(time.Now().Unix()))
		log.WithFields(logrus.Fields{
			"event":         "weekly_recommendation_published",
			"redis_channel": weeklyRedisChannel,
			"sent_at":       time.Now().UTC().Format(time.RFC3339Nano),
		}).Info("weekly recommendation published")
	})
	if err != nil {
		log.WithError(err).Fatal("schedule cron job")
	}

	c.Start()
	startHTTPServer(log)
	log.WithField("redis_channel", redisChannel).Info("scheduler started")

	<-ctx.Done()

	c.Stop()
	if err := rdb.Close(); err != nil {
		log.WithError(err).Warn("close redis")
	}
	log.WithField("signal", ctx.Err()).Info("scheduler stopped")
}

func publishMessage(ctx context.Context, rdb *redis.Client) error {
	payload := Payload{
		Message: "Hello from daily scheduler",
		SentAt:  time.Now().UTC(),
	}

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	return rdb.Publish(ctx, redisChannel, payloadJSON).Err()
}

func publishWeeklyRecommendation(ctx context.Context, rdb *redis.Client) error {
	payload := map[string]string{
		"fire_at": time.Now().UTC().Format(time.RFC3339),
	}

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	return rdb.Publish(ctx, weeklyRedisChannel, payloadJSON).Err()
}

func startHTTPServer(log *logrus.Logger) {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})

	server := &http.Server{
		Addr:              httpAddr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.WithField("addr", httpAddr).Info("metrics server starting")
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.WithError(err).Error("metrics server failed")
		}
	}()
}

func redisOptionsFromEnv() (*redis.Options, error) {
	raw := os.Getenv("REDIS_URL")
	if raw == "" {
		raw = getenv("REDIS_ADDR", "redis://localhost:6379/0")
	}

	if strings.Contains(raw, "://") {
		opt, err := redis.ParseURL(raw)
		if err != nil {
			return nil, fmt.Errorf("parse REDIS_URL: %w", err)
		}
		return opt, nil
	}

	return &redis.Options{Addr: raw}, nil
}

func getenv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func loggingExamples(log *logrus.Logger) {
	err := fmt.Errorf("redis connection refused")

	log.WithError(err).WithField("redis_addr", "redis://localhost:6379/0").Error("failed to connect to redis")

	log.WithFields(logrus.Fields{
		"event":         "job_scheduled",
		"job_id":        dailyCronJobName,
		"schedule":      "0 0 10 * * *",
		"redis_channel": redisChannel,
	}).Info("business event logged")

	log.WithFields(logrus.Fields{
		"recipient": "user@example.com",
		"attempt":   1,
		"success":   true,
	}).Debug("additional fields on a log entry")

	logger.LogWithLevel(log, logrus.DebugLevel, "explicit per-entry log level", logrus.Fields{
		"event": "example",
	})
}
