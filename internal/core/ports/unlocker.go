package ports

import "context"

// Unlocker is an interface that provides a way to retrieve passwords automatically
type Unlocker interface {
	// GetPassword retrieves a password to unlock a service
	GetPassword(ctx context.Context) (string, error)
}
