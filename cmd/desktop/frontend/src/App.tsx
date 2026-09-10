import React, { useState, useEffect, useCallback } from 'react'
import { App as AppBase, AuthService, VMService, ProtocolService, VMInfo as VMInfoType, ConnectionInfo as ConnectionInfoType, ProtocolConfig, AutostartStatus, StartHint, USBDevice, SystemKioskStatus, SystemKioskConfig, PveVMInfo } from '../bindings'
import * as wailsRuntime from '../wailsjs/runtime/runtime'
import { ConfigProvider, Layout, Progress, Card, Button, Tag, Modal, Switch, Tooltip, Badge, Input, message, Tabs, Select, InputNumber, Space, Popconfirm, Segmented, Upload, Spin, Table } from 'antd'
import {
  DesktopOutlined,
  SettingOutlined,
  UserOutlined,
  ReloadOutlined,
  LogoutOutlined,
  PlayCircleOutlined,
  PoweroffOutlined,
  RestOutlined,
  LinkOutlined,
  SafetyCertificateOutlined,
  ThunderboltOutlined,
  CloudServerOutlined,
  DatabaseOutlined,
  WifiOutlined,
  ApiOutlined,
  BulbOutlined,
  PlusOutlined,
  EditOutlined,
  DeleteOutlined,
  DisconnectOutlined,
  GlobalOutlined,
  MinusOutlined,
  CloseOutlined,
  UploadOutlined,
  PictureOutlined,
  CheckCircleOutlined,
  InfoCircleOutlined,
  ExclamationCircleOutlined,
  SlidersOutlined,
  LockOutlined,
  CloudUploadOutlined,
  SearchOutlined,
} from '@ant-design/icons'
import zhCN from 'antd/locale/zh_CN'

// ===== 本地官方 SVG 图标（OS 品牌彩色 Logo，用于卡片右侧大图标） =====
// 素材来源：Wikimedia Commons 官方品牌符号 + CentOS/Ubuntu/Kylin 官方品牌规范
import osWin7 from './assets/icons/os-windows-7.svg'
import osWin10 from './assets/icons/os-windows-10.svg'
import osWin11 from './assets/icons/os-windows-11.svg'
import osWinSrv2019 from './assets/icons/os-windows-server-2019.svg'
import osWinSrv2022 from './assets/icons/os-windows-server-2022.svg'
import osWinSrv2025 from './assets/icons/os-windows-server-2025.svg'
import osUbuntu from './assets/icons/os-ubuntu.svg'
import osCentos from './assets/icons/os-centos.svg'
import osKylin  from './assets/icons/os-kylin.svg'

interface VMInfo extends VMInfoType {}
interface ConnectionInfo extends ConnectionInfoType {}

interface Toast {
  id: number;
  message: string;
  type: 'success' | 'error' | 'info';
}

type PageView = 'vms'

const { Header, Sider, Content } = Layout

const appInstance = new AppBase()
const authService = new AuthService(appInstance)
const vmService = new VMService(authService)
const protocolService = new ProtocolService(appInstance)

