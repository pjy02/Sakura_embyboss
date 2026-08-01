package redisstore

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestPingIntegration(t *testing.T) {
	address := os.Getenv("SAKURA_V3_TEST_REDIS_ADDRESS")
	if address == "" {
		t.Skip("SAKURA_V3_TEST_REDIS_ADDRESS is not configured")
	}
	client := New(address, os.Getenv("SAKURA_V3_TEST_REDIS_PASSWORD"), 0)
	defer client.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := client.Ping(ctx); err != nil {
		t.Fatalf("ping Redis: %v", err)
	}
}
