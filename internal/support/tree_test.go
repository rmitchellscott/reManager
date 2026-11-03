package support

import (
	"errors"
	"io/fs"
	"os"
	"reflect"
	"testing"
	"time"
)

type fakeFileInfo struct {
	name string
	mode fs.FileMode
	size int64
}

func (f fakeFileInfo) Name() string       { return f.name }
func (f fakeFileInfo) Size() int64        { return f.size }
func (f fakeFileInfo) Mode() fs.FileMode  { return f.mode }
func (f fakeFileInfo) ModTime() time.Time { return time.Time{} }
func (f fakeFileInfo) IsDir() bool        { return f.mode.IsDir() }
func (f fakeFileInfo) Sys() any           { return nil }

type fakeRemoteDir struct {
	dirs  map[string][]os.FileInfo
	links map[string]string
}

func (f fakeRemoteDir) ReadDir(dirPath string) ([]os.FileInfo, error) {
	entries, ok := f.dirs[dirPath]
	if !ok {
		return nil, errors.New("permission denied")
	}
	return entries, nil
}

func (f fakeRemoteDir) ReadLink(linkPath string) (string, error) {
	target, ok := f.links[linkPath]
	if !ok {
		return "", errors.New("not a link")
	}
	return target, nil
}

func file(name string, size int64) os.FileInfo { return fakeFileInfo{name: name, size: size} }
func dir(name string) os.FileInfo              { return fakeFileInfo{name: name, mode: fs.ModeDir} }
func symlink(name string) os.FileInfo          { return fakeFileInfo{name: name, mode: fs.ModeSymlink} }

func TestBuildDirTree(t *testing.T) {
	remote := fakeRemoteDir{
		dirs: map[string][]os.FileInfo{
			"/home/root/xovi":                               {file("start", 120), dir("extensions.d"), dir("exthome"), dir("locked")},
			"/home/root/xovi/extensions.d":                  {symlink("appload.so"), file("qt-resource-rebuilder.so", 4096)},
			"/home/root/xovi/exthome":                       {dir("qt-resource-rebuilder")},
			"/home/root/xovi/exthome/qt-resource-rebuilder": {file("hashtab", 0)},
		},
		links: map[string]string{
			"/home/root/xovi/extensions.d/appload.so": "/home/root/.vellum/appload.so",
		},
	}

	want := DirTreeNode{
		Name: "/home/root/xovi",
		Type: DirTreeDirectory,
		Children: []DirTreeNode{
			{Name: "extensions.d", Type: DirTreeDirectory, Children: []DirTreeNode{
				{Name: "appload.so", Type: DirTreeSymlink, Target: "/home/root/.vellum/appload.so"},
				{Name: "qt-resource-rebuilder.so", Type: DirTreeFile, Size: 4096},
			}},
			{Name: "exthome", Type: DirTreeDirectory, Children: []DirTreeNode{
				{Name: "qt-resource-rebuilder", Type: DirTreeDirectory, Children: []DirTreeNode{
					{Name: "hashtab", Type: DirTreeFile},
				}},
			}},
			{Name: "locked", Type: DirTreeDirectory, Error: "permission denied"},
			{Name: "start", Type: DirTreeFile, Size: 120},
		},
	}

	if got := BuildDirTree(remote, "/home/root/xovi"); !reflect.DeepEqual(got, want) {
		t.Errorf("BuildDirTree mismatch\ngot:  %+v\nwant: %+v", got, want)
	}
}

func TestBuildDirTreeMissingRoot(t *testing.T) {
	want := DirTreeNode{Name: "/home/root/xovi", Type: DirTreeDirectory, Error: "permission denied"}
	if got := BuildDirTree(fakeRemoteDir{}, "/home/root/xovi"); !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}
