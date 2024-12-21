// tracker.go
package operationtracker

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type OperationTracker struct {
	RedisClient *redis.Client
	Logger      *zap.Logger
}

func NewOperationTracker(redisClient *redis.Client, logger *zap.Logger) *OperationTracker {
	return &OperationTracker{
		RedisClient: redisClient,
		Logger:      logger,
	}
}

func (t *OperationTracker) CreateOperation(ctx context.Context, userID string) (string, error) {
	operationID := uuid.New().String()
	op := Operation{
		Key:       operationID,
		Status:    "Pending",
		Timestamp: time.Now().UTC(),
		UserID:    userID,
	}

	t.Logger.Info("Creating operation",
		zap.String("operation_id", operationID),
		zap.String("user_id", userID),
	)

	data, err := json.Marshal(op)
	if err != nil {
		t.Logger.Error("Failed to marshal operation", zap.Error(err))
		return "", ErrFailedToMarshal
	}

	key := fmt.Sprintf("operations:%s:%s", userID, operationID)
	err = t.RedisClient.Set(ctx, key, data, 0).Err()
	if err != nil {
		t.Logger.Error(
			"Failed to save operation to Redis",
			zap.Error(err),
			zap.String("key", key),
		)
		return "", ErrFailedToSaveToRedis
	}

	// Publish to Redis Stream
	streamKey := fmt.Sprintf("notifications:%s", userID)
	err = t.RedisClient.XAdd(ctx, &redis.XAddArgs{
		Stream: streamKey,
		Values: map[string]interface{}{"data": data},
	}).Err()
	if err != nil {
		t.Logger.Error("Failed to publish to Redis Stream", zap.Error(err))
		return "", ErrFailedToPublishUpdate
	}

	t.Logger.Info("Operation created",
		zap.String("operation_id", operationID),
		zap.String("user_id", userID),
	)

	return operationID, nil
}

func (t *OperationTracker) UpdateOperation(ctx context.Context, composedOperationID, status, errorMsg string) error {
	t.Logger.Info("Updating operation",
		zap.String("operation_id", composedOperationID),
	)

	key := fmt.Sprintf("operations:%s", composedOperationID)
	val, err := t.RedisClient.Get(ctx, key).Result()
	if err != nil {
		t.Logger.Error(
			"Operation not found",
			zap.Error(err),
			zap.String("operation_id", composedOperationID),
		)
		return ErrOperationNotFound
	}

	var op Operation
	if err := json.Unmarshal([]byte(val), &op); err != nil {
		t.Logger.Error("Failed to unmarshal operation", zap.Error(err))
		return ErrFailedToUnmarshal
	}

	if op.Status != "Completed" && op.Status != "Failed" {
		return ErrInvalidOperationStatus
	}

	op.Status = status
	op.Error = errorMsg
	op.Timestamp = time.Now().UTC()
	op.Read = false

	data, err := json.Marshal(op)
	if err != nil {
		t.Logger.Error("Failed to marshal updated operation", zap.Error(err))
		return ErrFailedToMarshal
	}

	err = t.RedisClient.Set(ctx, key, data, 0).Err()
	if err != nil {
		t.Logger.Error(
			"Failed to update operation in Redis",
			zap.Error(err),
			zap.String("key", key),
		)
		return ErrFailedToUpdateRedis
	}

	// Publish to Redis Stream
	streamKey := fmt.Sprintf("notifications:%s", op.UserID)
	err = t.RedisClient.XAdd(ctx, &redis.XAddArgs{
		Stream: streamKey,
		Values: map[string]interface{}{"data": data},
	}).Err()
	if err != nil {
		t.Logger.Error(
			"Failed to publish to Redis Stream",
			zap.Error(err),
			zap.String("stream_key", streamKey),
		)
		return ErrFailedToPublishUpdate
	}

	t.Logger.Info("Operation updated",
		zap.String("operation_id", composedOperationID),
		zap.String("status", status),
	)

	return nil
}

