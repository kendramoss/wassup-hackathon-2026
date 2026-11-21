package httpapi

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/extract"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/media"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/planner"
)

// MediaExtractor drafts a product from checked images and scanned PDFs
// (extract.Extractor).
type MediaExtractor interface {
	ExtractMedia(ctx context.Context, files []extract.File, name string, tm *planner.Template) (extract.Draft, error)
}

// maxMediaRequest is the whole multipart body: the files' cap plus room for
// boundaries, headers and the two short fields.
const maxMediaRequest = media.MaxTotalBytes + 64<<10

// maxMediaField caps the name and family fields.
const maxMediaField = 512

// errMediaTooLarge and errMediaType map to 413 and 415.
var (
	errMediaTooLarge = errors.New("invalid input")
	errMediaType     = errors.New("invalid input")
)

// extractMedia drafts an editable product from one to four images or scanned
// PDFs (POST /v1/products/extract-media, multipart/form-data: "file" one to
// four times, optional "name" and "family"). It is a separate endpoint from
// /v1/products/extract, which takes JSON text, so each has one content type,
// one size limit and one schema.
//
// The files are read into memory (never to a temp file), checked by their
// bytes (media.Check), sent to the model once and dropped: they are not
// stored, cached or logged, and the draft carries only their type, size and
// page count. Nothing is predicted: the user reviews the draft first.
func (a *api) extractMedia(w http.ResponseWriter, r *http.Request) {
	if a.d.MediaExtractor == nil {
		writeError(w, http.StatusServiceUnavailable, "internal", "media extraction is not configured", nil)
		return
	}
	files, name, family, err := readMedia(w, r)
	defer func() {
		for i := range files {
			clear(files[i].Data)
		}
	}()
	switch {
	case errors.Is(err, errMediaTooLarge):
		writeError(w, http.StatusRequestEntityTooLarge, "invalid_input", err.Error(), nil)
		return
	case errors.Is(err, errMediaType):
		writeError(w, http.StatusUnsupportedMediaType, "invalid_input", err.Error(), nil)
		return
	case err != nil:
		a.fail(w, r, err)
		return
	}
	var tm *planner.Template
	if family != "" {
		t, ok := a.d.Templates[family]
		if !ok {
			a.fail(w, r, fmt.Errorf("%w: family %q has no template; known: %s", domain.ErrInvalid, family, a.families()))
			return
		}
		tm = &t
	}
	d, err := a.d.MediaExtractor.ExtractMedia(r.Context(), files, name, tm)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

// readMedia reads and checks the multipart body. Error messages name a file
// by its position only, never by its name or content.
func readMedia(w http.ResponseWriter, r *http.Request) (files []extract.File, name, family string, err error) {
	if !isMultipart(r) {
		return nil, "", "", fmt.Errorf("%w: send multipart/form-data with one to %d \"file\" fields", errMediaType, media.MaxFiles)
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxMediaRequest)
	mr, err := r.MultipartReader()
	if err != nil {
		return nil, "", "", fmt.Errorf("%w: upload: %v", domain.ErrInvalid, err)
	}
	total := 0
	tooLarge := fmt.Errorf("%w: the files are larger than %d MB together", errMediaTooLarge, media.MaxTotalBytes>>20)
	for {
		part, perr := mr.NextPart()
		if perr == io.EOF {
			break
		}
		if perr != nil {
			var mbe *http.MaxBytesError
			if errors.As(perr, &mbe) {
				return files, "", "", tooLarge
			}
			return files, "", "", fmt.Errorf("%w: upload: %v", domain.ErrInvalid, perr)
		}
		switch field := part.FormName(); field {
		case "file":
			if len(files) == media.MaxFiles {
				return files, "", "", fmt.Errorf("%w: send at most %d files", domain.ErrInvalid, media.MaxFiles)
			}
			b, rerr := io.ReadAll(io.LimitReader(part, int64(media.MaxTotalBytes-total)+1))
			var mbe *http.MaxBytesError
			if errors.As(rerr, &mbe) || total+len(b) > media.MaxTotalBytes {
				return files, "", "", tooLarge
			}
			if rerr != nil {
				return files, "", "", fmt.Errorf("%w: upload: %v", domain.ErrInvalid, rerr)
			}
			total += len(b)
			files = append(files, extract.File{Data: b})
		case "name", "family":
			b, rerr := io.ReadAll(io.LimitReader(part, maxMediaField+1))
			if rerr != nil || len(b) > maxMediaField {
				return files, "", "", fmt.Errorf("%w: %s must be at most %d bytes", domain.ErrInvalid, field, maxMediaField)
			}
			if field == "name" {
				name = strings.TrimSpace(string(b))
			} else {
				family = strings.TrimSpace(string(b))
			}
		default:
			return files, "", "", fmt.Errorf("%w: unknown field %q; send file, name and family only", domain.ErrInvalid, truncateField(field))
		}
	}
	if len(files) == 0 {
		return nil, "", "", fmt.Errorf("%w: a \"file\" field with an image or a scanned PDF is required", domain.ErrInvalid)
	}
	pages := 0
	// One budget for the whole upload: PDF stream decoding and parse time
	// are bounded across all files together (#504).
	budget := media.NewBudget()
	for i := range files {
		info, cerr := budget.Check(files[i].Data)
		switch {
		case errors.Is(cerr, media.ErrUnsupported):
			return files, "", "", fmt.Errorf("%w: file %d: %v", errMediaType, i+1, cerr)
		case cerr != nil:
			return files, "", "", fmt.Errorf("%w: file %d: %v", domain.ErrInvalid, i+1, cerr)
		}
		files[i].Info = info
		pages += info.Pages
	}
	if pages > media.MaxPDFPages {
		return files, "", "", fmt.Errorf("%w: the PDFs have %d pages together; the limit is %d. Send only the pages with the parts list", domain.ErrInvalid, pages, media.MaxPDFPages)
	}
	return files, name, family, nil
}

func truncateField(s string) string {
	if len(s) > 40 {
		return s[:40]
	}
	return s
}
