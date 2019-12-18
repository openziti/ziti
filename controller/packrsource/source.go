/*
	Copyright 2019 Netfoundry, Inc.

	Licensed under the Apache License, Version 2.0 (the "License");
	you may not use this file except in compliance with the License.
	You may obtain a copy of the License at

	https://www.apache.org/licenses/LICENSE-2.0

	Unless required by applicable law or agreed to in writing, software
	distributed under the License is distributed on an "AS IS" BASIS,
	WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
	See the License for the specific language governing permissions and
	limitations under the License.
*/

package packrsource

import (
	"bytes"
	"fmt"
	"github.com/gobuffalo/packr"
	"github.com/golang-migrate/migrate/source"
	"io"
	"io/ioutil"
	"net/http"
	nurl "net/url"
	"os"
	"path"
	"path/filepath"
)

type Packr struct {
	url        string
	path       string
	migrations *source.Migrations
	boxes      []*packr.Box
	boxMap     map[string]*packr.Box
}

func NewPakrSource(bs ...*packr.Box) *Packr {
	return &Packr{
		boxes: bs,
	}
}

func (f *Packr) Open(url string) (source.Driver, error) {
	u, err := nurl.Parse(url)
	if err != nil {
		return nil, err
	}

	// concat host and path to restore full path
	// host might be `.`
	p := u.Host + u.Path

	if len(p) == 0 {
		// default to current directory if no path
		wd, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		p = wd

	}

	nf := &Packr{
		url:        url,
		path:       p,
		migrations: source.NewMigrations(),
		boxes:      f.boxes,
		boxMap:     map[string]*packr.Box{},
	}
	for _, box := range f.boxes {
		curBox := box
		err = curBox.WalkPrefix(p, func(s string, f packr.File) error {
			fi, _ := f.FileInfo()
			if !fi.IsDir() {
				fn := filepath.Base(fi.Name())
				nf.boxMap[fn] = curBox
				m, err := source.DefaultParse(fn)

				if err != nil {
					// ignore files that we can't parse
					return nil
				}
				if !nf.migrations.Append(m) {
					return fmt.Errorf("unable to add file, null or possible duplicate: %v", fi.Name())
				}
			}
			return nil
		})

		if err != nil {
			return nil, err
		}
	}

	return nf, nil
}

func (f *Packr) Close() error {
	// nothing do to here
	return nil
}

func (f *Packr) First() (version uint, err error) {
	if v, ok := f.migrations.First(); !ok {
		return 0, &os.PathError{Op: "first", Path: f.path, Err: os.ErrNotExist}
	} else {
		return v, nil
	}
}

func (f *Packr) Prev(version uint) (prevVersion uint, err error) {
	if v, ok := f.migrations.Prev(version); !ok {
		return 0, &os.PathError{Op: fmt.Sprintf("prev for version %v", version), Path: f.path, Err: os.ErrNotExist}
	} else {
		return v, nil
	}
}

func (f *Packr) Next(version uint) (nextVersion uint, err error) {
	if v, ok := f.migrations.Next(version); !ok {
		return 0, &os.PathError{Op: fmt.Sprintf("next for version %v", version), Path: f.path, Err: os.ErrNotExist}
	} else {
		return v, nil
	}
}

func (f *Packr) ReadUp(version uint) (r io.ReadCloser, identifier string, err error) {
	if m, ok := f.migrations.Up(version); ok {
		box := f.boxMap[m.Raw]
		r, err := box.Open(path.Join(f.path, m.Raw))

		if err != nil {
			return nil, "", err
		}

		fb, err := toMemeoryBuff(r)

		if err != nil {
			return nil, "", err
		}
		return fb, m.Identifier, nil
	}
	return nil, "", &os.PathError{Op: fmt.Sprintf("read version %v", version), Path: f.path, Err: os.ErrNotExist}
}

func (f *Packr) ReadDown(version uint) (r io.ReadCloser, identifier string, err error) {
	if m, ok := f.migrations.Down(version); ok {
		box := f.boxMap[m.Raw]
		r, err := box.Open(path.Join(f.path, m.Raw))

		if err != nil {
			return nil, "", err
		}

		fb, err := toMemeoryBuff(r)

		if err != nil {
			return nil, "", err
		}
		return fb, m.Identifier, nil
	}
	return nil, "", &os.PathError{Op: fmt.Sprintf("read version %v", version), Path: f.path, Err: os.ErrNotExist}
}

type nopCloser struct {
	io.Reader
}

func (nopCloser) Close() error { return nil }

//Patch the io.ReadCloser from packr as the migration engine depends
//on a full implementation
func toMemeoryBuff(r http.File) (io.ReadCloser, error) {
	b, err := ioutil.ReadAll(r)

	if err != nil {
		return nil, err
	}

	fb := bytes.NewBuffer(b)

	return nopCloser{fb}, nil
}