function App() {
  const [messageApi, messageContextHolder] = message.useMessage()
  const [isLoggedIn, setIsLoggedIn] = useState(false)
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [serverUrl, setServerUrl] = useState('http://localhost:8080')
  const [vms, setVms] = useState<VMInfo[]>([])
  const [loading, setLoading] = useState(false)
  const [toasts, setToasts] = useState<Toast[]>([])
  const [currentView, setCurrentView] = useState<PageView>('vms')
  const [dependencies, setDependencies] = useState<Record<string, { available: boolean; message: string }>>({})
  const [autoLogin, setAutoLogin] = useState(true)
  const [isDark, setIsDark] = useState(true)
  const [collapsed, setCollapsed] = useState(false)
  const [protocolConfig, setProtocolConfig] = useState<ProtocolConfig | null>(null)
  const [isLocalMode, setIsLocalMode] = useState(false)
  const [vmEditorOpen, setVmEditorOpen] = useState(false)
  const [editingVM, setEditingVM] = useState<VMInfo | null>(null)
  const [loginMode, setLoginMode] = useState<'console' | 'local'>('console')
  const [localUsername, setLocalUsername] = useState('admin')
  const [localPassword, setLocalPassword] = useState('')
  // 记住密码 & 自动登录
  const [rememberPwd, setRememberPwd] = useState(false)
  const [autoLoginOnStart, setAutoLoginOnStart] = useState(false)
  // 控制台地址设置弹窗
  const [consoleSettingsOpen, setConsoleSettingsOpen] = useState(false)
  // 卡片设置弹窗
  const [cardSettingsVM, setCardSettingsVM] = useState<VMInfo | null>(null)
  const [cardSettingsOpen, setCardSettingsOpen] = useState(false)
  // 全局设置：品牌、强制全屏等
  const [brandName, setBrandName] = useState('瘦客户端云桌面')
  const [brandLogo, setBrandLogo] = useState('') // emoji 或 base64 图片
  const [forceFullscreen, setForceFullscreen] = useState(false)
  // 开机启动（全局设置）
  const [autostartNormal, setAutostartNormal] = useState(false)
  const [autostartKiosk, setAutostartKiosk] = useState(false)
  // 管理员密码验证（进入设置页面 / 编辑虚拟机信息 / 删除虚拟机 → 都需要验证）
  const [adminAuthOpen, setAdminAuthOpen] = useState(false)
  const [adminPwdInput, setAdminPwdInput] = useState('')
  const [adminAuthError, setAdminAuthError] = useState('')
  // 待执行动作：验证成功后执行的目标（打开设置 / 编辑某台虚拟机）
  type AdminPendingAction =
    | { type: 'settings' }
    | { type: 'editVM'; vm: VMInfo }
  const [adminPendingAction, setAdminPendingAction] = useState<AdminPendingAction>({ type: 'settings' })
  // 设置弹窗
  const [settingsModalOpen, setSettingsModalOpen] = useState(false)
  // 关于我们弹窗
  const [aboutModalOpen, setAboutModalOpen] = useState(false)
  // 电源操作弹窗（关机/重启本地机器）
  const [powerModalOpen, setPowerModalOpen] = useState(false)
  // 电源操作二次确认弹窗
  const [powerConfirmOpen, setPowerConfirmOpen] = useState(false)
  // 电源操作类型：'shutdown' | 'restart'
  const [powerAction, setPowerAction] = useState<'shutdown' | 'restart'>('shutdown')
  // 电源操作执行中
  const [powerLoading, setPowerLoading] = useState(false)
  // 连接保护：防止快速重复点击导致频繁弹窗
  const [connectingVMs, setConnectingVMs] = useState<Set<number>>(new Set())
  const connectCooldownRef = React.useRef<Map<number, number>>(new Map())

  // 应用版本号（构建时通过 //go:embed VERSION 注入，随构建自增；显示格式如 V1.003）
  const [appVersion, setAppVersion] = useState('V1.0.0')

  const ADMIN_PWD = () => localStorage.getItem('admin_password') || '123110'
  // 管理员密码验证通过后：根据 pendingAction 类型执行不同动作
  const tryAdminAuth = () => {
    const expected = ADMIN_PWD()
    if (adminPwdInput === expected) {
      setAdminAuthOpen(false)
      const action = adminPendingAction
      setAdminPwdInput('')
      setAdminAuthError('')
      switch (action.type) {
        case 'settings':
          setSettingsModalOpen(true)
          break
        case 'editVM':
          handleEditVM(action.vm)
          break
        // ★ default：兜底兼容「onClick={openAdminAuth} 直接把 MouseEvent 当参数」的老写法
        //   （MouseEvent.type === 'click' 时，当作默认的 settings 处理，避免被忽略）
        default:
          setSettingsModalOpen(true)
      }
      setAdminPendingAction({ type: 'settings' })
      return true
    } else {
      setAdminAuthError('密码错误，请重新输入')
      return false
    }
  }
  // 打开管理员验证弹窗（可选：传入验证成功后要执行的动作，默认打开设置）
  //   ⚠️ 注意：JSX onClick={openAdminAuth} 会把 MouseEvent 传进来作为第一个参数，
  //      所以第一个参数的默认值生效条件是「传参为 undefined」，被 MouseEvent 覆盖时默认值不会生效！
  //      必须：要么所有调用处都写 onClick={() => openAdminAuth({type:'settings'})}，要么
  //      这里做运行时检查，如果传入的不是合法 AdminPendingAction，就兜底用 settings
  const openAdminAuth = (action?: AdminPendingAction | MouseEvent | any) => {
    let safeAction: AdminPendingAction
    if (action && typeof action === 'object' && (action.type === 'settings' || action.type === 'editVM')) {
      safeAction = action as AdminPendingAction
    } else {
      // action 是 MouseEvent（type='click'）或其它非法值 → 兜底成 settings
      safeAction = { type: 'settings' }
    }
    setAdminPendingAction(safeAction)
    setAdminPwdInput('')
    setAdminAuthError('')
    setAdminAuthOpen(true)
  }

  // 初始化：从后端获取应用版本号（构建时 //go:embed VERSION 注入，随构建自增）
  useEffect(() => {
    appInstance.GetAppVersion().then((v: string) => setAppVersion(v)).catch(() => {})
  }, [])

  // ============== 初始化：持久化模式 / 记住密码 / 自动登录 / 品牌设置 ==============
  useEffect(() => {
    const token = localStorage.getItem('token')
    const savedUrl = localStorage.getItem('server_url')
    const savedTheme = localStorage.getItem('theme')
    const savedBrand = localStorage.getItem('brand_name') || ''
    const savedLogo = localStorage.getItem('brand_logo') || ''
    // 注意：默认是"无边框 + 自绘最小化/最大化/关闭按钮"的普通窗口，不是 Kiosk 全屏隐藏标题栏模式
    // 升级迁移：此版本开始显示自绘标题栏按钮，旧版本的 kiosk=true 先重置为 false，确保按钮能看到
    const migrated = localStorage.getItem('titlebar_buttons_v1')
    let savedForceFS = localStorage.getItem('force_fullscreen') === 'true'
    if (!migrated) {
      savedForceFS = false
      localStorage.setItem('force_fullscreen', 'false')
      localStorage.setItem('titlebar_buttons_v1', '1')
    }
    if (savedBrand) setBrandName(savedBrand)
    if (savedLogo) setBrandLogo(savedLogo)
    setForceFullscreen(savedForceFS)
    const savedLoginMode = localStorage.getItem('login_mode') as 'console' | 'local' | null
    const savedLocal = localStorage.getItem('local_mode') || localStorage.getItem('offline_mode')
    const mode: 'console' | 'local' = savedLoginMode || (savedLocal === 'true' ? 'local' : 'console')
    setLoginMode(mode)

    const savedRemember = localStorage.getItem('remember_pwd') === 'true'
    const savedAutoLogin = localStorage.getItem('auto_login_start') === 'true'
    setRememberPwd(savedRemember)
    setAutoLoginOnStart(savedAutoLogin)

    if (savedUrl) setServerUrl(savedUrl)
    if (savedTheme === 'light') setIsDark(false)

    if (savedRemember) {
      if (mode === 'local') {
        setLocalUsername(localStorage.getItem('local_saved_user') || 'admin')
        setLocalPassword(localStorage.getItem('local_saved_pwd') || '')
      } else {
        setUsername(localStorage.getItem('console_saved_user') || '')
        setPassword(localStorage.getItem('console_saved_pwd') || '')
      }
    }

    if (savedAutoLogin && savedRemember) {
      if (mode === 'local') {
        const u = localStorage.getItem('local_saved_user') || 'admin'
        const p = localStorage.getItem('local_saved_pwd') || ''
        if (u && p) {
          authService.LocalLogin(u, p).then(() => {
            setIsLocalMode(true)
            setIsLoggedIn(true)
            setCurrentView('vms')
          }).catch(() => {})
        }
      } else if (token) {
        authService.SetToken(token)
        setIsLoggedIn(true)
        setCurrentView('vms')
      }
    } else if (token && mode !== 'local') {
      authService.SetToken(token)
      setIsLoggedIn(true)
      setCurrentView('vms')
    }

    // ============== 需求5：开机无感启动 —— 读后端 StartHint ==============
    // 若带 --kiosk-autostart（开机启动设置成 Kiosk 模式自启），要做两件事：
    //   A) 强制自动登录（忽略用户手动勾选的 auto_login_start，否则"开机无感到首页"不成立）
    //   B) 登录完成后再进 Kiosk 全屏 + 封死 ALT+F4
    appInstance.GetStartHint().then((hint: StartHint) => {
      if (!hint?.kiosk_auto) return
      // kiosk_auto = true → 开机启动进入"无感 Kiosk"
      const tryLogin = () => new Promise<void>(async (resolve) => {
        try {
          const m2: 'console' | 'local' = (localStorage.getItem('login_mode') as any) || (localStorage.getItem('local_mode') === 'true' ? 'local' : 'console')
          if (m2 === 'local') {
            const u = localStorage.getItem('local_saved_user') || 'admin'
            const p = localStorage.getItem('local_saved_pwd') || ''
            if (u && p) {
              await authService.LocalLogin(u, p)
              setIsLocalMode(true)
              setIsLoggedIn(true)
              setCurrentView('vms')
              resolve()
              return
            }
          } else {
            const t2 = localStorage.getItem('token')
            if (t2) {
              authService.SetToken(t2)
              setIsLoggedIn(true)
              setCurrentView('vms')
              resolve()
              return
            }
            const u = localStorage.getItem('console_saved_user') || ''
            const p = localStorage.getItem('console_saved_pwd') || ''
            if (u && p) {
              try { await authService.Login(u, p) } catch {}
              if (authService.IsLoggedIn()) {
                setIsLoggedIn(true)
                setCurrentView('vms')
                resolve()
                return
              }
            }
          }
        } catch {}
        resolve()
      })
      tryLogin().finally(() => {
        // ★ 不管自动登录是否成功，都要尽快进 Kiosk 全屏 + 封死关闭按钮/ALT+F4，
        //   避免用户通过 Alt+F4 看到 Windows 桌面（用户明确要求"不要让用户看到桌面"）
        setForceFullscreen(true)
        localStorage.setItem('force_fullscreen', 'true')
        // 延迟一点让 Wails 窗口 ready，再触发全屏 + 锁定
        setTimeout(() => {
          try { wailsRuntime.WindowFullscreen() } catch {}
          appInstance.ApplyKioskMode().catch(() => {})
          appInstance.LockdownKioskClose().catch(() => {})
        }, 300)
        setTimeout(() => {
          try { wailsRuntime.WindowFullscreen() } catch {}
          appInstance.LockdownKioskClose().catch(() => {})
        }, 1200)
      })
    }).catch(() => {})

    // 读取开机启动开关状态（全局设置 UI 初始化）
    appInstance.GetAutostart().then((s: AutostartStatus) => {
      setAutostartNormal(!!s?.normal)
      setAutostartKiosk(!!s?.kiosk_auto)
    }).catch(() => {})
  }, [])

  // ============== 托盘事件：托盘菜单"设置"→前端切到 settings 页面 ==============
  useEffect(() => {
    let cleanup: (() => void) | null = null
    const handler = (...data: any[]) => {
      const target = data[0] as string
      if (target?.startsWith('settings')) {
        openAdminAuth()
      }
    }
    try {
      cleanup = wailsRuntime.EventsOn('tray:navigate', handler)
    } catch (e) {}
    return () => {
      try {
        if (cleanup) {
          cleanup()
        } else {
          wailsRuntime.EventsOff('tray:navigate')
        }
      } catch (e) {}
    }
  }, [])

  // 初始化完成后，如果 forceFullscreen=true（Kiosk），前端主动把窗口切到全屏（Wails runtime 原生 API，真生效）
  useEffect(() => {
    if (!forceFullscreen) return
    // 等 Wails WebView 完全 ready 后再触发一次，避免 early call 被忽略
    const t1 = setTimeout(() => { try { wailsRuntime.WindowFullscreen() } catch {} }, 200)
    const t2 = setTimeout(() => { try { wailsRuntime.WindowFullscreen() } catch {} }, 800)
    return () => { clearTimeout(t1); clearTimeout(t2) }
  }, [forceFullscreen])

  useEffect(() => {
    if (isLoggedIn && currentView === 'vms') {
      loadVMs()
      checkAllDependencies()
    }
  }, [isLoggedIn, currentView])

  // 设置弹窗打开时加载协议配置
  useEffect(() => {
    if (settingsModalOpen && !protocolConfig) {
      loadProtocolConfig()
    }
  }, [settingsModalOpen])

  const loadProtocolConfig = async () => {
    try {
      const config = await protocolService.GetProtocolConfig()
      setProtocolConfig(config)
    } catch (e) {
      showToast('加载协议配置失败', 'error')
    }
  }

  const handleSaveProtocolConfig = async (config: ProtocolConfig) => {
    try {
      const ok = await protocolService.SaveProtocolConfig(config)
      if (ok) {
        setProtocolConfig(config)
        showToast('协议参数已保存', 'success')
      } else {
        showToast('保存失败，请重试', 'error')
      }
    } catch (e) {
      showToast('保存失败', 'error')
    }
  }

  const handleResetProtocolConfig = async () => {
    try {
      const config = await protocolService.ResetProtocolConfig()
      setProtocolConfig(config)
      showToast('已恢复默认参数', 'info')
    } catch (e) {
      showToast('重置失败', 'error')
    }
  }

  useEffect(() => {
    document.body.className = isDark ? 'theme-dark' : 'theme-light'
    localStorage.setItem('theme', isDark ? 'dark' : 'light')
  }, [isDark])

  const showToast = (msg: string, type: 'success' | 'error' | 'info' | 'warning' = 'info') => {
    // 统一使用 Antd message API：低调纯文字，无大块背景
    const cfg = { content: msg, duration: (type === 'error' || type === 'warning') ? 2.5 : 1.5, style: { fontSize: 12, padding: '2px 8px', borderRadius: 6, boxShadow: 'none' } }
    if (type === 'success') messageApi.success(cfg)
    else if (type === 'error') messageApi.error(cfg)
    else if (type === 'warning') messageApi.warning(cfg)
    else messageApi.info(cfg)
  }

  const switchLoginMode = (mode: 'console' | 'local') => {
    setLoginMode(mode)
    localStorage.setItem('login_mode', mode)
  }

  const handleLogin = async () => {
    try {
      setLoading(true)
      authService.SetBaseURL(serverUrl)
      const result = await authService.Login(username, password)
      localStorage.setItem('token', result.token)
      localStorage.setItem('server_url', serverUrl)
      localStorage.setItem('login_mode', 'console')
      localStorage.removeItem('local_mode')
      localStorage.removeItem('offline_mode')
      if (rememberPwd) {
        localStorage.setItem('console_saved_user', username)
        localStorage.setItem('console_saved_pwd', password)
        localStorage.setItem('remember_pwd', 'true')
      } else {
        localStorage.removeItem('console_saved_user')
        localStorage.removeItem('console_saved_pwd')
        localStorage.setItem('remember_pwd', 'false')
      }
      localStorage.setItem('auto_login_start', autoLoginOnStart ? 'true' : 'false')
      setIsLocalMode(false)
      setIsLoggedIn(true)
      setCurrentView('vms')
      messageApi.success({ content: '控制台登录成功', duration: 1.2, style: { fontSize: 12, padding: 4, borderRadius: 6 } })
    } catch (error: any) {
      showToast(error.message || '登录失败（请确认服务器地址与账号密码）', 'error')
    } finally {
      setLoading(false)
    }
  }

  const handleLocalLogin = async () => {
    try {
      if (!localUsername || !localUsername.trim()) {
        showToast('请输入本地账号（默认 admin）', 'error')
        return
      }
      if (!localPassword) {
        showToast('请输入本地密码（默认 admin）', 'error')
        return
      }
      setLoading(true)
      await authService.LocalLogin(localUsername, localPassword)
      localStorage.setItem('local_mode', 'true')
      localStorage.setItem('offline_mode', 'true')
      localStorage.setItem('login_mode', 'local')
      localStorage.removeItem('token')
      if (rememberPwd) {
        localStorage.setItem('local_saved_user', localUsername)
        localStorage.setItem('local_saved_pwd', localPassword)
        localStorage.setItem('remember_pwd', 'true')
      } else {
        localStorage.removeItem('local_saved_user')
        localStorage.removeItem('local_saved_pwd')
        localStorage.setItem('remember_pwd', 'false')
      }
      localStorage.setItem('auto_login_start', autoLoginOnStart ? 'true' : 'false')
      setIsLocalMode(true)
      setIsLoggedIn(true)
      setCurrentView('vms')
      messageApi.success({ content: '本地模式登录成功', duration: 1.2, style: { fontSize: 12, padding: 4, borderRadius: 6 } })
    } catch (error: any) {
      showToast(error.message || '本地登录失败', 'error')
    } finally {
      setLoading(false)
    }
  }

  const handleLogout = () => {
    localStorage.removeItem('token')
    localStorage.removeItem('local_mode')
    localStorage.removeItem('offline_mode')
    authService.SetToken('')
    setIsLoggedIn(false)
    setIsLocalMode(false)
    setVms([])
    showToast('已退出登录', 'info')
  }

  const loadVMs = async () => {
    setLoading(true)
    try {
      // 给整个初始列表读取加 5s 超时：控制台模式网络故障也不会永远 loading
      const listTimeoutMs = 5000
      const listPromise = (async (): Promise<VMInfo[]> => {
        if (isLocalMode) {
          return await vmService.ListOfflineVMs()
        } else {
          const list = await vmService.ListVMs()
          const prefs = vmService.GetAllVMProtocolPreferences()
          return list.map(vm => {
            const customProto = prefs[String(vm.vmid)]
            return customProto ? { ...vm, protocol: customProto } : vm
          })
        }
      })()

      const timeoutPromise = new Promise<VMInfo[]>((_, reject) =>
        setTimeout(() => reject(new Error('列表加载超时')), listTimeoutMs)
      )

      let vmList: VMInfo[]
      try {
        vmList = await Promise.race([listPromise, timeoutPromise])
      } catch (listErr: any) {
        // 超时/失败：如果有旧缓存就先用旧缓存显示，否则空列表
        vmList = [...vms]
        const msg = isLocalMode
          ? '本地列表读取失败，显示缓存'
          : '服务器连接超时，显示已缓存列表'
        showToast(`${msg}（${listErr?.message || ''}）`, 'warning')
      }

      // ① 先立即显示：用本地缓存状态渲染卡片，UI 不阻塞
      setVms(vmList)
      setLoading(false)

      // ② 按电源控制后端**异步并行**刷新真实状态（后台刷新，不阻塞 UI）
      //    ★ 兼容旧数据：如果 power_backend 字段为空（老卡片在「电源控制后端」下拉功能之前保存的），
      //      但 hyperv_vmname 有值 → 自动视为 hyperv；但 pve_endpoint+pve_vmid 有值 → 自动视为 pve。
      //      否则旧卡片永远显示本地 stopped 缓存状态，不会发起任何异步查询。
      const withBackend: { vmid: number; backend: string; v: VMInfo }[] = []
      for (const v of vmList) {
        let backend = v.power_backend ||
          (v.protocol === 'spice' && v.pve_endpoint ? 'pve' : 'none')
        if ((!backend || backend === 'none') && v.hyperv_vmname) backend = 'hyperv'
        if ((!backend || backend === 'none') && v.pve_endpoint && v.pve_vmid) backend = 'pve'

        // ★ 如果有 hyperv_vmname，不管协议是什么，都走 Hyper-V 路径
        if (backend === 'hyperv' && v.hyperv_vmname) {
          withBackend.push({ vmid: v.vmid, backend, v })
        }
        else if (backend === 'pve' && v.pve_endpoint && v.pve_vmid) {
          withBackend.push({ vmid: v.vmid, backend, v })
        }
        // 没有 power_backend 的 RDP/FreeRDP 虚拟机：用 TCP 端口检测判断状态
        else if ((!backend || backend === 'none') && (v.protocol === 'rdp' || v.protocol === 'windowsrdp') && v.ip_address) {
          withBackend.push({ vmid: v.vmid, backend: 'rdp-port', v })
        }
        // ★ 如果有 hyperv_vmname 但 backend 不对，也走 Hyper-V
        else if (v.hyperv_vmname) {
          withBackend.push({ vmid: v.vmid, backend: 'hyperv', v })
        }
        // ★ RDP VM 有 ip_address 但没有 hyperv_vmname → 端口检测
        else if ((v.protocol === 'rdp' || v.protocol === 'windowsrdp') && v.ip_address) {
          withBackend.push({ vmid: v.vmid, backend: 'rdp-port', v })
        }
        // ★ Horizon / SPICE / VNC 有 ip_address 但没有有效后端 → 通用 TCP 端口检测（3389/443/5900）
        else if (v.ip_address) {
          withBackend.push({ vmid: v.vmid, backend: 'any-port', v })
        }
        // horizon 后端：不查（Connection Server 没有 VM 状态查询 API）
      }

      if (withBackend.length === 0) return

      // 带超时的 Promise 包装器：ms 毫秒未返回则返回 fallback 原值
      // 注意：<T,> 后面的逗号是为了避免 tsx 把 <T> 误解析为 JSX 标签
      const withTimeout = <T,>(p: Promise<T>, ms: number, fallback: T): Promise<T> =>
        Promise.race([
          p,
          new Promise<T>((resolve) => setTimeout(() => resolve(fallback), ms)),
        ])

      // ★ 查询去重：多张卡片指向同一台虚拟机（同 host + vmname）时只发起一次真实查询，
      //    结果共享给所有相同卡片。避免同一主机并发建立多个 DCOM/CIM 会话相互抢凭据，
      //    导致偶发 0x80070005「拒绝访问」和 COM 信号量耗尽。
      const queryCache = new Map<string, Promise<{ realStatus: string; errMsg: string }>>()

      const doQuery = async (backend: string, v: VMInfo): Promise<{ realStatus: string; errMsg: string }> => {
        try {
          if (backend === 'pve') {
            const realStatus = await withTimeout(
              protocolService.GetVMStatusViaPVE(
                v.pve_endpoint!, v.pve_username || '', v.pve_password || '',
                v.pve_node || '', v.pve_vmid!, !!v.pve_verify_ssl
              ),
              10000, // 10s 超时：PVE HTTPS 首次握手/证书验证偶尔较慢
              ''
            )
            return { realStatus, errMsg: '' }
          }
          if (backend === 'hyperv') {
            // 实测（8/12 独立测试）：等价 PowerShell 命令从启动进程到返回 EnabledState=2，
            // 单台 ceshi 虚拟机耗时 11458ms，其中绝大部分是 DCOM RPC 首次握手+凭据协商。
            // 所以前端超时必须 >11.5s，这里给 15s，留 30% 冗余应对网络波动。
            const HYPERV_TIMEOUT_MS = 15000
            const __TIMEOUT__ = '__HYPERv_TIMEOUT__'
            const realStatus = await withTimeout<string>(
              protocolService.GetVMStatusViaHyperV(
                v.hyperv_host || '.', v.hyperv_vmname!,
                !!v.hyperv_use_winrm, v.hyperv_username || '', v.hyperv_password || ''
              ),
              HYPERV_TIMEOUT_MS,
              __TIMEOUT__
            )
            if (realStatus === __TIMEOUT__) {
              return {
                realStatus: '',
                errMsg: `Hyper-V 查询超时（${HYPERV_TIMEOUT_MS / 1000}s）：主机 ${v.hyperv_host || '.'} 建立 CIM 会话较慢，仍显示本地状态（点击刷新重试即可）`
              }
            }
            return { realStatus, errMsg: '' }
          }
          // RDP 端口检测：没有电源后端的 RDP/FreeRDP 虚拟机
          if (backend === 'rdp-port') {
            const realStatus = await withTimeout(
              protocolService.CheckRDPPortStatus(v.ip_address!, 3389),
              5000, // 5s 超时：TCP 连接超时
              ''
            )
            return { realStatus, errMsg: '' }
          }
          // 通用端口检测：没有电源后端的 Horizon/SPICE/VNC 虚拟机
          // 依次探测 3389(RDP) → 443(HTTPS/Horizon) → 5900(VNC/SPICE)，有一个通就是 running
          if (backend === 'any-port') {
            const realStatus = await withTimeout(
              protocolService.CheckAnyPortStatus(v.ip_address!),
              6000, // 6s 超时：探测3个端口，每个2s
              ''
            )
            return { realStatus, errMsg: '' }
          }
          return { realStatus: '', errMsg: '' }
        } catch (e: any) {
          return { realStatus: '', errMsg: e?.message || String(e) || '未知错误' }
        }
      }

      const refreshTasks = withBackend.map(({ backend, v }) => {
        // 去重 key：唯一标识一次真实查询目标
        const key =
          backend === 'hyperv' ? `hyperv|${v.hyperv_host || '.'}|${v.hyperv_vmname}` :
          backend === 'pve' ? `pve|${v.pve_endpoint}|${v.pve_node}|${v.pve_vmid}` :
          `${backend}|${v.ip_address}`
        let p = queryCache.get(key)
        if (!p) {
          p = doQuery(backend, v)
          queryCache.set(key, p)
        }
        // 每张卡片应用自己的本地缓存兜底
        return p.then(({ realStatus, errMsg }) => ({ vmid: v.vmid, status: realStatus || v.status, errMsg }))
      })

      // 后台异步刷新，刷新完后只更新那些变了的卡片；如果有查询失败统一弹一条汇总 Toast 提示
      Promise.all(refreshTasks).then((refreshed) => {
        const statusMap = new Map<number, string>()
        const failedMsgs: string[] = []
        for (const r of refreshed) {
          if (r.status) statusMap.set(r.vmid, r.status)
          if (r.errMsg) failedMsgs.push(`#${r.vmid}: ${r.errMsg}`)
        }
        if (failedMsgs.length > 0) {
          // 统一提示失败摘要，不重复每台单独弹
          const summary = failedMsgs.length <= 2
            ? failedMsgs.join('；')
            : `${failedMsgs.slice(0, 2).join('；')} 等 ${failedMsgs.length} 台`
          showToast(`部分虚拟机状态查询失败（仍显示本地缓存）→ ${summary}`, 'warning')
        }
        if (statusMap.size === 0) return
        setVms(prev => prev.map(v => {
          const ns = statusMap.get(v.vmid)
          // 只在状态真正变化、且卡片当前不在过渡态（starting/stopping/rebooting）时才覆盖
          if (!ns || ns === v.status) return v
          const inTransition = v.status === 'starting' || v.status === 'stopping' || v.status === 'rebooting' ||
            v.status === 'suspending' || v.status === 'resuming'
          return inTransition ? v : { ...v, status: ns }
        }))
      }).catch(() => { /* 忽略，后台刷新失败不影响前台已渲染的卡片 */ })
    } catch (error: any) {
      showToast(error.message || '加载虚拟机列表失败', 'error')
      setLoading(false)
    }
  }

  const checkAllDependencies = async () => {
    const protocols = ['rdp', 'windowsrdp', 'spice', 'vnc', 'horizon'] as const
    // 并行查询所有协议依赖，不串行 await，缩短整体时间
    const tasks = protocols.map(async (proto): Promise<{ proto: string; ok: boolean; msg: string }> => {
      try {
        const result = await protocolService.CheckDependency(proto)
        if (Array.isArray(result)) {
          return { proto, ok: !!result[0], msg: String(result[1] ?? '') }
        } else {
          const avail = !!(result?.Available ?? result?.available)
          const msg = String(result?.Message ?? result?.message ?? (avail ? '已安装' : '未安装'))
          return { proto, ok: avail, msg }
        }
      } catch (e: any) {
        return { proto, ok: false, msg: e?.message || String(e) || '检查失败' }
      }
    })
    const results = await Promise.all(tasks)
    const deps: Record<string, { available: boolean; message: string }> = {}
    for (const r of results) {
      deps[r.proto] = { available: r.ok, message: r.msg }
    }
    setDependencies(deps)
  }

  // 乐观更新卡片状态：按钮点击后立即显示「正在开机/关机/重启」，避免页面卡住
  const updateVMStatusLocal = (vmid: number, status: string) => {
    setVms(prev => prev.map(v => v.vmid === vmid ? { ...v, status } : v))
  }

  const handleStartVM = async (vmid: number) => {
    try {
      updateVMStatusLocal(vmid, 'starting')
      if (isLocalMode) {
        const vmInfo = vms.find(v => v.vmid === vmid)
        const backend = vmInfo?.power_backend ||
          (vmInfo?.protocol === 'spice' && vmInfo?.pve_endpoint ? 'pve' : 'none')

        // ---- PVE 后端（Proxmox VE API）----
        if (backend === 'pve' && vmInfo?.pve_endpoint && vmInfo?.pve_vmid) {
          showToast('正在通过 PVE API 启动虚拟机...', 'info')
          await protocolService.ControlVMViaPVE(
            vmInfo.pve_endpoint, vmInfo.pve_username || '', vmInfo.pve_password || '',
            vmInfo.pve_node || '', vmInfo.pve_vmid, 'start', !!vmInfo.pve_verify_ssl
          )
          showToast('虚拟机启动命令已发送，正在开机…', 'success')
          setTimeout(loadVMs, 4000)
          return
        }

        // ---- Hyper-V 后端（本地 PowerShell Hyper-V Cmdlet）----
        if (backend === 'hyperv' && vmInfo?.hyperv_vmname) {
          showToast('正在通过 Hyper-V 启动虚拟机...', 'info')
          await protocolService.ControlVMViaHyperV(
            vmInfo.hyperv_host || '.', vmInfo.hyperv_vmname,
            !!vmInfo.hyperv_use_winrm, vmInfo.hyperv_username || '',
            vmInfo.hyperv_password || '', !!vmInfo.hyperv_force,
            'start'
          )
          showToast('Hyper-V 启动命令已发送，正在开机…', 'success')
          setTimeout(loadVMs, 4000)
          return
        }

        // ---- Horizon 后端：提示用户通过 Horizon 控制台操作 ----
        if (backend === 'horizon') {
          updateVMStatusLocal(vmid, vms.find(v => v.vmid === vmid)?.status || 'stopped')
          showToast('Horizon 虚拟机请通过 Horizon 管理控制台启动', 'warning')
          return
        }

        // 默认：本地模拟
        await vmService.StartOfflineVM(vmid)
        showToast('虚拟机已启动', 'success')
        loadVMs()
      } else {
        await vmService.StartVM(vmid)
        showToast('启动命令已发送，正在开机…', 'success')
        setTimeout(loadVMs, 3000)
      }
    } catch (error: any) {
      showToast(error.message || '启动失败', 'error')
      loadVMs()
    }
  }

  const handleStopVM = async (vmid: number, hard: boolean = false) => {
    try {
      updateVMStatusLocal(vmid, 'stopping')
      if (isLocalMode) {
        const vmInfo = vms.find(v => v.vmid === vmid)
        // ★ 兼容旧数据：power_backend 为空但 hyperv_vmname 有值 → hyperv；pve_endpoint+vmid → pve
        let backend: string = vmInfo?.power_backend ||
          (vmInfo?.protocol === 'spice' && vmInfo?.pve_endpoint ? 'pve' : 'none')
        if ((!backend || backend === 'none') && vmInfo?.hyperv_vmname) backend = 'hyperv'
        if ((!backend || backend === 'none') && vmInfo?.pve_endpoint && vmInfo?.pve_vmid) backend = 'pve'

        if (backend === 'pve' && vmInfo?.pve_endpoint && vmInfo?.pve_vmid) {
          // PVE：hard=false → shutdown(ACPI 软关)；hard=true → stop(硬断电)
          const pveAction = hard ? 'stop' : 'shutdown'
          showToast(hard ? '正在通过 PVE API 强制断电虚拟机...' : '正在通过 PVE API 安全关闭虚拟机...', 'info')
          await protocolService.ControlVMViaPVE(
            vmInfo.pve_endpoint, vmInfo.pve_username || '', vmInfo.pve_password || '',
            vmInfo.pve_node || '', vmInfo.pve_vmid, pveAction, !!vmInfo.pve_verify_ssl
          )
          showToast(hard ? 'PVE 强制断电命令已发送，正在关机…' : '虚拟机停止命令已发送，正在关机…', 'success')
          setTimeout(loadVMs, hard ? 2000 : 4000)
          return
        }

        if (backend === 'hyperv' && vmInfo?.hyperv_vmname) {
          // Hyper-V：硬关=RequestStateChange(3/TurnOff)，软关=RequestStateChange(4/ShutDown via Integration)
          const force = hard ? true : !!vmInfo.hyperv_force
          showToast(
            force ? '正在通过 Hyper-V 强制断电虚拟机...' : '正在通过 Hyper-V 安全关闭虚拟机...',
            'info'
          )
          await protocolService.ControlVMViaHyperV(
            vmInfo.hyperv_host || '.', vmInfo.hyperv_vmname,
            !!vmInfo.hyperv_use_winrm, vmInfo.hyperv_username || '',
            vmInfo.hyperv_password || '', force,
            'stop'
          )
          showToast(force ? 'Hyper-V 强制断电命令已发送，正在关机…' : 'Hyper-V 停止命令已发送，正在关机…', 'success')
          setTimeout(loadVMs, force ? 2000 : 4000)
          return
        }

        if (backend === 'horizon') {
          updateVMStatusLocal(vmid, vms.find(v => v.vmid === vmid)?.status || 'running')
          showToast('Horizon 虚拟机请通过 Horizon 管理控制台关闭', 'warning')
          return
        }

        await vmService.StopOfflineVM(vmid)
        showToast('虚拟机已停止', 'success')
        loadVMs()
      } else {
        await vmService.StopVM(vmid)
        showToast('停止命令已发送，正在关机…', 'success')
        setTimeout(loadVMs, 3000)
      }
    } catch (error: any) {
      showToast(error.message || '停止失败', 'error')
      loadVMs()
    }
  }

  const handleRebootVM = async (vmid: number, hard: boolean = false) => {
    try {
      updateVMStatusLocal(vmid, 'rebooting')
      if (isLocalMode) {
        const vmInfo = vms.find(v => v.vmid === vmid)
        // ★ 兼容旧数据
        let backend: string = vmInfo?.power_backend ||
          (vmInfo?.protocol === 'spice' && vmInfo?.pve_endpoint ? 'pve' : 'none')
        if ((!backend || backend === 'none') && vmInfo?.hyperv_vmname) backend = 'hyperv'
        if ((!backend || backend === 'none') && vmInfo?.pve_endpoint && vmInfo?.pve_vmid) backend = 'pve'

        if (backend === 'pve' && vmInfo?.pve_endpoint && vmInfo?.pve_vmid) {
          // PVE：hard=false → reboot(软重启)；hard=true → reset(硬重启/复位)
          const pveAction = hard ? 'reset' : 'reboot'
          showToast(hard ? '正在通过 PVE API 强制重启虚拟机...' : '正在通过 PVE API 重启虚拟机...', 'info')
          await protocolService.ControlVMViaPVE(
            vmInfo.pve_endpoint, vmInfo.pve_username || '', vmInfo.pve_password || '',
            vmInfo.pve_node || '', vmInfo.pve_vmid, pveAction, !!vmInfo.pve_verify_ssl
          )
          showToast(hard ? 'PVE 强制重启命令已发送，正在重启…' : '虚拟机重启命令已发送，正在重启…', 'success')
          setTimeout(loadVMs, 5000)
          return
        }

        if (backend === 'hyperv' && vmInfo?.hyperv_vmname) {
          // Hyper-V：硬重启=RequestStateChange(10/Reset)，软重启=RequestStateChange(11/Reboot via Integration)
          const force = hard ? true : !!vmInfo.hyperv_force
          showToast(force ? '正在通过 Hyper-V 强制重启虚拟机...' : '正在通过 Hyper-V 重启虚拟机...', 'info')
          await protocolService.ControlVMViaHyperV(
            vmInfo.hyperv_host || '.', vmInfo.hyperv_vmname,
            !!vmInfo.hyperv_use_winrm, vmInfo.hyperv_username || '',
            vmInfo.hyperv_password || '', force,
            'reboot'
          )
          showToast(force ? 'Hyper-V 强制重启命令已发送，正在重启…' : 'Hyper-V 重启命令已发送，正在重启…', 'success')
          setTimeout(loadVMs, 5000)
          return
        }

        if (backend === 'horizon') {
          updateVMStatusLocal(vmid, vms.find(v => v.vmid === vmid)?.status || 'running')
          showToast('Horizon 虚拟机请通过 Horizon 管理控制台重启', 'warning')
          return
        }

        await vmService.RebootOfflineVM(vmid)
        showToast('虚拟机已重启', 'success')
        loadVMs()
      } else {
        await vmService.RebootVM(vmid)
        showToast('重启命令已发送，正在重启…', 'success')
        setTimeout(loadVMs, 3000)
      }
    } catch (error: any) {
      showToast(error.message || '重启失败', 'error')
      loadVMs()
    }
  }

  // 一键连接：不再弹窗，直接用卡片已设置的协议启动
  const handleConnect = async (vmid: number) => {
    // 连接保护：防止快速重复点击
    if (connectingVMs.has(vmid)) {
      showToast('正在连接中，请稍候...', 'info')
      return
    }
    const lastConnectTime = connectCooldownRef.current.get(vmid) || 0
    const now = Date.now()
    if (now - lastConnectTime < 3000) {
      showToast('操作过于频繁，请稍候再试', 'warning')
      return
    }

    try {
      // 标记为连接中
      setConnectingVMs(prev => new Set(prev).add(vmid))
      connectCooldownRef.current.set(vmid, now)

      const connection = isLocalMode
        ? await vmService.GetOfflineConnection(vmid)
        : await vmService.GetConnection(vmid)
      if (!connection) {
        showToast('获取连接信息失败', 'error')
        return
      }
      const proto = connection.protocol
      const dep = dependencies[proto]
      if (!dep?.available) {
        showToast(`${getProtocolName(proto)} 协议依赖未安装: ${dep?.message || '未知'}`, 'error')
        return
      }

      // SPICE 协议：如果配置了 PVE 信息，走 PVE API 模式
      const vmInfo = vms.find(v => v.vmid === vmid)
      if (proto === 'spice' && vmInfo?.pve_endpoint && vmInfo?.pve_vmid) {
        showToast('正在通过 PVE API 获取 SPICE 连接信息...', 'info')
        try {
          await protocolService.LaunchSPICEViaPVE(
            vmInfo.pve_endpoint,
            vmInfo.pve_username || '',
            vmInfo.pve_password || '',
            vmInfo.pve_node || '',
            vmInfo.pve_vmid,
            !!vmInfo.pve_verify_ssl
          )
          showToast('SPICE 远程桌面已启动（PVE API 模式）', 'success')
        } catch (error: any) {
          showToast(error.message || 'PVE SPICE 连接失败', 'error')
        }
        return
      }
      const vmUsername = autoLogin ? (connection.username || '') : ''
      const vmPassword = autoLogin ? (connection.vm_password || '') : ''

      let width = 0, height = 0, fullscreen = false
      let colorDepth = 32, audio = true, clipboard = true, compression = true
      let driveMapping = false, printer = false, smartcard = false
      let hideClient = false
      let hideToolbar = false, blockAltF4 = false
      let usbRedirect = false
      let usbDevices = ''
      let usbMode = 'auto'
      let multiMon = false
      if (protocolConfig) {
        const resField = `${proto}_resolution` as keyof typeof protocolConfig
        const fsField = `${proto}_fullscreen` as keyof typeof protocolConfig
        const resStr = String(protocolConfig[resField] || '')
        const m = resStr.match(/^(\d+)\s*[x×*]\s*(\d+)$/)
        if (m) { width = parseInt(m[1], 10); height = parseInt(m[2], 10) }
        fullscreen = !!protocolConfig[fsField]
      }
      // 全局设置：强制所有客户端全屏（优先级高于协议单独设置）
      if (forceFullscreen) fullscreen = true
      if (protocolConfig) {
        if (proto === 'rdp') {
          const cd = protocolConfig['rdp_colorDepth' as keyof typeof protocolConfig]
          if (typeof cd === 'number' && cd > 0) colorDepth = cd
          audio = !!protocolConfig['rdp_audio' as keyof typeof protocolConfig]
          clipboard = !!protocolConfig['rdp_clipboard' as keyof typeof protocolConfig]
          compression = !!protocolConfig['rdp_compression' as keyof typeof protocolConfig]
          driveMapping = !!protocolConfig['rdp_driveMapping' as keyof typeof protocolConfig]
          hideToolbar = !!protocolConfig['rdp_hideToolbar' as keyof typeof protocolConfig]
          blockAltF4 = !!protocolConfig['rdp_blockAltF4' as keyof typeof protocolConfig]
          multiMon = !!protocolConfig['rdp_multiMon' as keyof typeof protocolConfig]
          usbRedirect = !!protocolConfig['rdp_usb' as keyof typeof protocolConfig]
          usbMode = String(protocolConfig['rdp_usbMode' as keyof typeof protocolConfig] || 'auto')
          usbDevices = String(protocolConfig['rdp_usbDevices' as keyof typeof protocolConfig] || '')
        }
        if (proto === 'spice') {
          smartcard = !!protocolConfig['spice_smartcard' as keyof typeof protocolConfig]
          clipboard = !!protocolConfig['spice_shareClipboard' as keyof typeof protocolConfig]
          hideToolbar = !!protocolConfig['spice_hideToolbar' as keyof typeof protocolConfig]
        }
        if (proto === 'horizon') {
          hideClient = !!protocolConfig['horizon_hideClient' as keyof typeof protocolConfig]
        }
        if (proto === 'windowsrdp') {
          const cd = protocolConfig['windowsrdp_colorDepth' as keyof typeof protocolConfig]
          if (typeof cd === 'number' && cd > 0) colorDepth = cd
          audio = !!protocolConfig['windowsrdp_audio' as keyof typeof protocolConfig]
          clipboard = !!protocolConfig['windowsrdp_clipboard' as keyof typeof protocolConfig]
          compression = !!protocolConfig['windowsrdp_compression' as keyof typeof protocolConfig]
          driveMapping = !!protocolConfig['windowsrdp_driveMapping' as keyof typeof protocolConfig]
          printer = !!protocolConfig['windowsrdp_printer' as keyof typeof protocolConfig]
          smartcard = !!protocolConfig['windowsrdp_smartcard' as keyof typeof protocolConfig]
          // 借用 hideToolbar / blockAltF4 / hideClient 三个字段承载 Windows RDP 专属：
          //   hideToolbar  → 多显示器 (use multimon)
          //   blockAltF4   → 「顶部连接栏」开关的取反 (displayconnectionbar = !blockAltF4)
          //   hideClient   → 是否启用自动注入凭据 (cmdkey 预存用户名密码)
          hideToolbar = !!protocolConfig['windowsrdp_multiMonitor' as keyof typeof protocolConfig]
          const showBar = !!protocolConfig['windowsrdp_showConnectionBar' as keyof typeof protocolConfig]
          blockAltF4 = !showBar
          hideClient = !!protocolConfig['windowsrdp_autoLogin' as keyof typeof protocolConfig]
        }
      }

      showToast(`正在启动 ${getProtocolName(proto)} 远程桌面...`, 'info')
      await protocolService.LaunchProtocolFull(
        proto, connection.host, connection.port,
        vmUsername, vmPassword || connection.password,
        connection.tls_port || 0, connection.broker_url || '',
        width, height, fullscreen,
        colorDepth, audio, clipboard, compression,
        driveMapping, printer, smartcard, hideClient,
        hideToolbar, blockAltF4, usbRedirect, usbDevices, usbMode, multiMon
      )
      showToast(autoLogin && (vmUsername || vmPassword) ? '远程桌面已启动（自动登录）' : '远程桌面已启动', 'success')
    } catch (error: any) {
      showToast(error.message || '启动远程桌面失败', 'error')
    } finally {
      // 解除连接中状态
      setConnectingVMs(prev => {
        const next = new Set(prev)
        next.delete(vmid)
        return next
      })
    }
  }

  // ============== 本地模式 VM 编辑 ==============
  const handleAddVM = () => {
    setEditingVM(null)
    setVmEditorOpen(true)
  }

  const handleEditVM = (vm: VMInfo) => {
    setEditingVM(vm)
    setVmEditorOpen(true)
  }

  const handleDeleteVM = async (vmid: number) => {
    try {
      await vmService.DeleteOfflineVM(vmid)
      showToast('虚拟机已删除', 'success')
      loadVMs()
    } catch (error: any) {
      showToast(error.message || '删除失败', 'error')
    }
  }

  const handleSaveVM = async (vm: VMInfo) => {
    try {
      if (editingVM) {
        await vmService.UpdateOfflineVM(vm)
        showToast('虚拟机信息已更新', 'success')
      } else {
        const { vmid, ...rest } = vm
        await vmService.AddOfflineVM(rest)
        showToast('虚拟机已添加', 'success')
      }
      setVmEditorOpen(false)
      setEditingVM(null)
      loadVMs()
    } catch (error: any) {
      showToast(error.message || '保存失败', 'error')
    }
  }

  const handleChangeVMProtocol = async (vmid: number, protocol: string) => {
    try {
      const ok = await vmService.SetVMProtocolPreference(vmid, protocol)
      if (ok) {
        setVms(prev => prev.map(vm => vm.vmid === vmid ? { ...vm, protocol } : vm))
        showToast(`${getProtocolName(protocol)} 协议已设为默认`, 'success')
      } else {
        showToast('设置失败，请重试', 'error')
      }
    } catch (e) {
      showToast('设置协议失败', 'error')
    }
  }

  const getProtocolIcon = (protocol: string) => {
    const icons: Record<string, string> = {
      rdp: '🖥️',
      windowsrdp: '🪟',
      spice: '🌶️',
      vnc: '📺',
      ssh: '🔐',
      horizon: '🌀',
    }
    return icons[protocol] || '💻'
  }

  const getProtocolName = (protocol: string) => {
    const names: Record<string, string> = {
      rdp: 'FreeRDP',
      windowsrdp: 'Windows RDP',
      spice: 'SPICE',
      vnc: 'VNC',
      ssh: 'SSH',
      horizon: 'Horizon',
    }
    return names[protocol] || protocol.toUpperCase()
  }

  const protocolColors: Record<string, string> = {
    rdp: '#6366f1',
    windowsrdp: '#0078D4',
    spice: '#f59e0b',
    vnc: '#10b981',
    ssh: '#3b82f6',
    horizon: '#8b5cf6',
  }

  const darkTheme = {
    token: {
      colorPrimary: '#6366f1',
      colorBgBase: '#0a0e27',
      colorBgContainer: '#121633',
      colorBgElevated: '#1a1f45',
      colorBorder: '#2a3066',
      colorBorderSecondary: '#1e2350',
      colorText: '#e2e8f0',
      colorTextSecondary: '#94a3b8',
      colorTextTertiary: '#64748b',
      colorTextQuaternary: '#475569',
      colorFill: '#1e2350',
      colorBgLayout: '#060919',
      colorInfo: '#3b82f6',
      colorSuccess: '#10b981',
      colorWarning: '#f59e0b',
      colorError: '#ef4444',
      borderRadius: 12,
    },
    components: {
      Card: { colorBgContainer: '#121633', colorBorderSecondary: '#1e2350' },
      Layout: { colorBgLayout: '#060919', colorBgContainer: '#121633' },
      Progress: { circleTextColor: '#e2e8f0' },
      Tag: { defaultBg: '#1e2350', defaultColor: '#94a3b8', defaultBorderColor: '#2a3066' },
    },
  }

  const lightTheme = {
    token: {
      colorPrimary: '#6366f1',
      colorBgBase: '#f8fafc',
      colorBgContainer: '#ffffff',
      colorBgElevated: '#ffffff',
      colorBorder: '#e2e8f0',
      colorBorderSecondary: '#f1f5f9',
      colorText: '#0f172a',
      colorTextSecondary: '#475569',
      colorTextTertiary: '#64748b',
      colorTextQuaternary: '#94a3b8',
      colorFill: '#f1f5f9',
      colorBgLayout: '#f1f5f9',
      colorInfo: '#3b82f6',
      colorSuccess: '#10b981',
      colorWarning: '#f59e0b',
      colorError: '#ef4444',
      borderRadius: 12,
    },
  }

  // ============== 自定义标题栏（自绘：无边框模式下提供 最小化/最大化/关闭 + 拖拽 + 双击切换最大化） ==============
  const TitleBar = ({ kiosk }: { kiosk: boolean }) => {
    const [maxed, setMaxed] = useState(false)
    useEffect(() => {
      if (kiosk) return
      wailsRuntime.WindowIsMaximised().then(setMaxed).catch(() => {})
    }, [kiosk, isLoggedIn])
    if (kiosk) return null
    // 高对比：让自绘最小化/最大化/关闭一眼就能看到
    const tbBg = isDark ? '#0e1230' : '#f8fafc'
    const tbText = isDark ? '#e2e8f0' : '#0f172a'
    const btnHover = isDark ? 'rgba(139, 92, 246, 0.35)' : 'rgba(99, 102, 241, 0.18)'
    const closeHover = '#ef4444'
    // 真正可用的窗口拖拽：Wails runtime 真实 API + 鼠标位移节流
    const dragStart = (e: React.MouseEvent) => {
      if (e.button !== 0) return
      const target = e.target as HTMLElement
      // 按钮区/输入区不触发拖动（实际上按钮在右侧独立容器里，不影响）
      if (target.closest('button') || target.closest('a') || target.closest('input') || target.closest('select') || target.closest('textarea')) return
      // 兼容旧设备：如果 WindowGetPosition 不可用，直接兜底返回
      if (typeof wailsRuntime.WindowGetPosition !== 'function' || typeof wailsRuntime.WindowSetPosition !== 'function') return
      ;(async () => {
        const pos = await wailsRuntime.WindowGetPosition()
        const startScreenX = e.screenX
        const startScreenY = e.screenY
        const startWinX = pos?.x ?? 0
        const startWinY = pos?.y ?? 0
        // 节流：16ms 内最多一次 SetPos，避免卡顿
        let pendingX = startWinX
        let pendingY = startWinY
        let raf = 0
        let dirty = false
        const flush = () => {
          raf = 0
          if (!dirty) return
          dirty = false
          try { wailsRuntime.WindowSetPosition(pendingX, pendingY) } catch {}
        }
        const onMove = (ev: MouseEvent) => {
          // ★ 自愈：如果左键实际已松开（快速点击/双击时 mouseup 可能先于监听器注册发生，
          //   或 mouseup 落在窗口外未被捕获），立即结束拖拽，避免鼠标"吸附"窗口跟着跑
          if ((ev.buttons & 1) === 0) { onUp(); return }
          pendingX = startWinX + (ev.screenX - startScreenX)
          pendingY = startWinY + (ev.screenY - startScreenY)
          if (!raf) { raf = window.requestAnimationFrame(flush); dirty = true }
          else dirty = true
        }
        const onUp = () => {
          document.removeEventListener('mousemove', onMove, true)
          document.removeEventListener('mouseup', onUp, true)
          if (raf) { cancelAnimationFrame(raf); raf = 0 }
          flush()
          try { wailsRuntime.WindowSetPosition(pendingX, pendingY) } catch {}
        }
        document.addEventListener('mousemove', onMove, true)
        document.addEventListener('mouseup', onUp, true)
      })()
    }
    const winBtnStyle: React.CSSProperties = {
      width: 52, height: 34, borderRadius: 0, border: 'none', boxShadow: 'none',
      color: tbText, background: 'transparent', padding: 0, display: 'inline-flex',
      alignItems: 'center', justifyContent: 'center', fontWeight: 700, cursor: 'pointer',
    }
    const toggleMax = async () => {
      try {
        wailsRuntime.WindowToggleMaximise()
        try {
          const v = await wailsRuntime.WindowIsMaximised()
          setMaxed(v)
        } catch {}
      } catch {}
    }
    return (
      <div
        onMouseDown={dragStart}
        onDoubleClick={toggleMax}
        style={{
          height: 34, width: '100%', flex: '0 0 34px',
          background: tbBg,
          borderBottom: `1px solid ${isDark ? '#3a3f7d' : '#cbd5e1'}`,
          boxShadow: isDark ? '0 1px 0 rgba(255,255,255,0.04) inset' : '0 1px 0 rgba(0,0,0,0.04) inset',
          display: 'flex', alignItems: 'center', justifyContent: 'space-between',
          padding: '0 0 0 14px',
          userSelect: 'none',
          color: tbText, fontSize: 13, position: 'relative', zIndex: 50,
        }}
      >
        <div style={{ display: 'flex', alignItems: 'center', gap: 10, fontWeight: 700, letterSpacing: 0.2 }}>
          {brandLogo ? (
            brandLogo.startsWith('data:') ? (
              <img src={brandLogo} alt="logo" style={{ width: 24, height: 24, objectFit: 'contain' }} />
            ) : (
              <span style={{ fontSize: 18, lineHeight: 1 }}>{brandLogo}</span>
            )
          ) : (
            <CloudServerOutlined style={{ color: '#8b5cf6', fontSize: 18 }} />
          )}
          <span style={{ fontSize: 13 }}>{brandName || '瘦客户端云桌面'}</span>
        </div>
        <div style={{ display: 'flex', alignItems: 'stretch', height: '100%' }}>
          <Tooltip title="最小化" mouseEnterDelay={0.6}>
            <button
              aria-label="minimize"
              style={winBtnStyle}
              onClick={() => { try { wailsRuntime.WindowMinimise() } catch {} }}
              onMouseOver={e => { (e.currentTarget as HTMLButtonElement).style.background = btnHover }}
              onMouseOut={e => { (e.currentTarget as HTMLButtonElement).style.background = 'transparent' }}
            >
              <MinusOutlined style={{ fontSize: 14 }} />
            </button>
          </Tooltip>
          <Tooltip title={maxed ? '还原' : '最大化'} mouseEnterDelay={0.6}>
            <button
              aria-label="maximize"
              style={winBtnStyle}
              onClick={toggleMax}
              onMouseOver={e => { (e.currentTarget as HTMLButtonElement).style.background = btnHover }}
              onMouseOut={e => { (e.currentTarget as HTMLButtonElement).style.background = 'transparent' }}
            >
              {maxed ? (
                <svg viewBox="0 0 14 14" width="12" height="12" fill="none" stroke="currentColor" strokeWidth="1.4">
                  <rect x="3.2" y="1.2" width="9" height="9" rx="1.2" />
                  <rect x="1.2" y="3.2" width="9" height="9" rx="1.2" fill={isDark ? '#050818' : '#e2e8f0'} />
                </svg>
              ) : (
                <svg viewBox="0 0 14 14" width="12" height="12" fill="none" stroke="currentColor" strokeWidth="1.4">
                  <rect x="1.2" y="1.2" width="11.6" height="11.6" rx="1.2" />
                </svg>
              )}
            </button>
          </Tooltip>
          <Tooltip title="关闭" mouseEnterDelay={0.6}>
            <button
              aria-label="close"
              style={{ ...winBtnStyle, width: 62 }}
              onClick={() => {
                // 自绘关闭按钮：不直接 Quit，调用 WindowClose 走"隐藏到系统托盘"逻辑
                // 真正退出请通过托盘菜单的"退出程序"或左侧菜单的"退出"
                ;(async () => {
                  try {
                    await appInstance.WindowClose()
                  } catch (e) {
                    // 兜底：无论什么原因 WindowClose 失败，都先隐藏窗口
                    // （绝不直接 Quit，避免托盘状态错乱；退出逻辑只在"退出程序"菜单里走 Quit）
                    try { wailsRuntime.WindowHide() } catch {}
                  }
                })()
              }}
              onMouseOver={e => {
                const t = e.currentTarget as HTMLButtonElement
                t.style.background = closeHover
                t.style.color = '#fff'
              }}
              onMouseOut={e => {
                const t = e.currentTarget as HTMLButtonElement
                t.style.background = 'transparent'
                t.style.color = tbText
              }}
            >
              <CloseOutlined style={{ fontSize: 14 }} />
            </button>
          </Tooltip>
        </div>
      </div>
    )
  }

  if (!isLoggedIn) {
    return (
      <ConfigProvider locale={zhCN} theme={isDark ? darkTheme : lightTheme}>
        {messageContextHolder}
        <div style={{ display: 'flex', flexDirection: 'column', width: '100%', height: '100%', overflow: 'hidden' }}>
          <TitleBar kiosk={forceFullscreen} />
          <div className="login-container" style={{ flex: 1, minHeight: 0 }}>
          <div className="login-bg-glow"></div>
          <div className="login-card">
            <div className="login-logo">
              <div className="logo-ring"></div>
              {brandLogo ? (
                brandLogo.startsWith('data:') ? (
                  <img src={brandLogo} alt="logo" style={{ width: 80, height: 80, objectFit: 'contain' }} />
                ) : (
                  <div style={{ fontSize: 64, lineHeight: 1 }}>{brandLogo}</div>
                )
              ) : (
                <CloudServerOutlined style={{ fontSize: 64, color: '#6366f1' }} />
              )}
            </div>
            <h1 className="login-title">{brandName || '瘦客户端云桌面'}</h1>
            <p className="login-subtitle">统一入口 · 云端漫游 · 一键直达</p>

            <div className="login-mode-switcher">
              <Segmented
                value={loginMode}
                onChange={(v) => switchLoginMode(v as 'console' | 'local')}
                options={[
                  { label: <span style={{ padding: '0 8px' }}><GlobalOutlined style={{ marginRight: 6 }} />控制台模式</span>, value: 'console' },
                  { label: <span style={{ padding: '0 8px' }}><DisconnectOutlined style={{ marginRight: 6 }} />本地模式</span>, value: 'local' },
                ]}
                style={{ background: isDark ? '#121633' : '#f1f5f9', borderRadius: 10, padding: 4 }}
              />
            </div>

            {loginMode === 'console' ? (
              <form className="login-form" onSubmit={(e) => { e.preventDefault(); handleLogin() }} key="console-form">
                <div className="form-group">
                  <UserOutlined className="form-icon" />
                  <input type="text" value={username} onChange={(e) => setUsername(e.target.value)} placeholder="控制台账号" autoFocus />
                </div>
                <div className="form-group">
                  <SafetyCertificateOutlined className="form-icon" />
                  <input type="password" value={password} onChange={(e) => setPassword(e.target.value)} placeholder="控制台密码" onKeyDown={(e) => { if (e.key === 'Enter') handleLogin() }} />
                </div>
                <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 12, marginTop: 4 }}>
                  <label style={{ display: 'flex', alignItems: 'center', gap: 6, cursor: 'pointer', color: isDark ? '#94a3b8' : '#475569', fontSize: 13 }}>
                    <input type="checkbox" checked={rememberPwd} onChange={e => setRememberPwd(e.target.checked)} style={{ accentColor: '#6366f1' }} /> 记住密码
                  </label>
                  <label style={{ display: 'flex', alignItems: 'center', gap: 6, cursor: 'pointer', color: isDark ? '#94a3b8' : '#475569', fontSize: 13 }}>
                    <input type="checkbox" checked={autoLoginOnStart} onChange={e => setAutoLoginOnStart(e.target.checked)} disabled={!rememberPwd} style={{ accentColor: '#6366f1' }} /> 自动登录
                  </label>
                  <Button type="text" size="small" icon={<SettingOutlined />} onClick={() => setConsoleSettingsOpen(true)} style={{ color: isDark ? '#94a3b8' : '#475569', fontSize: 13 }}>设置</Button>
                </div>
                <button type="submit" className="login-btn" disabled={loading}>{loading ? '正在连接控制台...' : '连接控制台并登录'}</button>
                <p className="login-hint-text">服务器: {serverUrl} · 点击「设置」修改控制台地址</p>
              </form>
            ) : (
              <form className="login-form" onSubmit={(e) => { e.preventDefault(); handleLocalLogin() }} key="local-form">
                <div className="form-group">
                  <UserOutlined className="form-icon" />
                  <input type="text" value={localUsername} onChange={(e) => setLocalUsername(e.target.value)} placeholder="本地账号 (默认 admin)" autoFocus />
                </div>
                <div className="form-group">
                  <SafetyCertificateOutlined className="form-icon" />
                  <input type="password" value={localPassword} onChange={(e) => setLocalPassword(e.target.value)} placeholder="本地密码 (默认 admin)" onKeyDown={(e) => { if (e.key === 'Enter') handleLocalLogin() }} />
                </div>
                <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 12, marginTop: 4 }}>
                  <label style={{ display: 'flex', alignItems: 'center', gap: 6, cursor: 'pointer', color: isDark ? '#94a3b8' : '#475569', fontSize: 13 }}>
                    <input type="checkbox" checked={rememberPwd} onChange={e => setRememberPwd(e.target.checked)} style={{ accentColor: '#6366f1' }} /> 记住密码
                  </label>
                  <label style={{ display: 'flex', alignItems: 'center', gap: 6, cursor: 'pointer', color: isDark ? '#94a3b8' : '#475569', fontSize: 13 }}>
                    <input type="checkbox" checked={autoLoginOnStart} onChange={e => setAutoLoginOnStart(e.target.checked)} disabled={!rememberPwd} style={{ accentColor: '#6366f1' }} /> 自动登录
                  </label>
                </div>
                <button type="submit" className="login-btn local-mode-btn" disabled={loading}>{loading ? '验证中...' : '进入本地模式'}</button>
                <p className="login-hint-text">提示：本地模式无需连接服务器，账号密码存储在本机（默认 admin / admin）</p>
              </form>
            )}

            <div className="login-protocols">
              <span className="protocol-item"><ApiOutlined /> FreeRDP</span>
              <span className="protocol-item"><DesktopOutlined /> Windows RDP</span>
              <span className="protocol-item"><CloudServerOutlined /> SPICE</span>
              <span className="protocol-item"><WifiOutlined /> VNC</span>
              <span className="protocol-item"><ThunderboltOutlined /> Horizon</span>
            </div>

            {/* ★ 登录页电源操作入口（Kiosk 强制全屏模式下无法通过任务栏关机，必须在此提供入口） */}
            <div style={{
              display: 'flex',
              justifyContent: 'center',
              alignItems: 'center',
              gap: 24,
              marginTop: 4,
              marginBottom: 12,
            }}>
              <Tooltip title="关闭计算机" placement="top">
                <button
                  onClick={() => { setPowerAction('shutdown'); setPowerConfirmOpen(true) }}
                  style={{
                    width: 46,
                    height: 46,
                    borderRadius: '50%',
                    border: `1px solid ${isDark ? 'rgba(239,68,68,0.35)' : 'rgba(239,68,68,0.25)'}`,
                    background: isDark ? 'rgba(239,68,68,0.1)' : 'rgba(239,68,68,0.05)',
                    color: '#ef4444',
                    cursor: 'pointer',
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'center',
                    transition: 'all 0.2s',
                  }}
                  onMouseEnter={(e) => {
                    e.currentTarget.style.background = 'rgba(239,68,68,0.18)'
                    e.currentTarget.style.transform = 'translateY(-2px)'
                  }}
                  onMouseLeave={(e) => {
                    e.currentTarget.style.background = isDark ? 'rgba(239,68,68,0.1)' : 'rgba(239,68,68,0.05)'
                    e.currentTarget.style.transform = 'translateY(0)'
                  }}
                >
                  <PoweroffOutlined style={{ fontSize: 18 }} />
                </button>
              </Tooltip>
              <div style={{ width: 1, height: 28, background: isDark ? 'rgba(255,255,255,0.08)' : 'rgba(15,23,42,0.08)' }} />
              <Tooltip title="重新启动计算机" placement="top">
                <button
                  onClick={() => { setPowerAction('restart'); setPowerConfirmOpen(true) }}
                  style={{
                    width: 46,
                    height: 46,
                    borderRadius: '50%',
                    border: `1px solid ${isDark ? 'rgba(99,102,241,0.35)' : 'rgba(99,102,241,0.25)'}`,
                    background: isDark ? 'rgba(99,102,241,0.1)' : 'rgba(99,102,241,0.05)',
                    color: '#6366f1',
                    cursor: 'pointer',
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'center',
                    transition: 'all 0.2s',
                  }}
                  onMouseEnter={(e) => {
                    e.currentTarget.style.background = 'rgba(99,102,241,0.18)'
                    e.currentTarget.style.transform = 'translateY(-2px)'
                  }}
                  onMouseLeave={(e) => {
                    e.currentTarget.style.background = isDark ? 'rgba(99,102,241,0.1)' : 'rgba(99,102,241,0.05)'
                    e.currentTarget.style.transform = 'translateY(0)'
                  }}
                >
                  <ReloadOutlined style={{ fontSize: 18 }} />
                </button>
              </Tooltip>
              <div style={{ width: 1, height: 28, background: isDark ? 'rgba(255,255,255,0.08)' : 'rgba(15,23,42,0.08)' }} />
              <Tooltip title="关于我们" placement="top">
                <button
                  onClick={() => setAboutModalOpen(true)}
                  style={{
                    width: 46,
                    height: 46,
                    borderRadius: '50%',
                    border: `1px solid ${isDark ? 'rgba(100,116,139,0.35)' : 'rgba(100,116,139,0.25)'}`,
                    background: isDark ? 'rgba(100,116,139,0.1)' : 'rgba(100,116,139,0.05)',
                    color: isDark ? '#94a3b8' : '#64748b',
                    cursor: 'pointer',
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'center',
                    transition: 'all 0.2s',
                  }}
                  onMouseEnter={(e) => {
                    e.currentTarget.style.background = isDark ? 'rgba(100,116,139,0.18)' : 'rgba(100,116,139,0.12)'
                    e.currentTarget.style.transform = 'translateY(-2px)'
                  }}
                  onMouseLeave={(e) => {
                    e.currentTarget.style.background = isDark ? 'rgba(100,116,139,0.1)' : 'rgba(100,116,139,0.05)'
                    e.currentTarget.style.transform = 'translateY(0)'
                  }}
                >
                  <InfoCircleOutlined style={{ fontSize: 18 }} />
                </button>
              </Tooltip>
            </div>

            <div className="login-theme-switch">
              <span><BulbOutlined /></span>
              <Switch checked={isDark} onChange={setIsDark} checkedChildren="深色" unCheckedChildren="浅色" />
            </div>
          </div>
          {toasts.map(toast => (
            <div key={toast.id} className={`toast toast-${toast.type}`}>{toast.message}</div>
          ))}
        </div>

        <Modal
          open={consoleSettingsOpen}
          onCancel={() => setConsoleSettingsOpen(false)}
          title={<div style={{ display: 'flex', alignItems: 'center', gap: 8 }}><SettingOutlined style={{ color: '#6366f1' }} /> 控制台地址设置</div>}
          footer={[
            <Button key="cancel" onClick={() => setConsoleSettingsOpen(false)}>取消</Button>,
            <Button key="ok" type="primary" onClick={() => { localStorage.setItem('server_url', serverUrl); setConsoleSettingsOpen(false); showToast('控制台地址已保存', 'success') }} style={{ background: 'linear-gradient(135deg, #6366f1, #8b5cf6)', border: 'none' }}>保存</Button>,
          ]}
          centered
          width={480}
        >
          <div style={{ padding: '8px 0' }}>
            <label style={{ display: 'block', marginBottom: 8, fontSize: 13, color: isDark ? '#94a3b8' : '#475569' }}>控制台服务器地址</label>
            <Input value={serverUrl} onChange={e => setServerUrl(e.target.value)} placeholder="如 http://192.168.1.1:8080" prefix={<ApiOutlined style={{ color: isDark ? '#64748b' : '#94a3b8' }} />} style={{ borderRadius: 8 }} />
            <p style={{ marginTop: 12, fontSize: 12, color: isDark ? '#64748b' : '#64748b' }}>此地址用于连接后端控制台服务器，验证账号密码后进入系统。</p>
          </div>
        </Modal>

        {/* ★ 注意：管理员验证 Modal 只在文件末尾 (L1541 附近) 保留一个实例，避免同一个 open={adminAuthOpen}
               绑定两个 <Modal> 导致 destroyOnClose 状态竞争（密码正确后 setSettingsModalOpen(true) 失效）。
               之前本位置（L1342）有一个重复的 adminAuth Modal，已经移除以修复"输入完密码不弹窗"的 Bug。 */}

        {/* 登录页 - 电源操作二次确认弹窗（Kiosk 全屏模式入口） */}
        <Modal
          open={powerConfirmOpen}
          onCancel={() => !powerLoading && setPowerConfirmOpen(false)}
          title={
            <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
              <ExclamationCircleOutlined style={{ color: powerAction === 'shutdown' ? '#ef4444' : '#6366f1' }} />
              确认{powerAction === 'shutdown' ? '关机' : '重启'}
            </div>
          }
          footer={null}
          centered
          width={420}
          maskClosable={false}
          closable={!powerLoading}
          destroyOnClose
        >
          <div style={{ padding: '20px 16px' }}>
            <div style={{ textAlign: 'center', marginBottom: 24 }}>
              <div style={{
                fontSize: 48, marginBottom: 16,
                color: powerAction === 'shutdown' ? '#ef4444' : '#6366f1',
              }}>
                {powerAction === 'shutdown' ? <PoweroffOutlined /> : <ReloadOutlined />}
              </div>
              <div style={{ fontSize: 17, fontWeight: 600, color: isDark ? '#e2e8f0' : '#0f172a', marginBottom: 8 }}>
                您确定要{powerAction === 'shutdown' ? '关闭计算机' : '重启计算机'}吗？
              </div>
              <div style={{ fontSize: 13, color: isDark ? '#94a3b8' : '#64748b', lineHeight: 1.6 }}>
                {powerAction === 'shutdown'
                  ? '计算机将在 5 秒后关闭，所有未保存的数据将丢失。'
                  : '计算机将在 5 秒后重启，所有未保存的数据将丢失。'}
                <br />本程序将自动退出。
              </div>
            </div>
            <div style={{ display: 'flex', gap: 12, justifyContent: 'center' }}>
              <button
                onClick={() => setPowerConfirmOpen(false)}
                disabled={powerLoading}
                style={{
                  padding: '10px 32px', borderRadius: 10,
                  border: `1px solid ${isDark ? '#334155' : '#cbd5e1'}`,
                  background: 'transparent',
                  color: isDark ? '#e2e8f0' : '#475569',
                  fontSize: 14, fontWeight: 600,
                  cursor: powerLoading ? 'not-allowed' : 'pointer',
                  opacity: powerLoading ? 0.5 : 1, transition: 'all 0.2s',
                }}
              >取消</button>
              <button
                onClick={async () => {
                  if (powerLoading) return
                  setPowerLoading(true)
                  try {
                    if (powerAction === 'shutdown') {
                      await appInstance.ShutdownMachine()
                    } else {
                      await appInstance.RestartMachine()
                    }
                    setTimeout(() => { setPowerLoading(false); setPowerConfirmOpen(false) }, 3000)
                  } catch (e) {
                    setPowerLoading(false)
                    showToast(`操作失败: ${e}`, 'error')
                  }
                }}
                disabled={powerLoading}
                style={{
                  padding: '10px 32px', borderRadius: 10, border: 'none',
                  background: powerAction === 'shutdown' ? '#ef4444' : '#6366f1',
                  color: '#fff', fontSize: 14, fontWeight: 600,
                  cursor: powerLoading ? 'not-allowed' : 'pointer',
                  opacity: powerLoading ? 0.7 : 1, transition: 'all 0.2s',
                  display: 'flex', alignItems: 'center', gap: 8,
                }}
              >
                {powerLoading && <ReloadOutlined spin />}
                确认{powerAction === 'shutdown' ? '关机' : '重启'}
              </button>
            </div>
          </div>
        </Modal>

        {/* 登录页 - 关于我们弹窗（未登录分支也要能打开） */}
        <Modal
          open={aboutModalOpen}
          onCancel={() => setAboutModalOpen(false)}
          title={<div style={{ display: 'flex', alignItems: 'center', gap: 8 }}><InfoCircleOutlined style={{ color: '#6366f1' }} /> 关于我们</div>}
          footer={null}
          centered
          width={480}
          destroyOnClose
        >
          <div style={{ textAlign: 'center', padding: '20px 16px' }}>
            <div style={{ fontSize: 48, marginBottom: 12 }}>☁️</div>
            <h2 style={{ color: isDark ? '#e2e8f0' : '#0f172a', fontSize: 22, fontWeight: 700, marginBottom: 8 }}>
              云桌面客户端
            </h2>
            <p style={{ color: isDark ? '#64748b' : '#64748b', fontSize: 14, marginBottom: 24 }}>
              一款基于 FreeRDP / Windows RDP / SPICE / VNC / Horizon 协议的轻量化云桌面客户端
            </p>
            <div style={{
              background: isDark ? 'rgba(99,102,241,0.1)' : 'rgba(99,102,241,0.05)',
              borderRadius: 12, padding: '16px 20px', marginBottom: 20, textAlign: 'left',
            }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 12 }}>
                <InfoCircleOutlined style={{ color: '#6366f1' }} />
                <span style={{ color: isDark ? '#e2e8f0' : '#0f172a', fontWeight: 600 }}>产品信息</span>
              </div>
              <div style={{ color: isDark ? '#94a3b8' : '#475569', fontSize: 13, lineHeight: 1.8 }}>
                <div>版本：{appVersion}</div>
                <div>协议支持：FreeRDP / Windows RDP / SPICE / VNC / Horizon</div>
                <div>运行环境：Windows 10/11/Server 2019+</div>
              </div>
            </div>
            <div style={{
              color: isDark ? '#475569' : '#94a3b8',
              fontSize: 12, lineHeight: 1.6,
              borderTop: `1px solid ${isDark ? '#1e2350' : '#e2e8f0'}`,
              paddingTop: 16,
              textAlign: 'center',
            }}>
              <div>本应用基于 MIT 协议开源</div>
              <div>© {new Date().getFullYear()} Thin Client Contributors</div>
            </div>
          </div>
        </Modal>
        </div>
      </ConfigProvider>
    )
  }

  return (
    <ConfigProvider locale={zhCN} theme={isDark ? darkTheme : lightTheme}>
      {messageContextHolder}
      <div style={{ display: 'flex', flexDirection: 'column', width: '100%', height: '100%', overflow: 'hidden' }}>
        <TitleBar kiosk={forceFullscreen} />
        <div style={{ flex: 1, minHeight: 0, overflow: 'hidden' }}>
      <Layout style={{ width: '100%', height: '100%', background: 'transparent' }}>
        <Sider collapsible collapsed={collapsed} onCollapse={setCollapsed} width={240} trigger={null} className="app-sider"
          style={{ background: isDark ? '#080c22' : '#ffffff', borderRight: `1px solid ${isDark ? '#1e2350' : '#e2e8f0'}` }}>
          <div className="sider-logo">
            {brandLogo ? (
              brandLogo.startsWith('data:') ? (
                <img src={brandLogo} alt="logo" style={{ width: collapsed ? 32 : 40, height: collapsed ? 32 : 40, objectFit: 'contain' }} />
              ) : (
                <span style={{ fontSize: collapsed ? 32 : 46, lineHeight: 1 }}>{brandLogo}</span>
              )
            ) : (
              <CloudServerOutlined style={{ fontSize: collapsed ? 32 : 46, color: '#6366f1' }} />
            )}
            {!collapsed && <span className="sider-title">{brandName || '瘦客户端云桌面'}</span>}
          </div>
          <div className="sider-menu">
            <MenuButton icon={<DesktopOutlined style={{ fontSize: 24 }} />} label="我的桌面" active={currentView === 'vms'} collapsed={collapsed} onClick={() => setCurrentView('vms')} isDark={isDark} />
          </div>
          <div className="sider-bottom">
            <MenuButton icon={<ReloadOutlined style={{ fontSize: 16 }} />} label="刷新" active={false} collapsed={collapsed} onClick={loadVMs} isDark={isDark} compact />
            <MenuButton icon={<InfoCircleOutlined style={{ fontSize: 16 }} />} label="关于我们" active={aboutModalOpen} collapsed={collapsed} onClick={() => setAboutModalOpen(true)} isDark={isDark} compact />
            <MenuButton icon={<SettingOutlined style={{ fontSize: 16 }} />} label="管理设置" active={settingsModalOpen} collapsed={collapsed} onClick={openAdminAuth} isDark={isDark} compact />
            <MenuButton icon={<LogoutOutlined style={{ fontSize: 16 }} />} label="退出登录" active={false} collapsed={collapsed} onClick={handleLogout} isDark={isDark} compact />
            <MenuButton icon={<PoweroffOutlined style={{ fontSize: 16 }} />} label="电源操作" active={powerModalOpen} collapsed={collapsed} onClick={() => setPowerModalOpen(true)} isDark={isDark} compact />
          </div>
        </Sider>

        <Layout style={{ background: 'transparent' }}>
          <Header className="app-header"
            style={{ background: isDark ? '#0a0e27' : '#f8fafc', borderBottom: `1px solid ${isDark ? '#1e2350' : '#e2e8f0'}` }}>
            <div className="header-left">
              <div className="page-title">
              <DesktopOutlined style={{ color: '#6366f1' }} /> <span>我的桌面</span>
            </div>
            </div>
            <div className="header-right">
              <Tag color={isLocalMode ? 'orange' : 'green'} style={{ marginRight: 12, padding: '2px 12px', borderRadius: 12, fontSize: 12, fontWeight: 600, display: 'flex', alignItems: 'center', gap: 4 }}>
                {isLocalMode ? <><DisconnectOutlined /> 本地模式</> : <><GlobalOutlined /> 控制台模式</>}
              </Tag>
              <Tooltip title={isDark ? '切换浅色' : '切换深色'}>
                <Button type="text" shape="circle" icon={isDark ? <BulbOutlined style={{ color: '#f59e0b' }} /> : <BulbOutlined />} onClick={() => setIsDark(!isDark)} />
              </Tooltip>
              <div className="header-user">
                <div className="user-avatar">{isLocalMode ? 'L' : (username[0]?.toUpperCase() || 'U')}</div>
                <div className="user-detail">
                  <div className="user-name">{isLocalMode ? '本地用户' : (username || '用户')}</div>
                  <div className="user-role">{isLocalMode ? '本地配置' : '管理员'}</div>
                </div>
              </div>
            </div>
          </Header>

          <Content className="app-content" style={{ background: 'transparent' }}>
            {isLocalMode && (
              <div style={{ marginBottom: 16, display: 'flex', justifyContent: 'flex-end' }}>
                <Button type="primary" icon={<PlusOutlined />} onClick={handleAddVM}
                  style={{ background: 'linear-gradient(135deg, #6366f1, #8b5cf6)', border: 'none', borderRadius: 10, height: 38, paddingLeft: 18, paddingRight: 18, fontWeight: 600, fontSize: 14 }}>
                  添加虚拟机
                </Button>
              </div>
            )}
            <VMGridView
              vms={vms}
              loading={loading}
              isDark={isDark}
              isOfflineMode={isLocalMode}
              onStartVM={handleStartVM}
              onStopVM={handleStopVM}
              onRebootVM={handleRebootVM}
              onConnect={handleConnect}
              onEditVM={handleEditVM}
              onDeleteVM={handleDeleteVM}
              onCardSettings={(vm) => { setCardSettingsVM(vm); setCardSettingsOpen(true) }}
              getProtocolIcon={getProtocolIcon}
              getProtocolName={getProtocolName}
              onChangeProtocol={handleChangeVMProtocol}
            />
          </Content>
        </Layout>
      </Layout>

      <VMEditorModal open={vmEditorOpen} editingVM={editingVM} isDark={isDark} onClose={() => { setVmEditorOpen(false); setEditingVM(null) }} onSave={handleSaveVM} />

      {/* 卡片设置弹窗：编辑/删除/协议切换 等隐藏功能 */}
      <Modal
        open={cardSettingsOpen}
        onCancel={() => { setCardSettingsOpen(false); setCardSettingsVM(null) }}
        title={<div style={{ display: 'flex', alignItems: 'center', gap: 8, fontSize: 17 }}><SettingOutlined style={{ color: '#6366f1', fontSize: 18 }} /> 虚拟机设置 · {cardSettingsVM?.name}</div>}
        footer={null}
        centered
        width={540}
      >
        {cardSettingsVM && (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 14, paddingTop: 16, paddingBottom: 8 }}>
            {/* ★ 连接协议快捷切换：已移除。VMEditorModal「🔒 编辑虚拟机信息」里有完整的协议 Select，
                   且带管理员密码校验，作为修改协议的唯一入口更安全，避免与编辑弹窗里的字段重复。 */}

            {/* ★ 编辑虚拟机信息：需要管理员密码验证（和管理设置共用同一套密码 123110 或自定义 admin_password），
                   防止普通用户乱改配置。验证通过后再调用 handleEditVM 打开编辑器。 */}
            <Button
              block
              icon={<SafetyCertificateOutlined />}
              onClick={() => {
                setCardSettingsOpen(false)
                openAdminAuth({ type: 'editVM', vm: cardSettingsVM })
              }}
              style={{ borderRadius: 10, height: 46, fontSize: 15, background: 'linear-gradient(135deg, #6366f1, #8b5cf6)', color: '#fff', border: 'none', fontWeight: 700 }}
            >
              🔒 编辑虚拟机信息
            </Button>
            {/* ★ 通用判断：当前卡片是否有可控制的电源后端（Hyper-V 或 PVE，含兼容旧数据）
                   卡片上已有圆形重启按钮 → 设置弹窗里不再显示"重启虚拟机"（只保留强制断电关机） */}
            {(() => {
              const hasPower =
                (cardSettingsVM.power_backend && cardSettingsVM.power_backend !== 'none') ||
                (cardSettingsVM.protocol === 'spice' && cardSettingsVM.pve_endpoint) ||
                !!cardSettingsVM.hyperv_vmname ||
                (!!cardSettingsVM.pve_endpoint && !!cardSettingsVM.pve_vmid)
              if (!hasPower) return null
              const isHyperV =
                cardSettingsVM.power_backend === 'hyperv' ||
                !!cardSettingsVM.hyperv_vmname
              const isPVE =
                cardSettingsVM.power_backend === 'pve' ||
                (cardSettingsVM.protocol === 'spice' && !!cardSettingsVM.pve_endpoint) ||
                (!!cardSettingsVM.pve_endpoint && !!cardSettingsVM.pve_vmid)
              const backendName = isHyperV ? 'Hyper-V' : (isPVE ? 'PVE' : '')
              return (
                <Popconfirm
                  title={`确定要通过${backendName}强制断电这台虚拟机吗？`}
                  description="⚠️ 强制断电 = 直接断电，相当于物理拔电源！未保存的数据会全部丢失，可能导致磁盘损坏。请仅在虚拟机卡死无响应时使用。"
                  okText="强制断电"
                  cancelText="取消"
                  okButtonProps={{ danger: true }}
                  onConfirm={() => { handleStopVM(cardSettingsVM.vmid, true); setCardSettingsOpen(false) }}
                >
                  <Button
                    block
                    danger
                    icon={<DisconnectOutlined />}
                    style={{ borderRadius: 10, height: 46, fontSize: 15, background: 'linear-gradient(135deg, #dc2626, #b91c1c)', color: '#fff', border: 'none', fontWeight: 600 }}
                  >
                    🔌 强制断电关机
                  </Button>
                </Popconfirm>
              )
            })()}
            <Popconfirm title="确定删除该虚拟机配置？" description="删除后无法恢复" okText="删除" cancelText="取消" okButtonProps={{ danger: true }} onConfirm={() => { handleDeleteVM(cardSettingsVM.vmid); setCardSettingsOpen(false) }}>
              <Button block danger icon={<DeleteOutlined />} style={{ borderRadius: 10, height: 46, fontSize: 15 }}>删除虚拟机</Button>
            </Popconfirm>
          </div>
        )}
      </Modal>

      {toasts.map(toast => (
        <div key={toast.id} className={`toast toast-${toast.type}`}>{toast.message}</div>
      ))}

      {/* 管理员密码验证弹窗（进入设置页面前） */}
      <Modal
        open={adminAuthOpen}
        onCancel={() => { setAdminAuthOpen(false); setAdminPwdInput(''); setAdminAuthError('') }}
        title={<div style={{ display: 'flex', alignItems: 'center', gap: 8 }}><SafetyCertificateOutlined style={{ color: '#6366f1' }} /> 管理员验证</div>}
        footer={[
          <Button key="cancel" onClick={() => { setAdminAuthOpen(false); setAdminPwdInput(''); setAdminAuthError('') }}>取消</Button>,
          <Button key="ok" type="primary" icon={<SafetyCertificateOutlined />} onClick={tryAdminAuth} style={{ background: 'linear-gradient(135deg, #6366f1, #8b5cf6)', border: 'none' }}>确认</Button>,
        ]}
        centered
        width={420}
        destroyOnClose
      >
        <div style={{ padding: '8px 0' }}>
          <div style={{ marginBottom: 12, padding: '10px 14px', background: isDark ? 'rgba(99,102,241,0.1)' : 'rgba(99,102,241,0.05)', borderRadius: 8, fontSize: 13, color: isDark ? '#a5b4fc' : '#6366f1' }}>
            🔒 请输入管理员密码以进入设置页面
          </div>
          <Input.Password
            value={adminPwdInput}
            onChange={(e) => { setAdminPwdInput(e.target.value); setAdminAuthError('') }}
            onPressEnter={tryAdminAuth}
            placeholder="请输入管理员密码"
            prefix={<SafetyCertificateOutlined style={{ color: isDark ? '#64748b' : '#94a3b8' }} />}
            status={adminAuthError ? 'error' : ''}
            style={{ borderRadius: 8 }}
            autoFocus
          />
          {adminAuthError && <div style={{ marginTop: 8, fontSize: 12, color: '#ef4444' }}>{adminAuthError}</div>}
        </div>
      </Modal>

      {/* 管理设置弹窗（带 Tabs） */}
      <Modal
        open={settingsModalOpen}
        onCancel={() => setSettingsModalOpen(false)}
        title={<div style={{ display: 'flex', alignItems: 'center', gap: 8 }}><SettingOutlined style={{ color: '#6366f1' }} /> 管理设置</div>}
        footer={null}
        centered
        width={880}
        destroyOnClose
        styles={{ body: { maxHeight: '72vh', overflowY: 'auto', padding: '16px 20px' } }}
      >
        <SettingsView
          isDark={isDark}
          isLocalMode={isLocalMode}
          brandName={brandName}
          setBrandName={(v) => { setBrandName(v); localStorage.setItem('brand_name', v); }}
          brandLogo={brandLogo}
          setBrandLogo={(v) => { setBrandLogo(v); localStorage.setItem('brand_logo', v); }}
          forceFullscreen={forceFullscreen}
          setForceFullscreen={(v) => { setForceFullscreen(v); localStorage.setItem('force_fullscreen', v ? 'true' : 'false'); }}
          autostartNormal={autostartNormal}
          autostartKiosk={autostartKiosk}
          setAutostartNormal={setAutostartNormal}
          setAutostartKiosk={setAutostartKiosk}
          rememberPwd={rememberPwd}
          setRememberPwd={setRememberPwd}
          autoLoginOnStart={autoLoginOnStart}
          setAutoLoginOnStart={setAutoLoginOnStart}
          protocolConfig={protocolConfig}
          onSaveConfig={handleSaveProtocolConfig}
          onResetConfig={handleResetProtocolConfig}
          getProtocolIcon={getProtocolIcon}
          getProtocolName={getProtocolName}
          localUsername={localUsername}
          onChangePassword={async (oldPwd, newPwd) => {
            try {
              const ok = await authService.LocalChangePassword(oldPwd, newPwd)
              if (ok) showToast('密码修改成功', 'success')
            } catch (e: any) {
              showToast(e.message || '密码修改失败', 'error')
              throw e
            }
          }}
          dependencies={dependencies}
          onRecheck={checkAllDependencies}
        />
      </Modal>

      {/* 关于我们弹窗 */}
      <Modal
        open={aboutModalOpen}
        onCancel={() => setAboutModalOpen(false)}
        title={<div style={{ display: 'flex', alignItems: 'center', gap: 8 }}><InfoCircleOutlined style={{ color: '#6366f1' }} /> 关于我们</div>}
        footer={null}
        centered
        width={480}
        destroyOnClose
      >
        <div style={{ textAlign: 'center', padding: '20px 16px' }}>
          <div style={{ fontSize: 48, marginBottom: 12 }}>☁️</div>
          <h2 style={{ color: isDark ? '#e2e8f0' : '#0f172a', fontSize: 22, fontWeight: 700, marginBottom: 8 }}>
            云桌面客户端
          </h2>
          <p style={{ color: isDark ? '#64748b' : '#64748b', fontSize: 14, marginBottom: 24 }}>
            一款基于 RDP / Horizon / SPICE 协议的轻量化云桌面客户端
          </p>

          <div style={{
            background: isDark ? 'rgba(99,102,241,0.1)' : 'rgba(99,102,241,0.05)',
            borderRadius: 12,
            padding: '16px 20px',
            marginBottom: 20,
            textAlign: 'left',
          }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 12 }}>
              <InfoCircleOutlined style={{ color: '#6366f1' }} />
              <span style={{ color: isDark ? '#e2e8f0' : '#0f172a', fontWeight: 600 }}>产品信息</span>
            </div>
            <div style={{ color: isDark ? '#94a3b8' : '#475569', fontSize: 13, lineHeight: 1.8 }}>
              <div>版本：{appVersion}</div>
              <div>协议支持：RDP / Windows RDP / SPICE / VNC / Horizon</div>
              <div>运行环境：Windows 10/11/Server 2019+</div>
            </div>
          </div>

          <div style={{
            color: isDark ? '#475569' : '#94a3b8',
            fontSize: 12,
            lineHeight: 1.6,
            borderTop: `1px solid ${isDark ? '#1e2350' : '#e2e8f0'}`,
            paddingTop: 16,
            textAlign: 'center',
          }}>
            <div>本应用基于 MIT 协议开源</div>
            <div>© {new Date().getFullYear()} Thin Client Contributors</div>
          </div>
        </div>
      </Modal>

      {/* 电源操作弹窗 - 选择关机/重启 */}
      <Modal
        open={powerModalOpen}
        onCancel={() => setPowerModalOpen(false)}
        title={<div style={{ display: 'flex', alignItems: 'center', gap: 8 }}><PoweroffOutlined style={{ color: '#ef4444' }} /> 电源操作</div>}
        footer={null}
        centered
        width={440}
        destroyOnClose
      >
        <div style={{ padding: '20px 16px' }}>
          <div style={{
            textAlign: 'center',
            color: isDark ? '#94a3b8' : '#64748b',
            fontSize: 13,
            marginBottom: 24,
            lineHeight: 1.6,
          }}>
            请选择您要执行的电源操作
            <div style={{ fontSize: 12, color: isDark ? '#64748b' : '#94a3b8', marginTop: 4 }}>
              （此操作将影响本地计算机，请谨慎操作）
            </div>
          </div>

          <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 16 }}>
            {/* 关机按钮 */}
            <button
              onClick={() => {
                setPowerAction('shutdown')
                setPowerModalOpen(false)
                setPowerConfirmOpen(true)
              }}
              style={{
                display: 'flex',
                flexDirection: 'column',
                alignItems: 'center',
                justifyContent: 'center',
                gap: 10,
                padding: '24px 16px',
                background: isDark ? 'rgba(239,68,68,0.1)' : 'rgba(239,68,68,0.05)',
                border: `1px solid ${isDark ? 'rgba(239,68,68,0.3)' : 'rgba(239,68,68,0.2)'}`,
                borderRadius: 14,
                cursor: 'pointer',
                transition: 'all 0.2s',
                color: '#ef4444',
              }}
              onMouseEnter={(e) => {
                e.currentTarget.style.background = 'rgba(239,68,68,0.15)'
                e.currentTarget.style.transform = 'translateY(-2px)'
              }}
              onMouseLeave={(e) => {
                e.currentTarget.style.background = isDark ? 'rgba(239,68,68,0.1)' : 'rgba(239,68,68,0.05)'
                e.currentTarget.style.transform = 'translateY(0)'
              }}
            >
              <PoweroffOutlined style={{ fontSize: 36, color: '#ef4444' }} />
              <span style={{ fontSize: 16, fontWeight: 600 }}>关机</span>
              <span style={{ fontSize: 11, color: isDark ? '#64748b' : '#94a3b8' }}>关闭计算机电源</span>
            </button>

            {/* 重启按钮 */}
            <button
              onClick={() => {
                setPowerAction('restart')
                setPowerModalOpen(false)
                setPowerConfirmOpen(true)
              }}
              style={{
                display: 'flex',
                flexDirection: 'column',
                alignItems: 'center',
                justifyContent: 'center',
                gap: 10,
                padding: '24px 16px',
                background: isDark ? 'rgba(99,102,241,0.1)' : 'rgba(99,102,241,0.05)',
                border: `1px solid ${isDark ? 'rgba(99,102,241,0.3)' : 'rgba(99,102,241,0.2)'}`,
                borderRadius: 14,
                cursor: 'pointer',
                transition: 'all 0.2s',
                color: '#6366f1',
              }}
              onMouseEnter={(e) => {
                e.currentTarget.style.background = 'rgba(99,102,241,0.15)'
                e.currentTarget.style.transform = 'translateY(-2px)'
              }}
              onMouseLeave={(e) => {
                e.currentTarget.style.background = isDark ? 'rgba(99,102,241,0.1)' : 'rgba(99,102,241,0.05)'
                e.currentTarget.style.transform = 'translateY(0)'
              }}
            >
              <ReloadOutlined style={{ fontSize: 36, color: '#6366f1' }} />
              <span style={{ fontSize: 16, fontWeight: 600 }}>重启</span>
              <span style={{ fontSize: 11, color: isDark ? '#64748b' : '#94a3b8' }}>重新启动计算机</span>
            </button>
          </div>

          <div style={{
            marginTop: 20,
            padding: '12px 16px',
            background: isDark ? 'rgba(234,179,8,0.08)' : 'rgba(234,179,8,0.05)',
            border: `1px solid ${isDark ? 'rgba(234,179,8,0.2)' : 'rgba(234,179,8,0.15)'}`,
            borderRadius: 10,
            display: 'flex',
            alignItems: 'flex-start',
            gap: 8,
          }}>
            <InfoCircleOutlined style={{ color: '#eab308', marginTop: 2 }} />
            <div style={{ fontSize: 12, color: isDark ? '#94a3b8' : '#64748b', lineHeight: 1.6 }}>
              关机/重启操作将在 5 秒后执行，期间请勿断开电源。
              <br />
              系统会自动退出云桌面连接并关闭本程序。
            </div>
          </div>
        </div>
      </Modal>

      {/* 电源操作二次确认弹窗 */}
      <Modal
        open={powerConfirmOpen}
        onCancel={() => !powerLoading && setPowerConfirmOpen(false)}
        title={
          <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            <ExclamationCircleOutlined style={{ color: powerAction === 'shutdown' ? '#ef4444' : '#6366f1' }} />
            确认{powerAction === 'shutdown' ? '关机' : '重启'}
          </div>
        }
        footer={null}
        centered
        width={420}
        maskClosable={false}
        closable={!powerLoading}
        destroyOnClose
      >
        <div style={{ padding: '20px 16px' }}>
          <div style={{
            textAlign: 'center',
            marginBottom: 24,
          }}>
            <div style={{
              fontSize: 48,
              marginBottom: 16,
              color: powerAction === 'shutdown' ? '#ef4444' : '#6366f1',
            }}>
              {powerAction === 'shutdown' ? <PoweroffOutlined /> : <ReloadOutlined />}
            </div>
            <div style={{
              fontSize: 17,
              fontWeight: 600,
              color: isDark ? '#e2e8f0' : '#0f172a',
              marginBottom: 8,
            }}>
              您确定要{powerAction === 'shutdown' ? '关闭计算机' : '重启计算机'}吗？
            </div>
            <div style={{
              fontSize: 13,
              color: isDark ? '#94a3b8' : '#64748b',
              lineHeight: 1.6,
            }}>
              {powerAction === 'shutdown'
                ? '计算机将在 5 秒后关闭，所有未保存的数据将丢失。'
                : '计算机将在 5 秒后重启，所有未保存的数据将丢失。'}
              <br />
              云桌面连接将自动断开，本程序将自动退出。
            </div>
          </div>

          <div style={{ display: 'flex', gap: 12, justifyContent: 'center' }}>
            <button
              onClick={() => setPowerConfirmOpen(false)}
              disabled={powerLoading}
              style={{
                padding: '10px 32px',
                borderRadius: 10,
                border: `1px solid ${isDark ? '#334155' : '#cbd5e1'}`,
                background: 'transparent',
                color: isDark ? '#e2e8f0' : '#475569',
                fontSize: 14,
                fontWeight: 600,
                cursor: powerLoading ? 'not-allowed' : 'pointer',
                opacity: powerLoading ? 0.5 : 1,
                transition: 'all 0.2s',
              }}
            >
              取消
            </button>
            <button
              onClick={async () => {
                if (powerLoading) return
                setPowerLoading(true)
                try {
                  if (powerAction === 'shutdown') {
                    await appInstance.ShutdownMachine()
                  } else {
                    await appInstance.RestartMachine()
                  }
                  // 后端会执行 os.Exit，这里不会返回
                  // 但作为兜底，3秒后关闭确认弹窗
                  setTimeout(() => {
                    setPowerLoading(false)
                    setPowerConfirmOpen(false)
                  }, 3000)
                } catch (e) {
                  setPowerLoading(false)
                  showToast(`操作失败: ${e}`, 'error')
                }
              }}
              disabled={powerLoading}
              style={{
                padding: '10px 32px',
                borderRadius: 10,
                border: 'none',
                background: powerAction === 'shutdown' ? '#ef4444' : '#6366f1',
                color: '#fff',
                fontSize: 14,
                fontWeight: 600,
                cursor: powerLoading ? 'not-allowed' : 'pointer',
                opacity: powerLoading ? 0.7 : 1,
                transition: 'all 0.2s',
                display: 'flex',
                alignItems: 'center',
                gap: 8,
              }}
            >
              {powerLoading && <ReloadOutlined spin />}
              确认{powerAction === 'shutdown' ? '关机' : '重启'}
            </button>
          </div>
        </div>
      </Modal>
        </div>
      </div>
    </ConfigProvider>
  )
}

