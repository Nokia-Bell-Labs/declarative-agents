import path from 'node:path'
import { parseArgs } from 'node:util'

export type E2EConfig = {
  baseURL: string
  executablePath: string
  artifactDir: string
}

export function parseE2EConfig(args: string[], cwd = process.cwd()): E2EConfig {
  const { values } = parseArgs({
    args,
    options: {
      'base-url': {
        type: 'string',
        default: 'http://127.0.0.1:18081/',
      },
      'executable-path': {
        type: 'string',
      },
      'artifact-dir': {
        type: 'string',
        default: path.join(cwd, 'e2e-artifacts'),
      },
    },
    strict: true,
  })

  if (!values['executable-path']) {
    throw new Error('--executable-path is required by puppeteer-core')
  }

  return {
    baseURL: values['base-url'],
    executablePath: values['executable-path'],
    artifactDir: path.resolve(cwd, values['artifact-dir']),
  }
}
