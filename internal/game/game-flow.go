package game

import (
	"fmt"
	"log"
	"slices"
	"time"

	"github.com/scythe504/skribblr-backend/internal"
	"github.com/scythe504/skribblr-backend/internal/utils"
)

// =============================================================================
// GAME FLOW - ROUND MANAGEMENT
// =============================================================================

// StartWaitingPhase shows next drawer countdown (10 seconds)
func StartWaitingPhase(room *internal.Room) {
	log.Printf("[StartWaitingPhase] Room %s: Function called", room.Id)

	// --- Critical section: update room state ---
	log.Printf("[StartWaitingPhase] Room %s: Acquiring lock", room.Id)
	room.Mu.Lock()
	log.Printf("[StartWaitingPhase] Room %s: Lock acquired", room.Id)

	// 1. Set phase
	log.Printf("[StartWaitingPhase] Room %s: Setting phase from %s to waiting", room.Id, room.Phase)
	room.Phase = internal.PhaseWaiting
	log.Printf("[StartWaitingPhase] Room %s: Phase set to %s", room.Id, room.Phase)

	// 2. Ensure CurrentIndex is valid
	log.Printf("[StartWaitingPhase] Room %s: CurrentIndex=%d, PlayerOrder length=%d", room.Id, room.CurrentIndex, len(room.PlayerOrder))
	if room.CurrentIndex >= len(room.PlayerOrder) {
		log.Printf("[StartWaitingPhase] Room %s: CurrentIndex %d >= PlayerOrder length %d, resetting to 0", room.Id, room.CurrentIndex, len(room.PlayerOrder))
		room.CurrentIndex = 0
		log.Printf("[StartWaitingPhase] Room %s: CurrentIndex reset to %d", room.Id, room.CurrentIndex)
	}

	// 3. Choose next drawer
	if len(room.PlayerOrder) == 0 {
		log.Printf("[StartWaitingPhase] Room %s: PlayerOrder is empty, unlocking and aborting", room.Id)
		room.Mu.Unlock()
		log.Printf("[StartWaitingPhase] Room %s: PlayerOrder empty, aborting waiting phase", room.Id)
		return
	}

	playerID := room.PlayerOrder[room.CurrentIndex]
	log.Printf("[StartWaitingPhase] Room %s: Selected playerID=%s from CurrentIndex=%d", room.Id, playerID, room.CurrentIndex)

	currentDrawer := room.Players[playerID]
	if currentDrawer == nil {
		log.Printf("[StartWaitingPhase] Room %s: Player %s not found in Players map, unlocking and returning", room.Id, playerID)
		// defensive: should not happen, but handle gracefully
		room.Mu.Unlock()
		log.Printf("[StartWaitingPhase] Room %s: Player %s not found in Players map", room.Id, playerID)
		return
	}

	log.Printf("[StartWaitingPhase] Room %s: Found currentDrawer: ID=%s, Username=%s", room.Id, currentDrawer.Id, currentDrawer.Username)
	room.Current = currentDrawer
	log.Printf("[StartWaitingPhase] Room %s: Set room.Current to drawer %s (%s)", room.Id, currentDrawer.Id, currentDrawer.Username)

	// 4. Reset per-player round state
	log.Printf("[StartWaitingPhase] Room %s: Resetting per-player round state for %d players", room.Id, len(room.Players))
	for _, p := range room.Players {
		log.Printf("[StartWaitingPhase] Room %s: Resetting state for player %s (%s): HasGuessed=%t->false, CanDraw=%t->false",
			room.Id, p.Id, p.Username, p.HasGuessed, p.CanDraw)
		p.HasGuessed = false
		p.CanDraw = false
		p.LastGuessTime = time.Time{}
	}
	log.Printf("[StartWaitingPhase] Room %s: Completed per-player state reset", room.Id)

	// 5. Clear round-level slices
	log.Printf("[StartWaitingPhase] Room %s: Clearing round-level data - CorrectGuessers length=%d, CanvasState length=%d",
		room.Id, len(room.CorrectGuessers), len(room.CanvasState))
	room.CorrectGuessers = make([]internal.PlayerGuess, 0)
	room.CanvasState = make([]internal.PixelMessage, 0)
	log.Printf("[StartWaitingPhase] Room %s: Cleared CorrectGuessers and CanvasState", room.Id)

	// Snapshot values to send outside lock
	roomID := room.Id
	drawerID := currentDrawer.Id
	drawerName := currentDrawer.Username
	roundNum := room.RoundNumber
	log.Printf("[StartWaitingPhase] Room %s: Snapshotted values - drawerID=%s, drawerName=%s, roundNum=%d",
		roomID, drawerID, drawerName, roundNum)

	log.Printf("[StartWaitingPhase] Room %s: Releasing lock", room.Id)
	room.Mu.Unlock()
	log.Printf("[StartWaitingPhase] Room %s: Lock released", room.Id)
	// --- end critical section ---

	// Prepare waiting-phase message (no locks held)
	log.Printf("[StartWaitingPhase] Room %s: Preparing waiting_phase message for drawer %s (%s)",
		roomID, drawerID, drawerName)
	waitingPhaseMessage := internal.Message[any]{
		Type: "waiting_phase",
		Data: map[string]any{
			"message": fmt.Sprintf("%s will draw next, selecting word...", drawerName),
			"room_id": roomID,
			"current_drawer": map[string]string{
				"id":       drawerID,
				"username": drawerName,
			},
			"phase":          "waiting",
			"time_remaining": 10, // displayed seconds
			"round_number":   roundNum,
		},
	}
	log.Printf("[StartWaitingPhase] Room %s: Created waiting_phase message with time_remaining=10", roomID)

	log.Printf("[StartWaitingPhase] Room %s: Entering waiting phase. Drawer=%s (%s), round=%d",
		roomID, drawerID, drawerName, roundNum)

	// Broadcast waiting_phase (uses SafeBroadcastToRoom which snapshots connections)
	log.Printf("[StartWaitingPhase] Room %s: Starting goroutine to broadcast waiting_phase message", roomID)
	SafeBroadcastToRoom(room, waitingPhaseMessage)

	// Start a short timer to move to word selection
	// Use StartPhaseTimer which we assume correctly distinguishes cancel vs natural expiry
	log.Printf("[StartWaitingPhase] Room %s: Starting 15-second phase timer for word selection transition", roomID)
	StartPhaseTimer(room, 15*time.Second, func() {
		log.Printf("[StartWaitingPhase] Room %s: Phase timer expired, starting goroutine for word selection", roomID)
		// call next phase in a goroutine to avoid blocking the timer goroutine
		StartWordSelection(room)
	})
	log.Printf("[StartWaitingPhase] Room %s: Function completed successfully", roomID)
}

