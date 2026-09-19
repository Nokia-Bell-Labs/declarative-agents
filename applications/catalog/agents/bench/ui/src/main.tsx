import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import config from 'virtual:ui-config'
import { AppShell } from '@declarative-agents/ui-kit'
import '@declarative-agents/ui-kit/styles.css'
import { registry } from './panels'
import './App.css'

// Sidebar and routes come from ui.yaml, bundled by the kit Vite plugin
// (srd004 R5); the panels read through the shell's same-origin kit client.
createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <AppShell config={config} registry={registry} />
  </StrictMode>,
)
