package main

import (
	"fmt"
	"image"
	"image/color"
	"io/fs"
	"log"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

const appName = "Sprite-Player"

type PlayMode int

const (
	ModeLoop PlayMode = iota
	ModeOnce
	ModePingPong
)

const (
	minFPS = 1
	maxFPS = 60
)

type Game struct {
	fonts uiFonts

	// Project
	title       string
	dir         string   // folder of the project ("" if none)
	files       []string // explicitly opened files; nil = all PNGs in dir
	paths       []string // currently loaded paths
	hasProjFile bool
	canvasW     int
	canvasH     int
	frames      []Frame
	sig         string
	lastPoll    time.Time
	loadErrs    []string

	// Playback
	cur      int
	playing  bool
	pingDir  int
	acc      float64
	lastTick time.Time

	// View
	zoomAuto   bool
	zoom       int
	panX, panY float64
	panning    bool
	panLastX   int
	panLastY   int
	fullscreen bool
	lastMove   time.Time
	mouseX     int
	mouseY     int
	canvasImg  *ebiten.Image

	// UI interaction
	pressedID   string
	pressTicks  int
	sliderDrag  bool
	hoverID     string
	hoverSince  time.Time
	tasks       chan func()
	dialogBusy  bool
	status      string
	statusErr   bool
	statusUntil time.Time

	settings      Settings
	settingsDirty bool

	scale  float64
	sw, sh int
}

func NewGame() *Game {
	g := &Game{
		tasks:    make(chan func(), 16),
		zoomAuto: true,
		zoom:     1,
		pingDir:  1,
		canvasW:  64,
		canvasH:  64,
		scale:    1,
		lastTick: time.Now(),
	}
	g.settings = loadSettings()
	if g.settings.FPS < minFPS || g.settings.FPS > maxFPS {
		g.settings.FPS = 12
	}
	if g.settings.Mode < 0 || g.settings.Mode > ModePingPong {
		g.settings.Mode = ModeLoop
	}
	return g
}

// ---------------------------------------------------------------- project

func (g *Game) setStatus(msg string, isErr bool) {
	g.status = msg
	g.statusErr = isErr
	d := 4 * time.Second
	if isErr {
		d = 7 * time.Second
	}
	g.statusUntil = time.Now().Add(d)
}

func (g *Game) clearFrames() {
	for _, f := range g.frames {
		f.Img.Deallocate()
	}
	g.frames = nil
}

// openProject opens a folder (files == nil) or a set of PNG files.
func (g *Game) openProject(dir string, files []string) {
	g.dir = dir
	g.files = files
	pf, ok := readProjectFile(dir)
	g.hasProjFile = ok
	g.title = filepath.Base(dir)
	if ok && strings.TrimSpace(pf.Title) != "" {
		g.title = pf.Title
	}
	g.cur = 0
	g.pingDir = 1
	g.acc = 0
	g.zoomAuto = true
	g.panX, g.panY = 0, 0
	g.reload(true)
	if ok && pf.Width > 0 && pf.Height > 0 {
		g.canvasW, g.canvasH = pf.Width, pf.Height
	} else if len(g.frames) > 0 {
		g.canvasW, g.canvasH = g.frames[0].W, g.frames[0].H
	}
	g.playing = len(g.frames) > 1
	g.lastTick = time.Now()
	if dir != "" {
		g.settings.LastOpenDir = dir
		g.settingsDirty = true
	}
	g.updateWindowTitle()

	switch {
	case len(g.loadErrs) > 0:
		g.setStatus(fmt.Sprintf("%d Datei(en) konnten nicht geladen werden: %s", len(g.loadErrs), strings.Join(g.loadErrs, ", ")), true)
	case len(g.frames) > 0:
		g.setStatus(fmt.Sprintf("%d Frames geladen", len(g.frames)), false)
	}
}

func (g *Game) currentPaths() []string {
	if g.files != nil {
		var out []string
		for _, f := range g.files {
			if _, err := os.Stat(f); err == nil {
				out = append(out, f)
			}
		}
		return out
	}
	if g.dir == "" {
		return nil
	}
	p, _ := listPNGs(g.dir)
	return p
}

// reload (re)loads all frames from disk.
func (g *Game) reload(initial bool) {
	paths := g.currentPaths()
	frames, errs := loadFrames(paths)
	g.clearFrames()
	g.frames = frames
	g.paths = paths
	g.loadErrs = errs
	g.sig = signature(paths)
	g.lastPoll = time.Now()
	if g.cur >= len(g.frames) {
		g.cur = max(0, len(g.frames)-1)
	}
	if !initial {
		if len(errs) > 0 {
			g.setStatus(fmt.Sprintf("Neu geladen – %d Datei(en) fehlerhaft: %s", len(errs), strings.Join(errs, ", ")), true)
		} else {
			g.setStatus(fmt.Sprintf("Neu geladen – %d Frames", len(g.frames)), false)
		}
	}
}

// pollChanges reloads automatically when PNGs in the folder change.
func (g *Game) pollChanges() {
	if g.dir == "" && g.files == nil {
		return
	}
	if time.Since(g.lastPoll) < time.Second {
		return
	}
	g.lastPoll = time.Now()
	paths := g.currentPaths()
	if signature(paths) != g.sig {
		wasEmpty := len(g.frames) == 0
		g.reload(false)
		if wasEmpty && len(g.frames) > 0 {
			if !g.hasProjFile {
				g.canvasW, g.canvasH = g.frames[0].W, g.frames[0].H
			}
			g.playing = len(g.frames) > 1
		}
	}
}

func (g *Game) saveProjectFile() {
	if g.dir == "" {
		return
	}
	err := writeProjectFile(g.dir, projectFile{Title: g.title, Width: g.canvasW, Height: g.canvasH})
	if err != nil {
		g.setStatus("Projektdatei konnte nicht gespeichert werden: "+err.Error(), true)
		return
	}
	g.hasProjFile = true
}

func (g *Game) updateWindowTitle() {
	if g.title == "" {
		ebiten.SetWindowTitle(appName)
		return
	}
	ebiten.SetWindowTitle(g.title + " – " + appName)
}

// handleDrop processes files or folders dropped onto the window.
func (g *Game) handleDrop(dropped fs.FS) {
	entries, err := fs.ReadDir(dropped, ".")
	if err != nil || len(entries) == 0 {
		return
	}
	var dirs, pngs []string
	ignored := 0
	for _, e := range entries {
		ap, ok := e.(ebiten.AbsPather)
		if !ok {
			continue
		}
		p := ap.AbsPath()
		st, err := os.Stat(p)
		if err != nil {
			continue
		}
		if st.IsDir() {
			dirs = append(dirs, p)
		} else if isPNG(p) {
			pngs = append(pngs, p)
		} else {
			ignored++
		}
	}
	if len(pngs) > 0 {
		sortNatural(pngs)
		g.openFiles(pngs)
		if ignored > 0 {
			g.setStatus(fmt.Sprintf("%d Frames geladen, %d Nicht-PNG-Datei(en) ignoriert", len(g.frames), ignored), false)
		}
		return
	}
	if len(dirs) > 0 {
		g.openProject(dirs[0], nil)
		if len(dirs) > 1 {
			g.setStatus("Mehrere Ordner abgelegt – nur der erste wurde geöffnet", false)
		}
		return
	}
	g.setStatus("Keine PNG-Dateien gefunden", true)
}

// openFiles opens individual PNG files. If they all live in the same folder
// and that folder contains every one of its PNGs, the folder is opened instead
// (so that new frames are picked up automatically). If a project folder is open
// and the files come from elsewhere, the user may copy them into the project.
func (g *Game) openFiles(pngs []string) {
	dir := filepath.Dir(pngs[0])
	sameDir := true
	for _, p := range pngs {
		if filepath.Dir(p) != dir {
			sameDir = false
			break
		}
	}
	if sameDir && g.hasProjFile && g.dir != "" && !samePath(dir, g.dir) {
		g.askCopyIntoProject(pngs)
		return
	}
	if sameDir {
		all, _ := listPNGs(dir)
		if len(all) == len(pngs) {
			g.openProject(dir, nil)
			return
		}
	}
	g.openProject(dir, pngs)
}

func samePath(a, b string) bool {
	return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
}

// ---------------------------------------------------------------- playback

func (g *Game) fps() int { return g.settings.FPS }

func (g *Game) setFPS(v int) {
	v = min(max(v, minFPS), maxFPS)
	if v != g.settings.FPS {
		g.settings.FPS = v
		g.settingsDirty = true
	}
}

func (g *Game) setMode(m PlayMode) {
	g.settings.Mode = m
	g.settingsDirty = true
	g.pingDir = 1
}

func (g *Game) togglePlay() {
	n := len(g.frames)
	if n < 2 {
		return
	}
	if !g.playing && g.settings.Mode == ModeOnce && g.cur == n-1 {
		g.cur = 0
	}
	g.playing = !g.playing
	g.acc = 0
}

func (g *Game) step(d int) {
	n := len(g.frames)
	if n == 0 {
		return
	}
	g.playing = false
	g.cur = ((g.cur+d)%n + n) % n
}

func (g *Game) advance() {
	n := len(g.frames)
	if n < 2 {
		return
	}
	switch g.settings.Mode {
	case ModeLoop:
		g.cur = (g.cur + 1) % n
	case ModeOnce:
		if g.cur < n-1 {
			g.cur++
		}
		if g.cur >= n-1 {
			g.playing = false
		}
	case ModePingPong:
		if g.pingDir == 0 {
			g.pingDir = 1
		}
		next := g.cur + g.pingDir
		if next < 0 || next >= n {
			g.pingDir = -g.pingDir
			next = g.cur + g.pingDir
		}
		g.cur = min(max(next, 0), n-1)
	}
}

// ---------------------------------------------------------------- view

func (g *Game) px(v float64) int { return int(math.Round(v * g.scale)) }

func (g *Game) topBarH() int    { return g.px(46) }
func (g *Game) bottomRowH() int { return g.px(40) }

// canvasArea is the region in which the canvas is shown.
func (g *Game) canvasArea() image.Rectangle {
	if g.fullscreen {
		return image.Rect(0, 0, g.sw, g.sh)
	}
	return image.Rect(0, g.topBarH(), g.sw, g.sh-2*g.bottomRowH())
}

func (g *Game) fitZoom() int {
	a := g.canvasArea()
	m := g.px(16)
	z := min((a.Dx()-m)/max(g.canvasW, 1), (a.Dy()-m)/max(g.canvasH, 1))
	return max(z, 1)
}

func (g *Game) effectiveZoom() int {
	if g.zoomAuto {
		return g.fitZoom()
	}
	return g.zoom
}

func (g *Game) setZoom(z int) {
	g.zoom = min(max(z, 1), 64)
	g.zoomAuto = false
}

func (g *Game) canvasRect() image.Rectangle {
	a := g.canvasArea()
	z := g.effectiveZoom()
	w, h := g.canvasW*z, g.canvasH*z
	x := a.Min.X + (a.Dx()-w)/2 + int(math.Round(g.panX))
	y := a.Min.Y + (a.Dy()-h)/2 + int(math.Round(g.panY))
	return image.Rect(x, y, x+w, y+h)
}

func (g *Game) setFullscreen(on bool) {
	g.fullscreen = on
	ebiten.SetFullscreen(on)
	g.lastMove = time.Now()
	g.panX, g.panY = 0, 0
}

// uiVisible reports whether the bars are shown (they auto-hide in fullscreen).
func (g *Game) uiVisible() bool {
	if !g.fullscreen {
		return true
	}
	if g.dialogBusy || g.pressedID != "" || g.sliderDrag {
		return true
	}
	if g.mouseY < g.topBarH() || g.mouseY > g.sh-2*g.bottomRowH() {
		return true
	}
	return time.Since(g.lastMove) < 2500*time.Millisecond
}

// ---------------------------------------------------------------- ebiten

func (g *Game) LayoutF(outsideW, outsideH float64) (float64, float64) {
	s := ebiten.Monitor().DeviceScaleFactor()
	if s <= 0 {
		s = 1
	}
	g.scale = s
	g.sw = int(math.Ceil(outsideW * s))
	g.sh = int(math.Ceil(outsideH * s))
	return float64(g.sw), float64(g.sh)
}

func (g *Game) Layout(w, h int) (int, int) { panic("unused") }

func (g *Game) Update() error {
	// Results of dialogs that ran in the background.
	for {
		select {
		case f := <-g.tasks:
			f()
			continue
		default:
		}
		break
	}

	if g.fullscreen != ebiten.IsFullscreen() {
		g.fullscreen = ebiten.IsFullscreen()
	}

	mx, my := ebiten.CursorPosition()
	if mx != g.mouseX || my != g.mouseY {
		g.lastMove = time.Now()
	}
	g.mouseX, g.mouseY = mx, my

	if d := ebiten.DroppedFiles(); d != nil && !g.dialogBusy {
		g.handleDrop(d)
	}

	if !g.dialogBusy {
		g.handleKeys()
		g.handleMouse()
	}

	g.pollChanges()

	// Timing
	now := time.Now()
	dt := now.Sub(g.lastTick).Seconds()
	g.lastTick = now
	if g.playing && len(g.frames) > 1 && !g.sliderDrag {
		g.acc += dt
		period := 1 / float64(g.fps())
		if g.acc > 0.5 {
			g.acc = period
		}
		for g.acc >= period && g.playing {
			g.acc -= period
			g.advance()
		}
	} else {
		g.acc = 0
	}

	if g.settingsDirty {
		saveSettings(g.settings)
		g.settingsDirty = false
	}
	return nil
}

func (g *Game) handleKeys() {
	ctrl := ebiten.IsKeyPressed(ebiten.KeyControl) || ebiten.IsKeyPressed(ebiten.KeyMeta)
	shift := ebiten.IsKeyPressed(ebiten.KeyShift)
	jp := inpututil.IsKeyJustPressed
	rep := func(k ebiten.Key) bool {
		d := inpututil.KeyPressDuration(k)
		return d == 1 || (d > 20 && d%3 == 0)
	}
	if ctrl {
		switch {
		case jp(ebiten.KeyN):
			g.dialogNewProject()
		case jp(ebiten.KeyO):
			g.dialogOpenFolder()
		}
		return
	}
	switch {
	case jp(ebiten.KeySpace):
		g.togglePlay()
	case rep(ebiten.KeyArrowRight):
		g.step(1)
	case rep(ebiten.KeyArrowLeft):
		g.step(-1)
	case jp(ebiten.KeyHome):
		g.playing = false
		g.cur = 0
	case jp(ebiten.KeyEnd):
		g.playing = false
		g.cur = max(len(g.frames)-1, 0)
	case rep(ebiten.KeyArrowUp):
		g.setFPS(g.fps() + pick(shift, 5, 1))
	case rep(ebiten.KeyArrowDown):
		g.setFPS(g.fps() - pick(shift, 5, 1))
	case jp(ebiten.KeyEqual), jp(ebiten.KeyNumpadAdd), jp(ebiten.KeyBracketRight):
		g.setZoom(g.effectiveZoom() + 1)
	case jp(ebiten.KeyMinus), jp(ebiten.KeyNumpadSubtract), jp(ebiten.KeySlash):
		g.setZoom(g.effectiveZoom() - 1)
	case jp(ebiten.KeyDigit0), jp(ebiten.KeyNumpad0):
		g.zoomAuto = true
		g.panX, g.panY = 0, 0
	case jp(ebiten.KeyDigit1), jp(ebiten.KeyNumpad1):
		g.setZoom(1)
		g.panX, g.panY = 0, 0
	case jp(ebiten.KeyF), jp(ebiten.KeyF11):
		g.setFullscreen(!g.fullscreen)
	case jp(ebiten.KeyEscape):
		if g.fullscreen {
			g.setFullscreen(false)
		}
	case jp(ebiten.KeyB):
		g.settings.WhiteBG = !g.settings.WhiteBG
		g.settingsDirty = true
	case jp(ebiten.KeyM):
		g.setMode((g.settings.Mode + 1) % 3)
	case jp(ebiten.KeyF5):
		if g.dir != "" || g.files != nil {
			g.reload(false)
		}
	}
}

func pick(c bool, a, b int) int {
	if c {
		return a
	}
	return b
}

func (g *Game) widgetAt(ws []Widget, x, y int) *Widget {
	for i := len(ws) - 1; i >= 0; i-- {
		if image.Pt(x, y).In(ws[i].Rect) {
			return &ws[i]
		}
	}
	return nil
}

func (g *Game) handleMouse() {
	ws := g.buildUI()
	mx, my := g.mouseX, g.mouseY
	visible := g.uiVisible()
	var over *Widget
	if visible {
		over = g.widgetAt(ws, mx, my)
	}

	id := ""
	if over != nil {
		id = over.ID
	}
	if id != g.hoverID {
		g.hoverID = id
		g.hoverSince = time.Now()
	}

	inBars := visible && !image.Pt(mx, my).In(g.canvasArea())
	if g.fullscreen && visible {
		inBars = my < g.topBarH() || my >= g.sh-2*g.bottomRowH()
	}

	// Cursor shape
	switch {
	case over != nil && over.Kind != KindLabel && !over.Disabled:
		ebiten.SetCursorShape(ebiten.CursorShapePointer)
	case g.panning:
		ebiten.SetCursorShape(ebiten.CursorShapeMove)
	default:
		ebiten.SetCursorShape(ebiten.CursorShapeDefault)
	}

	left := ebiten.MouseButtonLeft
	if inpututil.IsMouseButtonJustPressed(left) {
		switch {
		case over != nil && over.Kind == KindSlider:
			g.sliderDrag = true
		case over != nil && over.Kind == KindButton && !over.Disabled:
			g.pressedID = over.ID
			g.pressTicks = 0
			if over.Repeat {
				g.onClick(over.ID)
			}
		case !inBars && len(g.frames) > 0:
			g.panning = true
			g.panLastX, g.panLastY = mx, my
		}
	}
	if ebiten.IsMouseButtonPressed(left) {
		if g.sliderDrag {
			for _, w := range ws {
				if w.Kind == KindSlider {
					g.setFrameFromSlider(w.Rect, mx)
				}
			}
		}
		if g.pressedID != "" && over != nil && over.ID == g.pressedID && over.Repeat {
			g.pressTicks++
			if g.pressTicks > 24 && g.pressTicks%4 == 0 {
				g.onClick(g.pressedID)
			}
		}
		if g.panning {
			g.panX += float64(mx - g.panLastX)
			g.panY += float64(my - g.panLastY)
			g.panLastX, g.panLastY = mx, my
		}
	}
	if inpututil.IsMouseButtonJustReleased(left) {
		if g.pressedID != "" && over != nil && over.ID == g.pressedID && !over.Repeat {
			g.onClick(g.pressedID)
		}
		g.pressedID = ""
		g.sliderDrag = false
		g.panning = false
	}

	// Mouse wheel: zoom on the canvas, FPS over the FPS controls.
	if _, wy := ebiten.Wheel(); wy != 0 {
		d := 1
		if wy < 0 {
			d = -1
		}
		if over != nil && strings.HasPrefix(over.ID, "fps") {
			g.setFPS(g.fps() + d)
		} else if !inBars && len(g.frames) > 0 {
			g.setZoom(g.effectiveZoom() + d)
		}
	}

	// Middle click on the canvas resets the view.
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonMiddle) && !inBars {
		g.zoomAuto = true
		g.panX, g.panY = 0, 0
	}
}