function MenuButton({ icon, label, active, collapsed, onClick, isDark, compact = false }: {
  icon: React.ReactNode; label: string; active: boolean; collapsed: boolean; onClick: () => void; isDark: boolean; compact?: boolean;
}) {
  return (
    <button
      className={`sider-menu-item ${active ? 'active' : ''} ${compact ? 'sider-menu-item-compact' : ''}`}
      onClick={onClick}
      style={{
        background: active ? (isDark ? 'linear-gradient(135deg, rgba(99,102,241,0.25), rgba(99,102,241,0.05))' : 'linear-gradient(135deg, rgba(99,102,241,0.15), rgba(99,102,241,0.02))') : 'transparent',
        borderColor: active ? '#6366f1' : 'transparent',
        color: active ? '#6366f1' : (isDark ? '#94a3b8' : '#475569'),
        justifyContent: collapsed ? 'center' : 'flex-start',
        padding: compact
          ? (collapsed ? '6px' : '8px 12px')
          : (collapsed ? '16px' : '16px 20px'),
        fontSize: compact ? 13 : 17,
        minHeight: compact ? 36 : 56,
      }}
    >
      <span style={{ marginRight: collapsed ? 0 : (compact ? 8 : 16), display: 'flex', alignItems: 'center', justifyContent: 'center' }}>{icon}</span>
      {!collapsed && <span className="sider-menu-label" style={{ fontSize: compact ? 13 : 16, fontWeight: compact ? 500 : 600 }}>{label}</span>}
    </button>
  )
}

