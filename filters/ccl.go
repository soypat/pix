package filters

import (
	"image"

	"github.com/soypat/pix"
)

// Component describes a connected component found by [Labeler].
type Component struct {
	Area   int             // Number of pixels.
	Bounds image.Rectangle // Bounding box.
	First  image.Point     // First pixel in raster order (top row, leftmost).
}

// Labeler finds the 8-connected components of the nonzero pixels of a [pix.ShapeGray8] image
// with two passes and union-find. Labels are 1..n in raster order of each component's first
// pixel; 0 is background. Memory grows to the largest image labeled and is then reused.
type Labeler struct {
	labels []int32
	parent []int32
	remap  []int32
	comps  []Component
	w, h   int
}

// Label labels src and returns the number of components.
func (l *Labeler) Label(src *pix.Buffer) (int, error) {
	d := src.Dims()
	if d.Shape != pix.ShapeGray8 {
		return 0, errUnsupportedShape
	}
	w, h := d.Width, d.Height
	l.w, l.h = w, h
	l.labels = grow(l.labels, w*h)
	maxProvisional := ((w+1)/2)*((h+1)/2) + 1
	l.parent = grow(l.parent, maxProvisional+1)
	l.remap = grow(l.remap, maxProvisional+1)
	next := int32(1)
	for y := 0; y < h; y++ {
		row := src.Row(y)
		lab := l.labels[y*w : (y+1)*w]
		var up []int32
		if y > 0 {
			up = l.labels[(y-1)*w : y*w]
		}
		for x, v := range row {
			if v == 0 {
				lab[x] = 0
				continue
			}
			// Neighbors already visited: W, NW, N, NE.
			m := int32(0)
			if x > 0 && lab[x-1] != 0 {
				m = lab[x-1]
			}
			if up != nil {
				for dx := -1; dx <= 1; dx++ {
					if nx := x + dx; nx >= 0 && nx < w && up[nx] != 0 {
						if m == 0 {
							m = up[nx]
						} else if m != up[nx] {
							l.union(m, up[nx])
						}
					}
				}
			}
			if m == 0 {
				m = next
				l.parent[m] = m
				next++
			}
			lab[x] = m
		}
	}
	// Resolve roots to compact labels in raster order of first appearance.
	clear(l.remap[:next])
	l.comps = l.comps[:0]
	for y := 0; y < h; y++ {
		lab := l.labels[y*w : (y+1)*w]
		for x, p := range lab {
			if p == 0 {
				continue
			}
			root := l.find(p)
			c := l.remap[root]
			if c == 0 {
				l.comps = append(l.comps, Component{Bounds: image.Rect(x, y, x+1, y+1), First: image.Pt(x, y)})
				c = int32(len(l.comps))
				l.remap[root] = c
			}
			lab[x] = c
			comp := &l.comps[c-1]
			comp.Area++
			if x < comp.Bounds.Min.X {
				comp.Bounds.Min.X = x
			}
			if x >= comp.Bounds.Max.X {
				comp.Bounds.Max.X = x + 1
			}
			comp.Bounds.Max.Y = y + 1
		}
	}
	return len(l.comps), nil
}

func (l *Labeler) find(a int32) int32 {
	for l.parent[a] != a {
		l.parent[a] = l.parent[l.parent[a]]
		a = l.parent[a]
	}
	return a
}

func (l *Labeler) union(a, b int32) {
	ra, rb := l.find(a), l.find(b)
	if ra == rb {
		return
	}
	if ra < rb {
		l.parent[rb] = ra
	} else {
		l.parent[ra] = rb
	}
}

// Labels returns the label of every pixel of the last labeled image, row major with no padding.
func (l *Labeler) Labels() []int32 { return l.labels[:l.w*l.h] }

// Size returns the dimensions of the last labeled image.
func (l *Labeler) Size() (w, h int) { return l.w, l.h }

// Components returns the components of the last labeled image: label i is Components()[i-1].
func (l *Labeler) Components() []Component { return l.comps }