func (t *OperationTracker) GetOperation(ctx context.Context, userID, operationID string) (*OperationResult, error) {
	key := fmt.Sprintf("operations:%s:%s", userID, operationID)
	val, err := t.RedisClient.Get(ctx, key).Result()
	if err != nil {
		t.Logger.Error("Operation not found", zap.Error(err))
		return nil, ErrOperationNotFound
	}

	t.Logger.Info("Getting operation",
		zap.String("operation_id", operationID),
		zap.String("user_id", userID),
	)

	var op Operation
	if err := json.Unmarshal([]byte(val), &op); err != nil {
		t.Logger.Error("Failed to unmarshal operation", zap.Error(err))
		return nil, ErrFailedToUnmarshal
	}

	if op.UserID != userID {
		t.Logger.Warn("Unauthorized access attempt", zap.String("userID", userID))
		return nil, ErrUnauthorizedAccess
	}

	opResult := OperationResult{
		Key:       op.Key,
		Message:   op.Status,
		Timestamp: op.Timestamp,
		Read:      op.Read,
	}

	return &opResult, nil
}

func (t *OperationTracker) ListOperations(ctx context.Context, userID string) ([]Operation, error) {
	pattern := fmt.Sprintf("operations:%s:*", userID)
	t.Logger.Info("Listing operations", zap.String("pattern", pattern))
	keys, err := t.RedisClient.Keys(ctx, pattern).Result()
	if err != nil {
		t.Logger.Error("Failed to retrieve keys",
			zap.Error(err),
			zap.String("pattern", pattern),
		)
		return nil, ErrFailedToRetrieveKeys
	}

	var operations []Operation
	for _, key := range keys {
		val, err := t.RedisClient.Get(ctx, key).Result()
		if err != nil {
			continue
		}

		var op Operation
		if err := json.Unmarshal([]byte(val), &op); err != nil {
			continue
		}

		operations = append(operations, op)
	}

	return operations, nil
}

func (t *OperationTracker) MarkOperationAsRead(ctx context.Context, userID, operationID string) error {
	key := fmt.Sprintf("operations:%s:%s", userID, operationID)
	t.Logger.Info(
		"Marking operation as read",
		zap.String("operation_id", operationID),
	)
	val, err := t.RedisClient.Get(ctx, key).Result()
	if err != nil {
		t.Logger.Error(
			"Operation not found",
			zap.Error(err),
			zap.String("operation_id", operationID),
		)
		return ErrOperationNotFound
	}

	var op Operation
	if err := json.Unmarshal([]byte(val), &op); err != nil {
		t.Logger.Error("Failed to unmarshal operation", zap.Error(err))
		return ErrFailedToUnmarshal
	}

	op.Read = true

	updatedData, err := json.Marshal(op)
	if err != nil {
		t.Logger.Error("Failed to marshal updated operation", zap.Error(err))
		return ErrFailedToMarshal
	}

	err = t.RedisClient.Set(ctx, key, updatedData, 0).Err()
	if err != nil {
		t.Logger.Error("Failed to update operation in Redis", zap.Error(err))
		return ErrFailedToUpdateRedis
	}

	// Acknowledge and remove from Redis Stream
	streamKey := fmt.Sprintf("notifications:%s", userID)
	err = t.RedisClient.XAck(ctx, streamKey, "consumer-group", operationID).Err()
	if err != nil {
		t.Logger.Error("Failed to acknowledge message in Redis Stream", zap.Error(err))
		return ErrFailedToRemoveFromList
	}

	return nil
}

func (t *OperationTracker) GetUnreadNotifications(ctx context.Context, userID string) ([]OperationResult, error) {
	streamKey := fmt.Sprintf("notifications:%s", userID)
	entries, err := t.RedisClient.XRange(ctx, streamKey, "-", "+").Result()
	if err != nil {
		t.Logger.Error("Failed to retrieve notifications from Redis Stream", zap.Error(err))
		return nil, ErrFailedToRetrieveOps
	}

	var unreadOps []OperationResult
	for _, entry := range entries {
		var op Operation
		if err := json.Unmarshal([]byte(entry.Values["data"].(string)), &op); err != nil {
			continue
		}

		opResult := OperationResult{
			Key:       op.Key,
			Message:   op.Status,
			Timestamp: op.Timestamp,
			Read:      false,
		}

		unreadOps = append(unreadOps, opResult)
	}

	return unreadOps, nil
}