function getProtocolColor(proto: string): string {
  const colors: Record<string, string> = {
    rdp: '#6366f1', spice: '#f59e0b', vnc: '#10b981', ssh: '#3b82f6', horizon: '#8b5cf6',
  }
  return colors[proto] || '#6366f1'
}

/**
 * 本地 SVG 图标库（完全离线，不依赖网络）。
 * 所有图标均采用权威品牌视觉设计，内嵌为 data:image/svg+xml;utf8。
 * smallIcon: 单色白色版（卡片左上角彩色背景方块里显示）
 * largeIcon: 彩色品牌版（卡片右侧大尺寸 LOGO 展示）
 */
function svgUri(svg: string): string {
  // 不需要 base64，直接 UTF-8 百分号编码即可，体积更小
  return 'data:image/svg+xml;utf8,' + encodeURIComponent(svg.trim())
}

/* ===================== 图标常量定义（手写 SVG） ===================== */

// ===== Windows 7（经典四色窗口 + 波浪旗子风格，蓝#00BCF2） =====
const S_WIN7_WHITE = svgUri(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="white"><path d="M3 6c3-1 6-1 9 0v12c-3-1-6-1-9 0V6zm10 0c3-1 6-1 8 0v12c-2-1-5-1-8 0V6z"/></svg>`)
const L_WIN7 = svgUri(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 120 120">
  <!-- Win7 波浪四色旗子 -->
  <g transform="translate(14,12) scale(0.92)">
    <path fill="#F25022" d="M8 6 Q28 0 48 6 L48 56 Q28 50 8 56 Z"/>
    <path fill="#7FBA00" d="M52 6 Q72 0 92 6 L92 56 Q72 50 52 56 Z"/>
    <path fill="#00A4EF" d="M8 60 Q28 54 48 60 L48 110 Q28 104 8 110 Z"/>
    <path fill="#FFB900" d="M52 60 Q72 54 92 60 L92 110 Q72 104 52 110 Z"/>
  </g>
  <!-- 版本标记 -->
  <text x="60" y="116" text-anchor="middle" font-size="11" font-weight="bold" fill="#00BCF2" font-family="Arial">7</text>
</svg>`)

// ===== Windows 10（经典四色窗口 方形，蓝#00A4EF） =====
const S_WIN10_WHITE = svgUri(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="white"><path d="M3 5.5L11 4.3v7.7H3V5.5zm0 9.7H11V22.7L3 21.5v-6.3zm1-9.2v6.2h6.1L10 4.9 4 5.8zm0 10.4l6.1 1v-6.1H4v5.1zm8-11.2L22 3v8.8H12V4.5zm0 10H22V24L12 22.5v-8zm1-8.7V11h8.1L21 4.6l-8 1.2zm0 10l8 1.2V13H13v6.8z"/></svg>`)
const L_WIN10 = svgUri(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 120 120">
  <g transform="translate(8,10) scale(0.86)">
    <path fill="#00A4EF" d="M6 4 L55 0 L55 52 L6 52 Z" />
    <path fill="#F25022" d="M59 0 L108 4 L108 52 L59 52 Z" />
    <path fill="#7FBA00" d="M6 56 L55 56 L55 108 L6 104 Z" />
    <path fill="#FFB900" d="M59 56 L108 56 L108 104 L59 108 Z" />
  </g>
  <!-- 版本标记 -->
  <text x="60" y="116" text-anchor="middle" font-size="11" font-weight="bold" fill="#00A4EF" font-family="Arial">10</text>
</svg>`)

// ===== Windows 11（现代蓝色透视窗口，蓝#0078D4） =====
const S_WIN11_WHITE = svgUri(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="white"><path d="M3 3h8v8H3V3zm10 0h8v8h-8V3zM3 13h8v8H3v-8zm10 0h8v8h-8v-8z"/></svg>`)
const L_WIN11 = svgUri(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 120 120">
  <!-- Win11 现代化蓝色四块窗口（带透视，纯蓝色渐变） -->
  <defs>
    <linearGradient id="w11g" x1="0%" y1="0%" x2="100%" y2="100%">
      <stop offset="0%" stop-color="#0078D4"/>
      <stop offset="100%" stop-color="#005A9E"/>
    </linearGradient>
  </defs>
  <g transform="translate(28,28)">
    <!-- 左上 -->
    <path fill="url(#w11g)" d="M0 0 L28 0 L28 28 L0 28 Z"/>
    <!-- 右上 -->
    <path fill="url(#w11g)" d="M36 0 L64 0 L64 28 L36 28 Z" opacity="0.85"/>
    <!-- 左下 -->
    <path fill="url(#w11g)" d="M0 36 L28 36 L28 64 L0 64 Z" opacity="0.85"/>
    <!-- 右下 -->
    <path fill="url(#w11g)" d="M36 36 L64 36 L64 64 L36 64 Z" opacity="0.7"/>
  </g>
  <!-- 版本标记 -->
  <text x="60" y="116" text-anchor="middle" font-size="11" font-weight="bold" fill="#0078D4" font-family="Arial">11</text>
</svg>`)

// ===== Windows Server 2019（四色窗口 + 服务器机箱，深蓝#0066B3） =====
const S_WINSRV2019_WHITE = svgUri(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="white"><path d="M3 4h7v6H3V4zm9 0h7v6h-7V4zM3 12h7v6H3v-6zm9 0h7v6h-7v-6z"/><circle cx="20" cy="20" r="2" fill="white"/></svg>`)
const L_WINSRV2019 = svgUri(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 120 120">
  <!-- 四色窗口（小） -->
  <g transform="translate(20,10) scale(0.55)">
    <path fill="#00A4EF" d="M6 4 L55 0 L55 52 L6 52 Z" />
    <path fill="#F25022" d="M59 0 L108 4 L108 52 L59 52 Z" />
    <path fill="#7FBA00" d="M6 56 L55 56 L55 108 L6 104 Z" />
    <path fill="#FFB900" d="M59 56 L108 56 L108 104 L59 108 Z" />
  </g>
  <!-- 服务器机箱 -->
  <g transform="translate(30,68)">
    <rect x="0" y="0" width="60" height="40" rx="4" fill="#0066B3"/>
    <rect x="0" y="0" width="60" height="40" rx="4" fill="none" stroke="#004475" stroke-width="1.5"/>
    <!-- 服务器槽位 -->
    <rect x="6" y="6" width="48" height="8" rx="2" fill="#004475"/>
    <rect x="6" y="18" width="48" height="8" rx="2" fill="#004475"/>
    <!-- LED 指示灯 -->
    <circle cx="10" cy="10" r="1.5" fill="#10b981"/>
    <circle cx="10" cy="22" r="1.5" fill="#10b981"/>
    <!-- 光驱 -->
    <rect x="36" y="30" width="18" height="3" rx="1" fill="#004475"/>
  </g>
  <!-- 版本标记 -->
  <text x="60" y="118" text-anchor="middle" font-size="9" font-weight="bold" fill="#0066B3" font-family="Arial">Server 2019</text>
</svg>`)

// ===== Windows Server 2022（蓝色窗口 + 服务器机箱，蓝#0078D4） =====
const S_WINSRV2022_WHITE = svgUri(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="white"><path d="M3 3h7v7H3V3zm9 0h7v7h-7V3zM3 12h7v7H3v-7zm9 0h7v7h-7v-7z"/><rect x="18" y="18" width="4" height="4" rx="0.5" fill="white"/></svg>`)
const L_WINSRV2022 = svgUri(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 120 120">
  <!-- Win11 风格蓝色窗口（小） -->
  <g transform="translate(28,8) scale(0.7)">
    <path fill="#0078D4" d="M0 0 L28 0 L28 28 L0 28 Z"/>
    <path fill="#0078D4" d="M36 0 L64 0 L64 28 L36 28 Z" opacity="0.85"/>
    <path fill="#0078D4" d="M0 36 L28 36 L28 64 L0 64 Z" opacity="0.85"/>
    <path fill="#0078D4" d="M36 36 L64 36 L64 64 L36 64 Z" opacity="0.7"/>
  </g>
  <!-- 服务器机箱 -->
  <g transform="translate(30,66)">
    <rect x="0" y="0" width="60" height="42" rx="4" fill="#0078D4"/>
    <rect x="0" y="0" width="60" height="42" rx="4" fill="none" stroke="#005A9E" stroke-width="1.5"/>
    <!-- 服务器槽位 -->
    <rect x="6" y="6" width="48" height="9" rx="2" fill="#005A9E"/>
    <rect x="6" y="19" width="48" height="9" rx="2" fill="#005A9E"/>
    <!-- LED 指示灯 -->
    <circle cx="10" cy="10" r="1.5" fill="#10b981"/>
    <circle cx="10" cy="23" r="1.5" fill="#10b981"/>
    <!-- 光驱 -->
    <rect x="34" y="32" width="20" height="3" rx="1" fill="#005A9E"/>
  </g>
  <!-- 版本标记 -->
  <text x="60" y="118" text-anchor="middle" font-size="9" font-weight="bold" fill="#0078D4" font-family="Arial">Server 2022</text>
</svg>`)

// ===== Windows Server 2025（深蓝窗口 + 云服务器，蓝#0067C0） =====
const S_WINSRV2025_WHITE = svgUri(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="white"><path d="M3 3h7v7H3V3zm9 0h7v7h-7V3zM3 12h7v7H3v-7zm9 0h7v7h-7v-7z"/><path d="M18 18a3 3 0 0 0-3 3h6a3 3 0 0 0-3-3z" fill="white"/></svg>`)
const L_WINSRV2025 = svgUri(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 120 120">
  <defs>
    <linearGradient id="w25g" x1="0%" y1="0%" x2="100%" y2="100%">
      <stop offset="0%" stop-color="#0067C0"/>
      <stop offset="100%" stop-color="#004578"/>
    </linearGradient>
  </defs>
  <!-- Win11 风格深蓝窗口（小） -->
  <g transform="translate(28,6) scale(0.7)">
    <path fill="url(#w25g)" d="M0 0 L28 0 L28 28 L0 28 Z"/>
    <path fill="url(#w25g)" d="M36 0 L64 0 L64 28 L36 28 Z" opacity="0.85"/>
    <path fill="url(#w25g)" d="M0 36 L28 36 L28 64 L0 64 Z" opacity="0.85"/>
    <path fill="url(#w25g)" d="M36 36 L64 36 L64 64 L36 64 Z" opacity="0.7"/>
  </g>
  <!-- 云服务器图标 -->
  <g transform="translate(24,62)">
    <!-- 云形状 -->
    <path fill="#0067C0" d="M20 30 Q10 30 10 20 Q10 12 20 12 Q22 6 30 6 Q40 6 42 14 Q52 14 52 22 Q52 30 42 30 Z"/>
    <!-- 服务器机箱在云中 -->
    <rect x="20" y="18" width="24" height="16" rx="2" fill="white" opacity="0.9"/>
    <rect x="22" y="20" width="20" height="3" rx="1" fill="#0067C0"/>
    <rect x="22" y="25" width="20" height="3" rx="1" fill="#0067C0"/>
    <rect x="22" y="30" width="14" height="2" rx="1" fill="#0067C0"/>
    <!-- LED -->
    <circle cx="40" cy="21" r="1" fill="#10b981"/>
    <circle cx="40" cy="26" r="1" fill="#10b981"/>
  </g>
  <!-- 版本标记 -->
  <text x="60" y="118" text-anchor="middle" font-size="9" font-weight="bold" fill="#0067C0" font-family="Arial">Server 2025</text>
</svg>`)

// ===== 旧版兼容：Windows 通用 =====
const S_WIN_WHITE = S_WIN10_WHITE
const L_WIN = L_WIN10

// ===== Ubuntu （官方橙#E95420 + 白色圆圈 + 人形 + 两个小圆圈） =====
const S_UB_WHITE = svgUri(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="white">
  <path d="M12 2a10 10 0 1 0 0 20 10 10 0 0 0 0-20zm2.6 4.2a1.4 1.4 0 1 1 1.4 1.4 1.4 1.4 0 0 1-1.4-1.4zM12 4c1.2 0 2.3.2 3.4.6A8.5 8.5 0 0 0 12 6c-2 0-3.9.7-5.4 2a8 8 0 0 1 5.4-4zM5.8 8.4A8.5 8.5 0 0 1 12 6.1c1.7 0 3.3.5 4.6 1.4a1.4 1.4 0 0 1-1.3 0 1.4 1.4 0 0 1-1.4 0 8 8 0 0 0-3.9-1 8 8 0 0 0-4.2 1.9zm8.6-.6a1.4 1.4 0 1 1-1.4 1.4 1.4 1.4 0 0 1 1.4-1.4zM9.6 8.4A1.4 1.4 0 1 1 8.2 9.8 1.4 1.4 0 0 1 9.6 8.4zM4 12a8 8 0 0 1 .1-1.4 8 8 0 0 1 0 2.8A8 8 0 0 1 4 12zm4.1 5.9A8 8 0 0 1 5.1 14a8 8 0 0 1 0-4 8 8 0 0 1 3-2.9A6.9 6.9 0 0 0 5.1 12 6.9 6.9 0 0 0 8.1 18zM12 20a8.5 8.5 0 0 1-5.9-2.6 8 8 0 0 0 11.8 0A8.5 8.5 0 0 1 12 20zm4.9-1.6a7 7 0 0 0-9.8 0A1.4 1.4 0 1 1 9 21a8 8 0 0 1-5.3-4.1A1.4 1.4 0 1 1 6.2 16a5.2 5.2 0 0 0 .9 2.2A8 8 0 0 1 18.2 7.7 5.2 5.2 0 0 0 16 6.4a1.4 1.4 0 1 1 0-2.8 8 8 0 0 1 2 12.2 1.4 1.4 0 0 1-1.1 2.5zm4-8.7a1.4 1.4 0 1 1 1.4-1.4 1.4 1.4 0 0 1-1.4 1.4zm-2.5 1.4A1.4 1.4 0 1 1 14.8 12a1.4 1.4 0 0 1 1.6 1zM12.1 15.6a1.4 1.4 0 1 1-1.4-1.4 1.4 1.4 0 0 1 1.4 1.4z"/>
</svg>`)
const L_UB = svgUri(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 120 120">
  <circle cx="60" cy="60" r="52" fill="#E95420"/>
  <circle cx="60" cy="60" r="42" fill="none" stroke="white" stroke-width="3"/>
  <g fill="white">
    <circle cx="60" cy="24" r="7"/>
    <circle cx="28" cy="82" r="7"/>
    <circle cx="92" cy="82" r="7"/>
    <path d="M43 58a17 17 0 0 1 34 0l-3-5a12 12 0 0 0-28 0z"/>
  </g>
</svg>`)

// ===== CentOS （紫色#262577 + 两个红圈+红带） =====
const S_CE_WHITE = svgUri(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="white"><path d="M12 2a10 10 0 1 0 0 20 10 10 0 0 0 0-20zm0 4a6 6 0 0 1 6 6v1a5 5 0 0 0-10 0v-1a6 6 0 0 1 4-5.9zM12 19a7 7 0 0 1-2-13.7A8 8 0 0 0 5.2 17 7 7 0 0 1 12 19zm3.8-.4a8 8 0 0 0 4.5-6.7 6 6 0 0 1-5 8.2c.2-.5.4-1 .5-1.5z"/></svg>`)
const L_CE = svgUri(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 120 120">
  <circle cx="60" cy="60" r="54" fill="#8B2730"/>
  <circle cx="60" cy="60" r="42" fill="#262577"/>
  <circle cx="60" cy="60" r="30" fill="none" stroke="#9DCDF3" stroke-width="2.5"/>
  <circle cx="60" cy="26" r="8" fill="#8B2730"/>
  <circle cx="60" cy="94" r="8" fill="#8B2730"/>
  <rect x="56" y="18" width="8" height="84" fill="#8B2730"/>
</svg>`)

// ===== Red Hat （红#CC0000 + 帽子白边剪影） =====
const S_RH_WHITE = svgUri(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="white"><path d="M4 17c2 1 5 2 8 2s6-1 8-2c-1 3-5 5-8 5s-7-2-8-5zM12 4a6 6 0 0 0-6 5 4 4 0 0 0 2 4h8a4 4 0 0 0 2-4 6 6 0 0 0-6-5zm0-2a8 8 0 0 1 8 7 6 6 0 0 1-3 5H7a6 6 0 0 1-3-5 8 8 0 0 1 8-7z"/></svg>`)
const L_RH = svgUri(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 120 120">
  <!-- 红色帽子 -->
  <path fill="#CC0000" d="M30 80c10 8 26 12 30 12s20-4 30-12c-5 12-22 20-30 20S35 92 30 80z"/>
  <path fill="#E4002B" d="M60 15a40 40 0 0 0-40 32 28 28 0 0 0 14 26h52a28 28 0 0 0 14-26A40 40 0 0 0 60 15z"/>
  <ellipse cx="60" cy="90" rx="30" ry="5" fill="#A60017"/>
  <!-- 白色帽带 -->
  <path fill="white" d="M30 62a32 32 0 0 1 60 0" stroke="#A60017" stroke-width="2" fill-opacity="0"/>
  <rect x="30" y="62" width="60" height="5" fill="white" opacity="0.85"/>
</svg>`)

// ===== Debian （红#A81D33 + 漩涡 swirl） =====
const S_DB_WHITE = svgUri(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="white"><path d="M12 2a10 10 0 1 0 0 20 10 10 0 0 0 0-20zm2.5 5.2c.3.1.6.4.8 1-1.5 0-3 .9-3 2.2a2.6 2.6 0 0 0 3 2.6c1 0 1.8-.6 2.3-1.3a8 8 0 0 1 1.9 4.7 8 8 0 0 1-4.3 7 8 8 0 0 1-9.5-3.3A8 8 0 0 1 4 12 8 8 0 0 1 7.5 6 8 8 0 0 1 12 4.3c.2 0 .4 0 .5.2-.3.6-.2 1.4 0 2.7z"/></svg>`)
const L_DB = svgUri(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 120 120">
  <circle cx="60" cy="60" r="54" fill="#A81D33"/>
  <g fill="white" transform="translate(30,18)">
    <!-- Debian swirl -->
    <path d="M30 0c-11 0-22 6-27 16-7 15-3 34 10 44a30 30 0 0 0 34 0 30 30 0 0 0 0-42c-8-8-22-8-28 0-8 10-4 24 6 30 6 3 16 2 20-4s0-16-8-20c-4-2-10 0-12 4-2 4 0 10 4 12 2 1 4 1 6 0-1-3-1-6 0-8 1-2 4-3 6-2 3 1 5 4 4 7-1 4-5 7-9 6-5-1-9-6-8-11 2-7 10-10 16-7 4 2 6 8 5 13"/>
  </g>
</svg>`)

// ===== Fedora （蓝#294172 + 白F 蓝色渐变球） =====
const S_FD_WHITE = svgUri(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="white"><path d="M12 2a10 10 0 1 0 0 20 10 10 0 0 0 0-20zm2.5 5.5h-4v2h2v2h-2v5a3 3 0 0 0 3 3h1v-2h-1a1 1 0 0 1-1-1v-2h2l-.5-2h-1.5v-1h2.5z"/></svg>`)
const L_FD = svgUri(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 120 120">
  <defs>
    <radialGradient id="fd" cx="40%" cy="35%" r="70%">
      <stop offset="0%" stop-color="#3C6EB4"/>
      <stop offset="100%" stop-color="#294172"/>
    </radialGradient>
  </defs>
  <circle cx="60" cy="60" r="52" fill="url(#fd)"/>
  <!-- 白色 F -->
  <g fill="white">
    <rect x="42" y="32" width="10" height="56" rx="2"/>
    <rect x="42" y="32" width="32" height="10" rx="2"/>
    <rect x="42" y="54" width="24" height="10" rx="2"/>
  </g>
  <!-- 高光 -->
  <ellipse cx="48" cy="40" rx="12" ry="8" fill="white" opacity="0.22"/>
</svg>`)

// ===== Apple / macOS （黑剪影苹果 + 叶子） =====
const S_AP_WHITE = svgUri(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="white"><path d="M16.4 12.7c0-2.5 2-3.7 2.1-3.8-1.1-1.7-2.9-1.9-3.5-1.9-1.5-.2-2.9.9-3.7.9-.8 0-2-.9-3.2-.9-1.7 0-3.2 1-4.1 2.5-1.8 3.2-.5 7.9 1.3 10.5.9 1.2 1.9 2.6 3.3 2.6 1.3-.1 1.8-.8 3.3-.8s2 .8 3.3.8c1.4 0 2.3-1.3 3.2-2.5 1-1.5 1.4-2.9 1.4-3-.1 0-2.7-1-2.7-4.4zm-2.4-7.5c.7-.8 1.1-2 1-3.2-1 0-2.2.7-2.9 1.5-.6.7-1.2 1.9-1 3 1.1.1 2.2-.6 2.9-1.3z"/></svg>`)
const L_AP = svgUri(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 120 120">
  <!-- 苹果 -->
  <path fill="#111111" d="M82 76c0-14 11-21 12-21-7-10-17-11-21-11-9-1-17 5-21 5s-11-5-19-4C22 47 12 66 12 82c0 18 12 38 24 38 6 0 8-4 15-4s9 4 15 4c13 0 23-17 24-35-1 0-11-.4-15-9z"/>
  <!-- 叶子 -->
  <path fill="#555555" d="M76 20c0-8-7-16-14-20 1 8-6 16-14 20 10 0 14-2 14-2 0 0 0 2 0 2-1 3-4 7-7 8 0 0 12 0 21-8z"/>
</svg>`)

// ===== Android （绿#3DDC84 机器人） =====
const S_AN_WHITE = svgUri(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="white"><path d="M6 10a1 1 0 1 1 1 1 1 1 0 0 1-1-1zm10 0a1 1 0 1 1 1 1 1 1 0 0 1-1-1zM17.5 13a.5.5 0 0 0 0 1h1v6.5A1.5 1.5 0 0 1 17 22a1.5 1.5 0 0 1-1.5-1.5V20h-7v.5A1.5 1.5 0 0 1 7 22a1.5 1.5 0 0 1-1.5-1.5V14h1a.5.5 0 1 0 0-1h-1v-2h13zm-9-3h7a.5.5 0 0 0 .5-.5v-2h-8v2A.5.5 0 0 0 8.5 10zm1.8-5.2 1.4-1.4a.3.3 0 0 1 .4 0l1.4 1.4a.3.3 0 1 1-.4.4L12 3.9l-1.3 1.3a.3.3 0 1 1-.4-.4zM5.5 10v2h-1v-2h1zm13 0v2h-1v-2h1z"/></svg>`)
const L_AN = svgUri(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 120 120">
  <g fill="#3DDC84">
    <!-- 天线 -->
    <line x1="40" y1="26" x2="52" y2="40" stroke="#3DDC84" stroke-width="5" stroke-linecap="round"/>
    <line x1="80" y1="26" x2="68" y2="40" stroke="#3DDC84" stroke-width="5" stroke-linecap="round"/>
    <!-- 头 -->
    <rect x="32" y="38" width="56" height="30" rx="16" ry="16"/>
    <!-- 眼睛 -->
    <circle cx="48" cy="50" r="4" fill="white"/>
    <circle cx="72" cy="50" r="4" fill="white"/>
    <!-- 身体 -->
    <rect x="30" y="70" width="60" height="38" rx="6"/>
    <!-- 手臂 -->
    <rect x="16" y="72" width="12" height="32" rx="6"/>
    <rect x="92" y="72" width="12" height="32" rx="6"/>
    <!-- 腿 -->
    <rect x="42" y="106" width="12" height="10" rx="4"/>
    <rect x="66" y="106" width="12" height="10" rx="4"/>
  </g>
</svg>`)

