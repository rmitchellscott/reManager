// Runs in the page on every platform, so it must stay self-contained: the
// driver serializes it, and it cannot close over anything in this module.
function locate({ role, pattern, flags }) {
  const selectors = {
    button: 'button,[role=button]',
    tab: '[role=tab]',
    switch: '[role=switch]',
    textbox: 'input,textarea',
    heading: 'h1,h2,h3,h4',
    text: 'body *',
  }
  const name = new RegExp(pattern, flags)
  const named = el => {
    const labels = [el.getAttribute('aria-label'), el.getAttribute('placeholder'), el.textContent, el.labels?.[0]?.textContent]
      .map(label => (label ?? '').replace(/\s+/g, ' ').trim())
      .filter(Boolean)
    return labels.length === 0 ? name.test('') : labels.some(label => name.test(label))
  }
  const hitTestable = el => {
    const rect = el.getBoundingClientRect()
    const x = rect.x + rect.width / 2
    const y = rect.y + rect.height / 2
    const top = document.elementFromPoint(x, y)
    return top !== null && (el === top || el.contains(top)) ? { x, y } : null
  }
  for (const el of document.querySelectorAll(selectors[role])) {
    if (role === 'text' && el.children.length > 0) continue
    if (!named(el)) continue
    const rect = el.getBoundingClientRect()
    if (rect.width === 0 || rect.height === 0) continue
    if (rect.bottom < 0 || rect.top > window.innerHeight) el.scrollIntoView({ block: 'center' })
    const point = hitTestable(el)
    if (point) return { ...point, disabled: el.disabled === true || el.getAttribute('aria-disabled') === 'true' }
  }
  return null
}

function locateInRow({ rowPattern, rowFlags, pattern, flags }) {
  const rowName = new RegExp(rowPattern, rowFlags)
  const name = new RegExp(pattern, flags)
  for (const label of document.querySelectorAll('body *')) {
    if (label.children.length > 0 || !rowName.test((label.textContent ?? '').trim())) continue
    if (label.getClientRects().length === 0) continue
    for (let row = label.parentElement; row && row !== document.body; row = row.parentElement) {
      const button = [...row.querySelectorAll('button')].find(b => name.test((b.textContent ?? '').trim()))
      if (!button) continue
      button.scrollIntoView({ block: 'center' })
      const rect = button.getBoundingClientRect()
      if (rect.width === 0 || rect.height === 0) break
      return { x: rect.x + rect.width / 2, y: rect.y + rect.height / 2, disabled: button.disabled }
    }
  }
  return null
}

// React tracks an input's value through the native setter, so assigning
// .value directly is invisible to it and the change is dropped.
function fillTextbox({ pattern, flags, value }) {
  const name = new RegExp(pattern, flags)
  for (const el of document.querySelectorAll('input,textarea')) {
    const names = [el.getAttribute('aria-label'), el.getAttribute('placeholder'), el.labels?.[0]?.textContent]
    if (!names.some(label => label && name.test(label.trim()))) continue
    const setter = Object.getOwnPropertyDescriptor(Object.getPrototypeOf(el), 'value').set
    setter.call(el, value)
    el.dispatchEvent(new Event('input', { bubbles: true }))
    el.blur()
    return true
  }
  return false
}

const asPattern = name =>
  name instanceof RegExp
    ? { pattern: name.source, flags: name.flags }
    : { pattern: `^${name.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')}$`, flags: 'i' }

const sleep = ms => new Promise(resolve => setTimeout(resolve, ms))

// tap takes CSS pixels; each driver maps them to its own input coordinates.
export function ui(driver, { timeout = 90_000, settle = 1200 } = {}) {
  const waitFor = async (role, name, { enabled = false, wait = timeout } = {}) => {
    const deadline = Date.now() + wait
    for (;;) {
      const found = await driver.evaluate(locate, { role, ...asPattern(name) })
      if (found && !(enabled && found.disabled)) return found
      if (Date.now() > deadline) throw new Error(`no ${role} named ${name} after ${wait / 1000}s`)
      await sleep(500)
    }
  }
  return {
    waitFor,
    async isShown(role, name) {
      return (await driver.evaluate(locate, { role, ...asPattern(name) })) !== null
    },
    async tap(role, name) {
      const { x, y } = await waitFor(role, name, { enabled: true })
      await driver.tap(x, y)
      await sleep(settle)
    },
    async tapInRow(rowName, name) {
      const row = asPattern(rowName)
      const deadline = Date.now() + timeout
      for (;;) {
        const found = await driver.evaluate(locateInRow, { rowPattern: row.pattern, rowFlags: row.flags, ...asPattern(name) })
        if (found && !found.disabled) {
          await driver.tap(found.x, found.y)
          await sleep(settle)
          return
        }
        if (Date.now() > deadline) throw new Error(`no ${name} button in the ${rowName} row`)
        await sleep(500)
      }
    },
    async fill(name, value) {
      const deadline = Date.now() + timeout
      while (!(await driver.evaluate(fillTextbox, { ...asPattern(name), value }))) {
        if (Date.now() > deadline) throw new Error(`no textbox named ${name}`)
        await sleep(500)
      }
      await sleep(settle)
    },
    sleep,
  }
}
