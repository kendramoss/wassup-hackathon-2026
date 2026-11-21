package media

import (
	"bytes"
	"compress/zlib"
	"encoding/ascii85"
	"errors"
	"fmt"
	"io"
	"strconv"
	"time"
)

// PDF page counting reads the document's page tree through its
// cross-reference data, the way a PDF reader finds objects (#381, #503):
//
//   - startxref → the newest xref section (a classic table or an xref
//     stream), then /Prev (and a hybrid file's /XRefStm) to older ones. The
//     newest entry for an object decides where it lives: at a byte offset, in
//     an object stream, or nowhere (free).
//   - Only bytes an xref entry points at are parsed as an object, so text that
//     looks like "2 0 obj" inside stream data is never read as one.
//   - The trailer's /Root → /Pages → /Kids is walked once per node; a node
//     reached twice is refused (#504), so time is linear in the objects.
//
// Decompression and time are bounded by a Budget shared across all files of
// one upload (#504). Anything that cannot be resolved returns 0, which Check
// refuses as "pages could not be counted"; there is no raw-text fallback that
// could under-count a long document.

const (
	// maxInflate caps one decoded stream.
	maxInflate = 4 << 20
	// maxInflateTotal caps all decoded streams of one upload together.
	maxInflateTotal = 8 << 20
	// maxPDFTime caps the time spent counting the pages of one upload.
	maxPDFTime = 2 * time.Second
	// maxPDFObject caps object numbers, xref entries and resolved objects.
	maxPDFObject  = 200_000
	maxPDFDepth   = 64
	maxPDFXrefs   = 1_000
	maxPDFNesting = 64
)

var errPDFBudget = errors.New("pdf decode budget exhausted")

// Budget bounds the work of checking the files of one upload together:
// decoded stream bytes across every stream of every file, and wall time.
// A Budget is not safe for concurrent use.
type Budget struct {
	inflate  int
	deadline time.Time
}

// NewBudget returns the budget for one upload.
func NewBudget() *Budget {
	return &Budget{inflate: maxInflateTotal, deadline: time.Now().Add(maxPDFTime)}
}

func (bg *Budget) spend(n int) bool {
	if n > bg.inflate {
		bg.inflate = 0
		return false
	}
	bg.inflate -= n
	return true
}

func (bg *Budget) expired() bool { return time.Now().After(bg.deadline) }

// limit is how many bytes the next decode may produce.
func (bg *Budget) limit() int {
	if bg.inflate < maxInflate {
		return bg.inflate
	}
	return maxInflate
}

type xrefEntry struct {
	kind byte // 'n' at an offset, 'c' in an object stream, 'f' free
	off  int  // byte offset ('n') or object-stream number ('c')
	idx  int  // index inside the object stream ('c')
}

type pdfObjStm struct {
	plain []byte
	ids   []int
	offs  []int
}

type pdfDoc struct {
	b       []byte
	budget  *Budget
	xref    map[int]xrefEntry
	trailer map[string][]byte
	objs    map[int][]byte
	stms    map[int]*pdfObjStm
	busy    map[int]bool
	ops     int
}

// PDFPages returns the number of leaves of the document's page tree, or 0
// when the tree cannot be resolved. Callers treat 0 as "pages could not be
// counted" and refuse the file.
func PDFPages(b []byte) int { return NewBudget().pdfPages(b) }

func (bg *Budget) pdfPages(b []byte) int {
	if !bytes.HasPrefix(b, []byte("%PDF-")) {
		return 0
	}
	d := &pdfDoc{
		b:      b,
		budget: bg,
		xref:   make(map[int]xrefEntry),
		objs:   make(map[int][]byte),
		stms:   make(map[int]*pdfObjStm),
		busy:   make(map[int]bool),
	}
	if !d.readXref() {
		return 0
	}
	root := pdfRef(d.trailer["Root"])
	catalog, ok := d.dict(root)
	if !ok {
		return 0
	}
	pages := pdfRef(catalog["Pages"])
	if pages <= 0 {
		return 0
	}
	n, ok := d.countPages(pages, 0, make(map[int]bool))
	if !ok || n <= 0 {
		return 0
	}
	return n
}

// --- cross-reference data ---

