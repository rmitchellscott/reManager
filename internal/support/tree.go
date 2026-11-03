package support

import (
	"os"
	"path"
	"sort"
)

const XoviDir = "/home/root/xovi"

type RemoteDirReader interface {
	ReadDir(dirPath string) ([]os.FileInfo, error)
	ReadLink(linkPath string) (string, error)
}

type DirTreeNodeType string

const (
	DirTreeDirectory DirTreeNodeType = "directory"
	DirTreeFile      DirTreeNodeType = "file"
	DirTreeSymlink   DirTreeNodeType = "symlink"
)

type DirTreeNode struct {
	Name     string          `json:"name"`
	Type     DirTreeNodeType `json:"type"`
	Size     int64           `json:"size,omitempty"`
	Target   string          `json:"target,omitempty"`
	Error    string          `json:"error,omitempty"`
	Children []DirTreeNode   `json:"children,omitempty"`
}

func BuildDirTree(reader RemoteDirReader, rootPath string) DirTreeNode {
	root := DirTreeNode{Name: rootPath, Type: DirTreeDirectory}
	fillDirChildren(reader, rootPath, &root)
	return root
}

func fillDirChildren(reader RemoteDirReader, dirPath string, node *DirTreeNode) {
	entries, err := reader.ReadDir(dirPath)
	if err != nil {
		node.Error = err.Error()
		return
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	for _, entry := range entries {
		entryPath := path.Join(dirPath, entry.Name())
		child := DirTreeNode{Name: entry.Name()}

		switch {
		case entry.Mode()&os.ModeSymlink != 0:
			child.Type = DirTreeSymlink
			if target, err := reader.ReadLink(entryPath); err == nil {
				child.Target = target
			} else {
				child.Error = err.Error()
			}
		case entry.IsDir():
			child.Type = DirTreeDirectory
			fillDirChildren(reader, entryPath, &child)
		default:
			child.Type = DirTreeFile
			child.Size = entry.Size()
		}

		node.Children = append(node.Children, child)
	}
}
