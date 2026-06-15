# Opening book data

These TSV files are the [Lichess chess-openings dataset](https://github.com/lichess-org/chess-openings),
a list of several thousand named openings and their variations. Each file (`a.tsv`..`e.tsv`, by ECO
volume) has three columns: `eco`, `name`, `pgn`, where `pgn` is the opening's moves in SAN.

The dataset is released under the [Creative Commons CC0 1.0](https://creativecommons.org/publicdomain/zero/1.0/)
public-domain dedication.

`book.go` embeds these files and replays each line into a weighted, position-keyed opening book (see
the comments there). They are data, not code: regenerate by re-downloading from the upstream repo.
