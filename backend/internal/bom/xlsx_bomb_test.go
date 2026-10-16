package bom

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

func TestExcelUnzipBombIsRefused(t *testing.T) {
	f := excelize.NewFile()
	var good bytes.Buffer
	if err := f.Write(&good); err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(good.Bytes()), int64(good.Len()))
	if err != nil {
		t.Fatal(err)
	}
	var bomb bytes.Buffer
	zw := zip.NewWriter(&bomb)
	for _, zf := range zr.File { // keep a valid workbook
		w, _ := zw.Create(zf.Name)
		rc, _ := zf.Open()
		var b bytes.Buffer
		b.ReadFrom(rc)
		rc.Close()
		w.Write(b.Bytes())
	}
	zeros := make([]byte, 1<<20)
	for i := 0; i < 40; i++ { // 40 × 15 MB of zeros, ~600 KB compressed
		w, _ := zw.Create(fmt.Sprintf("xl/media/pad%d.bin", i))
		for j := 0; j < 15; j++ {
			w.Write(zeros)
		}
	}
	zw.Close()
	var m0, m1 runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m0)
	_, err = ReadRows(bytes.NewReader(bomb.Bytes()))
	runtime.ReadMemStats(&m1)
	if err == nil || !errors.Is(err, ErrInvalid) {
		t.Errorf("bomb accepted: %v", err)
	}
	if grew := int64(m1.TotalAlloc - m0.TotalAlloc); grew > 200<<20 {
		t.Errorf("reading the bomb allocated %d MB", grew>>20)
	}
}

func TestExcelWideAndLongSheetsAreBounded(t *testing.T) {
	f := excelize.NewFile()
	f.SetCellValue("Sheet1", "A1", "name")
	f.SetCellValue("Sheet1", "B1", "country")
	f.SetCellValue("Sheet1", "XFD2", "x") // one cell in the last column
	var b bytes.Buffer
	f.Write(&b)
	rows, err := ReadRows(bytes.NewReader(b.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if len(r) > maxSheetCols {
			t.Errorf("row has %d columns", len(r))
		}
	}
	long := excelize.NewFile()
	sw, _ := long.NewStreamWriter("Sheet1")
	for i := 1; i <= maxSheetRows+5; i++ {
		cell, _ := excelize.CoordinatesToCellName(1, i)
		sw.SetRow(cell, []any{"x"})
	}
	sw.Flush()
	var lb bytes.Buffer
	long.Write(&lb)
	if _, err := ReadRows(bytes.NewReader(lb.Bytes())); err == nil || !strings.Contains(err.Error(), "more than") {
		t.Errorf("long sheet = %v", err)
	}
}
