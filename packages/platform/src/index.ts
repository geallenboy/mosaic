// PlatformAdapter 运行时适配层
// 屏蔽 Web / Tauri 桌面 的差异，组件层不感知运行环境

export interface PlatformAdapter {
  // 文件操作
  saveFile(content: string, filename: string, mimeType?: string): Promise<void>
  saveFileDialog(defaultPath?: string): Promise<string | null>
  openFile(accept: string[]): Promise<{ name: string; content: string } | null>
  openFolder(): Promise<string | null>

  // 系统能力
  showNotification(title: string, body: string): Promise<void>
  copyToClipboard(text: string): Promise<void>

  // 安全存储（Tauri：系统密钥链；Web：sessionStorage）
  secureStore(key: string, value: string): Promise<void>
  secureGet(key: string): Promise<string | null>
  secureClear(key: string): Promise<void>

  // 环境信息
  isDesktop(): boolean
  getPlatform(): 'web' | 'tauri' | 'flutter'
}

export { WebAdapter } from './adapters/web'
export { TauriAdapter } from './adapters/tauri'

// 运行时自动选择适配器
function createPlatform(): PlatformAdapter {
  if (typeof window !== 'undefined' && '__TAURI_INTERNALS__' in window) {
    const { TauriAdapter } = require('./adapters/tauri')
    return new TauriAdapter()
  }
  const { WebAdapter } = require('./adapters/web')
  return new WebAdapter()
}

export const platform: PlatformAdapter = createPlatform()
