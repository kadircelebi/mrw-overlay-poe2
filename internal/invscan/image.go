// Package invscan reads stackable items off a screenshot of an item grid
// (the inventory, a stash tab): it finds the cells, tells which item each
// filled cell shows by comparing it with the items' icons, and reads the
// numbers the game writes on them.
package invscan

import (
	"image"
	"image/color"
	"math"
)

// Img is an RGB picture as floats (0–255), row by row, with an optional
// per-pixel weight: icons weigh 0 where they are transparent or where the
// game writes numbers over them.
type Img struct {
	W, H int
	Pix  []float32 // R, G, B per pixel
	Mask []float32 // nil: every pixel counts
}

func newImg(w, h int) Img { return Img{W: w, H: h, Pix: make([]float32, w*h*3)} }

// FromImage copies r of src.
func FromImage(src image.Image, r image.Rectangle) Img {
	r = r.Intersect(src.Bounds())
	out := newImg(r.Dx(), r.Dy())
	if rgba, ok := src.(*image.RGBA); ok {
		for y := 0; y < out.H; y++ {
			row := rgba.Pix[rgba.PixOffset(r.Min.X, r.Min.Y+y):]
			for x := 0; x < out.W; x++ {
				i := (y*out.W + x) * 3
				out.Pix[i], out.Pix[i+1], out.Pix[i+2] = float32(row[x*4]), float32(row[x*4+1]), float32(row[x*4+2])
			}
		}
		return out
	}
	for y := 0; y < out.H; y++ {
		for x := 0; x < out.W; x++ {
			c := color.RGBAModel.Convert(src.At(r.Min.X+x, r.Min.Y+y)).(color.RGBA)
			i := (y*out.W + x) * 3
			out.Pix[i], out.Pix[i+1], out.Pix[i+2] = float32(c.R), float32(c.G), float32(c.B)
		}
	}
	return out
}

// fromIcon copies an icon with its alpha channel as the mask (opaque where
// alpha is over half).
func fromIcon(src image.Image) Img {
	b := src.Bounds()
	out := newImg(b.Dx(), b.Dy())
	out.Mask = make([]float32, out.W*out.H)
	for y := 0; y < out.H; y++ {
		for x := 0; x < out.W; x++ {
			c := color.NRGBAModel.Convert(src.At(b.Min.X+x, b.Min.Y+y)).(color.NRGBA)
			i := y*out.W + x
			out.Pix[i*3], out.Pix[i*3+1], out.Pix[i*3+2] = float32(c.R), float32(c.G), float32(c.B)
			if c.A > 127 {
				out.Mask[i] = 1
			}
		}
	}
	return out
}

// Resize scales m to w×h: each output pixel averages the source area it
// covers (bilinear when enlarging). The mask follows the same way.
func (m Img) Resize(w, h int) Img {
	out := newImg(w, h)
	if m.Mask != nil {
		out.Mask = make([]float32, w*h)
	}
	sx, sy := float64(m.W)/float64(w), float64(m.H)/float64(h)
	for y := 0; y < h; y++ {
		y0, y1 := float64(y)*sy, float64(y+1)*sy
		for x := 0; x < w; x++ {
			x0, x1 := float64(x)*sx, float64(x+1)*sx
			var r, g, b, a, total float64
			if sx <= 1 && sy <= 1 {
				// Enlarging: bilinear around the output pixel's centre.
				fx, fy := (float64(x)+0.5)*sx-0.5, (float64(y)+0.5)*sy-0.5
				ix, iy := int(math.Floor(fx)), int(math.Floor(fy))
				tx, ty := fx-float64(ix), fy-float64(iy)
				for _, p := range [4]struct {
					dx, dy int
					w      float64
				}{{0, 0, (1 - tx) * (1 - ty)}, {1, 0, tx * (1 - ty)}, {0, 1, (1 - tx) * ty}, {1, 1, tx * ty}} {
					px, py := clamp(ix+p.dx, m.W-1), clamp(iy+p.dy, m.H-1)
					i := py*m.W + px
					r += p.w * float64(m.Pix[i*3])
					g += p.w * float64(m.Pix[i*3+1])
					b += p.w * float64(m.Pix[i*3+2])
					if m.Mask != nil {
						a += p.w * float64(m.Mask[i])
					}
					total += p.w
				}
			} else {
				for py := int(y0); py < int(math.Ceil(y1)) && py < m.H; py++ {
					wy := math.Min(y1, float64(py+1)) - math.Max(y0, float64(py))
					for px := int(x0); px < int(math.Ceil(x1)) && px < m.W; px++ {
						wx := math.Min(x1, float64(px+1)) - math.Max(x0, float64(px))
						wgt := wx * wy
						i := py*m.W + px
						r += wgt * float64(m.Pix[i*3])
						g += wgt * float64(m.Pix[i*3+1])
						b += wgt * float64(m.Pix[i*3+2])
						if m.Mask != nil {
							a += wgt * float64(m.Mask[i])
						}
						total += wgt
					}
				}
			}
			o := y*w + x
			out.Pix[o*3], out.Pix[o*3+1], out.Pix[o*3+2] = float32(r/total), float32(g/total), float32(b/total)
			if out.Mask != nil {
				if a/total > 0.5 {
					out.Mask[o] = 1
				}
			}
		}
	}
	return out
}

