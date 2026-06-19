# 02 客户端架构

## 1. 总体策略：Web First + Tauri 渐进增强

Mosaic 采用"一套 React 代码，多端运行"的策略：

```
React Web App（核心，apps/web）
        │
        ├── 浏览器环境   → 标准 Web 能力
        │
        └── Tauri 壳     → Web 能力 + Rust 原生能力层
                                │
                                ├── 本地文件系统操作
                                ├── 系统通知
                                ├── 原生对话框
                                └── 系统密钥链
```

**核心原则**：Tauri 的 Rust 层只做浏览器无法完成的事，90% 的代码在 React 层共享。组件不感知运行环境，通过 `PlatformAdapter` 抽象隔离差异。

## 2. Monorepo 包结构

```
packages/
├── ui/           # 共享 React 组件库
├── api-client/   # 从 OpenAPI Spec 自动生成（禁止手写）
└── platform/     # PlatformAdapter 运行时适配层
```

### packages/platform

平台适配层是 Web/桌面代码共享的核心。

```typescript
// packages/platform/src/index.ts

export interface PlatformAdapter {
  // 文件操作
  saveFile(content: string, filename: string, mimeType: string): Promise<void>
  saveFileDialog(defaultPath?: string): Promise<string | null>  // 选择保存路径
  openFile(accept: string[]): Promise<{ name: string; content: string } | null>
  openFolder(): Promise<string | null>

  // 系统能力
  showNotification(title: string, body: string): Promise<void>
  copyToClipboard(text: string): Promise<void>

  // 安全存储（Tauri：系统密钥链；Web：内存/sessionStorage）
  secureStore(key: string, value: string): Promise<void>
  secureGet(key: string): Promise<string | null>
  secureClear(key: string): Promise<void>

  // 环境信息
  isDesktop(): boolean
  getPlatform(): 'web' | 'tauri' | 'flutter'
}
```

```typescript
// packages/platform/src/adapters/web.ts
export class WebAdapter implements PlatformAdapter {
  async saveFile(content: string, filename: string, mimeType: string) {
    const blob = new Blob([content], { type: mimeType })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = filename
    a.click()
    URL.revokeObjectURL(url)
  }

  isDesktop() { return false }
  getPlatform() { return 'web' as const }
  // ... 其他方法的 Web 实现
}
```

```typescript
// packages/platform/src/adapters/tauri.ts
import { save, open } from '@tauri-apps/plugin-dialog'
import { writeTextFile } from '@tauri-apps/plugin-fs'
import { sendNotification } from '@tauri-apps/plugin-notification'
import { SecretService } from '@tauri-apps/plugin-stronghold'

export class TauriAdapter implements PlatformAdapter {
  async saveFile(content: string, filename: string) {
    const path = await save({ defaultPath: filename })
    if (path) await writeTextFile(path, content)
  }

  async showNotification(title: string, body: string) {
    await sendNotification({ title, body })
  }

  isDesktop() { return true }
  getPlatform() { return 'tauri' as const }
  // ... 其他方法的 Tauri 实现
}
```

```typescript
// packages/platform/src/index.ts - 运行时自动选择
export const platform: PlatformAdapter =
  typeof window !== 'undefined' && '__TAURI_INTERNALS__' in window
    ? new TauriAdapter()
    : new WebAdapter()
```

**React 组件只调用 `platform`，完全不感知运行环境：**

```typescript
// 任意组件中
import { platform } from '@mosaic/platform'

const handleExport = async () => {
  await platform.saveFile(markdownContent, 'project-brief.md', 'text/markdown')
}
```

## 3. apps/web：React Web 核心

### 3.1 目录结构

```
apps/web/src/
├── routes/                   # 页面路由（React Router v6）
│   ├── workspace/            # Workspace 首页
│   ├── project/
│   │   ├── create/           # 项目创建页
│   │   ├── workspace/        # 项目工作台（实时进度）
│   │   └── delivery/         # 交付中心
│   └── skill-hub/            # Skill Hub 页面
├── components/               # 页面级组件（import from @mosaic/ui）
├── stores/                   # Zustand 状态管理
│   ├── project.store.ts      # 当前项目状态
│   ├── delivery.store.ts     # 交付物状态
│   └── ui.store.ts           # UI 状态（侧边栏、模态框等）
├── hooks/                    # 自定义 Hook
│   ├── useProjectSSE.ts      # SSE 进度订阅
│   ├── useDeliverables.ts    # 交付物 TanStack Query
│   └── usePlatform.ts        # 平台能力调用封装
├── lib/
│   └── api.ts                # openapi-fetch 实例（import from @mosaic/api-client）
└── main.tsx
```

### 3.2 SSE 进度订阅

```typescript
// hooks/useProjectSSE.ts

export function useProjectSSE(projectID: string) {
  const setStatus = useProjectStore(s => s.setStatus)

  useEffect(() => {
    if (!projectID) return

    const es = new EventSource(`/api/v1/projects/${projectID}/progress`)

    es.addEventListener('progress', (e) => {
      const status = JSON.parse(e.data)
      setStatus(status)
    })

    es.addEventListener('close', () => {
      es.close()
    })

    es.onerror = () => {
      es.close()
    }

    return () => es.close()
  }, [projectID])
}
```

### 3.3 状态管理原则

