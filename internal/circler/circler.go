package circler

import (
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"log"
	"math"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/merlincox/wheeler/internal/tibetan"
	"github.com/tdewolff/canvas"
	"github.com/tdewolff/canvas/renderers/rasterizer"
)

type stretchingMode int

const (
	noStretching stretchingMode = iota
	xStretching
	yStretching
)

type Circler struct {
	bg       color.RGBA
	fg       color.RGBA
	dpi      float64
	rpm      float64
	fps      float64
	padding  float64
	text     string
	fontFace *canvas.FontFace
	verbose  bool
	routines int
	ratio    float64

	projector  *projector
	colourData colourData
	mutex      sync.Mutex
}

// projector can be used to project old images to projected images
type projector struct {
	xMap           map[int]int    // maps x positions in the projected image to x positions in the original image
	yMap           map[int]int    // maps y positions in the projected image to y positions in the original image
	width          int            // width of projected image
	height         int            // height of projected image
	stretchingMode stretchingMode // aspect ratio stretching: X, Y or none
}

func (d projector) rect() image.Rectangle {
	return image.Rect(
		0,
		0,
		d.width,
		d.height,
	)
}

func (d projector) originalX(x int) int {
	return d.xMap[x]
}

func (d projector) originalY(y int) int {
	if d.stretchingMode != yStretching {
		return y
	}
	return d.yMap[y]
}

// rgbData is an RGB colour with implied alpha = 255
type rgbData struct {
	r uint8
	g uint8
	b uint8
}

func fromRGBA(colour color.RGBA) rgbData {
	return rgbData{
		r: colour.R,
		g: colour.G,
		b: colour.B,
	}
}

// sqDist is the squared distance to an RGBA colour
func (rgb rgbData) sqDist(colour color.RGBA) uint32 {
	// uint8 cast to uint32 values to avoid overflow
	return (uint32(rgb.r-colour.R))*(uint32(rgb.r-colour.R)) + (uint32(rgb.g-colour.G))*(uint32(rgb.g-colour.G)) + (uint32(rgb.b-colour.B))*(uint32(rgb.b-colour.B))
}

func (rgb rgbData) RGBA() color.RGBA {
	return color.RGBA{
		R: rgb.r,
		G: rgb.g,
		B: rgb.b,
		A: 255,
	}
}

type rgbSqDist struct {
	bgSqDist uint32
	fgSqDist uint32
	rgb      rgbData
}

type rgbSqDists []rgbSqDist

func (rgbds rgbSqDists) Len() int      { return len(rgbds) }
func (rgbds rgbSqDists) Swap(i, j int) { rgbds[i], rgbds[j] = rgbds[j], rgbds[i] }
func (rgbds rgbSqDists) Less(i, j int) bool {
	if rgbds[i].bgSqDist == rgbds[j].bgSqDist {
		return rgbds[i].fgSqDist > rgbds[j].fgSqDist
	}
	return rgbds[i].bgSqDist < rgbds[j].bgSqDist
}

type colourData struct {
	rgbMap  map[rgbData]color.RGBA
	palette color.Palette
}

func New(dpi, rpm, fps, fontSize, padding float64, text, bgHex, fgHex, fontFilepath string, verbose bool, routines int, ratio float64) (*Circler, error) {
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

	bg, err := parseHexColour(bgHex)
	if err != nil {
		return nil, err
	}
	fg, err := parseHexColour(fgHex)
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
		routines: routines,
		ratio:    ratio,
	}, nil
}

func (c *Circler) Printf(format string, args ...any) {
	if c.verbose {
		log.Printf(format, args...)
	}
}

// palette generates a palette and as necessary remaps the rgbMap to its colours
func (c *Circler) palette() color.Palette {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.colourData.palette != nil {
		return c.colourData.palette
	}
	if len(c.colourData.rgbMap) <= 256 {
		// all the colours will fit in a palette suitable for a GIF
		c.colourData.palette = make(color.Palette, 0, len(c.colourData.rgbMap))
		// put bg in zero position
		c.colourData.palette = append(c.colourData.palette, c.bg)
		for _, colour := range c.colourData.rgbMap {
			if colour != c.bg {
				c.colourData.palette = append(c.colourData.palette, colour)
			}
		}
		return c.colourData.palette
	}

	sqDists := make(rgbSqDists, len(c.colourData.rgbMap))
	var bgDist, fgDist uint32

	// for all unique colours in the source, calc squared distance to fg and bg
	i := 0
	for rgb := range c.colourData.rgbMap {
		bgDist = rgb.sqDist(c.bg)
		fgDist = rgb.sqDist(c.fg)

		sqDists[i] = rgbSqDist{
			rgb:      rgb,
			fgSqDist: fgDist,
			bgSqDist: bgDist,
		}
		i++
	}
	// sort by distance to bg
	sort.Sort(sqDists)

	correction := float64(256) / float64(len(sqDists))
	var corrected int
	for i := 0; i < len(sqDists); i++ {
		corrected = round(float64(i) * correction)
		if corrected != i {
			c.colourData.rgbMap[sqDists[i].rgb] = sqDists[corrected].rgb.RGBA()
		}
	}
	colourIndex := make(map[color.RGBA]struct{}, 256)
	for _, colour := range c.colourData.rgbMap {
		colourIndex[colour] = struct{}{}
	}
	c.colourData.palette = make(color.Palette, 0, 256)
	// put bg in zero position
	c.colourData.palette = append(c.colourData.palette, c.bg)
	for colour := range colourIndex {
		if colour != c.bg {
			c.colourData.palette = append(c.colourData.palette, colour)
		}
	}
	c.Printf("Converted to 256 colour palette\n")
	return c.colourData.palette
}

