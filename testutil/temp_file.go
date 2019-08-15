package testutil

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

type TempFile struct {
	*os.File
}

func MkTempFileWithDir(dir string, t *testing.T) *TempFile {
	file, err := ioutil.TempFile(dir, "test-golang")
	assert.Nil(t, err)
	return &TempFile{file}
}

func MkTempFile(t *testing.T) *TempFile {
	return MkTempFileWithDir("", t)
}

func (f *TempFile) Remove() {
	os.Remove(f.Name())
}

func MkTempDir(t *testing.T) (string, func()) {
	name, err := ioutil.TempDir("", "test-golang")
	assert.NoError(t, err)
	return name, func() { os.RemoveAll(name) }
}

func WithTempDir(t *testing.T, f func(path string)) {
	name, release := MkTempDir(t)
	defer release()
	f(name)
}
