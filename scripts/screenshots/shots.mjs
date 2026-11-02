import path from 'node:path'
import { fileURLToPath } from 'node:url'

export const repo = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../..')
const store = path.join(repo, 'assets/store')
const guide = path.join(repo, 'docs/guide/images')

const hoursAgo = hours => Math.floor(Date.now() / 1000) - hours * 3600

const storeSix = {
  discover: 'screenshot-1-discover.png',
  package: 'screenshot-2-package.png',
  plan: 'screenshot-3-plan.png',
  upgrades: 'screenshot-4-upgrades.png',
  system: 'screenshot-5-system.png',
  osManager: 'screenshot-6-os-manager.png',
}

const storeFour = {
  discover: 'screenshot-1-discover.png',
  package: 'screenshot-2-package.png',
  upgrades: 'screenshot-3-upgrades.png',
  system: 'screenshot-4-system.png',
}

const readmeShots = scheme => ({
  discover: `screenshot-discover-${scheme}.png`,
  package: `screenshot-package-${scheme}.png`,
  plan: `screenshot-plan-${scheme}.png`,
  system: `screenshot-system-${scheme}.png`,
})

const desktopWindow = { width: 1012, height: 944 }

export const targets = {
  iphone: {
    platform: 'ios',
    device: 'iPhone 17 Pro Max',
    dir: path.join(store, 'app-store/iphone-6.9'),
    files: storeSix,
  },
  ipad: {
    platform: 'ios',
    device: 'iPad Pro 13-inch (M5)',
    landscape: true,
    dir: path.join(store, 'app-store/ipad-13'),
    files: storeSix,
  },
  play: {
    platform: 'android',
    screenSize: '1080x1920',
    dir: path.join(store, 'play-store'),
    files: storeFour,
  },
  // Windows renders through WebView2, which is Chromium.
  microsoft: {
    platform: 'desktop',
    browser: 'chromium',
    viewport: { width: 1366, height: 768 },
    dir: path.join(store, 'microsoft-store'),
    files: storeFour,
  },
  // Linux renders through WebKitGTK.
  flathub: {
    platform: 'desktop',
    browser: 'webkit',
    viewport: desktopWindow,
    dir: path.join(store, 'flathub'),
    files: storeFour,
  },
  // 1440x900 at 2x is 2880x1800, a Mac App Store screenshot size.
  mac: {
    platform: 'desktop',
    browser: 'webkit',
    viewport: { width: 1440, height: 900 },
    dir: path.join(store, 'app-store/mac'),
    files: storeSix,
  },
  'readme-light': {
    platform: 'desktop',
    browser: 'webkit',
    viewport: desktopWindow,
    windowShadow: true,
    dir: path.join(repo, 'assets/readme'),
    files: readmeShots('light'),
  },
  'readme-dark': {
    platform: 'desktop',
    browser: 'webkit',
    viewport: desktopWindow,
    windowShadow: true,
    colorScheme: 'dark',
    dir: path.join(repo, 'assets/readme'),
    files: readmeShots('dark'),
  },
  guide: {
    platform: 'desktop',
    browser: 'webkit',
    viewport: desktopWindow,
    windowShadow: true,
    // Written into the server's device file, so the list needs no keychain.
    savedDevices: [
      { id: 'a1b2c3d4e5f60718', name: 'Paper Pure', host: '10.11.99.1', authType: 'password', lastConnected: hoursAgo(1) },
      { id: 'b2c3d4e5f6071829', name: 'Paper Pro Move', host: '192.168.1.42', authType: 'password', lastConnected: hoursAgo(72) },
    ],
    dir: guide,
    files: {
      savedDevices: 'saved-devices.png',
      connect: 'connection-panel.png',
      discover: 'mods-tab.png',
      package: 'package-details.png',
      plan: 'plan.png',
      installing: 'installing-package.png',
      planResult: 'plan-result.png',
      system: 'maintenance-tab.png',
      osManager: 'os-manager.png',
      osVersions: 'os-versions.png',
      terminal: 'terminal.png',
      fileBrowser: 'file-browser.png',
      backup: 'backup-restore.png',
      backupProgress: 'backup-progress.png',
      backupComplete: 'backup-complete.png',
      documentImport: 'document-import.png',
      documentImportFiles: 'document-import-files.png',
      settings: 'settings.png',
      osChange: 'os-change.png',
      osChangeReview: 'os-change-review.png',
    },
  },
}

// The phone layout shows each tool as a labeled grid button, and the wider
// layouts as a row whose button only says Open.
async function openTool(app, name, action = 'Open') {
  if (await app.isShown('button', name)) await app.tap('button', name)
  else await app.tapInRow(name, action)
}