func (g *Game) setFrameFromSlider(r image.Rectangle, mx int) {
	n := len(g.frames)
	if n == 0 {
		return
	}
	g.playing = false
	inner := r.Inset(g.px(8))
	t := float64(mx-inner.Min.X) / float64(max(inner.Dx(), 1))
	t = min(max(t, 0), 1)
	g.cur = int(math.Round(t * float64(n-1)))
}

func (g *Game) onClick(id string) {
	switch id {
	case "new":
		g.dialogNewProject()
	case "open":
		g.dialogOpenFolder()
	case "openfiles":
		g.dialogOpenFiles()
	case "explorer":
		openInExplorer(g.dir)
	case "title":
		g.dialogRename()
	case "canvas":
		g.dialogCanvasSize()
	case "first":
		g.playing = false
		g.cur = 0
	case "prev":
		g.step(-1)
	case "play":
		g.togglePlay()
	case "next":
		g.step(1)
	case "last":
		g.playing = false
		g.cur = max(len(g.frames)-1, 0)
	case "fps-":
		g.setFPS(g.fps() - 1)
	case "fps+":
		g.setFPS(g.fps() + 1)
	case "mode-loop":
		g.setMode(ModeLoop)
	case "mode-once":
		g.setMode(ModeOnce)
	case "mode-ping":
		g.setMode(ModePingPong)
	case "bg-black":
		g.settings.WhiteBG = false
		g.settingsDirty = true
	case "bg-white":
		g.settings.WhiteBG = true
		g.settingsDirty = true
	case "zoom-":
		g.setZoom(g.effectiveZoom() - 1)
	case "zoom+":
		g.setZoom(g.effectiveZoom() + 1)
	case "zoom-fit":
		g.zoomAuto = true
		g.panX, g.panY = 0, 0
	case "fullscreen":
		g.setFullscreen(!g.fullscreen)
	}
}

