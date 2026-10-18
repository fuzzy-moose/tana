package main

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"math/rand/v2"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"
)

type gallery struct {
	index    int
	name     string
	title    string
	tags     []string
	comments bool
	uploaded time.Time
	archive  bool
	color    int
	pages    []page
}

type page struct {
	file   string
	format string
	shape  shape
}

// tagCases are the tag lists of the six tag design cases. Galleries with the
// first, third, fourth and fifth case also get shared and unique tags.
var tagCases = []struct {
	label string
	tags  []string
}{
	{"grouped-tags", []string{
		"language:japanese", "artist:zeta studio", "all ages",
		"artist:alpha studio", "parody:paper moon", "language:english",
		"character:aki", "group:demo circle",
	}},
	{"no-tags", nil},
	{"many-tags", manyTags()},
	{"long-values", []string{
		"other:" + strings.Repeat("longtag", 20),
		"artist:a fictional studio with a deliberately long descriptive name",
		"theme:a long tag value with spaces that can wrap onto several lines",
	}},
	{"custom-namespaces", []string{
		"mood:quiet", "setting:moon station", "custom:alpha studio",
		"artist:alpha studio", "all ages", "multi-work series",
	}},
	{"single-tag", []string{"language:english"}},
}

func manyTags() []string {
	var tags []string
	for _, namespace := range []string{"theme", "character", "artist"} {
		for number := 20; number > 0; number-- {
			tags = append(tags, fmt.Sprintf("%s:sample tag %02d", namespace, number))
		}
	}
	return tags
}

var (
	// sharedTags have common prefixes, so tag completion lists several matches.
	sharedTags = []string{
		"theme:comedy", "theme:romance", "theme:school", "theme:school life",
		"theme:schoolyard", "artist:alpha team", "artist:alphabet works",
		"group:paper lantern", "language:english", "language:japanese",
	}
	shortTitles = []string{"Hush", "Ember", "Kite", "Moth", "Echo", "Drift", "Halo", "Fern"}
	adjectives  = []string{"Quiet", "Crimson", "Lost", "Endless", "Paper", "Silver", "Midnight", "Gentle"}
	nouns       = []string{"Lighthouse", "Garden", "Letters", "Station", "Festival", "Harbor", "Library", "Comet"}
	places      = []string{"the Sea", "Kyoto", "the Old Town", "the Last Train", "the Moon", "Summer"}
	subtitles   = []string{
		"A Story About Two Friends Who Never Quite Said What They Meant",
		"Collected Chapters, Bonus Pages and Afterword Included in This Edition",
		"Volume One of an Unexpectedly Long Series With an Even Longer Name",
	}
	unicodeTitles = []string{
		"夏の終わりの約束", "星降る夜のカフェ", "푸른 바다의 노래", "雨宿りの図書館で",
		"Café Ümlaut — Ñandú über alles", "Ангел в метро", "Ελληνικό καλοκαίρι", "Mỹ Tho ★ Sunset",
	}
	circles   = []string{"Moonlit Press", "Studio Kumo", "Paper Lantern", "Night Owl"}
	artists   = []string{"Aoi Haru", "Ren Takeda", "Mio", "K. Sato"}
	parodies  = []string{"Original", "Paper Moon", "Star Courier", "Harbor Lights"}
	languages = []string{"English", "Japanese", "Digital", "English, Digital"}
	numbers   = []int{1, 2, 3, 9, 10, 11, 20, 100, 101}
	syllables = []string{"a", "ka", "mi", "no", "ri", "su", "to", "ha", "yu", "ze", "ko", "ra"}
)

func generate(ctx context.Context, library string, count int) (int, error) {
	galleries := plan(count)
	var jobs []renderJob
	for _, g := range galleries {
		directory := filepath.Join(library, g.name)
		if err := os.Mkdir(directory, 0o755); err != nil {
			return 0, err
		}
		if err := os.WriteFile(filepath.Join(directory, "galleryinfo.txt"), []byte(metadata(g)), 0o644); err != nil {
			return 0, err
		}
		for number, p := range g.pages {
			jobs = append(jobs, renderJob{path: filepath.Join(directory, p.file), gallery: g, number: number + 1, page: p})
		}
	}
	if err := renderAll(ctx, jobs); err != nil {
		return 0, err
	}
	for _, g := range galleries {
		if g.archive {
			if err := pack(library, g.name); err != nil {
				return 0, err
			}
		}
	}
	return len(jobs), nil
}

// plan describes the galleries. Cases cycle with different periods, so the
// default count combines each title, tag, cover and reader case several times.
func plan(count int) []gallery {
	rng := rand.New(rand.NewPCG(2026, 1))
	seenTitles := map[string]int{}
	seenTags := map[string]bool{}
	galleries := make([]gallery, count)
	for i := range galleries {
		tagCase := tagCases[(i/6)%len(tagCases)]
		g := gallery{
			index:    i,
			name:     fmt.Sprintf("%03d-%s", i+1, tagCase.label),
			title:    title(rng, i%6),
			tags:     slices.Clone(tagCase.tags),
			comments: i%3 != 2,
			uploaded: time.Date(2019, 1, 1, 0, 0, 0, 0, time.UTC).Add(time.Duration(rng.IntN(7*365*24)) * time.Hour),
			archive:  rng.IntN(2) == 1,
			color:    i,
		}
		if n := seenTitles[g.title]; n > 0 {
			g.title = fmt.Sprintf("%s %d", g.title, n+1)
		}
		seenTitles[g.title]++
		if len(g.tags) > 1 {
			g.tags = append(g.tags, sharedTags[rng.IntN(len(sharedTags))], sharedTags[rng.IntN(len(sharedTags))])
			g.tags = append(g.tags, "character:"+uniqueName(rng, seenTags))
			slices.Sort(g.tags)
			g.tags = slices.Compact(g.tags)
		}
		g.pages = pages(rng, i)
		galleries[i] = g
	}
	return galleries
}