// StartWordSelection presents 3 word choices to the current drawer.
// - sends choices only to the current drawer (via safe per-connection write)
// - broadcasts a "waiting_for_word" to others
// - sets room.WordChoices under lock and uses an idempotent selection path
func StartWordSelection(room *internal.Room) {
	// --- Critical section: snapshot current drawer and set words ---
	room.Mu.Lock()
	log.Printf("[StartWordSelection] room=%s: acquired lock, preparing word selection", room.Id)

	// Validate state
	if room.Current == nil {
		room.Mu.Unlock()
		log.Printf("[StartWordSelection] room=%s: no current drawer, aborting", room.Id)
		return
	}

	// generate choices (assumes utils.GenerateWordChoices exists and is safe)
	words := utils.GenerateWordChoices()
	log.Printf("[StartWordSelection] room=%s: generated word choices=%v", room.Id, words)

	room.WordChoices = words

	// capture the drawer pointer & room id for use outside lock
	currentDrawer := room.Current
	roomID := room.Id

	room.Mu.Unlock()
	log.Printf("[StartWordSelection] room=%s: released lock after snapshot", roomID)
	// --- end critical section ---

	// Prepare word selection message for the drawer
	wordSelectionMessage := internal.Message[internal.WordSelectionData]{
		Type: "word_selection",
		Data: internal.WordSelectionData{
			Message:   "Please select a word to draw",
			RoomId:    roomID,
			Choices:   words,
			TimeLimit: 15,
		},
	}

	// Send the choices to the drawer using the player's safe writer
	log.Printf("[StartWordSelection] room=%s: sending word choices to drawer %s (%s)",
		roomID, currentDrawer.Id, currentDrawer.Username)

	if err := currentDrawer.SafeWriteJSON(wordSelectionMessage); err != nil {
		// If send fails (player disconnected), auto-select the first word as fallback
		log.Printf("[StartWordSelection] room=%s: failed to send word choices to drawer %s (%s): %v. Auto-selecting first word",
			roomID, currentDrawer.Id, currentDrawer.Username, err)

		// run selection asynchronously to avoid blocking
		go HandleWordSelection(currentDrawer, words[0])
		return
	}

	log.Printf("[StartWordSelection] room=%s: sent word choices to drawer %s (%s)",
		roomID, currentDrawer.Id, currentDrawer.Username)

	// Broadcast to other players that we're waiting for drawer choice
	waitingMessage := internal.Message[any]{
		Type: "waiting_for_word",
		Data: map[string]any{
			"message":        fmt.Sprintf("Waiting for %s to select a word...", currentDrawer.Username),
			"current_drawer": currentDrawer.Username,
			"time_remaining": 15,
		},
	}
	go func() {
		log.Printf("[StartWordSelection] room=%s: broadcasting waiting message to all except drawer %s (%s)",
			roomID, currentDrawer.Id, currentDrawer.Username)
		SafeBroadcastToRoomExcept(room, waitingMessage, currentDrawer)
	}()

	// Start selection timer. If the drawer hasn't selected by timeout, auto-select first word.
	log.Printf("[StartWordSelection] room=%s: starting selection timer (15s)", roomID)
	StartPhaseTimer(room, 15*time.Second, func() {
		log.Printf("[StartWordSelection.Timer] room=%s: timer callback triggered", roomID)

		// In the timer callback we'll attempt an idempotent auto-selection.
		// Acquire lock to check whether the word is already set (someone may have selected it).
		room.Mu.Lock()
		alreadyChosen := room.Word != ""
		choicesCopy := append([]string(nil), room.WordChoices...) // snapshot choices
		room.Mu.Unlock()

		if alreadyChosen {
			log.Printf("[StartWordSelection.Timer] room=%s: word already chosen before timer expiry; skipping auto-select", roomID)
			return
		}
		if len(choicesCopy) == 0 {
			log.Printf("[StartWordSelection.Timer] room=%s: no choices available for auto-select", roomID)
			return
		}

		autoWord := choicesCopy[0]
		log.Printf("[StartWordSelection.Timer] room=%s: auto-selecting word '%s' for drawer %s (%s)",
			roomID, autoWord, currentDrawer.Id, currentDrawer.Username)

		// call HandleWordSelection asynchronously
		go HandleWordSelection(currentDrawer, autoWord)
	})
}

