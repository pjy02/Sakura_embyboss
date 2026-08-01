package redisstore

import (
	"context"

	"github.com/redis/go-redis/v9"
)

type Client struct {
	client *redis.Client
}

func New(address, password string, database int) *Client {
	return &Client{client: redis.NewClient(&redis.Options{
		Addr:     address,
		Password: password,
		DB:       database,
	})}
}

func (c *Client) Ping(ctx context.Context) error {
	return c.client.Ping(ctx).Err()
}

func (c *Client) Close() error {
	return c.client.Close()
}

func (c *Client) Raw() *redis.Client {
	return c.client
}
