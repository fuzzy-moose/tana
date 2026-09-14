package collectorapi

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"testing"
)

func TestCatalogFactsBatchesAndValidatesCompleteResponses(t *testing.T) {
	client, err := NewClient("https://collector.test", "secret")
	if err != nil {
		t.Fatal(err)
	}
	var sizes []int
	incomplete := false
	client.http.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		var input struct {
			GalleryIDs []int64 `json:"gallery_ids"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil ||
			r.Method != http.MethodPost || r.URL.Path != "/api/catalog/facts" || r.Header.Get("Authorization") != "Bearer secret" {
			t.Fatalf("facts request = %s %s %+v, %v", r.Method, r.URL.Path, input, err)
		}
		sizes = append(sizes, len(input.GalleryIDs))
		facts := make([]CatalogFact, 0, len(input.GalleryIDs))
		for _, id := range input.GalleryIDs {
			facts = append(facts, CatalogFact{GalleryID: id})
		}
		if incomplete {
			facts = facts[:len(facts)-1]
		}
		body, _ := json.Marshal(facts)
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(body))}, nil
	})
	ids := make([]int64, MaxCatalogFactsSize+1)
	for i := range ids {
		ids[i] = int64(i + 1)
	}
	facts, err := client.CatalogFacts(t.Context(), append(ids, 1))
	if err != nil || len(facts) != len(ids) || !reflect.DeepEqual(sizes, []int{1000, 1}) {
		t.Fatalf("batch sizes = %v facts = %d, %v", sizes, len(facts), err)
	}
	incomplete = true
	if _, err := client.CatalogFacts(t.Context(), []int64{1, 2}); err == nil {
		t.Fatal("accepted incomplete catalog facts")
	}
}

func TestCatalogSendsIndependentDefaultFilters(t *testing.T) {
	client, err := NewClient("https://collector.test", "secret")
	if err != nil {
		t.Fatal(err)
	}
	client.http.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		query := r.URL.Query()
		if query.Get("q") != "~red ~blue" || query.Get("default_q") != "-l:japanese$" ||
			!reflect.DeepEqual(query["category"], []string{"manga", "doujinshi"}) ||
			!reflect.DeepEqual(query["default_category"], []string{"manga", "artist cg"}) {
			t.Fatalf("catalog filters = %v", query)
		}
		body, _ := json.Marshal(CatalogResult{Items: []CatalogItem{}, Page: 1, PageSize: 24, TotalPages: 1})
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(body))}, nil
	})
	_, err = client.Catalog(t.Context(), CatalogOptions{
		Query: "~red ~blue", DefaultQuery: "-l:japanese$", Page: 1, PageSize: 24,
		Categories: []string{"manga", "doujinshi"}, DefaultCategories: []string{"manga", "artist cg"},
	})
	if err != nil {
		t.Fatal(err)
	}
}
