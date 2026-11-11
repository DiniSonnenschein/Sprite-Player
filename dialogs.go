package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/ncruces/zenity"
)

// runDialog runs blocking native dialogs in the background so that the
// window keeps drawing. The returned function is executed on the main loop.
func (g *Game) runDialog(f func() func()) {
	if g.dialogBusy {
		return
	}
	g.dialogBusy = true
	g.pressedID = ""
	g.panning = false
	g.sliderDrag = false
	go func() {
		apply := f()
		g.tasks <- func() {
			g.dialogBusy = false
			if apply != nil {
				apply()
			}
		}
	}()
}

func showError(title, msg string) {
	_ = zenity.Error(msg, zenity.Title(title), zenity.ErrorIcon)
}

func (g *Game) dialogNewProject() {
	parentDefault := g.settings.LastParentDir
	g.runDialog(func() func() {
		title, err := zenity.Entry("Titel des neuen Projekts:",
			zenity.Title("Neues Projekt (1/3)"), zenity.EntryText("Neue Animation"))
		if err != nil {
			return nil
		}
		title = strings.TrimSpace(title)
		folder := sanitizeFolderName(title)
		if folder == "" {
			showError("Neues Projekt", "Bitte gib einen Titel ein.")
			return nil
		}

		var w, h int
		def := "64x64"
		for {
			s, err := zenity.Entry("Leinwand-Größe in Pixeln (Breite x Höhe), z. B. 64x64 oder 128x96:",
				zenity.Title("Neues Projekt (2/3) – Leinwand"), zenity.EntryText(def))
			if err != nil {
				return nil
			}
			if w, h, err = parseSize(s); err == nil {
				break
			}
			def = s
			showError("Ungültige Größe", "Bitte gib die Größe als Breite x Höhe ein, z. B. 64x64 oder 128x96 (1 bis 8192 Pixel).")
		}

		opts := []zenity.Option{zenity.Directory(),
			zenity.Title("Neues Projekt (3/3) – Wo soll der Ordner „" + folder + "“ angelegt werden?")}
		if parentDefault != "" {
			opts = append(opts, zenity.Filename(parentDefault+string(os.PathSeparator)))
		}
		parent, err := zenity.SelectFile(opts...)
		if err != nil {
			return nil
		}
		dir := filepath.Join(parent, folder)
		if st, err := os.Stat(dir); err == nil {
			if !st.IsDir() {
				showError("Neues Projekt", "Es gibt bereits eine Datei mit dem Namen „"+folder+"“.")
				return nil
			}
			if zenity.Question("Der Ordner „"+folder+"“ existiert bereits.\nSoll er als Projektordner verwendet werden?",
				zenity.Title("Neues Projekt"), zenity.OKLabel("Verwenden"), zenity.CancelLabel("Abbrechen")) != nil {
				return nil
			}
		} else if err := os.MkdirAll(dir, 0o755); err != nil {
			showError("Neues Projekt", "Der Ordner konnte nicht angelegt werden:\n"+err.Error())
			return nil
		}
		if err := writeProjectFile(dir, projectFile{Title: title, Width: w, Height: h}); err != nil {
			showError("Neues Projekt", "Die Projektdatei konnte nicht gespeichert werden:\n"+err.Error())
			return nil
		}
		return func() {
			g.settings.LastParentDir = parent
			g.settingsDirty = true
			g.openProject(dir, nil)
			g.setStatus("Projekt „"+title+"“ angelegt", false)
		}
	})
}

func (g *Game) dialogOpenFolder() {
	start := g.dir
	if start == "" {
		start = g.settings.LastParentDir
	}
	g.runDialog(func() func() {
		opts := []zenity.Option{zenity.Directory(), zenity.Title("Projektordner öffnen")}
		if start != "" {
			opts = append(opts, zenity.Filename(start+string(os.PathSeparator)))
		}
		dir, err := zenity.SelectFile(opts...)
		if err != nil {
			return nil
		}
		return func() {
			g.openProject(dir, nil)
			if len(g.frames) == 0 && !g.hasProjFile {
				g.setStatus("In diesem Ordner liegen keine PNG-Dateien", true)
			}
		}
	})
}