// HandleWordSelection processes drawer's word choice
func HandleWordSelection(player *internal.Player, selectedWord string) {
	room := player.Room
	if room == nil {
		log.Printf("[HandleWordSelection] player %s: no room reference, aborting", player.Id)
		return
	}

	// Acquire lock and validate/set in one atomic operation.
	room.Mu.Lock()
	// 1. Verify player is current drawer
	if room.Current == nil || player.Id != room.Current.Id {
		log.Printf("[HandleWordSelection] room=%s player=%s (%s) is not current drawer, ignoring selection",
			room.Id, player.Id, player.Username)
		room.Mu.Unlock()
		return
	}

	// 1.5 If word already chosen (idempotency) -> ignore
	if room.Word != "" {
		log.Printf("[HandleWordSelection] room=%s: word already chosen ('%s'), ignoring selection by %s",
			room.Id, room.Word, player.Id)
		room.Mu.Unlock()
		return
	}

	// 2. Verify selectedWord exists in room.WordChoices
	if !slices.Contains(room.WordChoices, selectedWord) {
		log.Printf("[HandleWordSelection] room=%s player=%s chose invalid word: %q",
			room.Id, player.Id, selectedWord)
		room.Mu.Unlock()
		return
	}

	// 3. Set room.Word = selectedWord and clear choices (all under lock)
	room.Word = selectedWord
	room.WordChoices = make([]string, 0)
	log.Printf("[HandleWordSelection] room=%s: player=%s selected word '%s'", room.Id, player.Id, selectedWord)

	// Snapshot minimal info for later use (if needed) before unlock
	room.Mu.Unlock()

	// 4. Cancel word selection timer (safe to call after unlocking)
	CancelPhaseTimer(room)

	// 5. Transition to drawing phase. Do in a goroutine so caller (timer or ws reader) is not blocked.
	go func() {
		StartDrawingPhase(room)
	}()

	// done
}

