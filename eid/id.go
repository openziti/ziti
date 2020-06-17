package eid

import "github.com/teris-io/shortid"

func New() string {
	id, _ := shortid.GetDefault().Generate()

	return id
}
