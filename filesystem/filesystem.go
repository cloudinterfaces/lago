package filesystem

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

var badguys = map[string]struct{}{
	".go":      {},
	".c":       {},
	".h":       {},
	".cc":      {},
	".cpp":     {},
	".cxx":     {},
	".hh":      {},
	".hpp":     {},
	".hxx":     {},
	".m":       {},
	".s":       {},
	".S":       {},
	".swig":    {},
	".swigcxx": {},
	".syso":    {},
}

func zipfile(zw *zip.Writer, root, base string, allfiles bool, f *os.File, stat os.FileInfo) error {
	if !stat.Mode().IsRegular() {
		return fmt.Errorf("Not a regular file: %s", root)
	}
	if !allfiles {
		if _, ok := badguys[filepath.Ext(root)]; ok {
			return fmt.Errorf("All files not specified: %s", root)
		}
	}
	zh, err := zip.FileInfoHeader(stat)
	if err != nil {
		return err
	}
	zh.Name = path.Join(base, zh.Name)
	w, err := zw.CreateHeader(zh)
	if err != nil {
		return err
	}
	_, err = io.Copy(w, f)
	return err
}

// Zip writes root (or the contents thereof) to zw
// at base.
func Zip(zw *zip.Writer, root, base string, allfiles bool) error {
	f, err := os.Open(root)
	if err != nil {
		return err
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil {
		return err
	}
	if !stat.IsDir() {
		return zipfile(zw, root, base, allfiles, f, stat)
	}
	stats, err := f.Readdir(-1)
	if err != nil {
		return err
	}
	for _, stat = range stats {
		if stat.IsDir() || !stat.Mode().IsRegular() {
			continue
		}
		if !allfiles {
			if _, ok := badguys[filepath.Ext(stat.Name())]; ok {
				continue
			}
		}
		f, err = os.Open(filepath.Join(root, stat.Name()))
		if err != nil {
			return err
		}
		defer f.Close()
		zh, err := zip.FileInfoHeader(stat)
		if err != nil {
			return err
		}
		zh.Name = path.Join(base, zh.Name)
		w, err := zw.CreateHeader(zh)
		if err != nil {
			return err
		}
		if _, err = io.Copy(w, f); err != nil {
			return err
		}
		if err = f.Close(); err != nil {
			return err
		}
	}
	return nil
}

// ZipWalk writes the contents of root to zw at base recursively.
func ZipWalk(zw *zip.Writer, root, base string, allfiles bool) error {
	return filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if p == root {
			return nil
		}
		if info.IsDir() || !info.Mode().IsRegular() {
			return nil
		}
		if !allfiles {
			if _, ok := badguys[filepath.Ext(p)]; ok {
				return nil
			}
		}
		f, err := os.Open(p)
		if err != nil {
			return err
		}
		defer f.Close()
		zh, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		p = strings.Replace(p, root, "", 1)
		zh.Name = path.Join(base, filepath.ToSlash(p))
		w, err := zw.CreateHeader(zh)
		if err != nil {
			return err
		}
		_, err = io.Copy(w, f)
		return err
	})
}
