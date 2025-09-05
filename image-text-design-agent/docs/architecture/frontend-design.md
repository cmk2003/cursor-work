# 前端技术方案设计

## 1. 技术栈选型

### 1.1 核心框架
- **Vue 3.4+**：Composition API，更好的TypeScript支持
- **TypeScript 5.0+**：强类型支持，提升代码质量
- **Vite 5.0+**：快速的开发服务器和构建工具

### 1.2 UI组件库
- **Element Plus**：企业级UI组件库，组件丰富
- **Fabric.js**：强大的Canvas操作库，支持复杂图形编辑
- **Vue-Draggable-Plus**：拖拽组件库

### 1.3 状态管理
- **Pinia**：Vue 3官方推荐的状态管理
- **VueUse**：组合式API工具库

### 1.4 网络请求
- **Axios**：HTTP客户端
- **Socket.io-client**：WebSocket通信

### 1.5 其他工具
- **Vue Router 4**：路由管理
- **Tailwind CSS**：原子化CSS框架
- **Day.js**：日期处理
- **Lodash-es**：工具函数库

## 2. 项目结构

```
frontend/
├── src/
│   ├── api/                 # API接口层
│   │   ├── modules/         # 模块化接口
│   │   │   ├── user.ts      # 用户相关API
│   │   │   ├── project.ts   # 项目相关API
│   │   │   ├── design.ts    # 设计相关API
│   │   │   └── gallery.ts   # 灵感广场API
│   │   ├── request.ts       # Axios封装
│   │   └── types.ts         # API类型定义
│   │
│   ├── assets/              # 静态资源
│   │   ├── images/
│   │   ├── fonts/
│   │   └── styles/
│   │
│   ├── components/          # 公共组件
│   │   ├── common/          # 通用组件
│   │   │   ├── Loading.vue
│   │   │   ├── Modal.vue
│   │   │   └── Upload.vue
│   │   ├── canvas/          # 画布相关组件
│   │   │   ├── CanvasEditor.vue
│   │   │   ├── LayerPanel.vue
│   │   │   └── ToolBar.vue
│   │   └── lui/             # LUI相关组件
│   │       ├── ChatInput.vue
│   │       ├── PromptGuide.vue
│   │       └── GenerateStatus.vue
│   │
│   ├── composables/         # 组合式函数
│   │   ├── useCanvas.ts     # 画布操作
│   │   ├── useDesign.ts     # 设计逻辑
│   │   ├── useWebSocket.ts  # WebSocket连接
│   │   └── useAuth.ts       # 认证逻辑
│   │
│   ├── layouts/             # 布局组件
│   │   ├── MainLayout.vue
│   │   ├── AuthLayout.vue
│   │   └── CanvasLayout.vue
│   │
│   ├── router/              # 路由配置
│   │   ├── index.ts
│   │   ├── guards.ts        # 路由守卫
│   │   └── routes.ts        # 路由定义
│   │
│   ├── store/               # 状态管理
│   │   ├── modules/
│   │   │   ├── user.ts      # 用户状态
│   │   │   ├── project.ts   # 项目状态
│   │   │   ├── canvas.ts    # 画布状态
│   │   │   └── design.ts    # 设计状态
│   │   └── index.ts
│   │
│   ├── types/               # TypeScript类型定义
│   │   ├── user.d.ts
│   │   ├── project.d.ts
│   │   ├── canvas.d.ts
│   │   └── design.d.ts
│   │
│   ├── utils/               # 工具函数
│   │   ├── auth.ts          # 认证工具
│   │   ├── canvas.ts        # 画布工具
│   │   ├── storage.ts       # 本地存储
│   │   └── format.ts        # 格式化工具
│   │
│   ├── views/               # 页面组件
│   │   ├── auth/            # 认证相关页面
│   │   │   ├── Login.vue
│   │   │   ├── Register.vue
│   │   │   └── Forgot.vue
│   │   ├── workspace/       # 工作空间
│   │   │   ├── Projects.vue
│   │   │   ├── Editor.vue
│   │   │   └── Preview.vue
│   │   ├── gallery/         # 灵感广场
│   │   │   ├── Gallery.vue
│   │   │   ├── Detail.vue
│   │   │   └── Templates.vue
│   │   └── home/            # 首页
│   │       └── Home.vue
│   │
│   ├── App.vue              # 根组件
│   └── main.ts              # 入口文件
│
├── public/                  # 公共文件
├── tests/                   # 测试文件
├── .env                     # 环境变量
├── vite.config.ts           # Vite配置
├── tsconfig.json            # TypeScript配置
└── package.json             # 项目配置
```

