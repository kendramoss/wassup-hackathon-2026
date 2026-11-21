// Package media checks uploaded images and scanned PDFs before they are sent
// to a model. It trusts only the bytes: the type is sniffed from the magic
// number (never the file name or the Content-Type header), image sizes are
// read from the header without decoding the pixels (so a decompression bomb
// is refused before any large allocation), and PDF pages are counted so a
// long document is refused rather than sent. Nothing here stores, logs or
// copies the bytes.
package media

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg" // register DecodeConfig for JPEG
	_ "image/png"  // register DecodeConfig for PNG
	"io"
	"regexp"
	"strconv"
)

// Limits for one request. They are deliberately strict: a BOM photo or a
// short scanned spec sheet fits easily, and the model call stays cheap.
const (
	MaxFiles      = 4
	MaxTotalBytes = 8 << 20
	// MaxPixels caps one image (width x height). A phone photo is about
	// 12 MP; 40 MP refuses decompression bombs and panoramas.
	MaxPixels = 40_000_000
	// MaxSide caps either side of an image, in pixels.
	MaxSide = 12_000
	// MaxPDFPages caps the pages of all PDFs in one request together.
	MaxPDFPages = 10
)

// Types the endpoint accepts.
const (
	PNG  = "image/png"
	JPEG = "image/jpeg"
	WEBP = "image/webp"
	PDF  = "application/pdf"
)

// ErrUnsupported is a file whose bytes are not one of the accepted types.
var ErrUnsupported = errors.New("unsupported file type")

// ErrRejected is a file of an accepted type that fails a check (empty,
// damaged, too large in pixels or pages, encrypted).
var ErrRejected = errors.New("file rejected")

// checkError reads as its message alone and matches its kind with errors.Is.
type checkError struct {
	kind error
	msg  string
}

func (e checkError) Error() string { return e.msg }
func (e checkError) Unwrap() error { return e.kind }

func rejected(format string, a ...any) error {
	return checkError{ErrRejected, fmt.Sprintf(format, a...)}
}

// Info describes a checked file. It holds no content.
type Info struct {
	MIMEType string `json:"mime_type"`
	Bytes    int    `json:"bytes"`
	Width    int    `json:"width,omitempty"`
	Height   int    `json:"height,omitempty"`
	Pages    int    `json:"pages,omitempty"`
}

// Kind is "image" or "pdf".
func (i Info) Kind() string {
	if i.MIMEType == PDF {
		return "pdf"
	}
	return "image"
}

// Sniff returns the media type from the leading bytes, or "" when the bytes
// are not one of the accepted types.
func Sniff(b []byte) string {
	switch {
	case bytes.HasPrefix(b, []byte("\x89PNG\r\n\x1a\n")):
		return PNG
	case bytes.HasPrefix(b, []byte{0xFF, 0xD8, 0xFF}):
		return JPEG
	case len(b) >= 12 && bytes.Equal(b[0:4], []byte("RIFF")) && bytes.Equal(b[8:12], []byte("WEBP")):
		return WEBP
	case bytes.HasPrefix(b, []byte("%PDF-")):
		return PDF
	}
	return ""
}

// Check sniffs b and runs the checks for its type.
func Check(b []byte) (Info, error) {
	if len(b) == 0 {
		return Info{}, rejected("the file is empty")
	}
	info := Info{MIMEType: Sniff(b), Bytes: len(b)}
	switch info.MIMEType {
	case PNG, JPEG:
		cfg, _, err := image.DecodeConfig(bytes.NewReader(b))
		if err != nil {
			return Info{}, rejected("the image could not be read")
		}
		info.Width, info.Height = cfg.Width, cfg.Height
	case WEBP:
		w, h, err := webpSize(b)
		if err != nil {
			return Info{}, rejected("the image could not be read")
		}
		info.Width, info.Height = w, h
	case PDF:
		if pdfEncrypted.Match(b) {
			return Info{}, rejected("the PDF is encrypted; save an unprotected copy and try again")
		}
		n := PDFPages(b)
		if n == 0 {
			return Info{}, rejected("the PDF's pages could not be counted; it may be damaged")
		}
		info.Pages = n
		return info, nil
	default:
		return Info{}, checkError{ErrUnsupported, "send a PNG, JPEG or WebP image, or a PDF"}
	}
	if info.Width <= 0 || info.Height <= 0 {
		return Info{}, rejected("the image has no pixels")
	}
	if info.Width > MaxSide || info.Height > MaxSide || info.Width*info.Height > MaxPixels {
		return Info{}, rejected("the image is %d x %d pixels; the limit is %d pixels a side and %d megapixels",
			info.Width, info.Height, MaxSide, MaxPixels/1_000_000)
	}
	return info, nil
}

