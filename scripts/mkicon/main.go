// Command mkicon genera assets/catchup.ico, el ícono del atajo de escritorio.
//
// Es un generador offline: se corre a mano (`go run ./scripts/mkicon`), el .ico
// resultante se commitea y ni el binario ni el instalador dependen de este
// programa. Solo stdlib — nada de resource compilers ni goversioninfo, que
// romperían el techo de dependencias del proyecto.
//
// La marca es el motivo de matriz: cuadrado de esquinas redondeadas en
// accent-800 con cuatro puntos en 2×2 (los cuadrantes), el de arriba a la
// derecha en blanco (el quick win). Los colores salen de svg.SectionPalette
// para no duplicar hex.
package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log"
	"os"
	"path/filepath"

	"projectmapper/internal/svg"
)

// sizes son los tamaños que Windows usa: lista de archivos (16), escritorio
// (32/48) y vistas grandes / escalado HiDPI (256).
var sizes = []int{16, 32, 48, 256}

func main() {
	out := flag.String("out", filepath.Join("assets", "catchup.ico"), "ruta del .ico a escribir")
	flag.Parse()

	var images [][]byte
	for _, size := range sizes {
		var buf bytes.Buffer
		if err := png.Encode(&buf, render(size)); err != nil {
			log.Fatalf("encode %dpx: %v", size, err)
		}
		images = append(images, buf.Bytes())
	}

	ico, err := buildICO(sizes, images)
	if err != nil {
		log.Fatalf("armar ico: %v", err)
	}
	if dir := filepath.Dir(*out); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			log.Fatalf("crear %s: %v", dir, err)
		}
	}
	if err := os.WriteFile(*out, ico, 0o644); err != nil {
		log.Fatalf("escribir %s: %v", *out, err)
	}
	fmt.Printf("listo: %s (%d bytes, tamaños %v)\n", *out, len(ico), sizes)
}

// render dibuja el ícono a size×size con supersampling 4× y promediado por
// caja, que es todo el antialiasing que necesita una figura de rectángulo
// redondeado + círculos (sin dependencias de dibujo).
func render(size int) *image.RGBA {
	const ss = 4
	var (
		bg     = mustColor(svg.SectionPalette[4]) // accent-800
		dot    = mustColor(svg.SectionPalette[0]) // accent-400
		hero   = color.RGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}
		big    = size * ss
		fbig   = float64(big)
		corner = 0.22 * fbig // radio de las esquinas del cuadrado
		dotR   = 0.115 * fbig
	)

	// Centros de los 4 puntos en 2×2, en fracciones del lienzo.
	type point struct {
		cx, cy float64
		c      color.RGBA
	}
	points := []point{
		{0.34, 0.34, dot},
		{0.66, 0.34, hero}, // quick win: alto impacto, bajo esfuerzo
		{0.34, 0.66, dot},
		{0.66, 0.66, dot},
	}

	super := image.NewRGBA(image.Rect(0, 0, big, big))
	for y := 0; y < big; y++ {
		for x := 0; x < big; x++ {
			px, py := float64(x)+0.5, float64(y)+0.5
			if !insideRoundedRect(px, py, fbig, corner) {
				continue // queda transparente
			}
			c := bg
			for _, p := range points {
				dx, dy := px-p.cx*fbig, py-p.cy*fbig
				if dx*dx+dy*dy <= dotR*dotR {
					c = p.c
					break
				}
			}
			super.SetRGBA(x, y, c)
		}
	}
	return downsample(super, size, ss)
}

// insideRoundedRect indica si (px,py) cae dentro de un cuadrado de lado side
// con esquinas de radio r.
func insideRoundedRect(px, py, side, r float64) bool {
	if px < 0 || py < 0 || px > side || py > side {
		return false
	}
	// El punto se recorta al rectángulo interior (el que excluye la zona de
	// curvatura): si el recorte no lo movió está claramente dentro; si lo movió,
	// lo que importa es su distancia al centro de esa esquina.
	dx := px - clamp(px, r, side-r)
	dy := py - clamp(py, r, side-r)
	return dx*dx+dy*dy <= r*r
}

func clamp(v, lo, hi float64) float64 {
	switch {
	case v < lo:
		return lo
	case v > hi:
		return hi
	default:
		return v
	}
}

// downsample promedia bloques de ss×ss píxeles (alpha incluido, premultiplicado
// como espera image/png para RGBA).
func downsample(src *image.RGBA, size, ss int) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, size, size))
	area := float64(ss * ss)
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			var r, g, b, a float64
			for sy := 0; sy < ss; sy++ {
				for sx := 0; sx < ss; sx++ {
					c := src.RGBAAt(x*ss+sx, y*ss+sy)
					r += float64(c.R)
					g += float64(c.G)
					b += float64(c.B)
					a += float64(c.A)
				}
			}
			dst.SetRGBA(x, y, color.RGBA{
				R: uint8(r / area),
				G: uint8(g / area),
				B: uint8(b / area),
				A: uint8(a / area),
			})
		}
	}
	return dst
}

// buildICO empaqueta PNGs en un contenedor .ico (ICONDIR + un ICONDIRENTRY por
// tamaño + los datos). Windows Vista+ acepta entradas PNG, así que no hace
// falta escribir BMPs con máscara.
func buildICO(sizes []int, images [][]byte) ([]byte, error) {
	if len(sizes) != len(images) {
		return nil, fmt.Errorf("sizes (%d) e images (%d) no coinciden", len(sizes), len(images))
	}
	const dirHeader = 6
	const dirEntry = 16

	var buf bytes.Buffer
	write := func(v any) error { return binary.Write(&buf, binary.LittleEndian, v) }

	if err := write([3]uint16{0, 1, uint16(len(images))}); err != nil { // reserved, type=icon, count
		return nil, err
	}
	offset := dirHeader + dirEntry*len(images)
	for i, size := range sizes {
		dim := uint8(size) // 256 se codifica como 0
		if size >= 256 {
			dim = 0
		}
		if err := write([4]uint8{dim, dim, 0, 0}); err != nil { // ancho, alto, colores, reserved
			return nil, err
		}
		if err := write([2]uint16{1, 32}); err != nil { // planes, bits por pixel
			return nil, err
		}
		if err := write([2]uint32{uint32(len(images[i])), uint32(offset)}); err != nil {
			return nil, err
		}
		offset += len(images[i])
	}
	for _, img := range images {
		if _, err := buf.Write(img); err != nil {
			return nil, err
		}
	}
	return buf.Bytes(), nil
}

// mustColor parsea un hex "#rrggbb" del palette; los valores vienen de
// internal/svg, así que un fallo es un bug de programación, no de entrada.
func mustColor(hex string) color.RGBA {
	var r, g, b uint8
	if _, err := fmt.Sscanf(hex, "#%02x%02x%02x", &r, &g, &b); err != nil {
		log.Fatalf("hex inválido %q: %v", hex, err)
	}
	return color.RGBA{R: r, G: g, B: b, A: 0xff}
}
