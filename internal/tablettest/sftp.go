package tablettest

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"sync"
	"time"

	"github.com/pkg/sftp"
)

type FileInfo struct {
	Name    string      `json:"name"`
	Size    int64       `json:"size"`
	Mode    fs.FileMode `json:"mode"`
	ModTime int64       `json:"modTime"`
}

type FileAnswer struct {
	Op       string     `json:"op"`
	Path     string     `json:"path"`
	Content  []byte     `json:"content,omitempty"`
	Infos    []FileInfo `json:"infos,omitempty"`
	Target   string     `json:"target,omitempty"`
	NotExist bool       `json:"notExist,omitempty"`
	Error    string     `json:"error,omitempty"`
}

func (a *FileAnswer) err() error {
	switch {
	case a.NotExist:
		return os.ErrNotExist
	case a.Error != "":
		return errors.New(a.Error)
	}
	return nil
}

func (c *Cassette) addFile(answer *FileAnswer) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Files = append(c.Files, answer)
}

func answerFor(op, path string, err error) *FileAnswer {
	answer := &FileAnswer{Op: op, Path: path}
	switch {
	case errors.Is(err, fs.ErrNotExist):
		answer.NotExist = true
	case err != nil:
		answer.Error = err.Error()
	}
	return answer
}

func infoOf(info fs.FileInfo) FileInfo {
	return FileInfo{info.Name(), info.Size(), info.Mode(), info.ModTime().Unix()}
}

type replayedInfo struct{ info FileInfo }

func (i replayedInfo) Name() string       { return i.info.Name }
func (i replayedInfo) Size() int64        { return i.info.Size }
func (i replayedInfo) Mode() fs.FileMode  { return i.info.Mode }
func (i replayedInfo) ModTime() time.Time { return time.Unix(i.info.ModTime, 0) }
func (i replayedInfo) IsDir() bool        { return i.info.Mode.IsDir() }
func (i replayedInfo) Sys() any           { return nil }

type listing []fs.FileInfo

func (l listing) ListAt(dst []fs.FileInfo, offset int64) (int, error) {
	if offset >= int64(len(l)) {
		return 0, io.EOF
	}
	n := copy(dst, l[offset:])
	if offset+int64(n) >= int64(len(l)) {
		return n, io.EOF
	}
	return n, nil
}

type recordingFiles struct {
	client   *sftp.Client
	cassette *Cassette
}

func (f *recordingFiles) handlers() sftp.Handlers {
	return sftp.Handlers{FileGet: f, FilePut: f, FileCmd: f, FileList: f}
}

func (f *recordingFiles) Fileread(r *sftp.Request) (io.ReaderAt, error) {
	file, err := f.client.Open(r.Filepath)
	if err != nil {
		f.cassette.addFile(answerFor("read", r.Filepath, err))
		return nil, err
	}
	defer file.Close()
	content, err := io.ReadAll(file)
	answer := answerFor("read", r.Filepath, err)
	answer.Content = content
	f.cassette.addFile(answer)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(content), nil
}

func (f *recordingFiles) Filewrite(r *sftp.Request) (io.WriterAt, error) {
	return f.client.OpenFile(r.Filepath, openFlags(r.Pflags()))
}

func openFlags(p sftp.FileOpenFlags) int {
	flags := os.O_RDONLY
	switch {
	case p.Read && p.Write:
		flags = os.O_RDWR
	case p.Write:
		flags = os.O_WRONLY
	}
	if p.Append {
		flags |= os.O_APPEND
	}
	if p.Creat {
		flags |= os.O_CREATE
	}
	if p.Trunc {
		flags |= os.O_TRUNC
	}
	if p.Excl {
		flags |= os.O_EXCL
	}
	return flags
}

func (f *recordingFiles) Filecmd(r *sftp.Request) error {
	switch r.Method {
	case "Setstat":
		attrs, flags := r.Attributes(), r.AttrFlags()
		if flags.Permissions {
			if err := f.client.Chmod(r.Filepath, attrs.FileMode()); err != nil {
				return err
			}
		}
		if flags.Acmodtime {
			if err := f.client.Chtimes(r.Filepath, attrs.AccessTime(), attrs.ModTime()); err != nil {
				return err
			}
		}
		if flags.Size {
			return f.client.Truncate(r.Filepath, int64(attrs.Size))
		}
		return nil
	case "Rename":
		return f.client.PosixRename(r.Filepath, r.Target)
	case "Rmdir":
		return f.client.RemoveDirectory(r.Filepath)
	case "Remove":
		return f.client.Remove(r.Filepath)
	case "Mkdir":
		return f.client.Mkdir(r.Filepath)
	case "Symlink":
		return f.client.Symlink(r.Filepath, r.Target)
	case "Link":
		return f.client.Link(r.Filepath, r.Target)
	}
	return sftp.ErrSSHFxOpUnsupported
}

func (f *recordingFiles) Filelist(r *sftp.Request) (sftp.ListerAt, error) {
	switch r.Method {
	case "List":
		infos, err := f.client.ReadDir(r.Filepath)
		answer := answerFor("list", r.Filepath, err)
		for _, info := range infos {
			answer.Infos = append(answer.Infos, infoOf(info))
		}
		f.cassette.addFile(answer)
		return listing(infos), err
	case "Stat":
		return f.stat("stat", r.Filepath, f.client.Stat)
	case "Readlink":
		target, err := f.client.ReadLink(r.Filepath)
		answer := answerFor("readlink", r.Filepath, err)
		answer.Target = target
		f.cassette.addFile(answer)
		if err != nil {
			return nil, err
		}
		return listing{replayedInfo{FileInfo{Name: target}}}, nil
	}
	return nil, sftp.ErrSSHFxOpUnsupported
}