func clamp(v, hi int) int { return max(0, min(v, hi)) }

// opaqueBounds is the box of the mask's set pixels (icons come with
// different margins: the trade site's 64-pixel ones and poe2db's 108).
func (m Img) opaqueBounds() image.Rectangle {
	r := image.Rectangle{Min: image.Pt(m.W, m.H)}
	for y := 0; y < m.H; y++ {
		for x := 0; x < m.W; x++ {
			if m.Mask == nil || m.Mask[y*m.W+x] > 0 {
				r.Min.X, r.Min.Y = min(r.Min.X, x), min(r.Min.Y, y)
				r.Max.X, r.Max.Y = max(r.Max.X, x+1), max(r.Max.Y, y+1)
			}
		}
	}
	if r.Empty() {
		return image.Rect(0, 0, m.W, m.H)
	}
	return r
}

// Picture is r of m as an image, for keeping what a read saw.
func (m Img) Picture(r image.Rectangle) *image.RGBA {
	c := m.crop(r)
	out := image.NewRGBA(image.Rect(0, 0, c.W, c.H))
	for i := 0; i < c.W*c.H; i++ {
		for k := 0; k < 3; k++ {
			out.Pix[i*4+k] = uint8(max(0, min(255, c.Pix[i*3+k]+0.5)))
		}
		out.Pix[i*4+3] = 255
	}
	return out
}

// crop copies r of m (mask included).
func (m Img) crop(r image.Rectangle) Img {
	r = r.Intersect(image.Rect(0, 0, m.W, m.H))
	out := newImg(r.Dx(), r.Dy())
	if m.Mask != nil {
		out.Mask = make([]float32, out.W*out.H)
	}
	for y := 0; y < out.H; y++ {
		copy(out.Pix[y*out.W*3:(y+1)*out.W*3], m.Pix[((r.Min.Y+y)*m.W+r.Min.X)*3:])
		if m.Mask != nil {
			copy(out.Mask[y*out.W:(y+1)*out.W], m.Mask[(r.Min.Y+y)*m.W+r.Min.X:])
		}
	}
	return out
}

// NCC is the normalized cross-correlation of t (with its mask as weights)
// against w×h windows of m whose top-left corner moves over every offset
// in [0, m.W−t.W]×[0, m.H−t.H] (step 1). It returns the best score (1 =
// the same picture up to brightness and contrast) and where it was.
func NCC(m, t Img) (best float64, at image.Point) {
	return nccIn(m, t, image.Rect(0, 0, m.W-t.W+1, m.H-t.H+1))
}

// nccIn is NCC over the top-left corners in r only.
func nccIn(m, t Img, r image.Rectangle) (best float64, at image.Point) {
	p := prepare(t)
	r = r.Intersect(image.Rect(0, 0, m.W-t.W+1, m.H-t.H+1))
	best = -1
	if !p.ok {
		return best, at
	}
	for oy := r.Min.Y; oy < r.Max.Y; oy++ {
		for ox := r.Min.X; ox < r.Max.X; ox++ {
			if s := p.at(m, ox, oy); s > best {
				best, at = s, image.Pt(ox, oy)
			}
		}
	}
	return best, at
}

// prepared is a template's statistics, worked out once for many places:
// its weights, its values centred on their weighted mean (over the three
// channels) and weighted, and their spread.
type prepared struct {
	w, h   int
	tc, wt []float64
	sw, vt float64
	ok     bool
	// grey: the template's three channels are the same, and so are the
	// pictures it is matched in; only the first channel is read.
	grey bool
}

func prepare(t Img) prepared {
	n := t.W * t.H
	p := prepared{w: t.W, h: t.H, tc: make([]float64, n*3), wt: make([]float64, n)}
	var st float64
	for i := 0; i < n; i++ {
		w := 1.0
		if t.Mask != nil {
			w = float64(t.Mask[i])
		}
		p.wt[i] = w
		p.sw += 3 * w
		st += w * float64(t.Pix[i*3]+t.Pix[i*3+1]+t.Pix[i*3+2])
	}
	if p.sw == 0 {
		return p
	}
	mean := st / p.sw
	for i := 0; i < n; i++ {
		w := p.wt[i]
		for c := 0; c < 3; c++ {
			d := w * (float64(t.Pix[i*3+c]) - mean)
			p.tc[i*3+c] = d
			p.vt += d * d / math.Max(w, 1e-9)
		}
	}
	p.ok = p.vt > 1e-6
	return p
}