// ===== Linux（企鹅 Tux 黑/白/黄） =====
const S_LX_WHITE = svgUri(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="white"><path d="M12 2a5 5 0 0 0-5 5v1a6 6 0 0 0-2 10v1a2 2 0 0 0 2 2h1v-1a1 1 0 0 1 2 0V21h4v-1a1 1 0 0 1 2 0v1h1a2 2 0 0 0 2-2v-1a6 6 0 0 0-2-10V7a5 5 0 0 0-5-5zm-1 7a1 1 0 1 1-1 1 1 1 0 0 1 1-1zm3 0a1 1 0 1 1-1 1 1 1 0 0 1 1-1z"/></svg>`)
const L_LX = svgUri(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 120 120">
  <!-- 企鹅身体（黑） -->
  <ellipse cx="60" cy="68" rx="34" ry="42" fill="#111"/>
  <!-- 白肚皮 -->
  <ellipse cx="60" cy="74" rx="22" ry="30" fill="white"/>
  <!-- 头 -->
  <circle cx="60" cy="36" r="24" fill="#111"/>
  <!-- 白脸 -->
  <ellipse cx="60" cy="40" rx="14" ry="12" fill="white"/>
  <!-- 眼睛 -->
  <circle cx="53" cy="36" r="3.5" fill="#111"/>
  <circle cx="67" cy="36" r="3.5" fill="#111"/>
  <circle cx="52" cy="35" r="1" fill="white"/>
  <circle cx="66" cy="35" r="1" fill="white"/>
  <!-- 黄嘴 -->
  <path d="M54 44 Q60 50 66 44 L60 50 Z" fill="#FCC624"/>
  <!-- 黄脚 -->
  <ellipse cx="48" cy="108" rx="10" ry="4" fill="#FCC624"/>
  <ellipse cx="72" cy="108" rx="10" ry="4" fill="#FCC624"/>
  <!-- 翅膀（黑） -->
  <ellipse cx="26" cy="72" rx="8" ry="22" fill="#111" transform="rotate(-15 26 72)"/>
  <ellipse cx="94" cy="72" rx="8" ry="22" fill="#111" transform="rotate(15 94 72)"/>
</svg>`)

// ===== 银河麒麟（红#C8102E 长城/盾牌） =====
const S_KY_WHITE = svgUri(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="white"><path d="M12 2 4 5v6c0 5 3.5 9.5 8 11 4.5-1.5 8-6 8-11V5l-8-3zm-1 14-3-3 1.4-1.4L11 13.2l3.6-3.6L16 11z"/></svg>`)
const L_KY = svgUri(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 120 120">
  <!-- 盾牌底红 -->
  <path fill="#C8102E" d="M60 6 L14 24 v40 c0 30 22 52 46 58 24-6 46-28 46-58 V24 z"/>
  <!-- 金黄龙纹 -->
  <g fill="#FFD100">
    <circle cx="60" cy="60" r="20"/>
    <path d="M60 40 L70 55 L85 58 L75 68 L78 84 L60 76 L42 84 L45 68 L35 58 L50 55 Z"/>
  </g>
  <!-- 盾牌描边 -->
  <path d="M60 6 L14 24 v40 c0 30 22 52 46 58 24-6 46-28 46-58 V24 z" fill="none" stroke="#FFD100" stroke-width="3"/>
</svg>`)

// ===== 统信 UOS（蓝#0081FF 圆形 + U 白字） =====
const S_UO_WHITE = svgUri(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="white"><path d="M12 2a10 10 0 1 0 0 20 10 10 0 0 0 0-20zm-2 6h4v6a2 2 0 0 1-4 0V8z"/></svg>`)
const L_UO = svgUri(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 120 120">
  <circle cx="60" cy="60" r="54" fill="#0081FF"/>
  <!-- 白色 U 形 -->
  <path fill="white" d="M42 40 h12 v36 a6 6 0 0 0 12 0 V40 h12 v36 a18 18 0 0 1-36 0 z"/>
  <circle cx="60" cy="60" r="46" fill="none" stroke="white" stroke-width="2" opacity="0.3"/>
</svg>`)

/* ===================== 主函数：根据 os 名称匹配图标 ===================== */
function getOSInfo(osName: string | undefined | null) {
  const name = (osName || '').toString().toLowerCase().trim()
  let smallIcon = S_WIN10_WHITE
  let largeIcon = L_WIN10
  let iconBg = '#0078D4'   // 图标方块背景色（品牌主色）
  let displayName = osName || 'Windows 桌面'
  let themeColor = '#0078D4' // 卡片装饰色

  // ===== Windows Server 系列（优先匹配，避免被 Windows 通用捕获） =====
  // largeIcon 使用导入的本地官方 SVG 文件（Wikimedia Commons 规范）
  if (name.includes('server 2025') || name.includes('2025')) {
    smallIcon = S_WINSRV2025_WHITE; largeIcon = osWinSrv2025; iconBg = '#0067C0'; themeColor = '#0067C0'
    displayName = 'Windows Server 2025'
  } else if (name.includes('server 2022') || name.includes('2022')) {
    smallIcon = S_WINSRV2022_WHITE; largeIcon = osWinSrv2022; iconBg = '#0078D4'; themeColor = '#0078D4'
    displayName = 'Windows Server 2022'
  } else if (name.includes('server 2019') || name.includes('2019')) {
    smallIcon = S_WINSRV2019_WHITE; largeIcon = osWinSrv2019; iconBg = '#0066B3'; themeColor = '#0066B3'
    displayName = 'Windows Server 2019'
  } else if (name.includes('server 2016') || name.includes('2016')) {
    smallIcon = S_WINSRV2019_WHITE; largeIcon = osWinSrv2019; iconBg = '#0066B3'; themeColor = '#0066B3'
    displayName = 'Windows Server 2016'
  } else if (name.includes('server 2012') || name.includes('2012')) {
    smallIcon = S_WIN7_WHITE; largeIcon = osWin7; iconBg = '#00BCF2'; themeColor = '#00BCF2'
    displayName = 'Windows Server 2012'
  } else if (name.includes('server 2008') || name.includes('2008')) {
    smallIcon = S_WIN7_WHITE; largeIcon = osWin7; iconBg = '#00BCF2'; themeColor = '#00BCF2'
    displayName = 'Windows Server 2008'
  } else if (name.includes('server')) {
    smallIcon = S_WINSRV2022_WHITE; largeIcon = osWinSrv2022; iconBg = '#0078D4'; themeColor = '#0078D4'
    displayName = osName || 'Windows Server'
  }
  // ===== Windows 11 =====
  else if (name.includes('windows 11') || name.includes('win11') || name.includes('win 11') || (name.includes('windows') && name.includes('11'))) {
    smallIcon = S_WIN11_WHITE; largeIcon = osWin11; iconBg = '#0078D4'; themeColor = '#0078D4'
    displayName = 'Windows 11'
  }
  // ===== Windows 10 =====
  else if (name.includes('windows 10') || name.includes('win10') || name.includes('win 10') || (name.includes('windows') && name.includes('10'))) {
    smallIcon = S_WIN10_WHITE; largeIcon = osWin10; iconBg = '#00A4EF'; themeColor = '#00A4EF'
    displayName = 'Windows 10'
  }
  // ===== Windows 7 =====
  else if (name.includes('windows 7') || name.includes('win7') || name.includes('win 7') || (name.includes('windows') && name.includes('7'))) {
    smallIcon = S_WIN7_WHITE; largeIcon = osWin7; iconBg = '#00BCF2'; themeColor = '#00BCF2'
    displayName = 'Windows 7'
  }
  // ===== Windows XP =====
  else if (name.includes('windows xp') || name.includes('winxp') || name.includes('win xp') || name.includes('xp')) {
    smallIcon = S_WIN7_WHITE; largeIcon = osWin7; iconBg = '#00BCF2'; themeColor = '#00BCF2'
    displayName = 'Windows XP'
  }
  // ===== Windows 通用（未指定版本） =====
  else if (name.includes('windows') || name.includes('win')) {
    smallIcon = S_WIN10_WHITE; largeIcon = osWin10; iconBg = '#0078D4'; themeColor = '#0078D4'
    displayName = osName || 'Windows 桌面'
  }
  // ===== Ubuntu =====
  else if (name.includes('ubuntu')) {
    smallIcon = S_UB_WHITE; largeIcon = osUbuntu; iconBg = '#E95420'; themeColor = '#E95420'
    displayName = osName || 'Ubuntu'
  }
  // ===== CentOS =====
  else if (name.includes('centos') || name.includes('cent')) {
    smallIcon = S_CE_WHITE; largeIcon = osCentos; iconBg = '#262577'; themeColor = '#8B2730'
    displayName = osName || 'CentOS'
  }
  // ===== RedHat / RHEL =====
  else if (name.includes('redhat') || name.includes('rhel') || name.includes('red hat')) {
    smallIcon = S_RH_WHITE; largeIcon = L_RH; iconBg = '#CC0000'; themeColor = '#CC0000'
    displayName = osName || 'Red Hat'
  }
  // ===== Debian =====
  else if (name.includes('debian')) {
    smallIcon = S_DB_WHITE; largeIcon = L_DB; iconBg = '#A81D33'; themeColor = '#A81D33'
    displayName = osName || 'Debian'
  }
  // ===== Fedora =====
  else if (name.includes('fedora')) {
    smallIcon = S_FD_WHITE; largeIcon = L_FD; iconBg = '#294172'; themeColor = '#294172'
    displayName = osName || 'Fedora'
  }
  // ===== macOS =====
  else if (name.includes('mac') || name.includes('osx') || name.includes('apple')) {
    smallIcon = S_AP_WHITE; largeIcon = L_AP; iconBg = '#000000'; themeColor = '#555555'
    displayName = osName || 'macOS'
  }
  // ===== Android =====
  else if (name.includes('android')) {
    smallIcon = S_AN_WHITE; largeIcon = L_AN; iconBg = '#3DDC84'; themeColor = '#3DDC84'
    displayName = osName || 'Android'
  }
  // ===== 银河麒麟 =====
  else if (name.includes('kylin') || name.includes('麒麟') || name.includes('银河')) {
    smallIcon = S_KY_WHITE; largeIcon = osKylin; iconBg = '#1355BE'; themeColor = '#1355BE'
    displayName = osName || '银河麒麟'
  }
  // ===== 统信 UOS =====
  else if (name.includes('uos') || name.includes('deepin') || name.includes('统信')) {
    smallIcon = S_UO_WHITE; largeIcon = L_UO; iconBg = '#0081FF'; themeColor = '#0081FF'
    displayName = osName || '统信 UOS'
  }
  // ===== Linux 通用 =====
  else if (name.includes('linux')) {
    smallIcon = S_LX_WHITE; largeIcon = L_LX; iconBg = '#222222'; themeColor = '#FCC624'
    displayName = osName || 'Linux 桌面'
  }

  return { smallIcon, largeIcon, iconBg, displayName, themeColor }
}

function VMGridView({ vms, loading, isDark, isOfflineMode, onStartVM, onStopVM, onRebootVM, onConnect, onEditVM, onDeleteVM, onCardSettings, getProtocolIcon, getProtocolName, onChangeProtocol }: {
  vms: VMInfo[]; loading: boolean; isDark: boolean; isOfflineMode: boolean;
  onStartVM: (id: number) => void; onStopVM: (id: number) => void; onRebootVM: (id: number) => void;
  onConnect: (id: number) => void; onEditVM: (vm: VMInfo) => void; onDeleteVM: (id: number) => void;
  onCardSettings: (vm: VMInfo) => void;
  getProtocolIcon: (p: string) => string; getProtocolName: (p: string) => string;
  onChangeProtocol: (vmid: number, protocol: string) => void;
}) {
  return (
    <div className="vm-grid-view">
      <div className="grid-header" style={{ borderColor: isDark ? '#1e2350' : '#e2e8f0' }}>
        <h3 style={{ color: isDark ? '#e2e8f0' : '#0f172a' }}>
          <DesktopOutlined style={{ marginRight: 8, color: '#6366f1' }} />
          我的虚拟机 ({vms.length})
        </h3>
      </div>
      {vms.length === 0 ? (
        <div className="empty-state">
          <CloudServerOutlined style={{ fontSize: 64, color: isDark ? '#334155' : '#94a3b8' }} />
          <p>暂无可用的虚拟机</p>
        </div>
      ) : (
        <div className="vm-cards-grid">
          {vms.map(vm => {
            const osInfo = getOSInfo(vm.os)
            const isRunning = vm.status === 'running'
            // 是否有「有效电源控制后端」：
            // - power_backend !== 'none'
            // - 或 SPICE 协议 + PVE 地址（老配置，兼容 logic）
            const backend = vm.power_backend ||
              (vm.protocol === 'spice' && vm.pve_endpoint ? 'pve' : 'none')
            const hasPowerControl = backend !== 'none'
            // 状态标签（含过渡态）：颜色 + 文案 + 背景
            const getStatusTag = (st: string) => {
              switch (st) {
                case 'running':
                  return {
                    text: '● 运行中',
                    color: '#10b981',
                    bg: isDark ? 'rgba(16,185,129,0.15)' : 'rgba(16,185,129,0.1)',
                    border: isDark ? 'rgba(16,185,129,0.4)' : 'rgba(16,185,129,0.3)',
                  }
                case 'paused':
                  return {
                    text: '● 已暂停',
                    color: '#f59e0b',
                    bg: isDark ? 'rgba(245,158,11,0.15)' : 'rgba(245,158,11,0.1)',
                    border: isDark ? 'rgba(245,158,11,0.4)' : 'rgba(245,158,11,0.3)',
                  }
                case 'starting':
                  return {
                    text: '● 正在开机，请稍候…',
                    color: '#eab308',
                    bg: isDark ? 'rgba(234,179,8,0.15)' : 'rgba(234,179,8,0.1)',
                    border: isDark ? 'rgba(234,179,8,0.4)' : 'rgba(234,179,8,0.3)',
                  }
                case 'stopping':
                  return {
                    text: '● 正在关机…',
                    color: '#f97316',
                    bg: isDark ? 'rgba(249,115,22,0.15)' : 'rgba(249,115,22,0.1)',
                    border: isDark ? 'rgba(249,115,22,0.4)' : 'rgba(249,115,22,0.3)',
                  }
                case 'rebooting':
                  return {
                    text: '● 正在重启…',
                    color: '#3b82f6',
                    bg: isDark ? 'rgba(59,130,246,0.15)' : 'rgba(59,130,246,0.1)',
                    border: isDark ? 'rgba(59,130,246,0.4)' : 'rgba(59,130,246,0.3)',
                  }
                default:
                  return {
                    text: '● 已停止',
                    color: isDark ? '#94a3b8' : '#64748b',
                    bg: isDark ? 'rgba(100,116,139,0.15)' : 'rgba(100,116,139,0.08)',
                    border: isDark ? 'rgba(100,116,139,0.3)' : 'rgba(100,116,139,0.2)',
                  }
              }
            }
            const tag = getStatusTag(vm.status || 'stopped')
            return (
              <Card key={vm.vmid} className="vm-card-new" variant="borderless"
                style={{
                  background: isDark
                    ? `linear-gradient(145deg, rgba(${parseInt(osInfo.themeColor.slice(1,3),16)},${parseInt(osInfo.themeColor.slice(3,5),16)},${parseInt(osInfo.themeColor.slice(5,7),16)},0.06), rgba(18,22,51,0.85))`
                    : 'linear-gradient(145deg, #ffffff, #f8fafc)',
                  border: `1px solid ${isDark ? '#1e2350' : '#e2e8f0'}`,
                  borderRadius: 16,
                  padding: 0,
                  width: '100%',
                }}>
                <div style={{ padding: 'var(--vmc-top-pad, 10px 14px 6px 14px)' }}>
                  {/* ===== 顶部：系统图标方块 + 名称 + 状态 + 设置齿轮 ===== */}
                  <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', marginBottom: 'var(--vmc-section-gap, 8px)' }}>
                    <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--vmc-name-gap, 10px)' }}>
                      {/* 操作系统小图标方块 */}
                      <div style={{
                        width: 'var(--vmc-icon-box, 36px)', height: 'var(--vmc-icon-box, 36px)', borderRadius: 8,
                        background: osInfo.iconBg,
                        display: 'flex', alignItems: 'center', justifyContent: 'center',
                        boxShadow: `0 4px 14px ${osInfo.iconBg}55`,
                      }}>
                        <img src={osInfo.smallIcon} alt="os" style={{ width: 'var(--vmc-icon-sm, 20px)', height: 'var(--vmc-icon-sm, 20px)', filter: 'brightness(0) invert(1)' }} />
                      </div>
                      <div style={{ minWidth: 0 }}>
                        <div style={{
                          color: isDark ? '#f1f5f9' : '#0f172a',
                          fontSize: 'var(--vmc-name-fs, 15px)', fontWeight: 700, letterSpacing: 0.3, lineHeight: 1.2,
                          whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis',
                        }}>{vm.name}</div>
                        <Tag
                          style={{
                            marginTop: 4, borderRadius: 20, padding: 'var(--vmc-tag-pad, 2px 9px)',
                            background: tag.bg,
                            color: tag.color,
                            border: `1px solid ${tag.border}`,
                            fontSize: 'var(--vmc-tag-fs, 11px)', fontWeight: 600,
                          }}
                        >
                          {tag.text}
                        </Tag>
                      </div>
                    </div>
                    {/* 右上角设置齿轮 */}
                    <Tooltip title="设置">
                      <button
                        onClick={() => onCardSettings(vm)}
                        style={{
                          width: 'var(--vmc-gear, 30px)', height: 'var(--vmc-gear, 30px)', borderRadius: '50%',
                          background: 'transparent',
                          border: 'none', cursor: 'pointer',
                          display: 'flex', alignItems: 'center', justifyContent: 'center',
                          color: isDark ? '#64748b' : '#94a3b8',
                          transition: 'all 0.2s',
                        }}
                        onMouseEnter={(e) => { e.currentTarget.style.background = isDark ? 'rgba(100,116,139,0.15)' : 'rgba(100,116,139,0.08)'; e.currentTarget.style.color = osInfo.themeColor }}
                        onMouseLeave={(e) => { e.currentTarget.style.background = 'transparent'; e.currentTarget.style.color = isDark ? '#64748b' : '#94a3b8' }}
                      >
                        <SettingOutlined style={{ fontSize: 'var(--vmc-gear-icon, 16px)' }} />
                      </button>
                    </Tooltip>
                  </div>

                  {/* ===== 中部：深色背景信息区 + LOGO（加大+上下居中） ===== */}
                  <div style={{ padding: 'var(--vmc-mid-wrap, 4px 0 6px 0)' }}>
                    <div style={{
                      display: 'flex', alignItems: 'center', justifyContent: 'space-between',
                      /* 浅色模式：轻度灰色背景 + 深色文字（信息可读）；
                         深色模式：靛蓝暗调 + 浅色文字（保持原视觉） */
                      background: isDark ? 'rgba(30,35,80,0.6)' : '#e2e8f0',
                      border: isDark ? 'none' : '1px solid #cbd5e1',
                      borderRadius: 'var(--vmc-mid-radius, 9px)',
                      padding: 'var(--vmc-mid-pad, 12px 12px)',
                      /* 上下居中：使用 min-height 配合 alignItems=center */
                      minHeight: 'var(--vmc-mid-minh, 108px)',
                    }}>
                      <div style={{ flex: 1, display: 'flex', flexDirection: 'column', justifyContent: 'center', gap: 'var(--vmc-mid-gap-v, 7px)' }}>
                        <div style={{ display: 'flex', gap: 'var(--vmc-mid-gap-h, 14px)' }}>
                          <div>
                            <div style={{ fontSize: 'var(--vmc-mid-label-fs, 11px)', color: isDark ? '#94a3b8' : '#64748b', marginBottom: 2, fontWeight: 600 }}>操作系统</div>
                            <div style={{ fontSize: 'var(--vmc-mid-value-fs, 14px)', color: isDark ? '#e2e8f0' : '#0f172a', fontWeight: 700 }}>{osInfo.displayName}</div>
                          </div>
                          <div>
                            <div style={{ fontSize: 'var(--vmc-mid-label-fs, 11px)', color: isDark ? '#94a3b8' : '#64748b', marginBottom: 2, fontWeight: 600 }}>协议</div>
                            <div style={{ fontSize: 'var(--vmc-mid-value-fs, 14px)', color: getProtocolColor(vm.protocol), fontWeight: 700 }}>{getProtocolName(vm.protocol)}</div>
                          </div>
                        </div>
                        <div>
                          <div style={{ fontSize: 'var(--vmc-mid-label-fs, 11px)', color: isDark ? '#94a3b8' : '#64748b', marginBottom: 2, fontWeight: 600 }}>节点</div>
                          <div style={{ fontSize: 'var(--vmc-mid-value-fs, 14px)', color: isDark ? '#e2e8f0' : '#0f172a', fontWeight: 700 }}>{vm.node || '本地'}</div>
                        </div>
                      </div>
                      <div style={{
                        width: 'var(--vmc-logo, 56px)', height: 'var(--vmc-logo, 56px)',
                        display: 'flex', alignItems: 'center', justifyContent: 'center',
                        marginLeft: 'var(--vmc-logo-ml, 8px)',
                        /* 浅色模式轻度灰背景上 LOGO 不需要 dark invert，加深 drop-shadow 更清晰 */
                        filter: isDark ? 'drop-shadow(0 6px 16px rgba(0,0,0,0.12))' : 'drop-shadow(0 4px 10px rgba(0,0,0,0.08))',
                      }}>
                        <img src={osInfo.largeIcon} alt="os-logo" style={{
                          width: '100%', height: '100%', objectFit: 'contain',
                          opacity: 0.98,
                        }} />
                      </div>
                    </div>
                  </div>
                </div>

                {/* ===== 底部按钮（加大适配卡片高度） ===== */}
                <div style={{
                  padding: 'var(--vmc-bot-pad, 2px 14px 12px 14px)',
                  display: 'flex', alignItems: 'center', gap: 'var(--vmc-btn-gap, 10px)',
                }}>
                  {/* 停止状态：有电源后端才显示「启 动」按钮，无后端则只显示「一 键 连 接」 */}
                  {!isRunning ? (
                    hasPowerControl ? (
                      <Button type="primary" block icon={<PlayCircleOutlined style={{ fontSize: 'var(--vmc-btn-icon, 18px)' }} />} onClick={() => onStartVM(vm.vmid)}
                        style={{
                          background: 'linear-gradient(135deg, #10b981, #059669)',
                          border: 'none', borderRadius: 'var(--vmc-btn-radius, 10px)', height: 'var(--vmc-btn-h, 46px)', fontWeight: 800, fontSize: 'var(--vmc-btn-fs, 16px)',
                          boxShadow: '0 8px 20px rgba(16,185,129,0.3)',
                          letterSpacing: 1,
                        }}>
                        启 动
                      </Button>
                    ) : (
                      <Button type="primary" block icon={<LinkOutlined style={{ fontSize: 'var(--vmc-btn-icon, 18px)' }} />} onClick={() => onConnect(vm.vmid)}
                        style={{
                          background: 'linear-gradient(135deg, #6366f1, #8b5cf6)',
                          border: 'none', borderRadius: 'var(--vmc-btn-radius, 10px)', height: 'var(--vmc-btn-h, 46px)', fontWeight: 800, fontSize: 'var(--vmc-btn-fs, 16px)',
                          boxShadow: '0 8px 24px rgba(99,102,241,0.35)',
                          letterSpacing: 1,
                        }}>
                        一 键 连 接
                      </Button>
                    )
                  ) : (
                    <>
                      <Button type="primary" icon={<LinkOutlined style={{ fontSize: 'var(--vmc-btn-icon, 18px)' }} />} onClick={() => onConnect(vm.vmid)}
                        style={{
                          flex: 1, height: 'var(--vmc-btn-h, 46px)', fontWeight: 800, fontSize: 'var(--vmc-btn-fs, 16px)',
                          background: 'linear-gradient(135deg, #6366f1, #8b5cf6)',
                          border: 'none', borderRadius: 'var(--vmc-btn-radius, 10px)',
                          boxShadow: '0 8px 24px rgba(99,102,241,0.35)',
                          letterSpacing: 1,
                        }}>
                        一 键 连 接
                      </Button>
                      {hasPowerControl && (
                        <>
                          {/* ★ 重启按钮：仅 running 状态可用（否则灰掉禁用），放在关机按钮前面；加二次确认防止误触 */}
                          <Tooltip title={isRunning ? '重启虚拟机' : '仅运行中的虚拟机可重启'}>
                            <Popconfirm
                              title={`确定要重启「${vm.name}」吗？`}
                              description="重启过程中所有未保存的数据都会丢失，请先在虚拟机里保存好文件后再确认。"
                              okText="立即重启"
                              cancelText="取消"
                              okButtonProps={{ danger: true }}
                              disabled={!isRunning}
                              onConfirm={() => { if (isRunning) onRebootVM(vm.vmid) }}
                            >
                              {/* Popconfirm 需要接收到 onClick 的元素，这里用一个 span 包一层，避免 disabled button 吞掉 click */}
                              <span style={{ display: 'inline-block', cursor: isRunning ? 'pointer' : 'not-allowed' }}>
                                <button
                                  tabIndex={-1}
                                  disabled={!isRunning}
                                  style={{
                                    width: 'var(--vmc-power, 46px)', height: 'var(--vmc-power, 46px)', borderRadius: '50%',
                                    background: isRunning
                                      ? 'radial-gradient(circle at 30% 30%, #60a5fa, #3b82f6 55%, #2563eb)'
                                      : 'radial-gradient(circle at 30% 30%, #94a3b8, #64748b 55%, #475569)',
                                    border: isRunning ? '2px solid rgba(59,130,246,0.45)' : '2px solid rgba(100,116,139,0.35)',
                                    cursor: isRunning ? 'pointer' : 'not-allowed',
                                    color: '#ffffff',
                                    opacity: isRunning ? 1 : 0.75,
                                    display: 'flex', alignItems: 'center', justifyContent: 'center',
                                    boxShadow: isRunning ? '0 6px 20px rgba(59,130,246,0.4)' : 'none',
                                    transition: 'transform 0.15s',
                                    outline: 'none',
                                    padding: 0,
                                  }}
                                  onMouseEnter={(e) => { if (isRunning) e.currentTarget.style.transform = 'scale(1.05)' }}
                                  onMouseLeave={(e) => { e.currentTarget.style.transform = 'scale(1)' }}
                                >
                                  {/* ★ 重启图标：直接使用 Material Design Icons 官方的 refresh 字形（一整条 fill 实心白 path）
                                         — 左上有明显的向外箭头头，和循环圆弧连在一起，用户一眼就懂 */}
                                  <svg
                                    viewBox="0 0 24 24"
                                    style={{ width: 'var(--vmc-power-icon, 22px)', height: 'var(--vmc-power-icon, 22px)', color: '#fff', display: 'block' }}
                                    aria-hidden="true"
                                  >
                                    {/* MDI refresh 图标原始矢量（顺时针环形箭头 + 左上明显的箭头尖），
                                        fill=currentColor 保证实心白色，绝对能看清 */}
                                    <path
                                      fill="currentColor"
                                      d="M17.65 6.35C16.2 4.9 14.21 4 12 4A8 8 0 0 0 4 12a8 8 0 0 0 8 8c3.73 0 6.84-2.55 7.73-6h-2.08c-.82 2.33-3.04 4-5.65 4a6 6 0 0 1-6-6 6 6 0 0 1 6-6c1.66 0 3.14.69 4.22 1.78L13 11h7V4l-2.35 2.35z"
                                    />
                                  </svg>
                                </button>
                              </span>
                            </Popconfirm>
                          </Tooltip>
                          {/* ★ 关机按钮：仍放最后面；加二次确认防止误触 */}
                          <Tooltip title="关机">
                            <Popconfirm
                              title={`确定要关闭「${vm.name}」吗？`}
                              description="关机过程中所有未保存的数据都会丢失，请先在虚拟机里保存好文件后再确认。"
                              okText="立即关机"
                              cancelText="取消"
                              okButtonProps={{ danger: true }}
                              onConfirm={() => onStopVM(vm.vmid)}
                            >
                              <span style={{ display: 'inline-block', cursor: 'pointer' }}>
                                <button
                                  tabIndex={-1}
                                  style={{
                                    width: 'var(--vmc-power, 46px)', height: 'var(--vmc-power, 46px)', borderRadius: '50%',
                                    background: 'radial-gradient(circle at 30% 30%, #ff6b6b, #ef4444 55%, #dc2626)',
                                    border: '2px solid rgba(239,68,68,0.4)',
                                    cursor: 'pointer', color: '#ffffff',
                                    display: 'flex', alignItems: 'center', justifyContent: 'center',
                                    boxShadow: '0 6px 20px rgba(239,68,68,0.4)',
                                    transition: 'transform 0.15s',
                                    outline: 'none',
                                    padding: 0,
                                  }}
                                  onMouseEnter={(e) => { e.currentTarget.style.transform = 'scale(1.05)' }}
                                  onMouseLeave={(e) => { e.currentTarget.style.transform = 'scale(1)' }}
                                >
                                  <PoweroffOutlined style={{ fontSize: 'var(--vmc-power-icon, 22px)' }} />
                                </button>
                              </span>
                            </Popconfirm>
                          </Tooltip>
                        </>
                      )}
                    </>
                  )}
                </div>
              </Card>
            )
          })}
        </div>
      )}
    </div>
  )
}

function InfoChip({ label, value, color, isDark }: { label: string; value: string; color?: string; isDark: boolean }) {
  return (
    <div className="info-chip">
      <span className="chip-label" style={{ color: isDark ? '#64748b' : '#64748b' }}>{label}</span>
      <span className="chip-value" style={{ color: color || (isDark ? '#e2e8f0' : '#0f172a') }}>{value}</span>
    </div>
  )
}

// ============ 统一的管理设置页面（含 Tabs） ============
function SettingsView(props: {
  isDark: boolean; isLocalMode: boolean;
  brandName: string; setBrandName: (v: string) => void;
  brandLogo: string; setBrandLogo: (v: string) => void;
  forceFullscreen: boolean; setForceFullscreen: (v: boolean) => void;
  autostartNormal: boolean; autostartKiosk: boolean;
  setAutostartNormal: (v: boolean) => void; setAutostartKiosk: (v: boolean) => void;
  rememberPwd: boolean; setRememberPwd: (v: boolean) => void;
  autoLoginOnStart: boolean; setAutoLoginOnStart: (v: boolean) => void;
  protocolConfig: ProtocolConfig | null;
  onSaveConfig: (config: ProtocolConfig) => void;
  onResetConfig: () => void;
  getProtocolIcon: (p: string) => string;
  getProtocolName: (p: string) => string;
  localUsername: string;
  onChangePassword: (oldPwd: string, newPwd: string) => Promise<void>;
  dependencies: Record<string, { available: boolean; message: string }>;
  onRecheck: () => void;
}) {
  const { isDark } = props
  const tabBarStyle: React.CSSProperties = {
    background: isDark ? '#0a0e27' : '#f8fafc',
    borderRadius: 12,
    padding: '6px 6px',
    marginBottom: 20,
    border: `1px solid ${isDark ? '#1e2350' : '#e2e8f0'}`,
  }

  return (
    <Tabs
      defaultActiveKey="global"
      size="large"
      tabBarStyle={tabBarStyle}
      items={[
        {
          key: 'global',
          label: <span style={{ fontSize: 15, fontWeight: 600 }}><GlobalOutlined style={{ marginRight: 6, color: '#6366f1' }} />全局设置</span>,
          children: <GlobalSettingsPanel {...props} />,
        },
        {
          key: 'params',
          label: <span style={{ fontSize: 15, fontWeight: 600 }}><SlidersOutlined style={{ marginRight: 6, color: '#6366f1' }} />参数设置</span>,
          children: <ParamsSettingsPanel {...props} />,
        },
        {
          key: 'account',
          label: <span style={{ fontSize: 15, fontWeight: 600 }}><SafetyCertificateOutlined style={{ marginRight: 6, color: '#6366f1' }} />账户安全</span>,
          children: <AccountSecurityPanel {...props} />,
        },
        {
          key: 'deps',
          label: <span style={{ fontSize: 15, fontWeight: 600 }}><CheckCircleOutlined style={{ marginRight: 6, color: '#6366f1' }} />依赖检测</span>,
          children: <DependencyCheckPanel {...props} />,
        },
      ]}
    />
  )
}