## 3. 核心功能模块设计

### 3.1 用户管理模块

```typescript
// stores/user.ts
export const useUserStore = defineStore('user', {
  state: () => ({
    userInfo: null,
    token: '',
    permissions: []
  }),
  
  actions: {
    async login(credentials: LoginForm) {
      const { data } = await userApi.login(credentials)
      this.token = data.token
      this.userInfo = data.userInfo
      // 存储到本地
      storage.set('token', data.token)
    },
    
    async logout() {
      await userApi.logout()
      this.reset()
      router.push('/login')
    }
  }
})
```

### 3.2 LUI（Language User Interface）模块

```vue
<!-- components/lui/ChatInput.vue -->
<template>
  <div class="lui-container">
    <div class="prompt-suggestions" v-if="showSuggestions">
      <div v-for="item in suggestions" :key="item.id" 
           @click="selectSuggestion(item)">
        {{ item.text }}
      </div>
    </div>
    
    <div class="input-wrapper">
      <el-input
        v-model="prompt"
        type="textarea"
        :placeholder="placeholder"
        @keyup.enter="handleSubmit"
      />
      
      <div class="input-actions">
        <el-button @click="showAdvanced = true">
          高级选项
        </el-button>
        <el-button type="primary" @click="handleSubmit">
          生成设计
        </el-button>
      </div>
    </div>
    
    <!-- 高级选项面板 -->
    <AdvancedOptions v-model:visible="showAdvanced" 
                     v-model:options="advancedOptions" />
  </div>
</template>

<script setup lang="ts">
import { useDesignGeneration } from '@/composables/useDesign'

const { generateDesign, suggestions } = useDesignGeneration()

const handleSubmit = async () => {
  await generateDesign({
    prompt: prompt.value,
    options: advancedOptions.value
  })
}
</script>
```

### 3.3 画布编辑模块

```typescript
// composables/useCanvas.ts
export function useCanvas() {
  const canvasRef = ref<HTMLCanvasElement>()
  const fabricCanvas = ref<fabric.Canvas>()
  
  const initCanvas = () => {
    fabricCanvas.value = new fabric.Canvas(canvasRef.value, {
      width: 800,
      height: 600,
      backgroundColor: '#ffffff'
    })
    
    // 初始化画布事件
    setupCanvasEvents()
  }
  
  const addText = (text: string, options?: fabric.ITextOptions) => {
    const textObj = new fabric.IText(text, {
      left: 100,
      top: 100,
      fontSize: 20,
      ...options
    })
    fabricCanvas.value?.add(textObj)
  }
  
  const addImage = async (url: string) => {
    fabric.Image.fromURL(url, (img) => {
      img.scaleToWidth(200)
      fabricCanvas.value?.add(img)
    })
  }
  
  const exportCanvas = (format: 'png' | 'jpg' = 'png') => {
    return fabricCanvas.value?.toDataURL({
      format,
      quality: 1
    })
  }
  
  return {
    canvasRef,
    initCanvas,
    addText,
    addImage,
    exportCanvas
  }
}
```

### 3.4 实时生成状态展示

```typescript
// composables/useWebSocket.ts
export function useGenerationStatus() {
  const socket = ref<Socket>()
  const status = ref<GenerationStatus>({
    phase: 'waiting',
    progress: 0,
    message: ''
  })
  
  const connect = (taskId: string) => {
    socket.value = io(WS_URL, {
      query: { taskId }
    })
    
    socket.value.on('status', (data: GenerationStatus) => {
      status.value = data
    })
    
    socket.value.on('complete', (result: GenerationResult) => {
      // 处理生成完成
      handleGenerationComplete(result)
    })
    
    socket.value.on('error', (error: any) => {
      // 处理错误
      ElMessage.error(error.message)
    })
  }
  
  const disconnect = () => {
    socket.value?.disconnect()
  }
  
  return {
    status: readonly(status),
    connect,
    disconnect
  }
}
```

