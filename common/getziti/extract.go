package getziti

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func Unzip(src, dest string, filter func(path string) (string, bool)) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer func() {
		if err := r.Close(); err != nil {
			panic(err)
		}
	}()

	// Closure to address file descriptors issue with all the deferred .Close() methods
	extractAndWriteFile := func(f *zip.File) error {
		rc, err := f.Open()
		if err != nil {
			return err
		}
		defer func() {
			if err := rc.Close(); err != nil {
				panic(err)
			}
		}()

		path := filepath.Join(dest, f.Name)

		// Check for ZipSlip (Directory traversal)
		if !strings.HasPrefix(path, filepath.Clean(dest)+string(os.PathSeparator)) {
			return fmt.Errorf("illegal file path: %s", path)
		}

		if !f.FileInfo().IsDir() {
			fileDest, accept := filter(f.Name)
			if !accept {
				return nil
			}
			fullPath := filepath.Join(dest, fileDest)

			return CopyReaderToFile(rc, fullPath, f.Mode())
		}
		return nil
	}

	for _, f := range r.File {
		if err = extractAndWriteFile(f); err != nil {
			return err
		}
	}

	return nil
}

func UnTarGz(src, dest string, f func(path string) (string, bool)) error {
	fr, err := os.Open(src)
	if err != nil {
		return err
	}

	defer func() {
		if err = fr.Close(); err != nil {
			panic(err)
		}
	}()

	gzr, err := gzip.NewReader(fr)
	if err != nil {
		return err
	}

	r := tar.NewReader(gzr)

	for {
		header, err := r.Next()
		if err == io.EOF {
			return nil
		}

		if err != nil {
			return err
		}

		if header == nil {
			continue
		}

		if header.Typeflag == tar.TypeReg {
			fileDest, accept := f(header.Name)
			if !accept {
				continue
			}
			fullPath := filepath.Join(dest, fileDest)

			if err = CopyReaderToFile(r, fullPath, os.FileMode(header.Mode)); err != nil {
				return err
			}
		}
	}
}

func CopyReaderToFile(in io.Reader, dst string, mode os.FileMode) error {
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_RDWR, mode)
	if err != nil {
		return err
	}

	defer func() {
		if e := out.Close(); e != nil {
			pfxlog.Logger().WithError(err).Errorf("failed to copy dest file (%s)", dst)
		}
	}()

	_, err = io.Copy(out, in)
	if err != nil {
		return err
	}

	return out.Sync()
}
