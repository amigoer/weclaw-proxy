import { useState, useEffect, useCallback } from 'react'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { Badge } from '@/components/ui/badge'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle, DialogTrigger } from '@/components/ui/dialog'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { fetchAdapters, createAdapter, updateAdapter, deleteAdapter } from '@/lib/api'

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
}

// 支持的适配器类型
const ADAPTER_TYPES = [
  { value: 'openai', label: 'OpenAI 兼容', desc: '支持 OpenAI、DeepSeek、通义千问、Ollama 等' },
  { value: 'webhook', label: 'Webhook', desc: '转发到自定义 HTTP 端点（对接任意 Agent）' },
  { value: 'dify', label: 'Dify', desc: 'Dify Agent / 工作流' },
  { value: 'coze', label: 'Coze', desc: 'Coze Bot 平台' },
]

// 类型颜色映射
const TYPE_COLORS: Record<string, 'default' | 'secondary' | 'outline'> = {
  openai: 'default',
  webhook: 'secondary',
  dify: 'outline',
  coze: 'outline',
}

interface Props {
  onUpdate: () => void
}

export function AdaptersPage({ onUpdate }: Props) {
  const [adapters, setAdapters] = useState<AdapterConfig[]>([])
  const [dialogOpen, setDialogOpen] = useState(false)
  const [editingAdapter, setEditingAdapter] = useState<AdapterConfig | null>(null)
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
  const handleDelete = async (name: string) => {
    if (!confirm(`确定要删除适配器 "${name}" 吗？`)) return
    await deleteAdapter(name)
    await loadAdapters()
    onUpdate()
  }

  // 保存适配器
  const handleSave = async () => {
    if (!form.name.trim()) return

    if (editingAdapter) {
      await updateAdapter(editingAdapter.name, form as unknown as Record<string, unknown>)
    } else {
      await createAdapter(form as unknown as Record<string, unknown>)
    }

    setDialogOpen(false)
    await loadAdapters()
    onUpdate()
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

            <div className="space-y-4 py-4">
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
                <Select value={form.type} onValueChange={(v) => { if (v) setForm({ ...form, type: v }) }}>
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

              {form.type === 'openai' && (
                <>
                  <div className="grid grid-cols-4 items-center gap-4">
                    <Label htmlFor="adapter-model" className="text-right">模型</Label>
                    <Input
                      id="adapter-model"
                      value={form.model}
                      onChange={e => setForm({ ...form, model: e.target.value })}
                      className="col-span-3"
                      placeholder="gpt-4o"
                    />
                  </div>

                  <div className="grid grid-cols-4 items-start gap-4">
                    <Label htmlFor="adapter-prompt" className="text-right pt-3">系统提示</Label>
                    <Textarea
                      id="adapter-prompt"
                      value={form.system_prompt}
                      onChange={e => setForm({ ...form, system_prompt: e.target.value })}
                      className="col-span-3"
                      rows={3}
                      placeholder="你是一个友好的微信助手..."
                    />
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
                    <Button variant="ghost" size="sm" className="text-destructive" onClick={() => handleDelete(adapter.name)}>
                      删除
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}
      </CardContent>
    </Card>
  )
}
