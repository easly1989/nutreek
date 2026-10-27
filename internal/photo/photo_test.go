package photo

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"testing"
	"time"
)

// withEXIF puts an APP1 (EXIF) segment holding secret right after the SOI.
func withEXIF(t *testing.T, jpg []byte, secret string) []byte {
	t.Helper()
	payload := append([]byte("Exif\x00\x00"), secret...)
	seg := []byte{0xFF, 0xE1, 0, 0}
	binary.BigEndian.PutUint16(seg[2:], uint16(len(payload)+2))
	out := append([]byte{}, jpg[:2]...)
	out = append(out, seg...)
	out = append(out, payload...)
	return append(out, jpg[2:]...)
}

func solid(w, h int, c color.Color) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, c)
		}
	}
	return img
}

func size(t *testing.T, b []byte) image.Point {
	t.Helper()
	cfg, format, err := image.DecodeConfig(bytes.NewReader(b))
	if err != nil || format != "jpeg" {
		t.Fatalf("output is not a JPEG: %q, %v", format, err)
	}
	return image.Pt(cfg.Width, cfg.Height)
}

func TestProcessJPEG(t *testing.T) {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, solid(2400, 1600, color.RGBA{200, 80, 40, 255}), nil); err != nil {
		t.Fatal(err)
	}
	in := withEXIF(t, buf.Bytes(), "GPS 45.4642N 9.1900E")
	if _, err := jpeg.Decode(bytes.NewReader(in)); err != nil {
		t.Fatalf("the test JPEG does not decode: %v", err)
	}

	photo, thumb, err := Process(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if got := size(t, photo); got != image.Pt(1200, 800) {
		t.Errorf("photo %v, want 1200x800", got)
	}
	if got := size(t, thumb); got != image.Pt(ThumbSide, ThumbSide) {
		t.Errorf("thumb %v, want %dx%d", got, ThumbSide, ThumbSide)
	}
	for _, out := range [][]byte{photo, thumb} {
		if bytes.Contains(out, []byte("Exif")) || bytes.Contains(out, []byte("GPS")) || bytes.Contains(out, []byte{0xFF, 0xE1}) {
			t.Error("the EXIF segment survived")
		}
	}
}

func TestProcessPNG(t *testing.T) {
	img := solid(300, 900, color.RGBA{}) // fully transparent, portrait
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	photo, thumb, err := Process(context.Background(), buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if got := size(t, photo); got != image.Pt(300, 900) {
		t.Errorf("photo %v, want 300x900: small pictures keep their size", got)
	}
	out, err := jpeg.Decode(bytes.NewReader(thumb))
	if err != nil {
		t.Fatal(err)
	}
	if r, g, b, _ := out.At(80, 80).RGBA(); r>>8 < 250 || g>>8 < 250 || b>>8 < 250 {
		t.Errorf("transparent pixel came out %d,%d,%d, want white", r>>8, g>>8, b>>8)
	}
}

func TestProcessRefuses(t *testing.T) {
	var g bytes.Buffer
	if err := gif.Encode(&g, solid(10, 10, color.Black), nil); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		data []byte
		want error
	}{
		{"text", []byte("not a picture"), ErrFormat},
		{"gif", g.Bytes(), ErrFormat},
		{"truncated jpeg", []byte{0xFF, 0xD8, 0xFF}, ErrFormat},
		{"png over 16 MP", pngHeader(4000, 4001), ErrDimensions},
		// At the limit the header passes; the body is missing.
		{"png of 16 MP", pngHeader(4000, 4000), ErrFormat},
	}
	for _, c := range cases {
		if _, _, err := Process(context.Background(), c.data); !errors.Is(err, c.want) {
			t.Errorf("%s: %v, want %v", c.name, err, c.want)
		}
	}
}

// A call waiting for its turn gives up when its request ends.
func TestProcessWaitsForItsTurn(t *testing.T) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, solid(10, 10, color.Black)); err != nil {
		t.Fatal(err)
	}
	slot <- struct{}{} // another call is decoding
	defer func() { <-slot }()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, _, err := Process(ctx, buf.Bytes())
		done <- err
	}()
	select {
	case err := <-done:
		t.Fatalf("Process did not wait for the slot: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("cancelled while waiting: %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Process kept waiting after its context ended")
	}
}

// pngHeader is a PNG that stops after its header: enough for DecodeConfig.
func pngHeader(w, h uint32) []byte {
	data := make([]byte, 13)
	binary.BigEndian.PutUint32(data[0:], w)
	binary.BigEndian.PutUint32(data[4:], h)
	data[8], data[9] = 8, 6 // 8-bit RGBA
	chunk := append([]byte("IHDR"), data...)
	out := []byte("\x89PNG\r\n\x1a\n")
	out = binary.BigEndian.AppendUint32(out, uint32(len(data)))
	out = append(out, chunk...)
	return binary.BigEndian.AppendUint32(out, crc32.ChecksumIEEE(chunk))
}

func TestResizeAverages(t *testing.T) {
	// A 4x2 checkerboard of black and white shrinks to two mid-gray pixels.
	src := image.NewGray(image.Rect(0, 0, 4, 2))
	for i := range src.Pix {
		if (i%4+i/4)%2 == 0 {
			src.Pix[i] = 0xff
		}
	}
	dst := resize(src, src.Bounds(), 2, 1)
	for x := 0; x < 2; x++ {
		if v := dst.RGBAAt(x, 0).R; v < 126 || v > 129 {
			t.Errorf("pixel %d = %d, want about 127", x, v)
		}
	}
	// The source rectangle is honored: its right half is white.
	half := image.NewGray(image.Rect(0, 0, 4, 1))
	half.Pix[2], half.Pix[3] = 0xff, 0xff
	if v := resize(half, image.Rect(2, 0, 4, 1), 1, 1).RGBAAt(0, 0).R; v != 0xff {
		t.Errorf("crop of the white half = %d, want 255", v)
	}
}

func TestFit(t *testing.T) {
	cases := []struct{ w, h, ww, wh int }{
		{4000, 3000, 1200, 900},
		{3000, 4000, 900, 1200},
		{1200, 1200, 1200, 1200},
		{640, 480, 640, 480},
		{100000, 10, 1200, 1},
	}
	for _, c := range cases {
		if w, h := fit(c.w, c.h, MaxSide); w != c.ww || h != c.wh {
			t.Errorf("fit(%d, %d) = %dx%d, want %dx%d", c.w, c.h, w, h, c.ww, c.wh)
		}
	}
}
