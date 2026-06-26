package runner

import (
	"flag"
	"fmt"
	"image/gif"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/merlincox/wheeler/internal/circler"
)

type Config struct {
	Text           string
	OutputFilepath string
	FontFilepath   string
	Version        string
	Silent         bool
	Debug          bool
	Verbose        bool
}

var (
	text, outputFilepath, fontFilepath string
	verbose, debug, silent             bool
)

func Run(cfg Config) error {
	// default values
	bgHex := "EEFFFF"
	fgHex := "FF0000"
	dpi := 400.0
	rpm := 2.5
	fps := 50.0
	repeat := 1
	padding := 0.25
	fontSize := 32.0
	routines := runtime.NumCPU()

	flag.IntVar(&repeat, "repeat", repeat, "Number of times to repeat the text as a single line")
	flag.StringVar(&bgHex, "bg", bgHex, "Background colour in hex format such as FFFFFF (white)")
	flag.StringVar(&fgHex, "fg", fgHex, "Character colour in hex format such as FF0000 (red)")
	flag.Float64Var(&dpi, "dpi", dpi, "Dots per inch")
	flag.Float64Var(&rpm, "rpm", rpm, "Rotations per minute")
	flag.Float64Var(&fps, "fps", fps, "GIF frames per second")
	flag.Float64Var(&fontSize, "size", fontSize, "Font size")
	flag.Float64Var(&padding, "padding", padding, "Base padding as a fraction of the character height")
	flag.IntVar(&routines, "routines", routines, "Limit of simultaneous goroutines to use")

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
		fmt.Printf("%s %s\n", program, cfg.Version)
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

	cc, err := circler.New(dpi, rpm, fps, fontSize, padding, text, bgHex, fgHex, fontFilepath, verbose, debug, routines)
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
