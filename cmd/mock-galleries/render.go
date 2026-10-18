package main

import (
	"bufio"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	"image/jpeg"
	"image/png"
	"os"
	"strings"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

type shape struct {
	name string
	w, h int
}

var (
	portrait = shape{"Portrait page", 800, 1200}
	spread   = shape{"Landscape spread", 1600, 1200}
	square   = shape{"Square page", 1000, 1000}
	strip    = shape{"Tall strip", 800, 3600}
	panorama = shape{"Wide panorama", 2400, 700}
	small    = shape{"Small page", 240, 360}
	large    = shape{"Large page", 1800, 2700}

	covers      = []shape{portrait, spread, square, strip, panorama, small, large}
	mixedShapes = []shape{portrait, portrait, spread, square, strip, panorama, small}
)

var (
	regular = mustParse(goregular.TTF)
	bold    = mustParse(gobold.TTF)
	palette = []color.RGBA{
		{0x40, 0x5c, 0x70, 0xff}, {0x73, 0x51, 0x6b, 0xff}, {0x4c, 0x6a, 0x51, 0xff},
		{0x85, 0x5c, 0x3c, 0xff}, {0x61, 0x5a, 0x89, 0xff}, {0x39, 0x6c, 0x70, 0xff},
		{0x8a, 0x4b, 0x4b, 0xff}, {0x5a, 0x63, 0x6e, 0xff},
	}
)

func mustParse(data []byte) *opentype.Font {
	f, err := opentype.Parse(data)
	if err != nil {
		panic(err)
	}
	return f
}

// render draws a page with the gallery title, the page number and the page
// shape, so a person can check the page order in the reader.
func render(job renderJob) error {
	w, h := job.page.shape.w, job.page.shape.h
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	background := palette[job.gallery.color%len(palette)]
	draw.Draw(img, img.Bounds(), image.NewUniform(lighten(background)), image.Point{}, draw.Src)
	inset := max(min(w, h)/40, 2)
	draw.Draw(img, img.Bounds().Inset(inset), image.NewUniform(background), image.Point{}, draw.Src)

	side := min(w, h)
	textSize := max(float64(side)/18, 12)
	titleFace, err := face(regular, textSize)
	if err != nil {
		return err
	}
	numberFace, err := face(bold, float64(side)/5)
	if err != nil {
		return err
	}
	number := fmt.Sprintf("%d / %d", job.number, len(job.gallery.pages))
	var lines []line
	for _, text := range wrap(titleFace, job.gallery.title, fixed.I(w*85/100), 4) {
		lines = append(lines, line{titleFace, text})
	}
	lines = append(lines, line{titleFace, ""}, line{numberFace, number}, line{titleFace, ""},
		line{titleFace, fmt.Sprintf("%s · %d×%d", job.page.shape.name, w, h)})
	height := 0
	for _, l := range lines {
		height += l.face.Metrics().Height.Ceil()
	}
	drawLines(img, lines, (h-height)/2)
	// Tall pages also show the number at both ends, which are visible while
	// a person scrolls.
	if h > 2*w {
		end := []line{{titleFace, number}}
		drawLines(img, end, inset*2)
		drawLines(img, end, h-inset*2-titleFace.Metrics().Height.Ceil())
	}
	if job.page.format == "gif" {
		// A palette of the page colors avoids slow quantization and dithering.
		var colors color.Palette
		for _, base := range []color.RGBA{background, lighten(background)} {
			for step := range 16 {
				colors = append(colors, mix(base, step*255/15))
			}
		}
		paletted := image.NewPaletted(img.Bounds(), colors)
		draw.Draw(paletted, img.Bounds(), img, image.Point{}, draw.Src)
		return save(job.path, job.page.format, paletted)
	}
	return save(job.path, job.page.format, img)
}

// mix blends c with white; weight 255 gives white.
func mix(c color.RGBA, weight int) color.RGBA {
	blend := func(v uint8) uint8 { return uint8((int(v)*(255-weight) + 255*weight) / 255) }
	return color.RGBA{blend(c.R), blend(c.G), blend(c.B), 0xff}
}

type line struct {
	face font.Face
	text string
}

func face(f *opentype.Font, size float64) (font.Face, error) {
	return opentype.NewFace(f, &opentype.FaceOptions{Size: size, DPI: 72, Hinting: font.HintingFull})
}

func drawLines(img *image.RGBA, lines []line, top int) {
	drawer := font.Drawer{Dst: img, Src: image.White}
	y := fixed.I(top)
	for _, l := range lines {
		metrics := l.face.Metrics()
		drawer.Face = l.face
		width := drawer.MeasureString(l.text)
		drawer.Dot = fixed.Point26_6{X: (fixed.I(img.Bounds().Dx()) - width) / 2, Y: y + metrics.Ascent}
		drawer.DrawString(l.text)
		y += metrics.Height
	}
}

// wrap breaks text into at most limit lines no wider than width. It breaks
// at spaces where it can and between runes elsewhere, for example in CJK text.
func wrap(f font.Face, text string, width fixed.Int26_6, limit int) []string {
	var lines []string
	current := ""
	for _, word := range strings.Fields(text) {
		candidate := word
		if current != "" {
			candidate = current + " " + word
		}
		if font.MeasureString(f, candidate) <= width {
			current = candidate
			continue
		}
		if current != "" {
			lines = append(lines, current)
		}
		current = ""
		for _, r := range word {
			if current != "" && font.MeasureString(f, current+string(r)) > width {
				lines = append(lines, current)
				current = ""
			}
			current += string(r)
		}
	}
	if current != "" {
		lines = append(lines, current)
	}
	if len(lines) > limit {
		lines = append(lines[:limit-1], lines[limit-1]+" …")
	}
	return lines
}

func lighten(c color.RGBA) color.RGBA {
	return mix(c, 128)
}

func save(path, format string, img image.Image) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	writer := bufio.NewWriter(file)
	switch format {
	case "png":
		err = (&png.Encoder{CompressionLevel: png.BestSpeed}).Encode(writer, img)
	case "gif":
		err = gif.Encode(writer, img, nil)
	default:
		err = jpeg.Encode(writer, img, &jpeg.Options{Quality: 85})
	}
	if err != nil {
		return err
	}
	if err := writer.Flush(); err != nil {
		return err
	}
	return file.Close()
}
