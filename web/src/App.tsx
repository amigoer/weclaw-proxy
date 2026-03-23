import { useState, useEffect, useCallback, useRef } from 'react'
import { QRCodeSVG } from 'qrcode.react'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Card, CardContent, CardDescription, CardHeader } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Separator } from '@/components/ui/separator'
import {
  Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle,
} from '@/components/ui/dialog'
import {
  AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent,
  AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle,
  AlertDialogTrigger,
} from '@/components/ui/alert-dialog'
import { AdaptersPage } from './pages/Adapters'
import { RoutesPage } from './pages/Routes'
import { fetchStatus, logout, startLogin, getLoginStatus } from './lib/api'

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
  const [loginDialogOpen, setLoginDialogOpen] = useState(false)
  const [qrUrl, setQrUrl] = useState('')
  const [loginStatus, setLoginStatus] = useState('')
  const [loginMessage, setLoginMessage] = useState('')
  const pollRef = useRef<ReturnType<typeof setInterval> | null>(null)
  const autoLoginTriggered = useRef(false)

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

  // 首次检测到未登录时，自动弹出扫码 Dialog
  useEffect(() => {
    if (status && !status.weixin_connected && !autoLoginTriggered.current) {
      autoLoginTriggered.current = true
      handleStartLogin()
    }
  }, [status])

  // 开始登录
  const handleStartLogin = async () => {
    setLoginDialogOpen(true)
    setLoginStatus('loading')
    setLoginMessage('正在获取二维码...')
    setQrUrl('')

    const res = await startLogin()
    if (res.qr_url) {
      setQrUrl(res.qr_url)
      setLoginStatus('wait')
      setLoginMessage('请使用微信扫描二维码')
      // 开始轮询登录状态
      startPolling()
    } else {
      setLoginStatus('error')
      setLoginMessage(res.error || '获取二维码失败')
    }
  }

  // 轮询登录状态
  const startPolling = () => {
    stopPolling()
    pollRef.current = setInterval(async () => {
      try {
        const res = await getLoginStatus()
        setLoginStatus(res.status)
        if (res.qr_url && res.qr_url !== qrUrl) {
          setQrUrl(res.qr_url)
        }

        switch (res.status) {
          case 'scaned':
            setLoginMessage('已扫码，请在微信中确认...')
            break
          case 'confirmed':
            setLoginMessage('✅ 登录成功！')
            stopPolling()
            setTimeout(() => {
              setLoginDialogOpen(false)
              loadStatus()
            }, 1500)
            break
          case 'expired':
            setLoginMessage('二维码已过期，正在刷新...')
            break
          case 'error':
            setLoginMessage(res.message || '登录失败')
            stopPolling()
            break
          default:
            setLoginMessage('请使用微信扫描二维码')
        }
      } catch {
        // 忽略轮询错误
      }
    }, 2000)
  }

  const stopPolling = () => {
    if (pollRef.current) {
      clearInterval(pollRef.current)
      pollRef.current = null
    }
  }

  // Dialog 关闭时停止轮询
  const handleLoginDialogChange = (open: boolean) => {
    setLoginDialogOpen(open)
    if (!open) stopPolling()
  }

  const handleLogout = async () => {
    await logout()
    loadStatus()
  }

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
              <Badge
                variant={status.weixin_connected ? 'default' : 'destructive'}
                className={status.weixin_connected ? 'bg-green-600 hover:bg-green-700' : ''}
              >
                {status.weixin_connected ? '微信已连接' : '微信未连接'}
              </Badge>
            )}
            {status && !status.weixin_connected && (
              <Button size="sm" onClick={handleStartLogin}>
                扫码登录
              </Button>
            )}
            {status?.weixin_connected && (
              <AlertDialog>
                <AlertDialogTrigger>
                  <Button variant="ghost" size="sm" className="text-muted-foreground">
                    退出绑定
                  </Button>
                </AlertDialogTrigger>
                <AlertDialogContent>
                  <AlertDialogHeader>
                    <AlertDialogTitle>退出微信绑定</AlertDialogTitle>
                    <AlertDialogDescription>
                      确定要退出微信绑定吗？退出后需要重新扫码登录才能继续使用。
                    </AlertDialogDescription>
                  </AlertDialogHeader>
                  <AlertDialogFooter>
                    <AlertDialogCancel>取消</AlertDialogCancel>
                    <AlertDialogAction onClick={handleLogout} className="bg-destructive text-destructive-foreground hover:bg-destructive/90">
                      确认退出
                    </AlertDialogAction>
                  </AlertDialogFooter>
                </AlertDialogContent>
              </AlertDialog>
            )}
          </div>
        </div>
      </header>

      {/* 扫码登录 Dialog */}
      <Dialog open={loginDialogOpen} onOpenChange={handleLoginDialogChange}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>微信扫码登录</DialogTitle>
            <DialogDescription>
              使用微信扫描下方二维码绑定账号
            </DialogDescription>
          </DialogHeader>
          <div className="flex flex-col items-center gap-4 py-4">
            {qrUrl ? (
              <div className="p-4 bg-white rounded-xl">
                <QRCodeSVG value={qrUrl} size={220} level="L" />
              </div>
            ) : (
              <div className="w-[220px] h-[220px] bg-muted rounded-xl flex items-center justify-center">
                <span className="text-muted-foreground text-sm">加载中...</span>
              </div>
            )}
            <div className="flex items-center gap-2 text-sm">
              {loginStatus === 'wait' && (
                <div className="w-2 h-2 rounded-full bg-blue-500 animate-pulse" />
              )}
              {loginStatus === 'scaned' && (
                <div className="w-2 h-2 rounded-full bg-yellow-500 animate-pulse" />
              )}
              {loginStatus === 'confirmed' && (
                <div className="w-2 h-2 rounded-full bg-green-500" />
              )}
              {loginStatus === 'error' && (
                <div className="w-2 h-2 rounded-full bg-red-500" />
              )}
              <span className="text-muted-foreground">{loginMessage}</span>
            </div>
            {loginStatus === 'error' && (
              <Button variant="outline" size="sm" onClick={handleStartLogin}>
                重新获取二维码
              </Button>
            )}
          </div>
        </DialogContent>
      </Dialog>

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