// The order is load-bearing: each plan is cleared before the next shot so its
// bar does not carry over, and automatic OS updates goes off before System
// because the on state shows an amber status. The screenshots password is the
// demo whose terminal and file browser look like a real tablet's, and the
// os-update password is the same demo connecting after an OS update.
async function connectDemo(app, password, capture = async () => {}) {
  while (!(await app.isShown('textbox', /^IP address/))) {
    if (await app.isShown('button', 'Add reMarkable')) {
      await capture('savedDevices')
      await app.tap('button', 'Add reMarkable')
    }
    await app.sleep(500)
  }
  await capture('connect')
  await app.tap('switch', /^Remember this reMarkable/)
  await app.fill(/^IP address/, 'demo')
  await app.fill('SSH password', password)
  await app.tap('button', 'Connect')
}

export async function tour(app, capture, wants) {
  await connectDemo(app, 'screenshots', capture)

  await app.waitFor('tab', /^Mods/)
  await app.tap('tab', /^Mods/)
  await app.waitFor('heading', 'Popular this month')
  await app.sleep(3000)
  await capture('discover')

  await app.fill('Search mods…', 'remark')
  await app.tap('button', 'remarkdown')
  await app.tap('button', 'changelog')
  await app.waitFor('text', /^0\.3\.0$/)
  await capture('package')
  await app.tap('button', 'Close')

  for (const name of ['quicksettings-screenshot', 'rmstream']) {
    await app.fill('Search mods…', name)
    await app.tap('button', `Add ${name}`)
  }
  await app.fill('Search mods…', '')
  await app.tap('button', /^Changes/)
  await app.waitFor('text', /^framebuffer-spy$/)
  await capture('plan')
  if (wants('installing') || wants('planResult')) {
    await app.tap('button', 'Apply')
    await app.waitFor('button', 'Stop')
    await app.sleep(1500)
    await capture('installing')
    await app.waitFor('button', 'Done')
    await capture('planResult')
    await app.tap('button', 'Done')
  } else {
    await app.tap('button', 'Clear all')
  }
  if (await app.isShown('button', 'Close')) await app.tap('button', 'Close')

  await app.tap('tab', /^Upgrades/)
  await app.waitFor('text', /^\d+ upgrades? available$/)
  await app.tap('button', 'Add all')
  await app.waitFor('button', /^Changes/)
  await app.sleep(2000)
  await capture('upgrades')
  await app.tap('button', /^Changes/)
  await app.tap('button', 'Clear all')
  if (await app.isShown('button', 'Close')) await app.tap('button', 'Close')

  await app.tap('tab', /^System/)
  await app.tap('switch', /^$/)
  await app.waitFor('text', /^Off$/)
  await app.sleep(2000)
  await capture('system')

  await app.tap('button', 'Manage')
  await app.waitFor('text', /^Partition B$/)
  await app.sleep(3000)
  await capture('osManager')
  if (wants('osVersions')) {
    await app.tap('button', /^Choose a version/)
    await app.waitFor('text', /^Latest$/)
    await capture('osVersions')
    await app.tap('button', /^Choose a version/)
  }
  await app.tap('button', 'Close')

  await openTool(app, 'Terminal')
  await app.waitFor('button', 'Stop shell')
  await app.sleep(2000)
  await capture('terminal')
  await app.tap('button', 'Stop shell')
  if (await app.isShown('button', 'Close Terminal')) await app.tap('button', 'Close Terminal')

  await openTool(app, 'File Browser')
  await app.waitFor('text', /^suspended\.png$/)
  await capture('fileBrowser')
  await app.tap('button', 'Close file browser')

  await openTool(app, 'Backup & Restore')
  await app.waitFor('button', 'Back up')
  await capture('backup')
  if (wants('backupProgress') || wants('backupComplete')) {
    await app.tap('button', 'Back up')
    await app.waitFor('text', /^Downloading from your reMarkable/)
    await app.sleep(4000)
    await capture('backupProgress')
    await app.waitFor('text', /^Backup created$/)
    await capture('backupComplete')
    await app.tap('button', 'Close')
  } else {
    await app.tap('button', 'Cancel')
  }

  await openTool(app, 'Document Import', 'Import')
  await app.waitFor('text', /^Drop files here$/)
  await capture('documentImport')
  if (wants('documentImportFiles')) {
    await app.tap('text', /^Drop files here$/)
    await app.waitFor('text', /^3 files selected$/)
    await capture('documentImportFiles')
  }
  await app.tap('button', 'Cancel')

  await app.tap('button', 'Settings')
  await capture('settings')
  await app.tap('button', 'Close settings')

  await app.tap('button', 'Disconnect')
  await connectDemo(app, 'os-update')
  await app.waitFor('text', /^OS change detected$/i)
  await app.sleep(2000)
  await capture('osChange')
  await app.tap('button', 'Review')
  await app.sleep(2500)
  await capture('osChangeReview')
}
