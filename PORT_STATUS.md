## Port status: syzygy and nnue

This document records the state of the two newly ported packages, `syzygy/` and
`nnue/`, as of the most recent verification run.

### Whole-repo build and checks

All three commands pass cleanly with no source changes required:

- `go build ./...` — exit 0, no output.
- `go vet ./...` — exit 0, no output.
- `go test ./syzygy/ ./nnue/` — both packages pass (`-count=1`, no cache).

No compile errors needed fixing. Neither new package is imported by any existing
engine package (`position`, `search`, `engines`, `lichess`, `uci`, `uciclient`,
`web`, `cmd`), so the engine's behaviour is untouched. Both packages are
currently standalone libraries with their own tests; nothing wires them into
search or evaluation yet.

## syzygy

A Go port of Ronald de Man's Syzygy tablebase prober (the Fathom / `tbprobe.c`
+ `tbcore.c` lineage). The authoritative reference is vendored at
`reference/fathom/tbprobe.c` and `reference/fathom/tbprobe.h`.

### What is implemented

- WDL (`.rtbw`, win/draw/loss) probing only. The public entry point is
  `Tablebases.ProbeWDL(*position.Position) (wdl int, ok bool)` returning the
  side-to-move value in `{-2,-1,0,1,2}` (loss / blessed loss / draw / cursed
  win / win).
- Eager file loading (`Load(dir)` reads every `.rtbw` into RAM) with lazy
  per-table index parsing on first probe (`init.go`: `initTableWDL`,
  `setupPairs`), matching de Man's design.
- The full compressed-table machinery: material-key hashing (`calcKey`,
  `calcKeyFromPcs`), table-name parsing and `enc_type` selection
  (`parseName`), the recursive-pairing / canonical-Huffman block decoder
  (`decompressPairs`), and the piece/pawn index encoders (`encodePiece`,
  `encodePawn`, with the `norm`/`factor`/`pawnFile` tables).
- Probe-side search needed for correctness: `probe_ab` alpha-beta capture
  resolution (`probeAB`), `probe_wdl` including the en-passant special case
  (`probeWDL`, `genPawnEPCaptures`), and the minimal pseudo-legal move
  generation it relies on (`genCapturesOrPromotions`, `genMoves`, `doMove`,
  attack tables in `attacks.go`).
- Defensive hardening beyond the reference: `ProbeWDL` rejects castling rights,
  positions above `MaxPieces`, and illegal positions (`probeLegal`). There is
  an explicit guard against kingless-material-key collisions that would
  otherwise drive `decompressPairs` into a non-terminating bitstream. These
  guards are the package's own additions; the reference assumes legal input.

### Builds

Yes. `go build ./syzygy/` and `go vet ./syzygy/` are clean.

### Validated against real data, and the result

Genuine Syzygy WDL tables are committed under `testdata/syzygy/` and the WDL
magic word (`0x5d23e871`) is present in their headers. The suite covers:

- All five 3-man piece types: KQvK, KRvK (wins), KBvK, KNvK (draws), and KPvK
  (win with white to move, draw with black to move, plus an e-file variant).
- One genuine 4-man table, KQvKR, with win/win/loss cases across both sides to
  move. This pushes `MaxPieces()` to 4.

`TestProbesSucceedWithKnownValues` asserts `ok == true` and exact WDL values for
ten positions; the in-source comments state these expected values were
cross-checked against python-chess 1.11.2 on the same testdata.
`TestIllegalPositionIsRejectedAndDoesNotHang` confirms two illegal FENs return
`ok == false` and that probing terminates (guards a previously observed panic +
infinite-loop). All syzygy tests pass with no skips.

### DTZ (now implemented)

- DTZ probing IS implemented: `.rtbz` parsing, `ProbeDTZ` (probe_dtz), and
  `ProbeRoot` (probe_root root-move selection) are ported from Fathom's
  tbprobe.c. The search plays the `ProbeRoot` move directly at the root, so won
  endgames convert optimally under the fifty-move rule.
- Verified against python-chess as an oracle: `ProbeDTZ` matched on 11,254
  positions across all six fixture material types (only the documented
  old-vs-new mate-in-1 rounding differs, compensated in `probe_root`).
  `ProbeRoot` was optimal and win-preserving on 11,379 positions.

### Uncertain / TODO

- Validation breadth is narrow: only 3-man tables and a single 4-man table
  (KQvKR) are exercised. Pawn tables are covered only by KPvK. There is no
  test over 4-man tables with pawns (e.g. KPvKP beyond KPvK), no 5-man+ table,
  and no large random/perft-style cross-check against an external prober. The
  pawn-file / multi-file (`files == 4`) and split-table (`split`) code paths are
  therefore only lightly exercised.