// webpSize reads the canvas size from a WebP header (VP8, VP8L or VP8X).
func webpSize(b []byte) (int, int, error) {
	if len(b) < 30 {
		return 0, 0, errors.New("short")
	}
	chunk, data := string(b[12:16]), b[20:]
	switch chunk {
	case "VP8 ":
		// Frame tag (3 bytes), start code 9d 01 2a, then 14-bit sizes.
		if len(data) < 10 || data[3] != 0x9d || data[4] != 0x01 || data[5] != 0x2a {
			return 0, 0, errors.New("bad VP8")
		}
		w := int(binary.LittleEndian.Uint16(data[6:8]) & 0x3fff)
		h := int(binary.LittleEndian.Uint16(data[8:10]) & 0x3fff)
		return w, h, nil
	case "VP8L":
		if len(data) < 5 || data[0] != 0x2f {
			return 0, 0, errors.New("bad VP8L")
		}
		bits := binary.LittleEndian.Uint32(data[1:5])
		return int(bits&0x3fff) + 1, int((bits>>14)&0x3fff) + 1, nil
	case "VP8X":
		if len(data) < 10 {
			return 0, 0, errors.New("bad VP8X")
		}
		w := int(uint32(data[4]) | uint32(data[5])<<8 | uint32(data[6])<<16)
		h := int(uint32(data[7]) | uint32(data[8])<<8 | uint32(data[9])<<16)
		return w + 1, h + 1, nil
	}
	return 0, 0, errors.New("unknown WebP chunk")
}

var (
	pdfEncrypted = regexp.MustCompile(`/Encrypt\s*\d+\s+\d+\s+R|/Encrypt\s*<<`)
	// A page object: "/Type /Page" not followed by "s" (that is /Pages).
	pdfPage  = regexp.MustCompile(`/Type\s*/Page(?:[^s]|$)`)
	pdfCount = regexp.MustCompile(`/Type\s*/Pages\b[^>]*?/Count\s+(\d+)|/Count\s+(\d+)[^>]*?/Type\s*/Pages\b`)
	// The dictionary in front of a compressed object stream.
	pdfObjStm = regexp.MustCompile(`<<[^>]*?/Type\s*/ObjStm[^>]*?>>\s*stream\r?\n`)
)

// maxInflate caps the decompressed bytes read from object streams while
// counting pages, so a crafted stream cannot exhaust memory.
const maxInflate = 4 << 20

// PDFPages counts the pages of a PDF without rendering it: page objects in
// the file, page objects in its compressed object streams, and the page
// tree's /Count, whichever is largest (a maximum, not a sum, so a stream
// stored uncompressed is not counted twice). It returns 0 when it finds none.
func PDFPages(b []byte) int {
	pages := len(pdfPage.FindAllIndex(b, -1))
	inStreams := 0
	budget := maxInflate
	for _, loc := range pdfObjStm.FindAllIndex(b, -1) {
		if budget <= 0 {
			break
		}
		zr, err := zlib.NewReader(bytes.NewReader(b[loc[1]:]))
		if err != nil {
			continue
		}
		var buf bytes.Buffer
		n, _ := io.Copy(&buf, io.LimitReader(zr, int64(budget)))
		_ = zr.Close()
		budget -= int(n)
		inStreams += len(pdfPage.FindAllIndex(buf.Bytes(), -1))
		if c := maxCount(buf.Bytes()); c > pages {
			pages = c
		}
	}
	if inStreams > pages {
		pages = inStreams
	}
	if c := maxCount(b); c > pages {
		pages = c
	}
	return pages
}

func maxCount(b []byte) int {
	best := 0
	for _, m := range pdfCount.FindAllSubmatch(b, -1) {
		for _, g := range m[1:] {
			if n, err := strconv.Atoi(string(g)); err == nil && n > best {
				best = n
			}
		}
	}
	return best
}
