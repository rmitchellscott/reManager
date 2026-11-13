package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"golang.org/x/crypto/ssh"

	"reManager/internal/debug"
	"reManager/internal/deviceid"
	apperrors "reManager/internal/errors"
	"reManager/internal/platform"
	"reManager/internal/sshagent"
	"reManager/internal/sshconfig"
	"reManager/internal/storage"
	"reManager/internal/support"
	"reManager/internal/swupdate"
	"reManager/internal/vellum"
	versionpkg "reManager/internal/version"

	rmdevice "github.com/rmitchellscott/remarkable-go/device"
	rmexecutor "github.com/rmitchellscott/remarkable-go/executor"
	rmfilesystem "github.com/rmitchellscott/remarkable-go/filesystem"
)

type ConnectionService struct {
	*core
	os       *OSService
	shell    *ShellService
	device   *DeviceService
	packages *PackagesService
	files    *FilesService

	connectCancel context.CancelFunc

	keepaliveStop chan struct{}
	reconnectMu   sync.Mutex
	reconnect     *reconnectRun
	// Set when an automatic reconnect used every attempt, so a return to the
	// foreground can tell that failure from a rejected credential.
	reconnectExhausted bool
	currentConn        *connTarget
	liveClient         atomic.Pointer[ssh.Client]
	lastMachineID      string

	agentConn net.Conn
}

func (s *ConnectionService) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	s.wails = application.Get()
	configureMobileStorage()
	s.startup(ctx)
	return nil
}

func (s *ConnectionService) ServiceShutdown() error {
	s.shutdown(context.Background())
	return nil
}

func (s *ConnectionService) shutdown(ctx context.Context) {
	if s.logger != nil {
		s.logger.LogEvent("APP", "reManager shutting down")
	}
	s.Disconnect()
	if s.logger != nil {
		s.logger.Close()
	}
}

type CurrentConnection struct {
	DeviceType string `json:"deviceType"`
	DeviceID   string `json:"deviceId"`
}

// A WebView reload, such as an Android activity recreation, starts the frontend over while the connection here stays up.
func (s *ConnectionService) GetCurrentConnection() *CurrentConnection {
	s.mu.Lock()
	defer s.mu.Unlock()
	if (s.client == nil && !s.demo) || s.connectedDeviceType == "" {
		return nil
	}
	return &CurrentConnection{DeviceType: string(s.connectedDeviceType), DeviceID: s.connectedDeviceID}
}

// Connect-check results, kept per connection so a reloaded frontend gets them again without re-running the checks.
type connectState struct {
	gen           uint64
	warnings      *ConnectWarnings
	vellumChecked bool
	vellumFound   bool
	vellumMissing []string
	guide         *UserGuideStatus
}

func (c *core) recordConnectState(gen uint64, update func(state *connectState)) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.connGen.Load() != gen {
		return false
	}
	if c.connectState.gen != gen {
		c.connectState = connectState{gen: gen}
	}
	update(&c.connectState)
	return true
}

func (c *core) emitConnectWarnings(gen uint64, warnings ConnectWarnings) {
	if c.recordConnectState(gen, func(state *connectState) { state.warnings = &warnings }) {
		c.emitOperationEvent(eventConnectWarnings, warnings)
	}
}

// missing is the core packages a broken install lacks, and is nil for a working one.
func (c *core) emitVellumState(gen uint64, found bool, missing []string) {
	if c.recordVellumState(gen, found, missing) {
		c.emitVellumEvents(found, missing)
	}
}

func (c *core) recordVellumState(gen uint64, found bool, missing []string) bool {
	return c.recordConnectState(gen, func(state *connectState) {
		state.vellumChecked, state.vellumFound, state.vellumMissing = true, found, missing
	})
}

func (c *core) emitVellumEvents(found bool, missing []string) {
	if !found {
		c.emitOperationEvent(eventVellumMissing, nil)
		return
	}
	c.emitOperationEvent(eventVellumReady, nil)
	if missing != nil {
		c.emitOperationEvent(eventVellumBrokenInstall, missing)
	}
}

func (c *core) emitGuideStatus(gen uint64, status UserGuideStatus) {
	if c.recordConnectState(gen, func(state *connectState) { state.guide = &status }) {
		c.emitOperationEvent(eventGuideStatus, status)
	}
}

// Warnings go after the Vellum state, since the frontend treats them as the end of the checks.
func (s *ConnectionService) ResendConnectState() {
	s.mu.Lock()
	state := s.connectState
	current := state.gen == s.connGen.Load() && (s.client != nil || s.demo)
	s.mu.Unlock()
	if !current {
		return
	}
	if state.vellumChecked {
		s.emitVellumEvents(state.vellumFound, state.vellumMissing)
	}
	if state.warnings != nil {
		s.emitOperationEvent(eventConnectWarnings, *state.warnings)
	}
	if state.guide != nil {
		s.emitOperationEvent(eventGuideStatus, *state.guide)
	}
}

type ConnectionResult struct {
	Success     bool                `json:"success"`
	Message     string              `json:"message"`
	Code        apperrors.ErrorCode `json:"code,omitempty"`
	Retryable   bool                `json:"retryable,omitempty"`
	Device      string              `json:"device,omitempty"`
	FreshDevice bool                `json:"freshDevice,omitempty"`
}

const (
	eventConnectionLost           = "connection:lost"
	eventConnectionReconnecting   = "connection:reconnecting"
	eventConnectionRetryScheduled = "connection:retry-scheduled"
	eventConnectionRestored       = "connection:restored"
	eventConnectionFailed         = "connection:failed"
	eventWritableRootBusy         = "writable-root:busy"
	eventConnectWarnings          = "connect:warnings"
	eventGuideStatus              = "guide:status"
	eventVellumMissing            = "vellum:missing"
	eventVellumReady              = "vellum:ready"
	eventVellumBrokenInstall      = "vellum:broken-install"
)

func init() {
	application.RegisterEvent[ConnectionFailure](eventConnectionLost)
	application.RegisterEvent[ReconnectPhase](eventConnectionReconnecting)
	application.RegisterEvent[ReconnectPhase](eventConnectionRetryScheduled)
	application.RegisterEvent[ConnectionRestored](eventConnectionRestored)
	application.RegisterEvent[ConnectionFailure](eventConnectionFailed)
	application.RegisterEvent[bool](eventWritableRootBusy)
	application.RegisterEvent[ConnectWarnings](eventConnectWarnings)
	application.RegisterEvent[UserGuideStatus](eventGuideStatus)
	application.RegisterEvent[application.Void](eventVellumMissing)
	application.RegisterEvent[application.Void](eventVellumReady)
	application.RegisterEvent[[]string](eventVellumBrokenInstall)
}

// DeviceID is empty for a connection that was never saved.
type ConnectionFailure struct {
	Reason   string              `json:"reason"`
	Code     apperrors.ErrorCode `json:"code"`
	DeviceID string              `json:"deviceId"`
}

// A phase is one dial or one wait; ElapsedMs/TotalMs place it on the timeline of every attempt so the countdown shows overall progress.
type ReconnectPhase struct {
	Attempt     int    `json:"attempt"`
	MaxAttempts int    `json:"maxAttempts"`
	DeviceID    string `json:"deviceId"`
	DurationMs  int64  `json:"durationMs"`
	ElapsedMs   int64  `json:"elapsedMs"`
	TotalMs     int64  `json:"totalMs"`
}

type ConnectionRestored struct {
	DeviceID string `json:"deviceId"`
	Device   string `json:"device"`
}

