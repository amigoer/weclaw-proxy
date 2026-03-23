const API_BASE = ''

// 获取系统状态
export async function fetchStatus() {
  const res = await fetch(`${API_BASE}/api/status`)
  return res.json()
}

// Agent 适配器 API
export async function fetchAdapters() {
  const res = await fetch(`${API_BASE}/api/adapters`)
  return res.json()
}

export async function createAdapter(data: Record<string, unknown>) {
  const res = await fetch(`${API_BASE}/api/adapters`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data),
  })
  return res.json()
}

export async function updateAdapter(name: string, data: Record<string, unknown>) {
  const res = await fetch(`${API_BASE}/api/adapters/${name}`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data),
  })
  return res.json()
}

export async function deleteAdapter(name: string) {
  const res = await fetch(`${API_BASE}/api/adapters/${name}`, {
    method: 'DELETE',
  })
  return res.json()
}

// 路由 API
export async function fetchRoutes() {
  const res = await fetch(`${API_BASE}/api/routes`)
  return res.json()
}

export async function updateRoutes(data: Record<string, unknown>) {
  const res = await fetch(`${API_BASE}/api/routes`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data),
  })
  return res.json()
}

// 退出微信登录
export async function logout() {
  const res = await fetch(`${API_BASE}/api/logout`, {
    method: 'POST',
  })
  return res.json()
}

// 获取登录二维码
export async function startLogin() {
  const res = await fetch(`${API_BASE}/api/login/qrcode`, {
    method: 'POST',
  })
  return res.json()
}

// 获取登录状态
export async function getLoginStatus() {
  const res = await fetch(`${API_BASE}/api/login/status`)
  return res.json()
}
