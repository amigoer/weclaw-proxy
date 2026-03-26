import { useState, useEffect, useCallback } from 'react'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { Badge } from '@/components/ui/badge'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle, DialogTrigger } from '@/components/ui/dialog'
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from '@/components/ui/alert-dialog'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { fetchAdapters, createAdapter, updateAdapter, deleteAdapter, fetchModels } from '@/lib/api'

// 适配器类型定义
interface AdapterConfig {
  name: string
  type: string
  api_key?: string
  base_url?: string
  model?: string
  system_prompt?: string
  max_tokens?: number
  temperature?: number
  extra?: Record<string, string>
}

// 支持的适配器类型
const ADAPTER_TYPES = [
  { value: 'openai', label: 'OpenAI 兼容', desc: '支持 OpenAI、DeepSeek、通义千问、Ollama 等' },
  { value: 'gemini', label: 'Gemini', desc: 'Google Gemini API（支持搜索工具）' },
  { value: 'webhook', label: 'Webhook', desc: '转发到自定义 HTTP 端点（对接任意 Agent）' },
  { value: 'dify', label: 'Dify', desc: 'Dify Agent / 工作流' },
  { value: 'coze', label: 'Coze', desc: 'Coze Bot 平台' },
  { value: 'cli', label: 'CLI Agent', desc: '本地 CLI 工具（Codex、Claude Code、Gemini CLI）' },
]

// 类型颜色映射
const TYPE_COLORS: Record<string, 'default' | 'secondary' | 'outline'> = {
  openai: 'default',
  gemini: 'default',
  webhook: 'secondary',
  dify: 'outline',
  coze: 'outline',
  cli: 'secondary',
}

// 预置常见模型列表
const PRESET_MODELS = [
  'gpt-4o',
  'gpt-4o-mini',
  'gpt-4.1',
  'gpt-4.1-mini',
  'gpt-4.1-nano',
  'o3-mini',
  'o4-mini',
  'claude-sonnet-4-20250514',
  'claude-3-7-sonnet-20250219',
  'deepseek-chat',
  'deepseek-reasoner',
  'gemini-2.5-pro-preview-05-06',
  'gemini-2.5-flash-preview-04-17',
  'qwen-plus',
  'qwen-turbo',
]

// 模板变量定义
const TEMPLATE_VARS = [
  { name: '{cur_date}', desc: '当前日期' },
  { name: '{cur_time}', desc: '当前时间' },
  { name: '{cur_datetime}', desc: '日期时间' },
  { name: '{model_id}', desc: '模型 ID' },
  { name: '{model_name}', desc: '模型名称' },
  { name: '{locale}', desc: '语言环境' },
]

// 常用 Prompt 片段
const PROMPT_SNIPPETS = [
  { label: '📱 微信助手', text: '你是一个友好的微信智能助手。请用简洁的中文回复，避免使用 Markdown 格式。' },
  { label: '🕐 时间感知', text: '当前时间：{cur_datetime}。请在需要时参考当前时间信息。' },
  { label: '🎭 角色扮演', text: '你是一个专业的[领域]顾问。请基于你的专业知识，为用户提供准确、实用的建议。' },
  { label: '✂️ 简洁回复', text: '请用简洁明了的语言回复，每次回复控制在 200 字以内。不要使用 Markdown 格式。' },
  { label: '💬 多轮上下文', text: '请记住之前的对话内容，保持上下文连贯性。如果用户的问题不清楚，请主动追问。' },
  { label: '🛡️ 安全边界', text: '如果用户的请求涉及违法、有害或不当内容，请礼貌拒绝并说明原因。' },
]

interface Props {
  onUpdate: () => void
}

