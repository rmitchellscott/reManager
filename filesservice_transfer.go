package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pkg/sftp"
	"github.com/wailsapp/wails/v3/pkg/application"
	"golang.org/x/crypto/ssh"

	apperrors "reManager/internal/errors"
	"reManager/internal/storage"
)

const (
	eventTransfersSnapshot = "transfers:snapshot"
	eventTransfersFinished = "transfers:finished"
	eventFileBrowserError  = "filebrowser:error"
)

func init() {
	application.RegisterEvent[TransferSnapshot](eventTransfersSnapshot)
	application.RegisterEvent[TransferFinished](eventTransfersFinished)
	application.RegisterEvent[FileBrowserError](eventFileBrowserError)
}

type TransferFinished struct {
	ID         string        `json:"id"`
	Kind       TransferKind  `json:"kind"`
	RemotePath string        `json:"remotePath"`
	State      TransferState `json:"state"`
}

// A transfer refused before it was queued, so it has no row of its own.
type FileBrowserError struct {
	Message string              `json:"message"`
	Code    apperrors.ErrorCode `json:"code,omitempty"`
}

type TransferKind string

const (
	TransferUpload   TransferKind = "upload"
	TransferDownload TransferKind = "download"
)

type TransferState string

const (
	TransferQueued   TransferState = "queued"
	TransferStarting TransferState = "starting"
	TransferRunning  TransferState = "running"
	TransferDone     TransferState = "done"
	TransferFailed   TransferState = "failed"
	TransferCanceled TransferState = "canceled"
)

const (
	transferWorkerCount = 3
	maxTransferChannels = 8

	// spareChannelBudget excludes the per-worker base channels so a starting transfer never waits on a spare.
	spareChannelBudget = maxTransferChannels - transferWorkerCount

	// perTransferSpareCap stops one long transfer holding every spare, which would leave later ones single-channel.
	perTransferSpareCap = 3

	splitThresholdBytes = 32 << 20
	transferChunkSize   = 32 * 1024
	progressInterval    = 100 * time.Millisecond
)

type transferJob struct {
	id         string
	kind       TransferKind
	name       string
	localPath  string
	remotePath string
	isDir      bool
	// The transfer runs only on the connection it was queued on, so a queue left from one reMarkable never reaches another.
	connGen uint64

	ctx    context.Context
	cancel context.CancelFunc

	work *backgroundWork

	mu          sync.Mutex
	state       TransferState
	bytesDone   int64
	bytesTotal  int64
	filesDone   int
	filesTotal  int
	errMessage  string
	failedFiles []string
}

func (j *transferJob) addBytes(n int64) {
	j.mu.Lock()
	j.bytesDone += n
	if j.state == TransferStarting {
		j.state = TransferRunning
	}
	fraction := unknownProgress
	if j.bytesTotal > 0 {
		fraction = float64(j.bytesDone) / float64(j.bytesTotal)
	}
	j.mu.Unlock()
	if j.work != nil {
		j.work.Progress(fraction, j.name)
	}
}

func (j *transferJob) completeFile() {
	j.mu.Lock()
	j.filesDone++
	if j.state == TransferStarting {
		j.state = TransferRunning
	}
	j.mu.Unlock()
}

func (j *transferJob) addFailure(pathName string) {
	j.mu.Lock()
	j.failedFiles = append(j.failedFiles, pathName)
	j.mu.Unlock()
}

func (j *transferJob) setTotals(files int, bytes int64) {
	j.mu.Lock()
	j.filesTotal = files
	j.bytesTotal = bytes
	j.mu.Unlock()
}

func (j *transferJob) setState(state TransferState, errMessage string) {
	j.mu.Lock()
	j.state = state
	j.errMessage = errMessage
	j.mu.Unlock()
}

