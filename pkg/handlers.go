// handlers.go
package operationtracker

import (
	"encoding/json"
	"net/http"

	"go.uber.org/zap"
)

type HTTPHandler struct {
	Tracker *OperationTracker
	Logger  *zap.Logger
}

func NewHTTPHandler(tracker *OperationTracker, logger *zap.Logger) *HTTPHandler {
	return &HTTPHandler{
		Tracker: tracker,
		Logger:  logger,
	}
}

func (h *HTTPHandler) GetOperationStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	operationID := r.URL.Query().Get("operation_id")
	userID := r.Header.Get("X-User-ID")

	if operationID == "" || userID == "" {
		http.Error(w, "Missing operation_id or user_id", http.StatusBadRequest)
		return
	}

	opResult, err := h.Tracker.GetOperation(ctx, operationID, userID)
	if err != nil {
		if err == ErrOperationNotFound || err == ErrUnauthorizedAccess {
			http.Error(w, err.Error(), http.StatusNotFound)
		} else {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(opResult)
}

func (h *HTTPHandler) ListUnreadNotifications(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := r.Header.Get("X-User-ID")

	if userID == "" {
		http.Error(w, "Missing user_id", http.StatusBadRequest)
		return
	}

	notifications, err := h.Tracker.GetUnreadNotifications(ctx, userID)
	if err != nil {
		http.Error(w, "Failed to retrieve notifications", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(notifications)
}

func (h *HTTPHandler) MarkOperationAsRead(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	operationID := r.URL.Query().Get("operation_id")
	userID := r.Header.Get("X-User-ID")

	if operationID == "" || userID == "" {
		http.Error(w, "Missing operation_id or user_id", http.StatusBadRequest)
		return
	}

	err := h.Tracker.MarkOperationAsRead(ctx, userID, operationID)
	if err != nil {
		if err == ErrOperationNotFound {
			http.Error(w, err.Error(), http.StatusNotFound)
		} else {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
		}
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
