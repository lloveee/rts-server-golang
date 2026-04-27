package sim

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

type goldenEmpty struct {
	EmptyWorldHash string `json:"emptyWorldHash"`
}

// TestEmptyWorldHash_MatchesGoldenAnchor asserts Go's empty-world hash
// equals the pre-computed anchor in GoldenData.json. This catches any
// accidental drift between the exported golden data and live sim.
func TestEmptyWorldHash_MatchesGoldenAnchor(t *testing.T) {
	jsonPath := filepath.Join("..", "..", "..", "..", "..",
		"code", "_Unity", "rts-client-unity", "Assets", "Tests", "EditMode", "GoldenData.json")
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Skipf("golden data not found at %s: %v", jsonPath, err)
	}

	var g goldenEmpty
	if err := json.Unmarshal(data, &g); err != nil {
		t.Fatalf("failed to parse golden data: %v", err)
	}

	w := NewWorld(42, 100, 100)
	got := fmt.Sprintf("%016x", Hash(w))

	if got != g.EmptyWorldHash {
		t.Fatalf("Go empty-world hash %s != golden anchor %s", got, g.EmptyWorldHash)
	}
	t.Logf("empty-world parity: Go %s == golden anchor %s", got, g.EmptyWorldHash)
}
