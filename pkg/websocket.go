// websocket.go
package operationtracker

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

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

	channel := fmt.Sprintf("operation_updates:%s", userID)
	pubsub := h.Tracker.RedisClient.Subscribe(ctx, channel)
	defer pubsub.Close()

	msgChan := pubsub.Channel()

	// go func() {
	// 	<-r.Context().Done()

	// }()

	for {
		select {
		case <-ctx.Done():
			cancel()
			return
		case msg := <-msgChan:
			var op Operation
			if err := json.Unmarshal([]byte(msg.Payload), &op); err != nil {
				h.Logger.Error("Failed to unmarshal operation", zap.Error(err))
				continue
			}

			opResult := OperationResult{
				Key:       op.Key,
				Message:   op.Status,
				Timestamp: op.Timestamp,
				Read:      op.Read,
			}

			if err := conn.WriteJSON(opResult); err != nil {
				h.Logger.Error("Failed to write JSON to WebSocket", zap.Error(err))
				return
			}
		}
	}
}
