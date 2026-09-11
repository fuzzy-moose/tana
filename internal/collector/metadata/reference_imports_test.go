package metadata

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/panda"
)

// As in batchService, drive durable transitions directly so crash checkpoints
// and in-flight submissions do not depend on winning a scheduling race.
func batchImports(t *testing.T, db *sql.DB, dir string) *ReferenceImports {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	return &ReferenceImports{db: db, dir: dir, logger: slog.New(slog.DiscardHandler), ctx: t.Context(), wake: make(chan struct{}, 1)}
}

func acceptImport(t *testing.T, imports *ReferenceImports, input string) collectorapi.ReferenceImport {
	t.Helper()
	item, err := imports.Accept(t.Context(), "references.txt", strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	return item
}

func readImport(t *testing.T, imports *ReferenceImports, id string) collectorapi.ReferenceImport {
	t.Helper()
	item, err := imports.Get(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	return item
}

func parseImports(t *testing.T, imports *ReferenceImports) {
	t.Helper()
	for {
		worked, err := imports.step(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if !worked {
			return
		}
	}
}

func TestReadImportBatch(t *testing.T) {
	for _, tc := range []struct {
		name    string
		input   string
		refs    []panda.GalleryRef
		invalid int64
	}{
		{
			name:  "lines and blanks",
			input: "123,abc\n\n \t\n456,def\n",
			refs:  []panda.GalleryRef{{ID: 123, Token: "abc"}, {ID: 456, Token: "def"}},
		},
		{
			name:  "windows line endings",
			input: "123,abc\r\n\r\n456,def\r\n",
			refs:  []panda.GalleryRef{{ID: 123, Token: "abc"}, {ID: 456, Token: "def"}},
		},
		{
			name:  "final line without newline",
			input: "123,abc",
			refs:  []panda.GalleryRef{{ID: 123, Token: "abc"}},
		},
		{
			name:    "invalid records leave following references usable",
			input:   "broken\n0,zero\n-1,negative\n,missing\n2,\n3,space token\n4,two,fields\n5,\xff\n6,valid\n",
			refs:    []panda.GalleryRef{{ID: 6, Token: "valid"}},
			invalid: 8,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := readImportBatch(strings.NewReader(tc.input))
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got.refs, tc.refs) || got.invalid != tc.invalid || got.bytes != int64(len(tc.input)) || !got.done {
				t.Fatalf("batch = %+v; want references %v, invalid %d, bytes %d, done true", got, tc.refs, tc.invalid, len(tc.input))
			}
		})
	}
}

func TestReferenceImportParsingCheckpointsAndCountersSurviveRestart(t *testing.T) {
	dir := t.TempDir()
	db := openDB(t, dir)
	imports := batchImports(t, db, filepath.Join(dir, "imports"))
	seedRefs(t, db, 1)
	if _, err := db.Exec(`UPDATE gallery_refs SET metadata_attempted_at = 1, metadata_error = 'failed'`); err != nil {
		t.Fatal(err)
	}
	var input strings.Builder
	for id := 1; id <= importBatchRecords; id++ {
		fmt.Fprintf(&input, "%d,token%d\n", id, id)
	}
	input.WriteString("\n1,token1\n2,token2\n")
	input.WriteString("broken\n0,bad\n4,space token\n")
	input.WriteString("7,\xff\n")
	// Long records and a final line without a newline remain valid.
	fmt.Fprintf(&input, "257,%s", strings.Repeat("x", 80<<10))
	item := acceptImport(t, imports, input.String())
	if item.Status != "processing" || item.References != 0 || item.ProcessedBytes != 0 {
		t.Fatalf("acceptance started parsing synchronously: %+v", item)
	}
	if worked, err := imports.step(t.Context()); err != nil || !worked {
		t.Fatalf("first checkpoint: %v, %v", worked, err)
	}
	checkpoint := readImport(t, imports, item.ID)
	if checkpoint.References != importBatchRecords || checkpoint.Known != 1 || checkpoint.ProcessedBytes >= checkpoint.SizeBytes {
		t.Fatalf("checkpoint = %+v", checkpoint)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db = openDB(t, dir)
	imports = batchImports(t, db, imports.dir)
	if err := imports.recover(t.Context()); err != nil {
		t.Fatal(err)
	}
	parseImports(t, imports)
	got := readImport(t, imports, item.ID)
	if got.Status != "validating" || got.References != 257 || got.Known != 1 || got.Pending != 256 ||
		got.Duplicates != 2 || got.Invalid != 4 || got.ProcessedBytes != got.SizeBytes {
		t.Fatalf("recovered summary = %+v", got)
	}
	if _, err := os.Stat(imports.path(item.ID)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("parsed upload retained: %v", err)
	}
	// A completed parsing checkpoint cannot add rows or counters a second time.
	if err := imports.recordBatch(t.Context(), item.ID, checkpoint.ProcessedBytes, referenceImportBatch{
		refs: []panda.GalleryRef{{ID: 999, Token: "unused"}}, bytes: 5, invalid: 2, done: true,
	}); err != nil {
		t.Fatal(err)
	}
	if after := readImport(t, imports, item.ID); !reflect.DeepEqual(got, after) {
		t.Fatalf("replayed checkpoint changed summary: %+v", after)
	}
}

func TestReferenceImportCheckpointAndAdmissionRollbackTogether(t *testing.T) {
	db := openDB(t, t.TempDir())
	imports := batchImports(t, db, t.TempDir())
	item := acceptImport(t, imports, "1,token1\n")
	if _, err := db.Exec(`CREATE TRIGGER reject_import_checkpoint BEFORE UPDATE OF processed_bytes ON reference_imports
		BEGIN SELECT RAISE(FAIL, 'storage failure'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := imports.step(t.Context()); err == nil {
		t.Fatal("checkpoint unexpectedly committed")
	}
	if got := readImport(t, imports, item.ID); got.References != 0 || got.ProcessedBytes != 0 {
		t.Fatalf("partial checkpoint escaped rollback: %+v", got)
	}
	if _, err := db.Exec(`DROP TRIGGER reject_import_checkpoint`); err != nil {
		t.Fatal(err)
	}
	parseImports(t, imports)
	s := batchService(t, db, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return metadataResponse(t, panda.Metadata{ID: 1, Token: "token1"}), nil
	}))
	if _, err := db.Exec(`CREATE TRIGGER reject_import_completion BEFORE UPDATE OF status ON reference_imports
		WHEN NEW.status = 'completed' BEGIN SELECT RAISE(FAIL, 'storage failure'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.collect(t.Context()); err == nil {
		t.Fatal("admission unexpectedly committed")
	}
	if got := readImport(t, imports, item.ID); got.Pending != 1 || got.Imported != 0 {
		t.Fatalf("outcome escaped rollback: %+v", got)
	}
	if _, err := s.store.q.GetGalleryToken(t.Context(), 1); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("admission escaped rollback: %v", err)
	}
	if _, err := db.Exec(`DROP TRIGGER reject_import_completion`); err != nil {
		t.Fatal(err)
	}
	collectBatch(t, s)
	if got := readImport(t, imports, item.ID); got.Status != "completed" || got.Imported != 1 {
		t.Fatalf("admission did not recover: %+v", got)
	}
}

func TestReferenceImportsTryTokensSequentiallyAndReuseInventory(t *testing.T) {
	db := openDB(t, t.TempDir())
	imports := batchImports(t, db, t.TempDir())
	seedRefs(t, db, 10)
	if _, err := db.Exec(`UPDATE gallery_refs SET metadata_attempted_at = 1, metadata_error = 'failed'`); err != nil {
		t.Fatal(err)
	}
	item := acceptImport(t, imports, `1,wrong
1,token1
1,later
10,token10
10,conflict
1,wrong
`)
	parseImports(t, imports)
	if got := readImport(t, imports, item.ID); got.References != 5 || got.Known != 1 || got.Failed != 1 || got.Pending != 3 || got.Duplicates != 1 {
		t.Fatalf("initial outcomes = %+v", got)
	}
	var calls int
	s := batchService(t, db, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		if ids := requestedIDs(t, req); !reflect.DeepEqual(ids, []int64{1}) {
			t.Fatalf("conflicting tokens batched or known inventory refreshed: %v", ids)
		}
		if calls == 1 {
			return metadataResponse(t, panda.Metadata{ID: 1, Error: "incorrect key"}), nil
		}
		return metadataResponse(t, panda.Metadata{ID: 1, Token: "token1"}), nil
	}))
	collectBatch(t, s)
	if got := readImport(t, imports, item.ID); got.Pending != 2 || got.Failed != 2 {
		t.Fatalf("first token failure = %+v", got)
	}
	collectBatch(t, s)
	got := readImport(t, imports, item.ID)
	if got.Status != "completed" || got.Imported != 1 || got.Known != 1 || got.Failed != 3 || got.Pending != 0 || calls != 2 {
		t.Fatalf("token validation = %+v, calls=%d", got, calls)
	}
	if token, err := s.store.q.GetGalleryToken(t.Context(), 1); err != nil || token != "token1" {
		t.Fatalf("authoritative token = %q, %v", token, err)
	}
}

func TestReferenceImportsRunAfterOtherWorkOldestFirstAndShareExplicitFetch(t *testing.T) {
	db := openDB(t, t.TempDir())
	imports := batchImports(t, db, t.TempDir())
	first := acceptImport(t, imports, "1,token1\n9,token9\n")
	second := acceptImport(t, imports, "2,token2\n")
	parseImports(t, imports)
	seedRefs(t, db, 3, 4)
	if _, err := db.Exec(`UPDATE gallery_refs SET metadata_priority = 1 WHERE gallery_id = 3`); err != nil {
		t.Fatal(err)
	}
	var requests [][]int64
	s := batchService(t, db, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		ids := requestedIDs(t, req)
		requests = append(requests, ids)
		entries := make([]panda.Metadata, len(ids))
		for i, id := range ids {
			entries[i] = panda.Metadata{ID: id, Token: fmt.Sprintf("token%d", id)}
		}
		return metadataResponse(t, entries...), nil
	}))
	fetchJob(t, s, panda.GalleryRef{ID: 9, Token: "token9"})
	for range 4 {
		collectBatch(t, s)
	}
	if want := [][]int64{{9}, {4, 3}, {1}, {2}}; !reflect.DeepEqual(requests, want) {
		t.Fatalf("priority/order/shared work = %v, want %v", requests, want)
	}
	if got := readImport(t, imports, first.ID); got.Status != "completed" || got.Imported != 2 {
		t.Fatalf("explicit request did not satisfy bulk owner: %+v", got)
	}
	if got := readImport(t, imports, second.ID); got.Status != "completed" || got.Imported != 1 {
		t.Fatalf("second import = %+v", got)
	}
}

