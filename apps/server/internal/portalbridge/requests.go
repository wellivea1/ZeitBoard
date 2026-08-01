package portalbridge

import (
	"context"
	"errors"
	"fmt"
	"time"

	"non24.app/server/internal/portal"
	"non24.app/server/internal/store"
)

// VisitorProposalTTL bounds how long a visitor's request waits for an answer
// before its decision token expires along with every other proposal.
const VisitorProposalTTL = 7 * 24 * time.Hour

// RequestBridge carries visitor requests into the owner's proposal queue and
// carries decisions back. It is the only component holding both stores, and it
// is deliberately one-way per direction: the portal never learns anything
// about the private store beyond a status it was handed.
type RequestBridge struct {
	Portal  *portal.Store
	Private *store.Store
	Now     func() time.Time

	// OwnerDevice attributes portal-created proposals to an enrolled device.
	// Proposals are keyed to a creating device, and a visitor has none, so the
	// bridge borrows the owner's earliest enrolled device.
	OwnerDevice func(ctx context.Context) (string, error)
}

func (b RequestBridge) now() time.Time {
	if b.Now != nil {
		return b.Now().UTC()
	}
	return time.Now().UTC()
}

// Pump runs one full cycle in both directions. It is safe to call repeatedly;
// every step is idempotent.
func (b RequestBridge) Pump(ctx context.Context) error {
	if err := b.submitRequests(ctx); err != nil {
		return err
	}
	return b.deliverDecisions(ctx)
}

// submitRequests turns queued portal requests into pending private proposals.
// A failure leaves the request in `queued`, which is what the visitor sees:
// "waiting to reach them", not a false "they have it".
func (b RequestBridge) submitRequests(ctx context.Context) error {
	entries, err := b.Portal.PendingOutbox(ctx, 50)
	if err != nil {
		return fmt.Errorf("read portal outbox: %w", err)
	}
	if len(entries) == 0 {
		return nil
	}
	deviceID, err := b.ownerDevice(ctx)
	if err != nil {
		// Without an enrolled device there is nowhere to file the proposal.
		// Leaving the rows queued is correct: they are not lost, and the
		// visitor is not told the owner received something they did not.
		return fmt.Errorf("resolve owner device: %w", err)
	}

	var failures []error
	for _, entry := range entries {
		if entry.Kind != "proposal_submit" {
			continue
		}
		now := b.now()
		_, err := b.Private.CreateVisitorProposal(ctx, store.VisitorRequestInput{
			PortalRequestID: entry.Request.ID,
			ProfileID:       entry.Request.ProfileID,
			DeviceID:        deviceID,
			WindowStart:     entry.Request.WindowStart,
			WindowEnd:       entry.Request.WindowEnd,
			ZoneID:          entry.Request.ZoneID,
			DurationMinutes: entry.Request.DurationMinutes,
			BeyondHorizon:   entry.Request.BeyondHorizon,
			Handle:          entry.Request.Handle,
			Message:         entry.Request.Message,
			CreatedAt:       entry.Request.CreatedAt,
			ExpiresAt:       now.Add(VisitorProposalTTL),
		})
		if err != nil {
			if noteErr := b.Portal.NoteOutboxFailure(ctx, entry.ID, err.Error()); noteErr != nil {
				return noteErr
			}
			failures = append(failures, err)
			continue
		}
		if err := b.Portal.AckOutbox(ctx, entry.ID, entry.Request.ID, now); err != nil {
			// The proposal exists but the acknowledgement did not land. The
			// next pass re-submits, finds the existing proposal by its portal
			// request id, and acknowledges then.
			return fmt.Errorf("acknowledge portal outbox: %w", err)
		}
	}
	return errors.Join(failures...)
}

// deliverDecisions applies owner decisions to the portal store exactly once.
func (b RequestBridge) deliverDecisions(ctx context.Context) error {
	entries, err := b.Private.PendingPortalStatus(ctx, 50)
	if err != nil {
		return fmt.Errorf("read portal status outbox: %w", err)
	}
	for _, entry := range entries {
		status := portal.RequestDeclined
		if entry.Status == "approved" {
			status = portal.RequestApproved
		}
		if err := b.Portal.ApplyDecision(ctx, entry.PortalRequestID, status,
			entry.DecidedStart, entry.DecidedEnd, b.now()); err != nil {
			return fmt.Errorf("apply portal decision: %w", err)
		}
		if err := b.Private.AckPortalStatus(ctx, entry.ID); err != nil {
			return fmt.Errorf("acknowledge portal status: %w", err)
		}
	}
	return nil
}

func (b RequestBridge) ownerDevice(ctx context.Context) (string, error) {
	if b.OwnerDevice != nil {
		return b.OwnerDevice(ctx)
	}
	devices, err := b.Private.ListDevices(ctx)
	if err != nil {
		return "", err
	}
	for _, device := range devices {
		if device.RevokedAt == nil {
			return device.ID, nil
		}
	}
	return "", errors.New("no enrolled device is available to receive visitor requests")
}
