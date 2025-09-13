package utils

import (
	"log"
	"math/rand"
	"slices"
	"strings"
	"time"

	"github.com/scythe504/skribblr-backend/internal"
)

// =============================================================================
// UTILITY FUNCTIONS
// =============================================================================

// GetMaskedWord converts word to underscores for display
func GetMaskedWord(word string) string {
	// TODO:
	if word == "" {
		return ""
	}
	// 1. Convert each letter to underscore
	masked := make([]string,0, len(word))
	// 2. Preserve spaces and punctuation
	for i := range word {
		if word[i] == ' ' {
			masked[i] = " "
		} else {
			masked[i] = "_"
		}
	}

	// 3. Return format like "_ _ _ _ _"
	return strings.Join(masked, " ")
	// 4. Optional: reveal some letters as hints over time
}

func GenerateWordChoices() []string {
	// TODO:
	// Initialize random seed
	rand.NewSource(time.Now().UnixNano())
	
	var choices []string
	
	// 1. Select one word from each difficulty (easy, medium, hard)
	// 2. Randomize selection within each category
	easyChoice := easyWords[rand.Intn(len(easyWords))]
	mediumChoice := mediumWords[rand.Intn(len(mediumWords))]
	hardChoice := hardWords[rand.Intn(len(hardWords))]
	
	// Add to choices slice
	choices = append(choices, easyChoice.Text, mediumChoice.Text, hardChoice.Text)
	
	// 5. Ensure no duplicates (basic check - unlikely with different difficulty levels)
	// This is a simple check since words from different difficulty levels are unlikely to duplicate
	seen := make(map[string]bool)
	var uniqueChoices []string
	
	for _, word := range choices {
		if !seen[word] {
			seen[word] = true
			uniqueChoices = append(uniqueChoices, word)
		}
	}
	
	// If we somehow have duplicates, fill with random words from any category
	for len(uniqueChoices) < 3 {
		var randomWord string
		switch rand.Intn(3) {
		case 0:
			randomWord = easyWords[rand.Intn(len(easyWords))].Text
		case 1:
			randomWord = mediumWords[rand.Intn(len(mediumWords))].Text
		case 2:
			randomWord = hardWords[rand.Intn(len(hardWords))].Text
		}
		
		if !seen[randomWord] {
			seen[randomWord] = true
			uniqueChoices = append(uniqueChoices, randomWord)
		}
	}
	
	// 3. Shuffle the final array
	for i := len(uniqueChoices) - 1; i > 0; i-- {
		j := rand.Intn(i + 1)
		uniqueChoices[i], uniqueChoices[j] = uniqueChoices[j], uniqueChoices[i]
	}
	
	// 4. Return slice of 3 words
	return uniqueChoices
}

// UpdatePlayerOrder rebuilds the drawing rotation order
func UpdatePlayerOrder(room *internal.Room) {
	// TODO:
	room.Mu.Lock()
	defer room.Mu.Unlock()

	// 1. Clear existing PlayerOrder slice
	room.PlayerOrder = make([]string, 0)

	// 2. Add all connected players to slice
	for _, player := range room.Players {
		if player.IsConnected {
			room.PlayerOrder = append(room.PlayerOrder, player.Id)
		}
	}

	// 3. Optional: shuffle for fairness

	// 4. Adjust CurrentIndex if it's now invalid
	if room.CurrentIndex >= len(room.PlayerOrder) {
		room.CurrentIndex = 0
	}

	// 5. Handle case where current drawer left
	if room.Current != nil {
		found := slices.Contains(room.PlayerOrder, room.Current.Id)
		if !found {
			room.Current = nil     // no valid current drawer anymore
			room.CurrentIndex = 0  // reset index
		}
	}
}


// ValidateGameState checks room state consistency
func ValidateGameState(room *internal.Room) bool {
	room.Mu.RLock()
	defer room.Mu.RUnlock()

	// 1. Check CurrentIndex is valid for PlayerOrder
	if room.CurrentIndex < 0 || room.CurrentIndex >= len(room.PlayerOrder) {
		log.Printf("[ValidateGameState] Invalid CurrentIndex: %d (PlayerOrder length %d)", room.CurrentIndex, len(room.PlayerOrder))
		return false
	}

	// 2. Check Current player exists in Players map
	if room.Current != nil {
		if _, ok := room.Players[room.Current.Id]; !ok {
			log.Printf("[ValidateGameState] Current drawer %s not found in Players map", room.Current.Id)
			return false
		}
		// also make sure Current.Id matches PlayerOrder[CurrentIndex]
		if room.PlayerOrder[room.CurrentIndex] != room.Current.Id {
			log.Printf("[ValidateGameState] CurrentIndex/player mismatch: Current=%s, PlayerOrder[%d]=%s",
				room.Current.Id, room.CurrentIndex, room.PlayerOrder[room.CurrentIndex])
			return false
		}
	}

	// 3. Check Phase matches expected state
	switch room.Phase {
	case internal.PhaseWaiting, internal.PhaseDrawing, internal.PhaseRevealing, internal.PhaseEnded, internal.PhaseLobby:
		// valid states
	default:
		log.Printf("[ValidateGameState] Invalid Phase: %s", room.Phase)
		return false
	}

	// 4. Check timer state consistency
	if room.Timer != nil {
		remaining := room.Timer.Duration - time.Since(room.Timer.StartTime)
		if remaining < 0 && room.Timer.IsActive {
			log.Printf("[ValidateGameState] Timer expired but still marked active")
			return false
		}
	}

	// 5. If we reach here, state looks fine
	return true
}


// GetPlayerStats returns formatted statistics for a player
func GetPlayerStats(player *internal.Player) map[string]any {
	// TODO:
	// 1. Calculate accuracy rate (correct/total guesses)
	// 2. Calculate average guess time
	// 3. Count total games played
	// 4. Return formatted stats map
	return map[string]any{}
}

// =============================================================================
// ROOM STATISTICS & ANALYTICS
// =============================================================================

// GetRoomStats returns current room statistics
func GetRoomStats(room *internal.Room) map[string]interface{} {
	// TODO:
	// 1. Count connected players
	// 2. Calculate average scores
	// 3. Get game duration
	// 4. Count total guesses made
	// 5. Return formatted stats
	return map[string]interface{}{}
}

// LogGameEvent records important game events for analytics
func LogGameEvent[T any](room *internal.Room, eventType string, data T) {
	// TODO:
	// 1. Create structured log entry
	// 2. Include room ID, timestamp, event type
	// 3. Include relevant game state
	// 4. Write to log file or analytics service
	// 5. Handle different event types appropriately
}
