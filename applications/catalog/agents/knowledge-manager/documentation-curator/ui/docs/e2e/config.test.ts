import assert from 'node:assert/strict'
import test from 'node:test'
import { parseE2EConfig } from './config.ts'

test('uses stable defaults without environment configuration', () => {
  assert.deepEqual(parseE2EConfig(['--executable-path=/opt/chrome'], '/workspace/docs'), {
    baseURL: 'http://127.0.0.1:18081/',
    executablePath: '/opt/chrome',
    artifactDir: '/workspace/docs/e2e-artifacts',
  })
})

test('accepts every configuration override explicitly', () => {
  assert.deepEqual(parseE2EConfig([
    '--base-url=http://127.0.0.1:18081/',
    '--artifact-dir=e2e-artifacts',
    '--base-url=https://docs.example.test/root/',
    '--executable-path=/custom/chromium',
    '--artifact-dir=proof',
  ], '/workspace/docs'), {
    baseURL: 'https://docs.example.test/root/',
    executablePath: '/custom/chromium',
    artifactDir: '/workspace/docs/proof',
  })
})

test('requires the puppeteer-core browser contract', () => {
  assert.throws(
    () => parseE2EConfig([], '/workspace/docs'),
    /--executable-path is required by puppeteer-core/,
  )
})