func (f *recordingFiles) Lstat(r *sftp.Request) (sftp.ListerAt, error) {
	return f.stat("lstat", r.Filepath, f.client.Lstat)
}

func (f *recordingFiles) stat(op, path string, stat func(string) (fs.FileInfo, error)) (sftp.ListerAt, error) {
	info, err := stat(path)
	answer := answerFor(op, path, err)
	if err != nil {
		f.cassette.addFile(answer)
		return nil, err
	}
	answer.Infos = []FileInfo{infoOf(info)}
	f.cassette.addFile(answer)
	return listing{info}, nil
}

func (f *recordingFiles) RealPath(p string) (string, error) {
	resolved, err := f.client.RealPath(p)
	answer := answerFor("realpath", p, err)
	answer.Target = resolved
	f.cassette.addFile(answer)
	return resolved, err
}

// Writes land in an overlay that later reads see first, so a test that uploads a file can read it back.
type replayedFiles struct {
	mu      sync.Mutex
	answers map[string][]*FileAnswer
	asked   map[string]bool
	written map[string]*bytes.Buffer
	onMiss  func(command string)
}

func newReplayedFiles(cassette *Cassette, onMiss func(string)) *replayedFiles {
	f := &replayedFiles{answers: map[string][]*FileAnswer{}, asked: map[string]bool{}, written: map[string]*bytes.Buffer{}, onMiss: onMiss}
	for _, answer := range cassette.Files {
		key := answer.Op + " " + normalize(answer.Path)
		f.answers[key] = append(f.answers[key], answer)
	}
	return f
}

func fileKey(op, path string) string {
	return op + " " + normalize(path)
}

func (f *replayedFiles) unasked() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	keys := unaskedKeys(f.answers, f.asked)
	for i, key := range keys {
		keys[i] = "sftp " + key
	}
	return keys
}

func (f *replayedFiles) handlers() sftp.Handlers {
	return sftp.Handlers{FileGet: f, FilePut: f, FileCmd: f, FileList: f}
}

func (f *replayedFiles) next(op, path string) *FileAnswer {
	f.mu.Lock()
	defer f.mu.Unlock()
	key := fileKey(op, path)
	queue := f.answers[key]
	if len(queue) == 0 {
		f.onMiss("sftp " + key)
		return &FileAnswer{Op: op, Path: path, NotExist: true}
	}
	f.asked[key] = true
	if len(queue) > 1 {
		f.answers[key] = queue[1:]
	}
	return queue[0]
}

// A recording reads back a written file from the tablet, so an overlay hit stands in for that recorded answer.
func (f *replayedFiles) writtenFile(op, path string) ([]byte, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if buf, ok := f.written[path]; ok {
		f.asked[fileKey(op, path)] = true
		return bytes.Clone(buf.Bytes()), true
	}
	return nil, false
}

func (f *replayedFiles) Fileread(r *sftp.Request) (io.ReaderAt, error) {
	if content, ok := f.writtenFile("read", r.Filepath); ok {
		return bytes.NewReader(content), nil
	}
	answer := f.next("read", r.Filepath)
	if err := answer.err(); err != nil {
		return nil, err
	}
	return bytes.NewReader(answer.Content), nil
}

type overlayWriter struct {
	mu  *sync.Mutex
	buf *bytes.Buffer
}

func (w overlayWriter) WriteAt(p []byte, offset int64) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if grow := int(offset) + len(p) - w.buf.Len(); grow > 0 {
		w.buf.Write(make([]byte, grow))
	}
	copy(w.buf.Bytes()[offset:], p)
	return len(p), nil
}

func (f *replayedFiles) Filewrite(r *sftp.Request) (io.WriterAt, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	buf, ok := f.written[r.Filepath]
	if !ok || r.Pflags().Trunc {
		buf = &bytes.Buffer{}
		f.written[r.Filepath] = buf
	}
	return overlayWriter{&f.mu, buf}, nil
}

func (f *replayedFiles) Filecmd(r *sftp.Request) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	switch r.Method {
	case "Rename":
		if buf, ok := f.written[r.Filepath]; ok {
			f.written[r.Target] = buf
			delete(f.written, r.Filepath)
		}
	case "Remove":
		delete(f.written, r.Filepath)
	}
	return nil
}

func (f *replayedFiles) Filelist(r *sftp.Request) (sftp.ListerAt, error) {
	switch r.Method {
	case "List":
		answer := f.next("list", r.Filepath)
		var infos listing
		for _, info := range answer.Infos {
			infos = append(infos, replayedInfo{info})
		}
		return infos, answer.err()
	case "Stat":
		return f.stat("stat", r.Filepath)
	case "Readlink":
		answer := f.next("readlink", r.Filepath)
		if err := answer.err(); err != nil {
			return nil, err
		}
		return listing{replayedInfo{FileInfo{Name: answer.Target}}}, nil
	}
	return nil, sftp.ErrSSHFxOpUnsupported
}

func (f *replayedFiles) Lstat(r *sftp.Request) (sftp.ListerAt, error) {
	return f.stat("lstat", r.Filepath)
}

func (f *replayedFiles) stat(op, filePath string) (sftp.ListerAt, error) {
	if content, ok := f.writtenFile(op, filePath); ok {
		return listing{replayedInfo{FileInfo{Name: path.Base(filePath), Size: int64(len(content)), Mode: 0o644}}}, nil
	}
	answer := f.next(op, filePath)
	if err := answer.err(); err != nil {
		return nil, err
	}
	return listing{replayedInfo{answer.Infos[0]}}, nil
}

func (f *replayedFiles) RealPath(p string) (string, error) {
	answer := f.next("realpath", p)
	return answer.Target, answer.err()
}
