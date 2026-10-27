// Package photo turns an uploaded picture into the JPEGs stored for a recipe
// (#207): the photo, at most MaxSide px on its long side, and a square
// thumbnail. Re-encoding drops every metadata segment, EXIF and GPS included.
package photo

import (
	"bytes"
	"errors"
	"image"
	"image/jpeg"
	_ "image/png" // PNG for image.Decode
)

const (
	MaxSide   = 1200       // long side of the stored photo
	ThumbSide = 160        // side of the square thumbnail
	MaxPixels = 40_000_000 // larger pictures are refused before decoding them
	quality   = 82
)

var (
	// ErrFormat means the data is not a JPEG or PNG image.
	ErrFormat = errors.New("photo: not a JPEG or PNG image")
	// ErrDimensions means the picture has more than MaxPixels pixels.
	ErrDimensions = errors.New("photo: picture too large")
)

// Process decodes a JPEG or PNG and returns the photo and its center-cropped
// square thumbnail, both JPEG. A transparent background turns white.
func Process(data []byte) (photo, thumb []byte, err error) {
	// The header alone first: a small file can claim a huge picture.
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || (format != "jpeg" && format != "png") {
		return nil, nil, ErrFormat
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width*cfg.Height > MaxPixels {
		return nil, nil, ErrDimensions
	}
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, nil, ErrFormat
	}
	w, h := fit(src.Bounds().Dx(), src.Bounds().Dy(), MaxSide)
	big := resize(src, src.Bounds(), w, h)
	side := min(w, h)
	x0, y0 := (w-side)/2, (h-side)/2
	small := resize(big, image.Rect(x0, y0, x0+side, y0+side), ThumbSide, ThumbSide)
	if photo, err = encode(big); err != nil {
		return nil, nil, err
	}
	if thumb, err = encode(small); err != nil {
		return nil, nil, err
	}
	return photo, thumb, nil
}

// fit scales w×h down, keeping the ratio, so that neither side exceeds limit.
func fit(w, h, limit int) (int, int) {
	switch {
	case w <= limit && h <= limit:
		return w, h
	case w >= h:
		return limit, max(1, (h*limit+w/2)/w)
	default:
		return max(1, (w*limit+h/2)/h), limit
	}
}

// resize scales the r part of src to dw×dh, every destination pixel the
// average of the source pixels it covers (a box filter), over white.
func resize(src image.Image, r image.Rectangle, dw, dh int) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	for dy := 0; dy < dh; dy++ {
		y0, y1 := span(dy, dh, r.Dy())
		for dx := 0; dx < dw; dx++ {
			x0, x1 := span(dx, dw, r.Dx())
			var sr, sg, sb, sa uint64
			for y := y0; y < y1; y++ {
				for x := x0; x < x1; x++ {
					cr, cg, cb, ca := src.At(r.Min.X+x, r.Min.Y+y).RGBA()
					sr, sg, sb, sa = sr+uint64(cr), sg+uint64(cg), sb+uint64(cb), sa+uint64(ca)
				}
			}
			n := uint64((x1 - x0) * (y1 - y0))
			white := 0xffff*n - sa // premultiplied colors over white: c + (1 - alpha)
			i := dst.PixOffset(dx, dy)
			dst.Pix[i] = uint8((sr + white) / n >> 8)
			dst.Pix[i+1] = uint8((sg + white) / n >> 8)
			dst.Pix[i+2] = uint8((sb + white) / n >> 8)
			dst.Pix[i+3] = 0xff
		}
	}
	return dst
}

// span is the range [lo, hi) of the m source pixels that destination pixel i
// of n covers; never empty, so an enlargement repeats pixels.
func span(i, n, m int) (int, int) {
	lo, hi := i*m/n, (i+1)*m/n
	if hi <= lo {
		hi = lo + 1
	}
	return lo, hi
}

func encode(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
