package main

import (
	"reflect"
	"testing"
)

func TestNaturalSort(t *testing.T) {
	in := []string{"walk_10.png", "Walk_2.png", "walk_1.png", "walk_02b.png", "idle.png", "walk_003.png"}
	sortNatural(in)
	want := []string{"idle.png", "walk_1.png", "Walk_2.png", "walk_02b.png", "walk_003.png", "walk_10.png"}
	if !reflect.DeepEqual(in, want) {
		t.Fatalf("got %v", in)
	}
}

func TestParseSize(t *testing.T) {
	for s, want := range map[string][2]int{"64x64": {64, 64}, " 128 x 96 ": {128, 96}, "32×16": {32, 16}, "48": {48, 48}, "10*20": {10, 20}} {
		w, h, err := parseSize(s)
		if err != nil || w != want[0] || h != want[1] {
			t.Errorf("%q -> %d %d %v", s, w, h, err)
		}
	}
	for _, s := range []string{"", "x", "0x5", "abc", "1x2x3", "9000x1"} {
		if _, _, err := parseSize(s); err == nil {
			t.Errorf("%q should fail", s)
		}
	}
}

func TestAdvanceModes(t *testing.T) {
	g := NewGame()
	g.frames = make([]Frame, 4)
	seq := func(m PlayMode, steps int) []int {
		g.settings.Mode, g.cur, g.pingDir, g.playing = m, 0, 1, true
		var out []int
		for i := 0; i < steps; i++ {
			g.advance()
			out = append(out, g.cur)
		}
		return out
	}
	if got := seq(ModeLoop, 6); !reflect.DeepEqual(got, []int{1, 2, 3, 0, 1, 2}) {
		t.Errorf("loop %v", got)
	}
	if got := seq(ModePingPong, 8); !reflect.DeepEqual(got, []int{1, 2, 3, 2, 1, 0, 1, 2}) {
		t.Errorf("ping %v", got)
	}
	if got := seq(ModeOnce, 5); !reflect.DeepEqual(got, []int{1, 2, 3, 3, 3}) || g.playing {
		t.Errorf("once %v playing=%v", got, g.playing)
	}
}
