# 09: Tag weights for a list of tags

What to build: Tana can ask the collector for the weights of a list of tags, as the detail view of a local gallery has them. For each tag the answer says whether the Panda catalog has it, its effective weight (the pin, the author bonus or the calculated weight under the active model), and its pin. The answer also says whether a Panda score model is active. Without one, a tag has only its pin or the bonus as its weight. The detail view uses the route in ticket 14.

This is spec behaviour 53, the collector side of behaviours 42, 46 and 47, and behaviour 55 for this route. Its layers are specified in spec **Design**:

- `internal/collectorapi/catalog_score.go`: `ScoreTagWeights`, `ScoreTagWeight` and `Client.ScoreTagWeights`, which returns an error for an answer whose number of entries differs from the number of sent tags.
- `internal/collector/catalog`, new file: `ScoreTagWeights` (**Design → Tag weight lookup (ticket 09)**). One entry per sent tag, in the sent order, with `Tag` as sent. A tag is matched like a pin's tag: cut at its first colon, both parts trimmed, the namespace in lower case, the value folded as `replaceTags` folds it. An element without a colon or with an empty part is not known. The weight follows **Design → The weight rule** under the active model, and under no model when none is active. `Pin` is the tag's pin with its ID, kind, label and weight.
- `internal/collector/httpapi`: `POST /api/catalog/score/tags` with `{"tags": [string]}`, 200 with the answer. It decodes with `server.DecodeJSON`, so a body with unknown fields gives 400 `invalid_json`.
- `internal/local/httpapi/catalog_score.go`: `HandleCollectorScoreTagWeights`, registered as `POST /api/collector/catalog/score/tags`: `catalogCollectorConfigured`, `server.DecodeJSON`, the client call, `writeCatalogCollectorError`.

Hint: resolve all tags to their `catalog_tags` IDs in one statement and read their weights with the builder of tickets 04 and 07, with a NULL model ID when no model is active. A NULL model ID gives no calculated weight, so the pin and the bonus remain.

Test seams: package tests in `internal/collector/catalog` with `openCatalog`, `retain`, `seedScoreEvidence`, `applyCandidate` and `weightOf`. A test makes a tag a favorite author by inserting its row into `favorite_authors` directly. A case in `internal/collector/httpapi/catalog_test.go`. A client case in `internal/collectorapi` with a stubbed transport or `httptest.NewServer`, like `catalog_facts_test.go`. A local test in `internal/local/httpapi/catalog_test.go` through the real collector handler.

Blocked by: 07

Status: resolved

- [x] `ScoreTagWeights` returns one entry per sent tag, in the sent order, with the sent string as `Tag`. A tag of the Panda catalog is known, found without regard to case and with `_` for a space. A tag that the catalog does not have, and an element without `namespace:value`, are not known and have a nil `Weight`. An empty list gives an empty `Tags` whose JSON is `[]`.
- [x] With an active model, `Active` is true, a known tag has its calculated weight as `Weight`, and a known tag without a calculated weight has a nil `Weight`. A tag without a pin has a nil `Pin`.
- [x] A pinned tag has the pin's number as `Weight`, also a pin of 0, and a `Pin` with the pin's ID, the kind `tag`, the label and the weight.
- [x] With the bonus on, a favorite author's tag without a pin has at least the bonus as `Weight`.
- [x] Without an active model, `Active` is false. A pinned tag has its pin as `Weight`, a favorite author's tag has the bonus while it is on, and every other known tag has a nil `Weight`.
- [x] Collector `POST /api/catalog/score/tags` returns 200 with `active` and one entry under `tags` that has the keys `tag`, `known` and `weight`. A body with unknown fields gives 400 `invalid_json`.
- [x] `Client.ScoreTagWeights` returns an error when the collector answers with fewer or more entries than tags were sent.
- [x] Through the real collector handler, local `POST /api/collector/catalog/score/tags` returns 200 with the collector's answer, with a `pin` object on a pinned tag and no `pin` key on the others. Without a collector the route answers 503 `collector_not_configured`, and with a rejected token 502 `collector_unauthorized`.
- [x] `go vet ./...`, `go test ./...` and `gofmt -l .` are clean, with existing assertions unchanged.

## Resolution

Added `catalog.Service.ScoreTagWeights` (one statement over `unnest ... WITH ORDINALITY`, weights from `weightSQL` with a NULL model ID when none is active), the collector route `POST /api/catalog/score/tags`, `collectorapi.ScoreTagWeights`/`ScoreTagWeight` with `Client.ScoreTagWeights` (rejects an answer of another length), and the local route `POST /api/collector/catalog/score/tags`, with tests at each layer.
