import { useState, useEffect, useCallback } from 'react'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Card, CardContent, CardDescription, CardHeader } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Separator } from '@/components/ui/separator'
import { AdaptersPage } from './pages/Adapters'
import { RoutesPage } from './pages/Routes'
import { fetchStatus } from './lib/api'

// 系统状态类型
interface StatusInfo {
  weixin_connected: boolean
  account_id: string
  adapter_count: number
  active_sessions: number
  uptime: string
}

export default function App() {
  const [status, setStatus] = useState<StatusInfo | null>(null)

  const loadStatus = useCallback(async () => {
    try {
      const data = await fetchStatus()
      setStatus(data)
    } catch {
      // 后端未启动时忽略
    }
  }, [])

  useEffect(() => {
    loadStatus()
    const timer = setInterval(loadStatus, 5000)
    return () => clearInterval(timer)
  }, [loadStatus])

  return (
    <div className="min-h-screen bg-background text-foreground">
      {/* 顶部导航 */}
      <header className="border-b border-border bg-card/50 backdrop-blur-sm sticky top-0 z-50">
        <div className="max-w-6xl mx-auto px-6 py-4 flex items-center justify-between">
          <div className="flex items-center gap-3">
            <div className="w-8 h-8 bg-primary rounded-lg flex items-center justify-center text-primary-foreground font-bold text-sm">
              W
            </div>
            <h1 className="text-xl font-semibold tracking-tight">WeClaw-Proxy</h1>
            <Badge variant="secondary" className="text-xs">Admin</Badge>
          </div>
          <div className="flex items-center gap-3">
            {status && (
              <Badge variant={status.weixin_connected ? 'default' : 'destructive'}>
                {status.weixin_connected ? '微信已连接' : '微信未连接'}
              </Badge>
            )}
          </div>
        </div>
      </header>

      <main className="max-w-6xl mx-auto px-6 py-8">
        {/* 状态卡片 */}
        <div className="grid grid-cols-1 md:grid-cols-4 gap-4 mb-8">
          <Card>
            <CardHeader className="pb-2">
              <CardDescription>连接状态</CardDescription>
            </CardHeader>
            <CardContent>
              <div className="flex items-center gap-2">
                <div className={`w-2.5 h-2.5 rounded-full ${status?.weixin_connected ? 'bg-green-500' : 'bg-red-500'}`} />
                <span className="text-lg font-semibold">
                  {status?.weixin_connected ? '在线' : '离线'}
                </span>
              </div>
            </CardContent>
          </Card>

          <Card>
            <CardHeader className="pb-2">
              <CardDescription>已注册 Agent</CardDescription>
            </CardHeader>
            <CardContent>
              <span className="text-2xl font-bold">{status?.adapter_count ?? 0}</span>
            </CardContent>
          </Card>

          <Card>
            <CardHeader className="pb-2">
              <CardDescription>活跃会话</CardDescription>
            </CardHeader>
            <CardContent>
              <span className="text-2xl font-bold">{status?.active_sessions ?? 0}</span>
            </CardContent>
          </Card>

          <Card>
            <CardHeader className="pb-2">
              <CardDescription>账号 ID</CardDescription>
            </CardHeader>
            <CardContent>
              <span className="text-sm font-mono text-muted-foreground truncate block">
                {status?.account_id || '-'}
              </span>
            </CardContent>
          </Card>
        </div>

        <Separator className="mb-8" />

        {/* 功能标签页 */}
        <Tabs defaultValue="adapters" className="space-y-6">
          <TabsList>
            <TabsTrigger value="adapters">Agent 管理</TabsTrigger>
            <TabsTrigger value="routes">路由规则</TabsTrigger>
          </TabsList>

          <TabsContent value="adapters">
            <AdaptersPage onUpdate={loadStatus} />
          </TabsContent>

          <TabsContent value="routes">
            <RoutesPage />
          </TabsContent>
        </Tabs>
      </main>
    </div>
  )
}
