package media

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"math/rand"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"
)

// xsBuilder writes PDF 1.5 files the way current writers do: objects in
// Flate object streams, cross-reference streams (/W [1 4 2], optionally with
// the PNG Up predictor), and incremental updates chained by /Prev. Each
// section lists only the objects written since the previous one.
type xsBuilder struct {
	bytes.Buffer
	entries map[int][3]int // type, field 2, field 3
	prev    int
}

func newXS() *xsBuilder {
	b := &xsBuilder{entries: make(map[int][3]int), prev: -1}
	b.WriteString("%PDF-1.5\n%\xe2\xe3\xcf\xd3\n")
	return b
}

func (b *xsBuilder) obj(num int, body string) {
	b.entries[num] = [3]int{1, b.Len(), 0}
	fmt.Fprintf(b, "%d 0 obj\n%s\nendobj\n", num, body)
}

func (b *xsBuilder) stream(num int, dict string, data []byte) {
	b.entries[num] = [3]int{1, b.Len(), 0}
	fmt.Fprintf(b, "%d 0 obj\n<< %s /Length %d >>\nstream\n", num, dict, len(data))
	b.Write(data)
	b.WriteString("\nendstream\nendobj\n")
}

type pdfObjBody struct {
	id   int
	body string
}

func zlibBytes(p []byte) []byte {
	var z bytes.Buffer
	zw := zlib.NewWriter(&z)
	_, _ = zw.Write(p)
	_ = zw.Close()
	return z.Bytes()
}

// objStm writes objs into object stream num.
func (b *xsBuilder) objStm(num int, objs []pdfObjBody) {
	var header, content strings.Builder
	for i, o := range objs {
		fmt.Fprintf(&header, "%d %d ", o.id, content.Len())
		content.WriteString(o.body)
		content.WriteString("\n")
		b.entries[o.id] = [3]int{2, num, i}
	}
	plain := header.String() + content.String()
	b.stream(num, fmt.Sprintf("/Type /ObjStm /N %d /First %d /Filter /FlateDecode", len(objs), header.Len()),
		zlibBytes([]byte(plain)))
}

