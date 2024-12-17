// operation.go
package operationtracker

import (
	"time"
)

type Operation struct {
	Key       string    `json:"key"`
	Status    string    `json:"status"` // "Pending", "Completed", "Failed"
	Timestamp time.Time `json:"timestamp"`
	UserID    string    `json:"user_id"`
	Error     string    `json:"error,omitempty"`
	Read      bool      `json:"read"`
}

type OperationResult struct {
	Key       string    `json:"key"`
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
	Read      bool      `json:"read"`
}
