// Captures store and guide screenshots in demo mode.
//
//   node run.mjs [target...]   targets: iphone ipad duo-outer duo-inner play microsoft flathub mac readme-light readme-dark guide
//                              (default: all)
//   OUT=<dir> node run.mjs ... writes to <dir>/<target> instead of the repo
import { mkdir } from 'node:fs/promises'
import path from 'node:path'
import { targets, tour } from './shots.mjs'
import { ui } from './ui.mjs'

const runners = {
  desktop: async () => (await import('./desktop.mjs')).runDesktop,
  android: async () => (await import('./android.mjs')).runAndroid,
  ios: async () => (await import('./ios.mjs')).runIos,
}
const finished = Symbol('finished')

const names = process.argv.slice(2)
for (const name of names.length > 0 ? names : Object.keys(targets)) {
  const target = targets[name]
  if (!target) throw new Error(`unknown target ${name}; known: ${Object.keys(targets).join(' ')}`)
  const dir = process.env.OUT ? path.resolve(process.env.OUT, name) : target.dir
  await mkdir(dir, { recursive: true })
  const remaining = new Set(Object.keys(target.files))
  console.log(`${name}: starting`)
  const run = await runners[target.platform]()
  await run(target, async driver => {
    try {
      await tour(ui(driver), async shot => {
        if (!remaining.delete(shot)) return
        const file = path.join(dir, target.files[shot])
        await driver.screenshot(file)
        console.log(`${name}: ${path.relative(process.cwd(), file)}`)
        if (remaining.size === 0) throw finished
      }, shot => remaining.has(shot))
    } catch (err) {
      if (err === finished) return
      const file = path.join(dir, 'failure.png')
      await driver.screenshot(file).catch(() => {})
      console.error(`${name}: failed, the screen at the time is ${file}`)
      throw err
    }
  })
  if (remaining.size > 0) throw new Error(`${name}: never reached ${[...remaining].join(', ')}`)
}
