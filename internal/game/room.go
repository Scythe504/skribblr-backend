package game

import (
	"context"
	"fmt"
	"log"
	"slices"
	"sync"

	"github.com/scythe504/skribblr-backend/internal"
	"github.com/scythe504/skribblr-backend/internal/utils"
)

// =============================================================================
// ROOM MANAGEMENT
// =============================================================================

// GetJoinableRoom returns ID of a room that can accept new players
func GetJoinableRoom() string {
	// TODO:
	// 1. Lock rooms for reading
	RoomsMu.RLock()
	defer RoomsMu.RUnlock()

	// 2. Iterate through existing rooms
	for _, room := range Rooms {
		room.Mu.RLock()

		// 3. Check player count < MaxPlayersPerRoom
		if len(room.Players) >= MaxPlayersPerRoom {
			// MUST unlock before continue, or deadlock happens
			room.Mu.RUnlock()
			continue
		}

		// 4. Check room is in lobby phase
		if room.Phase == internal.PhaseLobby {
			roomID := room.Id
			room.Mu.RUnlock()
			log.Printf("[GetJoinableRoom] Found joinable room %s with %d players", roomID, len(room.Players))
			// 5. Return room ID
			return roomID
		}

		// Unlock if not usable
		room.Mu.RUnlock()
	}

	// No joinable room found
	log.Println("[GetJoinableRoom] No joinable room found")
	return ""
}

// getOrCreateRoom retrieves existing room or creates new one
func getOrCreateRoom(roomId string) *internal.Room {
	// TODO:
	// 1. Lock rooms map for writing
	RoomsMu.Lock()
	defer RoomsMu.Unlock()

	// 2. Check if room exists
	if room, exists := Rooms[roomId]; exists {
		log.Printf("[getOrCreateRoom] Found existing room %s (players: %d, phase: %s)",
			roomId, len(room.Players), room.Phase)
		return room
	}

	// 3. If not exists, create new room
	ctx, cancel := context.WithCancel(context.Background())
	newRoom := &internal.Room{
		Id:              roomId,
		Players:         make(map[string]*internal.Player),
		PlayersReady:    make(map[string]bool),
		CorrectGuessers: make([]internal.PlayerGuess, 0),
		PlayerOrder:     make([]string, 0),
		WordChoices:     make([]string, 0),
		Timer:           &internal.GameTimer{IsActive: false},
		Current:         nil,

		RoundStats:  make([]internal.RoundStats, 0),
		CanvasState: make([]internal.PixelMessage, 0),
		Phase:       internal.PhaseLobby,

		Context: ctx,
		Cancel:  cancel,

		CurrentIndex:   0,
		Word:           "",
		RoundNumber:    1,
		MaxRounds:      3,
		HasGameStarted: false,

		Mu: sync.RWMutex{},
	}

	Rooms[roomId] = newRoom

	log.Printf("[getOrCreateRoom] Created new room %s with default settings (maxRounds=%d, phase=%s)",
		roomId, newRoom.MaxRounds, newRoom.Phase)

	// 4. Return room pointer
	return newRoom
}

// AddPlayer joins a player to a room and sends initial messages
func AddPlayer(roomId string, player *internal.Player) error {
	// TODO:
	// 1. Get or create room
	room := getOrCreateRoom(roomId)

	// 2. Lock room for modifications
	room.Mu.Lock()

	// 3. Set player.Room reference
	player.Room = room

	// 4. Add player to room.Players map
	room.Players[player.Id] = player

	// 5. Set player initial state
	player.IsConnected = true
	player.IsReady = false

	// 6. Prepare welcome message
	welcomeMsg := internal.Message[any]{
		Type: "player_joined",
		Data: map[string]any{
			"message":     fmt.Sprintf("Welcome %s, %s has joined.", player.Username, player.Username),
			"player_data": player.ToPublicPlayer(), //  safer for broadcast
		},
	}

	log.Printf("[AddPlayer] Added player %s (%s) to room %s. Total players: %d",
		player.Id, player.Username, room.Id, len(room.Players))

	// Unlock before broadcasting
	room.Mu.Unlock()

	// 7. Broadcast player_joined to other players
	SafeBroadcastToRoomExcept(room, welcomeMsg, player)

	// 8. Send current game state to new player
	room.Mu.RLock()
	players := make([]*internal.Player, 0, len(room.Players))
	for _, p := range room.Players {
		players = append(players, p.ToPublicPlayer())
	}

	missingStateData := internal.Message[any]{
		Type: "welcome_msg",
		Data: map[string]any{
			"game_state": internal.GameStateData{
				Phase:           room.Phase,
				RoundNumber:     room.RoundNumber,
				MaxRounds:       room.MaxRounds,
				CurrentDrawer:   room.Current,
				TimeRemaining:   int64(room.Timer.TimeRemaining),
				Word:            utils.GetMaskedWord(room.Word),
				CorrectGuessers: room.CorrectGuessers,
				Players:         players,
			},
			"canvas_state": room.CanvasState,
		},
	}
	room.Mu.RUnlock()

	// Write directly to the joining player (not broadcasted)
	if err := player.SafeWriteJSON(missingStateData); err != nil {
		log.Printf("[AddPlayer] Failed to send state to player %s (%s): %v",
			player.Id, player.Username, err)
		return err
	}

	// 9. Enforce max players rule (do AFTER sending state, so player sees reason)
	room.Mu.RLock()
	if len(room.Players) > MaxPlayersPerRoom {
		room.Mu.RUnlock()
		log.Printf("[AddPlayer] Room %s is full, rejecting player %s (%s)",
			room.Id, player.Id, player.Username)
		return fmt.Errorf("max players reached for this room, please join another room")
	}
	room.Mu.RUnlock()

	log.Printf("[AddPlayer] Successfully initialized player %s (%s) in room %s",
		player.Id, player.Username, room.Id)
	return nil
}