func (g *Game) dialogOpenFiles() {
	start := g.dir
	g.runDialog(func() func() {
		opts := []zenity.Option{
			zenity.Title("PNG-Dateien öffnen"),
			zenity.FileFilters{{Name: "PNG-Bilder", Patterns: []string{"*.png"}, CaseFold: true}},
		}
		if start != "" {
			opts = append(opts, zenity.Filename(start+string(os.PathSeparator)))
		}
		files, err := zenity.SelectFileMultiple(opts...)
		if err != nil || len(files) == 0 {
			return nil
		}
		var pngs []string
		for _, f := range files {
			if isPNG(f) {
				pngs = append(pngs, f)
			}
		}
		return func() {
			if len(pngs) == 0 {
				g.setStatus("Keine PNG-Dateien ausgewählt", true)
				return
			}
			sortNatural(pngs)
			g.openFiles(pngs)
		}
	})
}

func (g *Game) dialogRename() {
	if g.dir == "" {
		return
	}
	cur := g.title
	g.runDialog(func() func() {
		t, err := zenity.Entry("Titel des Projekts:", zenity.Title("Titel ändern"), zenity.EntryText(cur))
		if err != nil {
			return nil
		}
		t = strings.TrimSpace(t)
		if t == "" {
			return nil
		}
		return func() {
			g.title = t
			g.saveProjectFile()
			g.updateWindowTitle()
		}
	})
}

func (g *Game) dialogCanvasSize() {
	cur := fmt.Sprintf("%dx%d", g.canvasW, g.canvasH)
	g.runDialog(func() func() {
		def := cur
		for {
			s, err := zenity.Entry("Leinwand-Größe in Pixeln (Breite x Höhe):", zenity.Title("Leinwand"), zenity.EntryText(def))
			if err != nil {
				return nil
			}
			w, h, err := parseSize(s)
			if err == nil {
				return func() {
					g.canvasW, g.canvasH = w, h
					g.saveProjectFile()
				}
			}
			def = s
			showError("Ungültige Größe", "Bitte gib die Größe als Breite x Höhe ein, z. B. 64x64 oder 128x96 (1 bis 8192 Pixel).")
		}
	})
}

// askCopyIntoProject is used when PNGs from another folder are dropped while a
// project is open.
func (g *Game) askCopyIntoProject(pngs []string) {
	projDir, projTitle := g.dir, g.title
	g.runDialog(func() func() {
		err := zenity.Question(
			fmt.Sprintf("Sollen die %d PNG-Datei(en) in das Projekt „%s“ kopiert werden?\n\n„Nur ansehen“ öffnet die Dateien, ohne sie zu kopieren.", len(pngs), projTitle),
			zenity.Title("PNGs ins Projekt kopieren?"), zenity.QuestionIcon,
			zenity.OKLabel("In Projekt kopieren"), zenity.CancelLabel("Nur ansehen"))
		if err != nil {
			return func() { g.openFilesDirect(pngs) }
		}
		var existing []string
		for _, p := range pngs {
			if _, err := os.Stat(filepath.Join(projDir, filepath.Base(p))); err == nil {
				existing = append(existing, filepath.Base(p))
			}
		}
		if len(existing) > 0 {
			list := strings.Join(existing[:min(len(existing), 8)], "\n")
			if len(existing) > 8 {
				list += fmt.Sprintf("\n… und %d weitere", len(existing)-8)
			}
			if zenity.Question(fmt.Sprintf("%d Datei(en) gibt es im Projekt schon:\n\n%s\n\nÜberschreiben?", len(existing), list),
				zenity.Title("Dateien überschreiben?"), zenity.WarningIcon,
				zenity.OKLabel("Überschreiben"), zenity.CancelLabel("Abbrechen")) != nil {
				return nil
			}
		}
		var failed []string
		for _, p := range pngs {
			if err := copyFile(p, filepath.Join(projDir, filepath.Base(p))); err != nil {
				failed = append(failed, filepath.Base(p))
			}
		}
		return func() {
			if g.dir == projDir {
				g.reload(true)
				if len(g.frames) > 1 {
					g.playing = true
				}
			} else {
				g.openProject(projDir, nil)
			}
			if len(failed) > 0 {
				g.setStatus("Nicht kopiert: "+strings.Join(failed, ", "), true)
			} else {
				g.setStatus(fmt.Sprintf("%d Datei(en) ins Projekt kopiert", len(pngs)), false)
			}
		}
	})
}

func (g *Game) openFilesDirect(pngs []string) {
	dir := filepath.Dir(pngs[0])
	all, _ := listPNGs(dir)
	if len(all) == len(pngs) {
		g.openProject(dir, nil)
		return
	}
	g.openProject(dir, pngs)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := dst + ".tmp-copy"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, dst)
}

func openInExplorer(dir string) {
	if dir == "" {
		return
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("explorer", dir)
	case "darwin":
		cmd = exec.Command("open", dir)
	default:
		cmd = exec.Command("xdg-open", dir)
	}
	_ = cmd.Start()
}
