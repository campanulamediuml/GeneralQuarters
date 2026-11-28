import { spawn } from 'node:child_process'
import { randomUUID } from 'node:crypto'
import { createInterface } from 'node:readline'
import path from 'node:path'

export class GQCLIError extends Error {
  constructor(details) {
    const code = details.error_code ? ` [${details.error_code}]` : ''
    super(`${details.message || 'gqcli 请求失败'}${code}`)
    this.name = 'GQCLIError'
    this.details = details
  }
}

export class GQCLIBridge {
  constructor(options = {}) {
    this.cliPath = options.cliPath || process.env.GQ_CLI_PATH || 'gqcli'
    this.cliArgs = options.cliArgs || []
    this.server = options.server || process.env.GQ_SERVER || 'http://127.0.0.1:8080'
    this.cwd = options.cwd || process.cwd()
    this.stateDir = options.stateDir || path.join(
      options.stateRoot || process.env.GQ_DSH_STATE_ROOT || path.join(this.cwd, '.gq', 'dsh'),
      options.stateKey ? encodeURIComponent(options.stateKey) : randomUUID(),
    )
    this.child = null
    this.pending = new Map()
    this.closed = false
  }

  async start() {
    if (this.closed) throw new Error('GQ CLI bridge 已关闭')
    if (this.child && this.child.exitCode === null && this.child.signalCode === null) return
    const child = spawn(this.cliPath, [...this.cliArgs, '-server', this.server, '-state-dir', this.stateDir], {
      cwd: this.cwd,
      env: process.env,
      stdio: ['pipe', 'pipe', 'pipe'],
    })
    this.child = child
    const lines = createInterface({ input: child.stdout, crlfDelay: Infinity })
    lines.on('line', line => this.handleLine(line))
    const errors = createInterface({ input: child.stderr, crlfDelay: Infinity })
    errors.on('line', line => console.error(`[gqcli] ${line}`))
    child.once('error', error => this.failAll(error))
    child.stdin.on('error', error => this.failAll(error))
    child.once('exit', (code, signal) => {
      if (this.child === child) this.child = null
      this.failAll(new Error(`gqcli 退出（code=${code}, signal=${signal}）`))
    })
  }

  handleLine(line) {
    let response
    try {
      response = JSON.parse(line)
    } catch {
      console.error('[gqcli] 忽略非 JSON stdout 输出')
      return
    }
    const pending = this.pending.get(response.id)
    if (!pending) return
    this.pending.delete(response.id)
    clearTimeout(pending.timer)
    pending.cleanup()
    if (response.ok) pending.resolve(response.result)
    else pending.reject(new GQCLIError(response.error || {}))
  }

  async request(method, params = {}, options = {}) {
    await this.start()
    if (options.signal?.aborted) throw options.signal.reason
    const id = randomUUID()
    const child = this.child
    const timeoutMS = options.timeoutMS ?? (method === 'gq_wait' ? (params.timeout_ms || 30000) + 10000 : 30000)
    return new Promise((resolve, reject) => {
      let abort
      const cleanup = () => options.signal?.removeEventListener('abort', abort)
      const pending = { resolve, reject, cleanup, timer: null }
      const cancel = error => {
        if (!this.pending.delete(id)) return
        clearTimeout(pending.timer)
        cleanup()
        this.sendCancel(id)
        reject(error)
      }
      abort = () => cancel(options.signal.reason || abortError())
      pending.timer = setTimeout(() => cancel(new Error(`gqcli ${method} 请求超时`)), timeoutMS)
      this.pending.set(id, pending)
      options.signal?.addEventListener('abort', abort, { once: true })
      try {
        child.stdin.write(`${JSON.stringify({ id, method, params })}\n`)
      } catch (error) {
        cancel(error)
      }
    })
  }

  sendCancel(targetID) {
    if (!this.child || this.child.stdin.destroyed) return
    try {
      this.child.stdin.write(`${JSON.stringify({ id: randomUUID(), method: 'gq_cancel', params: { target_id: targetID } })}\n`)
    } catch {
    }
  }

  failAll(error) {
    for (const pending of this.pending.values()) {
      clearTimeout(pending.timer)
      pending.cleanup()
      pending.reject(error)
    }
    this.pending.clear()
  }

  async close() {
    this.closed = true
    const child = this.child
    this.failAll(new Error('GQ CLI bridge 已关闭'))
    if (!child) return
    const exited = new Promise(resolve => child.once('exit', resolve))
    child.kill('SIGTERM')
    const timer = setTimeout(() => {
      if (child.exitCode === null && child.signalCode === null) child.kill('SIGKILL')
    }, 2000)
    timer.unref()
    await exited
    clearTimeout(timer)
  }
}

function abortError() {
  const error = new Error('工具调用已取消')
  error.name = 'AbortError'
  return error
}
