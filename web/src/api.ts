import type { GameView, Order, RoomResponse } from './types'
import { applyViewPatch, type ViewPatch } from './patch'
import { messageFor } from './display'

type PendingRpc = { resolve: (value: unknown) => void; reject: (reason: Error) => void }
let roomSocket: WebSocket | undefined
let rpcSequence = 0
const pendingRpc = new Map<string, PendingRpc>()

// 玩家先通过 RPC 拉全量底座，之后服务端在状态变化时只推与上一帧的差异。
// 这里负责把补丁合并成完整视图再交给视图层——视图层只关心"现在是什么样"。
type StateListener = (view: GameView) => void
let stateListener: StateListener | undefined
let pushedView: GameView | undefined

export function onPushedState(listener?: StateListener) {
  stateListener = listener
}

// adoptPushedView：客户端自己用 RPC 拉了一份全量之后，把这份对象交给补丁层当底座。
// 不接管的话，后续补丁会打在一个组件已经不渲染的旧对象上——界面看起来就"卡住不更新"。
export function adoptPushedView(view: GameView) {
  pushedView = view
}

export function attachRoomSocket(socket?: WebSocket) {
  if (roomSocket && roomSocket !== socket) {
    for (const pending of pendingRpc.values()) pending.reject(new Error('WebSocket 已断开'))
    pendingRpc.clear()
  }
  roomSocket = socket
}

export function handleRoomSocketMessage(raw: string): boolean {
  let message: { type?: string; id?: string; status?: number; body?: unknown; view?: unknown; patch?: unknown }
  try { message = JSON.parse(raw) as typeof message } catch { return false }
  if (message.type === 'state' || message.type === 'snapshot') {
    if (message.view) {
      pushedView = message.view as GameView
      stateListener?.(pushedView)
      return true
    }
    if (!message.patch) return true
    if (!pushedView) {
      // 还没拉到全量底座：这一帧直接丢掉，不回退成"断开重连"。
      // 底座（/state）正在同一条连接上加载，它反映的是加载那一刻的最新状态；
      // 而在这里重连只会连上同一条推送路径、再撞上同一帧，变成同步不上的死循环。
      console.warn('还没拿到全量底座，先丢弃这一帧补丁')
      return true
    }
    // 就地打补丁：改了哪个字段，只有读那个字段的组件会重算。
    try {
      applyViewPatch(pushedView, message.patch as ViewPatch)
    } catch (error) {
      // 合并不上说明本地这份底座已经不可信了。断开重连：新连接会重新拉一份全量，
      // 比硬撑着一份错的视图强得多。
      console.error('视图补丁合并失败，重连以重新同步', error)
      roomSocket?.close()
      return true
    }
    stateListener?.(pushedView)
    return true
  }
  if (message.type !== 'rpc_response' || !message.id) return false
  const pending = pendingRpc.get(message.id)
  if (!pending) return true
  pendingRpc.delete(message.id)
  const body = message.body as { error?: string; error_code?: string; error_params?: Record<string, string> } | undefined
  const status = message.status || 500
  if (status < 200 || status >= 300) pending.reject(new Error(messageFor(body?.error_code, body?.error_params, body?.error) || '请求失败（WS ' + status + '）'))
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
  if (!response.ok) throw new Error(messageFor(body.error_code, body.error_params, body.error) || '请求失败（' + response.status + '）')
  return body as T
}

function commandID() {
  if (typeof crypto.randomUUID === 'function') return crypto.randomUUID()
  const bytes = crypto.getRandomValues(new Uint8Array(16))
  return Array.from(bytes, (byte) => byte.toString(16).padStart(2, '0')).join('')
}

export const api = {
  // 观战链接 /watch/{token}：token 换 room_id，之后连 /ws?mode=spectator 订阅。
  watchRoom(token: string) {
    return request<{ room_id: string }>('/api/v1/watch/' + encodeURIComponent(token))
  },
  createRoom(name: string, scenarioID = '_coral_real', solo = false, faction: 'A' | 'B' = 'A') {
    return request<RoomResponse>('/api/v1/rooms', { method: 'POST', body: JSON.stringify({ name, scenario_id: scenarioID, solo, faction }) })
  },
  joinRoom(token: string, name: string) {
    return request<RoomResponse>('/api/v1/rooms/join', { method: 'POST', body: JSON.stringify({ token, name }) })
  },
  state(roomID: string) {
    return request<GameView>('/api/v1/rooms/' + roomID + '/state')
  },
  // base_turn 只用于拒绝"回合已经结算"的过期提交：同回合内重复操作一律接受，
  // 后写的顶掉先写的。不用版本号——那是服务端内部的状态计数，不进协议。
  command(roomID: string, turn: number, order: Order) {
    return request<{ accepted: boolean }>('/api/v1/rooms/' + roomID + '/commands', {
      method: 'POST',
      body: JSON.stringify({ ...order, command_id: commandID(), base_turn: turn }),
    })
  },
  // ready 只回一条回执：状态走推送，不为改一个标记拉一份全量。
  ready(roomID: string, turn: number, ready: boolean) {
    return request<{ accepted: boolean; turn: number }>('/api/v1/rooms/' + roomID + '/ready', {
      method: 'POST',
      body: JSON.stringify({ base_turn: turn, ready }),
    })
  },
}