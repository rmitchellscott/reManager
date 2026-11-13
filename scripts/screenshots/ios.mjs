import { execFile, spawn } from 'node:child_process'
import net from 'node:net'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { promisify } from 'node:util'
import { remote } from 'webdriverio'
import { repo } from './shots.mjs'

const bundleId = 'io.scottlabs.reManager'
const here = path.dirname(fileURLToPath(import.meta.url))
const run = promisify(execFile)
const sleep = ms => new Promise(resolve => setTimeout(resolve, ms))
const simctl = async (...args) => (await run('xcrun', ['simctl', ...args])).stdout.trim()
const agentDevice = (...args) => run(path.join(here, 'node_modules/.bin/agent-device'), [...args, '--json'])

async function findSimulator(name) {
  const { devices } = JSON.parse(await simctl('list', 'devices', 'available', '-j'))
  const matches = Object.entries(devices)
    .flatMap(([runtime, list]) => list.map(device => ({ ...device, runtime })))
    .filter(device => device.name === name)
    .sort((a, b) => b.runtime.localeCompare(a.runtime, undefined, { numeric: true }))
  if (matches.length === 0) throw new Error(`no simulator named ${name}`)
  return matches.find(device => device.state === 'Booted') ?? matches[0]
}

// The 12-hour clock and the en-US language only take effect after a reboot,
// and an iPad ignores AppleICUForce12HourTime unless the 24-hour flag is also
// cleared, so the status bar would read 09:41 without all four and a restart.
async function prepareSimulator(udid) {
  await simctl('bootstatus', udid, '-b')
  const twelveHour = await simctl('spawn', udid, 'defaults', 'read', '-g', 'AppleICUForce12HourTime').catch(() => '')
  if (twelveHour !== '1') {
    await simctl('spawn', udid, 'defaults', 'write', '-g', 'AppleLocale', 'en_US')
    await simctl('spawn', udid, 'defaults', 'write', '-g', 'AppleLanguages', '-array', 'en-US')
    await simctl('spawn', udid, 'defaults', 'write', '-g', 'AppleICUForce12HourTime', '-bool', 'true')
    await simctl('spawn', udid, 'defaults', 'write', '-g', 'AppleICUForce24HourTime', '-bool', 'false')
    await simctl('shutdown', udid)
    await simctl('bootstatus', udid, '-b')
  }
  await simctl('ui', udid, 'appearance', 'light')
  // charged draws a charging bolt; discharging at 100 does not.
  await simctl('status_bar', udid, 'override', '--time', '9:41', '--dataNetwork', 'wifi', '--wifiMode', 'active',
    '--wifiBars', '3', '--cellularMode', 'active', '--cellularBars', '4', '--batteryState', 'discharging', '--batteryLevel', '100')
}

// simctl has no hinge control. agent-device folds the iPhone Duo with a
// simulator HID event, and it refuses while another agent-device session
// holds the simulator.
async function setPose(udid, pose) {
  const session = ['--session', 'remanager-screenshots']
  await agentDevice('open', bundleId, '--platform', 'ios', '--udid', udid, ...session)
  try {
    await agentDevice('fold', pose, ...session)
  } finally {
    await agentDevice('close', ...session).catch(() => {})
  }
}

// The iPhone Duo has a framebuffer per panel, and simctl captures the outer
// one unless a display is named.
async function displayWithSize(udid, { width, height }) {
  const ports = (await simctl('io', udid, 'enumerate')).split(/\n(?=Port:)/)
  const port = ports.find(p => p.includes(`Default width: ${width}\n`) && p.includes(`Default height: ${height}\n`))
  const uuid = port?.match(/UUID: (\S+)/)?.[1]
  if (!uuid) throw new Error(`no ${width}x${height} display on ${udid}`)
  return uuid
}

async function isPortrait(file) {
  const { stdout } = await run('sips', ['-g', 'pixelWidth', '-g', 'pixelHeight', file])
  const [width, height] = [...stdout.matchAll(/pixel(?:Width|Height): (\d+)/g)].map(match => Number(match[1]))
  return height > width
}

const freePort = () =>
  new Promise((resolve, reject) => {
    const server = net.createServer().listen(0, () => {
      const { port } = server.address()
      server.close(() => resolve(port))
    })
    server.on('error', reject)
  })