// ============ 4 个设置面板（作为 Tabs 的子内容，不含独立 header） ============

function GlobalSettingsPanel({ isDark, isLocalMode, brandName, setBrandName, brandLogo, setBrandLogo, forceFullscreen, setForceFullscreen, autostartNormal, autostartKiosk, setAutostartNormal, setAutostartKiosk, rememberPwd, setRememberPwd, autoLoginOnStart, setAutoLoginOnStart }: {
  isDark: boolean; isLocalMode: boolean;
  brandName: string; setBrandName: (v: string) => void;
  brandLogo: string; setBrandLogo: (v: string) => void;
  forceFullscreen: boolean; setForceFullscreen: (v: boolean) => void;
  autostartNormal: boolean; autostartKiosk: boolean;
  setAutostartNormal: (v: boolean) => void; setAutostartKiosk: (v: boolean) => void;
  rememberPwd: boolean; setRememberPwd: (v: boolean) => void;
  autoLoginOnStart: boolean; setAutoLoginOnStart: (v: boolean) => void;
}) {
  const [msgApi, msgCtxHolder] = message.useMessage()
  const lowToast = (text: string, type: 'success' | 'error' | 'info' = 'success') => {
    const cfg = { content: text, duration: type === 'error' ? 2.5 : 1.5, style: { fontSize: 12, padding: '2px 8px', borderRadius: 6, boxShadow: 'none' } }
    if (type === 'success') msgApi.success(cfg)
    else if (type === 'error') msgApi.error(cfg)
    else msgApi.info(cfg)
  }

  const cardBg = isDark ? '#121633' : '#ffffff'
  const cardBorder = isDark ? '#1e2350' : '#e2e8f0'
  const labelText = { color: isDark ? '#e2e8f0' : '#0f172a' }
  const descText = { color: isDark ? '#64748b' : '#64748b' }
  const formRowStyle: React.CSSProperties = { display: 'flex', alignItems: 'center', justifyContent: 'space-between', padding: '12px 0', borderBottom: `1px solid ${cardBorder}`, gap: 16 }
  const formLabelStyle: React.CSSProperties = { flex: 1, minWidth: 0 }

  return (
    <>
      {msgCtxHolder}
      <div className="protocol-params-grid params-grid-fit">
        {/* 第 1 行（通栏）：显示 / 全屏模式 */}
        <Card className="param-card card-span2" variant="borderless" style={{ background: cardBg, border: `1px solid ${cardBorder}`, borderRadius: 16 }}>
          <div className="param-card-title" style={{ borderBottom: `1px solid ${cardBorder}` }}>
            <span style={{ fontSize: 20, marginRight: 8 }}>🛡️</span>
            <span style={labelText}>显示 / 全屏模式</span>
          </div>
          <div style={{ padding: '4px 4px' }}>
            <div style={formRowStyle}>
              <div style={formLabelStyle}>
                <div style={labelText}>强制 Kiosk 全屏模式</div>
                <div style={{ fontSize: 12, ...descText }}>锁定为无边框全屏，屏蔽 ALT+F4 / Win / Alt+Tab 等系统快捷键；只能通过此开关或下方"退出全屏"按钮恢复</div>
              </div>
              <Switch
                checked={forceFullscreen}
                onChange={async (v) => {
                  try {
                    if (v) {
                      try { wailsRuntime.WindowFullscreen() } catch {}
                      try { await appInstance.ApplyKioskMode() } catch {}
                      try { await appInstance.LockdownKioskClose() } catch {}
                    } else {
                      try { wailsRuntime.WindowUnfullscreen() } catch {}
                      try { await appInstance.UnlockKioskClose() } catch {}
                      try { await appInstance.ExitKioskMode() } catch {}
                    }
                    setForceFullscreen(v);
                    localStorage.setItem('force_fullscreen', v ? 'true' : 'false');
                    lowToast(v ? '已进入全屏无边框模式（ALT+F4 已禁用）' : '已恢复窗口模式', 'success');
                  } catch (e: any) {
                    lowToast((e?.message || String(e)) as string, 'error');
                  }
                }}
              />
            </div>
            {/* 退出全屏唯一入口 */}
            {forceFullscreen && (
              <div style={{ ...formRowStyle, borderBottom: 'none', background: isDark ? 'rgba(239,68,68,0.1)' : 'rgba(239,68,68,0.05)', borderRadius: 8, padding: '12px 16px', margin: '8px 0' }}>
                <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', width: '100%' }}>
                  <div style={formLabelStyle}>
                    <div style={labelText}>退出全屏模式</div>
                    <div style={{ fontSize: 12, color: isDark ? '#fca5a5' : '#dc2626' }}>⚠️ 此为唯一退出入口。全屏状态下所有快捷键（Alt+F4、Win 键、Alt+Tab 等）均被屏蔽。</div>
                  </div>
                  <Popconfirm
                    title="确定退出全屏模式？"
                    description="退出后将恢复窗口模式，可以正常操作本地系统。"
                    okText="退出全屏"
                    cancelText="取消"
                    okButtonProps={{ danger: true }}
                    onConfirm={async () => {
                      try {
                        wailsRuntime.WindowUnfullscreen();
                        await appInstance.UnlockKioskClose();
                        await appInstance.ExitKioskMode();
                        setForceFullscreen(false);
                        localStorage.setItem('force_fullscreen', 'false');
                        lowToast('已退出全屏模式', 'success');
                      } catch (e: any) {
                        lowToast((e?.message || String(e)) as string, 'error');
                      }
                    }}
                  >
                    <Button danger type="primary" icon={<LogoutOutlined />} style={{ borderRadius: 8 }}>退出全屏</Button>
                  </Popconfirm>
                </div>
              </div>
            )}
            {/* 开机启动（普通） */}
            <div style={formRowStyle}>
              <div style={formLabelStyle}>
                <div style={labelText}>开机自动启动（普通模式）</div>
                <div style={{ fontSize: 12, ...descText }}>Windows 登录后自动启动本程序到登录页（HKCU\\Run 注册表）</div>
              </div>
              <Switch
                checked={autostartNormal && !autostartKiosk}
                onChange={async (v) => {
                  try {
                    await appInstance.SetAutostart(v, false)
                    setAutostartNormal(v)
                    setAutostartKiosk(false)
                    lowToast(v ? '已开启开机启动（普通模式）' : '已关闭开机启动', 'success')
                  } catch (e: any) {
                    lowToast((e?.message || String(e)) as string, 'error')
                  }
                }}
              />
            </div>
            <div style={{ ...formRowStyle, borderBottom: 'none' }}>
              <div style={formLabelStyle}>
                <div style={labelText}>开机无感进入主页（Kiosk 自启）</div>
                <div style={{ fontSize: 12, ...descText }}>开机后自动登录 → 直接进入全屏云桌面首页，用户看不到桌面；需先勾选"记住密码+自动登录"</div>
              </div>
              <Switch
                checked={autostartKiosk}
                onChange={async (v) => {
                  try {
                    if (v) {
                      // 先保证记住密码和自动登录的勾选（否则开机后还要手动登录，"无感"不成立）
                      setRememberPwd(true)
                      setAutoLoginOnStart(true)
                      localStorage.setItem('remember_pwd', 'true')
                      localStorage.setItem('auto_login_start', 'true')
                    }
                    await appInstance.SetAutostart(v, v)
                    setAutostartKiosk(v)
                    setAutostartNormal(false)
                    lowToast(v ? '已开启开机无感进入 Kiosk 模式' : '已关闭开机无感自启', 'success')
                  } catch (e: any) {
                    lowToast((e?.message || String(e)) as string, 'error')
                  }
                }}
              />
            </div>
          </div>
        </Card>

        {/* 第 2 行：左 = 本地登录与退出（合并单卡，与右侧 Kiosk 等高对齐），右 = 系统级 Kiosk */}
        <Card className="param-card" variant="borderless" style={{ background: cardBg, border: `1px solid ${cardBorder}`, borderRadius: 16 }}
          styles={{ body: { display: 'flex', flexDirection: 'column', height: '100%', boxSizing: 'border-box' } }}>
          <div className="param-card-title" style={{ borderBottom: `1px solid ${cardBorder}` }}>
            <span style={{ fontSize: 20, marginRight: 8 }}>{isLocalMode ? '🔐' : '⛔'}</span>
            <span style={labelText}>{isLocalMode ? '本地登录与退出' : '退出程序'}</span>
          </div>
          <div style={{ padding: '4px 4px', flex: 1, display: 'flex', flexDirection: 'column' }}>
            {/* 登录行为（仅本地模式） */}
            {isLocalMode && (
              <>
                <div style={formRowStyle}>
                  <div style={formLabelStyle}>
                    <div style={labelText}>记住密码</div>
                    <div style={{ fontSize: 12, ...descText }}>下次打开登录页时自动填充上次登录的账号密码</div>
                  </div>
                  <Switch checked={rememberPwd} onChange={(v) => { setRememberPwd(v); localStorage.setItem('remember_pwd', v ? 'true' : 'false') }} />
                </div>
                <div style={{ ...formRowStyle, borderBottom: 'none' }}>
                  <div style={formLabelStyle}>
                    <div style={labelText}>启动程序时自动登录</div>
                    <div style={{ fontSize: 12, ...descText }}>需先勾选"记住密码"；勾选后打开程序直接跳到云桌面首页</div>
                  </div>
                  <Switch checked={autoLoginOnStart} disabled={!rememberPwd}
                    onChange={(v) => { setAutoLoginOnStart(v); localStorage.setItem('auto_login_start', v ? 'true' : 'false') }} />
                </div>
              </>
            )}

            {/* 退出程序（危险操作，钉在卡片底部，与右侧 Kiosk 卡片底边对齐） */}
            <div style={{ ...formRowStyle, borderBottom: 'none', background: isDark ? 'rgba(239,68,68,0.1)' : 'rgba(239,68,68,0.05)', borderRadius: 8, padding: '12px 16px', margin: 'auto 4px 4px' }}>
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', width: '100%' }}>
                <div style={formLabelStyle}>
                  <div style={labelText}>退出云桌面客户端</div>
                  <div style={{ fontSize: 12, color: isDark ? '#fca5a5' : '#dc2626' }}>
                    ⚠️ 点击后将真正退出本程序（不是隐藏到托盘）。如需重新启动，请从桌面快捷方式或开始菜单打开。
                  </div>
                </div>
                <Popconfirm
                  title="确定要退出程序吗？"
                  description="程序将完全退出，不会在后台运行。"
                  okText="退出程序"
                  cancelText="取消"
                  okButtonProps={{ danger: true }}
                  onConfirm={async () => {
                    try {
                      await appInstance.QuitApplication()
                    } catch (e: any) {
                      lowToast((e?.message || String(e)) as string, 'error')
                    }
                  }}
                >
                  <Button danger type="primary" icon={<PoweroffOutlined />} style={{ borderRadius: 8 }}>退出程序</Button>
                </Popconfirm>
              </div>
            </div>
          </div>
        </Card>

        {/* ★ 系统级嵌入 Kiosk（替换系统 Shell / 自动登录 / 控制面板注册） */}
        <SystemKioskSettingsCard
          isDark={isDark}
          cardBg={cardBg}
          cardBorder={cardBorder}
          labelText={labelText}
          descText={descText}
          formRowStyle={formRowStyle}
          formLabelStyle={formLabelStyle}
          lowToast={lowToast}
        />

        {/* ★ 硬盘保护 (UWF) — 仅 LTSC / Enterprise */}
        <UWFSettingsCard
          isDark={isDark}
          cardBg={cardBg}
          cardBorder={cardBorder}
          labelText={labelText}
          descText={descText}
          formRowStyle={formRowStyle}
          formLabelStyle={formLabelStyle}
          lowToast={lowToast}
        />

        {/* 第 3 行（通栏）：品牌定制 */}
        <Card className="param-card card-span2" variant="borderless" style={{ background: cardBg, border: `1px solid ${cardBorder}`, borderRadius: 16 }}>
          <div className="param-card-title" style={{ borderBottom: `1px solid ${cardBorder}` }}>
            <span style={{ fontSize: 20, marginRight: 8 }}>🎨</span>
            <span style={labelText}>品牌定制（登录页 / 侧边栏）</span>
          </div>
          <div style={{ padding: '4px 4px' }}>
            <div style={formRowStyle}>
              <div style={formLabelStyle}>
                <div style={labelText}>产品名称</div>
                <div style={{ fontSize: 12, ...descText }}>显示在登录页大标题与侧边栏位置（例如：云桌面）</div>
              </div>
              <Input value={brandName} onChange={e => setBrandName(e.target.value)} placeholder="瘦客户端云桌面" style={{ width: 260, borderRadius: 8 }} />
            </div>
            <div style={{ ...formRowStyle, borderBottom: 'none', alignItems: 'flex-start' }}>
              <div style={formLabelStyle}>
                <div style={labelText}>自定义 Logo</div>
                <div style={{ fontSize: 12, ...descText }}>
                  支持上传图片或输入 emoji。推荐 <b>128×128</b> 正方形 PNG（透明背景），文件 ≤ 200KB。登录页、侧边栏会同步显示。
                </div>
              </div>
              <div style={{ display: 'flex', flexDirection: 'column', gap: 8, alignItems: 'flex-start' }}>
                {/* 图片上传 */}
                <Upload
                  accept="image/png,image/jpeg,image/svg+xml,image/gif,image/webp,image/x-icon"
                  showUploadList={false}
                  beforeUpload={(file) => {
                    const isImg = file.type.startsWith('image/')
                    if (!isImg) { message.error('只能上传图片文件'); return false }
                    if (file.size > 200 * 1024) { message.error('图片大小不能超过 200KB'); return false }
                    const reader = new FileReader()
                    reader.onload = (e) => {
                      const base64 = e.target?.result as string
                      setBrandLogo(base64)
                      localStorage.setItem('brand_logo', base64)
                      message.success('Logo 上传成功')
                    }
                    reader.readAsDataURL(file)
                    return false
                  }}
                >
                  <Button icon={<UploadOutlined />} style={{ borderRadius: 8 }}>
                    上传图片
                  </Button>
                </Upload>
                {/* emoji 输入 */}
                <Input
                  value={brandLogo.startsWith('data:') ? '' : brandLogo}
                  onChange={e => { setBrandLogo(e.target.value); localStorage.setItem('brand_logo', e.target.value) }}
                  placeholder="或输入 emoji：☁️ 🖥️ 🚀"
                  style={{ width: 260, borderRadius: 8 }}
                />
                {/* 预览 + 清除 */}
                {brandLogo && (
                  <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                    <span style={{ fontSize: 12, color: isDark ? '#9ca3af' : '#6b7280' }}>预览：</span>
                    {brandLogo.startsWith('data:') ? (
                      <img src={brandLogo} alt="logo" style={{ width: 48, height: 48, objectFit: 'contain', borderRadius: 8, border: `1px solid ${cardBorder}` }} />
                    ) : (
                      <span style={{ fontSize: 36, lineHeight: 1 }}>{brandLogo}</span>
                    )}
                    <Button size="small" type="text" danger icon={<DeleteOutlined />} onClick={() => { setBrandLogo(''); localStorage.removeItem('brand_logo') }}>
                      清除
                    </Button>
                  </div>
                )}
              </div>
            </div>
          </div>
        </Card>
      </div>
    </>
  )
}

// ============ 系统级嵌入 Kiosk 卡片（替换系统 Shell / 自动登录 / 控制面板注册） ============
function SystemKioskSettingsCard(props: {
  isDark: boolean;
  cardBg: string; cardBorder: string;
  labelText: React.CSSProperties; descText: React.CSSProperties;
  formRowStyle: React.CSSProperties; formLabelStyle: React.CSSProperties;
  lowToast: (text: string, type?: 'success' | 'error' | 'info') => void;
}) {
  const { isDark, cardBg, cardBorder, labelText, descText, formRowStyle, formLabelStyle, lowToast } = props
  // ★ 标签+输入框行：标签固定不收缩不换行，输入框弹性伸缩（避免卡片较窄时标签被压成竖排单字）
  const inputRowLabelStyle: React.CSSProperties = { ...formLabelStyle, flex: '0 0 auto', minWidth: 'auto', whiteSpace: 'nowrap' }
  const inputRowInputStyle: React.CSSProperties = { flex: '1 1 auto', minWidth: 0, maxWidth: 320, borderRadius: 8 }
  const [loading, setLoading] = useState(false)
  const [status, setStatus] = useState<SystemKioskStatus | null>(null)
  const [registerUninstall, setRegisterUninstall] = useState(false)
  const [uninstallPublisher, setUninstallPublisher] = useState('Thin Client Team')
  const [uninstallVersion, setUninstallVersion] = useState('1.0.0')
  const [replaceShell, setReplaceShell] = useState(false)
  const [enableAutoLogin, setEnableAutoLogin] = useState(false)
  const [autoLoginUser, setAutoLoginUser] = useState('Administrator')
  const [autoLoginPwd, setAutoLoginPwd] = useState('')
  const [autoLoginDomain, setAutoLoginDomain] = useState('')
  const [showRebootConfirm, setShowRebootConfirm] = useState<{ mode: 'apply' | 'revert'; scheduled: boolean; config?: SystemKioskConfig } | null>(null)
  const [applyBusy, setApplyBusy] = useState(false)
  const [revertBusy, setRevertBusy] = useState(false)

  const load = useCallback(async (silent = false) => {
    try {
      if (!silent) setLoading(true)
      const st = await appInstance.GetSystemKioskStatus()
      setStatus(st)
      setRegisterUninstall(st.uninstallRegistered)
      if (st.uninstallPublisher) setUninstallPublisher(st.uninstallPublisher)
      if (st.uninstallVersion) setUninstallVersion(st.uninstallVersion)
      setReplaceShell(st.shellReplaced)
      setEnableAutoLogin(st.autoLogonEnabled)
      if (st.autoLogonUser) setAutoLoginUser(st.autoLogonUser)
      if (st.autoLogonDomain) setAutoLoginDomain(st.autoLogonDomain)
    } catch (e: any) {
      if (!silent) lowToast((e?.message || String(e)), 'error')
    } finally {
      if (!silent) setLoading(false)
    }
  }, [lowToast])

  useEffect(() => {
    load()
    const t = setTimeout(() => load(true), 300)
    return () => clearTimeout(t)
  }, [load])

  const doApply = async (withReboot: boolean) => {
    setApplyBusy(true)
    try {
      const cfg: SystemKioskConfig = {
        registerUninstall,
        uninstallPublisher: uninstallPublisher.trim(),
        uninstallVersion: uninstallVersion.trim(),
        replaceShell,
        enableAutoLogin,
        autoLoginUser: autoLoginUser.trim(),
        autoLoginPwd,
        autoLoginDomain: autoLoginDomain.trim(),
        scheduleReboot: withReboot,
      }
      if (replaceShell && !cfg.uninstallPublisher) return lowToast('请填写发行商', 'error')
      if (replaceShell && !cfg.uninstallVersion) return lowToast('请填写版本号', 'error')
      if (enableAutoLogin && !cfg.autoLoginUser) return lowToast('请填写 Windows 自动登录用户名', 'error')
      const r = await appInstance.ApplySystemKioskConfig(cfg)
      lowToast('系统嵌入设置已写入注册表' + (r.scheduledReboot ? '，10 秒后自动重启系统…' : '，请手动重启系统生效'), 'success')
      if (!r.scheduledReboot) await load(true)
    } catch (e: any) {
      lowToast((e?.message || String(e)), 'error')
    } finally {
      setApplyBusy(false)
      setShowRebootConfirm(null)
    }
  }

  const doRevert = async (withReboot: boolean) => {
    setRevertBusy(true)
    try {
      const r = await appInstance.RevertSystemKiosk(withReboot)
      lowToast('已解除系统级嵌入（还原 Explorer + 关自动登录 + 删控制面板条目）' + (r.scheduledReboot ? '，10 秒后自动重启系统…' : '，请手动重启系统生效'), 'success')
      if (!r.scheduledReboot) await load(true)
    } catch (e: any) {
      lowToast((e?.message || String(e)), 'error')
    } finally {
      setRevertBusy(false)
      setShowRebootConfirm(null)
    }
  }

  const alertColor = (ok: boolean) => ok ? (isDark ? '#86efac' : '#16a34a') : (isDark ? '#fca5a5' : '#dc2626')

  return (
    <Card className="param-card" variant="borderless" style={{ background: cardBg, border: `1px solid ${cardBorder}`, borderRadius: 16 }}>
      <div className="param-card-title" style={{ borderBottom: `1px solid ${cardBorder}` }}>
        <span style={{ fontSize: 20, marginRight: 8 }}>🖥️</span>
        <span style={labelText}>系统级嵌入 Kiosk（替换 Explorer / 自动登录 / 控制面板注册）</span>
        <span style={{ marginLeft: 12, fontSize: 12, color: isDark ? '#94a3b8' : '#475569' }}>
          当前权限：<span style={{ color: status?.isElevated ? alertColor(true) : alertColor(false) }}>
            {status?.isElevated ? '✔ 已管理员运行（写 HKLM 不弹 UAC）' : '⚠ 普通权限（写入时弹一次 UAC）'}
          </span>
        </span>
        {loading && <Spin size="small" style={{ marginLeft: 12 }} />}
      </div>
      <div style={{ padding: '4px 4px' }}>
        {/* 实时状态条 */}
        <div style={{
          padding: '12px 16px',
          background: isDark ? 'rgba(99,102,241,0.1)' : 'rgba(99,102,241,0.05)',
          borderRadius: 10,
          marginBottom: 16,
          display: 'grid',
          gridTemplateColumns: 'repeat(3, 1fr)',
          gap: 16,
          fontSize: 12,
          color: isDark ? '#cbd5e1' : '#334155',
        }}>
          <div>
            <div style={descText}>控制面板注册</div>
            <div style={{ color: status?.uninstallRegistered ? alertColor(true) : alertColor(false), fontWeight: 600, marginTop: 4 }}>
              {status?.uninstallRegistered ? `已注册 (${status.uninstallPublisher} v${status.uninstallVersion || '-'})` : '未注册'}
            </div>
          </div>
          <div>
            <div style={descText}>系统 Shell</div>
            <div style={{ color: status?.shellReplaced ? alertColor(true) : alertColor(false), fontWeight: 600, marginTop: 4 }}>
              {status?.shellReplaced ? `✔ 已替换 (${status.currentShell || '我们'})` : '未替换 (explorer.exe)'}
            </div>
          </div>
          <div>
            <div style={descText}>Winlogon 自动登录</div>
            <div style={{ color: status?.autoLogonEnabled ? alertColor(true) : alertColor(false), fontWeight: 600, marginTop: 4 }}>
              {status?.autoLogonEnabled ? `✔ 启用 (${(status.autoLogonDomain ? status.autoLogonDomain + '\\' : '') + status.autoLogonUser})` : '未启用'}
            </div>
          </div>
        </div>

        {/* 1. 控制面板注册 */}
        <div style={formRowStyle}>
          <div style={formLabelStyle}>
            <div style={labelText}>注册到「控制面板 → 程序和功能」</div>
            <div style={{ fontSize: 12, ...descText }}>让本软件显示在系统已安装程序列表里，可在控制面板卸载（解除嵌入时会自动删除此条目）</div>
          </div>
          <Switch checked={registerUninstall} onChange={setRegisterUninstall} />
        </div>
        {registerUninstall && (
          <>
            <div style={formRowStyle}>
              <div style={inputRowLabelStyle}><div style={labelText}>发行商</div></div>
              <Input style={inputRowInputStyle} value={uninstallPublisher} onChange={e => setUninstallPublisher(e.target.value)} />
            </div>
            <div style={{ ...formRowStyle, borderBottom: 'none' }}>
              <div style={inputRowLabelStyle}><div style={labelText}>版本号</div></div>
              <Input style={inputRowInputStyle} value={uninstallVersion} onChange={e => setUninstallVersion(e.target.value)} />
            </div>
          </>
        )}
        {!registerUninstall && <div style={{ height: 8 }} />}

        {/* 2. 替换系统 Shell */}
        <div style={{ ...formRowStyle, marginTop: 8 }}>
          <div style={formLabelStyle}>
            <div style={labelText}>替换系统 Shell（Winlogon\Shell）</div>
            <div style={{ fontSize: 12, ...descText }}>重启系统后，用户登录时不加载桌面/任务栏/开始菜单（Explorer 不启动），直接全屏进入云桌面客户端</div>
          </div>
          <Switch checked={replaceShell} onChange={setReplaceShell} />
        </div>

        {/* 3. 自动登录 */}
        <div style={formRowStyle}>
          <div style={formLabelStyle}>
            <div style={labelText}>启用 Windows 自动登录</div>
            <div style={{ fontSize: 12, ...descText }}>开机后跳过登录界面，直接登录指定 Windows 账号（与"替换 Shell"合用 = 开机一步进云桌面，用户看不到任何系统界面）</div>
          </div>
          <Switch checked={enableAutoLogin} onChange={setEnableAutoLogin} />
        </div>
        {enableAutoLogin && (
          <>
            <div style={formRowStyle}>
              <div style={inputRowLabelStyle}><div style={labelText}>Windows 用户名</div></div>
              <Input style={inputRowInputStyle} value={autoLoginUser} onChange={e => setAutoLoginUser(e.target.value)} placeholder="如：Administrator" />
            </div>
            <div style={formRowStyle}>
              <div style={inputRowLabelStyle}><div style={labelText}>Windows 密码</div></div>
              <Input.Password style={inputRowInputStyle} value={autoLoginPwd} onChange={e => setAutoLoginPwd(e.target.value)} placeholder="Windows 登录密码（明文存本机 Winlogon 注册表）" />
            </div>
            <div style={{ ...formRowStyle, borderBottom: 'none' }}>
              <div style={inputRowLabelStyle}><div style={labelText}>域 / 主机名（可空）</div></div>
              <Input style={inputRowInputStyle} value={autoLoginDomain} onChange={e => setAutoLoginDomain(e.target.value)} placeholder="域账号才填；本地账号留空" />
            </div>
          </>
        )}

        {/* 底部按钮区 */}
        <div style={{
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'center',
          gap: 12,
          marginTop: 20,
          paddingTop: 16,
          borderTop: `1px dashed ${cardBorder}`,
          flexWrap: 'wrap',
        }}>
          <div style={{ fontSize: 12, color: isDark ? '#94a3b8' : '#64748b', flex: 1, minWidth: 220 }}>
            💡 写入注册表时若不是管理员身份，会弹出一次 UAC 允许；成功后建议立即重启生效（解除嵌入同样需要重启）
          </div>
          <Popconfirm
            title="解除系统级嵌入？"
            description="将系统 Shell 还原为 Explorer.exe、关闭自动登录、删除控制面板条目。是否立即重启？"
            okText="解除并立即重启"
            cancelText="解除（稍后手动重启）"
            okButtonProps={{ danger: true }}
            onCancel={() => doRevert(false)}
            onConfirm={() => doRevert(true)}
          >
            <Button danger icon={<LockOutlined />} style={{ borderRadius: 8 }} loading={revertBusy}>
              🔓 解除系统嵌入
            </Button>
          </Popconfirm>
          <Button type="primary" icon={<CloudUploadOutlined />} style={{ background: 'linear-gradient(135deg,#6366f1,#8b5cf6)', border: 'none', borderRadius: 8 }}
            loading={applyBusy}
            onClick={() => setShowRebootConfirm({ mode: 'apply', scheduled: false, config: undefined })}
          >
            🔧 应用系统嵌入
          </Button>
        </div>
      </div>

      {/* 应用后的"立即重启确认"Modal */}
      <Modal
        open={!!showRebootConfirm && showRebootConfirm?.mode === 'apply'}
        onCancel={() => setShowRebootConfirm(null)}
        title={<div style={{ display: 'flex', alignItems: 'center', gap: 8 }}><ReloadOutlined style={{ color: '#6366f1' }} /> 应用成功 — 是否立即重启系统？</div>}
        centered width={520}
        destroyOnClose
        footer={[
          <Button key="later" onClick={() => doApply(false)}>稍后手动重启</Button>,
          <Button key="now" type="primary" icon={<ReloadOutlined />} onClick={() => doApply(true)}
            style={{ background: 'linear-gradient(135deg,#6366f1,#8b5cf6)', border: 'none', borderRadius: 8 }}>立即重启（推荐）</Button>,
        ]}
      >
        <div style={{ fontSize: 13, color: isDark ? '#94a3b8' : '#475569', lineHeight: 1.8 }}>
          ✔ 已写入注册表：
          <ul style={{ margin: '8px 0 8px 20px' }}>
            {registerUninstall && <li>控制面板卸载条目（发行商：{uninstallPublisher || '-'}，版本：{uninstallVersion || '-'}）</li>}
            {replaceShell && <li>系统 Shell 替换为本程序（下次登录不加载 Explorer，直接进云桌面）</li>}
            {enableAutoLogin && <li>Winlogon 自动登录：{(autoLoginDomain ? autoLoginDomain + '\\' : '') + autoLoginUser}</li>}
            {!registerUninstall && !replaceShell && !enableAutoLogin && <li>（你没有启用任何嵌入项，相当于"解除嵌入"。可直接点「解除系统嵌入」按钮。）</li>}
          </ul>
          <div style={{ color: isDark ? '#fca5a5' : '#dc2626' }}>
            ⚠️ Shell 替换和自动登录<strong>必须重启系统</strong>才真正生效。未重启前桌面仍是 Explorer。
          </div>
        </div>
      </Modal>
    </Card>
  )
}