// Facts rather than verdicts: which is a problem depends on what the System tab can fix, so a fact that could not be read is left out rather than sent as a default.
type ConnectWarnings struct {
	ReenableStatus           vellum.ReenableStatus        `json:"reenableStatus,omitempty"`
	Hashtab                  *HashtabVersionStatus        `json:"hashtab,omitempty"`
	UpdateService            *support.UpdateServiceStatus `json:"updateService,omitempty"`
	Xochitl                  *XochitlStatus               `json:"xochitl,omitempty"`
	TimezoneStatus           *TimezoneStatus              `json:"timezoneStatus,omitempty"`
	OSChannel                *OSChannelStatus             `json:"osChannel,omitempty"`
	IncompatibleWithLatestOS []string                     `json:"incompatibleWithLatestOS,omitempty"`
	OSMismatch               *OSVersionChange             `json:"osMismatch,omitempty"`
	CompatibilityStatus      *PackageCompatibilityStatus  `json:"compatibilityStatus,omitempty"`
}

type OSVersionChange struct {
	PrevVersion string `json:"prevVersion"`
	NewVersion  string `json:"newVersion"`
}

type SSHKey struct {
	Path string `json:"path"`
	Name string `json:"name"`
}

type SavedDeviceInfo struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Host          string `json:"host"`
	AuthType      string `json:"authType"`
	KeyPath       string `json:"keyPath,omitempty"`
	KeyID         string `json:"keyId,omitempty"`
	LastConnected int64  `json:"lastConnected,omitempty"`
	Timezone      string `json:"timezone,omitempty"`
}

func (s *ConnectionService) GetDefaultSSHKeys() []SSHKey {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}

	sshDir := filepath.Join(home, ".ssh")
	entries, err := os.ReadDir(sshDir)
	if err != nil {
		return nil
	}

	var keys []SSHKey
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasPrefix(name, "id_") && !strings.HasSuffix(name, ".pub") {
			keyPath := filepath.Join(sshDir, name)
			if info, err := os.Stat(keyPath); err == nil && info.Mode().IsRegular() {
				keys = append(keys, SSHKey{
					Path: keyPath,
					Name: name,
				})
			}
		}
	}

	return keys
}

// A managed key lives in the secret store because the platform gives the user no filesystem to put a key file on.
func (s *ConnectionService) readKeyData(ref storage.KeyRef) ([]byte, error) {
	if ref.Kind != storage.KeyRefManaged {
		return os.ReadFile(ref.Path)
	}
	if s.deviceStore == nil {
		return nil, fmt.Errorf("device store not initialized")
	}
	keyData, err := s.deviceStore.Keys().GetPrivateKey(ref)
	if err != nil {
		return nil, err
	}
	return []byte(keyData), nil
}

func (s *ConnectionService) SelectKeyFile() string {
	path, err := openFileDialog("Select SSH private key", "")
	if err != nil {
		return ""
	}
	return path
}

func (s *ConnectionService) GetSavedDevices() []SavedDeviceInfo {
	if s.deviceStore == nil {
		return []SavedDeviceInfo{}
	}

	devices, err := s.deviceStore.GetAll()
	if err != nil {
		fmt.Printf("Error getting saved devices: %v\n", err)
		return []SavedDeviceInfo{}
	}

	result := make([]SavedDeviceInfo, len(devices))
	for i, d := range devices {
		result[i] = SavedDeviceInfo{
			ID:            d.ID,
			Name:          d.Name,
			Host:          d.Host,
			AuthType:      d.AuthType,
			KeyPath:       d.KeyPath,
			KeyID:         d.KeyID,
			LastConnected: d.LastConnected,
			Timezone:      d.Timezone,
		}
	}
	return result
}

// DeviceOutlook is computed offline from cached state and the package index; it says connecting is worth doing, not what will happen, since the solver and vellum's check-os need the device.
type DeviceOutlook struct {
	DeviceID string `json:"deviceId"`
	Upgrades int    `json:"upgrades"`
	OSUpdate string `json:"osUpdate,omitempty"`
	// Reported rather than used to suppress the update, because a package that blocks permanently is a choice the user must make and hiding the update hides the choice.
	OSBlocked bool  `json:"osBlocked,omitempty"`
	CachedAt  int64 `json:"cachedAt"`
}

func (s *ConnectionService) CachedDeviceCount() int {
	if s.deviceInfoCache == nil {
		return 0
	}
	count, err := s.deviceInfoCache.Count()
	if err != nil {
		return 0
	}
	return count
}

// The connect badges read this cache, so they stay silent until each device is connected again.
func (s *ConnectionService) ClearDeviceInfoCache() error {
	if s.deviceInfoCache == nil {
		return fmt.Errorf("device info cache unavailable")
	}
	return s.deviceInfoCache.Clear()
}

func (s *ConnectionService) GetDeviceOutlooks() []DeviceOutlook {
	if s.deviceStore == nil || s.deviceInfoCache == nil || s.metadata == nil || !s.metadata.Ready() {
		return []DeviceOutlook{}
	}

	devices, err := s.deviceStore.GetAll()
	if err != nil {
		return []DeviceOutlook{}
	}

	// The bucket listing is a paginated walk with no cache of its own, so it is resolved once per device type rather than once per device.
	latestOS := map[string]string{}

	outlooks := []DeviceOutlook{}
	for _, device := range devices {
		cached, err := s.deviceInfoCache.Get(device.ID)
		if err != nil || cached == nil {
			continue
		}
		// Written from the connection rather than derived, so their absence means the device predates the field and cannot be evaluated yet.
		if cached.DeviceType == "" || cached.FirmwareVersion == "" || len(cached.InstalledPackages) == 0 {
			continue
		}

		outlook := DeviceOutlook{
			DeviceID: device.ID,
			CachedAt: cached.CachedAt,
			Upgrades: s.metadata.UpgradesFor(cached.InstalledPackages, cached.DeviceType, cached.FirmwareVersion, cached.Arch),
		}

		osKey := cached.DeviceType + "@" + cached.FirmwareVersion
		target, resolved := latestOS[osKey]
		if !resolved {
			target = latestOSFor(cached.DeviceType, cached.FirmwareVersion)
			latestOS[osKey] = target
		}
		if target != "" {
			outlook.OSUpdate = target
			outlook.OSBlocked = len(s.metadata.PackagesBlockingOS(cached.InstalledPackages, cached.DeviceType, target, cached.Arch)) > 0
		}

		outlooks = append(outlooks, outlook)
	}
	return outlooks
}

// The listing is an HTTP call against the image bucket, so it needs no device.
func latestOSFor(deviceType, firmware string) string {
	versions, err := swupdate.ListVersions(deviceType, []string{firmware})
	if err != nil {
		return ""
	}
	for _, v := range versions {
		if v.IsLatest {
			if versionpkg.Compare(v.Version, firmware) > 0 {
				return v.Version
			}
			return ""
		}
	}
	return ""
}

func (s *ConnectionService) SaveDevice(id, name, host, authType, password, keyPath, keyPassphrase, keyID string) (string, error) {
	if s.deviceStore == nil {
		return "", fmt.Errorf("device store not initialized")
	}

	device := storage.SavedDevice{
		ID:            id,
		Name:          name,
		Host:          host,
		AuthType:      authType,
		KeyPath:       keyPath,
		KeyID:         keyID,
		LastConnected: time.Now().Unix(),
	}

	savedID, err := s.deviceStore.Save(device, password, keyPassphrase)
	if err != nil {
		return "", err
	}
	if id == "" {
		s.adoptSavedDeviceID(host, savedID)
	}
	return savedID, nil
}