// readXref follows startxref and the /Prev chain. The first (newest) entry
// seen for an object wins, and the first trailer with a /Root is used.
func (d *pdfDoc) readXref() bool {
	i := bytes.LastIndex(d.b, []byte("startxref"))
	if i < 0 {
		return false
	}
	off, _, ok := pdfReadInt(d.b, pdfSkip(d.b, i+len("startxref")))
	if !ok {
		return false
	}
	seen := make(map[int]bool)
	for sections := 0; ; sections++ {
		if sections >= maxPDFXrefs || off <= 0 || off >= len(d.b) || seen[off] || d.budget.expired() {
			return false
		}
		seen[off] = true
		trailer, ok := d.readSection(off)
		if !ok {
			return false
		}
		if xs := trailer["XRefStm"]; xs != nil {
			// Hybrid file: the stream's entries rank below the table's and
			// above /Prev.
			if xo, ok := pdfInt(xs); ok && !seen[xo] {
				seen[xo] = true
				if _, ok := d.readSection(xo); !ok {
					return false
				}
			}
		}
		if d.trailer == nil && trailer["Root"] != nil {
			d.trailer = trailer
		}
		prev, has := trailer["Prev"]
		if !has {
			break
		}
		if off, ok = pdfInt(prev); !ok {
			return false
		}
	}
	return d.trailer != nil && len(d.xref) > 0
}

func (d *pdfDoc) add(num int, e xrefEntry) bool {
	if num < 0 || num > maxPDFObject {
		return false
	}
	if _, ok := d.xref[num]; !ok {
		if len(d.xref) >= maxPDFObject {
			return false
		}
		d.xref[num] = e
	}
	return true
}

// readSection reads one classic table and its trailer, or one xref stream.
func (d *pdfDoc) readSection(off int) (map[string][]byte, bool) {
	p := pdfSkip(d.b, off)
	if bytes.HasPrefix(d.b[p:], []byte("xref")) {
		return d.readTable(p + len("xref"))
	}
	dict, data, ok := d.objectAt(p, -1)
	if !ok || data == nil || pdfName(dict["Type"]) != "XRef" {
		return nil, false
	}
	return dict, d.readXrefStream(dict, data)
}

func (d *pdfDoc) readTable(p int) (map[string][]byte, bool) {
	b := d.b
	for {
		p = pdfSkip(b, p)
		if p >= len(b) || !pdfIsDigit(b[p]) {
			break
		}
		start, np, ok := pdfReadInt(b, p)
		if !ok {
			return nil, false
		}
		count, np, ok := pdfReadInt(b, pdfSkip(b, np))
		if !ok || count < 0 || count > maxPDFObject || start > maxPDFObject {
			return nil, false
		}
		p = np
		for k := 0; k < count; k++ {
			off, np, ok := pdfReadInt(b, pdfSkip(b, p))
			if !ok {
				return nil, false
			}
			_, np, ok = pdfReadInt(b, pdfSkip(b, np))
			if !ok {
				return nil, false
			}
			q := pdfSkip(b, np)
			if q >= len(b) || (b[q] != 'n' && b[q] != 'f') {
				return nil, false
			}
			e := xrefEntry{kind: 'f'}
			if b[q] == 'n' {
				e = xrefEntry{kind: 'n', off: off}
			}
			if start+k > 0 && !d.add(start+k, e) {
				return nil, false
			}
			p = q + 1
		}
	}
	if !bytes.HasPrefix(b[p:], []byte("trailer")) {
		return nil, false
	}
	dict, _, ok := pdfDict(b, pdfSkip(b, p+len("trailer")), 0)
	return dict, ok
}

func (d *pdfDoc) readXrefStream(dict map[string][]byte, data []byte) bool {
	plain, ok := d.decode(dict, data)
	if !ok {
		return false
	}
	ws := pdfInts(dict["W"])
	if len(ws) != 3 {
		return false
	}
	row := 0
	for _, w := range ws {
		if w < 0 || w > 8 {
			return false
		}
		row += w
	}
	if row == 0 {
		return false
	}
	index := pdfInts(dict["Index"])
	if dict["Index"] == nil {
		size, ok := pdfInt(dict["Size"])
		if !ok {
			return false
		}
		index = []int{0, size}
	}
	if len(index)%2 != 0 {
		return false
	}
	field := func(b []byte, w, def int) int {
		if w == 0 {
			return def
		}
		v := 0
		for _, c := range b[:w] {
			v = v<<8 | int(c)
		}
		return v
	}
	pos := 0
	for i := 0; i < len(index); i += 2 {
		start, count := index[i], index[i+1]
		if start < 0 || count < 0 || start > maxPDFObject || count > maxPDFObject {
			return false
		}
		for k := 0; k < count; k++ {
			if pos+row > len(plain) {
				return false
			}
			r := plain[pos : pos+row]
			pos += row
			t := field(r, ws[0], 1)
			f2 := field(r[ws[0]:], ws[1], 0)
			f3 := field(r[ws[0]+ws[1]:], ws[2], 0)
			var e xrefEntry
			switch t {
			case 0:
				e = xrefEntry{kind: 'f'}
			case 1:
				e = xrefEntry{kind: 'n', off: f2}
			case 2:
				e = xrefEntry{kind: 'c', off: f2, idx: f3}
			default:
				// Unknown types are references to the null object.
				e = xrefEntry{kind: 'f'}
			}
			if start+k > 0 && !d.add(start+k, e) {
				return false
			}
		}
	}
	return true
}

