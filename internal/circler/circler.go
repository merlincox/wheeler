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
	debug    bool
	routines int
	ratio    float64

	cylinderData *cylinderData
	colourData   colourData
	mutex        sync.Mutex
}

// cylinderData can be used to cyclindrify images of the original bounds
type cylinderData struct {
	xMap   map[int]int // maps x positions in new image to x positions in old image
	yMap   map[int]int // maps y positions in new image to y positions in old image
	width  int
	height int
	mode   stretchingMode
}

func (d cylinderData) rect() image.Rectangle {
	return image.Rect(
		0,
		0,
		d.width,
		d.height,
	)
}

func (d cylinderData) oldX(new int) int {
	return d.xMap[new]
}

func (d cylinderData) oldY(new int) int {
	if d.mode != yStretching {
		return new
	}
	return d.yMap[new]
}

type rgbData struct {
	r uint8
	g uint8
	b uint8
}

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

func New(dpi, rpm, fps, fontSize, padding float64, text, bgHex, fgHex, fontFilepath string, verbose, debug bool, routines int, ratio float64) (*Circler, error) {
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
		debug:    debug,
		routines: routines,
		ratio:    ratio,
	}, nil
}

func (c *Circler) Printf(format string, args ...any) {
	if c.verbose || c.debug {
		log.Printf(format, args...)
	}
}

func (c *Circler) Debugf(format string, args ...any) {
	if c.debug {
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

	// for all unique colours in the source, calc Square of distance to fg and bg
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
	textImage := c.createTextImage(strings.Repeat(c.text, 2))
	c.Printf("Full text image size: (%d, %d)\n", textImage.Bounds().Dx(), textImage.Bounds().Dy())
	c.readColourData(textImage)

	traversal := textImage.Bounds().Dx() / 2
	height := textImage.Bounds().Dy()
	startOffset := traversal / 2
	visibleLen := traversal / 2

	secsPerRev := 60.0 / float64(c.rpm)
	framesPerRev := round(secsPerRev * c.fps)
	advancePerFrame := float64(traversal) / float64(framesPerRev)
	frameDelay := round(100.0 / c.fps) // 100ths of a second

	images := make([]*image.Paletted, framesPerRev)
	delays := make([]int, framesPerRev)

	c.Printf("GIF data requires %d frames per rotation\n", framesPerRev)

	semaphore := make(chan struct{}, c.routines)

	var wg sync.WaitGroup

	for j := 0; j < framesPerRev; j++ {
		wg.Add(1)

		// This blocks if the semaphore channel is full
		semaphore <- struct{}{}

		go func(i int) {
			defer wg.Done()
			// Release the slot when the goroutine finishes
			defer func() { <-semaphore }()

			advance := advancePerFrame * float64(i)
			imageRect := image.Rect(startOffset+round(advance), 0, startOffset+visibleLen+round(advance), height)
			subimage := textImage.SubImage(imageRect).(*image.RGBA)
			c.Debugf("subimage size: %s\n", subimage.Bounds())
			subImagePaletted := c.rgbaToPaletted(subimage)
			c.Debugf("subImagePaletted size: %s\n", subImagePaletted.Bounds())
			cylindrified := c.cyclindrify(subImagePaletted)
			c.Debugf("cylindrified size: %s\n", cylindrified.Bounds())
			images[i] = cylindrified
			delays[i] = frameDelay

			if i%100 == 0 || i == framesPerRev-1 {
				c.Printf("Built frame %d of %d\n", i+1, framesPerRev)
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
			rgb := rgbData{
				r: colourAtXY.R,
				g: colourAtXY.G,
				b: colourAtXY.B,
			}
			c.colourData.rgbMap[rgb] = colourAtXY
		}
	}

	c.Printf("Found %d unique colours", len(c.colourData.rgbMap))
}

// rgbaToPaletted converts an RGBA image to a paletted image with its min at 0,0.
func (c *Circler) rgbaToPaletted(src *image.RGBA) *image.Paletted {
	bounds := src.Bounds()
	xOffset := bounds.Min.X
	yOffset := bounds.Min.Y
	newBounds := image.Rect(0, 0, bounds.Max.X-xOffset, bounds.Max.Y-yOffset)
	// calling c.palette() for the first time maps the existing RGBA colours into a max 256-colour palette
	dst := image.NewPaletted(newBounds, c.palette())
	var colourAtXY color.RGBA
	bgCount := 0
	fgCount := 0
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
			if selectedCol == c.bg {
				bgCount++
			} else {
				fgCount++
			}
			dst.Set(x-xOffset, y-yOffset, selectedCol)
		}
	}
	c.Debugf("bg pixels: %d, fg pixels: %d", bgCount, fgCount)
	return dst
}

