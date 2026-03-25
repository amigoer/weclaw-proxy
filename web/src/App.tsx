import { useState, useEffect, useCallback, useRef } from 'react'
import { QRCodeSVG } from 'qrcode.react'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Card, CardContent, CardDescription, CardHeader } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Separator } from '@/components/ui/separator'
import {
  Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle,
} from '@/components/ui/dialog'
import {
  AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent,
  AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { AdaptersPage } from './pages/Adapters'
import { RoutesPage } from './pages/Routes'
import { fetchStatus, logoutAccount, renameAccount, startLogin, getLoginStatus } from './lib/api'

// 系统状态类型
interface AccountInfo {
  account_id: string
  nickname?: string
  connected: boolean
}

interface StatusInfo {
  weixin_connected: boolean
  account_id: string
  accounts?: AccountInfo[]
  adapter_count: number
  active_sessions: number
  smart_routing_enabled: boolean
  uptime: string
}

export default function App() {
  const [status, setStatus] = useState<StatusInfo | null>(null)
  const [activeTab, setActiveTab] = useState('adapters')
  const [loginDialogOpen, setLoginDialogOpen] = useState(false)
  const [qrUrl, setQrUrl] = useState('')
  const [loginStatus, setLoginStatus] = useState('')
  const [loginMessage, setLoginMessage] = useState('')
  const pollRef = useRef<ReturnType<typeof setInterval> | null>(null)
  const autoLoginTriggered = useRef(false)

  // 登录成功后的绑定信息
  const [confirmedAccountId, setConfirmedAccountId] = useState('')
  const [loginNickname, setLoginNickname] = useState('')

  // 账号备注弹窗状态
  const [renameOpen, setRenameOpen] = useState(false)
  const [renameTarget, setRenameTarget] = useState('')
  const [renameValue, setRenameValue] = useState('')

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
            setLoginMessage('✅ 绑定成功！')
            setConfirmedAccountId(res.account_id || '')
            setLoginNickname('')
            stopPolling()
            loadStatus()
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
    if (!open) {
      stopPolling()
      setConfirmedAccountId('')
      setLoginNickname('')
    }
  }

  // 登录成功后完成操作（保存备注并关闭）
  const handleLoginDone = async () => {
    if (loginNickname.trim() && confirmedAccountId) {
      await renameAccount(confirmedAccountId, loginNickname.trim())
    }
    setLoginDialogOpen(false)
    setConfirmedAccountId('')
    setLoginNickname('')
    loadStatus()
  }

  const handleLogoutAccount = async (accountID: string) => {
    await logoutAccount(accountID)
    setLogoutTarget('')
    loadStatus()
  }

  // 退出确认弹窗状态
  const [logoutTarget, setLogoutTarget] = useState('')

  const handleRenameAccount = (accountID: string, currentName?: string) => {
    setRenameTarget(accountID)
    setRenameValue(currentName || '')
    setRenameOpen(true)
  }

  const handleRenameSave = async () => {
    await renameAccount(renameTarget, renameValue)
    setRenameOpen(false)
    loadStatus()
  }

  const accounts = status?.accounts || []

  // 显示账号名称（优先备注，其次截短 ID）
  const displayName = (a: AccountInfo) => {
    if (a.nickname) return a.nickname
    const id = a.account_id
    return id.length > 12 ? id.slice(0, 8) + '...' : id
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
            <a
              href="https://github.com/amigoer/weclaw-proxy"
              target="_blank"
              rel="noopener noreferrer"
              className="text-muted-foreground hover:text-foreground transition-colors"
              title="GitHub"
            >
              <svg viewBox="0 0 24 24" width="20" height="20" fill="currentColor">
                <path d="M12 0c-6.626 0-12 5.373-12 12 0 5.302 3.438 9.8 8.207 11.387.599.111.793-.261.793-.577v-2.234c-3.338.726-4.033-1.416-4.033-1.416-.546-1.387-1.333-1.756-1.333-1.756-1.089-.745.083-.729.083-.729 1.205.084 1.839 1.237 1.839 1.237 1.07 1.834 2.807 1.304 3.492.997.107-.775.418-1.305.762-1.604-2.665-.305-5.467-1.334-5.467-5.931 0-1.311.469-2.381 1.236-3.221-.124-.303-.535-1.524.117-3.176 0 0 1.008-.322 3.301 1.23.957-.266 1.983-.399 3.003-.404 1.02.005 2.047.138 3.006.404 2.291-1.552 3.297-1.23 3.297-1.23.653 1.653.242 2.874.118 3.176.77.84 1.235 1.911 1.235 3.221 0 4.609-2.807 5.624-5.479 5.921.43.372.823 1.102.823 2.222v3.293c0 .319.192.694.801.576 4.765-1.589 8.199-6.086 8.199-11.386 0-6.627-5.373-12-12-12z"/>
              </svg>
            </a>
            {status && (
              <div className="relative group">
                <Badge
                  variant={status.weixin_connected ? 'default' : 'destructive'}
                  className={`cursor-default ${status.weixin_connected ? 'bg-green-600 hover:bg-green-700' : ''}`}
                >
                  {status.weixin_connected
                    ? `微信已连接${accounts.length > 1 ? ` (${accounts.length})` : ''}`
                    : '微信未连接'}
                </Badge>
                {/* Hover 下拉卡片 */}
                {accounts.length > 0 && (
                  <div className="absolute right-0 top-full mt-2 w-64 bg-card border border-border rounded-lg shadow-lg opacity-0 invisible group-hover:opacity-100 group-hover:visible transition-all duration-200 z-50">
                    <div className="p-3 border-b border-border">
                      <p className="text-xs text-muted-foreground">已连接账号 ({accounts.length})</p>
                    </div>
                    <div className="p-2 space-y-1">
                      {accounts.map(a => (
                        <div key={a.account_id} className="flex items-center justify-between px-2 py-1.5 rounded-md hover:bg-muted/50 transition-colors">
                          <div className="flex items-center gap-2 min-w-0">
                            <div className="w-1.5 h-1.5 rounded-full bg-green-500 flex-shrink-0" />
                            <span className="text-sm truncate max-w-[140px]" title={a.account_id}>{displayName(a)}</span>
                          </div>
                          <Button
                            variant="ghost"
                            size="sm"
                            className="text-destructive hover:text-destructive h-6 px-2 text-xs flex-shrink-0"
                            onClick={() => setLogoutTarget(a.account_id)}
                          >
                            退出
                          </Button>
                        </div>
                      ))}
                    </div>
                  </div>
                )}
              </div>
            )}
            <Button size="sm" variant={status?.weixin_connected ? 'outline' : 'default'} onClick={handleStartLogin}>
              {status?.weixin_connected ? '+ 添加微信账号' : '扫码登录'}
            </Button>
          </div>
        </div>
      </header>

      {/* 扫码登录 Dialog */}
      <Dialog open={loginDialogOpen} onOpenChange={handleLoginDialogChange}>
        <DialogContent className="sm:max-w-md">
          {loginStatus === 'confirmed' && confirmedAccountId ? (
            /* 绑定成功界面 */
            <>
              <DialogHeader>
                <DialogTitle>🎉 绑定成功</DialogTitle>
                <DialogDescription>
                  微信账号已成功连接，可以为其设置一个备注以便识别
                </DialogDescription>
              </DialogHeader>
              <div className="space-y-4 py-4">
                <div className="flex items-center gap-3 p-3 rounded-lg bg-green-50 dark:bg-green-900/20 border border-green-200 dark:border-green-800">
                  <div className="w-10 h-10 rounded-full bg-green-100 dark:bg-green-900/40 flex items-center justify-center flex-shrink-0">
                    <div className="w-3 h-3 rounded-full bg-green-500" />
                  </div>
                  <div className="min-w-0">
                    <p className="text-sm font-medium text-green-700 dark:text-green-300">已连接</p>
                    <p className="text-xs text-muted-foreground font-mono truncate">{confirmedAccountId}</p>
                  </div>
                </div>
                <div className="grid grid-cols-4 items-center gap-3">
                  <Label htmlFor="login-nickname" className="text-right text-sm">备注</Label>
                  <Input
                    id="login-nickname"
                    value={loginNickname}
                    onChange={e => setLoginNickname(e.target.value)}
                    onKeyDown={e => e.key === 'Enter' && handleLoginDone()}
                    className="col-span-3"
                    placeholder="例如：工作微信、小号（可选）"
                    autoFocus
                  />
                </div>
              </div>
              <div className="flex justify-end gap-2">
                <Button variant="outline" size="sm" onClick={() => { setLoginDialogOpen(false); setConfirmedAccountId('') }}>跳过</Button>
                <Button size="sm" onClick={handleLoginDone}>完成</Button>
              </div>
            </>
          ) : (
            /* 扫码界面 */
            <>
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
            </>
          )}
        </DialogContent>
      </Dialog>

      {/* 退出确认弹窗 */}
      <AlertDialog open={!!logoutTarget} onOpenChange={open => { if (!open) setLogoutTarget('') }}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>确认退出微信账号</AlertDialogTitle>
            <AlertDialogDescription>
              确定要退出账号 <span className="font-medium text-foreground">{(() => { const a = accounts.find(x => x.account_id === logoutTarget); return a ? displayName(a) : logoutTarget; })()}</span> 吗？退出后需要重新扫码登录。
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>取消</AlertDialogCancel>
            <AlertDialogAction onClick={() => handleLogoutAccount(logoutTarget)} className="bg-destructive text-destructive-foreground hover:bg-destructive/90">
              确认退出
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      {/* 账号备注 Dialog */}
      <Dialog open={renameOpen} onOpenChange={setRenameOpen}>
        <DialogContent className="sm:max-w-sm">
          <DialogHeader>
            <DialogTitle>设置账号备注</DialogTitle>
            <DialogDescription>
              为账号 {renameTarget.length > 16 ? renameTarget.slice(0, 12) + '...' : renameTarget} 设置一个易识别的名称
            </DialogDescription>
          </DialogHeader>
          <div className="grid gap-3 py-2">
            <div className="grid grid-cols-4 items-center gap-3">
              <Label htmlFor="account-nickname" className="text-right">备注</Label>
              <Input
                id="account-nickname"
                value={renameValue}
                onChange={e => setRenameValue(e.target.value)}
                onKeyDown={e => e.key === 'Enter' && handleRenameSave()}
                className="col-span-3"
                placeholder="例如：工作微信、小号"
                autoFocus
              />
            </div>
          </div>
          <div className="flex justify-end gap-2">
            <Button variant="outline" size="sm" onClick={() => setRenameOpen(false)}>取消</Button>
            <Button size="sm" onClick={handleRenameSave}>确认</Button>
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

          <Card className="cursor-pointer hover:border-primary/50 transition-colors" onClick={() => setActiveTab('routes')}>
            <CardHeader className="pb-2">
              <CardDescription>智能路由</CardDescription>
            </CardHeader>
            <CardContent>
              <div className="flex items-center gap-2">
                <div className={`w-2.5 h-2.5 rounded-full ${status?.smart_routing_enabled ? 'bg-green-500' : 'bg-gray-400'}`} />
                <span className="text-lg font-semibold">
                  {status?.smart_routing_enabled ? '已开启' : '未开启'}
                </span>
              </div>
            </CardContent>
          </Card>
        </div>

        <Separator className="mb-8" />


        {/* 功能标签页 */}
        <Tabs value={activeTab} onValueChange={setActiveTab} className="space-y-6">
          <TabsList>
            <TabsTrigger value="accounts">微信账号</TabsTrigger>
            <TabsTrigger value="adapters">Agent 管理</TabsTrigger>
            <TabsTrigger value="routes">路由规则</TabsTrigger>
          </TabsList>

          <TabsContent value="accounts">
            <Card>
              <CardHeader>
                <div className="flex items-center justify-between">
                  <div>
                    <h3 className="text-lg font-semibold">微信账号管理</h3>
                    <p className="text-sm text-muted-foreground mt-1">管理已连接的微信账号，支持同时登录多个账号</p>
                  </div>
                  <Button size="sm" onClick={handleStartLogin}>
                    + 添加账号
                  </Button>
                </div>
              </CardHeader>
              <CardContent>
                {accounts.length === 0 ? (
                  <div className="text-center py-8 text-muted-foreground">
                    <p>尚未连接微信账号</p>
                    <Button variant="outline" size="sm" className="mt-3" onClick={handleStartLogin}>
                      扫码登录
                    </Button>
                  </div>
                ) : (
                  <div className="space-y-3">
                    {accounts.map(a => (
                      <div key={a.account_id} className="flex items-center justify-between p-3 rounded-lg border border-border hover:border-primary/30 transition-colors">
                        <div className="flex items-center gap-3 min-w-0">
                          <div className="w-9 h-9 rounded-full bg-green-100 dark:bg-green-900/30 flex items-center justify-center flex-shrink-0">
                            <div className="w-2.5 h-2.5 rounded-full bg-green-500" />
                          </div>
                          <div className="min-w-0">
                            <span
                              className="text-sm font-medium block truncate max-w-[240px] cursor-pointer hover:text-primary transition-colors"
                              title="点击修改备注"
                              onClick={() => handleRenameAccount(a.account_id, a.nickname)}
                            >
                              {displayName(a)}
                            </span>
                            <span className="text-xs text-muted-foreground font-mono truncate block max-w-[240px]">
                              ID: {a.account_id}
                            </span>
                          </div>
                        </div>
                        <div className="flex items-center gap-2 flex-shrink-0">
                          <Button
                            variant="ghost"
                            size="sm"
                            className="h-7 px-2 text-xs"
                            onClick={() => handleRenameAccount(a.account_id, a.nickname)}
                          >
                            备注
                          </Button>
                          <Button
                            variant="ghost"
                            size="sm"
                            className="text-destructive hover:text-destructive h-7 px-2 text-xs"
                            onClick={() => setLogoutTarget(a.account_id)}
                          >
                            退出
                          </Button>
                        </div>
                      </div>
                    ))}
                  </div>
                )}
              </CardContent>
            </Card>
          </TabsContent>

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