func (j *transferJob) view() TransferView {
	j.mu.Lock()
	defer j.mu.Unlock()

	var percentage float64
	if j.bytesTotal > 0 {
		percentage = float64(j.bytesDone) / float64(j.bytesTotal) * 100
	} else if j.state == TransferDone {
		percentage = 100
	}

	return TransferView{
		ID:          j.id,
		Kind:        j.kind,
		Name:        j.name,
		State:       j.state,
		BytesDone:   j.bytesDone,
		BytesTotal:  j.bytesTotal,
		FilesDone:   j.filesDone,
		FilesTotal:  j.filesTotal,
		Percentage:  percentage,
		Error:       j.errMessage,
		FailedFiles: j.failedFiles,
		IsDir:       j.isDir,
	}
}

type TransferView struct {
	ID          string        `json:"id"`
	Kind        TransferKind  `json:"kind"`
	Name        string        `json:"name"`
	State       TransferState `json:"state"`
	BytesDone   int64         `json:"bytesDone"`
	BytesTotal  int64         `json:"bytesTotal"`
	FilesDone   int           `json:"filesDone"`
	FilesTotal  int           `json:"filesTotal"`
	Percentage  float64       `json:"percentage"`
	Error       string        `json:"error,omitempty"`
	FailedFiles []string      `json:"failedFiles,omitempty"`
	IsDir       bool          `json:"isDir"`
}

type TransferSnapshot struct {
	Transfers        []TransferView `json:"transfers"`
	ActiveCount      int            `json:"activeCount"`
	BytesDone        int64          `json:"bytesDone"`
	BytesTotal       int64          `json:"bytesTotal"`
	Percentage       float64        `json:"percentage"`
	BytesPerSecond   float64        `json:"bytesPerSecond"`
	SecondsRemaining float64        `json:"secondsRemaining"`
}

type channelPool struct {
	slots chan struct{}
}

func newChannelPool(size int) *channelPool {
	return &channelPool{slots: make(chan struct{}, size)}
}

func (p *channelPool) tryAcquire() bool {
	select {
	case p.slots <- struct{}{}:
		return true
	default:
		return false
	}
}

func (p *channelPool) release() {
	<-p.slots
}

type transferManager struct {
	files *FilesService

	mu    sync.Mutex
	jobs  map[string]*transferJob
	order []string

	queue     chan *transferJob
	startOnce sync.Once
	dirty     atomic.Bool
	nextID    atomic.Uint64
	channels  *channelPool

	rateMu    sync.Mutex
	lastBytes int64
	lastTime  time.Time
	rate      float64
}

func (s *FilesService) transferManager() *transferManager {
	s.transferOnce.Do(func() {
		s.transfers = &transferManager{
			files:    s,
			jobs:     make(map[string]*transferJob),
			queue:    make(chan *transferJob, 1024),
			channels: newChannelPool(spareChannelBudget),
		}
	})
	return s.transfers
}

func (m *transferManager) start() {
	m.startOnce.Do(func() {
		for i := 0; i < transferWorkerCount; i++ {
			go m.worker()
		}
		go m.emitLoop()
	})
}

func (m *transferManager) enqueue(kind TransferKind, name, localPath, remotePath string, isDir bool) string {
	m.start()

	ctx, cancel := context.WithCancel(context.Background())
	job := &transferJob{
		id:         fmt.Sprintf("t%d", m.nextID.Add(1)),
		kind:       kind,
		name:       name,
		localPath:  localPath,
		remotePath: remotePath,
		isDir:      isDir,
		connGen:    m.files.connGen.Load(),
		ctx:        ctx,
		cancel:     cancel,
		state:      TransferQueued,
	}

	m.mu.Lock()
	m.jobs[job.id] = job
	m.order = append(m.order, job.id)
	m.mu.Unlock()

	m.markDirty()
	m.queue <- job
	return job.id
}

func transferWorkTitle(kind TransferKind) string {
	if kind == TransferUpload {
		return "Uploading to your reMarkable"
	}
	return "Downloading from your reMarkable"
}