// xref writes the section's xref stream as object num and the file's
// startxref; predictor selects /DecodeParms << /Predictor 12 /Columns 7 >>.
func (b *xsBuilder) xref(num, root int, predictor bool) {
	off := b.Len()
	b.entries[num] = [3]int{1, off, 0}
	ids := make([]int, 0, len(b.entries))
	for id := range b.entries {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	var index []string
	var rows []byte
	for i := 0; i < len(ids); {
		j := i
		for j+1 < len(ids) && ids[j+1] == ids[j]+1 {
			j++
		}
		index = append(index, fmt.Sprintf("%d %d", ids[i], j-i+1))
		for k := i; k <= j; k++ {
			e := b.entries[ids[k]]
			rows = append(rows, byte(e[0]), byte(e[1]>>24), byte(e[1]>>16), byte(e[1]>>8), byte(e[1]), byte(e[2]>>8), byte(e[2]))
		}
		i = j + 1
	}
	dict := fmt.Sprintf("/Type /XRef /Size %d /Index [%s] /W [1 4 2] /Root %d 0 R /Filter /FlateDecode",
		ids[len(ids)-1]+1, strings.Join(index, " "), root)
	data := rows
	if predictor {
		dict += " /DecodeParms << /Predictor 12 /Columns 7 >>"
		data = nil
		prev := make([]byte, 7)
		for p := 0; p < len(rows); p += 7 {
			data = append(data, 2)
			for i := 0; i < 7; i++ {
				data = append(data, rows[p+i]-prev[i])
			}
			prev = rows[p : p+7]
		}
	}
	if b.prev >= 0 {
		dict += fmt.Sprintf(" /Prev %d", b.prev)
	}
	b.stream(num, dict, zlibBytes(data))
	fmt.Fprintf(b, "startxref\n%d\n%%%%EOF\n", off)
	b.prev = off
	b.entries = make(map[int][3]int)
}

func kidsRefs(from, n int) string {
	refs := make([]string, n)
	for i := range refs {
		refs[i] = fmt.Sprintf("%d 0 R", from+i)
	}
	return strings.Join(refs, " ")
}

// compressedThen writes an original save whose catalog, page tree and first
// pages live in an object stream, then an incremental update that rewrites
// the page tree as a top-level object with `after` pages (#503 example 1).
func compressedThen(before, after int, predictor bool) []byte {
	b := newXS()
	objs := []pdfObjBody{
		{1, "<< /Type /Catalog /Pages 2 0 R >>"},
		{2, fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", kidsRefs(3, before), before)},
	}
	for i := 0; i < before; i++ {
		objs = append(objs, pdfObjBody{3 + i, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>"})
	}
	b.objStm(100, objs)
	b.xref(101, 1, predictor)
	b.obj(2, fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", kidsRefs(3, after), after))
	for i := before; i < after; i++ {
		b.obj(3+i, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>")
	}
	b.xref(102, 1, predictor)
	return b.Bytes()
}

// fakeObjInMetadata is a classic n-page PDF whose catalog's uncompressed XMP
// stream, written after the real page tree, holds the text of a one-page
// "2 0 obj" (#503 example 3).
func fakeObjInMetadata(n int) []byte {
	b := newPDFBuilder()
	b.WriteString("%PDF-1.4\n")
	b.obj(1, "<< /Type /Catalog /Pages 2 0 R /Metadata 500 0 R >>")
	b.obj(2, fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(pdfKidsRefs(n), " "), n))
	for i := 0; i < n; i++ {
		b.obj(i+3, "<< /Type /Page /Parent 2 0 R >>")
	}
	xmp := "<?xpacket begin=\"\"?>\n<x:xmpmeta xmlns:x=\"adobe:ns:meta/\">\n<!--\n" +
		"2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n-->\n</x:xmpmeta>\n<?xpacket end=\"w\"?>"
	b.obj(500, fmt.Sprintf("<< /Type /Metadata /Subtype /XML /Length %d >>\nstream\n%s\nendstream", len(xmp), xmp))
	b.xref(-1)
	return b.Bytes()
}

func TestPDFPagesFollowsNewestRevisionThroughXrefStreams(t *testing.T) {
	cases := []struct {
		name string
		b    []byte
		want int
	}{
		{"3 in an object stream, then 40 at top level", compressedThen(3, 40, false), 40},
		{"same, xref streams with the PNG Up predictor", compressedThen(3, 40, true), 40},
		{"12 in an object stream, then 5 at top level", compressedThen(12, 5, true), 5},
		{"40 pages, a fake one-page 2 0 obj in XMP", fakeObjInMetadata(40), 40},
	}
	for _, c := range cases {
		if got := PDFPages(c.b); got != c.want {
			t.Errorf("%s (%d bytes): got %d pages, want %d", c.name, len(c.b), got, c.want)
		}
	}
	for _, c := range cases {
		info, err := Check(c.b)
		if err != nil || info.Pages != c.want {
			t.Errorf("Check %s: %+v %v", c.name, info, err)
		}
	}
}

// Without cross-reference data the objects cannot be located; the file is
// refused rather than counted from a scan that can under-count (#503).
func TestPDFPagesRefusesWithoutXref(t *testing.T) {
	good := pdfWithPages(3)
	i := bytes.LastIndex(good, []byte("xref"))
	noXref := append(append([]byte(nil), good[:i]...), []byte("trailer << /Root 1 0 R >>\n%%EOF\n")...)
	badOffset := bytes.Replace(good, []byte("0000000009 00000 n"), []byte("0000000011 00000 n"), 1)
	if bytes.Equal(badOffset, good) {
		t.Fatal("offset not replaced")
	}
	for name, b := range map[string][]byte{"no xref": noXref, "wrong offset": badOffset} {
		if got := PDFPages(b); got != 0 {
			t.Errorf("%s: got %d, want a refusal (0)", name, got)
		}
	}
	if got := PDFPages(good); got != 3 {
		t.Fatalf("control: got %d, want 3", got)
	}
}

// dagPDF has a chain of /Pages nodes, each listing the next one twice, so a
// walk that revisits shared nodes takes 2^levels steps (#504).
func dagPDF(levels int, withXref bool) []byte {
	b := newPDFBuilder()
	b.WriteString("%PDF-1.4\n")
	b.obj(1, "<< /Type /Catalog /Pages 2 0 R >>")
	for k := 0; k < levels; k++ {
		n := 2 + k
		b.obj(n, fmt.Sprintf("<< /Type /Pages /Kids [%d 0 R %d 0 R] >>", n+1, n+1))
	}
	b.obj(2+levels, "<< /Type /Page >>")
	if withXref {
		b.xref(-1)
	} else {
		b.WriteString("trailer << /Root 1 0 R >>\n%%EOF\n")
	}
	return b.Bytes()
}

func TestPDFPagesRefusesSharedKidsQuickly(t *testing.T) {
	for _, withXref := range []bool{true, false} {
		b := dagPDF(64, withXref)
		start := time.Now()
		got := PDFPages(b)
		if got != 0 {
			t.Errorf("xref=%v: got %d, want a refusal (0)", withXref, got)
		}
		if el := time.Since(start); el > 200*time.Millisecond {
			t.Errorf("xref=%v: took %v", withXref, el)
		}
	}
	// A cycle and a kid listed twice are refused too.
	b := newPDFBuilder()
	b.WriteString("%PDF-1.4\n")
	b.obj(1, "<< /Type /Catalog /Pages 2 0 R >>")
	b.obj(2, "<< /Type /Pages /Kids [3 0 R 2 0 R] >>")
	b.obj(3, "<< /Type /Page /Parent 2 0 R >>")
	b.xref(-1)
	if got := PDFPages(b.Bytes()); got != 0 {
		t.Errorf("cycle: got %d", got)
	}
	b = newPDFBuilder()
	b.WriteString("%PDF-1.4\n")
	b.obj(1, "<< /Type /Catalog /Pages 2 0 R >>")
	b.obj(2, "<< /Type /Pages /Kids [3 0 R 3 0 R] /Count 2 >>")
	b.obj(3, "<< /Type /Page /Parent 2 0 R >>")
	b.xref(-1)
	if got := PDFPages(b.Bytes()); got != 0 {
		t.Errorf("repeated kid: got %d", got)
	}
}

// A /Type inside a nested dictionary is not the object's type: a page tree
// root that carries << /Type /Page >> in another key still has 12 pages.
func TestPDFPagesReadsTopLevelType(t *testing.T) {
	b := newPDFBuilder()
	b.WriteString("%PDF-1.4\n")
	b.obj(1, "<< /Type /Catalog /Pages 2 0 R >>")
	b.obj(2, fmt.Sprintf("<< /X << /Type /Page >> /Type /Pages /Kids [%s] /Count 12 >>", strings.Join(pdfKidsRefs(12), " ")))
	for i := 0; i < 12; i++ {
		b.obj(i+3, "<< /Type /Page /Parent 2 0 R /Annots [<< /Type /Pages >>] >>")
	}
	b.xref(-1)
	if got := PDFPages(b.Bytes()); got != 12 {
		t.Errorf("got %d, want 12", got)
	}
}

// bombStream is an object stream holding one object (id) followed by just
// under 4 MB (4 MB less 64 KB) of spaces: about 4 KB compressed.
func bombStream(b *xsBuilder, num, id int, body string) {
	head := fmt.Sprintf("%d 0 ", id)
	plain := append([]byte(head+body), bytes.Repeat([]byte(" "), maxInflate-len(head)-len(body)-64<<10)...)
	b.entries[id] = [3]int{2, num, 0}
	b.stream(num, fmt.Sprintf("/Type /ObjStm /N 1 /First %d /Filter /FlateDecode", len(head)), zlibBytes(plain))
}

// bombsPDF has a page tree whose pages each live in their own ~4 MB object
// stream, plus `unused` more such streams nothing refers to (#504).
func bombsPDF(pages, unused int) []byte {
	b := newXS()
	b.obj(1, "<< /Type /Catalog /Pages 2 0 R >>")
	b.obj(2, fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", kidsRefs(3, pages), pages))
	for i := 0; i < pages; i++ {
		bombStream(b, 1000+i, 3+i, "<< /Type /Page /Parent 2 0 R >>")
	}
	for i := 0; i < unused; i++ {
		bombStream(b, 5000+i, 3000+i, "<< /Type /Page >>")
	}
	b.xref(9000, 1, true)
	return b.Bytes()
}

func heapAfter(f func()) (retained, allocated uint64) {
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	f()
	runtime.ReadMemStats(&after)
	return after.HeapAlloc - min(after.HeapAlloc, before.HeapAlloc), after.TotalAlloc - before.TotalAlloc
}

func TestPDFPagesBoundsDecompression(t *testing.T) {
	// Two pages in two 4 MB streams fit the 8 MB budget; three do not.
	two, three := bombsPDF(2, 0), bombsPDF(3, 0)
	if got := PDFPages(two); got != 2 {
		t.Errorf("two streams: got %d, want 2", got)
	}
	if got := PDFPages(three); got != 0 {
		t.Errorf("three streams: got %d, want a refusal (0)", got)
	}
	// Streams nothing refers to are never decoded, however many there are.
	many := bombsPDF(1, 100)
	var got int
	_, alloc := heapAfter(func() { got = PDFPages(many) })
	if got != 1 {
		t.Errorf("1 page + 100 unused streams (%d bytes): got %d", len(many), got)
	}
	if alloc > 32<<20 {
		t.Errorf("1 page + 100 unused streams: allocated %d MB", alloc>>20)
	}
	// Referenced streams: allocation stays a small multiple of the budget,
	// and the expanded page objects do not keep their 4 MB buffers alive.
	_, alloc = heapAfter(func() { got = PDFPages(three) })
	if alloc > 64<<20 {
		t.Errorf("three streams: allocated %d MB", alloc>>20)
	}
}

// The budget is shared by the files of one upload: two files that each fit
// alone do not both fit together.
func TestBudgetIsSharedAcrossFiles(t *testing.T) {
	two := bombsPDF(2, 0)
	bg := NewBudget()
	if info, err := bg.Check(two); err != nil || info.Pages != 2 {
		t.Fatalf("first file: %+v %v", info, err)
	}
	if _, err := bg.Check(two); err == nil {
		t.Fatal("second file: accepted, want a refusal once the upload's budget is spent")
	}
	if info, err := Check(two); err != nil || info.Pages != 2 {
		t.Fatalf("fresh budget: %+v %v", info, err)
	}
}

func TestBudgetDeadline(t *testing.T) {
	bg := NewBudget()
	bg.deadline = time.Now().Add(-time.Second)
	if _, err := bg.Check(pdfWithPages(3)); err == nil {
		t.Fatal("expired budget: accepted")
	}
}

// Mutated and truncated files never panic, never take long, and never
// report more pages than the largest seed has.
func TestPDFPagesSurvivesMutations(t *testing.T) {
	seeds := [][]byte{
		pdfWithPages(5), pdfIncrementalSaves(3, 2), pdfEscapedPages(12), pdfObjStream(7),
		compressedThen(3, 9, true), compressedThen(12, 5, false), fakeObjInMetadata(6), dagPDF(10, true),
	}
	rng := rand.New(rand.NewSource(503504))
	iters := 3000
	if testing.Short() {
		iters = 300
	}
	tokens := [][]byte{[]byte(" obj "), []byte("endobj"), []byte("stream\n"), []byte("<<"), []byte(">>"), []byte("["), []byte("]"),
		[]byte("/Prev 9 "), []byte("/Kids [3 0 R 3 0 R]"), []byte("9999999999"), []byte("startxref\n0\n"), []byte("%")}
	for i := 0; i < iters; i++ {
		src := seeds[rng.Intn(len(seeds))]
		b := append([]byte(nil), src...)
		switch rng.Intn(4) {
		case 0:
			b = b[:rng.Intn(len(b))]
		case 1:
			for k := 0; k < 1+rng.Intn(8); k++ {
				b[rng.Intn(len(b))] = byte(rng.Intn(256))
			}
		case 2:
			p := rng.Intn(len(b))
			tok := tokens[rng.Intn(len(tokens))]
			b = append(b[:p], append(append([]byte(nil), tok...), b[p:]...)...)
		case 3:
			p, q := rng.Intn(len(b)), rng.Intn(len(b))
			if p > q {
				p, q = q, p
			}
			b = append(b[:p], b[q:]...)
		}
		if !bytes.HasPrefix(b, []byte("%PDF-")) {
			b = append([]byte("%PDF-1.5\n"), b...)
		}
		start := time.Now()
		n := PDFPages(b)
		if el := time.Since(start); el > 250*time.Millisecond {
			t.Fatalf("mutation %d took %v", i, el)
		}
		if n < 0 || n > 64 {
			t.Fatalf("mutation %d: %d pages from %d bytes", i, n, len(b))
		}
	}
}

func FuzzPDFPages(f *testing.F) {
	for _, s := range [][]byte{pdfWithPages(2), pdfObjStream(3), compressedThen(3, 5, true), fakeObjInMetadata(2), dagPDF(6, true)} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, b []byte) {
		_ = PDFPages(b)
	})
}
