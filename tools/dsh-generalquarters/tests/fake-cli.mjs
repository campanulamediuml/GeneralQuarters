import { createInterface } from 'node:readline'

const lines = createInterface({ input: process.stdin, crlfDelay: Infinity })
lines.on('line', line => {
  const request = JSON.parse(line)
  const respond = result => {
    process.stdout.write(`${JSON.stringify({ id: request.id, ok: true, result })}\n`)
  }
  if (request.method === 'echo' || request.method === 'gq_create') {
    setTimeout(() => respond({ pid: process.pid, params: request.params }), request.params.delay || 0)
  } else if (request.method === 'slow') {
    setTimeout(() => respond({ finished: true }), 5000)
  } else if (request.method === 'error') {
    process.stdout.write(`${JSON.stringify({ id: request.id, ok: false, error: { message: '命令被拒绝', error_code: 'invalid_order' } })}\n`)
  } else if (request.method === 'crash') {
    process.exit(23)
  }
})