func (m *transferManager) markDirty() {
	m.dirty.Store(true)
}

func (m *transferManager) worker() {
	for job := range m.queue {
		if job.ctx.Err() != nil {
			job.setState(TransferCanceled, "")
			m.markDirty()
			continue
		}

		job.setState(TransferStarting, "")
		m.markDirty()

		job.work = m.files.beginBackgroundWork(transferWorkTitle(job.kind))
		job.work.Progress(unknownProgress, job.name)
		err := m.run(job)
		if err != nil && job.ctx.Err() == nil {
			job.work.Fail(err.Error())
		}
		job.work.End()

		switch {
		case job.ctx.Err() != nil:
			job.setState(TransferCanceled, "")
			removeStagedFolderDownload(job)
		case err != nil:
			job.setState(TransferFailed, err.Error())
			removeStagedFolderDownload(job)
		default:
			job.setState(TransferDone, "")
			// A download completes in the queue rather than in DownloadFile, so the hand-off is raised here. iOS surfaces the sandbox Documents directory in the Files app, so it has no folder hand-off.
			if job.kind == TransferDownload && job.isDir {
				shareFolder(filepath.Join(job.localPath, path.Base(job.remotePath)))
			} else if job.kind == TransferDownload {
				shareFile(job.localPath)
			}
		}
		removeStagedFolderUpload(job)

		m.markDirty()
		m.emitSnapshot()

		m.files.wails.Event.Emit(eventTransfersFinished, TransferFinished{
			ID:         job.id,
			Kind:       job.kind,
			RemotePath: job.remotePath,
			State:      job.state,
		})
	}
}

func (m *transferManager) run(job *transferJob) error {
	if m.files.connGen.Load() != job.connGen {
		return fmt.Errorf("Your reMarkable disconnected before this transfer started. Start it again.")
	}
	client := m.files.getClient()
	if client == nil {
		return fmt.Errorf("not connected")
	}

	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		ue := apperrors.Classify(err)
		return fmt.Errorf("%s", ue.Message)
	}
	defer sftpClient.Close()

	if job.kind == TransferUpload {
		return m.runUpload(job, client, sftpClient)
	}
	return m.runDownload(job, sftpClient)
}

func (m *transferManager) runUpload(job *transferJob, client *ssh.Client, sftpClient *sftp.Client) error {
	if isSystemPath(job.remotePath) {
		if err := m.files.makeFilesystemWritable(client); err != nil {
			return fmt.Errorf("failed to prepare filesystem: %w", err)
		}
		defer m.files.restoreFilesystemDeferred(client)
	}

	if job.isDir {
		filesTotal, bytesTotal, err := m.files.countLocalFolder(job.localPath)
		if err != nil {
			return fmt.Errorf("failed to scan folder: %w", err)
		}
		job.setTotals(filesTotal, bytesTotal)
		m.markDirty()

		destPath := path.Join(job.remotePath, filepath.Base(job.localPath))
		return m.uploadTree(job, client, sftpClient, job.localPath, destPath)
	}

	info, err := os.Stat(job.localPath)
	if err != nil {
		return err
	}
	job.setTotals(1, info.Size())
	m.markDirty()

	destPath := path.Join(job.remotePath, info.Name())
	if err := m.uploadFile(job, client, sftpClient, job.localPath, destPath, info.Size()); err != nil {
		return err
	}
	job.completeFile()
	return nil
}

type treeEntry struct {
	localPath  string
	remotePath string
	size       int64
	isSymlink  bool
}