// ---------------------------------------------------------------- UI layout

func (g *Game) buildUI() []Widget {
	f := g.fonts.get(g.scale)
	var ws []Widget
	pad := g.px(8)
	gap := g.px(6)
	bh := g.px(28)
	btnW := func(label string) int { return textWidth(label, f.face) + g.px(24) }

	// Top bar
	tb := g.topBarH()
	y := (tb - bh) / 2
	x := pad
	add := func(w Widget) {
		ws = append(ws, w)
		x = w.Rect.Max.X + gap
	}
	button := func(id, label, tip string, width int) Widget {
		if width == 0 {
			width = btnW(label)
		}
		return Widget{ID: id, Kind: KindButton, Label: label, Tooltip: tip, Rect: image.Rect(x, y, x+width, y+bh), Center: true}
	}
	label := func(s string, width int, clr color.Color) Widget {
		if width == 0 {
			width = textWidth(s, f.face)
		}
		return Widget{ID: "", Kind: KindLabel, Label: s, Color: clr, Rect: image.Rect(x, y, x+width, y+bh)}
	}

	add(button("new", "Neues Projekt …", "Projektordner mit Titel und Leinwand-Größe anlegen (Strg+N)", 0))
	add(button("open", "Ordner öffnen …", "Projektordner öffnen (Strg+O) – oder Ordner/PNGs ins Fenster ziehen", 0))
	add(button("openfiles", "PNGs öffnen …", "Einzelne PNG-Dateien auswählen", 0))
	eb := button("explorer", "Im Explorer zeigen", "Projektordner im Explorer öffnen", 0)
	eb.Disabled = g.dir == ""
	add(eb)
	x += g.px(10)

	if g.title != "" {
		t := button("title", g.title, "Titel ändern", 0)
		t.Flat = true
		t.Center = false
		t.Rect.Max.X = t.Rect.Min.X + min(t.Rect.Dx(), g.px(320))
		add(t)
		c := button("canvas", fmt.Sprintf("Leinwand %d × %d", g.canvasW, g.canvasH), "Leinwand-Größe ändern", 0)
		c.Flat = true
		add(c)
		add(label(fmt.Sprintf("%d Frames", len(g.frames)), 0, colDim))
	}

	// Bottom row 1: transport + timeline
	rh := g.bottomRowH()
	y = g.sh - 2*rh + (rh-bh)/2
	x = pad
	iconBtn := func(id string, ic Icon, tip string) Widget {
		w := button(id, "", tip, g.px(36))
		w.Icon = ic
		w.Disabled = len(g.frames) == 0
		return w
	}
	add(iconBtn("first", IconFirst, "Zum ersten Frame (Pos1)"))
	add(iconBtn("prev", IconPrev, "Einen Frame zurück (←)"))
	pi := IconPlay
	tip := "Abspielen (Leertaste)"
	if g.playing {
		pi = IconPause
		tip = "Pause (Leertaste)"
	}
	pb := iconBtn("play", pi, tip)
	pb.Rect.Max.X += g.px(8)
	pb.Active = g.playing
	pb.Disabled = len(g.frames) < 2
	add(pb)
	add(iconBtn("next", IconNext, "Einen Frame vor (→)"))
	add(iconBtn("last", IconLast, "Zum letzten Frame (Ende)"))
	x += g.px(6)
	fl := "– / –"
	if len(g.frames) > 0 {
		fl = fmt.Sprintf("Frame %d / %d", g.cur+1, len(g.frames))
	}
	add(label(fl, textWidth("Frame 0000 / 0000", f.face), colText))
	sl := Widget{ID: "slider", Kind: KindSlider, Rect: image.Rect(x, y, g.sw-pad, y+bh)}
	if len(g.frames) == 0 {
		sl.Disabled = true
	}
	ws = append(ws, sl)

	// Bottom row 2: settings
	y = g.sh - rh + (rh-bh)/2
	x = pad
	small := g.px(28)
	add(label("FPS", 0, colDim))
	m := button("fps-", "−", "Langsamer (↓, Mausrad)", small)
	m.Repeat = true
	add(m)
	v := label(fmt.Sprintf("%d", g.fps()), g.px(30), colText)
	v.Center = true
	v.ID = "fps-value"
	add(v)
	p := button("fps+", "+", "Schneller (↑, Mausrad)", small)
	p.Repeat = true
	add(p)
	x += g.px(18)

	add(label("Modus", 0, colDim))
	for _, md := range []struct {
		id, label, tip string
		mode           PlayMode
	}{
		{"mode-loop", "Schleife", "Endlos wiederholen (M wechselt)", ModeLoop},
		{"mode-once", "Einmal", "Einmal abspielen und am Ende anhalten (M wechselt)", ModeOnce},
		{"mode-ping", "Ping-Pong", "Vorwärts und rückwärts im Wechsel (M wechselt)", ModePingPong},
	} {
		b := button(md.id, md.label, md.tip, 0)
		b.Active = g.settings.Mode == md.mode
		add(b)
		x -= gap - g.px(2)
	}
	x += gap + g.px(16)

	add(label("Hintergrund", 0, colDim))
	bb := button("bg-black", "Schwarz", "Schwarzer Hintergrund (B wechselt)", 0)
	bb.Active = !g.settings.WhiteBG
	add(bb)
	x -= gap - g.px(2)
	bw := button("bg-white", "Weiß", "Weißer Hintergrund (B wechselt)", 0)
	bw.Active = g.settings.WhiteBG
	add(bw)
	x += g.px(18)

	add(label("Zoom", 0, colDim))
	add(button("zoom-", "−", "Verkleinern (−, Mausrad)", small))
	zv := label(fmt.Sprintf("%d×", g.effectiveZoom()), g.px(36), colText)
	zv.Center = true
	add(zv)
	add(button("zoom+", "+", "Vergrößern (+, Mausrad)", small))
	zf := button("zoom-fit", "Einpassen", "Größte ganzzahlige Vergrößerung, die passt (0)", 0)
	zf.Active = g.zoomAuto
	add(zf)
	x += g.px(18)

	fsLabel := "Vollbild"
	if g.fullscreen {
		fsLabel = "Vollbild beenden"
	}
	fb := button("fullscreen", fsLabel, "Vollbild an/aus (F oder F11, Esc beendet)", 0)
	fb.Active = g.fullscreen
	add(fb)

	return ws
}

