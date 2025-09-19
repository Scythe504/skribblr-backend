package game

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/gorilla/websocket"
	"github.com/scythe504/skribblr-backend/internal"
	"github.com/scythe504/skribblr-backend/internal/utils"
)

// =============================================================================
// DRAWING SYSTEM
// =============================================================================

// HandlePixelDrawEnhanced processes drawing with permission verification
func HandlePixelDrawEnhanced(player *internal.Player, rawData json.RawMessage) {
	log.Printf("[HandlePixelDrawEnhanced] Processing draw request from player %s (%s)",
		player.Id, player.Username)

	// TODO: 0. Get room reference
	room := player.Room
	if room == nil {
		log.Printf("[HandlePixelDrawEnhanced] Player %s has no room reference", player.Username)
		return
	}

	// TODO: 1. Lock room mutex for concurrency safety
	room.Mu.Lock()
	defer room.Mu.Unlock() // Unlock at the end

	// TODO: 2. Verify game is in drawing phase
	if room.Phase != internal.PhaseDrawing {
		log.Printf("[HandlePixelDrawEnhanced] Room %s not in drawing phase (current: %s), ignoring draw request",
			room.Id, room.Phase)
		return
	}

	// TODO: 3. Verify player is the current drawer
	if room.Current != player {
		log.Printf("[HandlePixelDrawEnhanced] Player %s is not the current drawer in room %s",
			player.Username, room.Id)
		return
	}

	// TODO: 4. Verify player.CanDraw is true
	if !player.CanDraw {
		log.Printf("[HandlePixelDrawEnhanced] Player %s does not have draw permission in room %s",
			player.Username, room.Id)
		return
	}

	// TODO: 5. Parse rawData into PixelMessage struct
	var pixelMessage internal.PixelMessage
	if err := json.Unmarshal(rawData, &pixelMessage); err != nil {
		// - Handle errors gracefully
		log.Printf("[HandlePixelDrawEnhanced] Malformed pixelMessage json obj from player %s: %v",
			player.Username, err)
		// - If malformed JSON, return early
		return
	}

	// TODO: 6. Validate pixel data
	switch pixelMessage.Type {
	case internal.PixelPlace, internal.ErasePixel:
		if pixelMessage.X == nil || pixelMessage.Y == nil {
			log.Printf("[HandlePixelDrawEnhanced] Missing X/Y coordinates for single pixel operation from player %s",
				player.Username)
			return
		}

		// - Check bounds against server canonical canvas
		if *pixelMessage.X < 0 || *pixelMessage.X >= internal.CanvasWidth ||
			*pixelMessage.Y < 0 || *pixelMessage.Y >= internal.CanvasHeight {
			log.Printf("[HandlePixelDrawEnhanced] Pixel out of bounds from player %s: (%d,%d)",
				player.Username, *pixelMessage.X, *pixelMessage.Y)
			// - Pixel out of bounds, discard
			return
		}
	case internal.BatchPlace, internal.BatchErase:
		validPixels := []internal.GridPosition{}
		invalidCount := 0
		for _, p := range pixelMessage.Pixels {
			if p.GridX < 0 || p.GridX >= internal.CanvasWidth ||
				p.GridY < 0 || p.GridY >= internal.CanvasHeight {
				invalidCount++
				continue
			}
			validPixels = append(validPixels, p)
		}

		if invalidCount > 0 {
			log.Printf("[HandlePixelDrawEnhanced] Filtered %d invalid pixels from batch operation by player %s",
				invalidCount, player.Username)
		}

		pixelMessage.Pixels = validPixels
		if len(pixelMessage.Pixels) == 0 {
			log.Printf("[HandlePixelDrawEnhanced] No valid pixels in batch operation from player %s",
				player.Username)
			// Nothing to draw/erase
			return
		}
	}

	// TODO: 7. Normalize coordinates
	// - Use server canonical grid (GridWidth x GridHeight)
	// - If client sent scaled coordinates, convert to grid positions
	// - Maintain aspect ratio
	switch pixelMessage.Type {
	case internal.PixelPlace, internal.ErasePixel:
		gridX, gridY := internal.NormalizeCoordinates(*pixelMessage.X, *pixelMessage.Y, player.CanvasWidth, player.CanvasHeight)
		pixelMessage.X = &gridX
		pixelMessage.Y = &gridY
	case internal.BatchPlace, internal.BatchErase:
		for i, p := range pixelMessage.Pixels {
			gridX, gridY := internal.NormalizeCoordinates(p.GridX, p.GridY, player.CanvasWidth, player.CanvasHeight)
			pixelMessage.Pixels[i].GridX = gridX
			pixelMessage.Pixels[i].GridY = gridY
		}
	}

	if pixelMessage.Timestamp == 0 {
		pixelMessage.Timestamp = time.Now().UnixMilli()
	}

	// TODO: 8. Apply changes to room.CanvasState
	switch pixelMessage.Type {
	// - Single pixel: append/update canvas
	case internal.PixelPlace:
		room.CanvasState = append(room.CanvasState, pixelMessage)
		log.Printf("[HandlePixelDrawEnhanced] Added pixel at (%d,%d) by player %s",
			*pixelMessage.X, *pixelMessage.Y, player.Username)
	case internal.BatchPlace:
		room.CanvasState = append(room.CanvasState, pixelMessage)
		log.Printf("[HandlePixelDrawEnhanced] Added %d pixels in batch by player %s",
			len(pixelMessage.Pixels), player.Username)
		// - Batch: loop through each pixel and append/update
	case internal.ErasePixel:
		newCanvas := []internal.PixelMessage{}
		eraseCount := 0
		for _, existing := range room.CanvasState {
			if existing.Type == internal.PixelPlace &&
				existing.X != nil && existing.Y != nil &&
				*existing.X == *pixelMessage.X && *existing.Y == *pixelMessage.Y {
				eraseCount++
				continue // Skip this pixel (erase it)
			}
			newCanvas = append(newCanvas, existing)
		}
		// - Erase operations: remove pixels from canvas
		room.CanvasState = newCanvas
		log.Printf("[HandlePixelDrawEnhanced] Erased %d pixel(s) at (%d,%d) by player %s",
			eraseCount, *pixelMessage.X, *pixelMessage.Y, player.Username)
	case internal.BatchErase:
		eraseMap := map[string]struct{}{}
		for _, p := range pixelMessage.Pixels {
			key := fmt.Sprintf("%d_%d", p.GridX, p.GridY)
			eraseMap[key] = struct{}{}
		}

		newCanvas := []internal.PixelMessage{}
		eraseCount := 0
		for _, existing := range room.CanvasState {
			if existing.Type == internal.PixelPlace && existing.X != nil && existing.Y != nil {
				key := fmt.Sprintf("%d_%d", *existing.X, *existing.Y)
				if _, ok := eraseMap[key]; ok {
					eraseCount++
					continue // Skip this pixel (erase it)
				}
			}
			newCanvas = append(newCanvas, existing)
		}
		room.CanvasState = newCanvas
		log.Printf("[HandlePixelDrawEnhanced] Erased %d pixel(s) in batch by player %s",
			eraseCount, player.Username)
	}

	// TODO: 9. Broadcast pixel draw message to other players
	// - Keep type: PixelPlace, BatchPlace, ErasePixel, BatchErase
	// - Send normalized grid positions, not client pixel positions
	pixelDrawMessage := internal.Message[any]{
		Type: string(pixelMessage.Type),
		Data: pixelMessage,
	}

	// TODO: 10. Optional: throttle or rate-limit broadcasts
	// - Avoid flooding network for large batch operations

	// TODO: 12. Unlock room.Mu before broadcasting
	// - Broadcasting can be outside lock to avoid blocking other actions
	// CRITICAL FIX: Broadcast in goroutine to avoid holding lock during network I/O
	go func() {
		log.Printf("[HandlePixelDrawEnhanced] Broadcasting %s to other players in room %s",
			pixelMessage.Type, room.Id)
		SafeBroadcastToRoomExcept(room, pixelDrawMessage, room.Current)
	}()
}

