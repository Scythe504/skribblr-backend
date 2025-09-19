package game

import (
	"fmt"
	"log"
	"time"

	"github.com/scythe504/skribblr-backend/internal"
)

// =============================================================================
// GAME FLOW - LOBBY & INITIALIZATION (safe version)
// =============================================================================

// HandlePlayerReady toggles player ready state in lobby.
// Locks only at this level, never inside helpers.
func HandlePlayerReady(player *internal.Player, ready bool) {
	room := player.Room

	// --- Critical section ---
	room.Mu.Lock()

	if room.Phase != internal.PhaseLobby {
		log.Printf("[HandlePlayerReady] Room %s not in lobby phase (phase=%v)",
			room.Id, room.Phase)
		room.Mu.Unlock()
		return
	}

	// Update state
	player.IsReady = ready
	room.PlayersReady[player.Id] = ready

	// Prepare snapshot for broadcast
	lobbyUpdate := internal.Message[any]{
		Type: "lobby_update",
		Data: map[string]any{
			"player_id":     player.Id,
			"username":      player.Username,
			"is_ready":      ready,
			"ready_count":   len(room.PlayersReady),
			"total_players": len(room.Players),
		},
	}

	allReady := room.AreAllPlayersReady()
	enoughPlayers := len(room.Players) >= MinPlayersToStart

	log.Printf("[HandlePlayerReady] Room %s: Player %s (%s) ready=%v, ReadyCount=%d/%d",
		room.Id, player.Id, player.Username, ready, len(room.PlayersReady), len(room.Players))

	room.Mu.Unlock()
	// --- End critical section ---

	// Safe broadcast (async, no lock held)
	go SafeBroadcastToRoom(room, lobbyUpdate)

	// If all players ready, try starting game
	if allReady && enoughPlayers {
		log.Printf("[HandlePlayerReady] Room %s: All players ready. Starting game...", room.Id)
		go func() {
			if err := StartGame(room); err != nil {
				log.Printf("[HandlePlayerReady] Failed to start game in room %s: %v", room.Id, err)
			}
		}()
	}
}

// StartGame initializes a new game when conditions are met.
func StartGame(room *internal.Room) error {
	// --- Critical section ---
	room.Mu.Lock()

	if len(room.Players) < MinPlayersToStart {
		log.Printf("[StartGame] Room %s: Not enough players (%d/%d)",
			room.Id, len(room.Players), MinPlayersToStart)
		room.Mu.Unlock()
		return fmt.Errorf("not enough players to start game: %d/%d",
			len(room.Players), MinPlayersToStart)
	}
	if !room.AreAllPlayersReady() {
		log.Printf("[StartGame] Room %s: Not all players ready", room.Id)
		room.Mu.Unlock()
		return fmt.Errorf("not all players are ready in room %s", room.Id)
	}

	// Initialize state
	room.HasGameStarted = true
	room.RoundNumber = 1
	room.CurrentIndex = 0
	room.RoundStats = make([]internal.RoundStats, 0)
	room.ResetPlayerGuessState()

	// Build PlayerOrder
	room.PlayerOrder = make([]string, 0, len(room.Players))
	for playerId, isReady := range room.PlayersReady {
		if player := room.Players[playerId]; player != nil && player.IsConnected && isReady {
			room.PlayerOrder = append(room.PlayerOrder, playerId)
		}
	}

	// Snapshot
	playerOrderCopy := append([]string(nil), room.PlayerOrder...)
	playersSnapshot := make([]map[string]any, 0, len(room.Players))
	for _, p := range room.Players {
		playersSnapshot = append(playersSnapshot, map[string]any{
			"id":       p.Id,
			"username": p.Username,
			"score":    p.Score,
		})
	}

	gameStartedMsg := internal.Message[any]{
		Type: "game_started",
		Data: map[string]any{
			"message":       "Game has started!",
			"room_id":       room.Id,
			"players_count": len(playerOrderCopy),
			"players":       playersSnapshot,
		},
	}

	log.Printf("[StartGame] Room %s: Initialized game. Round=%d, PlayerOrder=%v",
		room.Id, room.RoundNumber, playerOrderCopy)

	room.Mu.Unlock()
	// --- End critical section ---

	// External actions
	log.Printf("[StartGame] Room %s: Entering waiting phase...", room.Id)
	StartWaitingPhase(room)

	log.Printf("[StartGame] Room %s: Broadcasting game_started to %d players",
		room.Id, len(playerOrderCopy))
	SafeBroadcastToRoom(room, gameStartedMsg)

	return nil
}

// ResetRoomToLobby returns room to waiting-for-players state
func ResetRoomToLobby(room *internal.Room) {
	// TODO:
	room.Mu.Lock()
	// 1. Cancel all active timers
	CancelPhaseTimer(room)
	// 2. Set Phase = PhaseLobby
	room.Phase = internal.PhaseLobby
	// 3. Set HasGameStarted = false
	room.HasGameStarted = false
	// 4. Reset all game state variables
	room.CorrectGuessers = make([]internal.PlayerGuess, 0)
	room.Word = ""
	room.RoundNumber = 1
	room.WordChoices = make([]string, 0, 3)
	room.Current = nil
	room.CurrentIndex = 0
	room.PlayerOrder = make([]string, 0)
	// 5. Set all players IsReady = false
	room.PlayersReady = make(map[string]bool)
	for playerId := range room.Players {
		room.Players[playerId].IsReady = false
		room.Players[playerId].ResetRoundState()
	}
	// 6. Clear scores, round stats, canvas state
	room.CanvasState = make([]internal.PixelMessage, 0)
	room.RoundStats = make([]internal.RoundStats, 0)
	for playerID := range room.Players {
		room.Players[playerID].Score = 0
	}
	// 7. Broadcast lobby_reset message
	lobbyResetMessage := internal.Message[any]{
		Type: "lobby_reset",
		Data: map[string]any{
			"message":          fmt.Sprintf("Lobby %s has been reset for new game", room.Id),
			"room_id":          room.Id,
			"timestamp":        time.Now().UnixMilli(),
			"players":          room.Players,
			"phase":            room.Phase,
			"current_drawer":   room.Current,
			"round_number":     room.RoundNumber,
			"max_rounds":       room.MaxRounds,
			"correct_guessers": room.CorrectGuessers,
		},
	}
	room.Mu.Unlock()
	SafeBroadcastToRoom(room, lobbyResetMessage)
}
