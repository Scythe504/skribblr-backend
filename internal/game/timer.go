package game

import (
	"context"
	"log"
	"time"

	"github.com/scythe504/skribblr-backend/internal"
)

// =============================================================================
// TIMER MANAGEMENT
// =============================================================================

// StartPhaseTimer creates and manages a phase timer with regular updates
func StartPhaseTimer(room *internal.Room, duration time.Duration, onExpire func()) {
	log.Printf("[StartPhaseTimer] Room %s: Function called with duration=%v", room.Id, duration)

	// --- Critical section ---
	log.Printf("[StartPhaseTimer] Room %s: Acquiring lock", room.Id)
	room.Mu.Lock()
	log.Printf("[StartPhaseTimer] Room %s: Lock acquired", room.Id)

	// 1. Cancel any existing timer
	log.Printf("[StartPhaseTimer] Room %s: Calling CancelPhaseTimer to cancel existing timer", room.Id)
	room.Mu.Unlock()
	CancelPhaseTimer(room)
	room.Mu.Lock()
	log.Printf("[StartPhaseTimer] Room %s: CancelPhaseTimer completed", room.Id)

	// 2. Create new context with cancellation
	log.Printf("[StartPhaseTimer] Room %s: Creating context with timeout %v", room.Id, duration)
	ctx, cancel := context.WithTimeout(context.Background(), duration)
	log.Printf("[StartPhaseTimer] Room %s: Context created successfully", room.Id)

	// 3. Create GameTimer struct
	startTime := time.Now()
	log.Printf("[StartPhaseTimer] Room %s: Creating GameTimer with startTime=%v, duration=%v, isActive=true",
		room.Id, startTime, duration)
	room.Timer = &internal.GameTimer{
		StartTime: startTime,
		Duration:  duration,
		IsActive:  true,
		Context:   ctx,
		Cancel:    cancel,
	}
	log.Printf("[StartPhaseTimer] Room %s: Timer started for %v", room.Id, duration)
	log.Printf("[StartPhaseTimer] Room %s: GameTimer created and assigned to room", room.Id)

	log.Printf("[StartPhaseTimer] Room %s: Releasing lock", room.Id)
	room.Mu.Unlock()
	log.Printf("[StartPhaseTimer] Room %s: Lock released", room.Id)
	// --- End critical section ---

	// 4. Start goroutine (no locks held)
	log.Printf("[StartPhaseTimer] Room %s: Starting timer goroutine", room.Id)
	go func() {
		log.Printf("[StartPhaseTimer] Room %s: Timer goroutine started", room.Id)

		log.Printf("[StartPhaseTimer] Room %s: Creating ticker with 1 second interval", room.Id)
		ticker := time.NewTicker(1 * time.Second)
		defer func() {
			log.Printf("[StartPhaseTimer] Room %s: Stopping ticker in defer", room.Id)
			ticker.Stop()
		}()
		log.Printf("[StartPhaseTimer] Room %s: Ticker created, entering select loop", room.Id)

		for {
			select {
			case <-ticker.C:
				// Periodic update (safe snapshot inside function)
				log.Printf("[StartPhaseTimer] Room %s: Ticker fired, calling BroadcastTimerUpdate", room.Id)
				BroadcastTimerUpdate(room)
				log.Printf("[StartPhaseTimer] Room %s: BroadcastTimerUpdate completed", room.Id)

			case <-ctx.Done():
				// Expiry or cancel
				log.Printf("[StartPhaseTimer] Room %s: Context done signal received", room.Id)

				log.Printf("[StartPhaseTimer] Room %s: Acquiring lock to check timer state", room.Id)
				room.Mu.Lock()
				active := room.Timer != nil && room.Timer.Context == ctx
				log.Printf("[StartPhaseTimer] Room %s: Timer active check - timer exists: %t, context matches: %t, active: %t",
					room.Id, room.Timer != nil, room.Timer != nil && room.Timer.Context == ctx, active)

				if active {
					// Mark inactive so BroadcastTimerUpdate stops
					log.Printf("[StartPhaseTimer] Room %s: Marking timer as inactive", room.Id)
					room.Timer.IsActive = false
					log.Printf("[StartPhaseTimer] Room %s: Timer marked as inactive", room.Id)
				}
				log.Printf("[StartPhaseTimer] Room %s: Releasing lock after timer state update", room.Id)
				room.Mu.Unlock()

				contextErr := ctx.Err()
				log.Printf("[StartPhaseTimer] Room %s: Context error: %v", room.Id, contextErr)

				if contextErr == context.DeadlineExceeded {
					// Natural expiry
					log.Printf("[StartPhaseTimer] Room %s: Timer expired after %v", room.Id, duration)
					log.Printf("[StartPhaseTimer] Room %s: Starting goroutine to call onExpire callback", room.Id)
					// Run callback in a separate goroutine so timer goroutine can exit immediately
					go onExpire()
				} else {
					// Cancelled explicitly
					log.Printf("[StartPhaseTimer] Room %s: Timer cancelled before expiry", room.Id)
				}
				log.Printf("[StartPhaseTimer] Room %s: Timer goroutine exiting", room.Id)
				return
			}
		}
	}()
	log.Printf("[StartPhaseTimer] Room %s: Function completed, timer goroutine launched", room.Id)
}