func (m *transferManager) collectTree(job *transferJob, sftpClient *sftp.Client, localPath, remotePath string, entries *[]treeEntry) error {
	if err := job.ctx.Err(); err != nil {
		return err
	}

	info, err := os.Lstat(localPath)
	if err != nil {
		return err
	}

	if info.Mode()&os.ModeSymlink != 0 {
		*entries = append(*entries, treeEntry{localPath: localPath, remotePath: remotePath, isSymlink: true})
		return nil
	}

	if !info.IsDir() {
		*entries = append(*entries, treeEntry{localPath: localPath, remotePath: remotePath, size: info.Size()})
		return nil
	}

	if err := sftpClient.MkdirAll(remotePath); err != nil {
		return err
	}

	children, err := os.ReadDir(localPath)
	if err != nil {
		return err
	}
	for _, child := range children {
		childLocal := filepath.Join(localPath, child.Name())
		childRemote := path.Join(remotePath, child.Name())
		if err := m.collectTree(job, sftpClient, childLocal, childRemote, entries); err != nil {
			return err
		}
	}
	return nil
}

func (m *transferManager) uploadEntry(job *transferJob, client *ssh.Client, sftpClient *sftp.Client, entry treeEntry) {
	if entry.isSymlink {
		target, err := os.Readlink(entry.localPath)
		if err != nil {
			job.addFailure(entry.localPath)
			return
		}
		if err := sftpClient.Symlink(target, entry.remotePath); err != nil {
			job.addFailure(entry.localPath)
		}
		job.completeFile()
		m.markDirty()
		return
	}

	if err := m.uploadFile(job, client, sftpClient, entry.localPath, entry.remotePath, entry.size); err != nil {
		if job.ctx.Err() == nil {
			job.addFailure(entry.localPath)
		}
		return
	}
	job.completeFile()
	m.markDirty()
}

// uploadTree sends the files of a folder concurrently, which overlaps the per-file round trips that dominate a tree of small files.
func (m *transferManager) uploadTree(job *transferJob, client *ssh.Client, sftpClient *sftp.Client, localPath, remotePath string) error {
	var entries []treeEntry
	if err := m.collectTree(job, sftpClient, localPath, remotePath, &entries); err != nil {
		return err
	}

	extraClients := make([]*sftp.Client, 0, perTransferSpareCap)
	defer func() {
		for _, extra := range extraClients {
			extra.Close()
			m.channels.release()
		}
	}()

	for len(extraClients) < perTransferSpareCap && len(extraClients)+1 < len(entries) {
		if !m.channels.tryAcquire() {
			break
		}
		extra, err := sftp.NewClient(client)
		if err != nil {
			m.channels.release()
			break
		}
		extraClients = append(extraClients, extra)
	}

	queue := make(chan treeEntry)
	var wg sync.WaitGroup

	for index := 0; index <= len(extraClients); index++ {
		worker := sftpClient
		if index > 0 {
			worker = extraClients[index-1]
		}
		wg.Add(1)
		go func(worker *sftp.Client) {
			defer wg.Done()
			for entry := range queue {
				if job.ctx.Err() != nil {
					return
				}
				m.uploadEntry(job, client, worker, entry)
			}
		}(worker)
	}

	for _, entry := range entries {
		if job.ctx.Err() != nil {
			break
		}
		queue <- entry
	}
	close(queue)
	wg.Wait()

	return job.ctx.Err()
}

func (m *transferManager) uploadFile(job *transferJob, client *ssh.Client, sftpClient *sftp.Client, localPath, destPath string, size int64) error {
	if size >= splitThresholdBytes {
		job.mu.Lock()
		bytesBeforeSplit := job.bytesDone
		job.mu.Unlock()

		err := m.uploadFileSplit(job, client, sftpClient, localPath, destPath, size)
		if err == nil {
			return nil
		}
		if job.ctx.Err() != nil {
			return err
		}

		job.mu.Lock()
		job.bytesDone = bytesBeforeSplit
		job.mu.Unlock()
		m.markDirty()
	}
	return m.uploadFileSequential(job, sftpClient, localPath, destPath, size)
}

