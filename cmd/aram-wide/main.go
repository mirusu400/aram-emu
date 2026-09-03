// Command aram-wide is a throwaway widescreen probe. It boots a title through
// the ordinary integration backend (so cheats and the carrier-auth responder
// apply), optionally at an overridden framebuffer width via ARAM_FB_WIDTH/
// ARAM_FB_HEIGHT, drives a fixed input sequence, and writes framebuffer PNGs
// plus a painted-extent report so we can see how much of a wider canvas the
// guest actually fills.
package main

import (
	"context"
	"flag"
	"fmt"
	"image"
	"image/png"
	"os"
	"time"

	"github.com/mirusu400/aram-emu/integration"
	"github.com/mirusu400/aram-frontend/frontend"
)

func main() {
	input := flag.String("input", "", "path to the title archive")
	outPrefix := flag.String("out", "wide", "output PNG path prefix")
	width := flag.Int("width", 0, "framebuffer width override (0 = device default)")
	height := flag.Int("height", 0, "framebuffer height override (0 = device default)")
	frames := flag.Int("frames", 900, "total frames to run")
	timeout := flag.Duration("timeout", 120*time.Second, "whole-run timeout")
	flag.Parse()

	if *input == "" {
		fmt.Fprintln(os.Stderr, "aram-wide: -input is required")
		os.Exit(2)
	}
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	backend := integration.NewBackend(nil)
	defer backend.Close()

	// Exercise the real frontend-facing API the Experiments panel uses.
	if *width > 0 && *height > 0 {
		if dc, ok := interface{}(backend).(frontend.DisplayConfigurator); ok {
			if err := dc.ConfigureDisplay(frontend.DisplaySettings{Width: *width, Height: *height}); err != nil {
				fmt.Fprintf(os.Stderr, "ConfigureDisplay: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("ConfigureDisplay %dx%d applied via frontend API\n", *width, *height)
		}
	}

	info, err := backend.OpenWithProgress(ctx, frontend.OpenRequest{
		Path:        *input,
		DisplayName: "wide-probe",
	}, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("opened: format=%s profile=%s sha=%s\n", info.Format, info.ProfileID, info.SHA256)

	if err := backend.Execute(ctx, frontend.CommandStart); err != nil {
		fmt.Fprintf(os.Stderr, "start: %v\n", err)
		os.Exit(1)
	}

	// Frames at which to tap a key, to advance logo -> title -> menu -> game.
	presses := map[int]string{}
	// Splash/title/menu/intro: hammer ok to blow through every cutscene + dialog.
	for f := 80; f <= 3200; f += 24 {
		presses[f] = "ok"
	}
	// Then wander to load and scroll around an outdoor map.
	dirs := []string{"right", "right", "down", "left", "left", "up", "right", "down"}
	di := 0
	for f := 3300; f <= *frames-40; f += 12 {
		presses[f] = dirs[di%len(dirs)]
		di++
	}
	// Snapshot every `shotEvery` frames plus the last frame.
	shotEvery := 400
	shots := map[int]bool{}
	for f := 400; f < *frames; f += shotEvery {
		shots[f] = true
	}
	if *frames-1 > 0 {
		shots[*frames-1] = true
	}

	firstPaint := -1
	bestRT, bestRTFrame := 0, -1
	for f := 0; f < *frames; f++ {
		if control, ok := presses[f]; ok && backend.State() == frontend.StateRunning {
			_ = backend.QueueInput(frontend.InputEvent{Control: control, Pressed: true})
		}
		if control, ok := presses[f-4]; ok && backend.State() == frontend.StateRunning {
			_ = backend.QueueInput(frontend.InputEvent{Control: control, Pressed: false})
		}
		if err := backend.RunFrame(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "frame %d: %v\n", f, err)
			break
		}
		img := backend.VideoFrame().Image
		extent, _, nonBlack := paintedExtent(img)
		if nonBlack > 0 && firstPaint < 0 {
			firstPaint = f
			fmt.Printf("first paint at frame %d\n", f)
		}
		// World content in the right 30% of the canvas (HUD like #SKIP is tiny,
		// so a large value means real world tiles reached the far right).
		rt := rightThirdFill(img)
		if rt > bestRT {
			bestRT, bestRTFrame = rt, f
		}
		if shots[f] {
			fmt.Printf("  [right30%%_fill=%d]\n", rt)
		}
		if shots[f] {
			b := img.Bounds()
			out := fmt.Sprintf("%s_f%d_%dx%d.png", *outPrefix, f, b.Dx(), b.Dy())
			if err := writePNG(out, img); err != nil {
				fmt.Fprintf(os.Stderr, "write %s: %v\n", out, err)
			} else {
				pct := 0.0
				if b.Dx() > 0 {
					pct = 100 * float64(extent+1) / float64(b.Dx())
				}
				fmt.Printf("shot f=%d %dx%d non_black=%d painted_to_x=%d (%.0f%% of width) -> %s\n",
					f, b.Dx(), b.Dy(), nonBlack, extent, pct, out)
			}
		}
	}
	fmt.Printf("done: first_paint=%d state=%s best_right30_fill=%d at frame %d\n",
		firstPaint, backend.State(), bestRT, bestRTFrame)
	// Snapshot the frame that had the most world content on the right, if we can
	// reproduce it is not trivial; instead report it so we can re-run to that frame.
}

// rightThirdFill counts non-black pixels in the right 30% of the image, minus
// the bottom 24 rows (where the #SKIP HUD lives). A large value means real
// world tiles reached the far-right region rather than just a HUD glyph.
func rightThirdFill(img image.Image) int {
	b := img.Bounds()
	x0 := b.Min.X + (b.Dx()*7)/10
	rgba, ok := img.(*image.RGBA)
	n := 0
	if !ok {
		for y := b.Min.Y; y < b.Max.Y-24; y++ {
			for x := x0; x < b.Max.X; x++ {
				r, g, bl, _ := img.At(x, y).RGBA()
				if r|g|bl != 0 {
					n++
				}
			}
		}
		return n
	}
	for y := b.Min.Y; y < b.Max.Y-24; y++ {
		for x := x0; x < b.Max.X; x++ {
			o := rgba.PixOffset(x, y)
			if rgba.Pix[o] != 0 || rgba.Pix[o+1] != 0 || rgba.Pix[o+2] != 0 {
				n++
			}
		}
	}
	return n
}

func paintedExtent(img image.Image) (maxX, maxY, nonBlack int) {
	maxX, maxY = -1, -1
	rgba, ok := img.(*image.RGBA)
	b := img.Bounds()
	if !ok {
		for y := b.Min.Y; y < b.Max.Y; y++ {
			for x := b.Min.X; x < b.Max.X; x++ {
				r, g, bl, _ := img.At(x, y).RGBA()
				if r|g|bl != 0 {
					nonBlack++
					if x > maxX {
						maxX = x
					}
					if y > maxY {
						maxY = y
					}
				}
			}
		}
		return
	}
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			o := rgba.PixOffset(x, y)
			if rgba.Pix[o] != 0 || rgba.Pix[o+1] != 0 || rgba.Pix[o+2] != 0 {
				nonBlack++
				if x > maxX {
					maxX = x
				}
				if y > maxY {
					maxY = y
				}
			}
		}
	}
	return
}

func writePNG(path string, img image.Image) error {
	fh, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := png.Encode(fh, img); err != nil {
		_ = fh.Close()
		return err
	}
	return fh.Close()
}
