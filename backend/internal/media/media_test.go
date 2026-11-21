package media

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func pngOf(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewGray(image.Rect(0, 0, w, h))
	img.Set(0, 0, color.White)
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// pngHeader is a PNG signature and a valid IHDR claiming w x h, with no
// pixel data: the shape of a decompression bomb's header.
func pngHeader(w, h uint32) []byte {
	var ihdr bytes.Buffer
	_ = binary.Write(&ihdr, binary.BigEndian, w)
	_ = binary.Write(&ihdr, binary.BigEndian, h)
	ihdr.Write([]byte{8, 0, 0, 0, 0})
	var b bytes.Buffer
	b.WriteString("\x89PNG\r\n\x1a\n")
	_ = binary.Write(&b, binary.BigEndian, uint32(ihdr.Len()))
	chunk := append([]byte("IHDR"), ihdr.Bytes()...)
	b.Write(chunk)
	_ = binary.Write(&b, binary.BigEndian, crc32.ChecksumIEEE(chunk))
	return b.Bytes()
}

func webpLossless(w, h int) []byte {
	bits := uint32(w-1) | uint32(h-1)<<14
	data := []byte{0x2f, 0, 0, 0, 0}
	binary.LittleEndian.PutUint32(data[1:], bits)
	data = append(data, make([]byte, 16)...)
	var b bytes.Buffer
	b.WriteString("RIFF")
	_ = binary.Write(&b, binary.LittleEndian, uint32(4+8+len(data)))
	b.WriteString("WEBPVP8L")
	_ = binary.Write(&b, binary.LittleEndian, uint32(len(data)))
	b.Write(data)
	return b.Bytes()
}

// pdfWithPages is a minimal uncompressed PDF with n page objects.
func pdfWithPages(n int) []byte {
	var b strings.Builder
	b.WriteString("%PDF-1.4\n1 0 obj << /Type /Catalog /Pages 2 0 R >> endobj\n")
	fmt.Fprintf(&b, "2 0 obj << /Type /Pages /Kids [] /Count %d >> endobj\n", n)
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "%d 0 obj << /Type /Page /Parent 2 0 R >> endobj\n", i+3)
	}
	b.WriteString("trailer << /Root 1 0 R >>\n%%EOF\n")
	return []byte(b.String())
}

// pdfObjStream hides n page objects (and the page tree) in a compressed
// object stream, as PDF 1.5+ writers do.
func pdfObjStream(n int) []byte {
	var inner strings.Builder
	fmt.Fprintf(&inner, "<< /Type /Pages /Kids [] /Count %d >>\n", n)
	for i := 0; i < n; i++ {
		inner.WriteString("<< /Type /Page /Parent 2 0 R >>\n")
	}
	var z bytes.Buffer
	zw := zlib.NewWriter(&z)
	_, _ = zw.Write([]byte(inner.String()))
	_ = zw.Close()
	var b bytes.Buffer
	b.WriteString("%PDF-1.5\n5 0 obj\n<< /Type /ObjStm /N 3 /First 10 /Filter /FlateDecode /Length ")
	fmt.Fprintf(&b, "%d >>\nstream\n", z.Len())
	b.Write(z.Bytes())
	b.WriteString("\nendstream\nendobj\n%%EOF\n")
	return b.Bytes()
}

func TestSniffTrustsBytesOnly(t *testing.T) {
	var jb bytes.Buffer
	_ = jpeg.Encode(&jb, image.NewGray(image.Rect(0, 0, 4, 4)), nil)
	cases := map[string][]byte{
		PNG:  pngOf(t, 2, 2),
		JPEG: jb.Bytes(),
		WEBP: webpLossless(10, 20),
		PDF:  pdfWithPages(1),
		"":   []byte("name,country\nmotor,China\n"),
	}
	for want, b := range cases {
		if got := Sniff(b); got != want {
			t.Errorf("Sniff(%q...) = %q, want %q", b[:4], got, want)
		}
	}
	// An HTML file renamed .png is still HTML.
	if _, err := Check([]byte("<html><script>alert(1)</script></html>")); !errors.Is(err, ErrUnsupported) {
		t.Errorf("html: %v, want ErrUnsupported", err)
	}
	// A GIF is a real image, but not one we accept.
	if _, err := Check([]byte("GIF89a\x01\x00\x01\x00")); !errors.Is(err, ErrUnsupported) {
		t.Errorf("gif: %v", err)
	}
}

