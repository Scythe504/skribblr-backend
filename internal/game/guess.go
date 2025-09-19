package game

import (
	"log"
	"strings"
	"time"

	"github.com/scythe504/skribblr-backend/internal"
)

// =============================================================================
// GUESS HANDLING
// =============================================================================

// HandleGuessEnhanced processes player guesses with enhanced scoring
func HandleGuessEnhanced(player *internal.Player, guess string) {
	// Defensive nil checks
	if player == nil {
		log.Println("[HandleGuessEnhanced] nil player, abort")
		return
	}
	room := player.Room
	if room == nil {
		log.Printf("[HandleGuessEnhanced] player=%s has no room, abort", player.Id)
		return
	}

	// Normalize incoming guess
	cleanedGuess := strings.ToLower(strings.TrimSpace(guess))

	room.Mu.Lock()
	// Basic validations under lock
	if room.Current != nil && player.Id == room.Current.Id {
		// Drawer cannot guess
		room.Mu.Unlock()
		log.Printf("[HandleGuessEnhanced] room=%s player=%s is drawer, ignoring guess", room.Id, player.Id)
		return
	}
	if player.HasGuessed {
		// Already guessed correctly
		room.Mu.Unlock()
		log.Printf("[HandleGuessEnhanced] room=%s player=%s already guessed, ignoring", room.Id, player.Id)
		return
	}

	// Normalize target word for comparison (room.Word may have original casing)
	target := strings.ToLower(strings.TrimSpace(room.Word))

	// Incorrect guess path
	if target == "" || target != cleanedGuess {
		// Update stats under lock
		player.TotalGuesses++

		// Build player guess snapshot to broadcast (use milliseconds)
		nowMs := int(time.Now().UnixMilli())
		playerGuess := internal.PlayerGuess{
			PlayerID:  player.Id,
			Username:  player.Username,
			GuessTime: nowMs,
			IsCorrect: false,
		}

		// Snapshot roomID for logs / broadcast and unlock before I/O
		roomID := room.Id
		room.Mu.Unlock()

		log.Printf("[HandleGuessEnhanced] room=%s player=%s guessed incorrect: %q", roomID, player.Id, guess)

		guessMessage := internal.Message[any]{
			Type: "guess_message",
			Data: map[string]any{
				"player_guess": playerGuess,
				"guessed_word": guess,
			},
		}

		// Broadcast asynchronously so we don't block the websocket reader
		go SafeBroadcastToRoom(room, guessMessage)
		return
	}

	// Correct guess path (we are still holding the lock)
	// Ensure timer start exists (defensive)
	var startTime time.Time
	if room.Timer != nil {
		startTime = room.Timer.StartTime
	}
	timeTaken := time.Since(startTime)
	timeTakenMs := timeTaken.Milliseconds()

	// position is 1-based ordering of correct guessers
	position := len(room.CorrectGuessers) + 1

	// Determine difficulty (same logic you had)
	var diff internal.WordDifficulty
	switch {
	case len(room.Word) >= 3 && len(room.Word) <= 5:
		diff = internal.DifficultyEasy
	case len(room.Word) > 5 && len(room.Word) <= 8:
		diff = internal.DifficultyMedium
	default:
		// treat very short or empty as easy, long words as hard
		if len(room.Word) > 8 {
			diff = internal.DifficultyHard
		} else {
			diff = internal.DifficultyEasy
		}
	}

	// Calculate points (assumes this function exists and takes duration, position, difficulty)
	points := CalculateGuessPoints(timeTaken, position, diff)

	// Build player guess entry (use milliseconds consistently)
	playerGuess := internal.PlayerGuess{
		PlayerID:  player.Id,
		Username:  player.Username,
		GuessTime: int(timeTakenMs),
		IsCorrect: true,
	}

	// Apply state updates under lock
	room.CorrectGuessers = append(room.CorrectGuessers, playerGuess)

	player.Score += points
	player.TotalGuesses++
	player.CorrectGuesses++
	player.HasGuessed = true

	// Award drawer points (rule you used)
	if room.Current != nil {
		room.Current.Score += 50
	}

	// Snapshot data for broadcasting and next-step decision
	resultData := internal.GameResultData{
		PlayerID:    player.Id,
		Username:    player.Username,
		IsCorrect:   true,
		Score:       points,
		Position:    position,
		TimeToGuess: timeTakenMs,
	}
	roomID := room.Id

	// Determine whether everyone else (non-drawer, connected players) has guessed
	allGuessed := true
	for _, p := range room.Players {
		// skip nils and the drawer
		if p == nil {
			continue
		}
		if room.Current != nil && p.Id == room.Current.Id {
			continue
		}
		// only consider connected players
		if p.IsConnected && !p.HasGuessed {
			allGuessed = false
			break
		}
	}

	room.Mu.Unlock() // release lock before any I/O

	// Broadcast the guess result (async)
	resultMessage := internal.Message[any]{
		Type: "guess_result",
		Data: resultData,
	}
	log.Printf("[HandleGuessEnhanced] room=%s player=%s guessed CORRECT (pos=%d points=%d timeMs=%d)",
		roomID, player.Id, position, points, timeTakenMs)

	go SafeBroadcastToRoom(room, resultMessage)

	// If everyone guessed, cancel timer and advance round
	if allGuessed {
		log.Printf("[HandleGuessEnhanced] room=%s: all players guessed -> ending round early", roomID)
		CancelPhaseTimer(room)
		// run NextRound asynchronously to avoid blocking caller
		NextRound(room)
	}
}

// CalculateGuessPoints determines points based on speed, position, and difficulty
func CalculateGuessPoints(timeTaken time.Duration, position int, wordDifficulty internal.WordDifficulty) int {
	// TODO:
	// 1. Set base points by difficulty:
	t := timeTaken.Seconds()
	p := position
	var basePoints int
	finalPoints := 0
	switch wordDifficulty {
	//    - Easy: 100 points
	case internal.DifficultyEasy:
		basePoints = 100
		//    - Medium: 150 points
	case internal.DifficultyMedium:
		basePoints = 150
		//    - Hard: 200 points
	case internal.DifficultyHard:
		basePoints = 200
	}

	// 2. Apply speed bonus (faster = more points):
	var speedMultiplier float32
	switch {
	case t >= 0 && t < 10:
		//    - < 10s: 150% multiplier
		speedMultiplier = 1.5
	case t >= 10 && t < 30:
		//    - < 30s: 125% multiplier
		speedMultiplier = 1.25
	case t >= 30 && t < 60:
		//    - < 60s: 100% multiplier
		speedMultiplier = 1
	case t > 60:
		//    - > 60s: 75% multiplier
		speedMultiplier = 0.75
	}

	// 3. Apply position penalty:
	var posMultiplier float32
	switch p {
	case 1:
		//    - 1st: 100% of calculated points
		posMultiplier = 1
	case 2:
		//    - 2nd: 80% of calculated points
		posMultiplier = 0.8
	case 3:
		//    - 3rd: 60% of calculated points
		posMultiplier = 0.6
	default:
		//    - 4th+: 40% of calculated points
		posMultiplier = 0.4
	}
	// 4. Return final calculated points
	finalPoints = int(float32(basePoints) * speedMultiplier * posMultiplier)
	return finalPoints
}
