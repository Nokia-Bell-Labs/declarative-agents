import fs from 'node:fs/promises'
import path from 'node:path'
import puppeteer, { type Browser, type HTTPResponse, type Page } from 'puppeteer-core'

type Trace = {
  status?: string
  iterations?: number
  terminal_signal?: string
  server?: string
  route?: string
  machine?: string
}

type CapturedResponse = {
  method: string
  url: string
  requestId: string
  status: number
  body: Record<string, unknown>
}

type DocEntry = {
  path: string
  category?: string
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
const capturedResponses: CapturedResponse[] = []

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

  const indexResponse = waitForGET(page, '/api/v1/docs')
  await page.goto(new URL('/docs', baseURL).href, { waitUntil: 'networkidle0' })
  await page.waitForSelector('.docs-category-toggle')

  const index = await captureResponse(await indexResponse)
  assert(index.status === 200, `index response status ${index.status}`)
  assert(index.method === 'GET', 'index read used GET')
  assert(Array.isArray(index.body.data), 'index data is an array')
  assert(includesDocumentPath(index.body, 'specs/use-cases/rel03.0-uc007-machine-request-documentation-ux.yaml'), 'nested document path listed')
  assertMachineRequestTrace(index.body, 'documents', 'DocumentIndexReady')

  await expandCategories(page)
  const sidebarText = await page.$eval('.docs-sidebar', node => node.textContent ?? '')
  assert(sidebarText.includes('Semantic Models'), 'sidebar groups semantic models')
  assert(sidebarText.includes('Releases'), 'sidebar groups releases')
  assert(sidebarText.includes('Use Cases'), 'sidebar groups use cases')
  await page.waitForSelector('.docs-link-title')

  const walk = await walkDocumentation(page, index.body)

  const semanticResponse = waitForGET(page, '/api/v1/docs/specs/semantic-models/machine-request-http.yaml')
  await clickDocument(page, 'Machine Request Http')
  await page.waitForSelector('.semantic-model-figure .mermaid-container svg')
  const semantic = await captureResponse(await semanticResponse)
  assert(semantic.status === 200, `semantic model response status ${semantic.status}`)
  assertMachineRequestTrace(semantic.body, 'document', 'DocumentDetailReady')

  const detailResponse = waitForGET(page, '/api/v1/docs/SPECIFICATIONS.yaml')
  await clickDocument(page, 'SPECIFICATIONS')
  await page.waitForSelector('.doc-viewer')
  await page.waitForSelector('.doc-raw-section')

  const detail = await captureResponse(await detailResponse)
  assert(detail.status === 200, `detail response status ${detail.status}`)
  assert(detail.method === 'GET', 'detail read used GET')
  assert(String(detail.body.raw ?? '').includes('id: agent-core-specifications'), 'raw YAML returned')
  assertMachineRequestTrace(detail.body, 'document', 'DocumentDetailReady')
  assertNoActionPostsForReads()

  const rendered = await page.$eval('.doc-viewer', node => node.textContent ?? '')
  assert(rendered.includes('Agent Core Specification Index'), 'pretty view rendered title')

  await page.click('.doc-raw-section summary')
  const rawText = await page.$eval('.doc-raw-section', node => node.textContent ?? '')
  assert(rawText.includes('id: agent-core-specifications'), 'raw YAML view visible')

  const nestedPath = 'specs/use-cases/rel03.0-uc007-machine-request-documentation-ux.yaml'
  const nestedResponse = waitForGET(page, `/api/v1/docs/${nestedPath}`)
  await clickDocument(page, 'Machine Request Documentation Ux')
  await page.waitForFunction(
    () => document.querySelector('.doc-viewer')?.textContent?.includes('Knowledge Manager UX uses machine_request document resources')
  )
  const nested = await captureResponse(await nestedResponse)
  assert(nested.status === 200, `nested detail response status ${nested.status}`)
  assert(nested.method === 'GET', 'nested detail read used GET')
  assert(String(nested.body.raw ?? '').includes('id: rel03.0-uc007-machine-request-documentation-ux'), 'nested raw YAML returned')
  assertMachineRequestTrace(nested.body, 'document', 'DocumentDetailReady')

  const roadmapResponse = waitForGET(page, '/api/v1/docs/road-map.yaml')
  await clickDocument(page, 'Road Map')
  await page.waitForSelector('.roadmap-srd-table')
  const roadmapText = await page.$eval('.roadmap-srd-table', node => node.textContent ?? '')
  assert(roadmapText.includes('SRD Index'), 'roadmap SRD index rendered as table')
  assert(roadmapText.includes('srd027-rest-tool-runtime'), 'roadmap SRD table includes linked SRD entries')
  const roadmap = await captureResponse(await roadmapResponse)
  assert(roadmap.status === 200, `roadmap response status ${roadmap.status}`)
  assertMachineRequestTrace(roadmap.body, 'document', 'DocumentDetailReady')

  await writeArtifacts('success', { index, walk, semantic, detail, nested, roadmap })
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

async function walkDocumentation(page: Page, body: Record<string, unknown>) {
  const docs = indexDocuments(body)
  const visited: string[] = []
  for (const doc of docs) {
    const response = waitForGET(page, `/api/v1/docs/${documentPath(doc.path)}`)
    await clickDocumentPath(page, doc.path)
    await page.waitForFunction(
      expected => window.location.pathname === expected,
      {},
      `/docs/${documentPath(doc.path)}`
    )
    const detail = await captureResponse(await response)
    assert(detail.status === 200, `walk ${doc.path} response status ${detail.status}`)
    assert(String(detail.body.raw ?? '').length > 0, `walk ${doc.path} raw YAML returned`)
    assertMachineRequestTrace(detail.body, 'document', 'DocumentDetailReady')
    await page.waitForSelector('.doc-viewer')
    await page.waitForSelector('.doc-raw-section')
    visited.push(doc.path)
  }
  return { count: visited.length, paths: visited }
}

function indexDocuments(body: Record<string, unknown>): DocEntry[] {
  const docs = body.data
  assert(Array.isArray(docs), 'walk source document index is an array')
  return docs.map((doc, i) => {
    assert(typeof doc === 'object' && doc !== null, `walk index entry ${i} is an object`)
    const entry = doc as Record<string, unknown>
    assert(typeof entry.path === 'string', `walk index entry ${i} has a path`)
    return {
      path: entry.path,
      category: typeof entry.category === 'string' ? entry.category : undefined,
    }
  })
}

async function clickDocumentPath(page: Page, path: string) {
  const route = `/docs/${documentPath(path)}`
  const clicked = await page.evaluate((wantedRoute: string, wantedPath: string) => {
    const links = Array.from(document.querySelectorAll<HTMLAnchorElement>('.docs-link'))
    const link = links.find(item => {
      const route = new URL(item.href).pathname
      return route === wantedRoute || decodeURIComponent(route) === `/docs/${wantedPath}`
    })
    link?.click()
    return Boolean(link)
  }, route, path)
  assert(clicked, `document link for ${path} found`)
}

function documentPath(path: string): string {
  return path.split('/').map(encodeURIComponent).join('/')
}

function waitForGET(page: Page, endpoint: string): Promise<HTTPResponse> {
  const expected = new URL(endpoint, baseURL).href
  return page.waitForResponse(res => res.request().method() === 'GET' && res.url() === expected)
}

async function captureResponse(response: HTTPResponse): Promise<CapturedResponse> {
  const body = await response.json() as Record<string, unknown>
  const captured = {
    method: response.request().method(),
    url: response.url(),
    requestId: `km-docs-${Date.now()}-${capturedResponses.length}`,
    status: response.status(),
    body,
  }
  capturedResponses.push(captured)
  return captured
}

function assertNoActionPostsForReads() {
  const actionPosts = networkLog.filter(item => {
    const url = new URL(item.url)
    return item.method === 'POST' && url.pathname === '/api/v1/actions'
  })
  assert(actionPosts.length === 0, 'browser did not POST /api/v1/actions for document reads')
}

function includesDocumentPath(body: Record<string, unknown>, path: string) {
  const docs = body.data as Array<Record<string, unknown>> | undefined
  return Array.isArray(docs) && docs.some(doc => doc.path === path)
}

function assertMachineRequestTrace(body: Record<string, unknown>, route: string, signal: string) {
  const trace = body.trace as Trace | undefined
  assert(typeof trace?.status === 'string', `${route} trace status present`)
  assert(typeof trace.iterations === 'number', `${route} trace iterations present`)
  assert(trace?.server === 'documentation_curator_requests', `${route} trace server present`)
  assert(trace.route === route, `${route} trace route present`)
  assert(trace.machine === 'documentation-curator-request', `${route} trace machine present`)
  assert(trace.terminal_signal === signal, `${route} terminal signal present`)
}

async function writeFailure(browser: Browser, err: unknown) {
  const pages = await browser.pages()
  const page = pages[pages.length - 1]
  if (page) {
    await page.screenshot({ path: path.join(artifactDir, 'failure.png'), fullPage: true })
    await fs.writeFile(path.join(artifactDir, 'page.html'), await page.content())
  }
  await writeArtifacts('failure', {
    error: err instanceof Error ? err.message : String(err),
    url: page?.url(),
    responses: capturedResponses,
  })
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