func TestCheckImages(t *testing.T) {
	info, err := Check(pngOf(t, 30, 20))
	if err != nil || info.Width != 30 || info.Height != 20 || info.MIMEType != PNG || info.Kind() != "image" {
		t.Fatalf("png: %+v %v", info, err)
	}
	info, err = Check(webpLossless(640, 480))
	if err != nil || info.Width != 640 || info.Height != 480 {
		t.Fatalf("webp: %+v %v", info, err)
	}
	for name, b := range map[string][]byte{
		"empty":               nil,
		"bomb 50k x 50k":      pngHeader(50000, 50000),
		"too wide":            pngHeader(MaxSide+1, 10),
		"too many pixels":     pngHeader(8000, 8000),
		"zero size":           pngHeader(0, 10),
		"truncated png":       []byte("\x89PNG\r\n\x1a\n\x00\x00"),
		"truncated jpeg":      {0xFF, 0xD8, 0xFF, 0xE0, 0x00},
		"webp too many px":    webpLossless(10000, 10000),
		"webp truncated head": []byte("RIFF\x00\x00\x00\x00WEBPVP8L"),
	} {
		if _, err := Check(b); !errors.Is(err, ErrRejected) {
			t.Errorf("%s: %v, want ErrRejected", name, err)
		}
	}
}

func TestPDFPages(t *testing.T) {
	for n := 1; n <= 12; n += 5 {
		if got := PDFPages(pdfWithPages(n)); got != n {
			t.Errorf("plain %d pages: got %d", n, got)
		}
		if got := PDFPages(pdfObjStream(n)); got != n {
			t.Errorf("object stream %d pages: got %d", n, got)
		}
	}
	if got := PDFPages([]byte("%PDF-1.4\nnothing here\n%%EOF")); got != 0 {
		t.Errorf("no pages: got %d", got)
	}
}

func TestCheckPDF(t *testing.T) {
	info, err := Check(pdfWithPages(3))
	if err != nil || info.Pages != 3 || info.Kind() != "pdf" {
		t.Fatalf("pdf: %+v %v", info, err)
	}
	enc := append(pdfWithPages(1), []byte("trailer << /Encrypt 9 0 R >>")...)
	if _, err := Check(enc); !errors.Is(err, ErrRejected) || !strings.Contains(err.Error(), "encrypted") {
		t.Errorf("encrypted: %v", err)
	}
	if _, err := Check([]byte("%PDF-1.7\n%%EOF")); !errors.Is(err, ErrRejected) {
		t.Errorf("no pages: %v", err)
	}
}

// A stream that inflates to far more than maxInflate is read only up to it.
func TestPDFPagesInflateIsCapped(t *testing.T) {
	var z bytes.Buffer
	zw := zlib.NewWriter(&z)
	_, _ = zw.Write(bytes.Repeat([]byte("0"), 64<<20))
	_ = zw.Close()
	var b bytes.Buffer
	b.WriteString("%PDF-1.5\n<< /Type /ObjStm /N 1 >>\nstream\n")
	b.Write(z.Bytes())
	b.WriteString("\nendstream\n<< /Type /Page >>\n")
	if got := PDFPages(b.Bytes()); got != 1 {
		t.Errorf("got %d pages", got)
	}
}

// The rendered demo BOMs (eval/media, used by cmd/mediacheck and the docs)
// pass the same checks as an upload.
func TestRenderedDemoBOMsPass(t *testing.T) {
	paths, _ := filepath.Glob(filepath.Join("..", "..", "..", "eval", "media", "images", "*.png"))
	if len(paths) != 5 {
		t.Fatalf("found %d rendered BOMs, want 5", len(paths))
	}
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		if info, err := Check(b); err != nil || info.MIMEType != PNG || info.Width < 500 {
			t.Errorf("%s: %+v %v", p, info, err)
		}
	}
}

// The varied set (eval/media/images-varied, D168) holds PNGs and JPEGs,
// some deliberately small (low-resolution scans); all pass the upload checks.
func TestVariedDemoBOMsPass(t *testing.T) {
	var paths []string
	for _, ext := range []string{"*.png", "*.jpg"} {
		m, _ := filepath.Glob(filepath.Join("..", "..", "..", "eval", "media", "images-varied", ext))
		paths = append(paths, m...)
	}
	if len(paths) != 15 {
		t.Fatalf("found %d varied BOM images, want 15", len(paths))
	}
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		if info, err := Check(b); err != nil || (info.MIMEType != PNG && info.MIMEType != JPEG) || info.Width < 200 {
			t.Errorf("%s: %+v %v", p, info, err)
		}
	}
}
