package utils

import (
	"math/rand"
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
	// 1. Clear existing PlayerOrder slice
	// 2. Add all connected players to slice
	// 3. Optional: shuffle for fairness
	// 4. Adjust CurrentIndex if it's now invalid
	// 5. Handle case where current drawer left
}

// ValidateGameState checks room state consistency
func ValidateGameState(room *internal.Room) bool {
	// TODO:
	// 1. Check CurrentIndex is valid for PlayerOrder
	// 2. Check Current player exists in Players map
	// 3. Check Phase matches expected state
	// 4. Check timer state consistency
	// 5. Return false if any inconsistencies found
	// 6. Log validation errors for debugging
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
func LogGameEvent(room *internal.Room, eventType string, data map[string]interface{}) {
	// TODO:
	// 1. Create structured log entry
	// 2. Include room ID, timestamp, event type
	// 3. Include relevant game state
	// 4. Write to log file or analytics service
	// 5. Handle different event types appropriately
}