// --- objects ---

// objectAt parses "num gen obj" at p and the object's value. want < 0
// accepts any number. For a stream, data is its raw (still encoded) bytes.
func (d *pdfDoc) objectAt(p, want int) (map[string][]byte, []byte, bool) {
	b := d.b
	if p < 0 || p >= len(b) {
		return nil, nil, false
	}
	p = pdfSkip(b, p)
	num, np, ok := pdfReadInt(b, p)
	if !ok || (want >= 0 && num != want) {
		return nil, nil, false
	}
	_, np, ok = pdfReadInt(b, pdfSkip(b, np))
	if !ok {
		return nil, nil, false
	}
	np = pdfSkip(b, np)
	if !pdfKeyword(b, np, "obj") {
		return nil, nil, false
	}
	vp := pdfSkip(b, np+len("obj"))
	if vp >= len(b) {
		return nil, nil, false
	}
	if b[vp] != '<' || vp+1 >= len(b) || b[vp+1] != '<' {
		// Not a dictionary (a number or an array): no stream follows.
		return map[string][]byte{"": b[vp:pdfValueEndOr(b, vp)]}, nil, true
	}
	dict, end, ok := pdfDict(b, vp, 0)
	if !ok {
		return nil, nil, false
	}
	q := pdfSkip(b, end)
	if !pdfKeyword(b, q, "stream") {
		return dict, nil, true
	}
	q += len("stream")
	if q < len(b) && b[q] == '\r' {
		q++
	}
	if q < len(b) && b[q] == '\n' {
		q++
	}
	if n, ok := d.length(dict["Length"]); ok && n >= 0 && q+n <= len(b) {
		if pdfKeyword(b, pdfSkip(b, q+n), "endstream") {
			return dict, b[q : q+n], true
		}
	}
	e := bytes.Index(b[q:], []byte("endstream"))
	if e < 0 {
		return nil, nil, false
	}
	return dict, bytes.TrimRight(b[q:q+e], "\r\n"), true
}

func (d *pdfDoc) length(v []byte) (int, bool) {
	if n, ok := pdfInt(v); ok {
		return n, true
	}
	ref := pdfRef(v)
	if ref <= 0 {
		return 0, false
	}
	dict, ok := d.dict(ref)
	if !ok {
		return 0, false
	}
	return pdfInt(dict[""])
}

// dict resolves object num through the xref and returns its dictionary (a
// non-dictionary value is under the key ""). Stream data is not kept.
func (d *pdfDoc) dict(num int) (map[string][]byte, bool) {
	if num <= 0 || d.busy[num] {
		return nil, false
	}
	d.ops++
	if d.ops > maxPDFObject || d.budget.expired() {
		return nil, false
	}
	if raw, ok := d.objs[num]; ok {
		return pdfBody(raw)
	}
	e, ok := d.xref[num]
	if !ok {
		return nil, false
	}
	d.busy[num] = true
	defer delete(d.busy, num)
	switch e.kind {
	case 'n':
		dict, _, ok := d.objectAt(e.off, num)
		return dict, ok
	case 'c':
		stm, ok := d.objStm(e.off)
		if !ok || e.idx < 0 || e.idx >= len(stm.ids) || stm.ids[e.idx] != num {
			return nil, false
		}
		start := stm.offs[e.idx]
		end := pdfValueEndOr(stm.plain, start)
		// Copy the object's own bytes: a slice would keep the whole decoded
		// stream alive (#504).
		raw := append([]byte(nil), stm.plain[start:end]...)
		d.objs[num] = raw
		return pdfBody(raw)
	}
	return nil, false
}

