// operation.go
package operationtracker

import (
	"encoding/json"
	"time"
)

type EventType string

const (
	OperationResultEvent EventType = "operation_result"
	ReadOperationEvent   EventType = "read_operation"
)

func (e EventType) String() string {
	return string(e)
}

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

type Message struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

type ReadOperation struct {
	OperationID string `json:"operation_id"`
}