func (c *Circler) BuildGIFData() *gif.GIF {
	c.Printf("Building GIF data using %d goroutines\n", c.routines)
	now := time.Now()
	// textImage has the text twice so the visible text can move across the end-start boundry
	textImage := c.createTextImage(strings.Repeat(c.text, 2))
	c.Printf("Full text image size: (%d, %d)\n", textImage.Bounds().Dx(), textImage.Bounds().Dy())
	c.readColourData(textImage)

	traversal := textImage.Bounds().Dx() / 2
	height := textImage.Bounds().Dy()
	startOffset := traversal / 2
	visibleLen := traversal / 2

	secsPerRotation := 60.0 / float64(c.rpm)
	framesPerRotation := round(secsPerRotation * c.fps)
	advancePerFrame := float64(traversal) / float64(framesPerRotation)
	frameDelay := round(100.0 / c.fps) // 100ths of a second

	images := make([]*image.Paletted, framesPerRotation)
	delays := make([]int, framesPerRotation)

	c.Printf("GIF data requires %d frames per rotation\n", framesPerRotation)

	semaphore := make(chan struct{}, c.routines)

	var wg sync.WaitGroup

	for j := 0; j < framesPerRotation; j++ {
		wg.Add(1)

		// This blocks if the semaphore channel is full
		semaphore <- struct{}{}

		go func(i int) {
			defer wg.Done()
			// Release the slot when the goroutine finishes
			defer func() { <-semaphore }()

			advance := advancePerFrame * float64(i)
			visibleRect := image.Rect(startOffset+round(advance), 0, startOffset+visibleLen+round(advance), height)
			visibleText := textImage.SubImage(visibleRect).(*image.RGBA)
			palettedVisibleText := c.convertToPaletted(visibleText)
			projected := c.project(palettedVisibleText)
			images[i] = projected
			delays[i] = frameDelay

			if i%100 == 0 || i == framesPerRotation-1 {
				c.Printf("Built frame %d of %d\n", i+1, framesPerRotation)
			}

		}(j)

	}
	wg.Wait()
	gifData := &gif.GIF{
		Image:     images,
		Delay:     delays,
		LoopCount: 0, // loop forever
	}
	c.Printf("Built GIF in %.2f seconds\n", time.Since(now).Seconds())
	return gifData
}

func (c *Circler) createTextImage(text string) *image.RGBA {
	textLine := canvas.NewTextLine(c.fontFace, text, canvas.Left)
	yPadding := textLine.Height * c.padding
	cvs := canvas.New(textLine.Width, textLine.Height)
	ctx := canvas.NewContext(cvs)
	ctx.SetFillColor(c.bg)
	ctx.SetStrokeColor(c.bg)
	ctx.DrawPath(0, 0, canvas.Rectangle(textLine.Width, textLine.Height))
	ctx.Fill()
	ctx.DrawText(0, yPadding, textLine)

	return rasterizer.Draw(cvs, canvas.DPI(c.dpi), canvas.DefaultColorSpace)
}

func (c *Circler) readColourData(src *image.RGBA) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	bounds := src.Bounds()
	c.colourData.rgbMap = make(map[rgbData]color.RGBA)
	// collect all unique colours in the source
	var colourAtXY color.RGBA
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			colourAtXY = src.RGBAAt(x, y)
			rgb := fromRGBA(colourAtXY)
			c.colourData.rgbMap[rgb] = colourAtXY
		}
	}

	c.Printf("Found %d unique colours", len(c.colourData.rgbMap))
}

// convertToPaletted converts an RGBA image to a paletted image with its min at 0,0.
func (c *Circler) convertToPaletted(src *image.RGBA) *image.Paletted {
	bounds := src.Bounds()
	xOffset := bounds.Min.X
	yOffset := bounds.Min.Y
	newBounds := image.Rect(0, 0, bounds.Max.X-xOffset, bounds.Max.Y-yOffset)
	// calling c.palette() for the first time maps the existing RGBA colours into a max 256-colour palette
	dst := image.NewPaletted(newBounds, c.palette())
	var colourAtXY color.RGBA
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			colourAtXY = src.RGBAAt(x, y)
			rgb := rgbData{
				r: colourAtXY.R,
				g: colourAtXY.G,
				b: colourAtXY.B,
			}
			selectedCol, ok := c.colourData.rgbMap[rgb]
			if !ok {
				c.Printf("missing colour in map")
			}
			dst.Set(x-xOffset, y-yOffset, selectedCol)
		}
	}
	return dst
}

