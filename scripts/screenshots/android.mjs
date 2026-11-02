import { execFile, spawn } from 'node:child_process'
import { writeFile } from 'node:fs/promises'
import os from 'node:os'
import path from 'node:path'
import { promisify } from 'node:util'
import { chromium } from 'playwright'
import { repo } from './shots.mjs'

const appId = 'io.scottlabs.reManager'
const sdk = process.env.ANDROID_HOME ?? path.join(os.homedir(), 'Library/Android/sdk')
const adbPath = path.join(sdk, 'platform-tools/adb')
const run = promisify(execFile)
const sleep = ms => new Promise(resolve => setTimeout(resolve, ms))

const adb = async (serial, ...args) =>
  (await run(adbPath, ['-s', serial, ...args], { encoding: 'buffer', maxBuffer: 64 << 20 })).stdout
const shell = async (serial, command) => (await adb(serial, 'shell', command)).toString().trim()

async function onlineEmulators() {
  const { stdout } = await run(adbPath, ['devices'])
  return stdout
    .split('\n')
    .map(line => line.split('\t'))
    .filter(([serial, state]) => serial.startsWith('emulator-') && state === 'device')
    .map(([serial]) => serial)
}

async function bootEmulator(avd) {
  const before = new Set(await onlineEmulators())
  spawn(path.join(sdk, 'emulator/emulator'), ['-avd', avd, '-no-snapshot-save'], { detached: true, stdio: 'ignore' }).unref()
  for (let i = 0; i < 300; i++) {
    const serial = (await onlineEmulators()).find(s => !before.has(s))
    if (serial && (await shell(serial, 'getprop sys.boot_completed').catch(() => '')) === '1') return serial
    await sleep(1000)
  }
  throw new Error(`emulator ${avd} never finished booting`)
}

async function demoStatusBar(serial, on) {
  const demo = (command, extras = '') =>
    shell(serial, `am broadcast -a com.android.systemui.demo -e command ${command} ${extras}`)
  if (!on) return demo('exit')
  await shell(serial, 'settings put global sysui_demo_allowed 1')
  await demo('enter')
  await demo('clock', '-e hhmm 0941')
  await demo('battery', '-e level 100 -e plugged false')
  await demo('network', '-e wifi show -e level 4 -e fully true')
  await demo('network', '-e mobile hide')
  await demo('notifications', '-e visible false')
}

async function devtoolsSocket(serial) {
  for (let i = 0; i < 60; i++) {
    const pid = await shell(serial, `pidof ${appId}`).catch(() => '')
    const sockets = await shell(serial, 'cat /proc/net/unix')
    if (pid && sockets.includes(`webview_devtools_remote_${pid}`)) return `webview_devtools_remote_${pid}`
    await sleep(1000)
  }
  throw new Error(`no inspectable WebView in ${appId}; it needs a debug build`)
}

// The app's WebView only exposes DevTools in a debug build, so Playwright
// drives it over the forwarded socket while adb takes the screenshot, which
// is the only capture that includes the system bars.
export async function runAndroid(target, shoot) {
  const existing = process.env.ANDROID_SERIAL ?? (await onlineEmulators())[0]
  const serial = existing ?? (await bootEmulator(process.env.ANDROID_AVD ?? 'Pixel_10'))
  await adb(serial, 'install', '-r', process.env.ANDROID_APK ?? path.join(repo, 'bin/reManager.apk'))
  await shell(serial, `pm clear ${appId}`)
  await shell(serial, `pm grant ${appId} android.permission.POST_NOTIFICATIONS`)
  await shell(serial, `wm size ${target.screenSize}`)
  await demoStatusBar(serial, true)
  let browser
  let port
  try {
    await shell(serial, `am start -W -n ${appId}/com.wails.app.MainActivity`)
    const socket = await devtoolsSocket(serial)
    port = (await adb(serial, 'forward', 'tcp:0', `localabstract:${socket}`)).toString().trim()
    browser = await chromium.connectOverCDP(`http://127.0.0.1:${port}`)
    const page = browser.contexts()[0].pages().find(p => !p.url().startsWith('about:'))
    await shoot({
      evaluate: (fn, arg) => page.evaluate(fn, arg),
      tap: (x, y) => page.mouse.click(x, y),
      screenshot: async file => writeFile(file, await adb(serial, 'exec-out', 'screencap', '-p')),
    })
  } finally {
    await browser?.close()
    if (port) await adb(serial, 'forward', '--remove', `tcp:${port}`).catch(() => {})
    await demoStatusBar(serial, false)
    await shell(serial, 'wm size reset')
    if (!existing) await adb(serial, 'emu', 'kill')
  }
}