func (m *transferManager) uploadFileSequential(job *transferJob, sftpClient *sftp.Client, localPath, destPath string, size int64) error {
	localFile, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer localFile.Close()

	writer, err := openRemoteFileForReplace(sftpClient, destPath)
	if err != nil {
		return err
	}

	buffer := make([]byte, transferChunkSize)
	for {
		if err := job.ctx.Err(); err != nil {
			writer.Abort()
			return err
		}

		n, readErr := localFile.Read(buffer)
		if n > 0 {
			if _, writeErr := writer.Write(buffer[:n]); writeErr != nil {
				writer.Abort()
				return writeErr
			}
			job.addBytes(int64(n))
			m.markDirty()
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			writer.Abort()
			return readErr
		}
	}

	return writer.Commit(size)
}

// uploadFileSplit writes byte ranges over separate SSH channels, each of which carries its own flow-control window.
func (m *transferManager) uploadFileSplit(job *transferJob, client *ssh.Client, sftpClient *sftp.Client, localPath, destPath string, size int64) error {
	desired := m.channelsForLink(sftpClient)
	if desired > perTransferSpareCap+1 {
		desired = perTransferSpareCap + 1
	}
	if desired < 2 {
		return fmt.Errorf("splitting not worthwhile")
	}

	writer, err := openRemoteFileForReplace(sftpClient, destPath)
	if err != nil {
		return err
	}

	extraClients := make([]*sftp.Client, 0, desired-1)
	defer func() {
		for _, extra := range extraClients {
			extra.Close()
			m.channels.release()
		}
	}()

	for len(extraClients) < desired-1 {
		if !m.channels.tryAcquire() {
			break
		}
		extra, err := sftp.NewClient(client)
		if err != nil {
			m.channels.release()
			break
		}
		extraClients = append(extraClients, extra)
	}

	channels := len(extraClients) + 1
	if channels < 2 {
		writer.Abort()
		return fmt.Errorf("no spare channels")
	}

	rangeSize := (size + int64(channels) - 1) / int64(channels)
	var wg sync.WaitGroup
	errs := make([]error, channels)

	for index := 0; index < channels; index++ {
		offset := int64(index) * rangeSize
		length := rangeSize
		if offset+length > size {
			length = size - offset
		}
		if length <= 0 {
			continue
		}

		wg.Add(1)
		go func(index int, offset, length int64) {
			defer wg.Done()

			localFile, err := os.Open(localPath)
			if err != nil {
				errs[index] = err
				return
			}
			defer localFile.Close()

			writeAt := writer.WriteAt
			if index > 0 {
				remoteFile, err := extraClients[index-1].OpenFile(writer.TempPath(), os.O_WRONLY)
				if err != nil {
					errs[index] = err
					return
				}
				defer remoteFile.Close()
				writeAt = remoteFile.WriteAt
			}

			buffer := make([]byte, transferChunkSize)
			for written := int64(0); written < length; {
				if err := job.ctx.Err(); err != nil {
					errs[index] = err
					return
				}

				readSize := int64(len(buffer))
				if length-written < readSize {
					readSize = length - written
				}
				n, readErr := localFile.ReadAt(buffer[:readSize], offset+written)
				if n > 0 {
					if _, writeErr := writeAt(buffer[:n], offset+written); writeErr != nil {
						errs[index] = writeErr
						return
					}
					written += int64(n)
					job.addBytes(int64(n))
					m.markDirty()
				}
				if readErr != nil && written < length {
					errs[index] = readErr
					return
				}
			}
		}(index, offset, length)
	}

	wg.Wait()

	for _, err := range errs {
		if err != nil {
			writer.Abort()
			return err
		}
	}

	return writer.Commit(size)
}

func (m *transferManager) channelsForLink(sftpClient *sftp.Client) int {
	roundTrip := m.files.measureRoundTrip(sftpClient)
	switch {
	case roundTrip <= time.Millisecond:
		return 4
	case roundTrip <= 5*time.Millisecond:
		return 6
	default:
		return maxTransferChannels
	}
}

