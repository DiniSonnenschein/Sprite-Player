# Sprite-Player

Spielt PNG-Bildfolgen (Sprite-Animationen) pixelgenau mit einstellbarer Bildrate ab.

## Start

`Sprite-Player.exe` per Doppelklick starten. Eine Installation ist nicht nötig.
Beim ersten Start zeigt Windows eventuell „Der Computer wurde durch Windows geschützt“.
Dann auf **Weitere Informationen** und danach auf **Trotzdem ausführen** klicken.

## Projekte

- **Neues Projekt …** fragt nach Titel und Leinwand-Größe (z. B. `64x64`) und danach nach dem Speicherort.
  Dort wird ein Ordner mit dem Titel angelegt. Darin speichert die Datei `sprite-projekt.json` Titel und Leinwand-Größe.
- Lege deine PNGs in diesen Ordner oder ziehe sie ins Fenster.
  Kommen die PNGs aus einem anderen Ordner, fragt das Programm, ob sie ins Projekt kopiert werden sollen.
- **Ordner öffnen …** oder einen Ordner ins Fenster ziehen öffnet ein Projekt.
  Ordner ohne Projektdatei funktionieren auch. Die Leinwand hat dann die Größe des ersten Bildes.
- Titel und Leinwand-Größe ändern: oben in der Leiste auf den Titel bzw. auf „Leinwand …“ klicken.
- Neue oder geänderte PNGs im Ordner werden nach etwa 1 Sekunde automatisch neu geladen.
- Beim nächsten Start wird das zuletzt geöffnete Projekt wieder geöffnet.

## Reihenfolge und Darstellung

- Die Bilder werden nach Dateinamen sortiert, Zahlen dabei als Zahlen: `lauf_2.png` kommt vor `lauf_10.png`.
- Bilder werden oben links an der Leinwand ausgerichtet. Was über die Leinwand hinausragt, wird abgeschnitten.
- Gezoomt wird nur in ganzen Stufen (1×, 2×, 3× …). So bleibt jedes Pixel scharf und gleich groß.

## Bedienung

| Aktion                        | Maus / Button              | Taste              |
|-------------------------------|----------------------------|--------------------|
| Abspielen / Pause             | ▶ / ❚❚                     | Leertaste          |
| Frame vor / zurück            | ▶❚ / ❚◀                    | → / ←              |
| Erster / letzter Frame        | ❚◀◀ / ▶▶❚                  | Pos1 / Ende        |
| Zu einem Frame springen       | Zeitleiste klicken/ziehen  |                    |
| FPS ändern (1–60)             | − / +, Mausrad über FPS    | ↑ / ↓ (Shift: ±5)  |
| Modus Schleife/Einmal/Ping-Pong | Buttons                  | M                  |
| Hintergrund Schwarz/Weiß      | Buttons                    | B                  |
| Zoom                          | − / +, Mausrad auf Bild    | + / −              |
| Einpassen / 1:1               | „Einpassen“, mittlere Maustaste | 0 / 1         |
| Bild verschieben              | Ziehen mit linker Maustaste |                   |
| Vollbild                      | „Vollbild“                 | F oder F11, Esc    |
| Neu laden                     |                            | F5                 |
| Neues Projekt / Ordner öffnen |                            | Strg+N / Strg+O    |

Im Vollbild blenden sich die Leisten aus und erscheinen wieder, sobald die Maus bewegt wird.

## Selbst bauen

Go 1.24 oder neuer ist nötig:

```
GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "-H windowsgui -s -w" -o Sprite-Player.exe .
```
