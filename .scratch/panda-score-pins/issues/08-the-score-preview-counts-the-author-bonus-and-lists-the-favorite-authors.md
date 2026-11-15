# 08: The score preview counts the author bonus and lists the favorite authors

What to build: the preview of the evaluation page says whether the author bonus is on, how many favorite authors there are, and lists the authors with the most Panda favorites, each with its favorite count and its effective weight under the candidate. Every number the preview gives about the candidate or the listing counts the bonus, as it counts the pins since ticket 05. The page shows all this in ticket 13.

This is the bonus side of spec behaviour 35, the data of behaviour 36, and the preview's part of behaviour 54. Its layers are specified in spec **Design**:

- `internal/collectorapi/catalog_score.go`: `ScoreFavoriteAuthor`, and on `ScorePreview` the fields `AuthorBonus *float64` (`json:"author_bonus"`), `FavoriteAuthorCount int` (`json:"favorite_author_count"`) and `FavoriteAuthors []ScoreFavoriteAuthor` (`json:"favorite_authors"`, never nil).
- `internal/collector/catalog`: **Design → Favorite authors in the preview (ticket 08)**. `PreviewScoreModel` reads the bonus and all rows of `favorite_authors` with their labels. `FavoriteAuthorCount` is the number of rows. `FavoriteAuthors` holds the `scorePreviewAuthors = 500` authors with the most favorites, more favorites first, then by label. Each `Weight` follows the Go side of **Design → The weight rule** with the candidate's calculated weights: the pin, else with the bonus on the larger of the calculated weight and the bonus, else the calculated weight, else nil. Without a candidate there are no calculated weights. The Go function of ticket 05 gains the favorite author part, so `heldOutTenths` counts the bonus. `scoreFilter.recent` already scores through the builder of ticket 07, so it needs a test and no code.
- The bonus counts whether or not the preview counts Panda favorites as "Interesting".
- No route is added, and the handlers need no code.

Test seams: package tests in `internal/collector/catalog` with `openCatalog`, `retain`, `seedScoreEvidence`, `seedScoredGalleries`, `previewScore`, `weightOf` and `histogramCounts`. A test makes a tag a favorite author by inserting its row into `favorite_authors` directly. New assertions in `TestLocalScoreModel` (`internal/local/httpapi/catalog_test.go`) or a test beside it. Existing assertions stay unchanged.

Hints (optional): in `seedScoreEvidence`, the "Not for me" galleries 1 to 3 carry `artist:a` (calculated weight about -0.47), and gallery 104 of `seedScoredGalleries` carries it with `female:glasses` and five more artist tags (score about -3.51, bucket -36). With `artist:a` a favorite author and a bonus of 100, those galleries score about 100 higher.

Blocked by: 05, 07

Status: resolved

- [x] `PreviewScoreModel` returns a nil `AuthorBonus` while the bonus is off, encoded as `"author_bonus":null`, and the stored number while it is on.
- [x] `FavoriteAuthorCount` is the number of favorite authors. `FavoriteAuthors` lists them with more favorites first and then by label, each with its label in the tag's stored spelling and its favorite count. Without favorite authors the count is 0 and the JSON holds `"favorite_authors":[]`.
- [x] With more than 500 favorite authors, `FavoriteAuthors` holds the 500 with the most favorites, and `FavoriteAuthorCount` counts all.
- [x] The `Weight` of a listed author is its effective weight under the candidate: its pin, else with the bonus on the larger of its calculated weight and the bonus, else its calculated weight, else nil. Without a candidate it is the pin, else the bonus while it is on, else nil.
- [x] The held-out scores count the bonus: with the bonus on, an evidence gallery that carries a favorite author's tag scores with the bonus where the bonus gives the tag's weight, and as before with the bonus off.
- [x] The recent scores count the bonus: a recent gallery that carries a favorite author's tag is counted in the bucket of its score with the bonus in `Recent.Candidate`, and in the bucket of its calculated score with the bonus off.
- [x] Through the real collector handler, the answer of local `POST /api/collector/catalog/score/preview` holds `author_bonus`, `favorite_author_count` and `favorite_authors` with the stored bonus and the favorite author's `label`, `favorites` and `weight`.
- [x] `go vet ./...`, `go test ./...` and `gofmt -l .` are clean, with existing assertions unchanged.

## Resolution

`ScorePreview` gained `AuthorBonus`, `FavoriteAuthorCount` and `FavoriteAuthors` (`ScoreFavoriteAuthor`); `PreviewScoreModel` reads the favorite authors and lists the 500 with the most favorites, and the Go weight rule became `weightRule.effective` with the bonus, so `heldOutTenths` and the listed weights count it. Tests in `score_author_bonus_preview_test.go` and `TestLocalScoreModel`.
