package match

import "strings"

// defaultOpenings is a small, varied book of opening lines (UCI moves from the
// start position) used to diversify self-play games. Identical games would make
// the match a single repeated data point; a spread of openings, each played
// twice with colours reversed, gives the SPRT independent samples and cancels
// any opening or first-move bias.
var defaultOpenings = []string{
	"",                          // the bare start position
	"e2e4 e7e5",                 // open game
	"e2e4 c7c5",                 // Sicilian
	"e2e4 e7e6",                 // French
	"e2e4 c7c6",                 // Caro-Kann
	"e2e4 d7d5",                 // Scandinavian
	"e2e4 g7g6",                 // Modern
	"e2e4 d7d6",                 // Pirc
	"e2e4 e7e5 g1f3 b8c6",       // knights out
	"e2e4 e7e5 g1f3 b8c6 f1b5",  // Ruy Lopez
	"e2e4 e7e5 g1f3 b8c6 f1c4",  // Italian
	"e2e4 c7c5 g1f3 d7d6",       // Sicilian, Najdorf-ish
	"e2e4 c7c5 g1f3 b8c6",       // Sicilian, open
	"d2d4 d7d5",                 // closed
	"d2d4 g8f6",                 // Indian
	"d2d4 d7d5 c2c4",            // Queen's Gambit
	"d2d4 d7d5 c2c4 e7e6",       // QGD
	"d2d4 d7d5 c2c4 c7c6",       // Slav
	"d2d4 g8f6 c2c4 e7e6",       // Nimzo / QID
	"d2d4 g8f6 c2c4 g7g6",       // King's Indian / Grünfeld
	"d2d4 f7f5",                 // Dutch
	"c2c4 e7e5",                 // English
	"c2c4 g8f6",                 // English
	"g1f3 d7d5",                 // Réti
	"g1f3 g8f6",                 // symmetrical
	"d2d4 d7d5 g1f3 g8f6 c2c4",  // QGD move order
	"e2e4 e7e6 d2d4 d7d5",       // French advance setup
	"e2e4 c7c6 d2d4 d7d5",       // Caro main
	"d2d4 g8f6 c2c4 c7c5",       // Benoni-ish
	"c2c4 c7c5",                 // symmetrical English
}

// openingMoves splits a book line into its UCI moves (nil for the start position).
func openingMoves(line string) []string {
	line = strings.TrimSpace(line)
	if line == "" {
		return nil
	}
	return strings.Fields(line)
}
