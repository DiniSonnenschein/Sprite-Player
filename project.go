package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
)

// projectFileName is the small settings file stored in every project folder.
const projectFileName = "sprite-projekt.json"

type projectFile struct {
	Title  string `json:"titel"`
	Width  int    `json:"leinwand_breite"`
	Height int    `json:"leinwand_hoehe"`
}

// Frame is one loaded PNG.
type Frame struct {
	Path string
	Name string
	Img  *ebiten.Image
	W, H int
}

func readProjectFile(dir string) (projectFile, bool) {
	var p projectFile
	if dir == "" {
		return p, false
	}
	b, err := os.ReadFile(filepath.Join(dir, projectFileName))
	if err != nil {
		return p, false
	}
	if err := json.Unmarshal(b, &p); err != nil {
		return p, false
	}
	return p, true
}

func writeProjectFile(dir string, p projectFile) error {
	b, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, projectFileName), append(b, '\n'), 0o644)
}

func isPNG(name string) bool {
	return strings.EqualFold(filepath.Ext(name), ".png")
}

// listPNGs returns all PNG files directly inside dir, naturally sorted.
func listPNGs(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() || !isPNG(e.Name()) {
			continue
		}
		out = append(out, filepath.Join(dir, e.Name()))
	}
	sortNatural(out)
	return out, nil
}

// sortNatural sorts paths by file name so that "walk_2" comes before "walk_10".
func sortNatural(paths []string) {
	sort.SliceStable(paths, func(i, j int) bool {
		return naturalLess(filepath.Base(paths[i]), filepath.Base(paths[j]))
	})
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

func naturalLess(origA, origB string) bool {
	a, b := strings.ToLower(origA), strings.ToLower(origB)
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		if isDigit(a[i]) && isDigit(b[j]) {
			si := i
			for i < len(a) && isDigit(a[i]) {
				i++
			}
			sj := j
			for j < len(b) && isDigit(b[j]) {
				j++
			}
			na := strings.TrimLeft(a[si:i], "0")
			nb := strings.TrimLeft(b[sj:j], "0")
			if len(na) != len(nb) {
				return len(na) < len(nb)
			}
			if na != nb {
				return na < nb
			}
			continue
		}
		if a[i] != b[j] {
			return a[i] < b[j]
		}
		i++
		j++
	}
	if len(a)-i != len(b)-j {
		return len(a)-i < len(b)-j
	}
	return origA < origB
}

func decodePNG(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return png.Decode(f)
}

// loadFrames decodes all given PNGs. Files that fail are reported in errs.
func loadFrames(paths []string) (frames []Frame, errs []string) {
	for _, p := range paths {
		img, err := decodePNG(p)
		if err != nil {
			errs = append(errs, filepath.Base(p))
			continue
		}
		b := img.Bounds()
		frames = append(frames, Frame{
			Path: p,
			Name: filepath.Base(p),
			Img:  ebiten.NewImageFromImage(img),
			W:    b.Dx(),
			H:    b.Dy(),
		})
	}
	return frames, errs
}

// signature describes the current state of the files so that changes on disk
// (new, deleted or re-exported frames) can be detected.
func signature(paths []string) string {
	var sb strings.Builder
	for _, p := range paths {
		st, err := os.Stat(p)
		if err != nil {
			sb.WriteString(p + "|missing\n")
			continue
		}
		fmt.Fprintf(&sb, "%s|%d|%d\n", p, st.Size(), st.ModTime().UnixNano())
	}
	return sb.String()
}

// parseSize accepts "64x64", "64 x 48", "64×48", "64*48" or just "64".
func parseSize(s string) (int, int, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	for _, sep := range []string{"×", "*", ",", ";", " "} {
		s = strings.ReplaceAll(s, sep, "x")
	}
	var parts []string
	for _, p := range strings.Split(s, "x") {
		if p != "" {
			parts = append(parts, p)
		}
	}
	if len(parts) == 1 {
		parts = append(parts, parts[0])
	}
	if len(parts) != 2 {
		return 0, 0, errors.New("ungültiges Format")
	}
	w, err1 := strconv.Atoi(parts[0])
	h, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil || w < 1 || h < 1 || w > 8192 || h > 8192 {
		return 0, 0, errors.New("ungültige Größe")
	}
	return w, h, nil
}

// sanitizeFolderName removes characters that Windows does not allow in folder names.
func sanitizeFolderName(s string) string {
	s = strings.Map(func(r rune) rune {
		if r < 32 || strings.ContainsRune(`<>:"/\|?*`, r) {
			return '_'
		}
		return r
	}, s)
	return strings.Trim(s, " .")
}
