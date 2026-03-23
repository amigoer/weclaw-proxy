import { useState, useEffect, useCallback } from 'react'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { Badge } from '@/components/ui/badge'
import { Separator } from '@/components/ui/separator'
import { fetchRoutes, updateRoutes, fetchAdapters } from '@/lib/api'

// 路由规则类型
interface RouteRule {
  match: {
    prefix?: string
    user_ids?: string[]
  }
  adapter: string
}

interface RoutingConfig {
  default_adapter: string
  rules: RouteRule[]
}

export function RoutesPage() {
  const [routing, setRouting] = useState<RoutingConfig>({ default_adapter: '', rules: [] })
  const [adapterNames, setAdapterNames] = useState<string[]>([])
  const [newRule, setNewRule] = useState({ prefix: '', adapter: '' })
  const [saving, setSaving] = useState(false)

  const load = useCallback(async () => {
    try {
      const [routeData, adapterData] = await Promise.all([fetchRoutes(), fetchAdapters()])
      setRouting(routeData || { default_adapter: '', rules: [] })
      setAdapterNames((adapterData || []).map((a: { name: string }) => a.name))
    } catch {
      // 忽略
    }
  }, [])

  useEffect(() => {
    load()
  }, [load])

  // 保存路由配置
  const handleSave = async () => {
    setSaving(true)
    try {
      await updateRoutes(routing as unknown as Record<string, unknown>)
    } finally {
      setSaving(false)
    }
  }

  // 添加路由规则
  const handleAddRule = () => {
    if (!newRule.prefix.trim() || !newRule.adapter) return
    setRouting({
      ...routing,
      rules: [...routing.rules, {
        match: { prefix: newRule.prefix.trim() },
        adapter: newRule.adapter,
      }],
    })
    setNewRule({ prefix: '', adapter: '' })
  }

  // 删除路由规则
  const handleDeleteRule = (index: number) => {
    setRouting({
      ...routing,
      rules: routing.rules.filter((_, i) => i !== index),
    })
  }

  return (
    <div className="space-y-6">
      {/* 默认 Agent */}
      <Card>
        <CardHeader>
          <CardTitle>默认 Agent</CardTitle>
          <CardDescription>
            当消息不匹配任何路由规则时，使用此 Agent 处理
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className="flex items-center gap-4">
            <Select
              value={routing.default_adapter}
              onValueChange={(v) => { if (v) setRouting({ ...routing, default_adapter: v }) }}
            >
              <SelectTrigger className="w-64">
                <SelectValue placeholder="选择默认 Agent" />
              </SelectTrigger>
              <SelectContent>
                {adapterNames.map(name => (
                  <SelectItem key={name} value={name}>{name}</SelectItem>
                ))}
              </SelectContent>
            </Select>
            <Button onClick={handleSave} disabled={saving}>
              {saving ? '保存中...' : '保存'}
            </Button>
          </div>
        </CardContent>
      </Card>

      {/* 路由规则 */}
      <Card>
        <CardHeader>
          <CardTitle>路由规则</CardTitle>
          <CardDescription>
            按消息前缀匹配路由到不同的 Agent，规则按顺序优先匹配
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-6">
          {/* 规则列表 */}
          {routing.rules.length > 0 ? (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead className="w-12">#</TableHead>
                  <TableHead>匹配前缀</TableHead>
                  <TableHead>目标 Agent</TableHead>
                  <TableHead className="text-right">操作</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {routing.rules.map((rule, i) => (
                  <TableRow key={i}>
                    <TableCell className="text-muted-foreground">{i + 1}</TableCell>
                    <TableCell>
                      <Badge variant="outline" className="font-mono">
                        {rule.match.prefix || '(无前缀)'}
                      </Badge>
                    </TableCell>
                    <TableCell className="font-medium">{rule.adapter}</TableCell>
                    <TableCell className="text-right">
                      <Button
                        variant="ghost"
                        size="sm"
                        className="text-destructive"
                        onClick={() => handleDeleteRule(i)}
                      >
                        删除
                      </Button>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          ) : (
            <div className="text-center py-8 text-muted-foreground">
              <p>暂无路由规则，所有消息将发送到默认 Agent</p>
            </div>
          )}

          <Separator />

          {/* 添加新规则 */}
          <div className="flex items-end gap-4">
            <div className="space-y-2 flex-1">
              <Label htmlFor="rule-prefix">消息前缀</Label>
              <Input
                id="rule-prefix"
                value={newRule.prefix}
                onChange={e => setNewRule({ ...newRule, prefix: e.target.value })}
                placeholder="例如: /画图"
              />
            </div>
            <div className="space-y-2 w-48">
              <Label>目标 Agent</Label>
              <Select
                value={newRule.adapter}
                onValueChange={(v) => { if (v) setNewRule({ ...newRule, adapter: v }) }}
              >
                <SelectTrigger>
                  <SelectValue placeholder="选择 Agent" />
                </SelectTrigger>
                <SelectContent>
                  {adapterNames.map(name => (
                    <SelectItem key={name} value={name}>{name}</SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <Button onClick={handleAddRule} variant="secondary">
              添加规则
            </Button>
          </div>

          {routing.rules.length > 0 && (
            <div className="flex justify-end">
              <Button onClick={handleSave} disabled={saving}>
                {saving ? '保存中...' : '保存路由规则'}
              </Button>
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  )
}