func round(f float64) int {
	return int(math.Round(f))
}

// buildCylinderData builds cylinderData which can cyclindrify successive images with the same bounds
func (c *Circler) buildCylinderData(img *image.Paletted) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	if c.cylinderData != nil {
		return
	}

	bounds := img.Bounds()

	oldWidthI := bounds.Dx()
	oldHeightI := bounds.Dy()
	oldWidthF := float64(oldWidthI)
	oldHeightF := float64(oldHeightI)

	// apply cylinder conversion but not stretching
	unstretchedNewWidth := oldWidthF * 2.0 / math.Pi
	newWidthF := unstretchedNewWidth

	newHeightF := oldHeightF
	c.Printf("Unstretched cylinder: (%d, %d)\n", round(unstretchedNewWidth), oldHeightI)

	cylinderRatio := unstretchedNewWidth / oldHeightF
	c.Printf("Unstretched cylinder ratio: %.2f\n", cylinderRatio)
	var mode stretchingMode
	if c.ratio != 0.0 && c.ratio != cylinderRatio {
		c.Printf("Desired ratio: %.2f\n", c.ratio)
		// c.ratio == desired ratio of width to height
		if c.ratio < cylinderRatio {
			newHeightF = unstretchedNewWidth / c.ratio
			c.Printf("Needs Y stretching to height: %.2f\n", newHeightF)

			mode = yStretching
		}
		if c.ratio > cylinderRatio {
			newWidthF = oldHeightF * c.ratio
			c.Printf("Needs X stretching to width: %.2f\n", newWidthF)

			mode = xStretching
		}
	}
	data := cylinderData{
		height: round(newHeightF),
		width:  round(newWidthF),
		mode:   mode,
	}

	data.xMap = make(map[int]int, data.width)

	if data.mode == yStretching {
		data.yMap = make(map[int]int, data.height)
	}

	// Cylinder effect parameters
	radius := unstretchedNewWidth / 2.0
	var newXF, angle, oldXF float64
	var oldXI, x, y int
	for x = 0; x < data.width; x++ {
		// Calculate the angle on the cylinder for the current x
		newXF = float64(x)
		// reversing the ratio stretching (if any)
		if data.mode == xStretching {
			newXF = newXF * unstretchedNewWidth / newWidthF
		}
		angle = math.Acos((radius - newXF) / radius)
		oldXF = oldWidthF * angle / math.Pi
		oldXI = round(oldXF)
		if oldXI < 0 || oldXI >= oldWidthI {
			continue
		}
		data.xMap[x] = oldXI
	}

	if data.mode == yStretching {
		for y = 0; y < data.height; y++ {
			data.yMap[y] = round(float64(y) * oldHeightF / newHeightF)
		}
	}
	c.cylinderData = &data
	c.Printf("Cylinder data created: (%d, %d)\n", c.cylinderData.rect().Dx(), c.cylinderData.rect().Dy())
}

func (c *Circler) cyclindrify(img *image.Paletted) *image.Paletted {
	c.buildCylinderData(img)
	newRect := c.cylinderData.rect()

	// Create a blank canvas for the output
	// Palette must have bg at index 0
	cylindrified := image.NewPaletted(newRect, img.Palette)

	// fill canvas with colours mapped from old image
	var oldX, y, oldY int
	for x := 0; x < c.cylinderData.width; x++ {
		oldX = c.cylinderData.oldX(x)
		for y = 0; y < c.cylinderData.height; y++ {
			oldY = c.cylinderData.oldY(y)
			cylindrified.Set(x, y, img.At(oldX, oldY))
		}
	}

	return cylindrified
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
