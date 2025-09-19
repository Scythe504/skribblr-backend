package game

import (
	"math"
	"slices"

	"github.com/scythe504/skribblr-backend/internal"
)

// CalculateFinalResults compiles leaderboard and awards from a finished game
func CalculateFinalResults(room *internal.Room) internal.FinalResults {
	room.Mu.Lock()
	defer room.Mu.Unlock()
	results := internal.FinalResults{}

	playerData := make([]internal.GameResultData, 0, len(room.Players))
	// TODO: 1. Gather all players into a slice
	for _, player := range room.Players {
		// - Iterate room.Players map
		// - Convert each Player into a GameResultData
		playerData = append(playerData, internal.GameResultData{
			// - Keep Score, Username, PlayerID
			PlayerID: player.Id,
			Username: player.Username,
			Score:    player.Score,
		})
	}

	// TODO: 2. Sort slice by Score descending
	slices.SortFunc(playerData, func(a internal.GameResultData, b internal.GameResultData) int {
		return b.Score - a.Score
	})
	// - Assign Position (1st, 2nd, …) as you go
	for idx := range playerData {
		playerData[idx].Position = idx + 1
	}
	results.Leaderboard = playerData

	// TODO: 3. Compute MVP
	// - Could be simply the highest scorer (first in sorted list)
	if len(playerData) > 0 {
		results.MVP = &playerData[0]
	}
	// TODO: 4. Compute fastest guesser
	results.FastestGuess = &internal.GameResultData{
		TimeToGuess: math.MaxInt64,
	}
	// - Iterate room.RoundStats → look at CorrectGuesses
	for _, stat := range room.RoundStats {
		// - Track the PlayerGuess with the lowest GuessTime
		for _, guessTime := range stat.CorrectGuessers {
			// - Convert that player into a GameResultData entry
			if int64(guessTime.GuessTime) < results.FastestGuess.TimeToGuess {
				results.FastestGuess.PlayerID = guessTime.PlayerID
				results.FastestGuess.Username = guessTime.Username
				results.FastestGuess.TimeToGuess = int64(guessTime.GuessTime)
			}
		}
	}
	if results.FastestGuess.TimeToGuess == math.MaxInt64 {
		results.FastestGuess = nil // no correct guesses recorded
	}

	// TODO: 6. Fill metadata
	// - results.RoundsPlayed = room.RoundNumber
	results.RoundsPlayed = room.RoundNumber
	// - results.TotalPlayers = len(room.Players)
	results.TotalPlayers = len(room.Players)

	// TODO: 7. Return results
	return results
}
