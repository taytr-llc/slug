// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"errors"
	"github.com/taytr-llc/slug/fightercarrier"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymstable"
)

// One original child owns the opaque captures. Public IDs only select those
// retained handles; they never reconstruct a capture from a newer source head.
func (c *OwnedGymChild) CaptureStable(ctx context.Context, view, id string) (OwnedStableCapture, error) {
	return ownedChildCall(ctx, c, view, false, func(work context.Context) (OwnedStableCapture, error) {
		if !payoutNonzeroID(id) {
			return OwnedStableCapture{}, gymstable.ErrInvalid
		}
		if c.captureID == id && c.capture != nil {
			return ownedCopyCapture(c.captureValue)
		}
		next, e := c.stable.Capture(work, c.session())
		if e != nil {
			return OwnedStableCapture{}, e
		}
		page, e := c.stable.Page(work, c.session(), next, 0, 8)
		if e != nil {
			return OwnedStableCapture{}, e
		}
		value := OwnedStableCapture{Version: 1, CaptureID: id, ViewID: view, Page: page}
		if e = ValidateOwnedStableCaptureWithScope(value, c.scope, c.view); e != nil {
			return OwnedStableCapture{}, e
		}
		// Assign only inside the original final disclosure fence. No database or
		// source traversal occurs in this bounded local publication.
		saved, e := ownedCopyCapture(value)
		if e != nil {
			return OwnedStableCapture{}, e
		}
		e = c.fence(work, func() error {
			if !c.mu.TryLock() {
				return gymstable.ErrBusy
			}
			defer c.mu.Unlock()
			c.captureID, c.capture, c.captureValue = id, next, saved
			head := page.Head
			c.snapshot.OwnershipHead = &head
			return nil
		})
		if e != nil {
			return OwnedStableCapture{}, e
		}
		return value, nil
	})
}
func (c *OwnedGymChild) StablePage(ctx context.Context, view, id string, after, limit uint32) (gymstable.OwnedPage, error) {
	return ownedChildCall(ctx, c, view, false, func(work context.Context) (gymstable.OwnedPage, error) {
		if !payoutNonzeroID(id) || id != c.captureID || c.capture == nil || limit == 0 || limit > 8 {
			return gymstable.OwnedPage{}, gymstable.ErrInvalid
		}
		return c.stable.Page(work, c.session(), c.capture, after, limit)
	})
}
func (c *OwnedGymChild) CaptureHistory(ctx context.Context, view, prospect string) (gymstable.OwnedCareerHistoryCapture, error) {
	return ownedChildCall(ctx, c, view, false, func(work context.Context) (gymstable.OwnedCareerHistoryCapture, error) {
		value, read, e := c.stable.PrepareCareerHistory(work, c.session(), prospect)
		if e != nil {
			return value, e
		}
		if read == nil || read.RetainedBytes() > gymstable.OwnedMaxHistoryRetainedBytes {
			return gymstable.OwnedCareerHistoryCapture{}, gymstable.ErrCapacity
		}
		e = c.fence(work, func() error { c.history, c.historyID = read, value.HistoryID; return nil })
		if e != nil {
			return gymstable.OwnedCareerHistoryCapture{}, e
		}
		return value, nil
	})
}
func (c *OwnedGymChild) HistoryPage(ctx context.Context, view, id string, after, limit uint32) (gymstable.OwnedCareerHistoryPage, error) {
	return ownedChildCall(ctx, c, view, false, func(work context.Context) (gymstable.OwnedCareerHistoryPage, error) {
		if !payoutNonzeroID(id) || id != c.historyID || c.history == nil || limit == 0 || limit > 8 {
			return gymstable.OwnedCareerHistoryPage{}, gymstable.ErrInvalid
		}
		return c.stable.ReadCareerHistory(work, c.session(), c.history, after, limit)
	})
}
func (c *OwnedGymChild) PrepareSlot(ctx context.Context, view string, request gymstable.OwnedSlotRequest) (gymstable.OwnedIntent, error) {
	return ownedChildCall(ctx, c, view, true, func(work context.Context) (gymstable.OwnedIntent, error) {
		return c.stable.PrepareSlot(work, c.session(), request)
	})
}
func (c *OwnedGymChild) ApplySlot(ctx context.Context, view string, id string) (gymstable.OwnedMutationResult, error) {
	return ownedChildCall(ctx, c, view, true, func(work context.Context) (gymstable.OwnedMutationResult, error) {
		return c.stable.ApplySlot(work, c.session(), id)
	})
}
func (c *OwnedGymChild) SlotOperation(ctx context.Context, view string, id string) (gymstable.OwnedOperation, error) {
	return ownedChildCall(ctx, c, view, false, func(work context.Context) (gymstable.OwnedOperation, error) {
		return c.stable.Operation(work, c.session(), id)
	})
}
func (c *OwnedGymChild) PendingSlot(ctx context.Context, view string) (*gymstable.OwnedOperation, error) {
	return ownedChildCall(ctx, c, view, false, func(work context.Context) (*gymstable.OwnedOperation, error) {
		return c.stable.Pending(work, c.session())
	})
}
func (c *OwnedGymChild) AbandonSlot(ctx context.Context, view string, id string) (gymstable.OwnedOperation, error) {
	return ownedChildCall(ctx, c, view, true, func(work context.Context) (gymstable.OwnedOperation, error) {
		return c.stable.Abandon(work, c.session(), id)
	})
}
func (c *OwnedGymChild) EntryUpcoming(ctx context.Context, view string) (gymentry.OwnedEventStatus, error) {
	return ownedChildCall(ctx, c, view, false, func(work context.Context) (gymentry.OwnedEventStatus, error) {
		return c.entries.Upcoming(work, c.session())
	})
}
func (c *OwnedGymChild) EntryStatus(ctx context.Context, view string, date string) (gymentry.OwnedEventStatus, error) {
	return ownedChildCall(ctx, c, view, false, func(work context.Context) (gymentry.OwnedEventStatus, error) {
		return c.entries.Status(work, c.session(), date)
	})
}
func (c *OwnedGymChild) PrepareEntry(ctx context.Context, view string, request gymentry.Request) (gymentry.OwnedIntent, error) {
	return ownedChildCall(ctx, c, view, true, func(work context.Context) (gymentry.OwnedIntent, error) {
		return c.entries.Prepare(work, c.session(), request)
	})
}
func (c *OwnedGymChild) ApplyEntry(ctx context.Context, view string, id string) (gymentry.OwnedMutationResult, error) {
	return ownedChildCall(ctx, c, view, true, func(work context.Context) (gymentry.OwnedMutationResult, error) {
		return c.entries.Apply(work, c.session(), id)
	})
}
func (c *OwnedGymChild) EntryOperation(ctx context.Context, view string, id string) (gymentry.OwnedOperation, error) {
	return ownedChildCall(ctx, c, view, false, func(work context.Context) (gymentry.OwnedOperation, error) {
		return c.entries.Operation(work, c.session(), id)
	})
}
func (c *OwnedGymChild) PendingEntry(ctx context.Context, view string) (*gymentry.OwnedOperation, error) {
	return ownedChildCall(ctx, c, view, false, func(work context.Context) (*gymentry.OwnedOperation, error) {
		return c.entries.Pending(work, c.session())
	})
}
func (c *OwnedGymChild) AbandonEntry(ctx context.Context, view string, id string) (gymentry.OwnedOperation, error) {
	return ownedChildCall(ctx, c, view, true, func(work context.Context) (gymentry.OwnedOperation, error) {
		return c.entries.Abandon(work, c.session(), id)
	})
}
func (c *OwnedGymChild) EventStatus(ctx context.Context, view string, date string) (gymevent.OwnedEventStatus, error) {
	return ownedChildCall(ctx, c, view, false, func(work context.Context) (gymevent.OwnedEventStatus, error) {
		return c.events.Observe(work, c.entries, c.session(), date)
	})
}
func (c *OwnedGymChild) EventRecord(ctx context.Context, view string, date string) (gymevent.OwnedSnapshotReceipt, error) {
	return ownedChildCall(ctx, c, view, false, func(work context.Context) (gymevent.OwnedSnapshotReceipt, error) {
		return c.events.Read(work, c.entries, c.session(), date)
	})
}
func (c *OwnedGymChild) TransferPrepare(ctx context.Context, view string, id, prospect, recipient string) (OwnedTransferReview, error) {
	return ownedChildCall(ctx, c, view, true, func(work context.Context) (OwnedTransferReview, error) {
		return c.transfers.Prepare(work, id, prospect, recipient)
	})
}
func (c *OwnedGymChild) TransferResume(ctx context.Context, view string, id, prospect string) (OwnedTransferReview, error) {
	return ownedChildCall(ctx, c, view, true, func(work context.Context) (OwnedTransferReview, error) { return c.transfers.Resume(work, id, prospect) })
}
func (c *OwnedGymChild) TransferConfirm(ctx context.Context, view string, id string) (fightercarrier.Operation, error) {
	return ownedChildCall(ctx, c, view, true, func(work context.Context) (fightercarrier.Operation, error) { return c.transfers.Confirm(work, id) })
}
func (c *OwnedGymChild) TransferReviewDelivery(ctx context.Context, view string, id, leg string) (OwnedTransferDeliveryReview, error) {
	return ownedChildCall(ctx, c, view, false, func(work context.Context) (OwnedTransferDeliveryReview, error) {
		return c.transfers.ReviewDelivery(work, id, leg)
	})
}
func (c *OwnedGymChild) TransferDeliver(ctx context.Context, view string, id string) (fightercarrier.Operation, error) {
	return ownedChildCall(ctx, c, view, true, func(work context.Context) (fightercarrier.Operation, error) { return c.transfers.Deliver(work, id) })
}
func (c *OwnedGymChild) TransferObserve(ctx context.Context, view string, id, leg string) (fightercarrier.Operation, error) {
	return ownedChildCall(ctx, c, view, false, func(work context.Context) (fightercarrier.Operation, error) {
		return c.transfers.Observe(work, id, leg)
	})
}
func (c *OwnedGymChild) TransferOperation(ctx context.Context, view string, id string) (fightercarrier.Operation, error) {
	return ownedChildCall(ctx, c, view, false, func(work context.Context) (fightercarrier.Operation, error) { return c.transfers.Operation(work, id) })
}
func (c *OwnedGymChild) TransferAbandon(ctx context.Context, view string, id string) (fightercarrier.Operation, error) {
	return ownedChildCall(ctx, c, view, true, func(work context.Context) (fightercarrier.Operation, error) {
		return c.transfers.AbandonUnsigned(work, id)
	})
}

func ownedChildCall[T any](ctx context.Context, c *OwnedGymChild, view string, mutation bool, fn func(context.Context) (T, error)) (T, error) {
	var zero T
	work, done, e := c.begin(ctx, view)
	if e != nil {
		return zero, e
	}
	defer done()
	value, e := fn(work)
	if e != nil {
		return zero, e
	}
	if e = c.fence(work, func() error { return nil }); e != nil {
		if mutation {
			return zero, errors.Join(gymstable.ErrOutcomeUnknown, e)
		}
		return zero, e
	}
	return value, nil
}

func ownedCopyCapture(v OwnedStableCapture) (OwnedStableCapture, error) {
	raw, e := gymstable.MarshalOwnedPage(v.Page)
	if e != nil {
		return OwnedStableCapture{}, e
	}
	p, e := gymstable.ParseOwnedPage(raw)
	if e != nil {
		return OwnedStableCapture{}, e
	}
	v.Page = p
	return v, nil
}

func (c *OwnedGymChild) TransferFinality(ctx context.Context, view, id string) (OwnedTransferFinality, error) {
	return ownedChildCall(ctx, c, view, false, func(work context.Context) (OwnedTransferFinality, error) { return c.transfers.Finality(work, id) })
}
