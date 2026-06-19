// API 客户端入口
// 由 openapi-typescript 从 backend/api/openapi.yaml 自动生成
// 禁止手写类型，运行 pnpm --filter @mosaic/api-client generate 更新

import createClient from 'openapi-fetch'
// import type { paths } from './schema'  // 待 openapi.yaml 生成后取消注释

// 用户侧 API 客户端
export const apiClient = createClient<Record<string, never>>({
  baseUrl: '',
})

// Admin API 客户端
export const adminApiClient = createClient<Record<string, never>>({
  baseUrl: '',
})

// 设置 JWT Token（登录后调用）
export function setAuthToken(token: string) {
  const headers = { Authorization: `Bearer ${token}` }
  apiClient.interceptors?.request.use((req) => {
    Object.assign(req.headers, headers)
    return req
  })
}
