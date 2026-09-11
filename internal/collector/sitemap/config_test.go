package sitemap

import "testing"

func TestConfigKeepsCompleteURL(t *testing.T) {
	want := "https://example.invalid/custom/index.xml?generation=4"
	cfg, err := LoadConfig(func(string) string { return want })
	if err != nil || cfg.URL != want {
		t.Fatalf("config = %+v, %v", cfg, err)
	}
	cfg, err = LoadConfig(func(string) string { return "" })
	if err != nil || cfg.URL != "https://xml.e-hentai.org/sitemap_index.xml" {
		t.Fatalf("default config = %+v, %v", cfg, err)
	}
}