// ---------------------------------------------------------------- drawing

func (g *Game) Draw(screen *ebiten.Image) {
	f := g.fonts.get(g.scale)
	screen.Fill(colArea)
	if g.fullscreen {
		screen.Fill(color.RGBA{0x18, 0x18, 0x1b, 0xff})
	}

	area := g.canvasArea()
	bg := color.Color(color.Black)
	if g.settings.WhiteBG {
		bg = color.White
	}

	if len(g.frames) > 0 || g.title != "" {
		g.drawCanvas(screen, area, bg)
	}
	if len(g.frames) == 0 {
		g.drawEmptyHint(screen, area)
	}

	if g.uiVisible() {
		g.drawBars(screen, f)
	}

	// Status message
	if g.status != "" && time.Now().Before(g.statusUntil) {
		g.drawStatus(screen, f)
	}
}

func (g *Game) drawCanvas(screen *ebiten.Image, area image.Rectangle, bg color.Color) {
	if g.canvasImg == nil || g.canvasImg.Bounds().Dx() != g.canvasW || g.canvasImg.Bounds().Dy() != g.canvasH {
		if g.canvasImg != nil {
			g.canvasImg.Deallocate()
		}
		g.canvasImg = ebiten.NewImage(g.canvasW, g.canvasH)
	}
	g.canvasImg.Fill(bg)
	if len(g.frames) > 0 {
		// Top-left aligned; anything beyond the canvas is cut off.
		g.canvasImg.DrawImage(g.frames[g.cur].Img, nil)
	}

	z := g.effectiveZoom()
	r := g.canvasRect()
	sub := screen.SubImage(area).(*ebiten.Image)

	// Thin frame around the canvas so its edges are always visible.
	bw := max(g.px(1), 1)
	fillRect(sub, image.Rect(r.Min.X-bw, r.Min.Y-bw, r.Max.X+bw, r.Max.Y+bw), colBorder)

	op := &ebiten.DrawImageOptions{}
	op.Filter = ebiten.FilterNearest
	op.GeoM.Scale(float64(z), float64(z))
	op.GeoM.Translate(float64(r.Min.X), float64(r.Min.Y))
	sub.DrawImage(g.canvasImg, op)
}