func (s *ConnectionService) adoptSavedDeviceID(host, savedID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.client == nil || s.connectedDeviceID != "" || s.currentConn == nil || s.currentConn.host != host {
		return
	}
	s.connectedDeviceID = savedID
	s.currentConn.deviceID = savedID
}

func (s *ConnectionService) DeleteSavedDevice(id string) error {
	if s.deviceStore == nil {
		return fmt.Errorf("device store not initialized")
	}
	if err := s.deviceStore.Delete(id); err != nil {
		return err
	}
	if s.deviceInfoCache != nil {
		_ = s.deviceInfoCache.Delete(id)
	}
	return nil
}

func (s *ConnectionService) ConnectToSavedDevice(id string) ConnectionResult {
	if s.deviceStore == nil {
		return ConnectionResult{
			Success:   false,
			Message:   "Could not load your saved reMarkables. Restart reManager and try again.",
			Code:      apperrors.ErrStorageFailed,
			Retryable: false,
		}
	}

	device, err := s.deviceStore.Get(id)
	if err != nil {
		return ConnectionResult{
			Success:   false,
			Message:   "This saved reMarkable no longer exists. It may have been removed.",
			Code:      apperrors.ErrDeviceNotFound,
			Retryable: false,
		}
	}

	if message := unusableSyncedAuth(device); message != "" {
		return ConnectionResult{
			Success:   false,
			Message:   message,
			Code:      apperrors.ErrKeyNotChosen,
			Retryable: false,
		}
	}

	gen := s.beginUserConnect()

	var result ConnectionResult
	if device.AuthType == "agent" {
		result = s.establishConnection(device.Host, "agent", "", storage.KeyRef{}, id, gen)
	} else if device.AuthType == "key" {
		keyRef := device.KeyRef()
		passphrase, _ := s.deviceStore.GetKeyPassphrase(keyRef, id)
		result = s.establishConnection(device.Host, "key", passphrase, keyRef, id, gen)
	} else {
		password, err := s.deviceStore.GetPassword(id)
		if err != nil {
			return ConnectionResult{
				Success:   false,
				Message:   "Could not retrieve the saved password. Reconnect and save this reMarkable again.",
				Code:      apperrors.ErrKeyringFailed,
				Retryable: false,
			}
		}
		result = s.establishConnection(device.Host, "password", password, storage.KeyRef{}, id, gen)
	}

	if result.Success {
		s.deviceStore.UpdateLastConnected(id, time.Now().Unix())
	}

	return result
}

func (s *ConnectionService) Connect(host, password string) ConnectionResult {
	return s.establishConnection(host, "password", password, storage.KeyRef{}, "", s.beginUserConnect())
}

func (s *ConnectionService) ConnectWithKey(host, keyPath, passphrase string) ConnectionResult {
	return s.establishConnection(host, "key", passphrase, storage.PathKeyRef(keyPath), "", s.beginUserConnect())
}

func (s *ConnectionService) ConnectWithManagedKey(host, keyID, passphrase string) ConnectionResult {
	ref := storage.ManagedKeyRef(keyID)
	if passphrase == "" && s.deviceStore != nil {
		passphrase, _ = s.deviceStore.Keys().GetPassphrase(ref, "")
	}
	return s.establishConnection(host, "key", passphrase, ref, "", s.beginUserConnect())
}

func (s *ConnectionService) ConnectWithAgent(host string) ConnectionResult {
	return s.establishConnection(host, "agent", "", storage.KeyRef{}, "", s.beginUserConnect())
}

func (s *ConnectionService) IsSSHAgentAvailable() bool {
	return sshagent.IsAvailable(s.sshAgentSocketPath())
}

func (s *ConnectionService) sshAgentSocketPath() string {
	if s.settingsStore == nil {
		return ""
	}
	settings, err := s.settingsStore.Load()
	if err != nil {
		return ""
	}
	return settings.SSHAgentSocketPath
}

// Canceling has to end the connection too, or it lives on behind a connect page that believes it was abandoned.
func (s *ConnectionService) CancelConnect() {
	s.connGen.Add(1)
	s.closeLiveClient()
	s.mu.Lock()
	if s.connectCancel != nil {
		s.connectCancel()
		s.connectCancel = nil
	}
	s.mu.Unlock()
	s.Disconnect()
}

// Attempts and waits are measured on one scale, not the waits alone, which would fill the bar before the last attempt had run.
func reconnectTimeline(attemptIdx int, waiting bool) (elapsedMs, totalMs int64) {
	dialMs := reconnectDialTimeout.Milliseconds()
	totalMs = int64(maxReconnectAttempts) * dialMs
	for i := 0; i < maxReconnectAttempts-1 && i < len(reconnectBackoff); i++ {
		totalMs += reconnectBackoff[i].Milliseconds()
	}
	elapsedMs = int64(attemptIdx) * dialMs
	for i := 0; i < attemptIdx && i < len(reconnectBackoff); i++ {
		elapsedMs += reconnectBackoff[i].Milliseconds()
	}
	if waiting {
		elapsedMs += dialMs
	}
	return elapsedMs, totalMs
}

// One automatic reconnect at a time; stopReconnect ends it and wakes it from its backoff wait, so a run that was ended cannot touch the state of one that started after it.
type reconnectRun struct {
	stop chan struct{}
	// Restarts the attempt count from the first attempt, skipping any backoff
	// wait, because the attempts before it ran on a network that was blocked.
	wake chan struct{}
}

func (s *ConnectionService) beginReconnect() *reconnectRun {
	s.reconnectMu.Lock()
	defer s.reconnectMu.Unlock()
	if s.reconnect != nil {
		return nil
	}
	s.reconnectExhausted = false
	s.reconnect = &reconnectRun{stop: make(chan struct{}), wake: make(chan struct{}, 1)}
	return s.reconnect
}

func (s *ConnectionService) wakeReconnect() bool {
	s.reconnectMu.Lock()
	defer s.reconnectMu.Unlock()
	if s.reconnect == nil {
		return false
	}
	select {
	case s.reconnect.wake <- struct{}{}:
	default:
	}
	return true
}

func (s *ConnectionService) takeExhaustedReconnect() bool {
	s.reconnectMu.Lock()
	defer s.reconnectMu.Unlock()
	exhausted := s.reconnectExhausted
	s.reconnectExhausted = false
	return exhausted
}

func (s *ConnectionService) endReconnect(run *reconnectRun) {
	s.reconnectMu.Lock()
	defer s.reconnectMu.Unlock()
	if s.reconnect == run {
		s.reconnect = nil
	}
}

func (s *ConnectionService) stopReconnect() {
	s.reconnectMu.Lock()
	defer s.reconnectMu.Unlock()
	if s.reconnect != nil {
		close(s.reconnect.stop)
		s.reconnect = nil
	}
}

func (s *ConnectionService) reconnectCurrent(run *reconnectRun) bool {
	s.reconnectMu.Lock()
	defer s.reconnectMu.Unlock()
	return s.reconnect == run
}

func (s *ConnectionService) isFastDialMode() bool {
	s.reconnectMu.Lock()
	defer s.reconnectMu.Unlock()
	return s.reconnect != nil
}