// removePlayer handles player disconnection and cleanup
func removePlayer(player *internal.Player) {
	// TODO:
	// 1. Get player's room
	room := player.Room
	if room == nil {
		log.Printf("[removePlayer] Player %s (%s) has no room reference, skipping",
			player.Id, player.Username)
		return
	}

	// 2. Lock room and modify shared state
	room.Mu.Lock()

	// Snapshot needed values before unlock
	wasCurrentDrawer := (room.Current == player)
	playerCountBefore := len(room.Players)

	// Remove from room data structures
	delete(room.Players, player.Id)
	delete(room.PlayersReady, player.Id)
	room.PlayerOrder = slices.DeleteFunc(room.PlayerOrder, func(s string) bool {
		return s == player.Id
	})

	// Calculate new player count after removal
	playerCountAfter := len(room.Players)

	log.Printf("[removePlayer] Removing player %s (%s) from room %s. Players before=%d after=%d",
		player.Id, player.Username, room.Id, playerCountBefore, playerCountAfter)

	room.Mu.Unlock()

	// 3. Handle drawer leaving mid-round
	if wasCurrentDrawer && room.Phase == internal.PhaseDrawing {
		log.Printf("[removePlayer] Player %s was the current drawer in room %s",
			player.Username, room.Id)
		CancelPhaseTimer(room)

		if playerCountAfter >= MinPlayersToStart {
			NextRound(room) // already acquires locks internally
		} else {
			ResetRoomToLobby(room)
		}
	} else if playerCountAfter < MinPlayersToStart && room.HasGameStarted {
		log.Printf("[removePlayer] Too few players to continue in room %s, resetting to lobby",
			room.Id)
		ResetRoomToLobby(room)
	}

	// 4. Cleanup room if empty
	if playerCountAfter == 0 {
		log.Printf("[removePlayer] Room %s is empty, cleaning up", room.Id)
		CleanupRoom(room)

		RoomsMu.Lock()
		delete(Rooms, room.Id)
		RoomsMu.Unlock()
		return
	}

	// 5. Broadcast player_left message (SNAPSHOT first, then async)
	leaveMessage := internal.Message[any]{
		Type: "player_left",
		Data: map[string]any{
			"message":           fmt.Sprintf("%s has left the game", player.Username),
			"player_id":         player.Id,
			"username":          player.Username,
			"players_remaining": playerCountAfter,
		},
	}

	// Safe: we are broadcasting with a snapshot, no lock required here
	SafeBroadcastToRoom(room, leaveMessage)

	// 6. Update game state for remaining players
	BroadcastGameState(room)
}

// CleanupRoom handles complete room shutdown
func CleanupRoom(room *internal.Room) {
	log.Printf("[CleanupRoom] Cleaning up room %s", room.Id)

	// 1. Cancel room context (stops timers, round goroutines, etc.)
	room.Mu.Lock()
	if room.Cancel != nil {
		log.Printf("[CleanupRoom] Cancelling context for room %s", room.Id)
		room.Cancel()
		room.Cancel = nil
	}

	// 2. Close all player connections
	for _, player := range room.Players {
		if player.Conn != nil {
			if err := player.Conn.Close(); err != nil {
				log.Printf("[CleanupRoom] Error closing connection for player %s (%s): %v",
					player.Id, player.Username, err)
			} else {
				log.Printf("[CleanupRoom] Closed connection for player %s (%s)",
					player.Id, player.Username)
			}
		}
	}
	room.Mu.Unlock()

	// 3. Remove room from global rooms map
	RoomsMu.Lock()
	if _, exists := Rooms[room.Id]; exists {
		delete(Rooms, room.Id)
		log.Printf("[CleanupRoom] Room %s removed from global rooms map", room.Id)
	}
	RoomsMu.Unlock()

	// 4. Clear room data structures (not strictly necessary, but helps GC and safety)
	room.Mu.Lock()
	room.Players = nil
	room.PlayersReady = nil
	room.PlayerOrder = nil
	room.CorrectGuessers = nil
	room.WordChoices = nil
	room.RoundStats = nil
	room.CanvasState = nil
	room.Current = nil
	room.Timer = nil
	room.Mu.Unlock()

	log.Printf("[CleanupRoom] Room %s cleanup completed", room.Id)
}