func (g *Game) drawEmptyHint(screen *ebiten.Image, area image.Rectangle) {
	f := g.fonts.get(g.scale)
	var lines []string
	if g.title == "" {
		lines = []string{
			"PNGs oder einen Ordner hierher ziehen",
			"oder oben „Neues Projekt …“ bzw. „Ordner öffnen …“ wählen.",
		}
	} else {
		lines = []string{
			"Dieses Projekt enthält noch keine PNGs.",
			"Ziehe PNGs in dieses Fenster oder speichere sie im Projektordner –",
			"neue oder geänderte Bilder werden automatisch geladen.",
		}
	}
	lh := g.px(24)
	total := lh * len(lines)
	y0 := area.Min.Y + area.Dy()/2 - total/2
	if g.title != "" {
		// Dark panel so the hint stays readable on top of the (white) canvas.
		w := 0
		for _, l := range lines {
			w = max(w, textWidth(l, f.big))
		}
		w += g.px(40)
		x0 := area.Min.X + (area.Dx()-w)/2
		fillRoundRect(screen, image.Rect(x0, y0-g.px(14), x0+w, y0+total+g.px(14)), float32(g.px(8)), color.RGBA{0x16, 0x16, 0x19, 0xf0})
	}
	for i, l := range lines {
		face := f.face
		clr := color.Color(colDim)
		if i == 0 {
			face = f.big
			clr = colText
		}
		r := image.Rect(area.Min.X, y0+i*lh, area.Max.X, y0+(i+1)*lh)
		drawTextIn(screen, l, face, r, true, clr)
	}
}