// pdfBody parses a value read from an object stream.
func pdfBody(raw []byte) (map[string][]byte, bool) {
	p := pdfSkip(raw, 0)
	if p+1 < len(raw) && raw[p] == '<' && raw[p+1] == '<' {
		dict, _, ok := pdfDict(raw, p, 0)
		return dict, ok
	}
	return map[string][]byte{"": raw[p:pdfValueEndOr(raw, p)]}, true
}

func (d *pdfDoc) objStm(num int) (*pdfObjStm, bool) {
	if s, ok := d.stms[num]; ok {
		return s, s != nil
	}
	d.stms[num] = nil
	e, ok := d.xref[num]
	if !ok || e.kind != 'n' {
		// An object stream is never itself inside an object stream.
		return nil, false
	}
	dict, data, ok := d.objectAt(e.off, num)
	if !ok || data == nil || pdfName(dict["Type"]) != "ObjStm" {
		return nil, false
	}
	n, ok1 := pdfInt(dict["N"])
	first, ok2 := pdfInt(dict["First"])
	if !ok1 || !ok2 || n <= 0 || n > maxPDFObject || first < 0 {
		return nil, false
	}
	plain, ok := d.decode(dict, data)
	if !ok || first > len(plain) {
		return nil, false
	}
	s := &pdfObjStm{plain: plain, ids: make([]int, n), offs: make([]int, n)}
	p := 0
	for i := 0; i < n; i++ {
		var id, off int
		if id, p, ok = pdfReadInt(plain[:first], pdfSkip(plain[:first], p)); !ok {
			return nil, false
		}
		if off, p, ok = pdfReadInt(plain[:first], pdfSkip(plain[:first], p)); !ok {
			return nil, false
		}
		if first+off >= len(plain) {
			return nil, false
		}
		s.ids[i], s.offs[i] = id, first+off
	}
	d.stms[num] = s
	return s, true
}

// --- page tree ---

// countPages walks the tree from num. seen is never cleared: a node reached
// twice (a repeated kid, a shared subtree or a cycle) makes the tree
// unresolvable, so the walk visits each object at most once (#504).
func (d *pdfDoc) countPages(num, depth int, seen map[int]bool) (int, bool) {
	if depth > maxPDFDepth || num <= 0 || seen[num] {
		return 0, false
	}
	seen[num] = true
	obj, ok := d.dict(num)
	if !ok {
		return 0, false
	}
	switch pdfName(obj["Type"]) {
	case "Page":
		return 1, true
	case "Pages":
	default:
		return 0, false
	}
	count := -1
	if v, has := obj["Count"]; has {
		c, ok := pdfInt(v)
		if !ok || c < 0 || c > maxPDFObject {
			return 0, false
		}
		count = c
	}
	kidsRaw := obj["Kids"]
	if ref := pdfRef(kidsRaw); ref > 0 {
		arr, ok := d.dict(ref)
		if !ok {
			return 0, false
		}
		kidsRaw = arr[""]
	}
	// A /Pages node without a /Kids array is not a resolvable tree.
	// Trusting /Count alone would let a short count hide loose pages.
	kids, ok := pdfRefs(kidsRaw)
	if !ok || len(kids) == 0 {
		return 0, false
	}
	sum := 0
	for _, kid := range kids {
		n, ok := d.countPages(kid, depth+1, seen)
		if !ok {
			return 0, false
		}
		sum += n
	}
	// A /Count that disagrees with the tree is not a reason to trust it:
	// refuse rather than choose the smaller number.
	if count >= 0 && sum != count {
		return 0, false
	}
	return sum, true
}

// --- filters ---

