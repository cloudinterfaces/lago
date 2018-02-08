package filesystem

import (
	"archive/zip"
	"bytes"
	"testing"
)

func TestZip(t *testing.T) {
	var statics = map[string]interface{}{
		"static/test1.txt": nil,
		"static/test2.txt": nil,
	}

	buf := new(bytes.Buffer)
	zf := zip.NewWriter(buf)
	err := Zip(zf, "_static/", "static", true)
	if err != nil {
		t.Fatal(err)
	}
	if err = zf.Close(); err != nil {
		t.Fatal(err)
	}
	r := bytes.NewReader(buf.Bytes())
	zr, err := zip.NewReader(r, int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}
	if l := len(zr.File); l != 2 {
		t.Fatalf("Want 2 files, got %v", l)
	}
	for _, f := range zr.File {
		if _, ok := statics[f.Name]; !ok {
			t.Fatalf("Filename %s not in statics", f.Name)
		}
	}
}

func TestRoundTrip(t *testing.T) {
	var statics = map[string]interface{}{
		"html/index.html": nil,
		"test1.txt":       nil,
		"test2.txt":       nil,
	}
	buf := new(bytes.Buffer)
	zf := zip.NewWriter(buf)
	if err := ZipWalk(zf, "_static/", "", true); err != nil {
		t.Fatal(err)
	}
	if err := zf.Close(); err != nil {
		t.Fatal(err)
	}
	r := bytes.NewReader(buf.Bytes())
	zr, err := zip.NewReader(r, int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}
	if l := len(zr.File); l != 3 {
		t.Fatalf("Want 3 files got %v", l)
	}
	for _, f := range zr.File {
		if _, ok := statics[f.Name]; !ok {
			t.Fatalf("Have %s not in statics", f.Name)
		}
	}
}