// The dialer's timeout covers only the TCP connect, so the handshake gets its own deadline: a peer that accepts TCP and never speaks SSH would otherwise hold the attempt open.
func (s *ConnectionService) dialWithContext(ctx context.Context, addr string, config *ssh.ClientConfig) (*ssh.Client, error) {
	timeout := initialDialTimeout
	if s.isFastDialMode() {
		timeout = reconnectDialTimeout
	}
	conn, err := dialTablet(ctx, addr, timeout)
	if err != nil {
		return nil, err
	}
	stopClosingOnCancel := context.AfterFunc(ctx, func() { conn.Close() })
	conn.SetDeadline(time.Now().Add(timeout))
	c, chans, reqs, err := ssh.NewClientConn(conn, addr, config)
	if !stopClosingOnCancel() {
		if err == nil {
			c.Close()
		}
		return nil, context.Canceled
	}
	if err != nil {
		conn.Close()
		return nil, err
	}
	conn.SetDeadline(time.Time{})
	return ssh.NewClient(c, chans, reqs), nil
}

func isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	errStr := strings.ToLower(err.Error())

	// Transport-level failures are retryable even when the message also contains "ssh: handshake failed", so check these first.
	retryableKeywords := []string{
		"no route to host",
		"connection refused",
		"i/o timeout",
		"connection reset by peer",
		"network is unreachable",
		"host is down",
		"connection timed out",
		"broken pipe",
		"did not properly respond",
		"wsarecv",
		"wsasend",
		"actively refused",
		"unexpected packet in response to channel open",
	}

	for _, keyword := range retryableKeywords {
		if strings.Contains(errStr, keyword) {
			return true
		}
	}

	nonRetryableKeywords := []string{
		"passphrase",
		"permission denied",
		"unable to authenticate",
		"ssh: handshake failed",
		"no auth",
		"authentication failed",
	}

	for _, keyword := range nonRetryableKeywords {
		if strings.Contains(errStr, keyword) {
			return false
		}
	}

	var netErr net.Error
	if errors.As(err, &netErr) {
		if netErr.Timeout() || netErr.Temporary() {
			return true
		}
	}

	return false
}

func (s *ConnectionService) dialWithContextWithRetry(ctx context.Context, addr string, config *ssh.ClientConfig) (*ssh.Client, error) {
	var maxRetries int
	var backoffDurations []time.Duration

	if s.isFastDialMode() {
		maxRetries = 0
		backoffDurations = []time.Duration{}
	} else {
		maxRetries = 3
		backoffDurations = []time.Duration{
			2 * time.Second,
			4 * time.Second,
			8 * time.Second,
		}
	}

	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		debug.Printf("[%s] dialWithContextWithRetry attempt %d/%d to %s\n", time.Now().Format("15:04:05.000"), attempt+1, maxRetries+1, addr)
		client, err := s.dialWithContext(ctx, addr, config)
		if err == nil {
			debug.Printf("[%s] dialWithContextWithRetry attempt %d/%d succeeded\n", time.Now().Format("15:04:05.000"), attempt+1, maxRetries+1)
			return client, nil
		}

		debug.Printf("[%s] dialWithContextWithRetry attempt %d/%d failed: %v\n", time.Now().Format("15:04:05.000"), attempt+1, maxRetries+1, err)
		lastErr = err

		if ctx.Err() == context.Canceled {
			return nil, context.Canceled
		}

		if !isRetryableError(err) {
			debug.Printf("[%s] Error not retryable, giving up\n", time.Now().Format("15:04:05.000"))
			return nil, err
		}

		if attempt == maxRetries {
			break
		}

		backoffDuration := backoffDurations[attempt]
		debug.Printf("[%s] dialWithContextWithRetry waiting %v before retry\n", time.Now().Format("15:04:05.000"), backoffDuration)

		select {
		case <-time.After(backoffDuration):
			continue
		case <-ctx.Done():
			return nil, context.Canceled
		}
	}

	return nil, lastErr
}

// secret is held in memory only so reconnect can redial.
type connTarget struct {
	host     string
	authType string
	secret   string
	keyRef   storage.KeyRef
	deviceID string // "" for a manual/unsaved connection
	gen      uint64
}

// Bumps the generation so a reconnect that completes later can't overwrite this newer connection.
func (s *ConnectionService) beginUserConnect() uint64 {
	s.stopReconnect()
	return s.connGen.Add(1)
}

var canceledConnectionResult = ConnectionResult{
	Success:   false,
	Message:   "Connection canceled.",
	Code:      apperrors.ErrOperationCanceled,
	Retryable: false,
}

