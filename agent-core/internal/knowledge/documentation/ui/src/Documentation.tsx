import { useState, useEffect, useRef, useCallback, useMemo, type ReactNode } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import {
  listDocs, getDoc, getConfig, getSource, getUXConfig, validateDocs, suggestDocChanges,
  approvePatch, rejectPatch,
  type DocEntry, type DocDetail, type ConfigDetail, type SourceDetail,
  type UXConfig, type ValidationReport, type SuggestionResponse, type PatchDecision,
} from './apiClient'
import mermaid from 'mermaid'
import hljs from 'highlight.js/lib/core'
import yaml from 'highlight.js/lib/languages/yaml'
import 'highlight.js/styles/github.css'

hljs.registerLanguage('yaml', yaml)

mermaid.initialize({ startOnLoad: false, theme: 'default', securityLevel: 'loose' })

// --- Constants ---

const GITLAB_BASE = 'https://gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/-/tree/master'
const GITLAB_BLOB = 'https://gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/-/blob/master'

const CODE_PATH_RE = /(?:pkg|cmd)\/[\w-]+(?:\/[\w.-]+)*/
const CONFIG_PATH_RE = /configs\/[\w-]+(?:\/[\w.*-]+)*/

const CATEGORY_ICONS: Record<string, string> = {
  overview: '📋',
  release: '🗺',
  'semantic-model': '🔀',
  'config-format': '⚙',
  srd: '📐',
  'use-case': '👤',
  'test-suite': '🧪',
}

const DEFAULT_CATEGORY_META: Record<string, { label: string; order: number }> = {
  overview:         { label: 'Overview',              order: 0 },
  release:          { label: 'Releases',              order: 1 },
  'semantic-model': { label: 'Semantic Models',       order: 2 },
  'config-format':  { label: 'Config Formats',        order: 3 },
  srd:              { label: 'Software Requirements', order: 4 },
  'use-case':       { label: 'Use Cases',             order: 5 },
  'test-suite':     { label: 'Test Suites',           order: 6 },
}

function categoryMeta(cat: string, uxConfig: UXConfig | null) {
  const configured = uxConfig?.sidebar.groups[cat] ?? DEFAULT_CATEGORY_META[cat]
  return configured ? { ...configured, icon: CATEGORY_ICONS[cat] ?? '📄' } : {
    label: cat.replace(/-/g, ' ').replace(/\b\w/g, c => c.toUpperCase()),
    icon: '📄',
    order: 99,
  }
}

function formatSidebarTitle(entry: DocEntry): { prefix: string; title: string } {
  const name = entry.name
  const srdMatch = name.match(/^(srd\d+)-(.+)$/)
  if (srdMatch) {
    const title = srdMatch[2].replace(/-/g, ' ').replace(/\b\w/g, c => c.toUpperCase())
    return { prefix: srdMatch[1].toUpperCase(), title }
  }
  const smMatch = name.match(/^(sm-)?(.+)$/)
  if (entry.category === 'semantic-model' && smMatch) {
    const title = (smMatch[2] ?? name).replace(/-/g, ' ').replace(/\b\w/g, c => c.toUpperCase())
    return { prefix: '', title }
  }
  const title = name
    .replace(/^rel[\d.]+-/, '')
    .replace(/^test-/, '')
    .replace(/-/g, ' ')
    .replace(/\b\w/g, c => c.toUpperCase())
  return { prefix: '', title }
}

// --- Types ---

interface SemanticState {
  name: string
  meaning: string
}

interface RoadmapRelease {
  version: string
  name?: string
  status?: string
  srds?: string[]
}

// --- Cross-document + code linking ---

type DocIndex = Map<string, string>

function buildDocIndex(docs: DocEntry[]): DocIndex {
  const index: DocIndex = new Map()
  for (const d of docs) {
    index.set(d.name, d.path)
    index.set(d.path, d.path)
    index.set(`docs/${d.path}`, d.path)

    const srdMatch = d.name.match(/^(srd\d+)/)
    if (srdMatch) index.set(srdMatch[1], d.path)

    const smId = d.name.match(/^(sm-\w+)/)
    if (smId) index.set(smId[1], d.path)
  }
  return index
}

function buildLinkPattern(index: DocIndex): RegExp {
  const docKeys = Array.from(index.keys())
    .sort((a, b) => b.length - a.length)
    .map(k => k.replace(/[.*+?^${}()|[\]\\]/g, '\\$&'))

  const parts = [
    CONFIG_PATH_RE.source,
    CODE_PATH_RE.source,
    ...docKeys,
  ]
  return new RegExp(`(${parts.join('|')})`, 'g')
}

function documentURL(path: string): string {
  return `/docs/${path.split('/').map(encodeURIComponent).join('/')}`
}

function decodeRoutePath(path: string | undefined): string | null {
  if (!path) return null
  return path.split('/').map(segment => {
    try {
      return decodeURIComponent(segment)
    } catch {
      return segment
    }
  }).join('/')
}