// BroadcastTimerUpdate sends current timer state to all players
func BroadcastTimerUpdate(room *internal.Room) {
	if room == nil {
		return
	}

	room.Mu.Lock()
	if room.Timer == nil || !room.Timer.IsActive {
		room.Mu.Unlock()
		return
	}

	remaining := max(room.Timer.Duration-time.Since(room.Timer.StartTime), 0)
	room.Timer.TimeRemaining = remaining

	// Snapshot timer update
	timerUpdateData := internal.TimerUpdateData{
		TimeRemaining: remaining.Milliseconds(),
		Phase:         room.Phase,
		IsActive:      room.Timer.IsActive,
	}
	roomID := room.Id

	room.Mu.Unlock()

	log.Printf("[BroadcastTimerUpdate] room=%s: remaining=%dms phase=%v active=%v",
		roomID, remaining.Milliseconds(), timerUpdateData.Phase, timerUpdateData.IsActive)

	SafeBroadcastToRoom(room, internal.Message[any]{
		Type: "timer_update",
		Data: timerUpdateData,
	})
}

// CancelPhaseTimer stops current phase timer
func CancelPhaseTimer(room *internal.Room) {
	log.Printf("[CancelPhaseTimer] Function called")

	if room == nil {
		log.Printf("[CancelPhaseTimer] Room is nil, returning early")
		return
	}

	log.Printf("[CancelPhaseTimer] Room %s: Room is valid, proceeding", room.Id)

	log.Printf("[CancelPhaseTimer] Room %s: Acquiring lock", room.Id)
	room.Mu.Lock()
	log.Printf("[CancelPhaseTimer] Room %s: Lock acquired", room.Id)

	if room.Timer == nil || !room.Timer.IsActive {
		log.Printf("[CancelPhaseTimer] Room %s: No active timer - Timer exists: %t, IsActive: %t",
			room.Id, room.Timer != nil, room.Timer != nil && room.Timer.IsActive)
		room.Mu.Unlock()
		log.Printf("[CancelPhaseTimer] Room %s: Released lock and returning early - no active timer", room.Id)
		return
	}

	log.Printf("[CancelPhaseTimer] Room %s: Active timer found - IsActive: %t, TimeRemaining: %v",
		room.Id, room.Timer.IsActive, room.Timer.TimeRemaining)

	// Cancel goroutine via context
	if room.Timer.Cancel != nil {
		log.Printf("[CancelPhaseTimer] Room %s: Calling timer.Cancel() function", room.Id)
		room.Timer.Cancel()
		log.Printf("[CancelPhaseTimer] Room %s: Timer.Cancel() called successfully", room.Id)
	} else {
		log.Printf("[CancelPhaseTimer] Room %s: Timer.Cancel is nil, cannot cancel context", room.Id)
	}

	log.Printf("[CancelPhaseTimer] Room %s: Setting timer.IsActive from %t to false", room.Id, room.Timer.IsActive)
	room.Timer.IsActive = false
	log.Printf("[CancelPhaseTimer] Room %s: Setting timer.TimeRemaining from %v to 0", room.Id, room.Timer.TimeRemaining)
	room.Timer.TimeRemaining = 0

	// Snapshot update before unlock
	log.Printf("[CancelPhaseTimer] Room %s: Creating timer update data snapshot - Phase: %s", room.Id, room.Phase)
	timerUpdateData := internal.TimerUpdateData{
		TimeRemaining: 0,
		Phase:         room.Phase,
		IsActive:      false,
	}
	roomID := room.Id
	log.Printf("[CancelPhaseTimer] Room %s: Snapshotted values - TimeRemaining: %d, Phase: %s, IsActive: %t",
		roomID, timerUpdateData.TimeRemaining, timerUpdateData.Phase, timerUpdateData.IsActive)

	log.Printf("[CancelPhaseTimer] Room %s: Releasing lock", room.Id)
	room.Mu.Unlock()
	log.Printf("[CancelPhaseTimer] Room %s: Lock released", roomID)

	log.Printf("[CancelPhaseTimer] room=%s: timer cancelled", roomID)

	log.Printf("[CancelPhaseTimer] Room %s: Broadcasting timer_update message with cancelled state", roomID)
	SafeBroadcastToRoom(room, internal.Message[any]{
		Type: "timer_update",
		Data: timerUpdateData,
	})
	log.Printf("[CancelPhaseTimer] Room %s: Timer cancellation completed successfully", roomID)
}