// decode applies the stream's filters, charging every decoded byte to the
// upload's budget.
func (d *pdfDoc) decode(dict map[string][]byte, data []byte) ([]byte, bool) {
	var filters []string
	if f := dict["Filter"]; f != nil {
		if name := pdfName(f); name != "" {
			filters = []string{name}
		} else {
			elems, ok := pdfArray(f)
			if !ok {
				return nil, false
			}
			for _, e := range elems {
				name := pdfName(e)
				if name == "" {
					return nil, false
				}
				filters = append(filters, name)
			}
		}
	}
	parms := make([]map[string][]byte, len(filters))
	if v := dict["DecodeParms"]; v != nil {
		if pd, _, ok := pdfDict(v, 0, 0); ok && len(filters) > 0 {
			parms[0] = pd
		} else if elems, ok := pdfArray(v); ok {
			for i, e := range elems {
				if i < len(parms) {
					parms[i], _, _ = pdfDict(e, 0, 0)
				}
			}
		}
	}
	out := data
	for i, f := range filters {
		limit := d.budget.limit()
		var err error
		switch f {
		case "FlateDecode", "Fl":
			out, err = flateBytes(out, limit)
			if err == nil {
				out, err = pdfPredict(out, parms[i])
			}
		case "ASCII85Decode", "A85":
			out, err = ascii85Bytes(out, limit)
		case "ASCIIHexDecode", "AHx":
			out, err = asciiHexBytes(out, limit)
		default:
			return nil, false
		}
		if err != nil || !d.budget.spend(len(out)) {
			return nil, false
		}
	}
	if len(out) > maxInflate {
		return nil, false
	}
	return out, true
}

// pdfPredict undoes a PNG predictor (/Predictor 10-15), which xref streams
// almost always use.
func pdfPredict(b []byte, parms map[string][]byte) ([]byte, error) {
	if parms == nil {
		return b, nil
	}
	pred, ok := pdfInt(parms["Predictor"])
	if !ok || pred <= 1 {
		return b, nil
	}
	if pred < 10 || pred > 15 {
		return nil, errors.New("unsupported predictor")
	}
	cols, colors, bpc := 1, 1, 8
	if v, ok := pdfInt(parms["Columns"]); ok {
		cols = v
	}
	if v, ok := pdfInt(parms["Colors"]); ok {
		colors = v
	}
	if v, ok := pdfInt(parms["BitsPerComponent"]); ok {
		bpc = v
	}
	if cols <= 0 || cols > 1<<16 || colors <= 0 || colors > 32 || (bpc != 1 && bpc != 2 && bpc != 4 && bpc != 8 && bpc != 16) {
		return nil, errors.New("bad predictor parameters")
	}
	bpp := (colors*bpc + 7) / 8
	rowLen := (cols*colors*bpc + 7) / 8
	if len(b)%(rowLen+1) != 0 {
		return nil, errors.New("predictor rows do not fit")
	}
	out := make([]byte, 0, len(b)/(rowLen+1)*rowLen)
	prev := make([]byte, rowLen)
	for p := 0; p < len(b); p += rowLen + 1 {
		ft, row := b[p], b[p+1:p+1+rowLen]
		cur := make([]byte, rowLen)
		for i := 0; i < rowLen; i++ {
			var left, upLeft byte
			if i >= bpp {
				left, upLeft = cur[i-bpp], prev[i-bpp]
			}
			up := prev[i]
			switch ft {
			case 0:
				cur[i] = row[i]
			case 1:
				cur[i] = row[i] + left
			case 2:
				cur[i] = row[i] + up
			case 3:
				cur[i] = row[i] + byte((int(left)+int(up))/2)
			case 4:
				cur[i] = row[i] + paeth(left, up, upLeft)
			default:
				return nil, errors.New("bad PNG filter type")
			}
		}
		out = append(out, cur...)
		prev = cur
	}
	return out, nil
}

