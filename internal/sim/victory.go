package sim

// GameResult encodes the outcome for a single player.
type GameResult uint8

const (
	GameOngoing GameResult = 0
	GameVictory GameResult = 1
	GameDefeat  GameResult = 2
	GameDraw    GameResult = 3
)

// checkVictory determines if the game has ended.
// A player loses if they have 0 surviving buildings or have surrendered.
func checkVictory(w *World) []PlayerResult {
	if len(w.Players) == 0 {
		return nil
	}

	aliveBld := make([]bool, len(w.Players))
	for i := range w.Buildings {
		b := &w.Buildings[i]
		if b.State != BldDead && int(b.Owner) < len(aliveBld) {
			aliveBld[b.Owner] = true
		}
	}

	losers := make([]bool, len(w.Players))
	living := 0
	for i := range w.Players {
		if !aliveBld[i] || w.Players[i].Surrendered {
			losers[i] = true
		} else {
			living++
		}
	}

	if living == len(w.Players) {
		return nil
	}

	results := make([]PlayerResult, len(w.Players))
	if living == 1 {
		for i := range w.Players {
			pid := uint8(i)
			if losers[i] {
				results[i] = PlayerResult{PlayerID: pid, Result: uint8(GameDefeat)}
			} else {
				results[i] = PlayerResult{PlayerID: pid, Result: uint8(GameVictory)}
			}
		}
	} else {
		for i := range w.Players {
			results[i] = PlayerResult{PlayerID: uint8(i), Result: uint8(GameDraw)}
		}
	}

	return results
}
