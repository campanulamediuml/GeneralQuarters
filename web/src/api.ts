import type { GameView, Order, RoomResponse } from './types'

type PendingRpc = { resolve: (value: unknown) => void; reject: (reason: Error) => void }
let roomSocket: WebSocket | undefined
let rpcSequence = 0
const pendingRpc = new Map<string, PendingRpc>()

export function attachRoomSocket(socket?: WebSocket) {
  if (roomSocket && roomSocket !== socket) {
    for (const pending of pendingRpc.values()) pending.reject(new Error('WebSocket 已断开'))
    pendingRpc.clear()
  }
  roomSocket = socket
}

export function handleRoomSocketMessage(raw: string): boolean {
  let message: { type?: string; id?: string; status?: number; body?: unknown }
  try { message = JSON.parse(raw) as typeof message } catch { return false }
  if (message.type !== 'rpc_response' || !message.id) return false
  const pending = pendingRpc.get(message.id)
  if (!pending) return true
  pendingRpc.delete(message.id)
  const body = message.body as { error?: string } | undefined
  const status = message.status || 500
  if (status < 200 || status >= 300) pending.reject(new Error(body?.error || '请求失败（WS ' + status + '）'))
  else pending.resolve(message.body)
  return true
}

async function request<T>(path: string, options: RequestInit = {}): Promise<T> {
  const roomRPC = ['/state', '/events', '/commands', '/ready'].some((suffix) => path.endsWith(suffix))
  if (roomSocket?.readyState === WebSocket.OPEN && path.startsWith('/api/v1/rooms/') && roomRPC) {
    const id = String(++rpcSequence)
    return new Promise<T>((resolve, reject) => {
      pendingRpc.set(id, { resolve: (value) => resolve(value as T), reject })
      try {
        roomSocket!.send(JSON.stringify({ type: 'rpc_request', id, method: options.method || 'GET', path, body: options.body ? JSON.parse(String(options.body)) : undefined }))
      } catch (error) {
        pendingRpc.delete(id)
        reject(error instanceof Error ? error : new Error('WebSocket 请求失败'))
      }
    })
  }
  if (roomRPC && path.startsWith('/api/v1/rooms/')) throw new Error('连接已断开，正在重连，请稍后重试')
  const response = await fetch(path, {
    ...options,
    credentials: 'same-origin',
    headers: { 'Content-Type': 'application/json', ...options.headers },
  })
  const body = await response.json().catch(() => ({ error: '服务器返回了无法解析的响应' }))
  if (!response.ok) throw new Error(body.error || '请求失败（' + response.status + '）')
  return body as T
}

function commandID() {
  if (typeof crypto.randomUUID === 'function') return crypto.randomUUID()
  const bytes = crypto.getRandomValues(new Uint8Array(16))
  return Array.from(bytes, (byte) => byte.toString(16).padStart(2, '0')).join('')
}

export const api = {
  createRoom(name: string, scenarioID = '_sandbox', solo = false, faction: 'A' | 'B' = 'A') {
    return request<RoomResponse>('/api/v1/rooms', { method: 'POST', body: JSON.stringify({ name, scenario_id: scenarioID, solo, faction }) })
  },
  joinRoom(token: string, name: string) {
    return request<RoomResponse>('/api/v1/rooms/join', { method: 'POST', body: JSON.stringify({ token, name }) })
  },
  state(roomID: string) {
    return request<GameView>('/api/v1/rooms/' + roomID + '/state')
  },
  command(roomID: string, version: number, order: Order) {
    return request<{ accepted: boolean }>('/api/v1/rooms/' + roomID + '/commands', {
      method: 'POST',
      body: JSON.stringify({ ...order, command_id: commandID(), base_version: version }),
    })
  },
  ready(roomID: string, version: number, ready: boolean) {
    return request<GameView>('/api/v1/rooms/' + roomID + '/ready', {
      method: 'POST',
      body: JSON.stringify({ base_version: version, ready }),
    })
  },
}
