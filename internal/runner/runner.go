package runner

import (
	"flag"
	"fmt"
	"image/gif"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/merlincox/wheeler/internal/circler"
)

type Config struct {
	Text           string
	OutputFilepath string
	FontFilepath   string
	BgHex          string
	FgHex          string
	DPI            float64
	RPM            float64
	FPS            float64
	Padding        float64
	FontSize       float64
	Aspect         string
	Repeat         int
	Routines       int
	Silent         bool
	Debug          bool
	Verbose        bool
}

func DefaultConfig() Config {
	return Config{
		BgHex:    "EEFFFF",
		FgHex:    "FF0000",
		DPI:      400.0,
		RPM:      2.5,
		FPS:      50.0,
		Repeat:   1,
		Padding:  0.25,
		FontSize: 32.0,
		Routines: runtime.NumCPU(),
	}
}

func Run(cfg Config, version string) error {
	var (
		repeat, routines                   int
		bgHex, fgHex, aspect               string
		dpi, rpm, fps, fontSize, padding   float64
		text, outputFilepath, fontFilepath string
		debug, verbose, silent             bool
	)

	flag.IntVar(&repeat, "repeat", cfg.Repeat, "Number of times to repeat the text as a single line")
	flag.StringVar(&bgHex, "bg", cfg.BgHex, "Background colour in hex format such as FFFFFF (white)")
	flag.StringVar(&fgHex, "fg", cfg.FgHex, "Character colour in hex format such as FF0000 (red)")
	flag.Float64Var(&dpi, "dpi", cfg.DPI, "Dots per inch")
	flag.Float64Var(&rpm, "rpm", cfg.RPM, "Rotations per minute")
	flag.Float64Var(&fps, "fps", cfg.FPS, "GIF frames per second")
	flag.Float64Var(&fontSize, "size", cfg.FontSize, "Font size")
	flag.Float64Var(&padding, "padding", cfg.Padding, "Base padding as a fraction of the character height")
	flag.StringVar(&aspect, "aspect", cfg.Aspect, "Desired aspect ratio of width to height in the form W:H")
	flag.IntVar(&routines, "routines", cfg.Routines, "Limit of simultaneous goroutines to use")

	flag.StringVar(&text, "text", cfg.Text, "Text to render (required)")
	flag.StringVar(&outputFilepath, "out", cfg.OutputFilepath, "Output file path (required)")
	flag.StringVar(&fontFilepath, "fontpath", cfg.FontFilepath, "Font file path (optional)")
	flag.BoolVar(&debug, "debug", cfg.Debug, "Print debug messages")
	flag.BoolVar(&verbose, "verbose", cfg.Verbose, "Print details of colour mapping and frame rendering in real time")
	flag.BoolVar(&silent, "silent", cfg.Silent, "Print no output")

	printVersion := flag.Bool("version", false, "Print version and exit")
	printUsage := flag.Bool("usage", false, "Print usage and exit")

	program := filepath.Base(os.Args[0])

	flag.Usage = func() {
		fmt.Printf("Usage:  %s [options]\n\n", program)
		flag.PrintDefaults()
		fmt.Printf("\n\n"+`Example:  %s -verbose -out images/wheeler.gif -text "ཨོཾ་ཨཱཿ་ཧཱུྃ་བཛྲ་གུ་རུ་པདྨ་སིདྡྷི་ཧཱུྃ།"`+"\n", program)
	}

	flag.Parse()

	if *printVersion {
		fmt.Printf("%s %s\n", program, version)
		return nil
	}

	if *printUsage {
		flag.Usage()
		return nil
	}

	if repeat < 1 {
		return fmt.Errorf("repeat must be 1 or greater")
	}

	if outputFilepath == "" {
		return fmt.Errorf("output file path is required")
	}

	if silent && verbose {
		return fmt.Errorf("cannot be both silent and verbose")
	}

	ratio, err := parseAspectRatio(aspect)
	if err != nil {
		return err
	}

	if debug {
		silent = false
	}

	if !silent && !verbose {
		defer func() {
			fmt.Printf(" wrote %s\n", outputFilepath)
		}()

		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()

		go func() {
			fmt.Printf("Building .")
			for {
				<-ticker.C
				fmt.Printf(".")
			}
		}()
	}
	text = strings.Repeat(text, repeat)

	cc, err := circler.New(dpi, rpm, fps, fontSize, padding, text, bgHex, fgHex, fontFilepath, verbose, debug, routines, ratio)
	if err != nil {
		return err
	}

	gifData := cc.BuildGIFData()

	gifFile, err := os.Create(outputFilepath)
	if err != nil {
		return fmt.Errorf("error creating GIF file '%s': %v", outputFilepath, err)
	}
	defer func() {
		_ = gifFile.Close()
		if verbose {
			log.Printf("Output to GIF file '%s'", outputFilepath)
		}
	}()

	if err = gif.EncodeAll(gifFile, gifData); err != nil {
		return fmt.Errorf("error encoding GIF: %v", err)
	}

	return nil
}

func parseAspectRatio(aspect string) (float64, error) {
	if aspect == "" {
		return 0, nil
	}
	parts := strings.Split(aspect, ":")
	if len(parts) != 2 {
		return 0, fmt.Errorf("invalid aspect ratio '%s', should be in the form width:height", aspect)
	}
	width, err := strconv.ParseFloat(parts[0], 64)
	if err != nil {
		return 0, fmt.Errorf("invalid aspect ratio width '%s'", parts[0])
	}
	height, err := strconv.ParseFloat(parts[1], 64)
	if err != nil {
		return 0, fmt.Errorf("invalid aspect ratio height '%s'", parts[1])
	}

	return width / height, nil
}
