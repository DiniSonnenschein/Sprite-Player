package main

import (
	"image"
	"image/color"
)

// 16×16 pixel-art icon: a film frame with a play triangle.
var iconPixels = [16]string{
	"................",
	".BBBBBBBBBBBBBB.",
	".BwBwBwBwBwBwBB.",
	".BBBBBBBBBBBBBB.",
	".BbbbbbbbbbbbbB.",
	".BbbbbWbbbbbbbB.",
	".BbbbbWWbbbbbbB.",
	".BbbbbWWWbbbbbB.",
	".BbbbbWWWWbbbbB.",
	".BbbbbWWWbbbbbB.",
	".BbbbbWWbbbbbbB.",
	".BbbbbWbbbbbbbB.",
	".BBBBBBBBBBBBBB.",
	".BwBwBwBwBwBwBB.",
	".BBBBBBBBBBBBBB.",
	"................",
}

func iconImage(scale int) *image.NRGBA {
	pal := map[byte]color.NRGBA{
		'.': {0, 0, 0, 0},
		'B': {0x1d, 0x1d, 0x21, 0xff},
		'w': {0xd8, 0xd8, 0xdc, 0xff},
		'b': {0x2f, 0x6b, 0xd8, 0xff},
		'W': {0xff, 0xff, 0xff, 0xff},
	}
	img := image.NewNRGBA(image.Rect(0, 0, 16*scale, 16*scale))
	for y, row := range iconPixels {
		for x := 0; x < 16; x++ {
			c := pal[row[x]]
			for dy := 0; dy < scale; dy++ {
				for dx := 0; dx < scale; dx++ {
					img.SetNRGBA(x*scale+dx, y*scale+dy, c)
				}
			}
		}
	}
	return img
}

func appIcons() []image.Image {
	return []image.Image{iconImage(1), iconImage(2), iconImage(3), iconImage(4)}
}