async function startAppium() {
  const port = await freePort()
  const server = spawn(path.join(here, 'node_modules/.bin/appium'), ['--port', String(port), '--log-level', 'error'], {
    env: { ...process.env, APPIUM_HOME: path.join(here, '.appium') },
    stdio: 'inherit',
  })
  for (let i = 0; i < 120; i++) {
    try {
      if ((await fetch(`http://127.0.0.1:${port}/status`)).ok) return { server, port }
    } catch {}
    await sleep(500)
  }
  server.kill()
  throw new Error('appium never started')
}

async function webviewContext(driver) {
  for (let i = 0; i < 120; i++) {
    const webview = (await driver.getContexts()).find(c => String(c.id ?? c).startsWith('WEBVIEW'))
    if (webview) return String(webview.id ?? webview)
    await sleep(1000)
  }
  throw new Error('the app never exposed an inspectable WebView')
}

// A freshly booted simulator can leave the first launch on a white screen
// while the catalog fetch times out; a relaunch loads it. Saved devices
// survive an uninstall in the simulator keychain, and then the app opens on
// the device list instead of the connect form.
const connectScreenShown = () =>
  document.getElementById('host') !== null ||
  [...document.querySelectorAll('button')].some(button => button.textContent.trim() === 'Add reMarkable')

async function waitForConnectScreen(driver) {
  for (let attempt = 0; attempt < 3; attempt++) {
    for (let i = 0; i < 90; i++) {
      if (await driver.execute(connectScreenShown).catch(() => false)) return
      await sleep(1000)
    }
    await driver.execute('mobile: terminateApp', { bundleId })
    await driver.execute('mobile: activateApp', { bundleId })
    await driver.switchContext(await webviewContext(driver))
  }
  throw new Error('the connect screen never loaded')
}

export async function runIos(target, shoot) {
  const simulator = await findSimulator(target.device)
  const bootedHere = simulator.state !== 'Booted'
  if (bootedHere) await simctl('boot', simulator.udid)
  await prepareSimulator(simulator.udid)
  await simctl('uninstall', simulator.udid, bundleId)
  await simctl('install', simulator.udid,
    process.env.IOS_APP || path.join(repo, 'build/ios/xcode/DerivedData/Build/Products/Debug-iphonesimulator/reManager.app'))
  if (target.pose) await setPose(simulator.udid, target.pose)
  const display = target.display ? [`--display=${await displayWithSize(simulator.udid, target.display)}`] : []

  const appium = await startAppium()
  let driver
  try {
    driver = await remote({
      hostname: '127.0.0.1',
      port: appium.port,
      logLevel: 'error',
      capabilities: {
        platformName: 'iOS',
        'appium:automationName': 'XCUITest',
        'appium:udid': simulator.udid,
        'appium:bundleId': bundleId,
        'appium:noReset': true,
        'appium:autoAcceptAlerts': true,
        ...(target.presetPose ? {} : { 'appium:orientation': target.landscape ? 'LANDSCAPE' : 'PORTRAIT' }),
        'appium:newCommandTimeout': 600,
        'appium:wdaLaunchTimeout': 600_000,
      },
    })
    await driver.switchContext(await webviewContext(driver))
    await waitForConnectScreen(driver)
    // On the open iPhone Duo, Appium and agent-device both report a rotation
    // that never happens, so a person sets the pose in the Simulator first.
    if (target.presetPose && !(await driver.execute(() => window.innerWidth > window.innerHeight)))
      throw new Error(`set the ${target.device} to ${target.presetPose} in the Simulator first`)

    await driver.switchContext('NATIVE_APP')
    const webview = await driver.getElementRect((await driver.$('//XCUIElementTypeWebView')).elementId)
    await driver.switchContext(await webviewContext(driver))
    const scale = webview.width / (await driver.execute(() => window.innerWidth))

    await shoot({
      evaluate: (fn, arg) => driver.execute(fn, arg),
      tap: (x, y) =>
        driver.execute('mobile: tap', { x: Math.round(webview.x + x * scale), y: Math.round(webview.y + y * scale) }),
      async screenshot(file) {
        await simctl('io', simulator.udid, 'screenshot', '--type=png', ...display, file)
        // Some simctl versions save a landscape simulator in the device's
        // native portrait orientation, and others save it upright.
        if (target.landscape && (await isPortrait(file))) await run('sips', ['-r', '90', file])
      },
    })
  } finally {
    await driver?.deleteSession().catch(() => {})
    appium.server.kill()
    await simctl('status_bar', simulator.udid, 'clear').catch(() => {})
    if (bootedHere) await simctl('shutdown', simulator.udid).catch(() => {})
  }
}