// prepareGrey is prepare for a grey template matched in grey pictures.
func prepareGrey(t Img) prepared {
	n := t.W * t.H
	p := prepared{w: t.W, h: t.H, tc: make([]float64, n), wt: make([]float64, n), grey: true}
	var st float64
	for i := 0; i < n; i++ {
		w := 1.0
		if t.Mask != nil {
			w = float64(t.Mask[i])
		}
		p.wt[i] = w
		p.sw += w
		st += w * float64(t.Pix[i*3])
	}
	if p.sw == 0 {
		return p
	}
	mean := st / p.sw
	for i := 0; i < n; i++ {
		d := p.wt[i] * (float64(t.Pix[i*3]) - mean)
		p.tc[i] = d
		p.vt += d * d / math.Max(p.wt[i], 1e-9)
	}
	p.ok = p.vt > 1e-6
	return p
}

// at is the template's NCC with the window of m whose top-left corner is
// (ox, oy), or -1 where m is flat there.
func (p prepared) at(m Img, ox, oy int) float64 {
	if p.grey {
		return p.atGrey(m, ox, oy)
	}
	var num, sx, sxx float64
	for y := 0; y < p.h; y++ {
		row := m.Pix[((oy+y)*m.W+ox)*3:]
		for x := 0; x < p.w; x++ {
			i := y*p.w + x
			w := p.wt[i]
			if w == 0 {
				continue
			}
			r, g, b := float64(row[x*3]), float64(row[x*3+1]), float64(row[x*3+2])
			num += r*p.tc[i*3] + g*p.tc[i*3+1] + b*p.tc[i*3+2]
			sx += w * (r + g + b)
			sxx += w * (r*r + g*g + b*b)
		}
	}
	vx := sxx - sx*sx/p.sw
	if vx <= 1e-6 {
		return -1
	}
	return num / math.Sqrt(vx*p.vt)
}

func (p prepared) atGrey(m Img, ox, oy int) float64 {
	var num, sx, sxx float64
	for y := 0; y < p.h; y++ {
		row := m.Pix[((oy+y)*m.W+ox)*3:]
		base := y * p.w
		for x := 0; x < p.w; x++ {
			w := p.wt[base+x]
			if w == 0 {
				continue
			}
			v := float64(row[x*3])
			num += v * p.tc[base+x]
			sx += w * v
			sxx += w * v * v
		}
	}
	vx := sxx - sx*sx/p.sw
	if vx <= 1e-6 {
		return -1
	}
	return num / math.Sqrt(vx*p.vt)
}

// greyWeight is what a grey pixel adds to the colour signature's grey bin,
// against up to 1 for a coloured one: liquids are grey smoke around a small
// red, orange or green stone, and counted in full the smoke drowns it.
const greyWeight = 0.1

// hueHist is a colour signature: 36 hue bins weighted by saturation and
// brightness for coloured pixels, and one bin for grey ones, unit length.
// Pixels with a zero mask weight (or below the darkness floor) are left out.
func hueHist(m Img, mask []float32) [37]float64 {
	var h [37]float64
	for i := 0; i < m.W*m.H; i++ {
		if mask != nil && mask[i] == 0 {
			continue
		}
		r, g, b := float64(m.Pix[i*3])/255, float64(m.Pix[i*3+1])/255, float64(m.Pix[i*3+2])/255
		mx, mn := math.Max(r, math.Max(g, b)), math.Min(r, math.Min(g, b))
		if mx <= 0.2 {
			continue
		}
		s := (mx - mn) / mx
		if s <= 0.2 {
			h[36] += greyWeight
			continue
		}
		d := mx - mn
		var hue float64
		switch mx {
		case r:
			hue = math.Mod((g-b)/d, 6)
		case g:
			hue = (b-r)/d + 2
		default:
			hue = (r-g)/d + 4
		}
		hue *= 60
		if hue < 0 {
			hue += 360
		}
		h[min(int(hue/10), 35)] += s * mx
	}
	var norm float64
	for _, v := range h {
		norm += v * v
	}
	if norm > 0 {
		norm = math.Sqrt(norm)
		for i := range h {
			h[i] /= norm
		}
	}
	return h
}

func dot(a, b [37]float64) float64 {
	var s float64
	for i := range a {
		s += a[i] * b[i]
	}
	return s
}
