import fs from 'node:fs/promises'
import path from 'node:path'
import puppeteer, { type Browser, type Page } from 'puppeteer-core'

type Trace = {
  status?: string
  iterations?: number
  terminal_signal?: string
}

type CapturedResponse = {
  url: string
  status: number
  body: Record<string, unknown>
}

type NetworkEntry = {
  method: string
  url: string
  status?: number
}

const baseURL = process.env.KM_DOCS_BASE_URL ?? 'http://127.0.0.1:18081/'
const executablePath = process.env.PUPPETEER_EXECUTABLE_PATH ?? process.env.CHROME_BIN
const artifactDir = process.env.KM_DOCS_ARTIFACT_DIR ?? path.join(process.cwd(), 'e2e-artifacts')
const networkLog: NetworkEntry[] = []
const consoleLog: string[] = []

async function main() {
  if (!executablePath) {
    throw new Error('PUPPETEER_EXECUTABLE_PATH or CHROME_BIN is required')
  }
  await fs.mkdir(artifactDir, { recursive: true })
  const browser = await puppeteer.launch({
    executablePath,
    headless: true,
    args: ['--no-sandbox', '--disable-setuid-sandbox'],
  })
  try {
    await runProof(browser)
  } catch (err) {
    await writeFailure(browser, err)
    throw err
  } finally {
    await browser.close()
  }
}

async function runProof(browser: Browser) {
  const page = await browser.newPage()
  page.on('console', msg => consoleLog.push(`${msg.type()}: ${msg.text()}`))
  page.on('pageerror', err => consoleLog.push(`pageerror: ${err.message}`))
  page.on('request', req => networkLog.push({ method: req.method(), url: req.url() }))
  page.on('response', res => {
    const entry = networkLog.find(item => item.url === res.url() && item.status === undefined)
    if (entry) entry.status = res.status()
  })

  await page.goto(new URL('/docs', baseURL).href, { waitUntil: 'networkidle0' })
  await page.waitForSelector('.docs-category-toggle')

  const index = await fetchEndpoint(page, '/api/v1/docs')
  assert(index.status === 200, `index response status ${index.status}`)
  assert(Array.isArray(index.body.data), 'index data is an array')
  assert(hasTrace(index.body), 'index machine_request trace evidence present')

  await expandCategories(page)
  await page.waitForSelector('.docs-link-title')
  await clickDocument(page, 'SPECIFICATIONS')
  await page.waitForSelector('.doc-viewer')
  await page.waitForSelector('.doc-raw-section')

  const detail = await fetchEndpoint(page, '/api/v1/docs/SPECIFICATIONS.yaml')
  assert(detail.status === 200, `detail response status ${detail.status}`)
  assert(String(detail.body.raw ?? '').includes('id: agent-core-specifications'), 'raw YAML returned')
  assert(hasTrace(detail.body), 'detail machine_request trace evidence present')

  const rendered = await page.$eval('.doc-viewer', node => node.textContent ?? '')
  assert(rendered.includes('Agent Core Specification Index'), 'pretty view rendered title')

  await page.click('.doc-raw-section summary')
  const rawText = await page.$eval('.doc-raw-section', node => node.textContent ?? '')
  assert(rawText.includes('id: agent-core-specifications'), 'raw YAML view visible')

  await writeArtifacts('success', { index, detail })
}

async function expandCategories(page: Page) {
  await page.evaluate(() => {
    for (const button of document.querySelectorAll<HTMLButtonElement>('.docs-category-toggle')) {
      if (!button.classList.contains('expanded')) {
        button.click()
      }
    }
  })
}

async function clickDocument(page: Page, label: string) {
  const clicked = await page.evaluate((wanted: string) => {
    const buttons = Array.from(document.querySelectorAll<HTMLButtonElement>('.docs-link'))
    const button = buttons.find(item => item.textContent?.includes(wanted))
    button?.click()
    return Boolean(button)
  }, label)
  assert(clicked, `document button containing ${label} found`)
}

async function fetchEndpoint(page: Page, endpoint: string): Promise<CapturedResponse> {
  return page.evaluate(async target => {
    const response = await fetch(target)
    const body = await response.json() as Record<string, unknown>
    return { url: new URL(target, location.href).href, status: response.status, body }
  }, endpoint)
}

function hasTrace(body: Record<string, unknown>) {
  const trace = body.trace as Trace | undefined
  return typeof trace?.status === 'string' && typeof trace.iterations === 'number'
}

async function writeFailure(browser: Browser, err: unknown) {
  const pages = await browser.pages()
  const page = pages[pages.length - 1]
  if (page) {
    await page.screenshot({ path: path.join(artifactDir, 'failure.png'), fullPage: true })
    await fs.writeFile(path.join(artifactDir, 'page.html'), await page.content())
  }
  await writeArtifacts('failure', { error: err instanceof Error ? err.message : String(err), url: page?.url() })
}

async function writeArtifacts(kind: string, data: Record<string, unknown>) {
  await fs.writeFile(path.join(artifactDir, 'network.json'), JSON.stringify(networkLog, null, 2))
  await fs.writeFile(path.join(artifactDir, 'console.log'), consoleLog.join('\n'))
  await fs.writeFile(path.join(artifactDir, `${kind}.json`), JSON.stringify(data, null, 2))
}

function assert(condition: boolean, message: string): asserts condition {
  if (!condition) {
    throw new Error(message)
  }
}

await main()