// StartDrawingPhase begins main drawing/guessing gameplay (75 seconds)
func StartDrawingPhase(room *internal.Room) {
	if room == nil {
		log.Println("[StartDrawingPhase] nil room, abort")
		return
	}

	// --- Critical section: set up round state ---
	room.Mu.Lock()
	log.Printf("[StartDrawingPhase] room=%s: acquiring lock for setup", room.Id)

	// validate that a word is present and current drawer exists
	if room.Current == nil {
		log.Printf("[StartDrawingPhase] room=%s: no current drawer, aborting drawing phase", room.Id)
		room.Mu.Unlock()
		return
	}
	if room.Word == "" {
		log.Printf("[StartDrawingPhase] room=%s: no word chosen, aborting drawing phase", room.Id)
		room.Mu.Unlock()
		return
	}

	// 1. Set phase
	room.Phase = internal.PhaseDrawing
	log.Printf("[StartDrawingPhase] room=%s: phase set to drawing", room.Id)

	// 2. Allow current drawer to draw
	room.Current.CanDraw = true
	log.Printf("[StartDrawingPhase] room=%s: drawer=%s can now draw", room.Id, room.Current.Id)

	// 3. Clear previous correct guessers
	room.CorrectGuessers = make([]internal.PlayerGuess, 0)
	log.Printf("[StartDrawingPhase] room=%s: cleared previous correct guessers", room.Id)

	// 4. Reset HasGuessed for all players
	for _, p := range room.Players {
		if p != nil {
			p.HasGuessed = false
		}
	}
	log.Printf("[StartDrawingPhase] room=%s: reset HasGuessed for all players", room.Id)

	// Snapshot values to use after unlocking
	roomID := room.Id
	drawer := room.Current     // pointer to drawer player
	wordForDrawer := room.Word // full word (private to drawer)
	timeLimit := int64(internal.DrawingPhaseDuration.Seconds())
	masked := utils.GetMaskedWord(room.Word)

	room.Mu.Unlock()
	log.Printf("[StartDrawingPhase] room=%s: released lock after setup", roomID)
	// --- End critical section ---

	log.Printf("[StartDrawingPhase] room=%s: starting drawing phase. drawer=%s, word_mask=%s",
		roomID, drawer.Id, masked)

	// 5. Start the phase timer - on expiry, decide next flow.
	StartPhaseTimer(room, internal.DrawingPhaseDuration, func() {
		// Timer callback: check whether everyone guessed; perform transition in its own goroutine.
		go func() {
			log.Printf("[StartDrawingPhase.Timer] room=%s: timer callback triggered", roomID)
			room.Mu.RLock()
			allGuessed := room.HasEveryoneGuessed()
			room.Mu.RUnlock()

			if allGuessed {
				log.Printf("[StartDrawingPhase.Timer] room=%s: everyone guessed before expiry -> StartRevealingPhase", roomID)
				StartRevealingPhase(room)
			} else {
				log.Printf("[StartDrawingPhase.Timer] room=%s: time expired -> NextRound", roomID)
				NextRound(room)
			}
		}()
	})
	log.Printf("[StartDrawingPhase] room=%s: phase timer started (%ds)", roomID, timeLimit)

	// 6. Broadcast masked word to all players except the drawer
	maskedWord := internal.MaskedWordData{
		RoomID:     roomID,
		MaskedWord: masked,
	}
	maskedWordMessage := internal.Message[any]{
		Type: "drawing_phase",
		Data: maskedWord,
	}

	go func() {
		log.Printf("[StartDrawingPhase] room=%s: broadcasting masked word to all except drawer=%s",
			roomID, drawer.Id)
		SafeBroadcastToRoomExcept(room, maskedWordMessage, drawer)
	}()

	// 7. Send full drawer data (private) to the drawer using safe per-connection writer
	drawerData := internal.Message[any]{
		Type: "drawing_phase",
		Data: map[string]any{
			"room_id":        roomID,
			"current_word":   wordForDrawer,
			"current_drawer": map[string]string{"id": drawer.Id, "username": drawer.Username},
			"phase":          internal.PhaseDrawing,
			"time_remaining": timeLimit,
		},
	}

	log.Printf("[StartDrawingPhase] room=%s: sending private drawer data to %s (%s)",
		roomID, drawer.Id, drawer.Username)

	if err := drawer.SafeWriteJSON(drawerData); err != nil {
		// If sending fails, log and continue; drawer may have disconnected — remove player will handle it.
		log.Printf("[StartDrawingPhase] room=%s: failed to send drawer data to %s (%s): %v",
			roomID, drawer.Id, drawer.Username, err)
	} else {
		log.Printf("[StartDrawingPhase] room=%s: successfully sent drawer data to %s (%s)",
			roomID, drawer.Id, drawer.Username)
	}
}