// ============ 硬盘保护 (UWF) 卡片（仅 LTSC / Enterprise 系统支持） ============
function UWFSettingsCard(props: {
  isDark: boolean;
  cardBg: string; cardBorder: string;
  labelText: React.CSSProperties; descText: React.CSSProperties;
  formRowStyle: React.CSSProperties; formLabelStyle: React.CSSProperties;
  lowToast: (text: string, type?: 'success' | 'error' | 'info') => void;
}) {
  const { isDark, cardBg, cardBorder, labelText, descText, formRowStyle, formLabelStyle, lowToast } = props
  const [loading, setLoading] = useState(false)
  const [status, setStatus] = useState<any>(null)
  const [busy, setBusy] = useState(false)
  const [rebootModal, setRebootModal] = useState<{ show: boolean; message: string }>({ show: false, message: '' })

  const load = useCallback(async () => {
    try {
      setLoading(true)
      const st = await (appInstance as any).GetUWFStatus()
      setStatus(st)
    } catch (e: any) {
      lowToast('查询 UWF 状态失败: ' + (e?.message || String(e)), 'error')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => { load() }, [load])

  const isOn = !!(status?.filterEnabled && status?.volumeProtected && !status?.pendingDisable)

  const handleToggle = async (v: boolean) => {
    try {
      setBusy(true)
      if (v) {
        const msg = await (appInstance as any).EnableUWF()
        setRebootModal({ show: true, message: msg })
      } else {
        const msg = await (appInstance as any).DisableUWF()
        setRebootModal({ show: true, message: msg })
      }
      await load()
    } catch (e: any) {
      lowToast(e?.message || String(e), 'error')
    } finally {
      setBusy(false)
    }
  }

  return (
    <Card className="param-card" variant="borderless" style={{ background: cardBg, border: `1px solid ${cardBorder}`, borderRadius: 16 }}>
      <div className="param-card-title" style={{ borderBottom: `1px solid ${cardBorder}` }}>
        <span style={{ fontSize: 20, marginRight: 8 }}>💾</span>
        <span style={labelText}>硬盘保护 (UWF)</span>
      </div>
      <Spin spinning={loading}>
        <div style={{ padding: '4px 4px' }}>
          {/* 开关行 */}
          <div style={formRowStyle}>
            <div style={formLabelStyle}>
              <div style={labelText}>启用硬盘保护</div>
              <div style={{ fontSize: 12, ...descText }}>
                开启后 C 盘变为只读，所有写入（包括软件安装、文件修改）在重启后自动还原；仅支持 LTSC 2019 / LTSC 2021 / Enterprise 版
              </div>
            </div>
            <Switch
              checked={isOn}
              loading={busy}
              disabled={!status?.supported}
              onChange={handleToggle}
            />
          </div>

          {/* 状态显示 */}
          {status && (
            <div style={{ ...formRowStyle, borderBottom: 'none', alignItems: 'flex-start', flexDirection: 'column', gap: 8 }}>
              <div style={{ display: 'flex', flexWrap: 'wrap', gap: 8 }}>
                <Tag color={status.supported ? 'blue' : 'default'}>
                  {status.supported ? '✓ 系统支持' : '✗ 系统不支持'}
                </Tag>
                {status.featureEnabled && <Tag color="green">功能已安装</Tag>}
                {status.filterEnabled && <Tag color="green">筛选器已启用</Tag>}
                {status.volumeProtected && <Tag color="green">C 盘已保护</Tag>}
                {status.needRebootStep1 && <Tag color="orange">待重启①（驱动加载）</Tag>}
                {status.needRebootStep2 && <Tag color="orange">待重启②（生效）</Tag>}
                {status.pendingDisable && <Tag color="red">已计划关闭</Tag>}
              </div>
              {status.message && (
                <div style={{
                  fontSize: 12,
                  color: status.needRebootStep1 || status.needRebootStep2
                    ? (isDark ? '#fbbf24' : '#d97706')
                    : (isDark ? '#94a3b8' : '#64748b'),
                  padding: '6px 10px',
                  background: isDark ? 'rgba(251,191,36,0.08)' : 'rgba(217,119,6,0.06)',
                  borderRadius: 6,
                  borderLeft: `3px solid ${isDark ? '#fbbf24' : '#d97706'}`,
                }}>
                  {status.message}
                </div>
              )}
            </div>
          )}
        </div>
      </Spin>

      {/* 重启提示弹窗 */}
      <Modal
        open={rebootModal.show}
        title={<span style={{ fontSize: 16 }}>💾 硬盘保护 (UWF)</span>}
        onCancel={() => setRebootModal({ show: false, message: '' })}
        footer={[
          <Button key="later" onClick={() => setRebootModal({ show: false, message: '' })}>稍后重启</Button>,
          <Button key="reboot" type="primary" danger onClick={async () => {
            setRebootModal({ show: false, message: '' })
            try { await (appInstance as any).RebootSystem() } catch {
              lowToast('重启命令发送失败，请手动重启', 'error')
            }
          }}>立即重启</Button>,
        ]}
        centered
      >
        <div style={{ fontSize: 14, lineHeight: 1.8, whiteSpace: 'pre-wrap' }}>{rebootModal.message}</div>
      </Modal>
    </Card>
  )
}

// ============ 参数设置面板 ============
function ParamsSettingsPanel({ isDark, protocolConfig, onSaveConfig, onResetConfig, getProtocolIcon, getProtocolName }: {
  isDark: boolean;
  protocolConfig: ProtocolConfig | null;
  onSaveConfig: (config: ProtocolConfig) => void;
  onResetConfig: () => void;
  getProtocolIcon: (p: string) => string;
  getProtocolName: (p: string) => string;
}) {
  const [draft, setDraft] = useState<ProtocolConfig | null>(protocolConfig)
  const [saving, setSaving] = useState(false)
  // USB 设备列表
  const [usbDeviceList, setUsbDeviceList] = useState<USBDevice[]>([])
  const [usbLoading, setUsbLoading] = useState(false)
  const [selectedUsbDevices, setSelectedUsbDevices] = useState<Set<string>>(new Set())

  useEffect(() => { setDraft(protocolConfig) }, [protocolConfig])

  const updateField = <K extends keyof ProtocolConfig>(key: K, value: ProtocolConfig[K]) => {
    setDraft(prev => prev ? { ...prev, [key]: value } : prev)
  }

  // 加载 USB 设备列表
  const loadUSBDevices = async () => {
    setUsbLoading(true)
    try {
      const devices = await protocolService.ListUSBDevices()
      setUsbDeviceList(devices || [])
      if (draft?.rdp_usbDevices) {
        const saved = new Set(draft.rdp_usbDevices.split(',').map(s => s.trim()).filter(Boolean))
        setSelectedUsbDevices(saved)
      }
    } catch (e) {
      message.error('加载USB设备列表失败')
    } finally {
      setUsbLoading(false)
    }
  }

  // 切换 USB 设备选中状态
  const toggleUsbDevice = (vidPid: string) => {
    setSelectedUsbDevices(prev => {
      const next = new Set(prev)
      if (next.has(vidPid)) next.delete(vidPid)
      else next.add(vidPid)
      updateField('rdp_usbDevices', Array.from(next).join(','))
      return next
    })
  }

  const handleSave = () => {
    if (!draft) return
    setSaving(true)
    setTimeout(() => { onSaveConfig(draft); setSaving(false); message.success('参数已保存') }, 200)
  }
  const handleReset = () => { onResetConfig() }

  const cardBg = isDark ? '#121633' : '#ffffff'
  const cardBorder = isDark ? '#1e2350' : '#e2e8f0'
  const labelText = { color: isDark ? '#e2e8f0' : '#0f172a' }
  const descText = { color: isDark ? '#64748b' : '#64748b' }
  const formRowStyle: React.CSSProperties = { display: 'flex', alignItems: 'center', justifyContent: 'space-between', padding: '12px 0', borderBottom: `1px solid ${cardBorder}`, gap: 16 }
  const formLabelStyle: React.CSSProperties = { flex: 1, minWidth: 0 }

  return (
    <>
      {/* 参数设置的操作按钮放在内容上方 */}
      <div style={{ display: 'flex', justifyContent: 'flex-end', marginBottom: 16 }}>
        <Space>
          <Button onClick={handleReset} style={{ height: 38, borderRadius: 10 }}>恢复默认</Button>
          <Button type="primary" icon={<SafetyCertificateOutlined />} loading={saving} onClick={handleSave}
            style={{ background: 'linear-gradient(135deg, #6366f1, #8b5cf6)', border: 'none', borderRadius: 10, height: 38, paddingLeft: 20, paddingRight: 20, fontWeight: 600 }}>保存参数</Button>
        </Space>
      </div>

      {!draft ? (
        <div style={{ textAlign: 'center', padding: 60, color: descText.color }}>配置加载中...</div>
      ) : (
        <div className="protocol-params-grid params-grid-auto">
          {/* RDP 参数 */}
          <Card className="param-card" variant="borderless" style={{ background: cardBg, border: `1px solid ${cardBorder}`, borderRadius: 16 }}>
            <div className="param-card-title" style={{ borderBottom: `1px solid ${cardBorder}` }}>
              <span style={{ fontSize: 22, marginRight: 8 }}>{getProtocolIcon('rdp')}</span>
              <span style={labelText}>FreeRDP</span>
              <Tag color="blue" style={{ marginLeft: 8 }}>远程桌面</Tag>
            </div>
            <div style={{ padding: '4px 4px' }}>
              <div style={formRowStyle}>
                <div style={formLabelStyle}><div style={labelText}>分辨率</div><div style={{ fontSize: 12, ...descText }}>桌面显示分辨率</div></div>
                <Select value={draft.rdp_resolution} onChange={v => updateField('rdp_resolution', v)} style={{ width: 160 }}
                  options={[
                    { value: '1024x768', label: '1024 × 768' }, { value: '1280x720', label: '1280 × 720' }, { value: '1280x1024', label: '1280 × 1024' },
                    { value: '1366x768', label: '1366 × 768' }, { value: '1440x900', label: '1440 × 900' }, { value: '1600x900', label: '1600 × 900' },
                    { value: '1680x1050', label: '1680 × 1050' }, { value: '1920x1080', label: '1920 × 1080 (FHD)' },
                    { value: '2560x1440', label: '2560 × 1440 (2K)' }, { value: '3840x2160', label: '3840 × 2160 (4K)' },
                  ]} />
              </div>
              <div style={formRowStyle}>
                <div style={formLabelStyle}><div style={labelText}>颜色深度</div><div style={{ fontSize: 12, ...descText }}>每像素位数 (bpp)</div></div>
                <Select value={draft.rdp_colorDepth} onChange={v => updateField('rdp_colorDepth', v)} style={{ width: 160 }}
                  options={[{ value: 8, label: '8 位' }, { value: 15, label: '15 位' }, { value: 16, label: '16 位' }, { value: 24, label: '24 位' }, { value: 32, label: '32 位' }]} />
              </div>
              <div style={formRowStyle}>
                <div style={formLabelStyle}><div style={labelText}>全屏模式</div><div style={{ fontSize: 12, ...descText }}>启动时进入全屏</div></div>
                <Switch checked={draft.rdp_fullscreen} onChange={v => updateField('rdp_fullscreen', v)} />
              </div>
              <div style={formRowStyle}>
                <div style={formLabelStyle}><div style={labelText}>音频重定向</div><div style={{ fontSize: 12, ...descText }}>在本地播放远程音频</div></div>
                <Switch checked={draft.rdp_audio} onChange={v => updateField('rdp_audio', v)} />
              </div>
              <div style={formRowStyle}>
                <div style={formLabelStyle}><div style={labelText}>剪贴板共享</div><div style={{ fontSize: 12, ...descText }}>允许本地与远程共享剪贴板</div></div>
                <Switch checked={draft.rdp_clipboard} onChange={v => updateField('rdp_clipboard', v)} />
              </div>
              <div style={formRowStyle}>
                <div style={formLabelStyle}><div style={labelText}>驱动器映射</div><div style={{ fontSize: 12, ...descText }}>将本地磁盘映射到远程会话</div></div>
                <Switch checked={draft.rdp_driveMapping} onChange={v => updateField('rdp_driveMapping', v)} />
              </div>
              <div style={formRowStyle}>
                <div style={formLabelStyle}><div style={labelText}>USB 设备重定向</div><div style={{ fontSize: 12, ...descText }}>将本地 USB 设备转发到远程桌面</div></div>
                <Switch checked={!!draft.rdp_usb} onChange={v => updateField('rdp_usb', v)} />
              </div>
              {draft.rdp_usb && (
                <div style={{ ...formRowStyle, flexDirection: 'column', alignItems: 'stretch', paddingLeft: 12, borderLeft: '2px solid rgba(99,102,241,0.3)' }}>
                  <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 10 }}>
                    <div style={{ fontSize: 13, fontWeight: 500, color: isDark ? '#ccc' : '#555' }}>重定向模式</div>
                    <Segmented size="small" value={draft.rdp_usbMode || 'auto'} onChange={v => updateField('rdp_usbMode', v as string)}
                      options={[{ label: '自动（全部设备）', value: 'auto' }, { label: '手动（选择设备）', value: 'manual' }]} />
                  </div>
                  {draft.rdp_usbMode !== 'manual' && (
                    <div style={{ fontSize: 12, color: isDark ? '#888' : '#666', padding: '6px 12px', borderRadius: 6, background: isDark ? 'rgba(99,102,241,0.1)' : 'rgba(99,102,241,0.05)', border: `1px solid ${isDark ? 'rgba(99,102,241,0.3)' : 'rgba(99,102,241,0.2)'}`, marginBottom: 8 }}>
                      开启后，所有插上的 USB 设备会自动重定向到远程桌面。新插入的 U 盘、鼠标、键盘等都会立即出现在远程会话中。
                    </div>
                  )}
                  {draft.rdp_usbMode === 'manual' && (
                    <>
                      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 8 }}>
                        <div style={{ fontSize: 13, fontWeight: 500, color: isDark ? '#ccc' : '#555' }}>
                          选择要重定向的 USB 设备 {!selectedUsbDevices.size ? '（未选择）' : `（已选 ${selectedUsbDevices.size} 个）`}
                        </div>
                        <Space size={6}>
                          <Button size="small" style={{ fontSize: 12 }} disabled={usbDeviceList.length === 0}
                            onClick={() => {
                              const allVidPids = usbDeviceList.map(d => `${d.vid}:${d.pid}`)
                              setSelectedUsbDevices(new Set(allVidPids))
                              updateField('rdp_usbDevices', allVidPids.join(','))
                            }}>全选</Button>
                          <Button size="small" style={{ fontSize: 12 }} disabled={selectedUsbDevices.size === 0}
                            onClick={() => { setSelectedUsbDevices(new Set()); updateField('rdp_usbDevices', '') }}>清空</Button>
                          <Button size="small" loading={usbLoading} onClick={loadUSBDevices} icon={<ReloadOutlined />} style={{ fontSize: 12 }}>刷新列表</Button>
                        </Space>
                      </div>
                      {usbDeviceList.length > 0 ? (
                        <div style={{ maxHeight: 200, overflowY: 'auto', borderRadius: 6, border: `1px solid ${isDark ? '#333' : '#e0e0e0'}` }}>
                          {usbDeviceList.map((dev, idx) => {
                            const vidPid = `${dev.vid}:${dev.pid}`
                            const checked = selectedUsbDevices.has(vidPid)
                            return (
                              <div key={vidPid + idx} style={{
                                display: 'flex', alignItems: 'center', justifyContent: 'space-between',
                                padding: '6px 12px', cursor: 'pointer',
                                borderBottom: idx < usbDeviceList.length - 1 ? `1px solid ${isDark ? '#222' : '#f0f0f0'}` : 'none',
                                background: checked ? (isDark ? 'rgba(99,102,241,0.15)' : 'rgba(99,102,241,0.08)') : 'transparent',
                                transition: 'background 0.2s',
                              }} onClick={() => toggleUsbDevice(vidPid)}>
                                <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                                  <div style={{
                                    width: 16, height: 16, borderRadius: 3, border: `2px solid ${checked ? '#6366f1' : (isDark ? '#555' : '#ccc')}`,
                                    background: checked ? '#6366f1' : 'transparent',
                                    display: 'flex', alignItems: 'center', justifyContent: 'center', flexShrink: 0,
                                  }}>
                                    {checked && <span style={{ color: 'white', fontSize: 10, lineHeight: 1 }}>✓</span>}
                                  </div>
                                  <span style={{ fontSize: 12, color: isDark ? '#ddd' : '#333' }}>{dev.name}</span>
                                </div>
                                <Tag style={{ fontSize: 10, margin: 0 }}>{dev.vid}:{dev.pid}</Tag>
                              </div>
                            )
                          })}
                        </div>
                      ) : (
                        <div style={{ fontSize: 12, color: isDark ? '#666' : '#999', padding: '8px 0' }}>点击"刷新列表"加载本地 USB 设备</div>
                      )}
                    </>
                  )}
                </div>
              )}
              <div style={formRowStyle}>
                <div style={formLabelStyle}><div style={labelText}>数据压缩</div><div style={{ fontSize: 12, ...descText }}>启用传输压缩以节省带宽</div></div>
                <Switch checked={draft.rdp_compression} onChange={v => updateField('rdp_compression', v)} />
              </div>
              <div style={formRowStyle}>
                <div style={formLabelStyle}><div style={labelText}>隐藏悬浮菜单</div><div style={{ fontSize: 12, ...descText }}>全屏连接后隐藏顶部悬浮工具栏（默认显示，可用它退出全屏/断开）</div></div>
                <Switch checked={draft.rdp_hideToolbar} onChange={v => updateField('rdp_hideToolbar', v)} />
              </div>
              <div style={formRowStyle}>
                <div style={formLabelStyle}><div style={labelText}>多显示器（双屏）</div><div style={{ fontSize: 12, ...descText }}>每块屏幕作为独立显示通道，窗口可跨屏拖动；服务端需专业版/企业版/服务器版 Windows（家庭版不支持），此模式下自动使用 sdl-freerdp 客户端</div></div>
                <Switch checked={!!draft.rdp_multiMon} onChange={v => updateField('rdp_multiMon', v)} />
              </div>
              <div style={{ ...formRowStyle, borderBottom: 'none' }}>
                <div style={formLabelStyle}><div style={labelText}>屏蔽 ALT+F4 关闭</div><div style={{ fontSize: 12, ...descText }}>禁止远程会话中按 ALT+F4 退出（移除窗口系统菜单+本地 Alt）</div></div>
                <Switch checked={!!draft.rdp_blockAltF4} onChange={v => updateField('rdp_blockAltF4', v)} />
              </div>
            </div>
          </Card>

          {/* Windows RDP (mstsc + RDP 文件) 参数 */}
          <Card className="param-card" variant="borderless" style={{ background: cardBg, border: `1px solid ${cardBorder}`, borderRadius: 16 }}>
            <div className="param-card-title" style={{ borderBottom: `1px solid ${cardBorder}` }}>
              <span style={{ fontSize: 22, marginRight: 8 }}>{getProtocolIcon('windowsrdp')}</span>
              <span style={labelText}>Windows RDP</span>
              <Tag color="blue" style={{ marginLeft: 8 }}>官方 mstsc</Tag>
              <Tag color="geekblue" style={{ marginLeft: 6 }}>生成 RDP 文件</Tag>
            </div>
            <div style={{ padding: '4px 4px' }}>
              <div style={formRowStyle}>
                <div style={formLabelStyle}><div style={labelText}>分辨率</div><div style={{ fontSize: 12, ...descText }}>窗口模式下远程桌面大小</div></div>
                <Select value={draft.windowsrdp_resolution} onChange={v => updateField('windowsrdp_resolution', v)} style={{ width: 160 }}
                  options={[
                    { value: '1024x768', label: '1024 × 768' }, { value: '1280x720', label: '1280 × 720' }, { value: '1280x1024', label: '1280 × 1024' },
                    { value: '1366x768', label: '1366 × 768' }, { value: '1440x900', label: '1440 × 900' }, { value: '1600x900', label: '1600 × 900' },
                    { value: '1680x1050', label: '1680 × 1050' }, { value: '1920x1080', label: '1920 × 1080 (FHD)' },
                    { value: '2560x1440', label: '2560 × 1440 (2K)' }, { value: '3840x2160', label: '3840 × 2160 (4K)' },
                  ]} />
              </div>
              <div style={formRowStyle}>
                <div style={formLabelStyle}><div style={labelText}>颜色深度</div><div style={{ fontSize: 12, ...descText }}>每像素位数 (bpp)</div></div>
                <Select value={draft.windowsrdp_colorDepth} onChange={v => updateField('windowsrdp_colorDepth', v)} style={{ width: 160 }}
                  options={[{ value: 8, label: '8 位' }, { value: 15, label: '15 位' }, { value: 16, label: '16 位' }, { value: 24, label: '24 位' }, { value: 32, label: '32 位' }]} />
              </div>
              <div style={formRowStyle}>
                <div style={formLabelStyle}><div style={labelText}>全屏模式</div><div style={{ fontSize: 12, ...descText }}>连接后直接进入全屏</div></div>
                <Switch checked={draft.windowsrdp_fullscreen} onChange={v => updateField('windowsrdp_fullscreen', v)} />
              </div>
              <div style={formRowStyle}>
                <div style={formLabelStyle}><div style={labelText}>多显示器支持</div><div style={{ fontSize: 12, ...descText }}>使用所有连接的显示器</div></div>
                <Switch checked={draft.windowsrdp_multiMonitor} onChange={v => updateField('windowsrdp_multiMonitor', v)} />
              </div>
              <div style={formRowStyle}>
                <div style={formLabelStyle}><div style={labelText}>顶部连接栏</div><div style={{ fontSize: 12, ...descText }}>全屏时显示顶部连接信息栏</div></div>
                <Switch checked={draft.windowsrdp_showConnectionBar} onChange={v => updateField('windowsrdp_showConnectionBar', v)} />
              </div>
              <div style={formRowStyle}>
                <div style={formLabelStyle}><div style={labelText}>音频重定向</div><div style={{ fontSize: 12, ...descText }}>在本地播放远程音频</div></div>
                <Switch checked={draft.windowsrdp_audio} onChange={v => updateField('windowsrdp_audio', v)} />
              </div>
              <div style={formRowStyle}>
                <div style={formLabelStyle}><div style={labelText}>剪贴板共享</div><div style={{ fontSize: 12, ...descText }}>允许本地与远程双向复制粘贴</div></div>
                <Switch checked={draft.windowsrdp_clipboard} onChange={v => updateField('windowsrdp_clipboard', v)} />
              </div>
              <div style={formRowStyle}>
                <div style={formLabelStyle}><div style={labelText}>驱动器映射</div><div style={{ fontSize: 12, ...descText }}>将本地磁盘共享到远程会话</div></div>
                <Switch checked={draft.windowsrdp_driveMapping} onChange={v => updateField('windowsrdp_driveMapping', v)} />
              </div>
              <div style={formRowStyle}>
                <div style={formLabelStyle}><div style={labelText}>打印机重定向</div><div style={{ fontSize: 12, ...descText }}>使用本地打印机打印远程文档</div></div>
                <Switch checked={draft.windowsrdp_printer} onChange={v => updateField('windowsrdp_printer', v)} />
              </div>
              <div style={formRowStyle}>
                <div style={formLabelStyle}><div style={labelText}>智能卡重定向</div><div style={{ fontSize: 12, ...descText }}>将本地智能卡/U盾 转发到远程</div></div>
                <Switch checked={draft.windowsrdp_smartcard} onChange={v => updateField('windowsrdp_smartcard', v)} />
              </div>
              <div style={formRowStyle}>
                <div style={formLabelStyle}><div style={labelText}>数据压缩</div><div style={{ fontSize: 12, ...descText }}>启用 RDP 协议压缩以节省带宽</div></div>
                <Switch checked={draft.windowsrdp_compression} onChange={v => updateField('windowsrdp_compression', v)} />
              </div>
              <div style={{ ...formRowStyle, borderBottom: 'none' }}>
                <div style={formLabelStyle}><div style={labelText}>自动注入凭据</div><div style={{ fontSize: 12, ...descText }}>一键链接前用 cmdkey 预存用户名密码（免二次输入）</div></div>
                <Switch checked={draft.windowsrdp_autoLogin} onChange={v => updateField('windowsrdp_autoLogin', v)} />
              </div>
            </div>
          </Card>

          {/* SPICE 参数 */}
          <Card className="param-card" variant="borderless" style={{ background: cardBg, border: `1px solid ${cardBorder}`, borderRadius: 16 }}>
            <div className="param-card-title" style={{ borderBottom: `1px solid ${cardBorder}` }}>
              <span style={{ fontSize: 22, marginRight: 8 }}>{getProtocolIcon('spice')}</span>
              <span style={labelText}>SPICE</span>
              <Tag color="purple" style={{ marginLeft: 8 }}>虚拟化桌面</Tag>
            </div>
            <div style={{ padding: '4px 4px' }}>
              <div style={formRowStyle}>
                <div style={formLabelStyle}><div style={labelText}>TLS 加密</div><div style={{ fontSize: 12, ...descText }}>使用 TLS 加密通信</div></div>
                <Switch checked={draft.spice_tls} onChange={v => updateField('spice_tls', v)} />
              </div>
              <div style={formRowStyle}>
                <div style={formLabelStyle}><div style={labelText}>图像压缩</div><div style={{ fontSize: 12, ...descText }}>压缩图像数据以节省带宽</div></div>
                <Switch checked={draft.spice_compression} onChange={v => updateField('spice_compression', v)} />
              </div>
              <div style={formRowStyle}>
                <div style={formLabelStyle}><div style={labelText}>全屏模式</div><div style={{ fontSize: 12, ...descText }}>启动时进入全屏</div></div>
                <Switch checked={draft.spice_fullscreen} onChange={v => updateField('spice_fullscreen', v)} />
              </div>
              <div style={formRowStyle}>
                <div style={formLabelStyle}><div style={labelText}>智能卡重定向</div><div style={{ fontSize: 12, ...descText }}>将本地智能卡转发到虚拟机</div></div>
                <Switch checked={draft.spice_smartcard} onChange={v => updateField('spice_smartcard', v)} />
              </div>
              <div style={formRowStyle}>
                <div style={formLabelStyle}><div style={labelText}>USB 重定向</div><div style={{ fontSize: 12, ...descText }}>将本地 USB 设备转发到虚拟机</div></div>
                <Switch checked={draft.spice_usb} onChange={v => updateField('spice_usb', v)} />
              </div>
              <div style={formRowStyle}>
                <div style={formLabelStyle}><div style={labelText}>隐藏悬浮菜单</div><div style={{ fontSize: 12, ...descText }}>全屏时隐藏顶部工具栏（隐藏后在虚拟机内关机/注销，客户端自动退出返回）</div></div>
                <Switch checked={draft.spice_hideToolbar} onChange={v => updateField('spice_hideToolbar', v)} />
              </div>
              <div style={{ ...formRowStyle, borderBottom: 'none' }}>
                <div style={formLabelStyle}><div style={labelText}>剪贴板共享</div><div style={{ fontSize: 12, ...descText }}>允许双向剪贴板共享</div></div>
                <Switch checked={draft.spice_shareClipboard} onChange={v => updateField('spice_shareClipboard', v)} />
              </div>
            </div>
          </Card>

          {/* VNC 参数 */}
          <Card className="param-card" variant="borderless" style={{ background: cardBg, border: `1px solid ${cardBorder}`, borderRadius: 16 }}>
            <div className="param-card-title" style={{ borderBottom: `1px solid ${cardBorder}` }}>
              <span style={{ fontSize: 22, marginRight: 8 }}>{getProtocolIcon('vnc')}</span>
              <span style={labelText}>VNC</span>
              <Tag color="orange" style={{ marginLeft: 8 }}>远程帧缓冲</Tag>
            </div>
            <div style={{ padding: '4px 4px' }}>
              <div style={formRowStyle}>
                <div style={formLabelStyle}><div style={labelText}>色彩级别</div><div style={{ fontSize: 12, ...descText }}>像素格式</div></div>
                <Select value={draft.vnc_colorLevel} onChange={v => updateField('vnc_colorLevel', v)} style={{ width: 160 }}
                  options={[{ value: 'rgb111', label: 'rgb111 (8色)' }, { value: 'rgb222', label: 'rgb222 (64色)' }, { value: 'rgb332', label: 'rgb332 (256色)' }, { value: 'low', label: '低彩色' }, { value: 'medium', label: '中彩色' }, { value: 'full', label: '全彩色' }]} />
              </div>
              <div style={formRowStyle}>
                <div style={formLabelStyle}><div style={labelText}>压缩级别</div><div style={{ fontSize: 12, ...descText }}>0-9，数值越大压缩越高</div></div>
                <InputNumber min={0} max={9} value={draft.vnc_compression} onChange={v => updateField('vnc_compression', Number(v || 0))} style={{ width: 160 }} />
              </div>
              <div style={formRowStyle}>
                <div style={formLabelStyle}><div style={labelText}>编码方式</div><div style={{ fontSize: 12, ...descText }}>图像编码算法</div></div>
                <Select value={draft.vnc_encodings} onChange={v => updateField('vnc_encodings', v)} style={{ width: 160 }}
                  options={[{ value: 'tight', label: 'Tight' }, { value: 'zrle', label: 'ZRLE' }, { value: 'hextile', label: 'Hextile' }, { value: 'raw', label: 'Raw' }, { value: 'copyrect', label: 'CopyRect' }]} />
              </div>
              <div style={formRowStyle}>
                <div style={formLabelStyle}><div style={labelText}>全屏模式</div><div style={{ fontSize: 12, ...descText }}>启动时进入全屏</div></div>
                <Switch checked={draft.vnc_fullscreen} onChange={v => updateField('vnc_fullscreen', v)} />
              </div>
              <div style={formRowStyle}>
                <div style={formLabelStyle}><div style={labelText}>只读模式</div><div style={{ fontSize: 12, ...descText }}>仅查看远程桌面，禁止输入</div></div>
                <Switch checked={draft.vnc_viewOnly} onChange={v => updateField('vnc_viewOnly', v)} />
              </div>
              <div style={{ ...formRowStyle, borderBottom: 'none' }}>
                <div style={formLabelStyle}><div style={labelText}>缩放显示</div><div style={{ fontSize: 12, ...descText }}>自动缩放以适应窗口</div></div>
                <Switch checked={draft.vnc_scaling} onChange={v => updateField('vnc_scaling', v)} />
              </div>
            </div>
          </Card>

          {/* Horizon 参数 */}
          <Card className="param-card" variant="borderless" style={{ background: cardBg, border: `1px solid ${cardBorder}`, borderRadius: 16 }}>
            <div className="param-card-title" style={{ borderBottom: `1px solid ${cardBorder}` }}>
              <span style={{ fontSize: 22, marginRight: 8 }}>{getProtocolIcon('horizon')}</span>
              <span style={labelText}>Horizon</span>
              <Tag color="green" style={{ marginLeft: 8 }}>云桌面</Tag>
            </div>
            <div style={{ padding: '4px 4px' }}>
              <div style={formRowStyle}>
                <div style={formLabelStyle}><div style={labelText}>显示协议</div><div style={{ fontSize: 12, ...descText }}>Blast 性能更优，PCoIP 兼容性好</div></div>
                <Select value={draft.horizon_protocol} onChange={v => updateField('horizon_protocol', v)} style={{ width: 160 }}
                  options={[{ value: 'blast', label: 'Blast Extreme' }, { value: 'pcoip', label: 'PCoIP' }, { value: 'rdp', label: 'RDP' }]} />
              </div>
              <div style={formRowStyle}>
                <div style={formLabelStyle}><div style={labelText}>全屏模式</div><div style={{ fontSize: 12, ...descText }}>启动时进入全屏</div></div>
                <Switch checked={draft.horizon_fullscreen} onChange={v => updateField('horizon_fullscreen', v)} />
              </div>
              <div style={formRowStyle}>
                <div style={formLabelStyle}><div style={labelText}>隐藏悬浮菜单</div><div style={{ fontSize: 12, ...descText }}>进入远程桌面后隐藏客户端控制栏</div></div>
                <Switch checked={draft.horizon_hideClient} onChange={v => updateField('horizon_hideClient', v)} />
              </div>
              <div style={formRowStyle}>
                <div style={formLabelStyle}><div style={labelText}>自动重连</div><div style={{ fontSize: 12, ...descText }}>网络中断后自动重连</div></div>
                <Switch checked={draft.horizon_autoReconnect} onChange={v => updateField('horizon_autoReconnect', v)} />
              </div>
              <div style={{ ...formRowStyle, borderBottom: 'none' }}>
                <div style={formLabelStyle}><div style={labelText}>USB 自动连接</div><div style={{ fontSize: 12, ...descText }}>自动将 USB 设备连接到远程</div></div>
                <Switch checked={draft.horizon_usbAutoConnect} onChange={v => updateField('horizon_usbAutoConnect', v)} />
              </div>
            </div>
          </Card>
        </div>
      )}
    </>
  )
}