func TestReferenceImportReusesInventoryDiscoveredWhileQueued(t *testing.T) {
	db := openDB(t, t.TempDir())
	imports := batchImports(t, db, t.TempDir())
	item := acceptImport(t, imports, "1,token1\n2,conflict\n3,token3\n")
	parseImports(t, imports)
	seedRefs(t, db, 1, 2)
	var calls int
	s := batchService(t, db, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return metadataResponse(t, panda.Metadata{ID: 1, Error: "not found"}, panda.Metadata{ID: 2, Token: "token2"}), nil
		}
		return metadataResponse(t, panda.Metadata{ID: 3, Token: "token3"}), nil
	}))
	collectBatch(t, s)
	collectBatch(t, s)
	if got := readImport(t, imports, item.ID); got.Status != "completed" || got.Known != 1 || got.Failed != 1 || got.Imported != 1 {
		t.Fatalf("late inventory admission ignored reuse policy: %+v", got)
	}
}

func TestReferenceImportReusesRelatedDiscoveryFromSameBatch(t *testing.T) {
	db := openDB(t, t.TempDir())
	imports := batchImports(t, db, t.TempDir())
	item := acceptImport(t, imports, "1,token1\n2,token2\n")
	parseImports(t, imports)
	s := batchService(t, db, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		// Failure precedes the response that discovers its authoritative token.
		return metadataResponse(t, panda.Metadata{ID: 2, Error: "not found"},
			panda.Metadata{ID: 1, Token: "token1", ParentID: 2, ParentToken: "token2"}), nil
	}))
	collectBatch(t, s)
	if got := readImport(t, imports, item.ID); got.Status != "completed" || got.Imported != 1 || got.Known != 1 || got.Failed != 0 {
		t.Fatalf("related discovery did not satisfy inventory reuse: %+v", got)
	}
}

