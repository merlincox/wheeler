package runner

import (
	"flag"
	"fmt"
	"image/gif"
	"log"
	"os"
	"strings"
	"time"

	"github.com/merlincox/wheeler/internal/circler"
)

func Run() error {
	bgHex := "EEFFFF"
	fgHex := "FF0000"
	dpi := 400.0
	rpm := 2.5
	fps := 50.0
	text := "ཨོཾ་ཨཱཿ་ཧཱུྃ་བཛྲ་གུ་རུ་པདྨ་སིདྡྷི་ཧཱུྃ།"
	repeat := 1
	padding := 0.25
	outputFile := "images/wheeler.gif"
	fontSize := 32.0
	verbose := false
	fontFilepath := ""
	silent := false

	flag.StringVar(&text, "text", text, "Text to render (required)")
	flag.IntVar(&repeat, "repeat", repeat, "Number of times to repeat the text as a single line")
	flag.StringVar(&bgHex, "bg", bgHex, "Background colour in hex format such as FFFFFF (white)")
	flag.StringVar(&fgHex, "fg", fgHex, "Character colour in hex format such as FF0000 (red)")
	flag.StringVar(&outputFile, "out", outputFile, "Output file path")
	flag.Float64Var(&dpi, "dpi", dpi, "Dots per inch")
	flag.Float64Var(&rpm, "rpm", rpm, "Rotations per minute")
	flag.Float64Var(&fps, "fps", fps, "GIF frames per second")
	flag.Float64Var(&fontSize, "size", fontSize, "Font size")
	flag.Float64Var(&padding, "padding", padding, "Base padding as a fraction of the character height")
	flag.BoolVar(&verbose, "verbose", verbose, "Verbose: print details of frame rendering")
	flag.BoolVar(&silent, "silent", silent, "Silent: print no output")
	flag.StringVar(&fontFilepath, "fontpath", "", "Font file path (optional)")

	flag.Parse()

	if text == "" {
		return fmt.Errorf("text is required")
	}

	if repeat < 1 {
		return fmt.Errorf("repeat must be 1 or greater")
	}

	if outputFile == "" {
		return fmt.Errorf("output file path is required")
	}

	if silent && verbose {
		return fmt.Errorf("cannot be both silent and verbose")
	}

	if !silent && !verbose {
		defer func() {
			fmt.Printf(" wrote %s\n", outputFile)
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

	cc, err := circler.New(dpi, rpm, fps, fontSize, padding, text, bgHex, fgHex, fontFilepath, verbose)
	if err != nil {
		return err
	}

	gifData := cc.BuildGIFData()

	gifFile, err := os.Create(outputFile)
	if err != nil {
		return fmt.Errorf("error creating GIF file '%s': %v", outputFile, err)
	}
	defer func() {
		_ = gifFile.Close()
		if verbose {
			log.Printf("Output to GIF file '%s'", outputFile)
		}
	}()

	if err = gif.EncodeAll(gifFile, gifData); err != nil {
		return fmt.Errorf("error encoding GIF: %v", err)
	}

	return nil
}