// ClearCanvas resets the drawing canvas
func ClearCanvas(room *internal.Room, clearedBy *internal.Player) {
	log.Printf("[ClearCanvas] Player %s requesting canvas clear in room %s",
		clearedBy.Username, room.Id)

	// TODO:
	room.Mu.Lock()
	// 1. Verify clearedBy is current drawer (or allow anyone?)
	if room.Current != clearedBy {
		log.Printf("[ClearCanvas] Player %s is not current drawer, denying clear request in room %s",
			clearedBy.Username, room.Id)
		room.Mu.Unlock()
		return
	}

	// 2. Clear room.CanvasState slice
	pixelCount := len(room.CanvasState)
	room.CanvasState = make([]internal.PixelMessage, 0)

	// 3. Prepare canvas_cleared message (snapshot data before unlock)
	clearedCanvasMessage := internal.Message[map[string]any]{
		Type: "canvas_cleared",
		Data: map[string]any{
			"room_id":      room.Id,
			"player_id":    clearedBy.Id,
			"canvas_state": room.CanvasState, // This is now empty slice
			"timestamp":    time.Now().UnixMilli(),
		},
	}

	room.Mu.Unlock()

	log.Printf("[ClearCanvas] Cleared %d pixels from canvas in room %s by player %s",
		pixelCount, room.Id, clearedBy.Username)

	// CRITICAL FIX: Broadcast in goroutine to avoid any potential deadlock
	go func() {
		log.Printf("[ClearCanvas] Broadcasting canvas_cleared to players in room %s", room.Id)
		SafeBroadcastToRoomExcept(room, clearedCanvasMessage, clearedBy)

		// 4. Log canvas clear action
		utils.LogGameEvent(room, clearedCanvasMessage.Type, clearedCanvasMessage.Data)
	}()
}

