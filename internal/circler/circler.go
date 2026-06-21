package circler

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	"log"
	"math"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/merlincox/wheeler/internal/tibetan"
	"github.com/tdewolff/canvas"
	"github.com/tdewolff/canvas/renderers/rasterizer"
)

type Circler struct {
	bg              color.Color
	fg              color.Color
	dpi             float64
	rpm             float64
	fps             float64
	padding         float64
	text            string
	fontFace        *canvas.FontFace
	verbose         bool
	cylinderifyData cylinderifyData
	paletteData     paletteData
}

// cylinderifyData can be used to cyclindrify images of the original bounds
type cylinderifyData struct {
	pixelMap map[int]int // maps x positions in new to x positions in old
	newWidth int
	height   int
	bounds   image.Rectangle
}

type rgbData struct {
	r uint8
	g uint8
	b uint8
}

type paletteData struct {
	rgbMap map[rgbData]color.Color
}

func (pd paletteData) toPaletteData() color.Palette {
	out := color.Palette{}
	for _, c := range pd.rgbMap {
		out = append(out, c)
	}
	return out
}

func New(dpi, rpm, fps, fontSize, padding float64, text, bgHex, fgHex, fontFilepath string, verbose bool) (*Circler, error) {
	if dpi <= 0.0 {
		return nil, fmt.Errorf("dots per inch must be greater than 0")
	}
	if fps <= 0.0 {
		return nil, fmt.Errorf("frames per second must be greater than 0")
	}
	if fontSize <= 0.0 {
		return nil, fmt.Errorf("font size must be greater than 0")
	}
	if padding < 0.0 {
		return nil, fmt.Errorf("padding must be greater than or equal to 0")
	}
	if text == "" {
		return nil, fmt.Errorf("text is required")
	}

	bg, err := parseHexColor(bgHex)
	if err != nil {
		return nil, err
	}
	fg, err := parseHexColor(fgHex)
	if err != nil {
		return nil, err
	}

	var fontFace *canvas.FontFace
	if fontFilepath == "" {
		fontFace, err = tibetan.GetFontFace(fontSize, fg)
		if err != nil {
			log.Printf("error loading font: %v", err)
			return nil, err
		}
	} else {
		fontFace, err = tibetan.GetFontFaceFromSource(filepath.Base(fontFilepath), fontFilepath, fontSize)
		if err != nil {
			log.Printf("error loading font: %v", err)
			return nil, err
		}
	}

	return &Circler{
		bg:       bg,
		fg:       fg,
		dpi:      dpi,
		rpm:      rpm,
		fps:      fps,
		padding:  padding,
		text:     text,
		verbose:  verbose,
		fontFace: fontFace,
	}, nil
}

func (c *Circler) Printf(format string, args ...any) {
	if c.verbose {
		log.Printf(format, args...)
	}
}

func (c *Circler) BuildGIFData() *gif.GIF {
	imageFromText := c.TextRGBAImage(strings.Repeat(c.text, 2))
	c.RGBAToPaletteData(imageFromText)

	traversal := imageFromText.Bounds().Dx() / 2
	height := imageFromText.Bounds().Dy()
	startOffset := traversal / 2
	visibleLen := traversal / 2

	secsPerRev := 60.0 / float64(c.rpm)
	framesPerRev := int(math.Round(secsPerRev * float64(c.fps)))
	movePerFrame := float64(traversal) / float64(framesPerRev)
	frameDelay := int(math.Round(100.0 / float64(c.fps))) // 100ths of a second

	images := make([]*image.Paletted, framesPerRev)
	delays := make([]int, framesPerRev)

	var move float64
	var imageRect image.Rectangle
	var subimage *image.RGBA
	var subImagePaletted, cylindified *image.Paletted

	c.Printf("Building GIF data with %d frames per revolution\n", framesPerRev)

	for i := 0; i < framesPerRev; i++ {
		move = movePerFrame * float64(i)
		imageRect = image.Rect(startOffset+int(math.Round(move)), 0, startOffset+visibleLen+int(math.Round(move)), height)
		subimage = imageFromText.SubImage(imageRect).(*image.RGBA)
		subImagePaletted = c.RGBAToPaletted(subimage)
		cylindified = c.Cyclindrify(subImagePaletted)
		images[i] = cylindified
		delays[i] = frameDelay

		if i%100 == 0 || i == framesPerRev-1 {
			c.Printf("Built frame %d of %d\n", i+1, framesPerRev)
		}
	}

	return &gif.GIF{
		Image:     images,
		Delay:     delays,
		LoopCount: 0, // loop forever
	}
}

func (c *Circler) TextRGBAImage(text string) *image.RGBA {
	textLine := canvas.NewTextLine(c.fontFace, text, canvas.Left)
	yPadding := textLine.Height * c.padding
	cvs := canvas.New(textLine.Width, textLine.Height)
	ctx := canvas.NewContext(cvs)
	ctx.SetFillColor(c.bg)
	ctx.DrawPath(0, 0, canvas.Rectangle(textLine.Width, textLine.Height))
	ctx.Fill()
	ctx.DrawText(0, yPadding, textLine)

	img := rasterizer.Draw(cvs, canvas.DPI(c.dpi), canvas.DefaultColorSpace)
	bounds := img.Bounds()
	frame := image.NewPaletted(bounds, c.palette())
	draw.FloydSteinberg.Draw(frame, bounds, img, image.Point{})

	return img
}

func (c *Circler) palette() color.Palette {
	return color.Palette{c.bg, c.fg}
}

