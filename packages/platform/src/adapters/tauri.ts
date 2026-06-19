import type { PlatformAdapter } from '../index'

// Tauri 插件按需动态导入，避免在非 Tauri 环境报错
// 实际使用时需安装：@tauri-apps/plugin-dialog @tauri-apps/plugin-fs
// @tauri-apps/plugin-notification

export class TauriAdapter implements PlatformAdapter {
  async saveFile(content: string, filename: string): Promise<void> {
    const { save } = await import('@tauri-apps/plugin-dialog')
    const { writeTextFile } = await import('@tauri-apps/plugin-fs')
    const path = await save({ defaultPath: filename })
    if (path) {
      await writeTextFile(path, content)
    }
  }

  async saveFileDialog(defaultPath?: string): Promise<string | null> {
    const { save } = await import('@tauri-apps/plugin-dialog')
    return await save({ defaultPath })
  }

  async openFile(accept: string[]): Promise<{ name: string; content: string } | null> {
    const { open } = await import('@tauri-apps/plugin-dialog')
    const { readTextFile } = await import('@tauri-apps/plugin-fs')
    const path = await open({ filters: [{ name: 'Files', extensions: accept }] })
    if (!path || typeof path !== 'string') return null
    const content = await readTextFile(path)
    const name = path.split('/').pop() ?? path
    return { name, content }
  }

  async openFolder(): Promise<string | null> {
    const { open } = await import('@tauri-apps/plugin-dialog')
    const result = await open({ directory: true })
    return typeof result === 'string' ? result : null
  }

  async showNotification(title: string, body: string): Promise<void> {
    const { sendNotification } = await import('@tauri-apps/plugin-notification')
    await sendNotification({ title, body })
  }

  async copyToClipboard(text: string): Promise<void> {
    await navigator.clipboard.writeText(text)
  }

  async secureStore(key: string, value: string): Promise<void> {
    // 使用系统密钥链存储（@tauri-apps/plugin-stronghold）
    // 简化版：先用 localStorage，后期迁移到 stronghold
    localStorage.setItem(`secure_${key}`, value)
  }

  async secureGet(key: string): Promise<string | null> {
    return localStorage.getItem(`secure_${key}`)
  }

  async secureClear(key: string): Promise<void> {
    localStorage.removeItem(`secure_${key}`)
  }

  isDesktop(): boolean { return true }
  getPlatform(): 'tauri' { return 'tauri' }
}