// UpdateDrawingPermissions sets who can draw based on game state
func UpdateDrawingPermissions(room *internal.Room) {
	log.Printf("[UpdateDrawingPermissions] Updating drawing permissions for room %s", room.Id)

	// TODO:
	room.Mu.Lock()

	// 1. Set all players CanDraw = false by default
	for id := range room.Players {
		room.Players[id].CanDraw = false
	}

	var currentDrawerId string
	var currentDrawerUsername string

	// 2. If in drawing phase, set current drawer CanDraw = true
	if room.Phase == internal.PhaseDrawing && room.Current != nil {
		room.Current.CanDraw = true
		currentDrawerId = room.Current.Id
		currentDrawerUsername = room.Current.Username
		log.Printf("[UpdateDrawingPermissions] Granted draw permission to player %s in room %s",
			currentDrawerUsername, room.Id)
	}

	// 3. Prepare drawing_permissions_updated message (snapshot data before unlock)
	drawingPermissionMessage := internal.Message[map[string]any]{
		Type: "drawing_permission_updated",
		Data: map[string]any{
			"room_id":   room.Id,
			"player_id": currentDrawerId,
			"message":   fmt.Sprintf("%s is now going to draw.", currentDrawerUsername),
		},
	}

	room.Mu.Unlock()

	// CRITICAL FIX: Broadcast in goroutine to avoid any potential deadlock
	go func() {
		log.Printf("[UpdateDrawingPermissions] Broadcasting permission update to room %s", room.Id)
		SafeBroadcastToRoom(room, drawingPermissionMessage)
	}()
}

// =============================================================================
// BROADCASTING & MESSAGING
// =============================================================================

func SafeBroadcastToRoom[T any](room *internal.Room, msg internal.Message[T]) {
	// 1. Snapshot connected players under lock
	room.Mu.Lock()
	players := make([]*internal.Player, 0, len(room.Players))
	for _, player := range room.Players {
		if player.IsConnected {
			players = append(players, player)
		}
	}
	room.Mu.Unlock()

	// 2. Iterate over snapshot and send
	successCount := 0
	for _, player := range players {
		if err := player.SafeWriteJSON(msg); err != nil {
			log.Printf("[Broadcast][Room:%s] Failed for player %s (%s): %v",
				room.Id, player.Id, player.Username, err)
			continue
		}
		successCount++
		log.Printf("[Broadcast][Room:%s] Sent to player %s (%s)",
			room.Id, player.Id, player.Username)
	}
	log.Printf("[Broadcast][Room:%s] Successfully sent to %d/%d players",
		room.Id, successCount, len(players))
}

