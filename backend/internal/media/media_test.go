package media

import (
	"bytes"
	"compress/zlib"
	"encoding/ascii85"
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

// pdfBuilder writes a classic PDF with correct xref offsets. The page tree
// helpers below are deliberately small but complete: names, /Kids and the
// revision chain are all real, so a regex over the bytes is not enough.
type pdfBuilder struct {
	bytes.Buffer
	offsets map[int]int
}

func newPDFBuilder() *pdfBuilder {
	return &pdfBuilder{offsets: make(map[int]int)}
}

func (b *pdfBuilder) obj(num int, body string) {
	b.offsets[num] = b.Len()
	fmt.Fprintf(&b.Buffer, "%d 0 obj\n%s\nendobj\n", num, body)
}

func (b *pdfBuilder) xref(prev int) int {
	start := b.Len()
	max := 0
	for n := range b.offsets {
		if n > max {
			max = n
		}
	}
	fmt.Fprintf(&b.Buffer, "xref\n0 %d\n0000000000 65535 f \n", max+1)
	for i := 1; i <= max; i++ {
		if off, ok := b.offsets[i]; ok {
			fmt.Fprintf(&b.Buffer, "%010d 00000 n \n", off)
		} else {
			b.WriteString("0000000000 65535 f \n")
		}
	}
	fmt.Fprintf(&b.Buffer, "trailer << /Size %d /Root 1 0 R", max+1)
	if prev >= 0 {
		fmt.Fprintf(&b.Buffer, " /Prev %d", prev)
	}
	fmt.Fprintf(&b.Buffer, " >>\nstartxref\n%d\n%%%%EOF\n", start)
	return start
}

func pdfKidsRefs(n int) []string {
	refs := make([]string, 0, n)
	for i := 0; i < n; i++ {
		refs = append(refs, fmt.Sprintf("%d 0 R", i+3))
	}
	return refs
}

// pdfWithPages is a minimal uncompressed PDF with a real n-leaf page tree.
func pdfWithPages(n int) []byte {
	b := newPDFBuilder()
	b.WriteString("%PDF-1.4\n")
	b.obj(1, "<< /Type /Catalog /Pages 2 0 R >>")
	b.obj(2, fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(pdfKidsRefs(n), " "), n))
	for i := 0; i < n; i++ {
		b.obj(i+3, "<< /Type /Page /Parent 2 0 R >>")
	}
	b.xref(-1)
	return b.Bytes()
}

// pdfIncrementalSaves rewrites every page object saves times, each with its
// own xref section and /Prev chain, as an annotating PDF editor does.
func pdfIncrementalSaves(n, saves int) []byte {
	b := newPDFBuilder()
	b.WriteString("%PDF-1.4\n")
	b.obj(1, "<< /Type /Catalog /Pages 2 0 R >>")
	b.obj(2, fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(pdfKidsRefs(n), " "), n))
	for i := 0; i < n; i++ {
		b.obj(i+3, "<< /Type /Page /Parent 2 0 R /Rotate 0 >>")
	}
	prev := b.xref(-1)
	for s := 1; s <= saves; s++ {
		for i := 0; i < n; i++ {
			b.obj(i+3, fmt.Sprintf("<< /Type /Page /Parent 2 0 R /Rotate %d >>", s*90))
		}
		prev = b.xref(prev)
	}
	return b.Bytes()
}

// pdfEscapedPages spells /Page and /Pages with the standard #xx name escapes
// (ISO 32000-1 §7.3.5) and hides a loose /Type /Page in a comment.
func pdfEscapedPages(n int) []byte {
	b := newPDFBuilder()
	b.WriteString("%PDF-1.5\n% /Type /Page\n")
	b.obj(1, "<< /Type /Catalog /Pages 2 0 R >>")
	b.obj(2, fmt.Sprintf("<< /Type /P#61ges /Kids [%s] /Count %d >>", strings.Join(pdfKidsRefs(n), " "), n))
	for i := 0; i < n; i++ {
		b.obj(i+3, "<< /Type /P#61ge /Parent 2 0 R /MediaBox [0 0 100 100] >>")
	}
	b.xref(-1)
	return b.Bytes()
}

// pdfObjStream hides n page objects (and the page tree) in a compressed
// object stream, with the N/First pair a real PDF 1.5+ writer uses.
func pdfObjStream(n int) []byte {
	return pdfObjStreamWithFilter(n, false)
}

