package handlers

import (
	"net/http"
	"sync"
	"time"
)

// RequestMetrics tracks request statistics
type RequestMetrics struct {
	Mu                 sync.RWMutex
	TotalRequests      int64
	SuccessfulRequests int64
	FailedRequests     int64
	ActiveRequests     int64
}

// HealthHandler handles health check requests
type HealthHandler struct {
	Metrics *RequestMetrics
}

// Handle handles health check requests
func (h *HealthHandler) Handle(w http.ResponseWriter, r *http.Request) {
	h.Metrics.Mu.RLock()
	health := map[string]interface{}{
		"status":             "healthy",
		"timestamp":          time.Now().UTC().Format(time.RFC3339),
		"total_requests":     h.Metrics.TotalRequests,
		"successful_requests": h.Metrics.SuccessfulRequests,
		"failed_requests":    h.Metrics.FailedRequests,
		"active_requests":    h.Metrics.ActiveRequests,
	}
	h.Metrics.Mu.RUnlock()

	WriteJSONResponse(w, http.StatusOK, APIResponse{
		Success: true,
		Data:    health,
	})
}

