package operationtracker

import "errors"

var (
	ErrOperationNotFound      = errors.New("operation not found")
	ErrFailedToMarshal        = errors.New("failed to marshal operation")
	ErrFailedToUnmarshal      = errors.New("failed to unmarshal operation")
	ErrFailedToSaveToRedis    = errors.New("failed to save operation to Redis")
	ErrFailedToUpdateRedis    = errors.New("failed to update operation in Redis")
	ErrFailedToRetrieveKeys   = errors.New("failed to retrieve keys")
	ErrFailedToRetrieveOps    = errors.New("failed to retrieve notifications")
	ErrUnauthorizedAccess     = errors.New("unauthorized access attempt")
	ErrFailedToPublishUpdate  = errors.New("failed to publish operation update")
	ErrFailedToRemoveFromList = errors.New("failed to remove operation from notifications list")
	ErrMissingUserID          = errors.New("missing user_id")
	ErrMissingOperationID     = errors.New("missing operation_id")
	ErrWebSocketUpgradeFailed = errors.New("WebSocket upgrade failed")
	ErrFailedToReceiveMessage = errors.New("failed to receive message")
	ErrFailedToWriteJSON      = errors.New("failed to write JSON to WebSocket")
)
