package main

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pkg/sftp"
	rmdevice "github.com/rmitchellscott/remarkable-go/device"
	"github.com/wailsapp/wails/v3/pkg/application"
	"golang.org/x/crypto/ssh"

	"reManager/internal/debug"
	"reManager/internal/logger"
	"reManager/internal/storage"
	"reManager/internal/vellum"
)

// Every service embeds the core, and Wails binds the exported methods a
// service gains through embedding, so the core has none: an exported method
// here would be bound once per service, and a ServiceStartup would run once per
// service.
type core struct {
	ctx             context.Context
	wails           *application.App
	demo            bool
	demoScreenshots bool
	demoSession     uint64
	demoPackages    map[string]string
	demoAutoUpdates bool
	demoTimezone    string
	demoFailure     string
	client          *ssh.Client
	mu              sync.Mutex
	operationsMu    sync.Mutex
	operations      map[string]*Operation
	operationSink   operationEmitter
	deviceStore     *storage.DeviceStore
	settingsStore   *storage.SettingsStore
	favoritesStore  *storage.FavoritesStore
	followsStore    *storage.FollowsStore
	deviceInfoCache *storage.DeviceInfoCacheStore
	vellumClient    *vellum.Client
	metadata        *vellum.MetadataStore

	logger       *logger.Logger
	operationLog *logger.CommandLog

	keepaliveTrigger    chan struct{}
	connectedDeviceID   string
	connectedDeviceType rmdevice.Type
	connectedDeviceArch rmdevice.Architecture
	connectedFirmware   string
	connectedMachineID  string
	writableRootBusy    bool
	connGen             atomic.Uint64
	connectState        connectState
	sessionMu           sync.Mutex
	// Separate from mu so the connection monitor can read install state while a
	// command that holds mu is stuck on a dead link.
	installMu          sync.Mutex
	installActive      bool
	installSession     *installSession
	backupActive       bool
	backupMu           sync.Mutex
	backgroundWorkMu   sync.Mutex
	backgroundWorks    []*backgroundWork
	backgroundReported backgroundWorkReport
	// Separate from mu, which a stuck command may hold, so a stop can always
	// reach the vellum commands streaming on the reMarkable.
	streamingMu       sync.Mutex
	streamingSessions map[*ssh.Session]int

	writableRootCount  int
	writableRootClient *ssh.Client
	writableRootMu     sync.Mutex
}

func (c *core) getClient() *ssh.Client {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.client
}

func (c *core) getVellumClient() *vellum.Client {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.vellumClient
}

// A newer connection may have replaced the client since vc was read, and that
// one must be left alone.
func (c *core) clearVellumClient(vc *vellum.Client) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.vellumClient == vc {
		c.vellumClient = nil
	}
}

func (c *core) getSFTPClient() (*sftp.Client, error) {
	if c.inDemoMode() {
		return nil, errDemoUnavailable
	}
	client := c.getClient()
	if client == nil {
		return nil, fmt.Errorf("not connected")
	}
	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		return nil, fmt.Errorf("failed to create SFTP client: %w", err)
	}
	return sftpClient, nil
}

func (c *core) startup(ctx context.Context) {
	c.ctx = ctx

	configDir, err := storage.GetConfigDir()
	if err == nil {
		l, lerr := logger.New(configDir)
		if lerr == nil {
			c.logger = l
			debug.SetFileLogger(l)
			c.logger.LogEvent("APP", "reManager starting, version="+version)
			go c.logger.CleanupOldCommandLogs(30 * 24 * time.Hour)
		}
	}

	store, err := storage.NewDeviceStore()
	if err != nil {
		fmt.Printf("Warning: could not initialize device store: %v\n", err)
	}
	c.deviceStore = store

	settingsStore, err := storage.NewSettingsStore()
	if err != nil {
		fmt.Printf("Warning: could not initialize settings store: %v\n", err)
	}
	c.settingsStore = settingsStore

	favoritesStore, err := storage.NewFavoritesStore()
	if err != nil {
		fmt.Printf("Warning: could not initialize favorites store: %v\n", err)
	}
	c.favoritesStore = favoritesStore

	followsStore, err := storage.NewFollowsStore()
	if err != nil {
		fmt.Printf("Warning: could not initialize follows store: %v\n", err)
	}
	c.followsStore = followsStore

	deviceInfoCache, err := storage.NewDeviceInfoCacheStore()
	if err != nil {
		fmt.Printf("Warning: could not initialize device info cache: %v\n", err)
	}
	c.deviceInfoCache = deviceInfoCache

	c.startSync(ctx)

	c.metadata = vellum.NewMetadataStore()
	go func() {
		if err := c.metadata.Load(); err != nil {
			debug.Printf("[DEBUG] Failed to load metadata: %v\n", err)
			c.wails.Event.Emit(eventMetadataError, err.Error())
			return
		}
		c.wails.Event.Emit(eventMetadataLoaded)
	}()
}