function linkifyText(
  text: string,
  pattern: RegExp,
  index: DocIndex,
  navigate: (path: string) => void,
  onConfigClick?: (configPath: string) => void,
  onSourceClick?: (sourcePath: string) => void,
): ReactNode[] {
  const parts: ReactNode[] = []
  let lastIndex = 0
  let match: RegExpExecArray | null
  const re = new RegExp(pattern.source, 'g')

  while ((match = re.exec(text)) !== null) {
    if (match.index > lastIndex) {
      parts.push(text.slice(lastIndex, match.index))
    }
    const matched = match[1]
    const docTarget = index.get(matched)
    if (docTarget) {
      parts.push(
        <a
          key={`d-${match.index}`}
          href={documentURL(docTarget)}
          className="doc-crosslink"
          onClick={e => { e.preventDefault(); navigate(docTarget) }}
        >
          {matched}
        </a>
      )
    } else if (CONFIG_PATH_RE.test(matched)) {
      const isFile = /\.\w+$/.test(matched)
      const gitlabUrl = `${isFile ? GITLAB_BLOB : GITLAB_BASE}/${matched}`
      parts.push(
        <a
          key={`cfg-${match.index}`}
          href={gitlabUrl}
          className="doc-configlink"
          target="_blank"
          rel="noopener noreferrer"
          onClick={onConfigClick && isFile ? (e => { e.preventDefault(); onConfigClick(matched) }) : undefined}
        >
          {matched}
        </a>
      )
    } else if (CODE_PATH_RE.test(matched)) {
      const isFile = /\.\w+$/.test(matched)
      const gitlabUrl = `${isFile ? GITLAB_BLOB : GITLAB_BASE}/${matched}`
      parts.push(
        <a
          key={`c-${match.index}`}
          href={gitlabUrl}
          className="doc-codelink"
          target="_blank"
          rel="noopener noreferrer"
          onClick={onSourceClick && isFile ? (e => { e.preventDefault(); onSourceClick(matched) }) : undefined}
        >
          {matched}
        </a>
      )
    } else {
      parts.push(matched)
    }
    lastIndex = re.lastIndex
  }
  if (lastIndex < text.length) {
    parts.push(text.slice(lastIndex))
  }
  return parts
}

// --- Inline mermaid ---

interface TextSegment {
  type: 'text' | 'mermaid'
  content: string
}

function splitMermaidBlocks(text: string): TextSegment[] {
  const segments: TextSegment[] = []
  const re = /```mermaid\s*\n([\s\S]*?)```/g
  let lastIndex = 0
  let match: RegExpExecArray | null

  while ((match = re.exec(text)) !== null) {
    if (match.index > lastIndex) {
      segments.push({ type: 'text', content: text.slice(lastIndex, match.index) })
    }
    segments.push({ type: 'mermaid', content: match[1].trim() })
    lastIndex = re.lastIndex
  }
  if (lastIndex < text.length) {
    segments.push({ type: 'text', content: text.slice(lastIndex) })
  }
  return segments
}

// --- Auto state diagram ---

function extractStateDiagram(content: Record<string, unknown>): string | null {
  const states = content.states as SemanticState[] | undefined
  if (!states || !Array.isArray(states) || states.length === 0) return null

  const lines: string[] = ['stateDiagram-v2']
  const stateNames = states.map(s => s.name)
  const terminalStates = new Set<string>()

  for (const s of states) {
    if ((s.meaning ?? '').toLowerCase().includes('terminal')) terminalStates.add(s.name)
  }

  const lifecycle = content.lifecycle as string | undefined
  if (!lifecycle) return null

  const transitionRe = /(\w+)\s*→\s*(\w+)(?:\s*\([^)]*\))?:\s*(.+)/g
  let match: RegExpExecArray | null
  const transitions: Array<{ from: string; to: string; label: string }> = []

  while ((match = transitionRe.exec(lifecycle)) !== null) {
    const label = match[3].trim()
      .replace(/[""]/g, '')
      .replace(/—/g, '-')
      .replace(/[()]/g, '')
    transitions.push({ from: match[1], to: match[2], label })
  }
  if (transitions.length === 0) return null

  const initial = stateNames[0]
  if (initial) lines.push(`  [*] --> ${initial}`)
  for (const t of transitions) lines.push(`  ${t.from} --> ${t.to} : ${t.label}`)
  for (const t of terminalStates) lines.push(`  ${t} --> [*]`)

  return lines.join('\n')
}

function extractSemanticModelFigure(content: Record<string, unknown>): string | null {
  const explicit = firstMermaidDiagram(content.diagrams)
  if (explicit) return explicit
  return relationshipFlowchart(content.relationships)
}

function firstMermaidDiagram(value: unknown): string | null {
  if (typeof value === 'string') {
    return firstMermaidBlock(value)
  }
  if (Array.isArray(value)) {
    for (const item of value) {
      const chart = firstMermaidDiagram(item)
      if (chart) return chart
    }
    return null
  }
  if (typeof value === 'object' && value !== null) {
    for (const item of Object.values(value as Record<string, unknown>)) {
      const chart = firstMermaidDiagram(item)
      if (chart) return chart
    }
  }
  return null
}

function firstMermaidBlock(text: string): string | null {
  const match = text.match(/```mermaid\s*\n([\s\S]*?)```/)
  return match ? match[1].trim() : null
}