func TestReferenceImportValidationSharesPersistentRequestBackoff(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		dir := t.TempDir()
		db := openDB(t, dir)
		imports := batchImports(t, db, filepath.Join(dir, "imports"))
		item := acceptImport(t, imports, "1,token1\n")
		parseImports(t, imports)
		var requests int
		transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
			requests++
			if requests == 1 {
				return &http.Response{StatusCode: http.StatusTooManyRequests, Header: http.Header{"Retry-After": {"120"}}, Body: io.NopCloser(strings.NewReader("busy"))}, nil
			}
			return metadataResponse(t, panda.Metadata{ID: 1, Token: "token1"}), nil
		})
		worker := New(t.Context(), db, apiClient(t, transport), slog.New(slog.DiscardHandler))
		synctest.Wait()
		worker.Close()
		if got := readImport(t, imports, item.ID); got.Pending != 1 || got.Failed != 0 {
			t.Fatalf("temporary error became terminal: %+v", got)
		}
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
		db = openDB(t, dir)
		imports = batchImports(t, db, imports.dir)
		worker = New(t.Context(), db, apiClient(t, transport), slog.New(slog.DiscardHandler))
		defer worker.Close()
		time.Sleep(119 * time.Second)
		synctest.Wait()
		if requests != 1 {
			t.Fatalf("restart bypassed shared cooldown: %d requests", requests)
		}
		time.Sleep(time.Second)
		synctest.Wait()
		if got := readImport(t, imports, item.ID); requests != 2 || got.Status != "completed" || got.Imported != 1 {
			t.Fatalf("validation did not resume: %+v, requests=%d", got, requests)
		}
	})
}

