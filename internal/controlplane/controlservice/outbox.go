// Levier - durable Control envelope outbox consumer.
//
// Runtime gateways persist daemon envelopes before forwarding them. Control
// claims those notifications, applies the envelope, then marks it published.
//
// Copyright 2026 The Levier Authors.
package controlservice

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"google.golang.org/protobuf/proto"

	rolev1 "github.com/fluffyTighnari/levier/gen/middleware/role/v1"
	runtimev1 "github.com/fluffyTighnari/levier/gen/middleware/runtime/v1"
	"github.com/fluffyTighnari/levier/internal/state/cluster"
)

// EnvelopeOutboxChannel is the durable Runtime Gateway -> Control channel.
const EnvelopeOutboxChannel = "control.envelope"

// EnvelopePayload is one daemon envelope stored in the notification outbox.
type EnvelopePayload struct {
	GatewayID string `json:"gateway_id"`
	DaemonID  string `json:"daemon_id"`
	Envelope  []byte `json:"envelope"`
}

// EncodeEnvelope serializes one daemon envelope for the outbox.
func EncodeEnvelope(gatewayID, daemonID string, env *runtimev1.Envelope) (json.RawMessage, error) {
	encoded, err := proto.Marshal(env)
	if err != nil {
		return nil, err
	}
	return json.Marshal(EnvelopePayload{
		GatewayID: gatewayID,
		DaemonID:  daemonID,
		Envelope:  encoded,
	})
}

// DecodeEnvelope restores one outbox envelope.
func DecodeEnvelope(payload json.RawMessage) (EnvelopePayload, *runtimev1.Envelope, error) {
	var stored EnvelopePayload
	if err := json.Unmarshal(payload, &stored); err != nil {
		return EnvelopePayload{}, nil, err
	}
	env := &runtimev1.Envelope{}
	if err := proto.Unmarshal(stored.Envelope, env); err != nil {
		return EnvelopePayload{}, nil, err
	}
	return stored, env, nil
}

// ConsumeEnvelopeOutbox claims and applies Runtime Gateway envelopes until ctx
// is canceled. Duplicate deliveries are safe at the event level; tool calls
// retain their invocation idempotency.
func ConsumeEnvelopeOutbox(
	ctx context.Context,
	store *cluster.Store,
	service *Service,
	consumerID string,
	pollInterval time.Duration,
	logger *slog.Logger,
) {
	if store == nil || service == nil {
		return
	}
	if pollInterval <= 0 {
		pollInterval = 100 * time.Millisecond
	}
	if logger == nil {
		logger = slog.Default()
	}
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		claimCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		notifications, err := store.ClaimNotifications(
			claimCtx,
			EnvelopeOutboxChannel,
			consumerID,
			32,
			30*time.Second,
		)
		cancel()
		if err != nil {
			if ctx.Err() == nil {
				logger.Warn("claim control envelopes failed", "err", err)
			}
			continue
		}
		for _, notification := range notifications {
			deliverCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			stored, env, decodeErr := DecodeEnvelope(notification.Payload)
			if decodeErr != nil {
				cancel()
				logger.Warn("discard invalid control envelope",
					"notification_id", notification.NotificationID,
					"err", decodeErr,
				)
				_ = store.MarkNotificationPublished(ctx, notification.NotificationID)
				continue
			}
			_, deliverErr := service.DeliverEnvelope(deliverCtx, &rolev1.DeliverEnvelopeRequest{
				GatewayId: stored.GatewayID,
				DaemonId:  stored.DaemonID,
				Envelope:  env,
			})
			cancel()
			if deliverErr != nil {
				logger.Warn("apply control envelope failed",
					"notification_id", notification.NotificationID,
					"daemon_id", stored.DaemonID,
					"err", deliverErr,
				)
				_ = store.RescheduleNotification(ctx, notification.NotificationID, time.Second)
				continue
			}
			if err := store.MarkNotificationPublished(ctx, notification.NotificationID); err != nil {
				logger.Warn("mark control envelope published failed",
					"notification_id", notification.NotificationID,
					"err", err,
				)
			}
		}
	}
}