func round(f float64) int {
	return int(math.Round(f))
}

// buildProjector builds the projector which can project successive images with the same bounds
// this handles both cylinder projection and aspect ratio stretching
func (c *Circler) buildProjector(img *image.Paletted) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	if c.projector != nil {
		return
	}

	bounds := img.Bounds()

	originalWidth := bounds.Dx()
	originalHeight := bounds.Dy()
	originalWidthF := float64(originalWidth)
	originalHeightF := float64(originalHeight)

	// apply cylinder conversion but not stretching (yet)
	unstretchedProjectedWidthF := originalWidthF * 2.0 / math.Pi
	projectedWidthF := unstretchedProjectedWidthF

	projectedHeightF := originalHeightF
	c.Printf("Unstretched projection dimensions: (%d, %d)\n", round(unstretchedProjectedWidthF), originalHeight)

	cylinderRatio := unstretchedProjectedWidthF / originalHeightF
	c.Printf("Unstretched projection ratio: %.2f\n", cylinderRatio)
	var mode stretchingMode
	if c.ratio != 0.0 && c.ratio != cylinderRatio {
		c.Printf("Desired aspect ratio: %.2f\n", c.ratio)
		// c.ratio == desired aspect ratio of width to height
		if c.ratio < cylinderRatio {
			projectedHeightF = unstretchedProjectedWidthF / c.ratio
			c.Printf("Needs Y stretching to height: %.2f\n", projectedHeightF)

			mode = yStretching
		}
		if c.ratio > cylinderRatio {
			projectedWidthF = originalHeightF * c.ratio
			c.Printf("Needs X stretching to width: %.2f\n", projectedWidthF)

			mode = xStretching
		}
	}
	c.projector = &projector{
		height:         round(projectedHeightF),
		width:          round(projectedWidthF),
		stretchingMode: mode,
	}

	c.projector.xMap = make(map[int]int, c.projector.width)

	if c.projector.stretchingMode == yStretching {
		c.projector.yMap = make(map[int]int, c.projector.height)
	}

	radius := unstretchedProjectedWidthF / 2.0

	var projectedXF, angle, originalXF float64
	var originalX, x, y int

	for x = 0; x < c.projector.width; x++ {
		// Calculate the angle on the cylinder for the current x
		projectedXF = float64(x)
		// reversing the aspect ratio stretching (if any)
		if c.projector.stretchingMode == xStretching {
			projectedXF = projectedXF * unstretchedProjectedWidthF / projectedWidthF
		}
		angle = math.Acos((radius - projectedXF) / radius)
		originalXF = originalWidthF * angle / math.Pi
		originalX = round(originalXF)
		if originalX < 0 || originalX >= originalWidth {
			continue
		}
		c.projector.xMap[x] = originalX
	}

	if c.projector.stretchingMode == yStretching {
		for y = 0; y < c.projector.height; y++ {
			c.projector.yMap[y] = round(float64(y) * originalHeightF / projectedHeightF)
		}
	}
	c.Printf("Projector created: (%d, %d)\n", c.projector.rect().Dx(), c.projector.rect().Dy())
}

func (c *Circler) project(img *image.Paletted) *image.Paletted {
	c.buildProjector(img)
	newRect := c.projector.rect()

	// Create a blank canvas for the projected output
	// Palette must have bg at index 0
	projected := image.NewPaletted(newRect, img.Palette)

	// fill canvas with colours mapped from the old image
	var oldX, y, oldY int
	for x := 0; x < c.projector.width; x++ {
		oldX = c.projector.originalX(x)
		for y = 0; y < c.projector.height; y++ {
			oldY = c.projector.originalY(y)
			projected.Set(x, y, img.At(oldX, oldY))
		}
	}

	return projected
}

func parseHexColour(s string) (color.RGBA, error) {
	if len(s) != 6 {
		return color.RGBA{}, fmt.Errorf("invalid hex colour string '%s': must be 6 characters", s)
	}
	r, err := strconv.ParseUint(s[0:2], 16, 8)
	if err != nil {
		return color.RGBA{}, fmt.Errorf("invalid RED hex colour element '%s': must be in range 00 to FF", s[0:2])
	}
	g, err := strconv.ParseUint(s[2:4], 16, 8)
	if err != nil {
		return color.RGBA{}, fmt.Errorf("invalid GREEN hex colour element '%s': must be in range 00 to FF", s[2:4])
	}
	b, err := strconv.ParseUint(s[4:6], 16, 8)
	if err != nil {
		return color.RGBA{}, fmt.Errorf("invalid BLUE hex colour element '%s': must be in range 00 to FF", s[4:6])
	}
	return color.RGBA{R: uint8(r), G: uint8(g), B: uint8(b), A: 255}, nil
}