// StartRevealingPhase shows word and round results (8 seconds)
func StartRevealingPhase(room *internal.Room) {
	// 1) Acquire lock and update state + compute round stat snapshot
	// Basic validations
	if room == nil {
		log.Println("[StartRevealingPhase] nil room, abort")
		return
	}
	room.Mu.Lock()

	// set phase
	room.Phase = internal.PhaseRevealing

	// cancel active drawing timer (use CancelPhaseTimer helper if available)
	// using CancelPhaseTimer keeps a single place for timer cleanup semantics
	CancelPhaseTimer(room)

	// ensure nobody can draw
	for _, p := range room.Players {
		if p != nil {
			p.CanDraw = false
		}
	}

	// create round stats entry (populate fields that we know exist)
	rs := internal.RoundStats{
		RoundNumber:     room.RoundNumber,
		DrawerId:        "",
		Word:            room.Word,
		CorrectGuessers: room.CorrectGuessers,
		TotalGuesses:    len(room.CorrectGuessers),
		StartTime:       time.Time{},
		EndTime:         time.Now(),
	}
	if room.Current != nil {
		rs.DrawerId = room.Current.Id
	}
	if room.Timer != nil {
		rs.StartTime = room.Timer.StartTime
	}

	// append to round stats
	room.RoundStats = append(room.RoundStats, rs)

	// compute next drawer index and next player snapshot (safe while holding lock)
	var nextPlayerPublic *internal.Player = nil
	var nextIndex int = -1
	if len(room.PlayerOrder) > 0 {
		// next index = (currentIndex+1) % len
		nextIndex = (room.CurrentIndex + 1) % len(room.PlayerOrder)
		nextID := room.PlayerOrder[nextIndex]
		if p := room.Players[nextID]; p != nil {
			// use ToPublicPlayer to avoid sending Conn/Room in messages
			nextPlayerPublic = p.ToPublicPlayer()
		}
	}

	// build finalScores as public snapshots
	finalScores := make([]*internal.Player, 0, len(room.Players))
	for _, p := range room.Players {
		if p != nil {
			finalScores = append(finalScores, p.ToPublicPlayer())
		}
	}

	// Determine simple end-of-game condition (conservative):
	// here we treat the game as ended if room.RoundNumber >= room.MaxRounds.
	// NOTE: if you want different rules (e.g. end when we've completed all player turns
	// in the last round), adjust this logic below.
	isGameEndedNow := room.RoundNumber >= room.MaxRounds

	// Snapshot some fields for broadcasting after unlock
	roundNum := room.RoundNumber
	drawerID := ""
	drawerName := ""
	if room.Current != nil {
		drawerID = room.Current.Id
		drawerName = room.Current.Username
	}
	word := room.Word
	roomID := room.Id

	room.Mu.Unlock() // release lock before doing any I/O or long work

	// 2) Build and broadcast round_end message (no locks held)
	roundEndData := internal.RoundEndData{
		Word:            word,
		DrawerID:        drawerID,
		CorrectGuessers: rs.CorrectGuessers,
		DrawerUsername:  drawerName,
		NextDrawer:      nextPlayerPublic,
		FinalScores:     finalScores,
		IsGameEnded:     isGameEndedNow,
	}
	roundEndMessage := internal.Message[any]{
		Type: "round_end",
		Data: roundEndData,
	}

	log.Printf("[StartRevealingPhase] room=%s round=%d drawer=%s word=%q correct=%d endNow=%v",
		roomID, roundNum, drawerID, word, len(rs.CorrectGuessers), isGameEndedNow)

	// broadcast (SafeBroadcastToRoom snapshots connections internally)
	SafeBroadcastToRoom(room, roundEndMessage)

	// 3) Start reveal timer: after 8s either EndGame or NextRound
	onRevealComplete := func() {
		// Re-check end condition under lock at expiry time (more accurate than earlier snapshot)
		room.Mu.Lock()
		shouldEnd := room.RoundNumber > room.MaxRounds ||
			(room.RoundNumber == room.MaxRounds && room.CurrentIndex == len(room.PlayerOrder)-1)

		room.Mu.Unlock()

		if shouldEnd {
			log.Printf("[StartRevealingPhase.timer] room=%s: ending game after reveal", roomID)
			EndGame(room)
		} else {
			log.Printf("[StartRevealingPhase.timer] room=%s: proceeding to NextRound", roomID)
			NextRound(room)
		}
	}

	StartPhaseTimer(room, 8*time.Second, onRevealComplete)
}

