# 09: Tana routes for the score model

What to build: the browser can reach the score model through the local service. Tana asks the collector for a preview and adds its own default filter to the request, applies a candidate, changes the active threshold, and turns the model off. The page that uses these routes comes in tickets 11 and 12.

This is spec behaviours 46 (the error codes), 52 (Tana supplies the default filter), 57, 58 and 59. Its layers are specified in spec **Design**:

- `internal/collectorapi`: `Client.PreviewScoreModel`, `Client.ApplyScoreModel`, `Client.SetScoreThreshold`, `Client.RemoveScoreModel` (the lines marked ticket 09). The preview decodes within the existing 4 MiB limit of `catalogRequest`'s pattern. 204 is success for the three changes. Other statuses return `downloadResponseError`.
- `internal/local/httpapi`, in the score routes file of ticket 02:
  - `HandleCollectorScorePreview(client, defaults *catalogfilter.Store)`, registered as `POST /api/collector/catalog/score/preview`. Body `{"favorites": true|false|null}`; a missing field means null. It loads the default filter as `HandleCollectorCatalog` does (same error handling) and always sends it: this page has no bypass.
  - `HandleCollectorApplyScoreModel` (`POST /api/collector/catalog/score/apply`, `{"candidate_id","threshold_tenths"}`), `HandleCollectorSetScoreThreshold` (`PUT /api/collector/catalog/score/threshold`, `{"threshold_tenths"}`), `HandleCollectorRemoveScoreModel` (`DELETE /api/collector/catalog/score/model`). They forward and leave validation to the collector.
  - `writeCatalogCollectorError` passes 400 `invalid_threshold` and 409 `score_candidate_not_found` and `score_model_not_active` through.

Test seams: `internal/local/httpapi/catalog_test.go`, running the real collector handler behind `httptest.NewServer` as `TestLocalCatalogVerdicts` does, with a `catalogfilter.Store` set up as in `catalog_default_filter_test.go`.

Blocked by: 06

Status: resolved

- [x] Through the real collector handler, local `POST /api/collector/catalog/score/preview` with `{"favorites":false}` returns 200 with the collector's preview JSON: the evidence counts of the seeded verdicts and a `candidate` with an `id`.
- [x] With a default filter saved in Tana's store, the collector receives it with the preview request (`default_query` and `default_categories` in the request body), and an empty filter is sent as empty values.
- [x] A body without `favorites` and one with `"favorites":null` both reach the collector with a null `favorites`.
- [x] Local `POST /api/collector/catalog/score/apply` with the preview's candidate ID returns 204, and `GET /api/collector/catalog` then carries `score_filter` with the applied `threshold_tenths`. An unknown candidate gives 409 `score_candidate_not_found`, and a threshold of 1001 gives 400 `invalid_threshold`.
- [x] Local `PUT /api/collector/catalog/score/threshold` returns 204 and the next catalog page carries the new threshold. Without an active model it gives 409 `score_model_not_active`.
- [x] Local `DELETE /api/collector/catalog/score/model` returns 204, and the next catalog page has no `score_filter` when no override exists.
- [x] Each of the four routes gives 503 `collector_not_configured` with a nil client and 502 `collector_unauthorized` with the wrong token. A collector that can't be reached gives 502 `collector_unavailable`. Bodies with unknown fields give 400 `invalid_json`.
- [x] `go vet ./...`, `go test ./...` and `gofmt -l .` are clean, with existing assertions unchanged.

## Resolution

`collectorapi.Client` gained `PreviewScoreModel`, `ApplyScoreModel`, `SetScoreThreshold` and `RemoveScoreModel`; `internal/local/httpapi/catalog_score.go` adds the four local score-model routes (the preview always sends Tana's default filter), `writeCatalogCollectorError` passes `invalid_threshold`, `score_candidate_not_found` and `score_model_not_active` through, and `TestLocalScoreModel` covers them through the real collector handler.
