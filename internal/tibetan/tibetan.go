package tibetan

import (
	_ "embed"
	"fmt"
	"os"

	"github.com/tdewolff/canvas"
)

//go:embed TibetanMachineUni.ttf
var tibetanMachineUniBytes []byte

func getFontFace(name string, src []byte, size float64, args ...any) (*canvas.FontFace, error) {
	fontFamily := canvas.NewFontFamily(name)
	if err := fontFamily.LoadFont(src, 0, canvas.FontRegular); err != nil {
		return nil, fmt.Errorf("error loading %s font: %w", name, err)
	}

	return fontFamily.Face(size, args...), nil
}

func getSourceBytes(filePath string) ([]byte, error) {
	fontBytes, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("error reading file '%s': %v", filePath, err)
	}
	return fontBytes, nil
}

func GetFontFaceFromSource(name, filePath string, size float64, args ...any) (*canvas.FontFace, error) {
	fontBytes, err := getSourceBytes(filePath)
	if err != nil {
		return nil, err
	}
	return getFontFace(name, fontBytes, size, args...)
}

func GetFontFace(size float64, args ...any) (*canvas.FontFace, error) {
	return getFontFace("TibetanMachineUni.ttf", tibetanMachineUniBytes, size, args...)
}