func (m *transferManager) runDownload(job *transferJob, sftpClient *sftp.Client) error {
	if job.isDir {
		filesTotal, bytesTotal, err := m.files.countRemoteFolder(sftpClient, job.remotePath)
		if err != nil {
			return fmt.Errorf("failed to scan folder: %w", err)
		}
		job.setTotals(filesTotal, bytesTotal)
		m.markDirty()

		localBase := filepath.Join(job.localPath, path.Base(job.remotePath))
		return m.downloadTree(job, sftpClient, job.remotePath, localBase)
	}

	remoteFile, err := sftpClient.Open(job.remotePath)
	if err != nil {
		ue := apperrors.Classify(err)
		return fmt.Errorf("%s", ue.Message)
	}
	defer remoteFile.Close()

	info, err := remoteFile.Stat()
	if err != nil {
		return err
	}
	job.setTotals(1, info.Size())
	m.markDirty()

	if err := m.writeRemoteFile(job, remoteFile, job.localPath); err != nil {
		return err
	}
	job.completeFile()
	return nil
}

func (m *transferManager) downloadTree(job *transferJob, sftpClient *sftp.Client, remotePath, localPath string) error {
	if err := job.ctx.Err(); err != nil {
		return err
	}

	info, err := sftpClient.Lstat(remotePath)
	if err != nil {
		return err
	}

	if info.Mode()&os.ModeSymlink != 0 {
		target, err := sftpClient.ReadLink(remotePath)
		if err != nil {
			job.addFailure(remotePath)
			return nil
		}
		if err := os.Symlink(target, localPath); err != nil {
			job.addFailure(remotePath)
		}
		job.completeFile()
		m.markDirty()
		return nil
	}

	if info.IsDir() {
		if err := os.MkdirAll(localPath, 0755); err != nil {
			return err
		}
		entries, err := sftpClient.ReadDir(remotePath)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			remoteChild := path.Join(remotePath, entry.Name())
			localChild := filepath.Join(localPath, entry.Name())
			if err := m.downloadTree(job, sftpClient, remoteChild, localChild); err != nil {
				return err
			}
		}
		return nil
	}

	remoteFile, err := sftpClient.Open(remotePath)
	if err != nil {
		job.addFailure(remotePath)
		return nil
	}
	defer remoteFile.Close()

	if err := m.writeRemoteFile(job, remoteFile, localPath); err != nil {
		if job.ctx.Err() != nil {
			return err
		}
		job.addFailure(remotePath)
		return nil
	}
	job.completeFile()
	m.markDirty()
	return nil
}

// writeRemoteFile uses WriteTo for concurrent ranged reads; a Read loop costs one round trip per buffer.
func (m *transferManager) writeRemoteFile(job *transferJob, remoteFile *sftp.File, localPath string) error {
	localFile, err := os.Create(localPath)
	if err != nil {
		return err
	}
	defer localFile.Close()

	counter := &progressWriter{job: job, manager: m, target: localFile}
	if _, err := remoteFile.WriteTo(counter); err != nil {
		localFile.Close()
		_ = os.Remove(localPath)
		return err
	}
	return nil
}

type progressWriter struct {
	job     *transferJob
	manager *transferManager
	target  io.Writer
}

func (w *progressWriter) Write(p []byte) (int, error) {
	if err := w.job.ctx.Err(); err != nil {
		return 0, err
	}
	n, err := w.target.Write(p)
	if n > 0 {
		w.job.addBytes(int64(n))
		w.manager.markDirty()
	}
	return n, err
}