func (g *Game) drawBars(screen *ebiten.Image, f *uiFonts) {
	alpha := uint8(0xff)
	if g.fullscreen {
		alpha = 0xe8
	}
	bar := colBar
	bar.A = alpha
	tb := g.topBarH()
	rh := g.bottomRowH()
	fillRect(screen, image.Rect(0, 0, g.sw, tb), bar)
	fillRect(screen, image.Rect(0, tb-1, g.sw, tb), colBarLine)
	fillRect(screen, image.Rect(0, g.sh-2*rh, g.sw, g.sh), bar)
	fillRect(screen, image.Rect(0, g.sh-2*rh, g.sw, g.sh-2*rh+1), colBarLine)

	ws := g.buildUI()
	var tipW *Widget
	for i := range ws {
		w := &ws[i]
		hover := w.ID != "" && w.ID == g.hoverID
		if hover && w.Tooltip != "" && time.Since(g.hoverSince) > 600*time.Millisecond && g.pressedID == "" {
			tipW = w
		}
		switch w.Kind {
		case KindLabel:
			clr := w.Color
			if clr == nil {
				clr = colText
			}
			drawTextIn(screen, w.Label, f.face, w.Rect, w.Center, clr)
		case KindButton:
			g.drawButton(screen, f, w, hover)
		case KindSlider:
			g.drawSlider(screen, w)
		}
	}
	if tipW != nil {
		g.drawTooltip(screen, f, tipW)
	}
}