func TestReferenceImportCancellationAndRetryPreserveOtherOwners(t *testing.T) {
	db := openDB(t, t.TempDir())
	imports := batchImports(t, db, t.TempDir())
	input := "1,token1\n2,token2\n"
	first := acceptImport(t, imports, input)
	second := acceptImport(t, imports, input)
	third := acceptImport(t, imports, input)
	parseImports(t, imports)
	var calls int
	s := batchService(t, db, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			// Cancelling an in-flight owner's work preserves the other owners.
			if _, err := imports.Cancel(t.Context(), first.ID); err != nil {
				t.Fatal(err)
			}
			return metadataResponse(t, panda.Metadata{ID: 1, Token: "token1"}, panda.Metadata{ID: 2, Error: "not found"}), nil
		}
		if ids := requestedIDs(t, req); !reflect.DeepEqual(ids, []int64{2}) {
			t.Fatalf("retry refreshed successful references: %v", ids)
		}
		return metadataResponse(t, panda.Metadata{ID: 2, Token: "token2"}), nil
	}))
	collectBatch(t, s)
	cancelled := readImport(t, imports, first.ID)
	unchanged := readImport(t, imports, third.ID)
	if cancelled.Status != "cancelled" || cancelled.Cancelled != 2 || cancelled.Imported != 0 ||
		unchanged.Status != "completed" || unchanged.Imported != 1 || unchanged.Failed != 1 {
		t.Fatalf("shared outcomes = cancelled %+v, third %+v", cancelled, unchanged)
	}
	retried, err := imports.Retry(t.Context(), second.ID)
	if err != nil || retried.ID != second.ID || retried.Status != "validating" || retried.Pending != 1 || retried.Imported != 1 || retried.Failed != 0 {
		t.Fatalf("retry = %+v, %v", retried, err)
	}
	// A deliberate re-upload shares the pending retry and reuses known success.
	fourth := acceptImport(t, imports, input)
	parseImports(t, imports)
	collectBatch(t, s)
	if got := readImport(t, imports, second.ID); got.Status != "completed" || got.Imported != 2 {
		t.Fatalf("retry outcome = %+v", got)
	}
	if got := readImport(t, imports, fourth.ID); got.Status != "completed" || got.Known != 1 || got.Imported != 1 {
		t.Fatalf("shared retry outcome = %+v", got)
	}
	if got := readImport(t, imports, third.ID); !reflect.DeepEqual(got, unchanged) {
		t.Fatalf("another owner's retry changed terminal outcomes: %+v", got)
	}
	if got := readImport(t, imports, first.ID); !reflect.DeepEqual(got, cancelled) {
		t.Fatalf("another owner's retry changed cancellation: %+v", got)
	}
	// Once a different import has confirmed it, retry uses inventory reuse.
	got, err := imports.Retry(t.Context(), third.ID)
	if err != nil || got.Status != "completed" || got.Known != 1 || got.Imported != 1 || calls != 2 {
		t.Fatalf("retry did not reuse confirmed inventory: %+v, %v, calls=%d", got, err, calls)
	}
}

