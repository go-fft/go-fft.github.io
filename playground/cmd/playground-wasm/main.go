// Copyright (c) the go-fft authors.
// SPDX-License-Identifier: BSD-3-Clause

//go:build js && wasm

// Command playground-wasm is the browser shell of the go-fft playground. It
// does three things and holds no logic: it sizes the toolkit to the screen's
// pixel ratio, publishes a few hooks the host page calls (the theme's
// colours, the clipboard, a paste, a debug probe), and hands the scene to
// go-widgets/webcanvas, which blits it into the <canvas> and routes the DOM's
// events back. Everything else is in the tagless playground package.
package main

import (
	"strconv"
	"strings"
	"syscall/js"

	playground "github.com/go-fft/go-fft.github.io/playground"
	"github.com/go-widgets/toolkit"
	"github.com/go-widgets/webcanvas"
)

const canvasID = "gofft-canvas"

func main() {
	dpr := 1.0
	if r := js.Global().Get("devicePixelRatio"); r.Type() == js.TypeNumber && r.Float() > 0 {
		dpr = r.Float()
	}
	playground.SetupText(dpr)

	vm := playground.NewViewModel()
	vm.Clipboard = func(s string) {
		if clip := js.Global().Get("navigator").Get("clipboard"); clip.Truthy() {
			clip.Call("writeText", s)
		}
	}
	canvas := js.Global().Get("document").Call("getElementById", canvasID)
	w, h := 1200, 800
	if canvas.Truthy() {
		w, h = canvas.Get("clientWidth").Int(), canvas.Get("clientHeight").Int()
	}
	scene := playground.NewScene(vm, max(int(float64(w)*dpr), 1), max(int(float64(h)*dpr), 1))

	// gofftSetPalette({bg, surface, ink, line, border, accent}) — CSS colour
	// strings read from the page's custom properties — recolours the canvas.
	js.Global().Set("gofftSetPalette", js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) == 1 {
			p := playground.LightPalette
			for key, dst := range map[string]*toolkit.RGBA{
				"bg": &p.Background, "surface": &p.Surface, "ink": &p.Ink,
				"line": &p.Line, "border": &p.Border, "accent": &p.Accent,
			} {
				if c, ok := parseColor(args[0].Get(key)); ok {
					*dst = c
				}
			}
			scene.SetPalette(p)
		}
		return nil
	}))
	// gofftPaste(text) inserts clipboard text where the keyboard is.
	js.Global().Set("gofftPaste", js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) == 1 && args[0].Type() == js.TypeString {
			return scene.Paste(args[0].String())
		}
		return false
	}))
	// gofftDebug() returns what a test harness asserts on: the control
	// rectangles in CSS pixels, and the derived text the view-model shows.
	js.Global().Set("gofftDebug", js.FuncOf(func(js.Value, []js.Value) any {
		rects := map[string]any{}
		for name, r := range scene.Rects() {
			rects[name] = []any{float64(r.X) / dpr, float64(r.Y) / dpr, float64(r.W) / dpr, float64(r.H) / dpr}
		}
		return map[string]any{
			"rects":     rects,
			"transform": vm.Transform.Get(),
			"preset":    vm.Preset.Get(),
			"n":         vm.N.Get(),
			"length":    vm.Length.Get(),
			"accuracy":  vm.Accuracy.Get(),
			"speed":     vm.Speed.Get(),
			"problem":   vm.Problem.Get(),
			"goCode":    vm.GoCode.Get(),
			"numpy":     vm.NumpyCode.Get(),
			"outTitle":  vm.OutputTitle.Get(),
			"outPoints": points(vm),
		}
	}))
	js.Global().Set("gofftPlaygroundReady", true)
	webcanvas.Run(canvasID, scene)
}

// points is the number of plotted output values.
func points(vm *playground.ViewModel) int {
	n := 0
	for _, c := range vm.OutputCurves.Slice() {
		n += len(c.Y)
	}
	return n
}

// parseColor reads "#rrggbb", "#rgb" or "rgb(r, g, b)" — what
// getComputedStyle returns for a custom property holding a colour.
func parseColor(v js.Value) (toolkit.RGBA, bool) {
	if v.Type() != js.TypeString {
		return toolkit.RGBA{}, false
	}
	s := strings.TrimSpace(v.String())
	if strings.HasPrefix(s, "#") {
		h := s[1:]
		if len(h) == 3 {
			h = string([]byte{h[0], h[0], h[1], h[1], h[2], h[2]})
		}
		if len(h) != 6 {
			return toolkit.RGBA{}, false
		}
		n, err := strconv.ParseUint(h, 16, 32)
		if err != nil {
			return toolkit.RGBA{}, false
		}
		return toolkit.RGB(uint8(n>>16), uint8(n>>8), uint8(n)), true
	}
	if i := strings.IndexByte(s, '('); i >= 0 && strings.HasSuffix(s, ")") {
		f := strings.FieldsFunc(s[i+1:len(s)-1], func(r rune) bool { return r == ',' || r == ' ' || r == '/' })
		if len(f) < 3 {
			return toolkit.RGBA{}, false
		}
		var c [3]uint8
		for k := range 3 {
			x, err := strconv.ParseFloat(f[k], 64)
			if err != nil {
				return toolkit.RGBA{}, false
			}
			c[k] = uint8(min(max(x, 0), 255))
		}
		return toolkit.RGB(c[0], c[1], c[2]), true
	}
	return toolkit.RGBA{}, false
}