func (g *Game) drawButton(screen *ebiten.Image, f *uiFonts, w *Widget, hover bool) {
	pressed := g.pressedID == w.ID && hover
	var bgc color.Color = colButton
	switch {
	case w.Disabled:
		bgc = color.RGBA{0x2a, 0x2a, 0x30, 0xff}
	case pressed:
		bgc = colPressed
	case w.Active && hover:
		bgc = colActiveHov
	case w.Active:
		bgc = colActive
	case hover:
		bgc = colHover
	case w.Flat:
		bgc = nil
	}
	if bgc != nil {
		fillRoundRect(screen, w.Rect, float32(g.px(5)), bgc)
	}
	clr := color.Color(colText)
	if w.Disabled {
		clr = colDisabled
	}
	if w.Icon != IconNone {
		drawIcon(screen, w.Icon, w.Rect, g.scale, clr)
		return
	}
	r := w.Rect
	if !w.Center {
		r.Min.X += g.px(10)
	}
	lbl := w.Label
	// Shorten text that does not fit (e.g. long titles).
	maxW := r.Dx() - g.px(10)
	if textWidth(lbl, f.face) > maxW {
		rs := []rune(lbl)
		for len(rs) > 1 && textWidth(string(rs)+"…", f.face) > maxW {
			rs = rs[:len(rs)-1]
		}
		lbl = string(rs) + "…"
	}
	face := f.face
	drawTextIn(screen, lbl, face, r, w.Center, clr)
}

