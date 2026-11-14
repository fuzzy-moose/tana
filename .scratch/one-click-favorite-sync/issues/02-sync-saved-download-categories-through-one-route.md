# 02: Sync the saved download categories through one collector route

What to build: Tana can ask the collector, in one request, for an incremental Panda favorite sync of every saved Panda automatic download category. The collector reads the saved selection itself and admits all those categories in one transaction, so the request either covers all of them or fails as a whole. The sidenav quick action (ticket 03) calls this route; nothing in the UI changes yet.

This ticket is the server side of spec behaviours 2, 7, 8 and 25. Its layers are each specified in spec **Design**:

- `internal/collector/favorites`: `ErrNoDownloadCategories` and `func (s *Service) EnqueueDownloadCategories() error`. Read the saved categories after taking the download-settings row lock, in the same transaction as the enqueue, then follow the path `Enqueue(category, false)` takes. Before the baseline is `ready` that path bootstraps or resumes all ten categories (`store.enqueue` in `downloads.go`). Afterwards it coalesces through `store.enqueueCategories`. Wake the worker, and return `ErrClosed` after `Close`, like `Service.enqueue`. Keep the lock order: settings row before `favorite_syncs`.
- `internal/collector/httpapi/favorites.go`: `HandleSyncDownloadCategories`, registered in `routes.go` as `POST /api/favorites/download-categories/sync`. It takes the body `{}` and returns 202 with an empty body, 409 `{"error":"no_download_categories"}`, or 503 on any other error.
- `internal/collectorapi/control.go`: `ErrNoDownloadCategories` and `func (c *Client) SyncDownloadCategories(ctx context.Context) error`. 202 gives nil, 409 gives `ErrNoDownloadCategories`, and any other status gives `*HTTPError`. Use the same 4 s timeout as `SyncFavorites`.
- `internal/local/httpapi/collector.go`: `HandleCollectorSyncDownloadCategories`, registered in `routes.go` as `POST /api/collector/favorites/download-categories/sync`. Map errors the way `HandleCollectorSyncFavorites` does.

Test seams: package tests in `internal/collector/favorites`, using `manualService`, `configureDownloads` and `bootstrap` from `downloads_test.go`, for which categories get queued. For the HTTP mapping, a local httpapi test in `internal/local/httpapi/collector_test.go` that runs the real collector handler behind `httptest.NewServer`, like `TestLocalCollectorStatusAndSync`. Don't change existing assertions.

Blocked by: None

Status: resolved

- [x] With the baseline `ready` and saved download categories `[1, 4]`, `EnqueueDownloadCategories` queues an incremental (non-full) sync for categories 1 and 4 only. The favorites status shows the other eight categories unchanged.
- [x] With no saved download categories, `EnqueueDownloadCategories` returns `ErrNoDownloadCategories` and queues nothing. This holds both before and after the baseline is ready.
- [x] With the baseline `not_started` and saved download categories `[2]`, `EnqueueDownloadCategories` queues full syncs of all ten categories and moves the baseline to `collecting`.
- [x] With the baseline `ready`, `EnqueueDownloadCategories` returns nil without adding work when a saved category is running or already queued. A category with a queued full re-sync still shows `queued_full` afterwards, and a running category stays the only sync for its category.
- [x] `POST /api/collector/favorites/download-categories/sync` with `{}` returns 202 `{"status":"accepted"}` after download categories are saved. With none saved, it returns 409 `{"error":"no_download_categories"}`. With a body carrying unknown fields, it returns 400.
- [x] The same local route returns 502 `collector_unauthorized` when the collector rejects Tana's token, and 503 `collector_not_configured` when Tana has no collector client.
- [x] `go vet ./...`, `go test ./...` and `gofmt -l .` are clean.

## Resolution

Added `favorites.Service.EnqueueDownloadCategories` (saved categories read under the settings lock via `store.enqueueSelected`), the collector route `POST /api/favorites/download-categories/sync`, `collectorapi.Client.SyncDownloadCategories`, and the local route `POST /api/collector/favorites/download-categories/sync`, with package tests in `downloads_test.go` and `TestLocalCollectorSyncDownloadCategories`.