func SafeBroadcastToRoomExcept[T any](room *internal.Room, msg internal.Message[T], exclude *internal.Player) {
	// 1. Snapshot connected players under lock
	room.Mu.Lock()
	players := make([]*internal.Player, 0, len(room.Players))
	for _, player := range room.Players {
		if player.IsConnected {
			players = append(players, player)
		}
	}
	room.Mu.Unlock()

	// 2. Iterate over snapshot and send, skipping excluded
	successCount := 0
	excludedCount := 0
	for _, player := range players {
		if exclude != nil && player.Id == exclude.Id {
			excludedCount++
			continue
		}
		if err := player.SafeWriteJSON(msg); err != nil {
			log.Printf("[BroadcastExcept][Room:%s] Failed for player %s (%s): %v",
				room.Id, player.Id, player.Username, err)
			continue
		}
		successCount++
		log.Printf("[BroadcastExcept][Room:%s] Sent to player %s (%s)",
			room.Id, player.Id, player.Username)
	}
	log.Printf("[BroadcastExcept][Room:%s] Successfully sent to %d players (excluded %d)",
		room.Id, successCount, excludedCount)
}

// BroadcastGameState sends complete game state to all players
func BroadcastGameState(room *internal.Room) {
	log.Printf("[BroadcastGameState] Broadcasting game state for room %s", room.Id)

	// Validate first to avoid sending broken state
	if !utils.ValidateGameState(room) {
		log.Printf("[BroadcastGameState] Invalid game state in room %s, skipping broadcast", room.Id)
		return
	}

	room.Mu.RLock()
	// 1. Create GameStateData struct with:
	baseState := internal.GameStateData{}
	//    - Current phase
	baseState.Phase = room.Phase
	//    - Round information
	baseState.RoundNumber = room.RoundNumber
	baseState.MaxRounds = room.MaxRounds
	//    - Player list (use ToPublicPlayer() to avoid sensitive data)
	for _, p := range room.Players {
		baseState.Players = append(baseState.Players, p.ToPublicPlayer())
	}
	//    - Current drawer info
	if room.Current != nil {
		baseState.CurrentDrawer = room.Current.ToPublicPlayer()
	}
	//    - Correct guessers
	baseState.CorrectGuessers = room.CorrectGuessers

	// CRITICAL FIX: Move timer access inside the lock to prevent race condition
	//    - Timer information
	if room.Timer != nil {
		baseState.TimeRemaining = int64(room.Timer.TimeRemaining)
	}
	//    - Masked word (if in drawing phase)
	maskedWord := ""
	fullWord := room.Word
	if baseState.Phase == internal.PhaseDrawing {
		maskedWord = utils.GetMaskedWord(room.Word)
	}

	// Snapshot current drawer for later use
	var currentDrawer *internal.Player
	if room.Current != nil {
		currentDrawer = room.Current
	}

	room.Mu.RUnlock() // Now safe to unlock, we have all needed data

	// Copy for guessers (masked word)
	guesserState := baseState
	guesserState.Word = maskedWord
	gameStateUpdateGuessers := internal.Message[internal.GameStateData]{
		Type: "game_state_update",
		Data: guesserState,
	}

	// Copy for drawer (full word)
	drawerState := baseState
	drawerState.Word = fullWord
	gameStateUpdateDrawer := internal.Message[internal.GameStateData]{
		Type: "game_state_update",
		Data: drawerState,
	}

	// 2. Send different data based on player role:
	if currentDrawer != nil {
		if err := currentDrawer.Conn.WriteJSON(gameStateUpdateDrawer); err != nil {
			log.Printf("[BroadcastGameState] Failed to send drawer state to %s: %v",
				currentDrawer.Username, err)
			utils.LogGameEvent(room, gameStateUpdateDrawer.Type, map[string]any{
				"game_state_data": drawerState,
				"err":             err.Error(),
			})
			if websocket.IsCloseError(err) {
				// CRITICAL FIX: Run removePlayer in goroutine to avoid potential deadlock
				go func() {
					log.Printf("[BroadcastGameState] Removing disconnected drawer %s", currentDrawer.Username)
					removePlayer(currentDrawer)
				}()
			}
		} else {
			log.Printf("[BroadcastGameState] Sent drawer state to %s", currentDrawer.Username)
		}
	}

	// 3. Broadcast game_state_update message to all other players
	// IMPROVEMENT: Run in goroutine to avoid blocking
	go func() {
		log.Printf("[BroadcastGameState] Broadcasting guesser state to room %s", room.Id)
		SafeBroadcastToRoomExcept(room, gameStateUpdateGuessers, currentDrawer)
	}()
}
