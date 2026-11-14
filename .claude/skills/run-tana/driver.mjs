#!/usr/bin/env node
// Headless browser driver for the Tana UI. Reads one command per line from stdin.
//
//   node .claude/skills/run-tana/driver.mjs <<'EOF'
//   nav #/collector/favorites
//   shot favorites
//   EOF
//
// Selectors are Playwright selectors, e.g. role=button[name="Sync all"],
// text=Favorites page, [aria-label="Favorite sync"].
// Screenshots go to $TANA_RUN_DIR/shots (default /tmp/tana-run/shots).
import { createRequire } from 'node:module'
import { mkdirSync } from 'node:fs'
import readline from 'node:readline'

const run = process.env.TANA_RUN_DIR ?? '/tmp/tana-run'
const base = process.env.TANA_URL ?? 'http://127.0.0.1:8090/'
const shots = `${run}/shots`
mkdirSync(shots, { recursive: true })
const { chromium } = createRequire(`${run}/node/`)('playwright')

const browser = await chromium.launch({ executablePath: process.env.CHROMIUM ?? '/usr/bin/chromium' })
let page
const polls = []
let t0 = Date.now()
async function open(width = 1280, height = 900) {
  await page?.close()
  page = await browser.newPage({ viewport: { width, height } })
  page.on('request', (r) => { if (r.url().includes('/favorites/status')) polls.push(((Date.now() - t0) / 1000).toFixed(1)) })
  page.on('console', (m) => { if (m.type() === 'error') console.log('console error:', m.text()) })
}
await open()

const text = async (sel) => (await page.locator(sel).first().innerText()).replace(/\n+/g, ' | ')
const commands = {
  // size <width> <height>: new page at that viewport (phone: size 390 800)
  size: (w, h) => open(+w, h ? +h : 900),
  // nav <hash or path>: open the page and wait for the first data
  nav: async (path) => { t0 = Date.now(); polls.length = 0; await page.goto(base + (path ?? '')); await page.waitForTimeout(2000) },
  click: (...s) => page.locator(s.join(' ')).first().click(),
  check: (...s) => page.locator(s.join(' ')).first().check(),
  wait: (ms) => page.waitForTimeout(+ms),
  // text <selector>: print the element text, lines joined by " | "
  text: async (...s) => console.log(await text(s.join(' '))),
  // sidenav: print the sidenav quick action text
  sidenav: async () => console.log(await text('[aria-label="Favorite sync"]').catch(() => '(no quick action)')),
  // watch <seconds>: print the quick action text each time it changes
  watch: async (secs) => {
    let last = ''
    for (let i = 0; i < +secs; i++) {
      await page.waitForTimeout(1000)
      const now = await text('[aria-label="Favorite sync"]').catch(() => '(no quick action)')
      if (now !== last) console.log(`${((Date.now() - t0) / 1000).toFixed(0)}s ${now}`)
      last = now
    }
  },
  // polls: print the times (s since nav) of favorites status requests
  polls: async () => console.log('status polls at:', polls.join(' ')),
  // shot <name> [full]: screenshot to $TANA_RUN_DIR/shots/<name>.png
  shot: async (name, full) => { const path = `${shots}/${name}.png`; await page.screenshot({ path, fullPage: full === 'full' }); console.log('shot', path) },
  // eval <js expression>: print the result
  eval: async (...js) => console.log(await page.evaluate(js.join(' '))),
}

for await (const line of readline.createInterface({ input: process.stdin })) {
  const [cmd, ...args] = line.trim().split(/\s+/)
  if (!cmd || cmd.startsWith('#')) continue
  if (!commands[cmd]) { console.log(`unknown command: ${cmd}`); continue }
  try { await commands[cmd](...args) } catch (e) { console.log(`${cmd} failed: ${e.message.split('\n')[0]}`) }
}
await browser.close()