func (m *transferManager) snapshot() TransferSnapshot {
	m.mu.Lock()
	views := make([]TransferView, 0, len(m.order))
	for _, id := range m.order {
		if job, ok := m.jobs[id]; ok {
			views = append(views, job.view())
		}
	}
	m.mu.Unlock()

	snapshot := TransferSnapshot{Transfers: views}
	for _, view := range views {
		if view.State == TransferQueued || view.State == TransferStarting || view.State == TransferRunning {
			snapshot.ActiveCount++
			snapshot.BytesTotal += view.BytesTotal
			snapshot.BytesDone += view.BytesDone
		}
	}
	if snapshot.BytesTotal > 0 {
		snapshot.Percentage = float64(snapshot.BytesDone) / float64(snapshot.BytesTotal) * 100
	}

	snapshot.BytesPerSecond = m.sampleRate(snapshot.BytesDone, snapshot.ActiveCount)
	if snapshot.BytesPerSecond > 0 {
		remaining := snapshot.BytesTotal - snapshot.BytesDone
		if remaining > 0 {
			snapshot.SecondsRemaining = float64(remaining) / snapshot.BytesPerSecond
		}
	}
	return snapshot
}

func (m *transferManager) sampleRate(bytesDone int64, activeCount int) float64 {
	m.rateMu.Lock()
	defer m.rateMu.Unlock()

	now := time.Now()
	if activeCount == 0 {
		m.lastBytes = 0
		m.lastTime = time.Time{}
		m.rate = 0
		return 0
	}
	if m.lastTime.IsZero() {
		m.lastBytes = bytesDone
		m.lastTime = now
		return 0
	}

	elapsed := now.Sub(m.lastTime).Seconds()
	if elapsed < 0.5 {
		return m.rate
	}

	delta := bytesDone - m.lastBytes
	m.lastBytes = bytesDone
	m.lastTime = now
	if delta < 0 {
		return m.rate
	}

	sample := float64(delta) / elapsed
	if m.rate == 0 {
		m.rate = sample
	} else {
		m.rate = m.rate*0.7 + sample*0.3
	}
	return m.rate
}

func (m *transferManager) emitSnapshot() {
	if m.files.ctx == nil {
		return
	}
	m.files.wails.Event.Emit(eventTransfersSnapshot, m.snapshot())
}

func (m *transferManager) emitLoop() {
	ticker := time.NewTicker(progressInterval)
	defer ticker.Stop()

	for range ticker.C {
		if m.dirty.Swap(false) {
			m.emitSnapshot()
		}
	}
}

func (s *FilesService) measureRoundTrip(sftpClient *sftp.Client) time.Duration {
	s.roundTripMu.Lock()
	defer s.roundTripMu.Unlock()

	generation := s.connGen.Load()
	if s.roundTripGen == generation && s.roundTrip > 0 {
		return s.roundTrip
	}

	const samples = 5
	start := time.Now()
	for i := 0; i < samples; i++ {
		if _, err := sftpClient.Stat("/home/root"); err != nil {
			return 5 * time.Millisecond
		}
	}

	s.roundTrip = time.Since(start) / samples
	s.roundTripGen = generation
	return s.roundTrip
}

func (s *FilesService) CancelTransfer(id string) {
	m := s.transferManager()
	m.mu.Lock()
	job, ok := m.jobs[id]
	m.mu.Unlock()
	if !ok {
		return
	}
	job.cancel()
	m.markDirty()
	m.emitSnapshot()
}

func (s *FilesService) CancelAllTransfers() {
	m := s.transferManager()
	m.mu.Lock()
	jobs := make([]*transferJob, 0, len(m.jobs))
	for _, job := range m.jobs {
		jobs = append(jobs, job)
	}
	m.mu.Unlock()

	for _, job := range jobs {
		job.cancel()
	}
	m.markDirty()
	m.emitSnapshot()
}

func (s *FilesService) ClearCompletedTransfers() {
	m := s.transferManager()
	m.mu.Lock()
	remaining := make([]string, 0, len(m.order))
	for _, id := range m.order {
		job, ok := m.jobs[id]
		if !ok {
			continue
		}
		job.mu.Lock()
		state := job.state
		job.mu.Unlock()
		if state == TransferDone || state == TransferFailed || state == TransferCanceled {
			delete(m.jobs, id)
			continue
		}
		remaining = append(remaining, id)
	}
	m.order = remaining
	m.mu.Unlock()

	m.markDirty()
	m.emitSnapshot()
}

