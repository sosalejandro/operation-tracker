package operationtracker

import (
	"context"
	"encoding/json"

	"github.com/go-redis/redis/v8"
	"go.uber.org/zap"
)

type Broadcaster struct {
	RedisClient *redis.Client
	Logger      *zap.Logger
	ChannelName string
}

func NewBroadcaster(redisClient *redis.Client, logger *zap.Logger) *Broadcaster {
	return &Broadcaster{
		RedisClient: redisClient,
		Logger:      logger,
	}
}

func (b *Broadcaster) Publish(ctx context.Context, channelName, msgType string, message Message) error {
	messageBytes, err := json.Marshal(message)
	if err != nil {
		b.Logger.Error("Failed to marshal message", zap.Error(err))
		return err
	}

	err = b.RedisClient.Publish(ctx, b.ChannelName, messageBytes).Err()
	if err != nil {
		b.Logger.Error("Failed to publish message", zap.Error(err))
		return err
	}

	b.Logger.Info("Message published", zap.String("channel", b.ChannelName), zap.String("type", msgType))
	return nil
}

func (b *Broadcaster) Subscribe(ctx context.Context, channelName string, ch chan<- Message) error {
	pubsub := b.RedisClient.Subscribe(ctx, channelName)
	defer pubsub.Close()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msg := <-pubsub.Channel():
			var message Message
			if err := json.Unmarshal([]byte(msg.Payload), &message); err != nil {
				b.Logger.Error("Failed to unmarshal message", zap.Error(err))
				continue
			}

			ch <- message
		}
	}
}
