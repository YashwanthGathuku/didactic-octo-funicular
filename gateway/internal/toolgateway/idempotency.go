package toolgateway

import (
	"context"
	"fmt"
	"sync"
)

// IdempotencyEntry represents a cached or in-flight execution.
type IdempotencyEntry struct {
	RequestHash string
	Response    *ToolResponse
	Done        chan struct{}
	Err         error
}

// IdempotencyCoordinator manages in-memory singleflight execution and replay detection.
type IdempotencyCoordinator struct {
	mu      sync.Mutex
	entries map[string]*IdempotencyEntry // compositeKey -> entry
}

// NewIdempotencyCoordinator creates a new IdempotencyCoordinator.
func NewIdempotencyCoordinator() *IdempotencyCoordinator {
	return &IdempotencyCoordinator{
		entries: make(map[string]*IdempotencyEntry),
	}
}

func compositeKey(tenantID, callerID, toolID, toolVersion, idempotencyKey string) string {
	return fmt.Sprintf("%s:%s:%s:%s:%s", tenantID, callerID, toolID, toolVersion, idempotencyKey)
}

// CheckOrLock checks if a request with this idempotency key is already completed or in-flight.
// If completed with the same request hash, it returns the cached response.
// If completed with a different request hash, it returns ErrIdempotencyConflict.
// If in-flight, it waits for the result or context cancellation.
// If new, it creates an in-flight entry and returns (nil, unlockFunc, nil).
func (c *IdempotencyCoordinator) CheckOrLock(
	ctx context.Context,
	tenantID, callerID, toolID, toolVersion, idempotencyKey, requestHash string,
) (*ToolResponse, func(resp *ToolResponse, err error), error) {
	key := compositeKey(tenantID, callerID, toolID, toolVersion, idempotencyKey)

	for {
		c.mu.Lock()
		entry, exists := c.entries[key]
		if !exists {
			// First caller: create in-flight entry
			entry = &IdempotencyEntry{
				RequestHash: requestHash,
				Done:        make(chan struct{}),
			}
			c.entries[key] = entry
			c.mu.Unlock()

			// Unlock callback executed when handler completes
			unlock := func(resp *ToolResponse, err error) {
				c.mu.Lock()
				defer c.mu.Unlock()
				entry.Response = resp
				entry.Err = err
				close(entry.Done)
			}
			return nil, unlock, nil
		}

		// Entry exists
		if entry.RequestHash != requestHash {
			c.mu.Unlock()
			return nil, nil, fmt.Errorf("%w: key %s already used with different payload", ErrIdempotencyConflict, idempotencyKey)
		}

		// Same request hash
		select {
		case <-entry.Done:
			// In-flight completed: return cached response
			resp := entry.Response
			err := entry.Err
			c.mu.Unlock()
			return resp, nil, err
		default:
			// Still in flight: wait outside lock
			doneChan := entry.Done
			c.mu.Unlock()

			select {
			case <-doneChan:
				// Re-loop to read response under lock
				continue
			case <-ctx.Done():
				return nil, nil, ctx.Err()
			}
		}
	}
}

// RecordDurableResult stores a completed durable result into the in-memory cache.
func (c *IdempotencyCoordinator) RecordDurableResult(
	tenantID, callerID, toolID, toolVersion, idempotencyKey, requestHash string,
	resp *ToolResponse,
) {
	key := compositeKey(tenantID, callerID, toolID, toolVersion, idempotencyKey)
	c.mu.Lock()
	defer c.mu.Unlock()

	entry := &IdempotencyEntry{
		RequestHash: requestHash,
		Response:    resp,
		Done:        make(chan struct{}),
	}
	close(entry.Done)
	c.entries[key] = entry
}
