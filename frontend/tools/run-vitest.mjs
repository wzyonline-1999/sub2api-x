#!/usr/bin/env node
import { spawn } from 'node:child_process'
import { resolve } from 'node:path'
import process from 'node:process'
import { fileURLToPath } from 'node:url'

const webStorageFlag = '--no-experimental-webstorage'
const currentNodeOptions = process.env.NODE_OPTIONS?.trim() ?? ''
const nodeOptions = currentNodeOptions ? currentNodeOptions.split(/\s+/) : []

if (
  process.allowedNodeEnvironmentFlags.has(webStorageFlag) &&
  !nodeOptions.includes(webStorageFlag)
) {
  nodeOptions.push(webStorageFlag)
}

const env = {
  ...process.env,
  ...(nodeOptions.length > 0 ? { NODE_OPTIONS: nodeOptions.join(' ') } : {})
}

const vitestArgs = process.argv.slice(2).filter((arg) => arg !== '--')
const vitestBin = resolve(fileURLToPath(new URL('..', import.meta.url)), 'node_modules/vitest/vitest.mjs')

const child = spawn(process.execPath, [vitestBin, ...vitestArgs], {
  env,
  stdio: 'inherit'
})

child.on('error', (error) => {
  console.error(error)
  process.exit(1)
})

child.on('exit', (code, signal) => {
  if (signal) {
    process.kill(process.pid, signal)
    return
  }
  process.exit(code ?? 1)
})