func (s *ConnectionService) establishConnection(host, authType, secret string, keyRef storage.KeyRef, deviceID string, gen uint64) ConnectionResult {
	if isDemoCredential(host, authType, secret) {
		s.stopConnectionMonitor()
		return s.startDemo(deviceID, secret == demoScreenshotSecret || secret == demoOSUpdateSecret, secret == demoOSUpdateSecret)
	}
	s.stopDemo()
	s.stopConnectionMonitor()

	s.mu.Lock()

	s.connectedDeviceID = ""

	if s.connectCancel != nil {
		s.connectCancel()
	}

	if s.client != nil {
		s.client.Close()
		s.client = nil
	}
	if s.agentConn != nil {
		s.agentConn.Close()
		s.agentConn = nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	s.connectCancel = cancel

	s.mu.Unlock()

	var authMethods []ssh.AuthMethod

	if authType == "agent" {
		conn, authMethod, err := sshagent.Connect(s.sshAgentSocketPath())
		if err != nil {
			return ConnectionResult{
				Success:   false,
				Message:   err.Error(),
				Code:      apperrors.ErrAuthFailed,
				Retryable: false,
			}
		}
		s.mu.Lock()
		s.agentConn = conn
		s.mu.Unlock()
		authMethods = append(authMethods, authMethod)
	} else if authType == "key" {
		keyData, err := s.readKeyData(keyRef)
		if err != nil {
			ue := apperrors.Classify(err)
			return ConnectionResult{
				Success:   false,
				Message:   ue.Message,
				Code:      ue.Code,
				Retryable: ue.Retryable,
			}
		}

		var signer ssh.Signer
		if secret != "" {
			signer, err = ssh.ParsePrivateKeyWithPassphrase(keyData, []byte(secret))
		} else {
			signer, err = ssh.ParsePrivateKey(keyData)
		}
		if err != nil {
			ue := apperrors.Classify(err)
			return ConnectionResult{
				Success:   false,
				Message:   ue.Message,
				Code:      ue.Code,
				Retryable: ue.Retryable,
			}
		}
		authMethods = append(authMethods, ssh.PublicKeys(signer))
	} else {
		authMethods = append(authMethods, ssh.Password(secret))
	}

	config := sshconfig.NewClientConfig("root", authMethods)

	addr := host
	if !strings.Contains(host, ":") {
		addr = host + ":22"
	}

	client, err := s.dialWithContextWithRetry(ctx, addr, config)
	if err != nil {
		if ctx.Err() == context.Canceled {
			return canceledConnectionResult
		}
		if s.logger != nil {
			s.logger.LogConnection("connect-failed", fmt.Sprintf("%s: %v", addr, err))
		}
		s.logUSBNetworkDiagnostics(addr)
		ue := apperrors.Classify(err)
		return ConnectionResult{
			Success:   false,
			Message:   ue.Message,
			Code:      ue.Code,
			Retryable: ue.Retryable,
		}
	}

	s.mu.Lock()
	if s.connGen.Load() != gen {
		s.mu.Unlock()
		client.Close()
		return ConnectionResult{
			Success:   false,
			Message:   "Connection superseded.",
			Code:      apperrors.ErrOperationCanceled,
			Retryable: false,
		}
	}
	s.client = client
	s.liveClient.Store(client)
	s.connectCancel = nil
	s.connectedDeviceID = deviceID
	s.currentConn = &connTarget{host, authType, secret, keyRef, deviceID, gen}
	debug.Println("[DEBUG] SSH connected, creating vellum client")
	s.vellumClient = vellum.NewClient(&wailsExecutor{core: s.core})
	s.mu.Unlock()

	if s.logger != nil {
		s.logger.LogConnection("connected", host)
	}

	if s.shellEmitsNoise(client) {
		debug.Println("[DEBUG] Device login shell emits output on non-interactive sessions")
		s.mu.Lock()
		s.client = nil
		s.vellumClient = nil
		s.currentConn = nil
		s.mu.Unlock()
		client.Close()
		return ConnectionResult{
			Success:   false,
			Message:   "Your reMarkable's shell prints output on SSH, which breaks file transfers. Guard ~/.bashrc so it stays silent in non-interactive sessions.",
			Code:      apperrors.ErrShellRCNoisy,
			Retryable: false,
		}
	}

	s.startConnectionMonitor(client)
	// Checked after the monitor starts rather than before, so a cancel that lands between the two still stops it.
	if s.connGen.Load() != gen {
		s.stopConnectionMonitor()
		return canceledConnectionResult
	}
	if s.settingsStore != nil {
		if settings, err := s.settingsStore.Load(); err == nil && settings.PreventSleep {
			if err := s.os.startPreventSleep(); err != nil {
				debug.Printf("[DEBUG] establishConnection: startPreventSleep failed: %v\n", err)
			}
		}
	}

	debug.Println("[DEBUG] Detecting device...")
	deviceType, deviceArch, firmware, err := s.detectDevice()
	debug.Printf("[DEBUG] Device detected: %s (%s), err: %v\n", deviceType, deviceArch, err)
	if s.connGen.Load() != gen {
		return canceledConnectionResult
	}
	if err != nil {
		return ConnectionResult{
			Success: true,
			Message: "Connected, but could not detect the reMarkable model.",
			Device:  "unknown",
		}
	}

	go func() {
		s.mu.Lock()
		vc := s.vellumClient
		s.mu.Unlock()
		if vc == nil || s.connGen.Load() != gen {
			return
		}

		if err := s.metadata.Refresh(); err != nil {
			debug.Printf("[DEBUG] Metadata refresh failed: %v\n", err)
		}

		debug.Println("[DEBUG] Checking if vellum is installed...")
		installed, err := vc.IsInstalled()
		debug.Printf("[DEBUG] Vellum installed: %v, err: %v\n", installed, err)

		var warnings ConnectWarnings
		vellumReady := false
		var osVersionStored string

		if err == nil && !installed {
			s.emitVellumState(gen, false, nil)
		} else if err == nil && installed {
			valid, missing, verr := vc.ValidateInstall()
			debug.Printf("[DEBUG] Vellum validation: valid=%v, missing=%v, err=%v\n", valid, missing, verr)
			if verr == nil && !valid {
				s.emitVellumState(gen, true, append([]string{}, missing...))
			} else {
				s.emitVellumState(gen, true, nil)
				vellumReady = true

				reenableStatus, reenableErr := vc.ReenableStatus()
				if reenableErr == nil && reenableStatus != "" {
					debug.Printf("[DEBUG] Reenable status: %s\n", reenableStatus)
					warnings.ReenableStatus = reenableStatus
				}
			}
		}

		hashtabStatus, hashtabErr := s.device.CheckHashtabVersion()
		debug.Printf("[DEBUG] Hashtab check: installed=%v, hashtabVersion=%s, firmwareVersion=%s, needsRebuild=%v, err=%v\n",
			hashtabStatus.Installed, hashtabStatus.HashtabVersion, hashtabStatus.FirmwareVersion, hashtabStatus.NeedsRebuild, hashtabErr)

		var installedVersions map[string]string
		if vellumReady {
			installedVersions, _ = vc.ListInstalledWithVersions()
		}

		updateStatus, updateErr := s.device.GetUpdateServiceStatus()
		debug.Printf("[DEBUG] Auto-update check: enabled=%v, running=%v, err=%v\n", updateStatus.Enabled, updateStatus.Running, updateErr)

		xochitlStatus, xochitlErr := s.device.GetXochitlStatus()
		debug.Printf("[DEBUG] Xochitl check: running=%v, xoviActive=%v, activeLauncher=%q, currentLauncher=%q, err=%v\n",
			xochitlStatus.Running, xochitlStatus.XoviActive, xochitlStatus.ActiveLauncher, xochitlStatus.CurrentLauncher, xochitlErr)

		if hashtabErr == nil {
			warnings.Hashtab = &hashtabStatus
		}
		if updateErr == nil {
			warnings.UpdateService = &updateStatus
		}
		if xochitlErr == nil {
			warnings.Xochitl = &xochitlStatus
		}

		timezoneStatus, timezoneErr := s.device.GetTimezoneStatus()
		debug.Printf("[DEBUG] Timezone check: device=%s, saved=%s, needsUpdate=%v, err=%v\n",
			timezoneStatus.DeviceTimezone, timezoneStatus.SavedTimezone, timezoneStatus.NeedsUpdate, timezoneErr)

		if timezoneStatus.DeviceTimezone != "" && timezoneStatus.SavedTimezone == "" {
			s.mu.Lock()
			deviceID := s.connectedDeviceID
			s.mu.Unlock()
			if deviceID != "" && s.deviceStore != nil {
				if err := s.deviceStore.FillMissingTimezone(deviceID, timezoneStatus.DeviceTimezone); err == nil {
					debug.Printf("[DEBUG] Auto-saved device timezone: %s\n", timezoneStatus.DeviceTimezone)
					timezoneStatus.SavedTimezone = timezoneStatus.DeviceTimezone
				}
			}
		}
		if timezoneErr == nil {
			warnings.TimezoneStatus = &timezoneStatus
		}

		channelStatus := s.os.osChannelStatus()
		debug.Printf("[DEBUG] OS channel check: active=%s, latestPublic=%s\n",
			channelStatus.ActiveVersion, channelStatus.LatestPublic)
		if channelStatus.LatestPublic != "" {
			warnings.OSChannel = &channelStatus
			installedNames := make([]string, 0, len(installedVersions))
			for name := range installedVersions {
				installedNames = append(installedNames, name)
			}
			warnings.IncompatibleWithLatestOS = s.os.incompatibleWithOS(channelStatus.LatestPublic, installedNames)
			debug.Printf("[DEBUG] OS update compatibility: target=%s, installed=%v, incompatible=%v\n",
				channelStatus.LatestPublic, installedNames, warnings.IncompatibleWithLatestOS)
		}

		if vellumReady {
			osState, err := vc.GetOSVersionState()
			if err == nil {
				debug.Printf("[DEBUG] GetOSVersionState: stored=%q, current=%q, mismatch=%v\n", osState.StoredVersion, osState.CurrentVersion, osState.Mismatch)
				osVersionStored = osState.StoredVersion
			} else {
				debug.Printf("[DEBUG] GetOSVersionState: %v\n", err)
			}
			if err == nil && osState.Mismatch {
				debug.Printf("[DEBUG] OS mismatch detected: stored=%s, current=%s\n",
					osState.StoredVersion, osState.CurrentVersion)
				warnings.OSMismatch = &OSVersionChange{
					PrevVersion: osState.StoredVersion,
					NewVersion:  osState.CurrentVersion,
				}

				var filteredInstalled []string
				if installedVersions != nil {
					for name := range installedVersions {
						if !hiddenPackages[name] {
							filteredInstalled = append(filteredInstalled, name)
						}
					}
				}

				s.packages.refreshDevicePackageIndex(vc, string(deviceArch))

				compat, compatErr := vc.CheckOSCompatibility(osState.CurrentVersion)

				compatStatus := PackageCompatibilityStatus{
					InstalledPackages: filteredInstalled,
					CurrentOsVersion:  osState.CurrentVersion,
					StoredOsVersion:   osState.StoredVersion,
				}

				if compatErr != nil || compat == nil || compat.FetchFailed {
					compatStatus.FetchFailed = true
					compatStatus.CompatiblePackages = filteredInstalled
					compatStatus.IncompatiblePackages = []string{}
				} else {
					statusMap := make(map[string]string)
					for _, name := range append(append(compat.Compatible, compat.Incompatible...), compat.NoConstraint...) {
						pkg := s.metadata.GetPackage(name)
						if pkg != nil && pkg.Status != "" && pkg.Status != "maintained" {
							statusMap[name] = pkg.Status
						}
					}
					compatStatus.CompatiblePackages = append(compat.Compatible, compat.NoConstraint...)
					compatStatus.IncompatiblePackages = compat.Incompatible
					compatStatus.StatusMap = statusMap
				}

				warnings.CompatibilityStatus = &compatStatus
			}

			s.emitConnectWarnings(gen, warnings)
		}

		_, hasLauncherctl := installedVersions["launcherctl"]

		if s.deviceInfoCache != nil {
			s.mu.Lock()
			cacheDeviceID := s.connectedDeviceID
			s.mu.Unlock()
			if cacheDeviceID != "" {
				cached := storage.CachedDeviceInfo{
					Timezone:             timezoneStatus.DeviceTimezone,
					HashtabInstalled:     hashtabStatus.Installed,
					HashtabVersion:       hashtabStatus.HashtabVersion,
					HashtabNeedsRebuild:  hashtabStatus.NeedsRebuild,
					LauncherctlInstalled: hasLauncherctl,
					CurrentLauncher:      xochitlStatus.CurrentLauncher,
					ActiveLauncher:       xochitlStatus.ActiveLauncher,
					UpdateServiceEnabled: updateStatus.Enabled,
					UpdateServiceRunning: updateStatus.Running,
					OSVersion:            osVersionStored,
					CachedAt:             time.Now().Unix(),
				}
				s.mu.Lock()
				cached.MachineType = s.connectedDeviceType.DisplayName()
				cached.DeviceType = string(s.connectedDeviceType)
				cached.Arch = string(s.connectedDeviceArch)
				cached.FirmwareVersion = s.connectedFirmware
				s.mu.Unlock()
				if installedVersions != nil {
					cached.InstalledPackages = installedVersions
				}
				if partInfo, err := s.os.GetPartitionInfo(); err == nil {
					cached.PartitionInfo = partInfo
				}
				if err := s.deviceInfoCache.Set(cacheDeviceID, cached); err != nil {
					debug.Printf("[DEBUG] Failed to cache device info: %v\n", err)
				}
			}
		}

		if !vellumReady {
			s.emitConnectWarnings(gen, warnings)
		}

		s.emitGuideStatus(gen, s.files.checkUserGuide())
	}()

	machineID, machineErr := s.readMachineID()
	if machineErr != nil {
		// An unreadable machine ID only costs the swapped-device check, so the connection continues without one.
		debug.Printf("[DEBUG] Could not identify device: %v\n", machineErr)
	}

	s.mu.Lock()
	if s.connGen.Load() != gen {
		s.mu.Unlock()
		return canceledConnectionResult
	}
	s.connectedDeviceType = deviceType
	s.connectedDeviceArch = deviceArch
	s.connectedFirmware = firmware
	previousMachineID := s.lastMachineID
	s.connectedMachineID = machineID
	if machineID != "" {
		s.lastMachineID = machineID
	}
	s.mu.Unlock()

	freshDevice := machineID == "" || machineID != previousMachineID
	s.reconcileSavedMachineID(deviceID, machineID)

	go s.os.maybeRecoverLegacyConf()

	return ConnectionResult{
		Success:     true,
		Message:     "Connected",
		Device:      string(deviceType),
		FreshDevice: freshDevice,
	}
}

// A saved entry is a host and a credential, so it can reach a different tablet after a cable swap, and the cached state it accumulated belongs to the old one.
func (s *ConnectionService) reconcileSavedMachineID(deviceID, machineID string) {
	if deviceID == "" || machineID == "" || s.deviceStore == nil {
		return
	}

	device, err := s.deviceStore.Get(deviceID)
	if err != nil {
		return
	}

	if device.MachineID == machineID {
		return
	}

	if device.MachineID != "" && s.deviceInfoCache != nil {
		debug.Println("[DEBUG] Saved device now reaches a different reMarkable, dropping its cached state")
		_ = s.deviceInfoCache.Delete(deviceID)
	}

	if err := s.deviceStore.UpdateMachineID(deviceID, machineID); err != nil {
		debug.Printf("[DEBUG] Failed to record device identity: %v\n", err)
	}
}

const shellProbeToken = "__REMANAGER_SHELL_OK__"

func (s *ConnectionService) shellEmitsNoise(client *ssh.Client) bool {
	session, err := client.NewSession()
	if err != nil {
		return false
	}
	defer session.Close()

	// Combined because runCommand reads stderr too, so rc noise there corrupts every parsed result just the same.
	out, err := session.CombinedOutput("printf '%s' " + shellProbeToken)
	if err != nil && len(out) == 0 {
		return false
	}
	return string(out) != shellProbeToken
}

// readMachineID returns the derived identity for the connected tablet. The
// machine ID itself is never stored or logged: it may be a function of the
// device serial, and only an equality test is ever needed.
func (s *ConnectionService) readMachineID() (string, error) {
	client := s.getClient()
	if client == nil {
		return "", fmt.Errorf("not connected")
	}

	session, err := client.NewSession()
	if err != nil {
		return "", fmt.Errorf("failed to open session: %w", err)
	}
	defer session.Close()

	out, err := session.Output("cat /etc/machine-id")
	if err != nil {
		return "", fmt.Errorf("failed to read machine ID: %w", err)
	}

	return deviceid.Derive(string(out))
}

func (c *core) detectDevice() (rmdevice.Type, rmdevice.Architecture, string, error) {
	client := c.getClient()

	if client == nil {
		return rmdevice.Unknown, "", "", fmt.Errorf("not connected")
	}

	fs, err := rmfilesystem.NewSSH(client)
	if err != nil {
		return rmdevice.Unknown, "", "", fmt.Errorf("failed to create filesystem: %w", err)
	}
	defer fs.Close()

	deviceType, err := rmdevice.Detect(fs)
	if err != nil {
		return rmdevice.Unknown, "", "", fmt.Errorf("failed to detect device: %w", err)
	}

	exec := rmexecutor.NewSSH(client, rmexecutor.WithLogger(debug.SlogLogger()))
	arch, err := rmdevice.DetectArchitecture(exec)
	if err != nil {
		return deviceType, "", "", fmt.Errorf("failed to detect architecture: %w", err)
	}

	firmware, _ := rmdevice.DetectVersion(fs)

	return deviceType, arch, firmware, nil
}

// Closing the client without s.mu releases any command that holds s.mu while
// it waits on the reMarkable, so a hung command can never block a disconnect.
func (s *ConnectionService) closeLiveClient() {
	if client := s.liveClient.Swap(nil); client != nil {
		client.Close()
	}
}

func (s *ConnectionService) Disconnect() {
	if s.logger != nil {
		s.logger.LogConnection("disconnect", "user initiated")
	}

	s.stopReconnect()
	s.connGen.Add(1)
	s.closeLiveClient()

	s.stopDemo()
	s.stopConnectionMonitor()

	s.os.stopPreventSleep()
	s.shell.StopShell()
	s.cancelOperationsWaitingOnPrompts()
	s.files.CancelAllTransfers()

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.connectCancel != nil {
		s.connectCancel()
		s.connectCancel = nil
	}

	s.currentConn = nil
	s.connectedDeviceID = ""
	s.connectedDeviceType = ""
	s.connectedDeviceArch = ""
	s.connectedFirmware = ""
	// Both cleared: disconnecting by hand ends the session, so connecting again
	// is a first look even at the same reMarkable. An automatic reconnect does
	// not come through here, and so still counts as a return.
	s.connectedMachineID = ""
	s.lastMachineID = ""
	s.writableRootBusy = false

	if s.client != nil {
		s.client.Close()
		s.client = nil
	}
	if s.agentConn != nil {
		s.agentConn.Close()
		s.agentConn = nil
	}
	s.vellumClient = nil
}

func (c *core) isConnected() bool {
	if c.inDemoMode() {
		return true
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.client != nil
}

func (c *core) claimWritableRoot(deviceType rmdevice.Type) error {
	if !deviceType.IsPaperPro() {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.writableRootBusy {
		return fmt.Errorf("another operation is modifying the filesystem")
	}
	c.writableRootBusy = true
	c.wails.Event.Emit(eventWritableRootBusy, true)
	return nil
}

func (c *core) releaseWritableRootClaim(deviceType rmdevice.Type) {
	if !deviceType.IsPaperPro() {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.writableRootBusy = false
	c.wails.Event.Emit(eventWritableRootBusy, false)
}

const (
	keepaliveInterval    = 15 * time.Second
	keepaliveTimeout     = 5 * time.Second
	maxReconnectAttempts = 5

	initialDialTimeout = 10 * time.Second
	// Reconnect dials once per attempt, and this bounds each of its two stages,
	// the TCP connect and the SSH handshake. The UI sizes the reconnect window
	// from it, which a link that answers promptly stays inside.
	reconnectDialTimeout = 5 * time.Second

	installKeepaliveTimeout = 20 * time.Second
	installKeepaliveMaxMiss = 3
)

var reconnectBackoff = []time.Duration{
	2 * time.Second,
	4 * time.Second,
	8 * time.Second,
	16 * time.Second,
	30 * time.Second,
}

func (s *ConnectionService) startConnectionMonitor(client *ssh.Client) {
	s.mu.Lock()
	if s.keepaliveStop != nil {
		s.mu.Unlock()
		return
	}
	s.keepaliveStop = make(chan struct{})
	s.keepaliveTrigger = make(chan struct{}, 1)
	s.mu.Unlock()

	go s.connectionMonitorLoop(client)
}

func (s *ConnectionService) stopConnectionMonitor() {
	s.mu.Lock()
	if s.keepaliveStop != nil {
		close(s.keepaliveStop)
		s.keepaliveStop = nil
	}
	s.keepaliveTrigger = nil
	s.mu.Unlock()
}

// Must invoke via a goroutine.
func (c *core) triggerConnectionCheck() {
	// During an install the ticker deliberately tolerates dropouts
	if c.isInstallActive() {
		return
	}
	c.mu.Lock()
	ch := c.keepaliveTrigger
	c.mu.Unlock()
	if ch == nil {
		return
	}
	select {
	case ch <- struct{}{}:
	default:
	}
}

// The monitor owns the client it watches and never waits on s.mu while
// checking or closing it. A caller that holds s.mu can still be waiting on the
// dead link, and is only released once the client is closed.
func (s *ConnectionService) connectionMonitorLoop(client *ssh.Client) {
	ticker := time.NewTicker(keepaliveInterval)
	defer ticker.Stop()

	s.mu.Lock()
	stopCh := s.keepaliveStop
	triggerCh := s.keepaliveTrigger
	s.mu.Unlock()

	misses := 0
	for {
		select {
		case <-stopCh:
			return
		case <-triggerCh:
			if err := checkConnection(client, keepaliveTimeout); err != nil {
				s.handleConnectionLost(client, err)
				return
			}
			misses = 0
		case <-ticker.C:
			timeout := keepaliveTimeout
			maxMiss := 1
			if s.isInstallActive() {
				timeout = installKeepaliveTimeout
				maxMiss = installKeepaliveMaxMiss
			}
			if err := checkConnection(client, timeout); err != nil {
				misses++
				if misses >= maxMiss {
					s.handleConnectionLost(client, err)
					return
				}
			} else {
				misses = 0
			}
		}
	}
}

func isConnectionDeadError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, io.EOF) {
		return true
	}
	msg := strings.ToLower(err.Error())
	for _, s := range []string{
		"use of closed network connection",
		"connection reset by peer",
		"broken pipe",
		"unexpected packet in response to channel open",
		"did not properly respond",
		"wsarecv",
		"wsasend",
		"actively refused",
	} {
		if strings.Contains(msg, s) {
			return true
		}
	}
	return false
}

func checkConnection(client *ssh.Client, timeout time.Duration) error {
	done := make(chan error, 1)
	go func() {
		_, _, err := client.SendRequest("keepalive@openssh.com", true, nil)
		done <- err
	}()

	select {
	case err := <-done:
		return err
	case <-time.After(timeout):
		return fmt.Errorf("keepalive timeout")
	}
}

func (s *ConnectionService) handleConnectionLost(client *ssh.Client, err error) {
	if s.logger != nil {
		s.logger.LogConnection("connection-lost", err.Error())
	}
	// Must be set before the client.Close() below unblocks the install's command.
	s.installMu.Lock()
	if s.installSession != nil {
		s.installSession.mu.Lock()
		s.installSession.connLost = true
		s.installSession.mu.Unlock()
	}
	s.installMu.Unlock()
	client.Close()

	s.mu.Lock()
	var lostHost string
	if s.currentConn != nil {
		lostHost = s.currentConn.host
	}
	s.mu.Unlock()
	s.logUSBNetworkDiagnostics(lostHost)

	s.mu.Lock()
	disconnectedByUser := s.currentConn != nil && s.currentConn.gen != s.connGen.Load()
	if s.client != client || disconnectedByUser {
		s.mu.Unlock()
		return
	}
	s.client = nil
	if s.agentConn != nil {
		s.agentConn.Close()
		s.agentConn = nil
	}
	s.vellumClient = nil
	s.writableRootBusy = false
	// Retained through the reconnect, so a manual retry can redial with the
	// secret the user typed. Only an explicit disconnect, or an auth failure that
	// proves the secret wrong, clears it.
	var target *connTarget
	if s.currentConn != nil {
		t := *s.currentConn
		target = &t
	}
	s.keepaliveStop = nil
	s.keepaliveTrigger = nil
	s.mu.Unlock()

	s.outputToAllOperations("\nConnection lost.\n")

	ue := apperrors.Classify(err)
	deviceID := ""
	if target != nil {
		deviceID = target.deviceID
	}
	s.wails.Event.Emit(eventConnectionLost, ConnectionFailure{
		Reason:   ue.Message,
		Code:     ue.Code,
		DeviceID: deviceID,
	})

	if target != nil {
		go s.attemptReconnect(*target)
	} else {
		s.wails.Event.Emit(eventConnectionFailed, ConnectionFailure{
			Reason: "Connection lost. Reconnect to continue.",
			Code:   apperrors.ErrHostDown,
		})
	}
}

func (s *ConnectionService) attemptReconnect(target connTarget) {
	debug.Printf("[%s] attemptReconnect started for device %q (gen %d)\n", time.Now().Format("15:04:05.000"), target.deviceID, target.gen)

	run := s.beginReconnect()
	if run == nil {
		debug.Printf("[%s] Already reconnecting, skipping\n", time.Now().Format("15:04:05.000"))
		return
	}

	restored := false
	defer func() {
		s.endReconnect(run)
		if !restored && s.connGen.Load() == target.gen {
			s.cancelOperationsWaitingOnPrompts()
		}
		debug.Printf("[%s] attemptReconnect finished\n", time.Now().Format("15:04:05.000"))
	}()

	s.mu.Lock()
	lostMachineID := s.lastMachineID
	s.mu.Unlock()

	// A disconnect or a newer connection ends this reconnect. Its own failure is
	// then not news: the user already chose what happens next.
	superseded := func() bool {
		if !s.reconnectCurrent(run) || s.connGen.Load() != target.gen {
			debug.Printf("[%s] Reconnect canceled/superseded\n", time.Now().Format("15:04:05.000"))
			return true
		}
		return false
	}

	for attempt := 0; attempt < maxReconnectAttempts; attempt++ {
		if superseded() {
			return
		}

		debug.Printf("[%s] Reconnect attempt %d/%d starting\n", time.Now().Format("15:04:05.000"), attempt+1, maxReconnectAttempts)

		attemptElapsed, attemptTotal := reconnectTimeline(attempt, false)
		s.wails.Event.Emit(eventConnectionReconnecting, ReconnectPhase{
			Attempt:     attempt + 1,
			MaxAttempts: maxReconnectAttempts,
			DeviceID:    target.deviceID,
			DurationMs:  reconnectDialTimeout.Milliseconds(),
			ElapsedMs:   attemptElapsed,
			TotalMs:     attemptTotal,
		})

		result := s.establishConnection(target.host, target.authType, target.secret, target.keyRef, target.deviceID, target.gen)
		debug.Printf("[%s] Reconnect attempt %d/%d result: success=%v, message=%s\n", time.Now().Format("15:04:05.000"), attempt+1, maxReconnectAttempts, result.Success, result.Message)

		// Retrying cannot fix a rejected credential, and repeated auth attempts are
		// what trip sshd rate limiting. A wrong secret is also not worth keeping.
		if !result.Success && !result.Retryable {
			if superseded() {
				return
			}
			if result.Code == apperrors.ErrAuthFailed {
				s.mu.Lock()
				s.currentConn = nil
				s.mu.Unlock()
			}
			s.wails.Event.Emit(eventConnectionFailed, ConnectionFailure{
				Reason:   result.Message,
				Code:     result.Code,
				DeviceID: target.deviceID,
			})
			return
		}

		if result.Success {
			if s.connGen.Load() != target.gen {
				return
			}
			if s.reconnectReachedDifferentTablet(lostMachineID) {
				s.abandonSwappedReconnect(target.deviceID)
				return
			}
			if target.deviceID != "" && s.deviceStore != nil {
				s.deviceStore.UpdateLastConnected(target.deviceID, time.Now().Unix())
			}
			if s.logger != nil {
				s.logger.LogConnection("reconnected", target.deviceID)
			}
			s.installMu.Lock()
			sess := s.installSession
			s.installMu.Unlock()
			newClient := s.getClient()
			if sess != nil && newClient != nil {
				select {
				case sess.resume <- newClient:
				default:
				}
			}
			restored = true
			s.wails.Event.Emit(eventConnectionRestored, ConnectionRestored{
				DeviceID: target.deviceID,
				Device:   result.Device,
			})
			return
		}

		if attempt < maxReconnectAttempts-1 {
			backoffIdx := attempt
			if backoffIdx >= len(reconnectBackoff) {
				backoffIdx = len(reconnectBackoff) - 1
			}
			debug.Printf("[%s] Waiting %v before next attempt\n", time.Now().Format("15:04:05.000"), reconnectBackoff[backoffIdx])
			// Emitted as the wait begins, not as the attempt begins: the dial can
			// take seconds of its own, so a countdown started earlier would lie.
			waitElapsed, waitTotal := reconnectTimeline(attempt, true)
			s.wails.Event.Emit(eventConnectionRetryScheduled, ReconnectPhase{
				Attempt:     attempt + 2,
				MaxAttempts: maxReconnectAttempts,
				DeviceID:    target.deviceID,
				DurationMs:  reconnectBackoff[backoffIdx].Milliseconds(),
				ElapsedMs:   waitElapsed,
				TotalMs:     waitTotal,
			})
			select {
			case <-time.After(reconnectBackoff[backoffIdx]):
			case <-run.stop:
			case <-run.wake:
				attempt = -1
			}
			debug.Printf("[%s] Backoff complete\n", time.Now().Format("15:04:05.000"))
		} else {
			select {
			case <-run.wake:
				attempt = -1
			default:
			}
		}
	}

	debug.Printf("[%s] All reconnect attempts exhausted\n", time.Now().Format("15:04:05.000"))
	if superseded() {
		return
	}
	s.reconnectMu.Lock()
	s.reconnectExhausted = true
	s.reconnectMu.Unlock()
	s.wails.Event.Emit(eventConnectionFailed, ConnectionFailure{
		Reason:   "Could not reconnect after several attempts. Check your connection and try again.",
		Code:     apperrors.ErrTimeout,
		DeviceID: target.deviceID,
	})
}

// An identity that could not be read on either side counts as the same tablet,
// since the check has nothing to compare.
func (s *ConnectionService) reconnectReachedDifferentTablet(lostMachineID string) bool {
	s.mu.Lock()
	reachedMachineID := s.connectedMachineID
	s.mu.Unlock()
	return lostMachineID != "" && reachedMachineID != "" && reachedMachineID != lostMachineID
}

// Everything the session holds, including a paused install, belongs to the
// tablet that was lost, so the new one is left for a fresh connect instead.
func (s *ConnectionService) abandonSwappedReconnect(deviceID string) {
	debug.Println("[DEBUG] Reconnect reached a different reMarkable, disconnecting")
	if s.logger != nil {
		s.logger.LogConnection("reconnect reached a different reMarkable", deviceID)
	}
	s.Disconnect()
	s.wails.Event.Emit(eventConnectionFailed, ConnectionFailure{
		Reason:   "A different reMarkable answered at this address. Connect again to continue with it.",
		Code:     apperrors.ErrDeviceChanged,
		DeviceID: deviceID,
	})
}

// A device saved on another host can arrive naming auth this host cannot use:
// a key file that stayed behind, or an SSH agent a phone does not have.
func unusableSyncedAuth(device *storage.SavedDevice) string {
	if device.AuthType == "key" && !device.HasKey() {
		return "This reMarkable was saved on another device with a key file that does not sync. Choose an SSH key to use here."
	}
	if device.AuthType == "agent" && !platform.HasSSHAgent() {
		return "This reMarkable was saved on another device that uses an SSH agent, which is not available here. Choose an SSH key to use here."
	}
	return ""
}
