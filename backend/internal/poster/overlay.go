// Package poster provides image composition for sunset countdown poster overlays.
// It downloads original posters, composites a "Leaving in X days" banner at the
// bottom using the bundled Noto Sans font, and returns the modified image as JPEG bytes.
package poster

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	_ "image/png" // Register PNG decoder for posters that arrive as PNG
	"math"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"

	"capacitarr/assets/fonts"
)

// parsedFont caches the parsed Noto Sans Bold font. Parsed once on first use.
var (
	parsedFont     *opentype.Font
	parsedFontOnce sync.Once
	parsedFontErr  error
)

// BannerHeight is the fraction of the poster height used for the gradient banner.
const BannerHeight = 0.12

// ComposeOverlay renders a "Leaving in X days" countdown banner onto the bottom
// of a poster image. Returns the composited image as JPEG bytes.
//
// The banner is a gradient from transparent to semi-opaque black with white text
// rendered in Noto Sans Bold. Text varies by days remaining:
//   - 0: "Last day"
//   - 1: "Leaving tomorrow"
//   - N: "Leaving in N days"
func ComposeOverlay(original []byte, daysRemaining int) ([]byte, error) {
	src, _, err := image.Decode(bytes.NewReader(original))
	if err != nil {
		return nil, fmt.Errorf("decode poster image: %w", err)
	}

	bounds := src.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	if w < 10 || h < 10 {
		return nil, fmt.Errorf("poster too small: %dx%d", w, h)
	}

	// Create output image
	out := image.NewRGBA(bounds)
	draw.Draw(out, bounds, src, bounds.Min, draw.Src)

	// Draw gradient banner at the bottom
	bannerH := int(math.Round(float64(h) * BannerHeight))
	if bannerH < 20 {
		bannerH = 20
	}
	bannerTop := h - bannerH
	for y := bannerTop; y < h; y++ {
		progress := float64(y-bannerTop) / float64(bannerH) // 0.0 at top → 1.0 at bottom
		alpha := uint8(math.Round(progress * 200))          // Max 200/255 opacity (~78%)
		bannerColor := color.NRGBA{R: 0, G: 0, B: 0, A: alpha}
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			draw.Draw(out,
				image.Rect(x, y, x+1, y+1),
				&image.Uniform{C: bannerColor},
				image.Point{},
				draw.Over,
			)
		}
	}

	// Render countdown text
	text := countdownText(daysRemaining)
	if err := drawText(out, text, bounds.Min.X, bannerTop, w, bannerH); err != nil {
		return nil, fmt.Errorf("draw text: %w", err)
	}

	// Encode as JPEG
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, out, &jpeg.Options{Quality: 92}); err != nil {
		return nil, fmt.Errorf("encode poster JPEG: %w", err)
	}

	return buf.Bytes(), nil
}

// ContentHash returns a hex-encoded SHA-256 hash of the image data.
// Used to detect if a poster has been changed by the user since it was cached.
func ContentHash(data []byte) string {
	h := sha256.Sum256(data)
	return fmt.Sprintf("%x", h[:16]) // 32 hex chars (128 bits) — sufficient for dedup
}

// countdownText returns the human-readable countdown string.
func countdownText(daysRemaining int) string {
	switch {
	case daysRemaining <= 0:
		return "Last day"
	case daysRemaining == 1:
		return "Leaving tomorrow"
	default:
		return fmt.Sprintf("Leaving in %d days", daysRemaining)
	}
}

// loadFont parses the embedded Noto Sans Bold TTF data. Called once via sync.Once.
func loadFont() (*opentype.Font, error) {
	parsedFontOnce.Do(func() {
		parsedFont, parsedFontErr = opentype.Parse(fonts.NotoSansBold)
		if parsedFontErr != nil {
			parsedFontErr = fmt.Errorf("parse Noto Sans Bold: %w", parsedFontErr)
		}
	})
	return parsedFont, parsedFontErr
}

// drawText renders white text centered in the banner area using Noto Sans Bold.
// Font size is calculated dynamically based on banner height to scale with poster resolution.
func drawText(img *image.RGBA, text string, bannerX, bannerTop, bannerW, bannerH int) error {
	ft, err := loadFont()
	if err != nil {
		return err
	}

	// Scale font size to ~50% of banner height for readable text
	fontSize := float64(bannerH) * 0.5
	if fontSize < 10 {
		fontSize = 10
	}

	face, err := opentype.NewFace(ft, &opentype.FaceOptions{
		Size:    fontSize,
		DPI:     72,
		Hinting: font.HintingFull,
	})
	if err != nil {
		return fmt.Errorf("create font face: %w", err)
	}
	defer func() { _ = face.Close() }()

	metrics := face.Metrics()

	// Text dimensions
	textWidth := font.MeasureString(face, text).Ceil()
	textHeight := (metrics.Ascent + metrics.Descent).Ceil()

	// Center horizontally and vertically in the banner
	x := bannerX + (bannerW-textWidth)/2
	y := bannerTop + (bannerH+textHeight)/2

	// Draw text
	drawer := &font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(color.White),
		Face: face,
		Dot:  fixed.Point26_6{X: fixed.I(x), Y: fixed.I(y)},
	}
	drawer.DrawString(text)
	return nil
}