func (g *Game) drawSlider(screen *ebiten.Image, w *Widget) {
	inner := w.Rect.Inset(g.px(8))
	cy := w.Rect.Min.Y + w.Rect.Dy()/2
	th := g.px(4)
	track := image.Rect(inner.Min.X, cy-th/2, inner.Max.X, cy-th/2+th)
	fillRoundRect(screen, track, float32(th)/2, colButton)
	n := len(g.frames)
	if n == 0 {
		return
	}
	pos := func(i int) int {
		if n == 1 {
			return inner.Min.X
		}
		return inner.Min.X + int(math.Round(float64(i)*float64(inner.Dx())/float64(n-1)))
	}
	// Tick per frame if there is enough room.
	if n > 1 && inner.Dx()/(n-1) >= g.px(6) {
		for i := 0; i < n; i++ {
			x := pos(i)
			fillRect(screen, image.Rect(x, cy-g.px(6), x+max(g.px(1), 1), cy+g.px(6)), colHover)
		}
	}
	px := pos(g.cur)
	filled := image.Rect(inner.Min.X, track.Min.Y, px, track.Max.Y)
	if filled.Dx() > 0 {
		fillRoundRect(screen, filled, float32(th)/2, colActive)
	}
	kr := float32(g.px(7))
	c := colText
	if g.hoverID == "slider" || g.sliderDrag {
		c = color.RGBA{0xff, 0xff, 0xff, 0xff}
	}
	fillRoundRect(screen, image.Rect(px-int(kr), cy-int(kr), px+int(kr), cy+int(kr)), kr, c)
}

func (g *Game) drawTooltip(screen *ebiten.Image, f *uiFonts, w *Widget) {
	tw := textWidth(w.Tooltip, f.face) + g.px(16)
	th := g.px(26)
	x := min(max(w.Rect.Min.X, g.px(4)), g.sw-tw-g.px(4))
	y := w.Rect.Max.Y + g.px(6)
	if y+th > g.sh {
		y = w.Rect.Min.Y - th - g.px(6)
	}
	r := image.Rect(x, y, x+tw, y+th)
	fillRoundRect(screen, r, float32(g.px(4)), colTooltip)
	drawTextIn(screen, w.Tooltip, f.face, r, true, colText)
}

func (g *Game) drawStatus(screen *ebiten.Image, f *uiFonts) {
	area := g.canvasArea()
	tw := min(textWidth(g.status, f.face)+g.px(24), g.sw-g.px(16))
	th := g.px(30)
	x := area.Min.X + (area.Dx()-tw)/2
	y := area.Min.Y + g.px(12)
	if g.fullscreen && g.uiVisible() {
		y = g.topBarH() + g.px(12)
	}
	r := image.Rect(x, y, x+tw, y+th)
	c := color.RGBA{0x10, 0x10, 0x12, 0xe8}
	fillRoundRect(screen, r, float32(g.px(6)), c)
	tc := color.Color(colText)
	if g.statusErr {
		tc = colError
	}
	drawTextIn(screen, g.status, f.face, r, true, tc)
}

// ---------------------------------------------------------------- main

func main() {
	g := NewGame()
	ebiten.SetWindowTitle(appName)
	ebiten.SetWindowSize(1200, 800)
	ebiten.SetWindowSizeLimits(1040, 520, -1, -1)
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	ebiten.SetWindowIcon(appIcons())
	ebiten.SetTPS(120)

	// A folder or PNG files passed on the command line (e.g. via "Öffnen mit")
	// are opened directly.
	if len(os.Args) > 1 {
		g.openArgs(os.Args[1:])
	} else if d := g.settings.LastOpenDir; d != "" {
		if st, err := os.Stat(d); err == nil && st.IsDir() {
			g.openProject(d, nil)
		}
	}

	if err := ebiten.RunGameWithOptions(g, &ebiten.RunGameOptions{}); err != nil {
		log.Fatal(err)
	}
	saveSettings(g.settings)
}

func (g *Game) openArgs(args []string) {
	var pngs []string
	for _, a := range args {
		p, err := filepath.Abs(a)
		if err != nil {
			continue
		}
		st, err := os.Stat(p)
		if err != nil {
			continue
		}
		if st.IsDir() {
			g.openProject(p, nil)
			return
		}
		if isPNG(p) {
			pngs = append(pngs, p)
		}
	}
	if len(pngs) > 0 {
		sortNatural(pngs)
		g.openFiles(pngs)
	}
}