func paeth(a, b, c byte) byte {
	p := int(a) + int(b) - int(c)
	pa, pb, pc := abs(p-int(a)), abs(p-int(b)), abs(p-int(c))
	switch {
	case pa <= pb && pa <= pc:
		return a
	case pb <= pc:
		return b
	}
	return c
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func flateBytes(b []byte, limit int) ([]byte, error) {
	zr, err := zlib.NewReader(bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	out, err := io.ReadAll(io.LimitReader(zr, int64(limit)+1))
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
		return nil, err
	}
	if len(out) > limit {
		return nil, errPDFBudget
	}
	return out, nil
}

func ascii85Bytes(b []byte, limit int) ([]byte, error) {
	if i := bytes.Index(b, []byte("~>")); i >= 0 {
		b = b[:i]
	}
	r := ascii85.NewDecoder(bytes.NewReader(b))
	out, err := io.ReadAll(io.LimitReader(r, int64(limit)+1))
	if err != nil {
		return nil, err
	}
	if len(out) > limit {
		return nil, errPDFBudget
	}
	return out, nil
}

func asciiHexBytes(b []byte, limit int) ([]byte, error) {
	if len(b)/2 > limit+1 {
		return nil, errPDFBudget
	}
	out := make([]byte, 0, len(b)/2)
	var hi *byte
	for _, c := range b {
		if c == '>' {
			break
		}
		if pdfIsSpace(c) {
			continue
		}
		v, ok := hexVal(c)
		if !ok {
			return nil, fmt.Errorf("bad ASCIIHex digit %q", c)
		}
		if hi == nil {
			x := v
			hi = &x
			continue
		}
		out = append(out, *hi<<4|v)
		hi = nil
	}
	if hi != nil {
		out = append(out, *hi<<4)
	}
	if len(out) > limit {
		return nil, errPDFBudget
	}
	return out, nil
}

// --- lexer ---

func pdfIsSpace(c byte) bool {
	return c == 0 || c == '\t' || c == '\n' || c == '\f' || c == '\r' || c == ' '
}

func pdfIsDelim(c byte) bool {
	switch c {
	case '(', ')', '<', '>', '[', ']', '{', '}', '/', '%':
		return true
	}
	return pdfIsSpace(c)
}

func pdfIsDigit(c byte) bool { return c >= '0' && c <= '9' }

// pdfSkip skips white space and comments.
func pdfSkip(b []byte, p int) int {
	for p < len(b) {
		switch {
		case pdfIsSpace(b[p]):
			p++
		case b[p] == '%':
			for p < len(b) && b[p] != '\n' && b[p] != '\r' {
				p++
			}
		default:
			return p
		}
	}
	return p
}

// pdfKeyword reports whether the regular token at p is exactly kw.
func pdfKeyword(b []byte, p int, kw string) bool {
	if p < 0 || !bytes.HasPrefix(b[min(p, len(b)):], []byte(kw)) {
		return false
	}
	e := p + len(kw)
	return e == len(b) || pdfIsDelim(b[e])
}

// pdfReadInt reads an unsigned integer at p.
func pdfReadInt(b []byte, p int) (int, int, bool) {
	start := p
	for p < len(b) && pdfIsDigit(b[p]) && p-start < 12 {
		p++
	}
	if p == start || (p < len(b) && pdfIsDigit(b[p])) {
		return 0, p, false
	}
	n, err := strconv.Atoi(string(b[start:p]))
	return n, p, err == nil
}

// pdfValueEnd returns the end of the value at p: a dictionary, array,
// string, hex string, name or regular token (a number or keyword). An
// indirect reference "n g R" is three values; pdfDict and pdfArray join them.
func pdfValueEnd(b []byte, p, depth int) (int, bool) {
	if p >= len(b) || depth > maxPDFNesting {
		return 0, false
	}
	switch c := b[p]; c {
	case '<':
		if p+1 < len(b) && b[p+1] == '<' {
			_, end, ok := pdfDict(b, p, depth)
			return end, ok
		}
		e := bytes.IndexByte(b[p:], '>')
		if e < 0 {
			return 0, false
		}
		return p + e + 1, true
	case '[':
		q := p + 1
		for {
			q = pdfSkip(b, q)
			if q >= len(b) {
				return 0, false
			}
			if b[q] == ']' {
				return q + 1, true
			}
			e, ok := pdfValueEnd(b, q, depth+1)
			if !ok {
				return 0, false
			}
			q = e
		}
	case '(':
		level := 0
		for q := p; q < len(b); q++ {
			switch b[q] {
			case '\\':
				q++
			case '(':
				level++
			case ')':
				level--
				if level == 0 {
					return q + 1, true
				}
			}
		}
		return 0, false
	case '/':
		q := p + 1
		for q < len(b) && !pdfIsDelim(b[q]) {
			q++
		}
		return q, true
	case ')', '>', ']':
		return 0, false
	case '{', '}':
		return p + 1, true
	default:
		q := p
		for q < len(b) && !pdfIsDelim(b[q]) {
			q++
		}
		return q, q > p
	}
}

func pdfValueEndOr(b []byte, p int) int {
	if e, ok := pdfValueEnd(b, p, 0); ok {
		return e
	}
	return p
}

// pdfJoinRef extends an integer value at [p,e) to "n g R" when one follows.
func pdfJoinRef(b []byte, p, e int) int {
	if _, end, ok := pdfReadInt(b, p); !ok || end != e {
		return e
	}
	q := pdfSkip(b, e)
	_, q2, ok := pdfReadInt(b, q)
	if !ok || q2 == q || (q2 < len(b) && !pdfIsDelim(b[q2])) {
		return e
	}
	r := pdfSkip(b, q2)
	if pdfKeyword(b, r, "R") {
		return r + 1
	}
	return e
}

// pdfDict parses the dictionary at p into its top-level keys (names decoded,
// so /P#61ge is /Page) and raw values. Nested dictionaries stay raw, so a
// /Type inside a nested dictionary is not the object's type. A repeated key
// keeps its last value.
func pdfDict(b []byte, p, depth int) (map[string][]byte, int, bool) {
	if depth > maxPDFNesting || p+1 >= len(b) || b[p] != '<' || b[p+1] != '<' {
		return nil, 0, false
	}
	dict := make(map[string][]byte)
	q := p + 2
	for {
		q = pdfSkip(b, q)
		if q+1 < len(b) && b[q] == '>' && b[q+1] == '>' {
			return dict, q + 2, true
		}
		if q >= len(b) || b[q] != '/' {
			return nil, 0, false
		}
		ke, _ := pdfValueEnd(b, q, depth+1)
		key := string(pdfDecodeName(b[q+1 : ke]))
		vs := pdfSkip(b, ke)
		ve, ok := pdfValueEnd(b, vs, depth+1)
		if !ok {
			return nil, 0, false
		}
		ve = pdfJoinRef(b, vs, ve)
		dict[key] = b[vs:ve]
		q = ve
	}
}

// pdfArray splits an array value into its elements, references joined.
func pdfArray(v []byte) ([][]byte, bool) {
	p := pdfSkip(v, 0)
	if p >= len(v) || v[p] != '[' {
		return nil, false
	}
	var out [][]byte
	q := p + 1
	for {
		q = pdfSkip(v, q)
		if q >= len(v) {
			return nil, false
		}
		if v[q] == ']' {
			return out, true
		}
		e, ok := pdfValueEnd(v, q, 1)
		if !ok {
			return nil, false
		}
		e = pdfJoinRef(v, q, e)
		out = append(out, v[q:e])
		q = e
	}
}

// pdfName returns a name value without its slash, or "".
func pdfName(v []byte) string {
	v = bytes.TrimSpace(v)
	if len(v) < 2 || v[0] != '/' {
		return ""
	}
	return string(pdfDecodeName(v[1:]))
}

func pdfInt(v []byte) (int, bool) {
	v = bytes.TrimSpace(v)
	n, e, ok := pdfReadInt(v, 0)
	return n, ok && e == len(v)
}

func pdfInts(v []byte) []int {
	elems, ok := pdfArray(v)
	if !ok {
		return nil
	}
	out := make([]int, 0, len(elems))
	for _, e := range elems {
		n, ok := pdfInt(e)
		if !ok {
			return nil
		}
		out = append(out, n)
	}
	return out
}

// pdfRef returns n for an indirect reference "n g R", or 0.
func pdfRef(v []byte) int {
	n, p, ok := pdfReadInt(v, pdfSkip(v, 0))
	if !ok {
		return 0
	}
	_, p2, ok := pdfReadInt(v, pdfSkip(v, p))
	if !ok || p2 == p {
		return 0
	}
	r := pdfSkip(v, p2)
	if !pdfKeyword(v, r, "R") || pdfSkip(v, r+1) != len(v) {
		return 0
	}
	return n
}

// pdfRefs reads an array of references; any other element fails.
func pdfRefs(v []byte) ([]int, bool) {
	elems, ok := pdfArray(v)
	if !ok {
		return nil, false
	}
	out := make([]int, 0, len(elems))
	for _, e := range elems {
		n := pdfRef(e)
		if n <= 0 {
			return nil, false
		}
		out = append(out, n)
	}
	return out, true
}

func pdfDecodeName(name []byte) []byte {
	if bytes.IndexByte(name, '#') < 0 {
		return name
	}
	out := make([]byte, 0, len(name))
	for i := 0; i < len(name); i++ {
		if name[i] == '#' && i+2 < len(name) {
			h, ok1 := hexVal(name[i+1])
			l, ok2 := hexVal(name[i+2])
			if ok1 && ok2 {
				out = append(out, h<<4|l)
				i += 2
				continue
			}
		}
		out = append(out, name[i])
	}
	return out
}

func hexVal(c byte) (byte, bool) {
	switch {
	case c >= '0' && c <= '9':
		return c - '0', true
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10, true
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10, true
	}
	return 0, false
}
