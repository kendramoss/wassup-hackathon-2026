package media

import (
	"bytes"
	"compress/zlib"
	"encoding/ascii85"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// PDF page counting reads the document's page tree, not a regex over the
// bytes. Incremental saves rewrite page objects (so an old revision must not
// add pages), and PDF names may be written with #xx escapes (so /P#61ge is a
// page too). A tree that cannot be resolved is refused by returning 0; no
// raw-text fallback can silently undercount a long document (#381).

const (
	maxInflate   = 4 << 20
	maxPDFObject = 200_000
	maxPDFDepth  = 64
)

var (
	pdfObjHeader = regexp.MustCompile(`(?:^|[\r\n])[ \t]*(\d+)[ \t]+\d+[ \t]+obj\b`)
	pdfStreamAt  = regexp.MustCompile(`\bstream\r?\n`)
	pdfPrev      = regexp.MustCompile(`/Prev[ \t]+(\d+)`)
	pdfStartXref = regexp.MustCompile(`startxref[ \t]+(\d+)`)
	pdfLength    = regexp.MustCompile(`/Length[ \t]+(\d+)`)
	pdfLengthRef = regexp.MustCompile(`/Length[ \t]+(\d+)[ \t]+\d+[ \t]+R\b`)
	pdfNs        = regexp.MustCompile(`/N[ \t]+(\d+)`)
	pdfFirst     = regexp.MustCompile(`/First[ \t]+(\d+)`)
	pdfCount     = regexp.MustCompile(`/Count[ \t]+(\d+)`)
	pdfKids      = regexp.MustCompile(`/Kids[ \t]*\[([\s\S]*?)\]`)
	pdfKidsRef   = regexp.MustCompile(`/Kids[ \t]+(\d+)[ \t]+\d+[ \t]+R\b`)
	pdfRefAny    = regexp.MustCompile(`(\d+)[ \t]+\d+[ \t]+R\b`)
	pdfRootRef   = regexp.MustCompile(`/Root[ \t]+(\d+)[ \t]+\d+[ \t]+R\b`)
	pdfPagesRef  = regexp.MustCompile(`/Pages[ \t]+(\d+)[ \t]+\d+[ \t]+R\b`)
	pdfFilter    = regexp.MustCompile(`/Filter[ \t]*(?:\[([\s\S]*?)\]|/([A-Za-z0-9]+))`)
	pdfTypePage  = regexp.MustCompile(`/Type[ \t]*/Page\b`)
	pdfTypePages = regexp.MustCompile(`/Type[ \t]*/Pages\b`)
	pdfTypeCat   = regexp.MustCompile(`/Type[ \t]*/Catalog\b`)
	pdfTypeObjSt = regexp.MustCompile(`/Type[ \t]*/ObjStm\b`)
	pdfTypeXRef  = regexp.MustCompile(`/Type[ \t]*/XRef\b`)
	pdfAnyInt    = regexp.MustCompile(`(\d+)`)
)

type pdfObject struct {
	raw []byte
}

type pdfDoc struct {
	b       []byte
	objects map[int]pdfObject
}

// PDFPages returns the number of leaves of the document's page tree, or 0
// when the tree cannot be resolved. Callers treat 0 as "pages could not be
// counted" and refuse the file.
func PDFPages(b []byte) int {
	if !bytes.HasPrefix(b, []byte("%PDF-")) {
		return 0
	}
	d := pdfDoc{b: pdfUnescapeNames(b), objects: make(map[int]pdfObject)}
	if !d.load() {
		return 0
	}
	d.dropFreed()
	root, ok := d.rootPages()
	if !ok {
		return 0
	}
	n, ok := d.countPages(root, 0, make(map[int]bool))
	if !ok || n <= 0 {
		return 0
	}
	return n
}

// pdfUnescapeNames decodes #xx in PDF names only. Strings, comments and
// stream data are copied unchanged, so a comment saying "% /Type /Page"
// cannot become a page object and a hex escape inside a name can.
func pdfUnescapeNames(b []byte) []byte {
	out := make([]byte, 0, len(b))
	inString, stringDepth, inHex, inComment := 0, 0, false, false
	for i := 0; i < len(b); i++ {
		c := b[i]
		if inComment {
			out = append(out, c)
			if c == '\n' || c == '\r' {
				inComment = false
			}
			continue
		}
		if inHex {
			out = append(out, c)
			if c == '>' {
				inHex = false
			}
			continue
		}
		if inString > 0 {
			out = append(out, c)
			if c == '\\' && i+1 < len(b) {
				i++
				out = append(out, b[i])
			} else if c == '(' {
				stringDepth++
			} else if c == ')' {
				stringDepth--
				if stringDepth == 0 {
					inString = 0
				}
			}
			continue
		}
		switch c {
		case '%':
			inComment = true
			out = append(out, c)
		case '(':
			inString = 1
			stringDepth = 1
			out = append(out, c)
		case '<':
			if i+1 < len(b) && b[i+1] == '<' {
				out = append(out, c, b[i+1])
				i++
				continue
			}
			inHex = true
			out = append(out, c)
		case '/':
			start := i
			i++
			for i < len(b) && !pdfIsDelim(b[i]) {
				i++
			}
			out = append(out, pdfDecodeName(b[start:i])...)
			i--
		default:
			out = append(out, c)
		}
	}
	return out
}

func pdfIsDelim(c byte) bool {
	return c <= 0x20 || strings.IndexByte("()<>[]{}/%", c) >= 0
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

// load reads every top-level object, later revisions winning over earlier
// ones, then unpacks compressed object streams. Xref free entries are applied
// afterwards.
func (d *pdfDoc) load() bool {
	locs := pdfObjHeader.FindAllSubmatchIndex(d.b, -1)
	if len(locs) == 0 || len(locs) > maxPDFObject {
		return false
	}
	for _, loc := range locs {
		n, err := strconv.Atoi(string(d.b[loc[2]:loc[3]]))
		if err != nil || n <= 0 {
			continue
		}
		d.objects[n] = pdfObject{raw: pdfRawObject(d.b, loc[2])}
	}
	if len(d.objects) == 0 {
		return false
	}
	return d.expandObjectStreams()
}

// pdfRawObject returns an object from its number up to endobj. A stream is
// skipped through endstream first: its binary data may contain "endobj".
func pdfRawObject(b []byte, from int) []byte {
	end := bytes.Index(b[from:], []byte("endobj"))
	if s := pdfStreamAt.FindIndex(b[from:]); s != nil && (end < 0 || s[0] < end) {
		data := from + s[1]
		if e := bytes.Index(b[data:], []byte("endstream")); e >= 0 {
			after := data + e + len("endstream")
			if o := bytes.Index(b[after:], []byte("endobj")); o >= 0 {
				return b[from : after+o+len("endobj")]
			}
		}
	}
	if end < 0 {
		return b[from:]
	}
	return b[from : from+end+len("endobj")]
}

func (d *pdfDoc) expandObjectStreams() bool {
	for _, num := range d.sorted() {
		raw := d.objects[num].raw
		if !pdfTypeObjSt.Match(raw) {
			continue
		}
		data, ok := pdfStreamData(d, raw)
		if !ok {
			continue
		}
		plain, ok := pdfDecodeFilters(raw, data)
		if !ok || len(plain) > maxInflate {
			continue
		}
		n := pdfFirstInt(raw, pdfNs)
		first := pdfFirstInt(raw, pdfFirst)
		if n <= 0 || n > maxPDFObject || first < 0 || first > len(plain) {
			continue
		}
		header := strings.Fields(string(plain[:first]))
		if len(header) < 2*n {
			continue
		}
		starts := make([]int, n)
		ids := make([]int, n)
		for i := 0; i < n; i++ {
			ids[i], _ = strconv.Atoi(header[2*i])
			off, err := strconv.Atoi(header[2*i+1])
			if err != nil || ids[i] <= 0 {
				continue
			}
			starts[i] = first + off
		}
		for i := 0; i < n; i++ {
			end := len(plain)
			if i+1 < n && starts[i+1] > starts[i] {
				end = starts[i+1]
			}
			if ids[i] > 0 && starts[i] >= first && starts[i] < end && end <= len(plain) {
				d.objects[ids[i]] = pdfObject{raw: plain[starts[i]:end]}
			}
		}
	}
	return true
}

func (d *pdfDoc) sorted() []int {
	nums := make([]int, 0, len(d.objects))
	for n := range d.objects {
		nums = append(nums, n)
	}
	sort.Ints(nums)
	return nums
}

// dropFreed follows the /Prev chain of classic xref tables. An object freed
// in the newest revision is removed even though its old bytes are still in
// the file; an older entry must not bring it back.
func (d *pdfDoc) dropFreed() {
	idx := bytes.LastIndex(d.b, []byte("startxref"))
	if idx < 0 {
		return
	}
	loc := pdfStartXref.FindSubmatch(d.b[idx:])
	if loc == nil {
		return
	}
	off, err := strconv.Atoi(string(loc[1]))
	if err != nil {
		return
	}
	seenSection := make(map[int]bool)
	status := make(map[int]byte) // first (newest) entry for an object wins
	for off > 0 && off < len(d.b) {
		if seenSection[off] {
			return
		}
		seenSection[off] = true
		section := bytes.TrimLeft(d.b[off:], " \t\r\n")
		if !bytes.HasPrefix(section, []byte("xref")) {
			return
		}
		p := 4
		for {
			p = skipSpace(section, p)
			if p >= len(section) || !isDigit(section[p]) {
				break
			}
			start, np := readInt(section, p)
			p = skipSpace(section, np)
			count, np := readInt(section, p)
			p = skipSpace(section, np)
			if count < 0 || count > maxPDFObject {
				return
			}
			for k := 0; k < count; k++ {
				eol := bytes.IndexByte(section[p:], '\n')
				if eol < 0 {
					return
				}
				fields := strings.Fields(string(section[p : p+eol]))
				if len(fields) < 3 {
					return
				}
				obj := start + k
				if _, seen := status[obj]; !seen {
					status[obj] = fields[2][0]
				}
				p += eol + 1
			}
		}
		t := bytes.Index(section, []byte("trailer"))
		if t < 0 {
			return
		}
		prev := pdfPrev.FindSubmatch(section[t:])
		if prev == nil {
			break
		}
		off, err = strconv.Atoi(string(prev[1]))
		if err != nil {
			return
		}
	}
	for obj, st := range status {
		if st == 'f' {
			delete(d.objects, obj)
		}
	}
}

func skipSpace(b []byte, p int) int {
	for p < len(b) && (b[p] == ' ' || b[p] == '\t' || b[p] == '\r' || b[p] == '\n') {
		p++
	}
	return p
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

func readInt(b []byte, p int) (int, int) {
	start := p
	for p < len(b) && isDigit(b[p]) {
		p++
	}
	n, _ := strconv.Atoi(string(b[start:p]))
	return n, p
}

// rootPages reaches the page tree through the newest trailer's /Root (or an
// xref stream's /Root, or the catalog object), then its /Pages reference.
func (d *pdfDoc) rootPages() (int, bool) {
	if i := bytes.LastIndex(d.b, []byte("trailer")); i >= 0 {
		if ref := pdfRefValue(d.b[i:], pdfRootRef); ref > 0 {
			if n, ok := d.catalogPages(ref); ok {
				return n, true
			}
		}
	}
	for _, num := range d.sorted() {
		raw := d.objects[num].raw
		if pdfTypeXRef.Match(raw) {
			if ref := pdfRefValue(raw, pdfRootRef); ref > 0 {
				if n, ok := d.catalogPages(ref); ok {
					return n, true
				}
			}
		}
	}
	for _, num := range d.sorted() {
		if pdfTypeCat.Match(d.objects[num].raw) {
			if n, ok := d.catalogPages(num); ok {
				return n, true
			}
		}
	}
	// Some writers (and our small fixtures) have no catalog; a /Pages object
	// with no /Parent is the root. This is still a parsed tree, not a regex
	// count of loose page objects.
	for _, num := range d.sorted() {
		raw := d.objects[num].raw
		if pdfTypePages.Match(raw) && !bytes.Contains(raw, []byte("/Parent")) {
			return num, true
		}
	}
	return 0, false
}

func (d *pdfDoc) catalogPages(catalog int) (int, bool) {
	raw, ok := d.objects[catalog]
	if !ok {
		return 0, false
	}
	ref := pdfRefValue(raw.raw, pdfPagesRef)
	if ref <= 0 {
		return 0, false
	}
	if _, ok := d.objects[ref]; !ok {
		return 0, false
	}
	return ref, true
}

func pdfRefValue(raw []byte, re *regexp.Regexp) int {
	m := re.FindSubmatch(raw)
	if m == nil {
		return 0
	}
	n, err := strconv.Atoi(string(m[1]))
	if err != nil {
		return 0
	}
	return n
}

func (d *pdfDoc) countPages(num, depth int, seen map[int]bool) (int, bool) {
	if depth > maxPDFDepth || num <= 0 || seen[num] {
		return 0, false
	}
	obj, ok := d.objects[num]
	if !ok {
		return 0, false
	}
	seen[num] = true
	defer delete(seen, num)
	switch {
	case pdfTypePage.Match(obj.raw):
		return 1, true
	case !pdfTypePages.Match(obj.raw):
		return 0, false
	}
	count := -1
	if m := pdfCount.FindSubmatch(obj.raw); m != nil {
		count, _ = strconv.Atoi(string(m[1]))
		if count < 0 || count > maxPDFObject {
			return 0, false
		}
	}
	var kidsRaw []byte
	if m := pdfKids.FindSubmatch(obj.raw); m != nil {
		kidsRaw = m[1]
	} else if ref := pdfRefValue(obj.raw, pdfKidsRef); ref > 0 {
		if kids, ok := d.objects[ref]; ok {
			if m := pdfKids.FindSubmatch(kids.raw); m != nil {
				kidsRaw = m[1]
			}
		}
	}
	if len(kidsRaw) == 0 {
		// A /Pages node without a /Kids array is not a resolvable tree.
		// Trusting /Count alone would let a short count hide loose pages.
		return 0, false
	}
	var kids []int
	for _, r := range pdfRefAny.FindAllSubmatch(kidsRaw, -1) {
		n, err := strconv.Atoi(string(r[1]))
		if err != nil || n <= 0 {
			return 0, false
		}
		kids = append(kids, n)
	}
	if len(kids) == 0 {
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

func pdfStreamData(d *pdfDoc, raw []byte) ([]byte, bool) {
	loc := pdfStreamAt.FindIndex(raw)
	if loc == nil {
		return nil, false
	}
	start := loc[1]
	if m := pdfLength.FindSubmatch(raw[:loc[0]]); m != nil {
		if n, err := strconv.Atoi(string(m[1])); err == nil && n >= 0 && start+n <= len(raw) {
			return raw[start : start+n], true
		}
	}
	if m := pdfLengthRef.FindSubmatch(raw[:loc[0]]); m != nil {
		if lengthObj, ok := d.objects[pdfInt(m[1])]; ok {
			if n := pdfFirstInt(lengthObj.raw, pdfAnyInt); n >= 0 && start+n <= len(raw) {
				return raw[start : start+n], true
			}
		}
	}
	e := bytes.Index(raw[start:], []byte("endstream"))
	if e < 0 {
		return nil, false
	}
	return bytes.TrimRight(raw[start:start+e], "\r\n"), true
}

func pdfDecodeFilters(dict, data []byte) ([]byte, bool) {
	filters := pdfFilterNames(dict)
	out := data
	for _, f := range filters {
		var err error
		switch f {
		case "FlateDecode", "Fl":
			out, err = flateBytes(out)
		case "ASCII85Decode", "A85":
			out, err = ascii85Bytes(out)
		case "ASCIIHexDecode", "AHx":
			out, err = asciiHexBytes(out)
		default:
			return nil, false
		}
		if err != nil || len(out) > maxInflate {
			return nil, false
		}
	}
	return out, true
}

func pdfFilterNames(dict []byte) []string {
	m := pdfFilter.FindSubmatch(dict)
	if m == nil {
		return nil
	}
	if len(m[2]) > 0 {
		return []string{string(m[2])}
	}
	var names []string
	for _, n := range bytes.Split(m[1], []byte{'/'}) {
		name := strings.TrimSpace(string(n))
		if i := strings.IndexAny(name, " \t\r\n[]"); i >= 0 {
			name = name[:i]
		}
		if name != "" {
			names = append(names, name)
		}
	}
	return names
}

func pdfFirstInt(raw []byte, re *regexp.Regexp) int {
	m := re.FindSubmatch(raw)
	if m == nil {
		return -1
	}
	return pdfInt(m[1])
}

func pdfInt(b []byte) int {
	n, err := strconv.Atoi(string(b))
	if err != nil {
		return -1
	}
	return n
}

func flateBytes(b []byte) ([]byte, error) {
	zr, err := zlib.NewReader(bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	out, err := io.ReadAll(io.LimitReader(zr, maxInflate+1))
	if err != nil {
		return nil, err
	}
	if len(out) > maxInflate {
		return nil, errors.New("pdf stream too large")
	}
	return out, nil
}

func ascii85Bytes(b []byte) ([]byte, error) {
	r := ascii85.NewDecoder(bytes.NewReader(b))
	out, err := io.ReadAll(io.LimitReader(r, maxInflate+1))
	if err != nil {
		return nil, err
	}
	if len(out) > maxInflate {
		return nil, errors.New("pdf stream too large")
	}
	return out, nil
}

func asciiHexBytes(b []byte) ([]byte, error) {
	out := make([]byte, 0, len(b)/2)
	var hi *byte
	for _, c := range b {
		if c == '>' {
			break
		}
		if c <= ' ' {
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
	if len(out) > maxInflate {
		return nil, errors.New("pdf stream too large")
	}
	return out, nil
}