- **服务端状态**（项目数据、交付物）：TanStack Query 管理，不手动维护缓存。
- **客户端 UI 状态**（侧边栏开合、当前选中的入口）：Zustand 管理。
- **SSE 实时状态**（Skill 执行进度）：Zustand 接收并存储，TanStack Query 在完成后失效缓存触发重新获取。

### 3.4 八大入口的组件组织

```
components/
└── entries/
    ├── EntryContainer.tsx    # 通用入口容器（标题、状态、导出按钮）
    ├── DocumentEntry.tsx     # Documents 入口渲染
    ├── SlidesEntry.tsx       # Slides 入口渲染
    ├── SheetsEntry.tsx       # Sheets 入口渲染（表格视图）
    ├── ImagesEntry.tsx       # Images 入口渲染
    ├── VideosEntry.tsx       # Videos 入口渲染
    ├── PodcastsEntry.tsx     # Podcasts 入口渲染
    ├── SkypageEntry.tsx      # Skypage 入口渲染（HTML 预览）
    └── DeveloperEntry.tsx    # AI Developer 入口渲染（代码块）
```

## 4. apps/desktop：Tauri 桌面壳

### 4.1 架构原则

- Tauri 壳尽可能薄：`src-tauri/` 只包含 Rust 原生能力的 IPC 命令，不包含业务逻辑。
- UI 层完全复用 `apps/web` 的 React 代码，通过 Vite 构建后嵌入 WebView。
- 新增的桌面专属功能（文件操作、系统通知等）通过 Tauri Plugin 实现，对应 `packages/platform` 的 TauriAdapter。

### 4.2 Rust 层 IPC 命令（仅列出需要 Rust 原生能力的部分）

```rust
// src-tauri/src/main.rs

#[tauri::command]
async fn read_local_file(path: String) -> Result<String, String> {
    std::fs::read_to_string(&path).map_err(|e| e.to_string())
}

#[tauri::command]
async fn write_local_file(path: String, content: String) -> Result<(), String> {
    std::fs::write(&path, content).map_err(|e| e.to_string())
}

// 注：文件选择对话框、系统通知等通过 Tauri 官方插件实现，不需要自定义命令
```

### 4.3 桌面专属能力（V0.1 优先级）

| 能力 | 实现方式 | 用户价值 |
| --- | --- | --- |
| 导出交付物到本地文件夹 | `@tauri-apps/plugin-fs` + `@tauri-apps/plugin-dialog` | 直接保存到 Downloads，无需浏览器下载 |
| Skill 完成系统通知 | `@tauri-apps/plugin-notification` | 后台执行，完成时提醒 |
| 从本地导入素材 | `@tauri-apps/plugin-dialog` open 对话框 | 直接选择本地图片/文档 |
| 本地项目草稿缓存 | `@tauri-apps/plugin-store` | 离线可查看，断网不丢失 |

### 4.4 应用分发

- macOS：`.dmg` 通过 `tauri build` 生成，通过 GitHub Releases 分发。
- Windows：`.msi` / `.exe` 同上。
- 自动更新：使用 `@tauri-apps/plugin-updater`，从 GitHub Releases 检测新版本。

## 5. packages/ui：共享组件库

基于 shadcn/ui + Tailwind CSS 构建，Web 和桌面共用。

```
packages/ui/src/
├── components/
│   ├── primitives/           # 基础组件（Button, Input, Badge 等）
│   ├── layout/               # 布局组件（Sidebar, Header, Panel）
│   ├── project/              # 项目相关组件（ProjectCard, BriefViewer）
│   ├── skill/                # Skill 状态展示（SkillStatusBadge, SkillTimeline）
│   └── delivery/             # 交付物组件（DeliverableCard, ExportMenu）
├── hooks/                    # 通用 Hook（useDebounce, useLocalStorage）
└── index.ts                  # 统一导出
```

**设计约定**：
- 所有组件接受 `className` 覆写，不硬编码样式细节。
- 组件只依赖 `@mosaic/platform` 和 `@mosaic/api-client`，不直接调用任何端专属 API。
- 使用 Storybook 维护组件文档（中期引入）。

## 6. apps/mobile：Flutter 预留设计

V1+ 阶段实现，当前只保留目录结构与接口约定。

### 6.1 定位与使用场景

Flutter 移动端定位为**消费侧**，不是生产侧：

| 场景 | 移动端支持 |
| --- | --- |
| 查看交付中心 / 预览交付物 | 优先支持 |
| 审批/确认 AI 输出 | 优先支持 |
| 分享项目链接 | 优先支持 |
| 创建新项目 | 简化版表单 |
| 完整编辑交付物 | 不支持（引导到 Web/桌面） |

### 6.2 API 客户端生成

Flutter 端的 API 客户端通过 openapi-generator 自动生成，不手写：

```bash
openapi-generator generate \
  -i backend/api/openapi.yaml \
  -g dart-dio \
  -o apps/mobile/lib/api_client
```

### 6.3 目录结构（预留）

```
apps/mobile/
├── lib/
│   ├── api_client/           # openapi-generator 生成，禁止手写
│   ├── screens/
│   │   ├── delivery/         # 交付中心
│   │   └── project/          # 项目列表与详情
│   ├── widgets/              # 通用 Widget
│   └── main.dart
└── pubspec.yaml
```
