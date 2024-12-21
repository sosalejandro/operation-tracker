// websocket.go
package operationtracker

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

type WebSocketHandler struct {
	Tracker  *OperationTracker
	Logger   *zap.Logger
	Upgrader websocket.Upgrader
}

func NewWebSocketHandler(tracker *OperationTracker, logger *zap.Logger) *WebSocketHandler {
	return &WebSocketHandler{
		Tracker: tracker,
		Logger:  logger,
		Upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		},
	}
}

func (h *WebSocketHandler) ServeWebSocket(w http.ResponseWriter, r *http.Request) {
	userID := r.URL.Query().Get("user_id")
	if userID == "" {
		http.Error(w, ErrMissingUserID.Error(), http.StatusBadRequest)
		return
	}

	conn, err := h.Upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.Logger.Error("Failed to upgrade to WebSocket", zap.Error(err))
		http.Error(w, ErrWebSocketUpgradeFailed.Error(), http.StatusBadRequest)
		return
	}
	defer conn.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	streamKey := fmt.Sprintf("notifications:%s", userID)
	groupName := fmt.Sprintf("group_%s_%s", userID, uuid.New().String()) // Unique consumer group
	consumerName := fmt.Sprintf("consumer_%s", userID)

	// Create consumer group if it doesn't exist
	err = h.Tracker.RedisClient.XGroupCreateMkStream(ctx, streamKey, groupName, "$").Err()
	if err != nil && err != redis.Nil {
		h.Logger.Error("Failed to create consumer group", zap.Error(err))
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	wsMsgChan := make(chan Message)
	streamMsgChan := make(chan redis.XMessage)

	go func() {
		defer close(wsMsgChan)
		for {
			select {
			case <-ctx.Done():
				h.Logger.Info("Closing WebSocket message channel")
				return
			default:
				_, message, err := conn.ReadMessage()
				h.Logger.Info("Received message from WebSocket", zap.String("message", string(message)))
				if err != nil {
					if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
						h.Logger.Error("Failed to read message from WebSocket", zap.Error(err))
					}
					return
				}

				var msg Message
				if err := json.Unmarshal(message, &msg); err != nil {
					h.Logger.Error("Failed to unmarshal message", zap.Error(err))
					continue
				}

				wsMsgChan <- msg
				h.Logger.Info("Sent message to WebSocket message channel", zap.String("type", msg.Type))
			}
		}
	}()

	go func() {
		defer close(streamMsgChan)
		for {
			select {
			case <-ctx.Done():
				h.Logger.Info("Closing stream message channel")
				return
			default:
				// Consume messages from Redis Stream
				streams, err := h.Tracker.RedisClient.XReadGroup(ctx, &redis.XReadGroupArgs{
					Group:    groupName,
					Consumer: consumerName,
					Streams:  []string{streamKey, ">"},
					Count:    1,
					Block:    0,
				}).Result()
				h.Logger.Info("Consumed messages from Redis Stream", zap.Any("streams", streams))
				if err != nil {
					h.Logger.Error("Failed to read from Redis Stream", zap.Error(err))
					continue
				}

				for _, stream := range streams {
					for _, message := range stream.Messages {
						streamMsgChan <- message
						h.Logger.Info("Sent message to stream message channel", zap.Any("message", message))
					}
				}
			}
		}
	}()

	go func() {
		<-r.Context().Done()
		cancel()
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-wsMsgChan:
			switch msg.Type {
			case ReadOperationEvent.String():
				if err := h.handleReadOperation(ctx, msg, userID); err != nil {
					h.Logger.Error("Failed to handle read operation", zap.Error(err))
				}
				h.Logger.Info("Handled read operation", zap.Any("operation_id", msg.Data), zap.String("user_id", userID))
			default:
				h.Logger.Warn("Unknown message type", zap.String("type", msg.Type))
			}
		case message := <-streamMsgChan:
			var op Operation
			if err := json.Unmarshal([]byte(message.Values["data"].(string)), &op); err != nil {
				h.Logger.Error("Failed to unmarshal operation", zap.Error(err))
				continue
			}

			opResult := OperationResult{
				Key:       op.Key,
				Message:   op.Status,
				Timestamp: op.Timestamp,
				Read:      op.Read,
			}

			response := Message{
				Type: OperationResultEvent.String(),
				Data: json.RawMessage{},
			}
			response.Data, _ = json.Marshal(opResult)

			if err := conn.WriteJSON(response); err != nil {
				h.Logger.Error("Failed to write JSON to WebSocket", zap.Error(err))
				return
			}

			// Acknowledge the message
			err = h.Tracker.RedisClient.XAck(ctx, streamKey, groupName, message.ID).Err()
			if err != nil {
				h.Logger.Error("Failed to acknowledge message in Redis Stream", zap.Error(err))
			}
		}
	}
}

func (h *WebSocketHandler) handleReadOperation(ctx context.Context, msg Message, userID string) error {
	var readOp ReadOperation
	if err := json.Unmarshal(msg.Data, &readOp); err != nil {
		h.Logger.Error("Failed to unmarshal read operation", zap.Error(err))
		return err
	}

	err := h.Tracker.MarkOperationAsRead(ctx, userID, readOp.OperationID)
	if err != nil {
		h.Logger.Error("Failed to mark operation as read", zap.Error(err))
		return err
	}

	return nil
}