func (c *Circler) RGBAToPaletteData(src *image.RGBA) {
	bounds := src.Bounds()
	c.paletteData.rgbMap = make(map[rgbData]color.Color)
	// collect all unique colours in the source
	var colorAtXY color.RGBA
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			colorAtXY = src.RGBAAt(x, y)
			rgb := rgbData{
				r: colorAtXY.R,
				g: colorAtXY.G,
				b: colorAtXY.B,
			}
			c.paletteData.rgbMap[rgb] = colorAtXY
		}
	}
	// for all unique colours in the source, calc whether the FG or BG is closest
	for rgb := range c.paletteData.rgbMap {
		bg := c.bg.(color.RGBA)
		fg := c.fg.(color.RGBA)
		bgDist := (rgb.r-bg.R)*(rgb.r-bg.R) + (rgb.g-bg.G)*(rgb.g-bg.G) + (rgb.b-bg.B)*(rgb.b-bg.B)
		fgDist := (rgb.r-fg.R)*(rgb.r-fg.R) + (rgb.g-fg.G)*(rgb.g-fg.G) + (rgb.b-fg.B)*(rgb.b-fg.B)

		c.paletteData.rgbMap[rgb] = c.bg
		if fgDist < bgDist {
			c.paletteData.rgbMap[rgb] = c.fg
		}
	}
	c.Printf("Mapped %d unique colours", len(c.paletteData.rgbMap))
}

// RGBAToPaletted converts an RGBA image to a 2-colour paletted image with min at 0,0.
func (c *Circler) RGBAToPaletted(src *image.RGBA) *image.Paletted {
	bounds := src.Bounds()
	xOffset := bounds.Min.X
	yOffset := bounds.Min.Y
	newBounds := image.Rect(0, 0, bounds.Max.X-xOffset, bounds.Max.Y-yOffset)
	dst := image.NewPaletted(newBounds, c.palette())
	var colorAtXY color.RGBA
	bgCount := 0
	fgCount := 0
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			colorAtXY = src.RGBAAt(x, y)
			rgb := rgbData{
				r: colorAtXY.R,
				g: colorAtXY.G,
				b: colorAtXY.B,
			}
			selectedCol, ok := c.paletteData.rgbMap[rgb]
			if !ok {
				c.Printf("missing colour in map")
			}
			if selectedCol == c.bg {
				bgCount++
			} else {
				fgCount++
			}
			dst.Set(x-xOffset, y-yOffset, selectedCol)
		}
	}
	c.Printf("bg pixels: %d, fg pixels: %d", bgCount, fgCount)
	return dst
}

// makeCyclindrifyData returns a cylinderifyData that can be used to cyclindrify images of the same bounds
func makeCyclindrifyData(img *image.Paletted) cylinderifyData {
	bounds := img.Bounds()
	oldWidthI := bounds.Dx()
	height := bounds.Dy()

	oldWidthF := float64(oldWidthI)
	newWidthF := oldWidthF * 2.0 / math.Pi
	newWidthI := int(math.Round(newWidthF))
	cyl := cylinderifyData{
		pixelMap: make(map[int]int, newWidthI),
		newWidth: newWidthI,
		height:   height,
		bounds:   bounds,
	}

	// Cylinder effect parameters
	radius := newWidthF / 2.0
	var newXF, radians, oldXF float64
	var oldXI, newXI, y int
	for newXI = 0; newXI < newWidthI; newXI++ {
		// Calculate the angle on the cylinder for the current x
		newXF = float64(newXI)
		radians = math.Acos((radius - newXF) / radius)
		oldXF = oldWidthF * radians / math.Pi
		oldXI = int(math.Round(oldXF))
		// Source Y remains the same
		if oldXI < 0 || oldXI >= oldWidthI {
			continue
		}
		for y = 0; y < height; y++ {
			cyl.pixelMap[newXI] = oldXI
		}
	}

	return cyl
}

func (c *Circler) Cyclindrify(img *image.Paletted) *image.Paletted {
	if c.cylinderifyData.pixelMap == nil || c.cylinderifyData.bounds != img.Bounds() {
		c.cylinderifyData = makeCyclindrifyData(img)
	}
	newRect := image.Rect(0, 0, c.cylinderifyData.newWidth, c.cylinderifyData.height)

	// Create a blank canvas for the output
	distorted := image.NewPaletted(newRect, img.Palette)
	for newXI := 0; newXI < c.cylinderifyData.newWidth; newXI++ {
		oldXI, ok := c.cylinderifyData.pixelMap[newXI]
		if !ok {
			continue
		}
		for y := 0; y < c.cylinderifyData.height; y++ {
			distorted.Set(newXI, y, img.At(oldXI, y))
		}
	}

	return distorted
}

func parseHexColor(s string) (color.RGBA, error) {
	if len(s) != 6 {
		return color.RGBA{}, fmt.Errorf("invalid hex color string '%s': must be 6 characters", s)
	}
	r, err := strconv.ParseUint(s[0:2], 16, 8)
	if err != nil {
		return color.RGBA{}, fmt.Errorf("invalid RED hex color element '%s': must be in range 00 to FF", s[0:2])
	}
	g, err := strconv.ParseUint(s[2:4], 16, 8)
	if err != nil {
		return color.RGBA{}, fmt.Errorf("invalid GREEN hex color element '%s': must be in range 00 to FF", s[2:4])
	}
	b, err := strconv.ParseUint(s[4:6], 16, 8)
	if err != nil {
		return color.RGBA{}, fmt.Errorf("invalid BLUE hex color element '%s': must be in range 00 to FF", s[4:6])
	}
	return color.RGBA{uint8(r), uint8(g), uint8(b), 255}, nil
}
