import test from 'node:test'
import assert from 'node:assert/strict'
import { mkdtemp, rm } from 'node:fs/promises'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { GQCLIBridge, GQCLIError } from '../lib/bridge.mjs'
import { apply } from '../lib/index.mjs'

const directory = path.dirname(fileURLToPath(import.meta.url))
const fakeCLI = path.join(directory, 'fake-cli.mjs')

async function fixture(t) {
  const stateRoot = await mkdtemp(path.join(directory, '.bridge-test-'))
  const bridge = new GQCLIBridge({ cliPath: process.execPath, cliArgs: [fakeCLI], stateRoot })
  t.after(async () => {
    await bridge.close()
    await rm(stateRoot, { recursive: true, force: true })
  })
  return bridge
}

test('常驻进程复用且并发响应按 id 匹配', async t => {
  const bridge = await fixture(t)
  const [slow, fast] = await Promise.all([
    bridge.request('echo', { value: 'slow', delay: 40 }),
    bridge.request('echo', { value: 'fast', delay: 1 }),
  ])
  assert.equal(slow.params.value, 'slow')
  assert.equal(fast.params.value, 'fast')
  assert.equal(slow.pid, fast.pid)
  assert.equal(bridge.pending.size, 0)
})

test('服务端错误保留 error_code', async t => {
  const bridge = await fixture(t)
  await assert.rejects(bridge.request('error'), error => {
    assert.ok(error instanceof GQCLIError)
    assert.equal(error.details.error_code, 'invalid_order')
    return true
  })
})

test('gq_wait 类长请求可取消且不影响后续调用', async t => {
  const bridge = await fixture(t)
  const controller = new AbortController()
  const waiting = bridge.request('slow', {}, { signal: controller.signal, timeoutMS: 5000 })
  controller.abort()
  await assert.rejects(waiting)
  const response = await bridge.request('echo', { value: 'after-abort' })
  assert.equal(response.params.value, 'after-abort')
})

test('进程崩溃后用同一状态目录重启', async t => {
  const bridge = await fixture(t)
  const stateDir = bridge.stateDir
  const first = await bridge.request('echo', {})
  await assert.rejects(bridge.request('crash'), /gqcli 退出/)
  const second = await bridge.request('echo', {})
  assert.notEqual(first.pid, second.pid)
  assert.equal(bridge.stateDir, stateDir)
})

test('DSH 插件只注册六个受约束的 GQ 工具', async t => {
  const tools = []
  let dispose
  const ctx = {
    tools: { register: definition => tools.push(definition) },
    effect: callback => { dispose = callback() },
  }
  apply(ctx, { cliPath: process.execPath, cliArgs: [fakeCLI] })
  assert.deepEqual(tools.map(tool => tool.name).sort(), [
    'gq_command', 'gq_create', 'gq_join', 'gq_ready', 'gq_state', 'gq_wait',
  ])
  for (const tool of tools) {
    assert.equal(tool.parameters.additionalProperties, false)
    assert.equal(typeof tool.execute, 'function')
    assert.equal(tool.output.schema.type, 'object')
  }
  await dispose()
})

test('不同 DSH session 使用不同的常驻 CLI', async t => {
  const tools = []
  let dispose
  const stateRoot = await mkdtemp(path.join(directory, '.session-test-'))
  const ctx = {
    tools: { register: definition => tools.push(definition) },
    effect: callback => { dispose = callback() },
  }
  apply(ctx, { cliPath: process.execPath, cliArgs: [fakeCLI], stateRoot })
  t.after(async () => {
    await dispose()
    await rm(stateRoot, { recursive: true, force: true })
  })
  const create = tools.find(tool => tool.name === 'gq_create')
  const execute = session => create.execute({}, {
    agent: { session: { id: session } },
    signal: new AbortController().signal,
  })
  const [a, b, again] = await Promise.all([execute('session-a'), execute('session-b'), execute('session-a')])
  assert.equal(a.pid, again.pid)
  assert.notEqual(a.pid, b.pid)
})