// ============ 账户安全面板 ============
function AccountSecurityPanel({ isDark, isLocalMode, localUsername, onChangePassword }: {
  isDark: boolean; isLocalMode: boolean; localUsername: string;
  onChangePassword?: (oldPwd: string, newPwd: string) => Promise<void>;
}) {
  // 登录用户密码
  const [oldPwd, setOldPwd] = useState('')
  const [newPwd, setNewPwd] = useState('')
  const [confirmPwd, setConfirmPwd] = useState('')
  const [pwdChanging, setPwdChanging] = useState(false)
  // 管理员密码
  const [adminOldPwd, setAdminOldPwd] = useState('')
  const [adminNewPwd, setAdminNewPwd] = useState('')
  const [adminConfirmPwd, setAdminConfirmPwd] = useState('')
  const [adminPwdChanging, setAdminPwdChanging] = useState(false)
  const [msgApi, msgCtxHolder] = message.useMessage()
  const lowToast = (text: string, type: 'success' | 'error' | 'info' = 'success') => {
    const cfg = { content: text, duration: type === 'error' ? 2.5 : 1.5, style: { fontSize: 12, padding: '2px 8px', borderRadius: 6, boxShadow: 'none' } }
    if (type === 'success') msgApi.success(cfg)
    else if (type === 'error') msgApi.error(cfg)
    else msgApi.info(cfg)
  }

  const handleChangePasswordSubmit = async () => {
    if (!onChangePassword) return
    if (!oldPwd || !newPwd || !confirmPwd) { message.error('请填写完整的密码信息'); return }
    if (newPwd !== confirmPwd) { message.error('两次输入的新密码不一致'); return }
    if (newPwd.length < 4) { message.error('新密码长度至少 4 位'); return }
    try {
      setPwdChanging(true)
      await onChangePassword(oldPwd, newPwd)
      setOldPwd(''); setNewPwd(''); setConfirmPwd('')
    } catch { } finally { setPwdChanging(false) }
  }

  const cardBg = isDark ? '#121633' : '#ffffff'
  const cardBorder = isDark ? '#1e2350' : '#e2e8f0'
  const labelText = { color: isDark ? '#e2e8f0' : '#0f172a' }
  const descText = { color: isDark ? '#64748b' : '#64748b' }
  const formRowStyle: React.CSSProperties = { display: 'flex', alignItems: 'center', justifyContent: 'space-between', padding: '12px 0', borderBottom: `1px solid ${cardBorder}`, gap: 16 }
  const formLabelStyle: React.CSSProperties = { flex: 1, minWidth: 0 }

  return (
    <>
      {msgCtxHolder}
      {isLocalMode ? (
        <div className="protocol-params-grid">
          {/* 卡片 1：登录用户密码设置 */}
          <Card className="param-card" variant="borderless" style={{ background: cardBg, border: `1px solid ${cardBorder}`, borderRadius: 16, maxWidth: 620 }}>
            <div className="param-card-title" style={{ borderBottom: `1px solid ${cardBorder}` }}>
              <span style={{ fontSize: 20, marginRight: 8 }}><UserOutlined style={{ color: '#6366f1' }} /></span>
              <span style={labelText}>登录用户密码设置</span>
              <Tag color="orange" style={{ marginLeft: 8 }}>当前账号：{localUsername || 'admin'}</Tag>
            </div>
            <div style={{ padding: '20px 4px' }}>
              <div style={formRowStyle}>
                <div style={formLabelStyle}>
                  <div style={labelText}>原密码</div>
                  <div style={{ fontSize: 12, ...descText }}>请输入当前登录账号密码（默认 admin）</div>
                </div>
                <Input.Password value={oldPwd} onChange={(e) => setOldPwd(e.target.value)} placeholder="请输入原密码" style={{ width: 260, borderRadius: 8 }} />
              </div>
              <div style={formRowStyle}>
                <div style={formLabelStyle}>
                  <div style={labelText}>新密码</div>
                  <div style={{ fontSize: 12, ...descText }}>长度至少 4 位</div>
                </div>
                <Input.Password value={newPwd} onChange={(e) => setNewPwd(e.target.value)} placeholder="请输入新密码" style={{ width: 260, borderRadius: 8 }} />
              </div>
              <div style={{ ...formRowStyle, borderBottom: 'none' }}>
                <div style={formLabelStyle}>
                  <div style={labelText}>确认新密码</div>
                  <div style={{ fontSize: 12, ...descText }}>请再次输入新密码</div>
                </div>
                <Input.Password value={confirmPwd} onChange={(e) => setConfirmPwd(e.target.value)} placeholder="请再次输入新密码" style={{ width: 260, borderRadius: 8 }} />
              </div>
              <div style={{ marginTop: 24, display: 'flex', justifyContent: 'flex-end' }}>
                <Button type="primary" icon={<SafetyCertificateOutlined />} loading={pwdChanging} onClick={handleChangePasswordSubmit}
                  style={{ background: 'linear-gradient(135deg, #6366f1, #8b5cf6)', border: 'none', borderRadius: 10, height: 42, paddingLeft: 28, paddingRight: 28, fontWeight: 600 }}>确认修改密码</Button>
              </div>
            </div>
          </Card>

          {/* 卡片 2：管理员密码设置 */}
          <Card className="param-card" variant="borderless" style={{ background: cardBg, border: `1px solid ${cardBorder}`, borderRadius: 16, maxWidth: 620 }}>
            <div className="param-card-title" style={{ borderBottom: `1px solid ${cardBorder}` }}>
              <span style={{ fontSize: 20, marginRight: 8 }}><SafetyCertificateOutlined style={{ color: '#ef4444' }} /></span>
              <span style={labelText}>管理员密码设置</span>
              <Tag color="red" style={{ marginLeft: 8 }}>用于进入管理设置</Tag>
            </div>
            <div style={{ padding: '20px 4px' }}>
              <div style={formRowStyle}>
                <div style={formLabelStyle}>
                  <div style={labelText}>原管理员密码</div>
                  <div style={{ fontSize: 12, ...descText }}>请输入当前管理员密码（默认 123110）</div>
                </div>
                <Input.Password value={adminOldPwd} onChange={(e) => setAdminOldPwd(e.target.value)} placeholder="请输入原管理员密码" style={{ width: 260, borderRadius: 8 }} />
              </div>
              <div style={formRowStyle}>
                <div style={formLabelStyle}>
                  <div style={labelText}>新管理员密码</div>
                  <div style={{ fontSize: 12, ...descText }}>长度至少 4 位</div>
                </div>
                <Input.Password value={adminNewPwd} onChange={(e) => setAdminNewPwd(e.target.value)} placeholder="请输入新管理员密码" style={{ width: 260, borderRadius: 8 }} />
              </div>
              <div style={{ ...formRowStyle, borderBottom: 'none' }}>
                <div style={formLabelStyle}>
                  <div style={labelText}>确认新密码</div>
                  <div style={{ fontSize: 12, ...descText }}>请再次输入新管理员密码</div>
                </div>
                <Input.Password value={adminConfirmPwd} onChange={(e) => setAdminConfirmPwd(e.target.value)} placeholder="请再次输入新管理员密码" style={{ width: 260, borderRadius: 8 }} />
              </div>
              <div style={{ marginTop: 24, display: 'flex', justifyContent: 'flex-end' }}>
                <Button type="primary" danger icon={<SafetyCertificateOutlined />} loading={adminPwdChanging} onClick={async () => {
                  if (!adminOldPwd || !adminNewPwd || !adminConfirmPwd) { lowToast('请填写完整', 'error'); return }
                  const stored = localStorage.getItem('admin_password') || '123110'
                  if (adminOldPwd !== stored) { lowToast('原管理员密码错误', 'error'); return }
                  if (adminNewPwd.length < 4) { lowToast('新密码长度至少 4 位', 'error'); return }
                  if (adminNewPwd !== adminConfirmPwd) { lowToast('两次输入的新密码不一致', 'error'); return }
                  setAdminPwdChanging(true)
                  try {
                    localStorage.setItem('admin_password', adminNewPwd)
                    lowToast('管理员密码修改成功', 'success')
                    setAdminOldPwd(''); setAdminNewPwd(''); setAdminConfirmPwd('')
                  } catch (e) {
                    lowToast('保存失败，请重试', 'error')
                  } finally {
                    setAdminPwdChanging(false)
                  }
                }}
                  style={{ borderRadius: 10, height: 42, paddingLeft: 28, paddingRight: 28, fontWeight: 600 }}>确认修改管理员密码</Button>
              </div>
            </div>
          </Card>
        </div>
      ) : (
        <div style={{ textAlign: 'center', padding: 60, color: descText.color }}>
          <SafetyCertificateOutlined style={{ fontSize: 48, color: isDark ? '#374151' : '#d1d5db', marginBottom: 16 }} />
          <p>账户安全设置仅在本地模式下可用</p>
          <p style={{ fontSize: 13 }}>请切换到本地模式后管理账户密码</p>
        </div>
      )}
    </>
  )
}


// ============ 依赖检测面板 ============
function DependencyCheckPanel({ isDark, dependencies, onRecheck, getProtocolIcon, getProtocolName }: {
  isDark: boolean;
  dependencies: Record<string, { available: boolean; message: string }>;
  onRecheck: () => void;
  getProtocolIcon: (p: string) => string;
  getProtocolName: (p: string) => string;
}) {
  const protocols = ['rdp', 'windowsrdp', 'spice', 'vnc', 'horizon']
  const cardBorder = isDark ? '#1e2350' : '#e2e8f0'

  return (
    <>
      {/* 依赖检测：重新检测按钮放在内容上方 */}
      <div style={{ display: 'flex', justifyContent: 'flex-end', marginBottom: 16 }}>
        <Button type="primary" icon={<ReloadOutlined />} onClick={onRecheck}
          style={{ background: 'linear-gradient(135deg, #6366f1, #8b5cf6)', border: 'none', borderRadius: 10, height: 38, paddingLeft: 20, paddingRight: 20, fontWeight: 600 }}>重新检测</Button>
      </div>

      <div className="dependencies-grid">
        {protocols.map(proto => {
          const dep = dependencies[proto]
          const available = dep?.available ?? false
          return (
            <Card key={proto} className="dependency-card-new" variant="borderless"
              style={{
                background: isDark ? (available ? 'linear-gradient(145deg, rgba(16,185,129,0.08), rgba(18,22,51,0.8))' : 'linear-gradient(145deg, rgba(239,68,68,0.08), rgba(18,22,51,0.8))') : (available ? 'linear-gradient(145deg, rgba(16,185,129,0.05), #ffffff)' : 'linear-gradient(145deg, rgba(239,68,68,0.05), #ffffff)'),
                border: `1px solid ${cardBorder}`, borderRadius: 16,
              }}>
              <div className="dep-card-header">
                <div className="dep-icon" style={{ background: `${getProtocolColor(proto)}22` }}>{getProtocolIcon(proto)}</div>
                <div className="dep-info">
                  <div className="dep-name" style={{ color: isDark ? '#e2e8f0' : '#0f172a' }}>{getProtocolName(proto)}</div>
                </div>
                <Progress type="circle" size={48} percent={available ? 100 : 0} strokeColor={available ? '#10b981' : '#ef4444'} trailColor={isDark ? '#1e2350' : '#e2e8f0'} format={() => available ? '✓' : '✗'} />
              </div>
            </Card>
          )
        })}
      </div>
    </>
  )
}


function VMEditorModal({ open, editingVM, isDark, onClose, onSave }: {
  open: boolean; editingVM: VMInfo | null; isDark: boolean; onClose: () => void; onSave: (vm: VMInfo) => void;
}) {
  const defaultVM: VMInfo = {
    vmid: 0, name: '', status: 'stopped', node: '本地', cpu_cores: 2, memory_mb: 4096, disk_gb: 60,
    os: 'Windows 10', protocol: 'rdp', ip_address: '', vm_username: '', vm_password: '',
    power_backend: 'none',
    pve_endpoint: '', pve_username: '', pve_password: '', pve_node: '', pve_vmid: 0, pve_verify_ssl: false,
    hyperv_host: '.', hyperv_vmname: '', hyperv_use_winrm: false, hyperv_username: '', hyperv_password: '', hyperv_force: false,
  }
  const [form, setForm] = useState<VMInfo>(editingVM || defaultVM)
  useEffect(() => {
    // 向后兼容：老的 SPICE+PVE 配置（没填 power_backend）→ 自动补成 'pve'
    if (editingVM) {
      const pb = editingVM.power_backend
      if (!pb && editingVM.protocol === 'spice' && editingVM.pve_endpoint) {
        setForm({ ...editingVM, power_backend: 'pve' })
      } else {
        setForm(editingVM || defaultVM)
      }
    } else {
      setForm(defaultVM)
    }
  }, [editingVM, open])
  const update = <K extends keyof VMInfo>(key: K, value: VMInfo[K]) => { setForm(prev => ({ ...prev, [key]: value })) }

  // ===== PVE 虚拟机查询弹窗 =====
  const [pveVmListOpen, setPveVmListOpen] = useState(false)
  const [pveVmListLoading, setPveVmListLoading] = useState(false)
  const [pveVmList, setPveVmList] = useState<PveVMInfo[]>([])

  const handleQueryPveVMs = async () => {
    if (!form.pve_endpoint?.trim()) { message.warning('请先填写 PVE 服务器地址（例如 https://192.168.11.222:8006）'); return }
    if (!form.pve_username?.trim()) { message.warning('请先填写 PVE 用户名'); return }
    if (!form.pve_password?.trim()) { message.warning('请先填写 PVE 密码'); return }
    setPveVmListLoading(true)
    setPveVmList([])
    setPveVmListOpen(true)
    try {
      const list = await protocolService.ListVMsViaPVE(form.pve_endpoint, form.pve_username, form.pve_password, !!form.pve_verify_ssl)
      if (!list || list.length === 0) {
        message.warning('该账号下未查询到任何虚拟机，请确认 PVE ACL 授权是否正确')
        return
      }
      setPveVmList(list)
    } catch (e: any) {
      message.error('查询 PVE 虚拟机失败：' + (e?.message || String(e)))
      setPveVmListOpen(false)
    } finally {
      setPveVmListLoading(false)
    }
  }

  const handlePickPveVM = (vm: PveVMInfo) => {
    // 自动填入 节点名 / VMID / 虚拟机名称；同时尝试给「IP 地址」字段填入 PVE 地址（作为 placeholder）
    update('pve_node', vm.node)
    update('pve_vmid', vm.vmID)
    if (!form.name?.trim() && vm.name) {
      update('name', vm.name)
    }
    // SPICE 协议不需要 IP（通过 PVE API 获取），其他协议用户还需要补 VM 自己的 IP
    setPveVmListOpen(false)
    message.success(`已选择：${vm.name}（VMID=${vm.vmID}，节点=${vm.node}）`)
  }

  const pveVMColumns = [
    { title: 'VMID', dataIndex: 'vmID', key: 'vmID', width: 70, render: (v: number) => <b style={{ color: '#6366f1' }}>{v}</b> },
    {
      title: '状态', dataIndex: 'status', key: 'status', width: 90,
      render: (v: string) => {
        const color = v === 'running' ? 'green' : v === 'stopped' ? 'default' : v === 'suspended' ? 'gold' : 'default'
        const txt = v === 'running' ? '运行中' : v === 'stopped' ? '已停止' : v === 'suspended' ? '挂起' : v
        return <Tag color={color as any} style={{ margin: 0 }}>{txt}</Tag>
      }
    },
    { title: '虚拟机名称', dataIndex: 'name', key: 'name', render: (v: string) => <span style={{ fontWeight: 500 }}>{v}</span> },
    { title: '类型', dataIndex: 'type', key: 'type', width: 70, render: (v: string) => v === 'qemu' ? 'VM' : 'LXC' },
    { title: '所在节点', dataIndex: 'node', key: 'node', width: 100 },
    {
      title: '操作', key: 'action', width: 90,
      render: (_: any, record: PveVMInfo) => (
        <Button type="link" onClick={() => handlePickPveVM(record)} size="small" style={{ padding: 0 }}>选择</Button>
      )
    },
  ]

  const handleSave = () => {
    if (!form.name.trim()) { message.error('请输入虚拟机名称'); return }
    // IP 地址：SPICE 协议通过 PVE 动态获取，不需要；其余协议需要 IP
    if (form.protocol !== 'spice' && !form.ip_address.trim()) {
      message.error('请输入 IP 地址'); return
    }
    // 电源后端校验
    const pb = form.power_backend
    if (pb === 'pve') {
      if (!form.pve_endpoint?.trim()) { message.error('请输入 PVE 服务器地址'); return }
      if (!form.pve_username?.trim()) { message.error('请输入 PVE 用户名'); return }
      if (!form.pve_password?.trim()) { message.error('请输入 PVE 密码'); return }
      if (!form.pve_node?.trim()) { message.error('请输入 PVE 节点名'); return }
      if (!form.pve_vmid || form.pve_vmid <= 0) { message.error('请输入 PVE 虚拟机 ID'); return }
    } else if (pb === 'hyperv') {
      if (!form.hyperv_vmname?.trim()) { message.error('请输入 Hyper-V 虚拟机名称'); return }
      if (!!form.hyperv_use_winrm && !form.hyperv_host?.trim()) {
        message.error('启用 WinRM 远程时请填写 Hyper-V 主机地址'); return
      }
    }
    onSave(form)
  }

  const inputStyle: React.CSSProperties = { borderRadius: 8, height: 42, fontSize: 14 }
  const labelStyle: React.CSSProperties = { display: 'block', marginBottom: 8, fontSize: 14, fontWeight: 500, color: isDark ? '#94a3b8' : '#475569' }
  const pb = form.power_backend || 'none'

  return (
    <Modal open={open} onCancel={onClose} footer={null} width={660} centered destroyOnClose
      title={<div style={{ display: 'flex', alignItems: 'center', gap: 8, fontSize: 17 }}>{editingVM ? <><EditOutlined style={{ color: '#6366f1', fontSize: 18 }} /> 编辑虚拟机</> : <><PlusOutlined style={{ color: '#6366f1', fontSize: 18 }} /> 添加虚拟机</>}</div>}>
      <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '18px 24px', marginTop: 20, marginBottom: 8 }}>
        <div style={{ gridColumn: '1 / 3' }}>
          <label style={labelStyle}>虚拟机名称 *</label>
          <Input value={form.name} onChange={e => update('name', e.target.value)} placeholder="例如：我的 Windows 桌面" style={inputStyle} />
        </div>
        {form.protocol !== 'spice' && (
          <div style={{ gridColumn: '1 / 3' }}>
            <label style={labelStyle}>IP 地址 *</label>
            <Input value={form.ip_address} onChange={e => update('ip_address', e.target.value)} placeholder="例如：192.168.1.100" style={inputStyle} />
          </div>
        )}
        {/* ---- 远程登录凭据（所有非 SPICE 协议显示） ---- */}
        {form.protocol !== 'spice' && (
          <>
            <div style={{ gridColumn: '1 / 3', borderTop: `1px solid ${isDark ? '#1e2350' : '#e2e8f0'}`, margin: '6px 0', paddingTop: 18 }}>
              <label style={{ ...labelStyle, fontSize: 15, fontWeight: 600, color: isDark ? '#e2e8f0' : '#0f172a', marginBottom: 0 }}>
                <SafetyCertificateOutlined style={{ marginRight: 6, color: '#6366f1' }} />远程登录凭据（自动登录时使用）
              </label>
            </div>
            <div>
              <label style={labelStyle}>用户名</label>
              <Input value={form.vm_username || ''} onChange={e => update('vm_username', e.target.value)} placeholder="例如：administrator" style={inputStyle} prefix={<UserOutlined style={{ color: isDark ? '#64748b' : '#94a3b8' }} />} />
            </div>
            <div>
              <label style={labelStyle}>密码</label>
              <Input.Password value={form.vm_password || ''} onChange={e => update('vm_password', e.target.value)} placeholder="远程登录密码" style={inputStyle} />
            </div>
          </>
        )}
        <div>
          <label style={labelStyle}>连接协议</label>
          <Select value={form.protocol} onChange={v => update('protocol', v)} style={{ width: '100%', height: 42 }}
            options={[
              { value: 'rdp', label: '🖥️ FreeRDP' },
              { value: 'windowsrdp', label: '🪟 Windows RDP' },
              { value: 'spice', label: '🌶️ SPICE' },
              { value: 'vnc', label: '📺 VNC' },
              { value: 'horizon', label: '🌀 Horizon' },
            ]} />
        </div>
        <div>
          <label style={labelStyle}>操作系统</label>
          <div style={{ display: 'flex', gap: 10, alignItems: 'center' }}>
            <Select value={form.os} onChange={v => update('os', v)} style={{ width: '100%', height: 42 }}
              options={[
                { value: 'Windows 11', label: '🪟 Windows 11' },
                { value: 'Windows 10', label: '🪟 Windows 10' },
                { value: 'Windows 7', label: '🪟 Windows 7' },
                { value: 'Windows XP', label: '🪟 Windows XP' },
                { value: 'Windows Server 2025', label: '🖥️ Windows Server 2025' },
                { value: 'Windows Server 2022', label: '🖥️ Windows Server 2022' },
                { value: 'Windows Server 2019', label: '🖥️ Windows Server 2019' },
                { value: 'Windows Server 2016', label: '🖥️ Windows Server 2016' },
                { value: 'Windows Server 2012', label: '🖥️ Windows Server 2012' },
                { value: 'Windows Server 2008', label: '🖥️ Windows Server 2008' },
                { value: 'Ubuntu 22.04', label: '🟧 Ubuntu 22.04' },
                { value: 'Ubuntu 24.04', label: '🟧 Ubuntu 24.04' },
                { value: 'CentOS 7', label: '🟪 CentOS 7' },
                { value: 'CentOS Stream 9', label: '🟪 CentOS Stream 9' },
                { value: 'Debian 12', label: '🟥 Debian 12' },
                { value: 'Red Hat Enterprise Linux 9', label: '🟥 Red Hat Enterprise Linux 9' },
                { value: 'Fedora 40', label: '🟦 Fedora 40' },
                { value: 'macOS', label: '🍎 macOS' },
                { value: '银河麒麟 Kylin', label: '🔴 银河麒麟 Kylin' },
                { value: '统信 UOS', label: '🔵 统信 UOS' },
                { value: 'Android', label: '🤖 Android' },
                { value: 'Linux', label: '🐧 Linux' },
              ]} />
            {/* 实时图标预览 */}
            {(() => {
              const osInfo = getOSInfo(form.os)
              return (
                <div style={{
                  width: 44, height: 44, borderRadius: 10, flexShrink: 0,
                  background: osInfo.iconBg,
                  display: 'flex', alignItems: 'center', justifyContent: 'center',
                  boxShadow: `0 2px 8px ${osInfo.iconBg}55`,
                }}>
                  <img src={osInfo.smallIcon} alt="os-preview" style={{ width: 26, height: 26, filter: 'brightness(0) invert(1)' }} />
                </div>
              )
            })()}
          </div>
        </div>

        {/* ============= 新增：电源控制后端（与协议解耦）============= */}
        <div style={{ gridColumn: '1 / 3', borderTop: `1px solid ${isDark ? '#1e2350' : '#e2e8f0'}`, margin: '6px 0', paddingTop: 18 }}>
          <label style={{ ...labelStyle, fontSize: 15, fontWeight: 600, color: isDark ? '#e2e8f0' : '#0f172a', marginBottom: 10 }}>
            <PoweroffOutlined style={{ marginRight: 6, color: '#ef4444' }} />电源控制后端
            <span style={{ fontSize: 12, fontWeight: 400, color: isDark ? '#64748b' : '#64748b', marginLeft: 10 }}>
              独立于连接协议选择：决定开关机按钮用哪种 API 控制虚拟机电源
            </span>
          </label>
          <Select value={pb} onChange={v => update('power_backend', v)} style={{ width: '100%', height: 42 }}
            options={[
              { value: 'none', label: '🚫 无（仅本地模拟，不真实控电源）' },
              { value: 'pve', label: '🟧 Proxmox VE (PVE API) — 启动/停止/重启' },
              { value: 'hyperv', label: '🪟 Hyper-V (PowerShell Cmdlet) — 本机 / WinRM 远程' },
              { value: 'horizon', label: '🌀 Horizon 专用（通过 Horizon 管理控制台操作）' },
            ]} />
        </div>

        {/* ---- PVE 后端字段 ---- */}
        {pb === 'pve' && (
          <>
            <div style={{ gridColumn: '1 / 3' }}>
              <label style={labelStyle}>PVE 服务器地址 *</label>
              <Input value={form.pve_endpoint || ''} onChange={e => update('pve_endpoint', e.target.value)} placeholder="例如：https://192.168.10.10:8006" style={inputStyle} prefix={<CloudServerOutlined style={{ color: isDark ? '#64748b' : '#94a3b8' }} />} />
            </div>
            <div>
              <label style={labelStyle}>PVE 用户名 *</label>
              <Input value={form.pve_username || ''} onChange={e => update('pve_username', e.target.value)} placeholder="例如：1000（无需加@pve，自动补全）或 root@pam" style={inputStyle} prefix={<UserOutlined style={{ color: isDark ? '#64748b' : '#94a3b8' }} />} />
            </div>
            <div>
              <label style={labelStyle}>PVE 密码 *</label>
              <Input.Password value={form.pve_password || ''} onChange={e => update('pve_password', e.target.value)} placeholder="PVE 登录密码" style={inputStyle} />
            </div>
            <div>
              <label style={labelStyle}>PVE 节点名 *</label>
              <Input value={form.pve_node || ''} onChange={e => update('pve_node', e.target.value)} placeholder="例如：pve 或 node1" style={inputStyle} />
            </div>
            <div>
              <label style={labelStyle}>虚拟机 ID (VMID) *</label>
              <Space.Compact style={{ width: '100%' }}>
                <Input type="number" value={form.pve_vmid || ''} onChange={e => update('pve_vmid', parseInt(e.target.value) || 0)} placeholder="例如：100" style={inputStyle} />
                <Button
                  type="default"
                  icon={<SearchOutlined />}
                  onClick={() => handleQueryPveVMs()}
                  style={{ height: inputStyle.height, borderLeft: '1px solid ' + (isDark ? 'rgba(255,255,255,0.08)' : '#e5e7eb'), paddingLeft: 14, paddingRight: 14 }}
                >
                  查询 PVE 虚拟机
                </Button>
              </Space.Compact>
            </div>
            <div style={{ gridColumn: '1 / 3' }}>
              <label style={{ ...labelStyle, display: 'flex', alignItems: 'center', gap: 8, cursor: 'pointer' }}>
                <input type="checkbox" checked={!!form.pve_verify_ssl} onChange={e => update('pve_verify_ssl', e.target.checked)} style={{ width: 16, height: 16 }} />
                验证 SSL 证书（自签名证书请取消勾选）
              </label>
            </div>
          </>
        )}

        {/* ---- Hyper-V 后端字段 ---- */}
        {pb === 'hyperv' && (
          <>
            <div>
              <label style={labelStyle}>Hyper-V 主机</label>
              <Input value={form.hyperv_host || '.'} onChange={e => update('hyperv_host', e.target.value)} placeholder='. = 本机，或远程主机名' style={inputStyle} prefix={<CloudServerOutlined style={{ color: isDark ? '#64748b' : '#94a3b8' }} />} />
            </div>
            <div>
              <label style={labelStyle}>Hyper-V 虚拟机名称 *</label>
              <Input value={form.hyperv_vmname || ''} onChange={e => update('hyperv_vmname', e.target.value)} placeholder="例如：Win10-Office" style={inputStyle} prefix={<DesktopOutlined style={{ color: isDark ? '#64748b' : '#94a3b8' }} />} />
            </div>
            <div style={{ gridColumn: '1 / 3' }}>
              <label style={{ ...labelStyle, display: 'flex', alignItems: 'center', gap: 8, cursor: 'pointer', marginBottom: 0 }}>
                <input type="checkbox" checked={!!form.hyperv_use_winrm} onChange={e => update('hyperv_use_winrm', e.target.checked)} style={{ width: 16, height: 16 }} />
                使用 WinRM 远程连接 Hyper-V（本机直连请取消勾选）
              </label>
            </div>
            {!!form.hyperv_use_winrm && (
              <>
                <div>
                  <label style={labelStyle}>WinRM 账号</label>
                  <Input value={form.hyperv_username || ''} onChange={e => update('hyperv_username', e.target.value)} placeholder="例如：administrator" style={inputStyle} prefix={<UserOutlined style={{ color: isDark ? '#64748b' : '#94a3b8' }} />} />
                </div>
                <div>
                  <label style={labelStyle}>WinRM 密码</label>
                  <Input.Password value={form.hyperv_password || ''} onChange={e => update('hyperv_password', e.target.value)} placeholder="远程账号密码" style={inputStyle} />
                </div>
              </>
            )}
            <div style={{ gridColumn: '1 / 3' }}>
              <label style={{ ...labelStyle, display: 'flex', alignItems: 'center', gap: 8, cursor: 'pointer', marginBottom: 0 }}>
                <input type="checkbox" checked={!!form.hyperv_force} onChange={e => update('hyperv_force', e.target.checked)} style={{ width: 16, height: 16 }} />
                强制断电式关机（Stop-VM -Force），关闭=软关（调用集成服务安全关机）
              </label>
            </div>
          </>
        )}

        {/* ---- Horizon 后端提示 ---- */}
        {pb === 'horizon' && (
          <div style={{ gridColumn: '1 / 3', padding: '14px 16px', borderRadius: 10, background: isDark ? 'rgba(99,102,241,0.08)' : '#eef2ff', border: `1px solid ${isDark ? 'rgba(99,102,241,0.25)' : '#c7d2fe'}` }}>
            <div style={{ fontSize: 14, fontWeight: 600, color: isDark ? '#a5b4fc' : '#4f46e5', marginBottom: 4 }}>
              🌀 Horizon 虚拟机
            </div>
            <div style={{ fontSize: 12, color: isDark ? '#94a3b8' : '#6366f1', lineHeight: 1.8 }}>
              Horizon 云桌面的开机/关机由 Horizon 连接服务器 (Connection Server) 统一管理，瘦客户端不直接控制底层虚拟化平台（vSphere / vCenter / Hyper-V）。<br />
              本应用中的卡片开关机按钮将提示用户通过 Horizon 管理控制台 (Horizon Console / Admin UI) 操作对应桌面池。
            </div>
          </div>
        )}

      </div>
      <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 12, marginTop: 28, marginBottom: 8 }}>
        <Button onClick={onClose} style={{ borderRadius: 10, height: 44, paddingLeft: 24, paddingRight: 24, fontSize: 14 }}>取消</Button>
        <Button type="primary" onClick={handleSave} style={{ background: 'linear-gradient(135deg, #6366f1, #8b5cf6)', border: 'none', borderRadius: 10, height: 44, paddingLeft: 32, paddingRight: 32, fontWeight: 600, fontSize: 15 }}>
          {editingVM ? '保存修改' : '添加虚拟机'}
        </Button>
      </div>

      {/* ===== PVE 虚拟机查询结果弹窗 ===== */}
      <Modal
        open={pveVmListOpen}
        onCancel={() => setPveVmListOpen(false)}
        footer={null}
        width={760}
        centered
        destroyOnClose
        title={<div style={{ display: 'flex', alignItems: 'center', gap: 8 }}><SearchOutlined style={{ color: '#6366f1' }} />PVE 虚拟机列表（账号：{form.pve_username}）</div>}>
        <div style={{ marginTop: 8 }}>
          <div style={{ fontSize: 12, color: isDark ? '#94a3b8' : '#64748b', marginBottom: 12 }}>
            💡 以下是该账号在 PVE 上被授权访问的所有虚拟机，点击任意一行（或右侧「选择」按钮）即可自动填入节点名 / VMID / 虚拟机名称。
          </div>
          <Table<PveVMInfo>
            rowKey={r => `${r.node}-${r.vmID}`}
            columns={pveVMColumns}
            dataSource={pveVmList}
            loading={pveVmListLoading}
            size="middle"
            locale={{ emptyText: pveVmListLoading ? '正在从 PVE 拉取...' : '该账号下没有虚拟机' }}
            pagination={{ pageSize: 10, showSizeChanger: false }}
            onRow={record => ({
              onClick: () => handlePickPveVM(record),
              style: { cursor: 'pointer' },
            })}
          />
        </div>
      </Modal>
    </Modal>
  )
}

export default App
