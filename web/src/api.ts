import type { GameView, Order, RoomResponse } from './types'

async function request<T>(path: string, options: RequestInit = {}): Promise<T> {
  const response = await fetch(path, {
    ...options,
    credentials: 'same-origin',
    headers: { 'Content-Type': 'application/json', ...options.headers },
  })
  const body = await response.json().catch(() => ({ error: '服务器返回了无法解析的响应' }))
  if (!response.ok) throw new Error(body.error || `请求失败（${response.status}）`)
  return body as T
}

function commandID() {
  if (typeof crypto.randomUUID === 'function') return crypto.randomUUID()
  const bytes = crypto.getRandomValues(new Uint8Array(16))
  return Array.from(bytes, (byte) => byte.toString(16).padStart(2, '0')).join('')
}

export const api = {
  createRoom(name: string) {
    return request<RoomResponse>('/api/v1/rooms', { method: 'POST', body: JSON.stringify({ name, scenario_id: '_sandbox' }) })
  },
  joinRoom(token: string, name: string) {
    return request<RoomResponse>('/api/v1/rooms/join', { method: 'POST', body: JSON.stringify({ token, name }) })
  },
  state(roomID: string) {
    return request<GameView>(`/api/v1/rooms/${roomID}/state`)
  },
  command(roomID: string, version: number, order: Order) {
    return request<{ accepted: boolean }>(`/api/v1/rooms/${roomID}/commands`, {
      method: 'POST',
      body: JSON.stringify({ ...order, command_id: commandID(), base_version: version }),
    })
  },
  ready(roomID: string, version: number, ready: boolean) {
    return request<GameView>(`/api/v1/rooms/${roomID}/ready`, {
      method: 'POST',
      body: JSON.stringify({ base_version: version, ready }),
    })
  },
}