function relationshipFlowchart(value: unknown): string | null {
  if (typeof value !== 'object' || value === null || Array.isArray(value)) return null
  const edges = Object.keys(value as Record<string, unknown>)
    .map(relationshipEdge)
    .filter((edge): edge is [string, string] => edge !== null)
  if (edges.length === 0) return null
  return ['flowchart LR', ...edges.map(([from, to]) => `  ${nodeID(from)}[${nodeLabel(from)}] --> ${nodeID(to)}[${nodeLabel(to)}]`)].join('\n')
}

function relationshipEdge(name: string): [string, string] | null {
  const [from, to] = name.split('_to_')
  if (!from || !to) return null
  return [from, to]
}

function nodeID(value: string): string {
  return value.replace(/[^A-Za-z0-9]/g, '_')
}

function nodeLabel(value: string): string {
  return value.replace(/_/g, ' ')
}

// --- Components ---

function MermaidDiagram({ chart }: { chart: string }) {
  const ref = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!ref.current) return
    const id = `mermaid-${Date.now()}-${Math.random().toString(36).slice(2, 7)}`
    ref.current.innerHTML = ''
    mermaid.render(id, chart).then(({ svg }) => {
      if (ref.current) ref.current.innerHTML = svg
    }).catch((err: unknown) => {
      if (!ref.current) return
      const msg = err instanceof Error ? err.message : String(err)
      console.error('Mermaid render error:', msg, '\nChart:\n', chart)
      ref.current.innerHTML = `<pre class="mermaid-error">${msg}\n\n--- source ---\n${chart.replace(/</g, '&lt;')}</pre>`
    })
  }, [chart])

  return <div ref={ref} className="mermaid-container" />
}

function LinkedText({ text, pattern, index, navigate, onSourceClick }: {
  text: string
  pattern: RegExp
  index: DocIndex
  navigate: (path: string) => void
  onSourceClick?: (sourcePath: string) => void
}) {
  const segments = splitMermaidBlocks(text)
  return (
    <>
      {segments.map((seg, i) =>
        seg.type === 'mermaid'
          ? <MermaidDiagram key={i} chart={seg.content} />
          : <span key={i}>{linkifyText(seg.content, pattern, index, navigate, undefined, onSourceClick)}</span>
      )}
    </>
  )
}

function ProseBlock({ text, pattern, index, navigate }: {
  text: string
  pattern: RegExp
  index: DocIndex
  navigate: (path: string) => void
}) {
  const segments = splitMermaidBlocks(text)
  return (
    <div className="doc-text">
      {segments.map((seg, i) =>
        seg.type === 'mermaid'
          ? <MermaidDiagram key={i} chart={seg.content} />
          : <ProseText key={i} text={seg.content} pattern={pattern} index={index} navigate={navigate} />
      )}
    </div>
  )
}

function ProseText({ text, pattern, index, navigate }: {
  text: string
  pattern: RegExp
  index: DocIndex
  navigate: (path: string) => void
}) {
  const blocks = text.split(/\n\s*\n/).map(block => block.trim()).filter(Boolean)
  return (
    <>
      {blocks.map((block, i) =>
        isBulletBlock(block)
          ? <BulletList key={i} block={block} pattern={pattern} index={index} navigate={navigate} />
          : (
            <p key={i} className="doc-paragraph">
              {linkifyText(reflowText(block), pattern, index, navigate)}
            </p>
          )
      )}
    </>
  )
}

function BulletList({ block, pattern, index, navigate }: {
  block: string
  pattern: RegExp
  index: DocIndex
  navigate: (path: string) => void
}) {
  return (
    <ul className="doc-bullet-list">
      {bulletItems(block).map((item, i) => (
        <li key={i}>{linkifyText(item, pattern, index, navigate)}</li>
      ))}
    </ul>
  )
}

function isBulletBlock(block: string): boolean {
  return block.split('\n').some(line => /^\s*-\s+/.test(line))
}

function bulletItems(block: string): string[] {
  const items: string[] = []
  for (const line of block.split('\n')) {
    const bullet = line.match(/^\s*-\s+(.*)$/)
    if (bullet) {
      items.push(bullet[1].trim())
    } else if (items.length > 0) {
      items[items.length - 1] = `${items[items.length - 1]} ${line.trim()}`.trim()
    }
  }
  return items
}

function reflowText(text: string): string {
  return text.replace(/\s*\n\s*/g, ' ').replace(/\s+/g, ' ').trim()
}

