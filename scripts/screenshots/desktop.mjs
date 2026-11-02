import { spawn } from 'node:child_process'
import { mkdir, mkdtemp, rm, writeFile } from 'node:fs/promises'
import net from 'node:net'
import os from 'node:os'
import path from 'node:path'
import { chromium, webkit } from 'playwright'
import { repo } from './shots.mjs'

const freePort = () =>
  new Promise((resolve, reject) => {
    const server = net.createServer().listen(0, () => {
      const { port } = server.address()
      server.close(() => resolve(port))
    })
    server.on('error', reject)
  })

async function waitForServer(url, child) {
  for (let i = 0; i < 120; i++) {
    if (child.exitCode !== null) throw new Error(`server exited with ${child.exitCode}`)
    try {
      if ((await fetch(url)).ok) return
    } catch {}
    await new Promise(resolve => setTimeout(resolve, 500))
  }
  throw new Error(`server at ${url} never answered`)
}

// Gives a capture the rounded corners and drop shadow of a macOS window
// capture, on a transparent margin so it sits on any page color. macOS draws a
// dark hairline around every window and, in dark mode, a light one just inside.
const shadowMargin = { top: 38, side: 56, bottom: 90 }
async function addWindowShadow(browser, png, viewport, colorScheme) {
  const innerEdge = colorScheme === 'dark' ? 'inset 0 0 0 0.5px rgba(255,255,255,.12)' : 'none'
  const page = await browser.newPage({
    viewport: { width: viewport.width + 2 * shadowMargin.side, height: viewport.height + shadowMargin.top + shadowMargin.bottom },
    deviceScaleFactor: 2,
  })
  try {
    await page.setContent(`<body style="margin:0;padding:${shadowMargin.top}px ${shadowMargin.side}px 0">
      <div style="position:relative;width:${viewport.width}px;border-radius:12px;box-shadow:0 0 0 0.5px rgba(0,0,0,.35),0 22px 50px rgba(0,0,0,.35)">
        <img style="display:block;width:100%;border-radius:12px" src="data:image/png;base64,${png.toString('base64')}">
        <div style="position:absolute;inset:0;border-radius:12px;box-shadow:${innerEdge}"></div>
      </div>`)
    return await page.screenshot({ omitBackground: true })
  } finally {
    await page.close()
  }
}

// A throwaway HOME keeps the server's saved devices and settings away from the
// real ones, and every target gets a fresh server because a connection
// outlives the page that made it.
export async function runDesktop(target, shoot) {
  const home = await mkdtemp(path.join(os.tmpdir(), 'remanager-screenshots-'))
  if (target.savedDevices) {
    const configDir = path.join(home, 'Library/Application Support/reManager')
    await mkdir(configDir, { recursive: true })
    await writeFile(path.join(configDir, 'devices.json'), JSON.stringify(target.savedDevices, null, 2))
  }
  const port = await freePort()
  const server = spawn(path.join(repo, 'bin/reManager-server'), [], {
    env: { ...process.env, HOME: home, WAILS_SERVER_PORT: String(port) },
    stdio: 'ignore',
  })
  const browser = await { chromium, webkit }[target.browser].launch()
  try {
    const url = `http://localhost:${port}/`
    await waitForServer(url, server)
    const page = await browser.newPage({ viewport: target.viewport, deviceScaleFactor: 2, colorScheme: target.colorScheme ?? 'light' })
    await page.goto(url)
    await shoot({
      evaluate: (fn, arg) => page.evaluate(fn, arg),
      tap: (x, y) => page.mouse.click(x, y),
      screenshot: async file => {
        if (!target.windowShadow) return page.screenshot({ path: file })
        await writeFile(file, await addWindowShadow(browser, await page.screenshot(), target.viewport, target.colorScheme))
      },
    })
  } finally {
    await browser.close()
    server.kill()
    const killer = setTimeout(() => server.kill('SIGKILL'), 3000)
    await new Promise(resolve => (server.exitCode !== null ? resolve() : server.once('exit', resolve)))
    clearTimeout(killer)
    await rm(home, { recursive: true, force: true })
  }
}