// NextRound advances to next player or ends game
func NextRound(room *internal.Room) {
	if room == nil {
		log.Println("[NextRound] nil room, abort")
		return
	}

	log.Printf("[NextRound] room=%s: acquired lock, advancing round", room.Id)
	// Update order safely
	utils.UpdatePlayerOrder(room)
	room.Mu.Lock()
	log.Printf("[NextRound] room=%s: updated player order=%v", room.Id, room.PlayerOrder)

	// No players left → end game
	if len(room.PlayerOrder) == 0 {
		room.Mu.Unlock()
		log.Printf("[NextRound] room=%s: no players left, ending game", room.Id)
		go EndGame(room) // async, don’t block
		return
	}

	// Advance index with wraparound
	prevIndex := room.CurrentIndex
	room.CurrentIndex = (room.CurrentIndex + 1) % len(room.PlayerOrder)
	room.Word = ""
	wrapped := room.CurrentIndex == 0
	log.Printf("[NextRound] room=%s: advanced index prev=%d new=%d wrapped=%v",
		room.Id, prevIndex, room.CurrentIndex, wrapped)

	if wrapped {
		room.RoundNumber++
		log.Printf("[NextRound] room=%s: round incremented to %d", room.Id, room.RoundNumber)

		if room.RoundNumber > room.MaxRounds {
			rn := room.RoundNumber
			room.Mu.Unlock()
			log.Printf("[NextRound] room=%s: round %d > maxRounds %d → ending game",
				room.Id, rn, room.MaxRounds)
			go EndGame(room) // async
			return
		}
	}

	// Assign new drawer
	nextPlayerID := room.PlayerOrder[room.CurrentIndex]
	room.Current = room.Players[nextPlayerID]
	log.Printf("[NextRound] room=%s: assigned new drawer id=%s", room.Id, nextPlayerID)

	// Validate state
	room.Mu.Unlock()
	if !utils.ValidateGameState(room) {
		log.Printf("[NextRound] room=%s: invalid game state (order=%v index=%d)",
			room.Id, room.PlayerOrder, room.CurrentIndex)
	}
	room.Mu.Lock()
	// Snapshot for logs after unlock
	nextDrawerID := nextPlayerID
	nextDrawerName := ""
	if room.Current != nil {
		nextDrawerName = room.Current.Username
	}
	roundNum := room.RoundNumber

	room.Mu.Unlock()
	log.Printf("[NextRound] room=%s: released lock", room.Id)

	// Start waiting phase outside lock
	log.Printf("[NextRound] room=%s: → next drawer %s (%s), round=%d (prevIndex=%d newIndex=%d)",
		room.Id, nextDrawerID, nextDrawerName, roundNum, prevIndex, room.CurrentIndex)

	go StartWaitingPhase(room) // async
	log.Printf("[NextRound] room=%s: started waiting phase goroutine", room.Id)
}

// EndGame finishes game and shows final results
func EndGame(room *internal.Room) {
	if room == nil {
		log.Println("[EndGame] nil room, abort")
		return
	}

	room.Mu.Lock()

	// Set ended phase & cleanup timers
	room.Phase = internal.PhaseEnded
	CancelPhaseTimer(room)

	// Snapshot room ID for logging
	roomID := room.Id

	room.Mu.Unlock()

	// Compute results outside lock
	resultData := CalculateFinalResults(room)

	// Broadcast final leaderboard
	resultMessage := internal.Message[any]{
		Type: "game_ended",
		Data: resultData,
	}
	log.Printf("[EndGame] room=%s: broadcasting final results", roomID)
	SafeBroadcastToRoom(room, resultMessage)

	// Start 30s timer to reset to lobby (async)
	StartPhaseTimer(room, 30*time.Second, func() {
		log.Printf("[EndGame.timer] room=%s: returning to lobby", roomID)
		go ResetRoomToLobby(room)
	})
}