function YamlSection({ label, value, pattern, index, onNavigate }: {
  label: string
  value: unknown
  pattern: RegExp
  index: DocIndex
  onNavigate: (path: string) => void
}) {
  if (value === null || value === undefined) return null

  if (typeof value === 'string') {
    return (
      <div className="doc-section">
        <h3>{label}</h3>
        <ProseBlock text={value} pattern={pattern} index={index} navigate={onNavigate} />
      </div>
    )
  }

  if (Array.isArray(value)) {
    if (label === 'Releases' && isRoadmapReleaseArray(value)) {
      return <RoadmapReleasesTable releases={value} pattern={pattern} index={index} onNavigate={onNavigate} />
    }

    const objects = value.filter((item): item is Record<string, unknown> =>
      typeof item === 'object' && item !== null && !Array.isArray(item)
    )

    let isUniformTable = false
    let columns: string[] = []
    let isKvTable = false

    if (objects.length === value.length && objects.length >= 2) {
      const keySets = objects.map(o => Object.keys(o).sort().join(','))
      const allSameSchema = keySets.every(k => k === keySets[0])
      const keyCount = Object.keys(objects[0]).length

      if (allSameSchema && keyCount >= 2) {
        isUniformTable = true
        columns = Object.keys(objects[0])
      } else if (keyCount === 1) {
        isKvTable = true
      }
    }

    if (isUniformTable) {
      return (
        <div className="doc-section">
          <h3>{label}</h3>
          <div className="table-container">
            <table className="doc-table">
              <thead>
                <tr>
                  {columns.map(col => (
                    <th key={col}>{col}</th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {objects.map((obj, i) => (
                  <tr key={i}>
                    {columns.map(col => {
                      const v = obj[col]
                      return (
                        <td key={col}>
                          {v === undefined || v === null
                            ? ''
                            : typeof v === 'string'
                              ? <LinkedText text={v} pattern={pattern} index={index} navigate={onNavigate} />
                              : Array.isArray(v)
                                ? v.map(String).join(', ')
                                : String(v)}
                        </td>
                      )
                    })}
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )
    }

    if (isKvTable) {
      const rows = objects.map(obj => {
        const [k, v] = Object.entries(obj)[0]
        return { key: k, value: v }
      })
      const allSameKey = rows.every(r => r.key === rows[0].key)

      return (
        <div className="doc-section">
          <h3>{label}</h3>
          <div className="table-container">
            <table className="doc-table">
              <thead>
                <tr>
                  <th>{allSameKey ? 'status' : 'key'}</th>
                  <th>description</th>
                </tr>
              </thead>
              <tbody>
                {rows.map((row, i) => (
                  <tr key={i}>
                    <td><span className={`doc-kv-badge doc-kv-${row.key.toLowerCase()}`}>{row.key}</span></td>
                    <td>
                      {typeof row.value === 'string'
                        ? <LinkedText text={row.value} pattern={pattern} index={index} navigate={onNavigate} />
                        : String(row.value)}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )
    }

    return (
      <div className="doc-section">
        <h3>{label}</h3>
        {value.map((item, i) => {
          if (typeof item === 'string') {
            return (
              <div key={i} className="doc-list-item">
                • <LinkedText text={item} pattern={pattern} index={index} navigate={onNavigate} />
              </div>
            )
          }
          if (typeof item === 'object' && item !== null) {
            const obj = item as Record<string, unknown>
            return (
              <div key={i} className="doc-card">
                {Object.entries(obj).map(([k, v]) => (
                  <div key={k} className="doc-card-field">
                    <span className="doc-card-label">{k}:</span>{' '}
                    <span className="doc-card-value">
                      {typeof v === 'string'
                        ? <LinkedText text={v} pattern={pattern} index={index} navigate={onNavigate} />
                        : Array.isArray(v)
                          ? v.map(String).join(', ')
                          : String(v)}
                    </span>
                  </div>
                ))}
              </div>
            )
          }
          return <div key={i}>{JSON.stringify(item)}</div>
        })}
      </div>
    )
  }

  if (typeof value === 'object') {
    const obj = value as Record<string, unknown>
    return (
      <div className="doc-section">
        <h3>{label}</h3>
        <div className="doc-nested">
          {Object.entries(obj).map(([k, v]) => (
            <YamlSection key={k} label={k} value={v} pattern={pattern} index={index} onNavigate={onNavigate} />
          ))}
        </div>
      </div>
    )
  }

  return (
    <div className="doc-section">
      <h3>{label}</h3>
      <span className="doc-value">{String(value)}</span>
    </div>
  )
}

function SidebarCategory({ cat, entries, selectedPath, onSelect, uxConfig }: {
  cat: string
  entries: DocEntry[]
  selectedPath: string | null
  onSelect: (path: string) => void
  uxConfig: UXConfig | null
}) {
  const hasActive = entries.some(d => d.path === selectedPath)
  const [open, setOpen] = useState(hasActive)
  const meta = categoryMeta(cat, uxConfig)

  useEffect(() => {
    if (hasActive) setOpen(true)
  }, [hasActive])

  return (
    <div className="docs-category">
      <button className={`docs-category-toggle ${open ? 'open' : ''}`} onClick={() => setOpen(!open)}>
        <span className="docs-category-icon">{meta.icon}</span>
        <span className="docs-category-label">{meta.label}</span>
        <span className="docs-category-count">{entries.length}</span>
        <span className="docs-category-chevron">{open ? '▾' : '▸'}</span>
      </button>
      {open && (
        <ul className="docs-list">
          {entries.map(d => {
            const { prefix, title } = formatSidebarTitle(d)
            return (
              <li key={d.path}>
                <a
                  href={documentURL(d.path)}
                  className={`docs-link ${selectedPath === d.path ? 'docs-link-active' : ''}`}
                  onClick={e => { e.preventDefault(); onSelect(d.path) }}
                >
                  {prefix && <span className="docs-link-prefix">{prefix}</span>}
                  <span className="docs-link-title">{title}</span>
                </a>
              </li>
            )
          })}
        </ul>
      )}
    </div>
  )
}

function isRoadmapReleaseArray(value: unknown[]): value is RoadmapRelease[] {
  return value.every(item => {
    if (typeof item !== 'object' || item === null || Array.isArray(item)) return false
    const release = item as Record<string, unknown>
    return typeof release.version === 'string' && Array.isArray(release.srds)
  })
}

function RoadmapReleasesTable({ releases, pattern, index, onNavigate }: {
  releases: RoadmapRelease[]
  pattern: RegExp
  index: DocIndex
  onNavigate: (path: string) => void
}) {
  return (
    <div className="doc-section">
      <h3>Releases</h3>
      <div className="table-container">
        <table className="doc-table roadmap-srd-table">
          <thead>
            <tr>
              <th>Release</th>
              <th>Name</th>
              <th>Status</th>
              <th>SRD Index</th>
            </tr>
          </thead>
          <tbody>
            {releases.map(release => (
              <tr key={release.version}>
                <td><span className="doc-kv-badge">{release.version}</span></td>
                <td>{release.name ?? ''}</td>
                <td>
                  {release.status && (
                    <span className={`doc-kv-badge doc-kv-${release.status.replace(/_/g, '-')}`}>
                      {release.status}
                    </span>
                  )}
                </td>
                <td>
                  <div className="roadmap-srd-list">
                    {(release.srds ?? []).map(srd => (
                      <span key={srd} className="roadmap-srd-item">
                        <LinkedText text={srd} pattern={pattern} index={index} navigate={onNavigate} />
                      </span>
                    ))}
                  </div>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  )
}

// --- Requirements section for SRDs ---

export function isRequirementsBlock(value: unknown): value is Record<string, { title: string; items: Array<Record<string, string>> }> {
  if (typeof value !== 'object' || value === null || Array.isArray(value)) return false
  const entries = Object.entries(value as Record<string, unknown>)
  if (entries.length === 0) return false
  return entries.every(([key, v]) => {
    if (!/^R\d+/.test(key)) return false
    if (typeof v !== 'object' || v === null) return false
    const obj = v as Record<string, unknown>
    return typeof obj.title === 'string' && Array.isArray(obj.items)
  })
}

export function RequirementsSection({ requirements, pattern, index, onNavigate }: {
  requirements: Record<string, { title: string; items: Array<Record<string, string>> }>
  pattern: RegExp
  index: DocIndex
  onNavigate: (path: string) => void
}) {
  const groups = Object.entries(requirements).sort(([a], [b]) => {
    const na = parseInt(a.replace(/^R/, ''), 10)
    const nb = parseInt(b.replace(/^R/, ''), 10)
    return na - nb
  })

  return (
    <div className="doc-section">
      <h3>Requirements</h3>
      {groups.map(([groupId, group]) => (
        <div key={groupId} className="requirements-group">
          <h4 className="requirements-group-title">
            <span className="requirements-group-badge">{groupId}</span>
            {group.title}
          </h4>
          <div className="table-container">
            <table className="doc-table">
              <thead>
                <tr>
                  <th>ID</th>
                  <th>Requirement</th>
                </tr>
              </thead>
              <tbody>
                {group.items.map((item, i) => {
                  const [id, desc] = Object.entries(item)[0] ?? ['', '']
                  return (
                    <tr key={i}>
                      <td><span className="requirements-id-badge">{id}</span></td>
                      <td>
                        <LinkedText text={String(desc)} pattern={pattern} index={index} navigate={onNavigate} />
                      </td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          </div>
        </div>
      ))}
    </div>
  )
}

// --- Configuration block for semantic models ---

function ConfigurationBlock({ config, onConfigClick }: {
  config: Record<string, unknown>
  onConfigClick: (configPath: string) => void
}) {
  const rows: Array<{ label: string; paths: string[] }> = []

  for (const [key, val] of Object.entries(config)) {
    if (typeof val === 'string') {
      rows.push({ label: key, paths: [val] })
    } else if (Array.isArray(val)) {
      rows.push({ label: key, paths: val.filter((v): v is string => typeof v === 'string') })
    }
  }

  return (
    <div className="doc-section">
      <h3>Configuration</h3>
      <div className="table-container">
        <table className="doc-table config-block-table">
          <thead>
            <tr>
              <th>Type</th>
              <th>Path</th>
            </tr>
          </thead>
          <tbody>
            {rows.map(row => (
              <tr key={row.label}>
                <td>
                  <span className="config-type-badge">{row.label}</span>
                </td>
                <td>
                  {row.paths.map((p, i) => {
                    const isFile = /\.\w+$/.test(p)
                    return (
                      <div key={i} className="config-path-row">
                        <a
                          href={`${isFile ? GITLAB_BLOB : GITLAB_BASE}/${p}`}
                          className="doc-configlink"
                          onClick={isFile ? (e => {
                            e.preventDefault()
                            onConfigClick(p)
                          }) : undefined}
                        >
                          {p}
                        </a>
                      </div>
                    )
                  })}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  )
}

function HighlightedYaml({ raw, className }: { raw: string; className?: string }) {
  const ref = useRef<HTMLPreElement>(null)

  useEffect(() => {
    if (ref.current) {
      const result = hljs.highlight(raw, { language: 'yaml' })
      ref.current.innerHTML = result.value
    }
  }, [raw])

  return <pre ref={ref} className={`${className ?? 'config-yaml-view'} hljs`} />
}

// --- Model/Definition toggle for diagrams ---

function ModelDefinitionToggle({ chart, machinePath }: {
  chart: string
  machinePath: string | null
}) {
  const [tab, setTab] = useState<'model' | 'definition'>('model')
  const [configData, setConfigData] = useState<ConfigDetail | null>(null)
  const [configLoading, setConfigLoading] = useState(false)
  const [configError, setConfigError] = useState<string | null>(null)

  useEffect(() => {
    if (tab === 'definition' && machinePath && !configData) {
      setConfigLoading(true)
      setConfigError(null)
      const apiPath = machinePath.replace(/^configs\//, '')
      getConfig(apiPath)
        .then(setConfigData)
        .catch(() => setConfigError('Failed to load machine definition'))
        .finally(() => setConfigLoading(false))
    }
  }, [tab, machinePath, configData])

  if (!machinePath) {
    return <MermaidDiagram chart={chart} />
  }

  return (
    <div className="model-def-toggle">
      <div className="model-def-tabs">
        <button
          className={`model-def-tab ${tab === 'model' ? 'model-def-tab-active' : ''}`}
          onClick={() => setTab('model')}
        >
          Model
        </button>
        <button
          className={`model-def-tab ${tab === 'definition' ? 'model-def-tab-active' : ''}`}
          onClick={() => setTab('definition')}
        >
          Definition
        </button>
        <span className="model-def-path">{machinePath}</span>
      </div>
      <div className="model-def-content">
        {tab === 'model' && <MermaidDiagram chart={chart} />}
        {tab === 'definition' && (
          <>
            {configLoading && <div className="loading">Loading definition...</div>}
            {configError && <div className="error">{configError}</div>}
            {configData && <HighlightedYaml raw={configData.raw} />}
          </>
        )}
      </div>
    </div>
  )
}

// --- Config file viewer overlay ---

function ConfigViewer({ configPath, onClose }: {
  configPath: string
  onClose: () => void
}) {
  const [data, setData] = useState<ConfigDetail | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    setLoading(true)
    setError(null)
    const apiPath = configPath.replace(/^configs\//, '')
    getConfig(apiPath)
      .then(setData)
      .catch(() => setError('Failed to load config file'))
      .finally(() => setLoading(false))
  }, [configPath])

  return (
    <div className="config-viewer-overlay" onClick={onClose}>
      <div className="config-viewer-panel" onClick={e => e.stopPropagation()}>
        <div className="config-viewer-header">
          <h3>{configPath}</h3>
          <button className="config-viewer-close" onClick={onClose}>&times;</button>
        </div>
        <div className="config-viewer-body">
          {loading && <div className="loading">Loading...</div>}
          {error && <div className="error">{error}</div>}
          {data && <HighlightedYaml raw={data.raw} />}
        </div>
      </div>
    </div>
  )
}

// --- Source file viewer overlay ---

function SourceViewer({ sourcePath, onClose }: {
  sourcePath: string
  onClose: () => void
}) {
  const [data, setData] = useState<SourceDetail | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    setLoading(true)
    setError(null)
    getSource(sourcePath)
      .then(setData)
      .catch(() => setError('Failed to load source file (is --source configured?)'))
      .finally(() => setLoading(false))
  }, [sourcePath])

  return (
    <div className="config-viewer-overlay" onClick={onClose}>
      <div className="config-viewer-panel" onClick={e => e.stopPropagation()}>
        <div className="config-viewer-header">
          <h3>{sourcePath}</h3>
          <div className="source-viewer-meta">
            {data && <span className="source-lang-badge">{data.language}</span>}
            <button className="config-viewer-close" onClick={onClose}>&times;</button>
          </div>
        </div>
        <div className="config-viewer-body">
          {loading && <div className="loading">Loading...</div>}
          {error && (
            <div className="source-viewer-fallback">
              <p>{error}</p>
              <a
                href={`${GITLAB_BLOB}/${sourcePath}`}
                target="_blank"
                rel="noopener noreferrer"
                className="source-gitlab-link"
              >
                View on GitLab →
              </a>
            </div>
          )}
          {data && <pre className="config-yaml-view">{data.content}</pre>}
        </div>
      </div>
    </div>
  )
}

function CuratorWorkflowPanel({ docPath, raw }: { docPath: string; raw: string }) {
  const [instruction, setInstruction] = useState('')
  const [validation, setValidation] = useState<ValidationReport | null>(null)
  const [suggestion, setSuggestion] = useState<SuggestionResponse | null>(null)
  const [decision, setDecision] = useState<PatchDecision | null>(null)
  const [busy, setBusy] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)

  const runValidation = async () => {
    setBusy('validate')
    setError(null)
    try {
      setValidation(await validateDocs([docPath]))
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Validation failed')
    } finally {
      setBusy(null)
    }
  }

  const createSuggestion = async () => {
    if (!instruction.trim()) {
      setError('Describe the requested change before generating a suggestion.')
      return
    }
    setBusy('suggest')
    setError(null)
    try {
      setSuggestion(await suggestDocChanges(docPath, instruction, raw.slice(0, 2000)))
      setDecision(null)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Suggestion failed')
    } finally {
      setBusy(null)
    }
  }

  const decide = async (action: 'approve' | 'reject') => {
    if (!suggestion) return
    setBusy(action)
    setError(null)
    try {
      const result = action === 'approve'
        ? await approvePatch(suggestion.patch_id, 'documentation-curator-ui', 'Reviewed in UI')
        : await rejectPatch(suggestion.patch_id, 'documentation-curator-ui', 'Rejected in UI')
      setDecision(result)
    } catch (err) {
      setError(err instanceof Error ? err.message : `${action} failed`)
    } finally {
      setBusy(null)
    }
  }

  return (
    <div className="doc-section curator-panel">
      <div className="curator-panel-header">
        <div>
          <h3>Curator Workflow</h3>
          <p>Validate this document and draft reviewable changes. Decisions do not apply file writes.</p>
        </div>
        <button className="curator-button" onClick={runValidation} disabled={busy !== null}>
          {busy === 'validate' ? 'Validating...' : 'Validate'}
        </button>
      </div>

      {validation && (
        <div className={`curator-status curator-status-${validation.status}`}>
          <strong>{validation.status}</strong>
          <span>{validation.findings.length} findings across {validation.checked_paths.length} checked paths</span>
        </div>
      )}
      {validation?.findings.length ? (
        <ul className="curator-findings">
          {validation.findings.map((finding, idx) => (
            <li key={`${finding.code}-${idx}`}>
              <span className="curator-finding-code">{finding.code}</span>
              <span>{finding.path}: {finding.message}</span>
            </li>
          ))}
        </ul>
      ) : null}

      <label className="curator-label" htmlFor="curator-instruction">Suggestion instruction</label>
      <textarea
        id="curator-instruction"
        className="curator-textarea"
        value={instruction}
        onChange={e => setInstruction(e.target.value)}
        placeholder="Describe the documentation change to propose."
      />
      <button className="curator-button" onClick={createSuggestion} disabled={busy !== null}>
        {busy === 'suggest' ? 'Generating...' : 'Generate proposal'}
      </button>

      {suggestion && (
        <div className="curator-suggestion">
          <div className="curator-suggestion-header">
            <strong>{suggestion.patch_id}</strong>
            <span>{suggestion.status}</span>
          </div>
          <ul>
            {suggestion.suggestions.map(item => <li key={item}>{item}</li>)}
          </ul>
          <pre>{suggestion.proposed_patch}</pre>
          <div className="curator-actions">
            <button className="curator-button" onClick={() => decide('approve')} disabled={busy !== null}>
              Approve record
            </button>
            <button className="curator-button curator-button-secondary" onClick={() => decide('reject')} disabled={busy !== null}>
              Reject record
            </button>
          </div>
        </div>
      )}

      {decision && (
        <div className="curator-status">
          <strong>{decision.status}</strong>
          <span>applied: {String(decision.applied)}</span>
        </div>
      )}
      {error && <div className="error">{error}</div>}
    </div>
  )
}

// --- Main page ---

export default function Documentation() {
  const { '*': rawPath } = useParams()
  const navigate = useNavigate()
  const [docs, setDocs] = useState<DocEntry[]>([])
  const [detail, setDetail] = useState<DocDetail | null>(null)
  const [loading, setLoading] = useState(true)
  const [detailLoading, setDetailLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [uxConfig, setUXConfig] = useState<UXConfig | null>(null)
  const [viewingConfig, setViewingConfig] = useState<string | null>(null)
  const [viewingSource, setViewingSource] = useState<string | null>(null)
  const selectedPath = decodeRoutePath(rawPath)

  useEffect(() => {
    Promise.all([getUXConfig(), listDocs()])
      .then(([ux, entries]) => {
        setUXConfig(ux)
        setDocs(entries)
      })
      .catch(() => setError('Failed to load documentation index'))
      .finally(() => setLoading(false))
  }, [])

  const docIndex = useMemo(() => buildDocIndex(docs), [docs])
  const linkPattern = useMemo(() => buildLinkPattern(docIndex), [docIndex])

  const selectDoc = useCallback((path: string) => {
    navigate(documentURL(path))
  }, [navigate])

  useEffect(() => {
    if (!selectedPath) {
      setDetail(null)
      return
    }
    setDetailLoading(true)
    getDoc(selectedPath)
      .then(setDetail)
      .catch(() => setError('Failed to load document'))
      .finally(() => setDetailLoading(false))
  }, [selectedPath])

  const grouped = docs.reduce<Record<string, DocEntry[]>>((acc, d) => {
    ;(acc[d.category] ??= []).push(d)
    return acc
  }, {})

  const sortedCategories = Object.keys(grouped).sort((a, b) => {
    return categoryMeta(a, uxConfig).order - categoryMeta(b, uxConfig).order
  })

  if (loading) return <div className="loading">Loading documentation...</div>
  if (error && docs.length === 0) return <div className="error">{error}</div>

  const content = detail?.content as Record<string, unknown> | null
  const stateDiagram = content ? extractStateDiagram(content) : null

  const isSemanticModel = detail && docs.find(d => d.path === detail.path)?.category === 'semantic-model'
  const semanticModelFigure = isSemanticModel && content ? extractSemanticModelFigure(content) : null
  const configBlock = content?.configuration as Record<string, unknown> | undefined
  const machinePath = isSemanticModel && configBlock && typeof configBlock.machine === 'string'
    ? configBlock.machine : null
  const presentation = uxConfig?.presentation

  const topFields = ['id', 'title', 'version']
  const textFields = ['purpose', 'problem', 'overview', 'summary', 'executive_summary', 'what_this_does', 'why_we_build_this', 'trigger', 'lifecycle', 'pipeline_diagram']
  const skipFields = new Set([...topFields, ...textFields, 'states', 'signals', 'configuration', 'requirements'])

  return (
    <div className="docs-layout">
      <aside className="docs-sidebar">
        <div className="docs-sidebar-header">
          <h2 className="docs-sidebar-title">{uxConfig?.sidebar.title ?? 'Documentation'}</h2>
          <span className="docs-sidebar-total">{docs.length} docs</span>
        </div>
        {sortedCategories.map(cat => (
          <SidebarCategory
            key={cat}
            cat={cat}
            entries={grouped[cat]}
            selectedPath={selectedPath}
            onSelect={selectDoc}
            uxConfig={uxConfig}
          />
        ))}
      </aside>

      <section className="docs-content">
        {!selectedPath && (
          <div className="docs-welcome">
            <h1>{uxConfig?.title ?? 'Documentation'}</h1>
            <p>Select a document from the sidebar to view its contents.</p>
            <div className="docs-overview-grid">
              {sortedCategories.map(cat => {
                const meta = categoryMeta(cat, uxConfig)
                return (
                  <div key={cat} className="docs-overview-card" onClick={() => { const first = grouped[cat]?.[0]; if (first) selectDoc(first.path) }} style={{ cursor: 'pointer' }}>
                    <span className="docs-overview-icon">{meta.icon}</span>
                    <h3>{meta.label}</h3>
                    <span className="docs-count">{grouped[cat].length} {grouped[cat].length === 1 ? 'document' : 'documents'}</span>
                  </div>
                )
              })}
            </div>
          </div>
        )}

        {detailLoading && <div className="loading">Loading document...</div>}

        {detail && !detailLoading && content && (
          <div className="doc-viewer">
            <div className="doc-header">
              {'title' in content && <h1>{String(content.title)}</h1>}
              {'id' in content && <span className="doc-id mono">{String(content.id)}</span>}
            </div>

            {textFields.map(field =>
              content[field] ? (
                <YamlSection
                  key={field}
                  label={field.replace(/_/g, ' ').replace(/\b\w/g, c => c.toUpperCase())}
                  value={content[field]}
                  pattern={linkPattern}
                  index={docIndex}
                  onNavigate={selectDoc}
                />
              ) : null
            )}

            {isSemanticModel && configBlock && (
              <ConfigurationBlock
                config={configBlock}
                onConfigClick={setViewingConfig}
              />
            )}

            {presentation?.state_diagram !== false && semanticModelFigure && (
              <div className="doc-section semantic-model-figure">
                <h3>Semantic Model Figure</h3>
                <MermaidDiagram chart={semanticModelFigure} />
              </div>
            )}

            {presentation?.state_diagram !== false && stateDiagram && (
              <div className="doc-section">
                <h3>State Diagram</h3>
                <ModelDefinitionToggle chart={stateDiagram} machinePath={machinePath} />
              </div>
            )}

            {'states' in content && Array.isArray(content.states) && (
              <div className="doc-section">
                <h3>States</h3>
                <div className="doc-states-grid">
                  {(content.states as SemanticState[]).map(s => (
                    <div key={s.name} className="doc-state-card">
                      <span className="config-state-badge">{s.name}</span>
                      <p className="doc-state-meaning">{s.meaning}</p>
                    </div>
                  ))}
                </div>
              </div>
            )}

            {'signals' in content && Array.isArray(content.signals) && (
              <div className="doc-section">
                <h3>Signals</h3>
                <div className="doc-states-grid">
                  {(content.signals as Array<{ name: string; trigger: string }>).map(s => (
                    <div key={s.name} className="doc-state-card">
                      <span className="timeline-signal">{s.name}</span>
                      <p className="doc-state-meaning">{s.trigger}</p>
                    </div>
                  ))}
                </div>
              </div>
            )}

            {Object.entries(content)
              .filter(([k]) => !skipFields.has(k) && !textFields.includes(k))
              .map(([k, v]) => (
                <YamlSection
                  key={k}
                  label={k.replace(/_/g, ' ').replace(/\b\w/g, c => c.toUpperCase())}
                  value={v}
                  pattern={linkPattern}
                  index={docIndex}
                  onNavigate={selectDoc}
                />
              ))}

            <CuratorWorkflowPanel docPath={detail.path} raw={detail.raw} />

            {presentation?.raw_yaml_toggle !== false && (
              <div className="doc-section doc-raw-section">
                <details>
                  <summary className="config-toggle">View raw YAML</summary>
                  <HighlightedYaml raw={detail.raw} className="doc-raw" />
                </details>
              </div>
            )}
          </div>
        )}
      </section>

      {presentation?.config_viewer !== false && viewingConfig && (
        <ConfigViewer configPath={viewingConfig} onClose={() => setViewingConfig(null)} />
      )}

      {presentation?.source_viewer !== false && viewingSource && (
        <SourceViewer sourcePath={viewingSource} onClose={() => setViewingSource(null)} />
      )}
    </div>
  )
}
