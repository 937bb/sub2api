package middleware

import (
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type bpsImageCapacityCounters struct {
	admitted         uint64
	requestsRejected uint64
	bytesRejected    uint64
	lastPeriodic     time.Time
	lastRejection    time.Time
}

type bpsImageCapacityObservation struct {
	reason           string
	requests         int
	maxRequests      int
	reservedBytes    int64
	limitBytes       int64
	requestWeight    int64
	admitted         uint64
	requestsRejected uint64
	bytesRejected    uint64
}

// Called under the pool lock so the rejection reason and occupancy agree.
// Counters include every acquisition decision; logs are bounded per pool.
func (b *bpsImageAdmissionBudget) observeLocked(weight int64, reason string, now time.Time) *bpsImageCapacityObservation {
	if reason == "admitted" {
		if !b.observed.lastPeriodic.IsZero() && now.Sub(b.observed.lastPeriodic) < 30*time.Second {
			return nil
		}
		b.observed.lastPeriodic = now
	} else {
		if !b.observed.lastRejection.IsZero() && now.Sub(b.observed.lastRejection) < time.Second {
			return nil
		}
		b.observed.lastRejection = now
	}
	limitBytes, maxRequests := b.limits()
	return &bpsImageCapacityObservation{
		reason: reason, requests: b.requests, maxRequests: maxRequests,
		reservedBytes: b.bytes, limitBytes: limitBytes, requestWeight: weight,
		admitted: b.observed.admitted, requestsRejected: b.observed.requestsRejected,
		bytesRejected: b.observed.bytesRejected,
	}
}

func logBPSImageCapacity(c *gin.Context, pool string, observation *bpsImageCapacityObservation) {
	if observation == nil {
		return
	}
	fields := []zap.Field{
		zap.String("pool", pool), zap.String("reason", observation.reason),
		zap.Int("in_flight", observation.requests), zap.Int("max_requests", observation.maxRequests),
		zap.Int64("reserved_bytes", observation.reservedBytes), zap.Int64("limit_bytes", observation.limitBytes),
		zap.Int64("request_weight_bytes", observation.requestWeight),
		zap.Uint64("admitted_total", observation.admitted),
		zap.Uint64("requests_rejected_total", observation.requestsRejected),
		zap.Uint64("bytes_rejected_total", observation.bytesRejected),
	}
	if key, ok := GetAPIKeyFromContext(c); ok && key != nil {
		fields = append(fields, zap.Int64("api_key_id", key.ID))
		if key.Group != nil {
			fields = append(fields, zap.Int64("group_id", key.Group.ID))
		}
	}
	log := logger.FromContext(c.Request.Context())
	if observation.reason == "admitted" {
		log.Info("image_admission_capacity", fields...)
	} else {
		log.Warn("image_admission_capacity", fields...)
	}
}
