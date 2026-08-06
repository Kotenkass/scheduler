package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/robfig/cron/v3"
)

const (
	redisChannel = "send_message"
)

type Payload struct {
	Message   string    `json:"message"`
	SentAt    time.Time `json:"sent_at"`
	Recipient string    `json:"recipient,omitempty"`
}

func main() {
	redisAddr := getenv("REDIS_ADDR", "localhost:6379")
	redisPassword := os.Getenv("REDIS_PASSWORD")
	redisDB := mustAtoi(getenv("REDIS_DB", "0"))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	rdb := redis.NewClient(&redis.Options{
		Addr:     redisAddr,
		Password: redisPassword,
		DB:       redisDB,
	})
	defer rdb.Close()

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	if err := rdb.Ping(pingCtx).Err(); err != nil {
		cancel()
		log.Fatalf("connect to redis at %s: %v", redisAddr, err)
	}
	cancel()

	c := cron.New(cron.WithSeconds())

	_, err := c.AddFunc("0 0 10 * * *", func() {
		if err := publishMessage(ctx, rdb); err != nil {
			log.Printf("publish %s: %v", redisChannel, err)
			return
		}
		log.Printf("published %s at %s", redisChannel, time.Now().Format(time.RFC3339))
	})
	if err != nil {
		log.Fatalf("schedule cron job: %v", err)
	}

	c.Start()
	log.Printf("scheduler started; publishing to redis channel %q every day at 10:00:00", redisChannel)

	<-ctx.Done()

	_ = c.Stop()
	_ = rdb.Close()
	log.Printf("scheduler stopped: %v", ctx.Err())
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

func getenv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func mustAtoi(s string) int {
	var n int
	_, err := fmt.Sscan(s, &n)
	if err != nil {
		log.Fatalf("invalid integer %q: %v", s, err)
	}
	return n
}