- The cursed-win / blessed-loss DTZ path (`wdl == ±1`, the `dtz += 100` offset
  and the mapped-value doubling) is ported verbatim from C but not exercised by
  the small 3-4-man fixtures (which contain no cursed wins); the live 3-4-5
  tables on the server will exercise it, but it has no local oracle test.
- Cursed-win / blessed-loss WDL values (`±1`) are produced by the code path but are
  not directly asserted by any test case (all expected values are in
  `{-2,0,2}`).

## nnue

A Go port of the classic Stockfish HalfKP NNUE evaluator
(`HalfKP_256x2-32-32`). The authoritative reference is vendored under
`reference/nnue/` (`evaluate_nnue.cpp`, `nnue_feature_transformer.h`,
`features/half_kp.cpp`, the `layers/` directory, etc.).

### What is implemented

- File loading (`Load`, `ReadNetwork`) of the standard little-endian classic
  NNUE serialization: version word `0x7AF32F16`, 32-bit architecture hash, the
  ASCII architecture string, the feature-transformer block (256 int16 biases +
  41024x256 int16 weights), then the dense stack read in Stockfish's recursive
  order (affine1 512->32, affine2 32->32, output 32->1, each as int32 biases +
  int8 weights).
- Serialization (`Serialize`, `Save`, `NewEmptyNetwork`) producing the exact
  inverse byte layout, so a network round-trips.
- HalfKP feature indexing: the `PS_` offsets, `orient` (180-degree perspective
  flip, `s ^ 63`), `MakeIndex`, `appendActiveIndices`, and the
  `kppBoardIndex` per-perspective piece table.
- The accumulator (`Accumulator.Refresh`, `Add`, `Remove`) with both perspectives
  and incremental update support.
- Full forward evaluation (`Eval`, `EvalWith`): feature-transformer clip to
  `[0,127]`, the affine + clipped-ReLU dense stack, and the divide-by-`FV_SCALE`
  (16) to centipawns, side-to-move perspective. Scalar reference arithmetic only
  (no SIMD), matching the reference's scalar path.
- Architecture-hash computation (`ExpectedNetworkHash` and the per-layer hash
  helpers) used to validate that a loaded file is the expected architecture
  (`Network.HashOK`).

### Builds

Yes. `go build ./nnue/` and `go vet ./nnue/` are clean.

### Validated against real data, and the result

A genuine 21 MB classic Stockfish HalfKP net is committed at
`nnue/testdata/net.nnue`. Its header was inspected directly: version
`0x7AF32F16`, hash `0x3E5AA6EE`, arch string
`Features=HalfKP(Friend)[41024->256x2],Network=Affine...`. Tests that run
against it (none were skipped, because the net is present):

- `TestExpectedNetworkHash` pins the computed architecture hash to
  `0x3E5AA6EE`, the value embedded in the real file — so the `PS_` offsets,
  layer stack, and dimensions are confirmed correct against real data.
- `TestRealNetLoads` loads it with `HashOK == true` and the correct
  feature-transformer weight count (41024 * 256).
- `TestRealNetStartPositionSmall` requires `|eval| <= 100 cp` at the starting
  position (a correctly loaded balanced net).
- `TestRealNetMaterialSign` checks the eval sign and rough magnitude for a side
  up a queen, from both sides to move (>= +500 / <= -500 / <= -300 cp).
- `TestRealNetIncrementalMatchesRefresh` confirms an incrementally maintained
  accumulator matches a from-scratch refresh, and that the resulting evals are
  identical, on the real net.

Additionally, structural and round-trip behaviour is checked with a tiny
hand-made network (`TestTinyNetworkRoundTrip`, `TestTinyNetworkEvalDeterministic`,
`TestAccumulatorAddRemoveSymmetry`) and the feature-indexing invariants
(`TestFeatureDimensions`, `TestOrient`, `TestMakeIndexRange`). All nnue tests
pass.

### Uncertain / TODO

- No exact numeric cross-check against Stockfish. The real-net tests assert
  sign, rough magnitude, and self-consistency (incremental == refresh,
  round-trip stability), but no test compares a specific centipawn eval of a
  specific FEN to the value the reference Stockfish produces for the same net.
  An off-by-a-few-cp quantization or rounding discrepancy would not be caught.
- Only the HalfKP_256x2-32-32 architecture is supported. There is no support
  for HalfKAv2 / later "big"/"small" net formats, no header-driven architecture
  dispatch — the dimensions are hard-coded.
- Scalar evaluation only; no SIMD. Correct but not optimized, and not
  benchmarked for use inside a real search.
- Not wired into the engine. `engines`/`search` do not call into `nnue`; there
  is no incremental accumulator maintenance tied to make/unmake moves in the
  engine's own move loop yet, so the incremental path is exercised only by
  tests, not in play.
- `EvalWith` keeps an `Accumulator.computed` flag that the engine would need to
  respect when integrating incremental updates; that integration contract is
  untested outside the package.