## 4. 性能优化策略

### 4.1 路由懒加载

```typescript
const routes = [
  {
    path: '/editor',
    component: () => import('@/views/workspace/Editor.vue')
  },
  {
    path: '/gallery',
    component: () => import('@/views/gallery/Gallery.vue')
  }
]
```

### 4.2 图片优化

```typescript
// utils/image.ts
export const optimizeImage = async (file: File): Promise<Blob> => {
  const canvas = document.createElement('canvas')
  const ctx = canvas.getContext('2d')
  const img = new Image()
  
  return new Promise((resolve) => {
    img.onload = () => {
      // 限制最大尺寸
      const maxSize = 1920
      let { width, height } = img
      
      if (width > maxSize || height > maxSize) {
        const ratio = Math.min(maxSize / width, maxSize / height)
        width *= ratio
        height *= ratio
      }
      
      canvas.width = width
      canvas.height = height
      ctx?.drawImage(img, 0, 0, width, height)
      
      canvas.toBlob((blob) => {
        resolve(blob!)
      }, 'image/jpeg', 0.85)
    }
    
    img.src = URL.createObjectURL(file)
  })
}
```

### 4.3 虚拟滚动

```vue
<!-- 灵感广场使用虚拟滚动 -->
<template>
  <VirtualList
    :items="galleryItems"
    :item-height="300"
    :buffer="5"
  >
    <template #default="{ item }">
      <GalleryCard :data="item" />
    </template>
  </VirtualList>
</template>
```

## 5. 安全策略

### 5.1 XSS防护

```typescript
// 使用DOMPurify清理用户输入
import DOMPurify from 'dompurify'

export const sanitizeHTML = (dirty: string): string => {
  return DOMPurify.sanitize(dirty, {
    ALLOWED_TAGS: ['b', 'i', 'em', 'strong', 'a'],
    ALLOWED_ATTR: ['href']
  })
}
```

### 5.2 请求拦截

```typescript
// api/request.ts
axios.interceptors.request.use(
  config => {
    // 添加token
    const token = storage.get('token')
    if (token) {
      config.headers.Authorization = `Bearer ${token}`
    }
    
    // CSRF token
    config.headers['X-CSRF-Token'] = getCsrfToken()
    
    return config
  },
  error => {
    return Promise.reject(error)
  }
)
```

## 6. 测试策略

### 6.1 单元测试

```typescript
// tests/unit/components/ChatInput.spec.ts
import { mount } from '@vue/test-utils'
import ChatInput from '@/components/lui/ChatInput.vue'

describe('ChatInput', () => {
  it('should emit generate event on submit', async () => {
    const wrapper = mount(ChatInput)
    const input = wrapper.find('textarea')
    
    await input.setValue('创建一个蓝色商务风格海报')
    await wrapper.find('.submit-btn').trigger('click')
    
    expect(wrapper.emitted('generate')).toBeTruthy()
    expect(wrapper.emitted('generate')[0]).toEqual([{
      prompt: '创建一个蓝色商务风格海报'
    }])
  })
})
```

### 6.2 E2E测试

```typescript
// tests/e2e/design-flow.spec.ts
import { test, expect } from '@playwright/test'

test('complete design generation flow', async ({ page }) => {
  // 登录
  await page.goto('/login')
  await page.fill('[name="username"]', 'testuser')
  await page.fill('[name="password"]', 'testpass')
  await page.click('[type="submit"]')
  
  // 创建设计
  await page.goto('/editor')
  await page.fill('.lui-input', '生成一个生日派对邀请函')
  await page.click('.generate-btn')
  
  // 等待生成完成
  await expect(page.locator('.generation-complete')).toBeVisible({
    timeout: 30000
  })
  
  // 验证结果
  await expect(page.locator('.canvas-container img')).toBeVisible()
})
```