func title(rng *rand.Rand, kind int) string {
	pick := func(values []string) string { return values[rng.IntN(len(values))] }
	typical := func() string { return pick(adjectives) + " " + pick(nouns) + " of " + pick(places) }
	switch kind {
	case 0:
		return pick(shortTitles)
	case 1:
		return typical()
	case 2:
		return typical() + ": " + pick(subtitles) + " — " + pick(subtitles)
	case 3:
		return pick(unicodeTitles)
	case 4:
		return fmt.Sprintf("[%s (%s)] %s (%s) [%s]", pick(circles), pick(artists), typical(), pick(parodies), pick(languages))
	default:
		return fmt.Sprintf("%d %s", numbers[rng.IntN(len(numbers))], pick(nouns))
	}
}

func uniqueName(rng *rand.Rand, seen map[string]bool) string {
	for {
		name := ""
		for range 3 + rng.IntN(2) {
			name += syllables[rng.IntN(len(syllables))]
		}
		if !seen[name] {
			seen[name] = true
			return name
		}
	}
}

var archiveTime = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

var (
	pageCounts = []int{24, 1, 27, 2, 21, 7, 30, 13, 22, 25}
	formats    = []string{"jpg", "jpg", "jpg", "png", "png", "gif"}
)

// pages chooses the reader case from i: portrait pages only, portrait pages
// with landscape spreads, a tall strip series, or mixed proportions and formats.
// Galleries with spreads or mixed pages start with a cover of the cover case.
func pages(rng *rand.Rand, i int) []page {
	count := pageCounts[i%len(pageCounts)]
	if i%40 == 39 {
		count = 200 + i%50
	}
	layout := i % 4
	format := formats[rng.IntN(len(formats))]
	// Unpadded file names check the natural sort order of pages.
	padded := rng.IntN(3) != 0
	result := make([]page, count)
	for n := range result {
		p := page{format: format, shape: portrait}
		switch layout {
		case 1:
			if (n+1)%4 == 0 {
				p.shape = spread
			}
		case 2:
			p.shape = shape{strip.name, strip.w, 2400 + rng.IntN(1600)}
		case 3:
			p.shape = mixedShapes[rng.IntN(len(mixedShapes))]
			p.format = formats[rng.IntN(len(formats))]
		}
		if n == 0 && layout != 0 && layout != 2 {
			p.shape = covers[i%len(covers)]
		}
		if padded {
			p.file = fmt.Sprintf("%03d.%s", n+1, p.format)
		} else {
			p.file = fmt.Sprintf("%d.%s", n+1, p.format)
		}
		result[n] = p
	}
	return result
}

func metadata(g gallery) string {
	lines := []string{
		"Title:       " + g.title,
		"Upload Time: " + g.uploaded.Format("2006-01-02 15:04"),
		"Uploaded By: tana-mock",
		"Downloaded:  " + g.uploaded.Add(time.Duration(1+g.index%90)*24*time.Hour).Format("2006-01-02 15:04"),
	}
	// An omitted Tags header represents no tags; an empty header is malformed.
	if len(g.tags) > 0 {
		lines = append(lines, "Tags:        "+strings.Join(g.tags, ", "))
	}
	if g.comments {
		lines = append(lines, "", "Uploader's Comments:", "",
			"Mock gallery for local UI checks. All names and metadata are fake.", "",
			"More pages: https://example.com/mock/"+g.name)
	}
	lines = append(lines, "", "Generated by Tana's mock gallery command.", "")
	return strings.Join(lines, "\n")
}

type renderJob struct {
	path    string
	gallery gallery
	number  int
	page    page
}

func renderAll(ctx context.Context, jobs []renderJob) error {
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	queue := make(chan renderJob)
	var wg sync.WaitGroup
	for range runtime.NumCPU() {
		wg.Go(func() {
			for job := range queue {
				if err := render(job); err != nil {
					cancel(fmt.Errorf("render %s: %w", job.path, err))
				}
			}
		})
	}
send:
	for _, job := range jobs {
		select {
		case queue <- job:
		case <-ctx.Done():
			break send
		}
	}
	close(queue)
	wg.Wait()
	return context.Cause(ctx)
}

// pack replaces a gallery directory with a .cbz archive. Entries stay in a
// subfolder that has the gallery name.
func pack(library, name string) error {
	directory := filepath.Join(library, name)
	entries, err := os.ReadDir(directory)
	if err != nil {
		return err
	}
	file, err := os.Create(directory + ".cbz")
	if err != nil {
		return err
	}
	defer file.Close()
	archive := zip.NewWriter(file)
	for _, entry := range entries {
		writer, err := archive.CreateHeader(&zip.FileHeader{Name: name + "/" + entry.Name(), Method: zip.Store, Modified: archiveTime})
		if err != nil {
			return err
		}
		source, err := os.Open(filepath.Join(directory, entry.Name()))
		if err != nil {
			return err
		}
		_, err = io.Copy(writer, source)
		source.Close()
		if err != nil {
			return err
		}
	}
	if err := archive.Close(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.RemoveAll(directory)
}