func (s *FilesService) GetTransfers() TransferSnapshot {
	return s.transferManager().snapshot()
}

func (s *FilesService) UploadFilesFromPaths(localPaths []string, remotePath string) {
	if s.rejectDemoTransfer() {
		return
	}
	if s.getClient() == nil {
		s.wails.Event.Emit(eventFileBrowserError, FileBrowserError{
			Message: "Not connected to your reMarkable.",
			Code:    apperrors.ErrHostDown,
		})
		return
	}

	manager := s.transferManager()
	for _, localPath := range localPaths {
		info, err := os.Stat(localPath)
		if err != nil {
			continue
		}
		manager.enqueue(TransferUpload, info.Name(), localPath, remotePath, info.IsDir())
	}
}

func (s *FilesService) UploadFolder(remotePath string) {
	if s.rejectDemoTransfer() {
		return
	}
	if s.getClient() == nil {
		s.wails.Event.Emit(eventFileBrowserError, FileBrowserError{
			Message: "Not connected to your reMarkable.",
			Code:    apperrors.ErrHostDown,
		})
		return
	}

	if stagesFolderTransfers() {
		s.pickFolderToUpload(func(localDir string) {
			s.transferManager().enqueue(TransferUpload, filepath.Base(localDir), localDir, remotePath, true)
		})
		return
	}

	go func() {
		localDir, err := openDirectoryDialog("Select folder to upload", "")
		if err != nil || localDir == "" {
			return
		}
		s.transferManager().enqueue(TransferUpload, filepath.Base(localDir), localDir, remotePath, true)
	}()
}

func (s *FilesService) DownloadFile(remotePath string) {
	if s.rejectDemoTransfer() {
		return
	}
	if s.getClient() == nil {
		s.wails.Event.Emit(eventFileBrowserError, FileBrowserError{
			Message: "Not connected to your reMarkable.",
			Code:    apperrors.ErrHostDown,
		})
		return
	}

	go func() {
		filename := path.Base(remotePath)
		localPath, err := saveFileDialog("Save file", filename, s.lastDownloadDir())
		if err != nil || localPath == "" {
			return
		}
		s.rememberDownloadDir(filepath.Dir(localPath))
		s.transferManager().enqueue(TransferDownload, filename, localPath, remotePath, false)
	}()
}

func (s *FilesService) DownloadFolder(remotePath string) {
	if s.rejectDemoTransfer() {
		return
	}
	if s.getClient() == nil {
		s.wails.Event.Emit(eventFileBrowserError, FileBrowserError{
			Message: "Not connected to your reMarkable.",
			Code:    apperrors.ErrHostDown,
		})
		return
	}

	go func() {
		if stagesFolderTransfers() {
			stagingDir, err := os.MkdirTemp("", "folder-download-")
			if err != nil {
				s.wails.Event.Emit(eventFileBrowserError, FileBrowserError{
					Message: fmt.Sprintf("Could not prepare the download: %v", err),
				})
				return
			}
			s.transferManager().enqueue(TransferDownload, path.Base(remotePath), stagingDir, remotePath, true)
			return
		}
		localDir, err := openDirectoryDialog("Save folder to", s.lastDownloadDir())
		if err != nil || localDir == "" {
			return
		}
		s.rememberDownloadDir(localDir)
		s.transferManager().enqueue(TransferDownload, path.Base(remotePath), localDir, remotePath, true)
	}()
}

func (s *FilesService) lastDownloadDir() string {
	if s.settingsStore == nil {
		return ""
	}
	settings, err := s.settingsStore.Load()
	if err != nil {
		return ""
	}
	return settings.LastDownloadDir
}

func (s *FilesService) rememberDownloadDir(dir string) {
	if s.settingsStore == nil {
		return
	}
	_ = s.settingsStore.Update(func(settings *storage.Settings) {
		settings.LastDownloadDir = dir
	})
}