export function AdaptersPage({ onUpdate }: Props) {
  const [adapters, setAdapters] = useState<AdapterConfig[]>([])
  const [dialogOpen, setDialogOpen] = useState(false)
  const [editingAdapter, setEditingAdapter] = useState<AdapterConfig | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<string | null>(null)
  const [form, setForm] = useState<AdapterConfig>({
    name: '',
    type: 'openai',
    api_key: '',
    base_url: '',
    model: 'gpt-4o',
    system_prompt: '',
    max_tokens: 4096,
    temperature: 0.7,
  })
  const [modelOptions, setModelOptions] = useState<string[]>(PRESET_MODELS)
  const [loadingModels, setLoadingModels] = useState(false)

  // 在 Textarea 光标位置插入文本
  const insertAtCursor = (textareaId: string, text: string) => {
    const el = document.getElementById(textareaId) as HTMLTextAreaElement
    if (!el) return
    const start = el.selectionStart
    const end = el.selectionEnd
    const value = form.system_prompt || ''
    const newValue = value.slice(0, start) + text + value.slice(end)
    setForm({ ...form, system_prompt: newValue })
    requestAnimationFrame(() => {
      el.focus()
      el.setSelectionRange(start + text.length, start + text.length)
    })
  }

  // 追加 prompt 片段到末尾
  const appendSnippet = (textareaId: string, text: string) => {
    const current = form.system_prompt || ''
    const newValue = current ? current.trimEnd() + '\n' + text : text
    setForm({ ...form, system_prompt: newValue })
    requestAnimationFrame(() => {
      const el = document.getElementById(textareaId) as HTMLTextAreaElement
      if (el) {
        el.focus()
        el.setSelectionRange(newValue.length, newValue.length)
      }
    })
  }

  const loadAdapters = useCallback(async () => {
    try {
      const data = await fetchAdapters()
      setAdapters(data || [])
    } catch {
      // 忽略加载错误
    }
  }, [])

  useEffect(() => {
    loadAdapters()
  }, [loadAdapters])

  // 打开新建对话框
  const handleAdd = () => {
    setEditingAdapter(null)
    setForm({
      name: '',
      type: 'openai',
      api_key: '',
      base_url: '',
      model: 'gpt-4o',
      system_prompt: '',
      max_tokens: 4096,
      temperature: 0.7,
    })
    setDialogOpen(true)
  }

  // 打开编辑对话框
  const handleEdit = (adapter: AdapterConfig) => {
    setEditingAdapter(adapter)
    setForm({ ...adapter })
    setDialogOpen(true)
  }

  // 删除适配器
  const handleDelete = async () => {
    if (!deleteTarget) return
    await deleteAdapter(deleteTarget)
    setDeleteTarget(null)
    await loadAdapters()
    onUpdate()
  }

  // 保存适配器
  const handleSave = async () => {
    if (!form.name.trim()) return
    if (form.type === 'cli' && !form.extra?.work_dir?.trim()) return

    try {
      if (editingAdapter) {
        await updateAdapter(editingAdapter.name, form as unknown as Record<string, unknown>)
      } else {
        await createAdapter(form as unknown as Record<string, unknown>)
      }

      setDialogOpen(false)
      await loadAdapters()
      onUpdate()
    } catch (err) {
      console.error('保存适配器失败:', err)
    }
  }

  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-4">
        <div>
          <CardTitle>Agent 适配器</CardTitle>
          <CardDescription className="mt-1">
            管理接入的 AI Agent，支持 OpenAI、Webhook 等多种类型
          </CardDescription>
        </div>
        <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
          <DialogTrigger>
            <Button onClick={handleAdd}>+ 添加 Agent</Button>
          </DialogTrigger>
          <DialogContent className="sm:max-w-lg">
            <DialogHeader>
              <DialogTitle>{editingAdapter ? '编辑 Agent' : '添加 Agent'}</DialogTitle>
              <DialogDescription>
                配置 Agent 的连接信息
              </DialogDescription>
            </DialogHeader>

            <div className="space-y-4 py-4 overflow-y-auto flex-1 min-h-0">
              <div className="grid grid-cols-4 items-center gap-4">
                <Label htmlFor="adapter-name" className="text-right">名称</Label>
                <Input
                  id="adapter-name"
                  value={form.name}
                  onChange={e => setForm({ ...form, name: e.target.value })}
                  className="col-span-3"
                  placeholder="my-agent"
                  disabled={!!editingAdapter}
                />
              </div>

              <div className="grid grid-cols-4 items-center gap-4">
                <Label className="text-right">类型</Label>
                <Select value={form.type} onValueChange={(v: string | null) => { if (v) setForm({ ...form, type: v }) }}>
                  <SelectTrigger className="col-span-3">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent className="w-80">
                    {ADAPTER_TYPES.map(t => (
                      <SelectItem key={t.value} value={t.value}>
                        <div className="flex flex-col">
                          <span className="font-medium">{t.label}</span>
                          <span className="text-muted-foreground text-xs">{t.desc}</span>
                        </div>
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>

              {form.type === 'cli' ? (
                <>
                  <div className="grid grid-cols-4 items-center gap-4">
                    <Label htmlFor="adapter-cmd" className="text-right">命令路径</Label>
                    <Input
                      id="adapter-cmd"
                      value={form.base_url}
                      onChange={e => setForm({ ...form, base_url: e.target.value })}
                      className="col-span-3"
                      placeholder="codex / claude / gemini"
                    />
                  </div>

                  <div className="grid grid-cols-4 items-center gap-4">
                    <Label htmlFor="adapter-args" className="text-right">子命令参数</Label>
                    <Input
                      id="adapter-args"
                      value={form.extra?.args || ''}
                      onChange={e => setForm({ ...form, extra: { ...form.extra, args: e.target.value } })}
                      className="col-span-3"
                      placeholder="自动推断（codex→exec, claude→-p）"
                    />
                  </div>

                  <div className="grid grid-cols-4 items-center gap-4">
                    <Label htmlFor="adapter-timeout" className="text-right">超时(秒)</Label>
                    <Input
                      id="adapter-timeout"
                      value={form.extra?.timeout || ''}
                      onChange={e => setForm({ ...form, extra: { ...form.extra, timeout: e.target.value } })}
                      className="col-span-3"
                      placeholder="120"
                    />
                  </div>

                  <div className="grid grid-cols-4 items-center gap-4">
                    <Label htmlFor="adapter-workdir" className="text-right">
                      工作目录 <span className="text-destructive">*</span>
                    </Label>
                    <Input
                      id="adapter-workdir"
                      value={form.extra?.work_dir || ''}
                      onChange={e => setForm({ ...form, extra: { ...form.extra, work_dir: e.target.value } })}
                      className="col-span-3"
                      placeholder="CLI 执行时的工作目录（如 /path/to/project）"
                      required
                    />
                  </div>
                </>
              ) : (
                <>
                  <div className="grid grid-cols-4 items-center gap-4">
                    <Label htmlFor="adapter-url" className="text-right">API 地址</Label>
                    <Input
                      id="adapter-url"
                      value={form.base_url}
                      onChange={e => setForm({ ...form, base_url: e.target.value })}
                      className="col-span-3"
                      placeholder={form.type === 'openai' ? 'https://api.openai.com/v1' : 'https://your-agent.com/chat'}
                    />
                  </div>

                  <div className="grid grid-cols-4 items-center gap-4">
                    <Label htmlFor="adapter-key" className="text-right">API Key</Label>
                    <Input
                      id="adapter-key"
                      type="password"
                      value={form.api_key}
                      onChange={e => setForm({ ...form, api_key: e.target.value })}
                      className="col-span-3"
                      placeholder="sk-..."
                    />
                  </div>
                </>
              )}

              {form.type === 'openai' && (
                <>
                  <div className="grid grid-cols-4 items-center gap-4">
                    <Label htmlFor="adapter-model" className="text-right">模型</Label>
                    <div className="col-span-3 flex gap-2">
                      <div className="relative flex-1">
                        <Input
                          id="adapter-model"
                          value={form.model}
                          onChange={e => setForm({ ...form, model: e.target.value })}
                          placeholder="输入或选择模型"
                          list="model-options"
                        />
                        <datalist id="model-options">
                          {modelOptions.map(m => (
                            <option key={m} value={m} />
                          ))}
                        </datalist>
                      </div>
                      <Button
                        type="button"
                        variant="outline"
                        size="sm"
                        className="shrink-0 text-xs px-3"
                        disabled={loadingModels || !form.base_url}
                        onClick={async () => {
                          setLoadingModels(true)
                          const models = await fetchModels(form.base_url || '', form.api_key || '')
                          if (models.length > 0) {
                            setModelOptions(models)
                          }
                          setLoadingModels(false)
                        }}
                      >
                        {loadingModels ? '加载中...' : '获取模型'}
                      </Button>
                    </div>
                  </div>

                  <div className="grid grid-cols-4 items-start gap-4">
                    <Label htmlFor="adapter-prompt" className="text-right pt-3">系统提示</Label>
                    <div className="col-span-3 space-y-2">
                      <Textarea
                        id="adapter-prompt"
                        value={form.system_prompt}
                        onChange={e => setForm({ ...form, system_prompt: e.target.value })}
                        rows={3}
                        placeholder="你是一个友好的微信助手..."
                      />
                      <div className="flex flex-wrap gap-1">
                        <span className="text-muted-foreground text-xs leading-6">插入变量：</span>
                        {TEMPLATE_VARS.map(v => (
                          <button
                            key={v.name}
                            type="button"
                            title={v.desc}
                            className="inline-flex items-center rounded-md bg-muted px-1.5 py-0.5 font-mono text-[11px] text-muted-foreground hover:bg-primary/10 hover:text-primary cursor-pointer transition-colors"
                            onClick={() => insertAtCursor('adapter-prompt', v.name)}
                          >
                            {v.name}
                          </button>
                        ))}
                      </div>
                      <details className="group">
                        <summary className="text-xs text-muted-foreground cursor-pointer hover:text-foreground transition-colors select-none">
                          常用 Prompt 片段
                        </summary>
                        <div className="flex flex-wrap gap-1.5 mt-2">
                          {PROMPT_SNIPPETS.map(s => (
                            <button
                              key={s.label}
                              type="button"
                              title={s.text}
                              className="inline-flex items-center rounded-md border border-border bg-background px-2 py-1 text-xs hover:bg-primary/10 hover:border-primary/30 hover:text-primary cursor-pointer transition-colors"
                              onClick={() => appendSnippet('adapter-prompt', s.text)}
                            >
                              {s.label}
                            </button>
                          ))}
                        </div>
                      </details>
                    </div>
                  </div>
                </>
              )}

              {form.type === 'gemini' && (
                <>
                  <div className="grid grid-cols-4 items-center gap-4">
                    <Label htmlFor="gemini-model" className="text-right">模型</Label>
                    <Input
                      id="gemini-model"
                      value={form.model}
                      onChange={e => setForm({ ...form, model: e.target.value })}
                      className="col-span-3"
                      placeholder="gemini-2.5-flash-preview-04-17"
                    />
                  </div>

                  <div className="grid grid-cols-4 items-start gap-4">
                    <Label htmlFor="gemini-prompt" className="text-right pt-3">系统提示</Label>
                    <div className="col-span-3 space-y-2">
                      <Textarea
                        id="gemini-prompt"
                        value={form.system_prompt}
                        onChange={e => setForm({ ...form, system_prompt: e.target.value })}
                        rows={3}
                        placeholder="你是一个友好的微信助手..."
                      />
                      <div className="flex flex-wrap gap-1">
                        <span className="text-muted-foreground text-xs leading-6">插入变量：</span>
                        {TEMPLATE_VARS.map(v => (
                          <button
                            key={v.name}
                            type="button"
                            title={v.desc}
                            className="inline-flex items-center rounded-md bg-muted px-1.5 py-0.5 font-mono text-[11px] text-muted-foreground hover:bg-primary/10 hover:text-primary cursor-pointer transition-colors"
                            onClick={() => insertAtCursor('gemini-prompt', v.name)}
                          >
                            {v.name}
                          </button>
                        ))}
                      </div>
                      <details className="group">
                        <summary className="text-xs text-muted-foreground cursor-pointer hover:text-foreground transition-colors select-none">
                          常用 Prompt 片段
                        </summary>
                        <div className="flex flex-wrap gap-1.5 mt-2">
                          {PROMPT_SNIPPETS.map(s => (
                            <button
                              key={s.label}
                              type="button"
                              title={s.text}
                              className="inline-flex items-center rounded-md border border-border bg-background px-2 py-1 text-xs hover:bg-primary/10 hover:border-primary/30 hover:text-primary cursor-pointer transition-colors"
                              onClick={() => appendSnippet('gemini-prompt', s.text)}
                            >
                              {s.label}
                            </button>
                          ))}
                        </div>
                      </details>
                    </div>
                  </div>

                  <div className="grid grid-cols-4 items-center gap-4">
                    <Label className="text-right">搜索工具</Label>
                    <div className="col-span-3 flex items-center gap-2">
                      <input
                        type="checkbox"
                        id="gemini-search"
                        checked={form.extra?.enable_search === 'true'}
                        onChange={e => setForm({
                          ...form,
                          extra: { ...form.extra, enable_search: e.target.checked ? 'true' : '' }
                        })}
                        className="h-4 w-4 rounded border-input"
                      />
                      <Label htmlFor="gemini-search" className="text-sm font-normal text-muted-foreground">
                        启用 Google Search（AI 可联网搜索最新信息）
                      </Label>
                    </div>
                  </div>
                </>
              )}
            </div>

            <DialogFooter>
              <Button variant="outline" onClick={() => setDialogOpen(false)}>取消</Button>
              <Button onClick={handleSave}>{editingAdapter ? '保存' : '添加'}</Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      </CardHeader>

      <CardContent>
        {adapters.length === 0 ? (
          <div className="text-center py-12 text-muted-foreground">
            <p className="text-lg mb-2">还没有配置任何 Agent</p>
            <p className="text-sm">点击上方「添加 Agent」开始接入你的 AI Agent</p>
          </div>
        ) : (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>名称</TableHead>
                <TableHead>类型</TableHead>
                <TableHead>API 地址</TableHead>
                <TableHead>模型</TableHead>
                <TableHead className="text-right">操作</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {adapters.map(adapter => (
                <TableRow key={adapter.name}>
                  <TableCell className="font-medium">{adapter.name}</TableCell>
                  <TableCell>
                    <Badge variant={TYPE_COLORS[adapter.type] || 'outline'}>
                      {ADAPTER_TYPES.find(t => t.value === adapter.type)?.label || adapter.type}
                    </Badge>
                  </TableCell>
                  <TableCell className="text-muted-foreground text-sm font-mono max-w-48 truncate">
                    {adapter.base_url || '-'}
                  </TableCell>
                  <TableCell className="text-sm">{adapter.model || '-'}</TableCell>
                  <TableCell className="text-right space-x-2">
                    <Button variant="ghost" size="sm" onClick={() => handleEdit(adapter)}>
                      编辑
                    </Button>
                    <Button variant="ghost" size="sm" className="text-destructive" onClick={() => setDeleteTarget(adapter.name)}>
                      删除
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}
      </CardContent>

      {/* 删除确认弹窗 */}
      <AlertDialog open={!!deleteTarget} onOpenChange={(open: boolean) => { if (!open) setDeleteTarget(null) }}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>确认删除</AlertDialogTitle>
            <AlertDialogDescription>
              确定要删除适配器「{deleteTarget}」吗？此操作不可撤销。
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>取消</AlertDialogCancel>
            <AlertDialogAction onClick={handleDelete} className="bg-destructive text-destructive-foreground hover:bg-destructive/90">
              删除
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </Card>
  )
}
