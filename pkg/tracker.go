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

	data, err := json.Marshal(op)
	if err != nil {
		t.Logger.Error("Failed to marshal operation", zap.Error(err))
		return "", ErrFailedToMarshal
	}

	key := fmt.Sprintf("operations:%s:%s", userID, operationID)
	err = t.RedisClient.Set(ctx, key, data, 0).Err()
	if err != nil {
		t.Logger.Error("Failed to save operation to Redis", zap.Error(err))
		return "", ErrFailedToSaveToRedis
	}

	// Store in a list for unread notifications
	listKey := fmt.Sprintf("notifications:%s", userID)
	err = t.RedisClient.LPush(ctx, listKey, data).Err()
	if err != nil {
		t.Logger.Error("Failed to push operation to notifications list", zap.Error(err))
		return "", ErrFailedToSaveToRedis
	}

	// Publish to Pub/Sub channel
	pubSubChannel := fmt.Sprintf("operation_updates:%s", userID)
	err = t.RedisClient.Publish(ctx, pubSubChannel, data).Err()
	if err != nil {
		t.Logger.Error("Failed to publish to Pub/Sub", zap.Error(err))
		return "", ErrFailedToPublishUpdate
	}

	return operationID, nil
}

func (t *OperationTracker) UpdateOperation(ctx context.Context, composedOperationID, status, errorMsg string) error {
	key := fmt.Sprintf("operations:%s", composedOperationID)
	val, err := t.RedisClient.Get(ctx, key).Result()
	if err != nil {
		t.Logger.Error("Operation not found", zap.Error(err))
		return ErrOperationNotFound
	}

	var op Operation
	if err := json.Unmarshal([]byte(val), &op); err != nil {
		t.Logger.Error("Failed to unmarshal operation", zap.Error(err))
		return ErrFailedToUnmarshal
	}

	op.Status = status
	op.Error = errorMsg
	op.Timestamp = time.Now().UTC()

	data, err := json.Marshal(op)
	if err != nil {
		t.Logger.Error("Failed to marshal updated operation", zap.Error(err))
		return ErrFailedToMarshal
	}

	err = t.RedisClient.Set(ctx, key, data, 0).Err()
	if err != nil {
		t.Logger.Error("Failed to update operation in Redis", zap.Error(err))
		return ErrFailedToUpdateRedis
	}

	// Push to notifications list
	listKey := fmt.Sprintf("notifications:%s", op.UserID)
	err = t.RedisClient.LPush(ctx, listKey, data).Err()
	if err != nil {
		t.Logger.Error("Failed to push updated operation to notifications list", zap.Error(err))
		return ErrFailedToSaveToRedis
	}

	// Publish the update
	pubSubChannel := fmt.Sprintf("operation_updates:%s", op.UserID)
	err = t.RedisClient.Publish(ctx, pubSubChannel, data).Err()
	if err != nil {
		t.Logger.Error("Failed to publish operation update", zap.Error(err))
		return ErrFailedToPublishUpdate
	}

	return nil
}

func (t *OperationTracker) GetOperation(ctx context.Context, userID, operationID string) (*OperationResult, error) {
	key := fmt.Sprintf("operations:%s:%s", userID, operationID)
	val, err := t.RedisClient.Get(ctx, key).Result()
	if err != nil {
		t.Logger.Error("Operation not found", zap.Error(err))
		return nil, ErrOperationNotFound
	}

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
	keys, err := t.RedisClient.Keys(ctx, pattern).Result()
	if err != nil {
		t.Logger.Error("Failed to retrieve keys", zap.Error(err))
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
	val, err := t.RedisClient.Get(ctx, key).Result()
	if err != nil {
		t.Logger.Error("Operation not found", zap.Error(err))
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

	// Optionally, remove from notifications list
	listKey := fmt.Sprintf("notifications:%s", userID)
	err = t.RedisClient.LRem(ctx, listKey, 0, updatedData).Err()
	if err != nil {
		t.Logger.Error("Failed to remove operation from notifications list", zap.Error(err))
		return ErrFailedToRemoveFromList
	}

	return nil
}

func (t *OperationTracker) GetUnreadNotifications(ctx context.Context, userID string) ([]OperationResult, error) {
	listKey := fmt.Sprintf("notifications:%s", userID)
	ops, err := t.RedisClient.LRange(ctx, listKey, 0, -1).Result()
	if err != nil {
		t.Logger.Error("Failed to retrieve notifications", zap.Error(err))
		return nil, ErrFailedToRetrieveOps
	}

	var unreadOps []OperationResult
	for _, opData := range ops {
		var op Operation
		if err := json.Unmarshal([]byte(opData), &op); err != nil {
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
