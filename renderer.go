package memecreator

import (
	"image"
	"image/draw"

	"github.com/golang/freetype"
	"github.com/golang/freetype/truetype"
	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
)

// RenderMeme handles creating new meme image with text.
func RenderMeme(fontBytes []byte, src image.Image, top, bottom string) (draw.Image, error) {
	srcBounds := src.Bounds()
	newImage := image.NewNRGBA64(image.Rect(0, 0, srcBounds.Dx(), srcBounds.Dy()))
	draw.Draw(newImage, newImage.Bounds(), src, srcBounds.Min, draw.Src)
	dst := draw.Image(newImage)

	f, err := freetype.ParseFont(fontBytes)
	if err != nil {
		return nil, err
	}

	c := freetype.NewContext()
	c.SetDPI(72)
	c.SetFont(f)

	c.SetClip(dst.Bounds())
	c.SetDst(dst)
	c.SetSrc(image.White)
	c.SetHinting(font.HintingNone)

	// draw top text line
	topTextWidth, topFontSize := computeFontSize(f, top, srcBounds.Dx())
	c.SetFontSize(float64(topFontSize))

	topPt := freetype.Pt((srcBounds.Dx()-topTextWidth)/2.0, 15+int(c.PointToFixed(72)>>6))
	if _, err := c.DrawString(top, topPt); err != nil {
		return nil, err
	}

	// draw bottom text line
	bottomTextWidth, bottomFontSize := computeFontSize(f, bottom, srcBounds.Dx())
	c.SetFontSize(float64(bottomFontSize))

	bottomPt := freetype.Pt((srcBounds.Dx()-bottomTextWidth)/2.0, (src.Bounds().Dy()-100)+int(c.PointToFixed(72)>>6))
	if _, err := c.DrawString(bottom, bottomPt); err != nil {
		return nil, err
	}

	return dst, nil
}

// computeFontSize computes optimal size for text based on text length.
func computeFontSize(f *truetype.Font, text string, width int) (int, int) {
	for _, size := range []fixed.Int26_6{72, 48, 36, 24} {
		textWidth := fixed.Int26_6(0)

		for _, r := range text {
			textWidth += f.HMetric(size, f.Index(r)).AdvanceWidth
		}

		if textWidth <= fixed.Int26_6(width) {
			return int(textWidth), int(size)
		}
	}

	return 0, 24
}