func pdfObjStreamWithFilter(n int, ascii85First bool) []byte {
	refs := pdfKidsRefs(n)
	var header, content strings.Builder
	add := func(id int, body string) {
		fmt.Fprintf(&header, "%d %d ", id, content.Len())
		content.WriteString(body)
	}
	add(1, "<< /Type /Catalog /Pages 2 0 R >>")
	add(2, fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(refs, " "), n))
	for i := 0; i < n; i++ {
		// A nested dictionary in a page proves a filter/stream reader cannot
		// stop at the first '>' when it looks for the object-stream header.
		add(i+3, fmt.Sprintf("<< /Type /Page /Parent 2 0 R /Resources << /Font << /F1 %d 0 R >> >> >>", 100+i))
	}
	plain := append([]byte(header.String()), []byte(content.String())...)
	var z bytes.Buffer
	zw := zlib.NewWriter(&z)
	_, _ = zw.Write(plain)
	_ = zw.Close()
	data := z.Bytes()
	if ascii85First {
		enc := make([]byte, ascii85.MaxEncodedLen(len(data)))
		n := ascii85.Encode(enc, data)
		data = enc[:n]
	}
	filter := "/FlateDecode"
	if ascii85First {
		filter = "[/ASCII85Decode /FlateDecode]"
	}
	b := newXS()
	b.stream(900, fmt.Sprintf("/Type /ObjStm /N %d /First %d /Filter %s", 2+n, header.Len(), filter), data)
	for i := 0; i < 2+n; i++ {
		b.entries[i+1] = [3]int{2, 900, i}
	}
	b.xref(901, 1, false)
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
		if got := PDFPages(pdfObjStreamWithFilter(n, true)); got != n {
			t.Errorf("ASCII85+Flate object stream %d pages: got %d", n, got)
		}
	}
	// The two reproductions from #381.
	if got := PDFPages(pdfIncrementalSaves(3, 3)); got != 3 {
		t.Errorf("incrementally saved 3-page PDF: got %d, want 3", got)
	}
	if got := PDFPages(pdfEscapedPages(40)); got != 40 {
		t.Errorf("40 pages with escaped names and a stray comment: got %d, want 40", got)
	}
	for name, b := range map[string][]byte{
		"no objects":  []byte("%PDF-1.4\nnothing here\n%%EOF"),
		"loose pages": []byte("%PDF-1.4\n1 0 obj << /Type /Page >> endobj\n%%EOF"),
		"count only":  []byte("%PDF-1.4\n1 0 obj << /Type /Pages /Count 40 >> endobj\n%%EOF"),
	} {
		if got := PDFPages(b); got != 0 {
			t.Errorf("%s: got %d, want a refusal (0)", name, got)
		}
	}
}

func TestCheckPDF(t *testing.T) {
	info, err := Check(pdfWithPages(3))
	if err != nil || info.Pages != 3 || info.Kind() != "pdf" {
		t.Fatalf("pdf: %+v %v", info, err)
	}
	info, err = Check(pdfIncrementalSaves(3, 3))
	if err != nil || info.Pages != 3 {
		t.Fatalf("incrementally saved pdf: %+v %v", info, err)
	}
	info, err = Check(pdfEscapedPages(40))
	if err != nil || info.Pages != 40 {
		t.Fatalf("escaped 40-page pdf: %+v %v", info, err)
	}
	enc := append(pdfWithPages(1), []byte("trailer << /Encrypt 9 0 R >>")...)
	if _, err := Check(enc); !errors.Is(err, ErrRejected) || !strings.Contains(err.Error(), "encrypted") {
		t.Errorf("encrypted: %v", err)
	}
	if _, err := Check([]byte("%PDF-1.7\n%%EOF")); !errors.Is(err, ErrRejected) {
		t.Errorf("no pages: %v", err)
	}
}

// A stream that inflates to far more than maxInflate is not expanded into a
// page count; the file is refused rather than under-counted by a fallback.
func TestPDFPagesRefusesHugeObjectStream(t *testing.T) {
	var z bytes.Buffer
	zw := zlib.NewWriter(&z)
	_, _ = zw.Write(bytes.Repeat([]byte("0"), 8<<20))
	_ = zw.Close()
	var b bytes.Buffer
	b.WriteString("%PDF-1.5\n9 0 obj\n<< /Type /ObjStm /N 1 /First 8 /Filter /FlateDecode /Length ")
	fmt.Fprintf(&b, "%d >>\nstream\n", z.Len())
	b.Write(z.Bytes())
	b.WriteString("\nendstream\nendobj\n%%EOF\n")
	if got := PDFPages(b.Bytes()); got != 0 {
		t.Errorf("got %d pages, want a refusal (0)", got)
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
