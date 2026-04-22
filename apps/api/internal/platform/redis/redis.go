package redis

import (
	"context"
	"fmt"
	"time"

	"api/internal/platform/config"

	"github.com/redis/go-redis/v9"
)

type Client struct {
	rdb *redis.Client
}

func NewRedis(cfg config.RedisConfig) *Client {
	rdb := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%s", cfg.Host, cfg.Port),
		Password: cfg.Auth,
		DB:       0,
	})

	return &Client{rdb: rdb}
}

func (c *Client) Ping(ctx context.Context) error {
	return c.rdb.Ping(ctx).Err()
}

func (c *Client) Set(ctx context.Context, key string, value interface{}) error {
	return c.rdb.Set(ctx, key, value, 0).Err()
}

func (c *Client) SetWithExpiry(ctx context.Context, key string, value interface{}, expiry time.Duration) error {
	return c.rdb.Set(ctx, key, value, expiry).Err()
}

func (c *Client) Get(ctx context.Context, key string) (string, error) {
	return c.rdb.Get(ctx, key).Result()
}

func (c *Client) Del(ctx context.Context, key string) error {
	return c.rdb.Del(ctx, key).Err()
}

func (c *Client) GetClient() *redis.Client {
	return c.rdb
}