func TestReferenceImportRetentionKeepsActiveWorkAndInventory(t *testing.T) {
	db := openDB(t, t.TempDir())
	imports := batchImports(t, db, t.TempDir())
	seedRefs(t, db, 1)
	completed := acceptImport(t, imports, "1,token1\n")
	active := acceptImport(t, imports, "2,token2\n")
	parseImports(t, imports)
	expired := time.Now().Add(-JobRetention - time.Second).UnixMilli()
	if _, err := db.Exec(`UPDATE reference_imports SET created_at = ?`, expired); err != nil {
		t.Fatal(err)
	}
	if got, err := imports.List(t.Context(), 100, 0); err != nil || len(got) != 2 {
		t.Fatalf("creation time expired recent completion/active work: %+v, %v", got, err)
	}
	if _, err := db.Exec(`UPDATE reference_imports SET completed_at = ? WHERE id = ?`, expired, completed.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := imports.Get(t.Context(), completed.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expired history readable: %v", err)
	}
	parseImports(t, imports)
	var entries int
	if err := db.QueryRow(`SELECT count(*) FROM reference_import_entries WHERE import_id = ?`, completed.ID).Scan(&entries); err != nil || entries != 0 {
		t.Fatalf("expired retry records retained: %d, %v", entries, err)
	}
	if got := readImport(t, imports, active.ID); got.Pending != 1 {
		t.Fatalf("active work expired: %+v", got)
	}
	var token string
	if err := db.QueryRow(`SELECT token FROM gallery_refs WHERE gallery_id = 1`).Scan(&token); err != nil || token != "token1" {
		t.Fatalf("history cleanup removed inventory: %q, %v", token, err)
	}
}

type repeatedImportByte byte

func (b repeatedImportByte) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = byte(b)
	}
	return len(p), nil
}

type failedImportReader struct{}

func (failedImportReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

func TestReferenceImportAcceptanceLimitsAndUnownedFileRecovery(t *testing.T) {
	db := openDB(t, t.TempDir())
	imports := batchImports(t, db, t.TempDir())
	if _, err := imports.Accept(t.Context(), "partial.txt", io.MultiReader(
		strings.NewReader("1,token1\n"), failedImportReader{},
	)); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("interrupted upload accepted: %v", err)
	}
	if _, err := imports.Accept(t.Context(), "too-big.txt", repeatedImportByte(' ')); !errors.Is(err, ErrImportTooLarge) {
		t.Fatalf("oversized upload accepted: %v", err)
	}
	if got, err := imports.List(t.Context(), 100, 0); err != nil || len(got) != 0 {
		t.Fatalf("unaccepted work persisted: %+v, %v", got, err)
	}
	if files, err := os.ReadDir(imports.dir); err != nil || len(files) != 0 {
		t.Fatalf("unaccepted uploads retained: %v, %v", files, err)
	}
	item, err := imports.Accept(t.Context(), "limit.txt", io.LimitReader(repeatedImportByte(' '), collectorapi.MaxReferenceImportBytes))
	if err != nil || item.SizeBytes != collectorapi.MaxReferenceImportBytes {
		t.Fatalf("maximum-size file rejected: %+v, %v", item, err)
	}
	if _, err := imports.Cancel(t.Context(), item.ID); err != nil {
		t.Fatal(err)
	}
	parseImports(t, imports)
	if refs, err := batchService(t, db, nil).store.pendingRefs(t.Context()); err != nil || len(refs) != 0 {
		t.Fatalf("cancelled upload scheduled validation: %v, %v", refs, err)
	}
	for _, name := range []string{"upload-interrupted.part", "unowned.txt"} {
		if err := os.WriteFile(filepath.Join(imports.dir, name), []byte("unfinished"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := imports.recover(t.Context()); err != nil {
		t.Fatal(err)
	}
	if files, err := os.ReadDir(imports.dir); err != nil || len(files) != 0 {
		t.Fatalf("unowned files not recovered: %v, %v", files, err)
	}
}

func TestReferenceImportWorkerRecoversAcceptedEmptyFile(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		db := openDB(t, t.TempDir())
		imports := batchImports(t, db, t.TempDir())
		item := acceptImport(t, imports, " \n\n")
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		worker, err := NewReferenceImports(ctx, db, imports.dir, slog.New(slog.DiscardHandler))
		if err != nil {
			t.Fatal(err)
		}
		defer worker.Close()
		synctest.Wait()
		if got := readImport(t, worker, item.ID); got.Status != "completed" || got.Invalid != 0 || got.References != 0 || got.ProcessedBytes != got.SizeBytes {
			t.Fatalf("empty import did not recover: %+v", got)
		}
	})
}
