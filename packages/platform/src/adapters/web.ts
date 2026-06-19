import type { PlatformAdapter } from '../index'

export class WebAdapter implements PlatformAdapter {
  async saveFile(content: string, filename: string, mimeType = 'text/plain'): Promise<void> {
    const blob = new Blob([content], { type: mimeType })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = filename
    document.body.appendChild(a)
    a.click()
    document.body.removeChild(a)
    URL.revokeObjectURL(url)
  }

  async saveFileDialog(_defaultPath?: string): Promise<string | null> {
    // Web 端不支持选择保存路径，直接触发下载
    return null
  }

  async openFile(accept: string[]): Promise<{ name: string; content: string } | null> {
    return new Promise((resolve) => {
      const input = document.createElement('input')
      input.type = 'file'
      input.accept = accept.join(',')
      input.onchange = async (e) => {
        const file = (e.target as HTMLInputElement).files?.[0]
        if (!file) { resolve(null); return }
        const content = await file.text()
        resolve({ name: file.name, content })
      }
      input.click()
    })
  }

  async openFolder(): Promise<string | null> {
    // Web 端不支持选择文件夹路径
    return null
  }

  async showNotification(title: string, body: string): Promise<void> {
    if ('Notification' in window && Notification.permission === 'granted') {
      new Notification(title, { body })
    }
  }

  async copyToClipboard(text: string): Promise<void> {
    await navigator.clipboard.writeText(text)
  }

  async secureStore(key: string, value: string): Promise<void> {
    sessionStorage.setItem(key, value)
  }

  async secureGet(key: string): Promise<string | null> {
    return sessionStorage.getItem(key)
  }

  async secureClear(key: string): Promise<void> {
    sessionStorage.removeItem(key)
  }

  isDesktop(): boolean { return false }
  getPlatform(): 'web' { return 'web' }
}
