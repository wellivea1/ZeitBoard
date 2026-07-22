package main

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	medicationcore "non24.app/core/medication"
	storage "non24.app/core/storage/sqlite"
)

const (
	medicationReminderPollInterval = 20 * time.Second
	medicationReminderGrace        = 2 * time.Minute
	medicationReminderTitle        = "Medication reminder"
)

type medicationReminderDelivery struct {
	Due     int
	Claimed int
	Shown   int
	Failed  int
}

type medicationReminderServiceState struct {
	Running   bool
	LastError string
}

func (a *App) startMedicationReminderService(parent context.Context) {
	if parent == nil {
		parent = context.Background()
	}
	a.reminderMu.Lock()
	if a.reminderCancel != nil {
		a.reminderMu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(parent)
	done := make(chan struct{})
	a.reminderCancel = cancel
	a.reminderDone = done
	a.reminderRunning = true
	a.reminderLastError = ""
	a.reminderMu.Unlock()

	go func() {
		defer func() {
			a.reminderMu.Lock()
			a.reminderRunning = false
			a.reminderMu.Unlock()
			close(done)
		}()
		a.checkMedicationReminders(ctx)
		ticker := time.NewTicker(medicationReminderPollInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				a.checkMedicationReminders(ctx)
			}
		}
	}()
}

func (a *App) stopMedicationReminderService() {
	a.reminderMu.RLock()
	cancel := a.reminderCancel
	done := a.reminderDone
	a.reminderMu.RUnlock()
	if cancel == nil {
		return
	}
	cancel()
	if done != nil {
		<-done
	}
	a.reminderMu.Lock()
	if a.reminderDone == done {
		a.reminderCancel = nil
		a.reminderDone = nil
	}
	a.reminderMu.Unlock()
}

func (a *App) checkMedicationReminders(ctx context.Context) {
	if _, err := a.deliverMedicationReminders(ctx, a.currentTime().UTC().Truncate(time.Second)); err != nil && !errors.Is(err, context.Canceled) {
		a.setMedicationReminderError("Medication reminders could not be checked; scheduled times remain unchanged.")
	}
}

func (a *App) deliverMedicationReminders(ctx context.Context, now time.Time) (medicationReminderDelivery, error) {
	result := medicationReminderDelivery{}
	if now.IsZero() {
		return result, errors.New("reminder check time is required")
	}
	store, err := a.requireStore()
	if err != nil {
		return result, err
	}
	medications, err := store.ListMedications(ctx)
	if err != nil {
		return result, err
	}
	from := now.Add(-medicationReminderGrace)
	to := now.Add(time.Nanosecond)
	for _, record := range medications {
		if !record.Active || record.Schedule == nil || !record.Schedule.ReminderEnabled {
			continue
		}
		expansion, err := medicationcore.ExpandSchedule(*record.Schedule, from, to)
		if err != nil {
			return result, fmt.Errorf("expand persisted medication schedule: %w", err)
		}
		for _, occurrence := range expansion.Occurrences {
			result.Due++
			claim := storage.MedicationReminderClaim{
				OccurrenceID: medicationReminderOccurrenceID(record.MedicationID, occurrence.At.UTC),
				MedicationID: record.MedicationID,
				ScheduledAt:  occurrence.At.UTC,
				ClaimedAt:    now,
			}
			claimed, err := store.ClaimMedicationReminder(ctx, claim)
			if err != nil {
				return result, err
			}
			if !claimed {
				continue
			}
			result.Claimed++
			message := "Reminder you set: " + medicationNotificationLabel(record.Label) + "."
			if err := a.tray.Notify(medicationReminderTitle, message); err != nil {
				result.Failed++
				a.setMedicationReminderError("A medication reminder could not be shown. ZeitBoard will not repeat it automatically.")
				continue
			}
			result.Shown++
		}
	}
	return result, nil
}

func medicationReminderOccurrenceID(medicationID string, scheduledAt time.Time) string {
	digest := sha256.Sum256([]byte(medicationID + "|" + scheduledAt.UTC().Format(time.RFC3339Nano)))
	return fmt.Sprintf("reminder_%x", digest[:20])
}

func medicationNotificationLabel(label string) string {
	cleaned := strings.Map(func(value rune) rune {
		if unicode.IsControl(value) {
			return ' '
		}
		return value
	}, label)
	cleaned = strings.Join(strings.Fields(cleaned), " ")
	if cleaned == "" {
		return "Medication"
	}
	return cleaned
}

func (a *App) setMedicationReminderError(message string) {
	a.reminderMu.Lock()
	a.reminderLastError = message
	a.reminderMu.Unlock()
}

func (a *App) medicationReminderServiceState() medicationReminderServiceState {
	a.reminderMu.RLock()
	defer a.reminderMu.RUnlock()
	return medicationReminderServiceState{
		Running:   a.reminderRunning,
		LastError: a.reminderLastError,
	}
}

func (a *App) medicationReminderStatus(medications []storage.MedicationRecord) (string, string) {
	enabled := 0
	for _, record := range medications {
		if record.Active && record.Schedule != nil && record.Schedule.ReminderEnabled {
			enabled++
		}
	}
	if enabled == 0 {
		return "disabled", "Desktop reminders are off. Enable them only on a clock schedule you entered."
	}
	state := a.medicationReminderServiceState()
	if state.LastError != "" {
		if state.Running {
			return "error", state.LastError
		}
		return "unavailable", state.LastError
	}
	if !state.Running {
		return "unavailable", "Desktop reminder delivery is not running; scheduled times remain stored and unchanged."
	}
	return "ready", fmt.Sprintf("Desktop reminders are active for %d %s you configured.", enabled, plural(enabled, "medication", "medications"))
}
