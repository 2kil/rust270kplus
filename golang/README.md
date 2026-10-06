# 进程亲和性管理器（Go + Wails）

Windows 10/11 x64 桌面版本。后端使用 Go 与 Wails 2，前端使用 TypeScript、Vite 和 Pico CSS，由 Node.js 与 pnpm 构建。发布后的 exe 不需要 Node.js、pnpm 或 Go，运行时需要 WebView2。

## 环境与开发

需要 Node.js 22+、pnpm 11、Go 1.25+、Wails 2.10.1 和 Windows WebView2。

```powershell
cd D:\GitSum\rust270kplus\golang
go install github.com/wailsapp/wails/v2/cmd/wails@v2.10.1
pnpm --dir frontend install --frozen-lockfile
wails dev
```

Wails 会启动 Go 后端和 Vite，并自动生成 `frontend/wailsjs/` 调用绑定。前端开发服务使用 `127.0.0.1:1421`；运行 `wails dev` 后，可通过其浏览器开发地址检查界面。单独启动 `pnpm --dir frontend dev` 不包含 Go 调用桥接。

## 验证与构建

```powershell
pnpm --dir frontend build
go test ./...
go vet ./...
wails build
```

输出文件：`build/bin/process-affinity-manager-go.exe`。构建默认使用 Windows GUI 子系统，嵌入图标、前端资源和 `requireAdministrator` 清单，启动时申请管理员权限。可用 `go test -race ./...` 检查并发；此命令还需要支持 Go race 的 C 编译器。

在真实 Windows 桌面中运行可选的集成检查：

```powershell
go test -tags "desktop,production,desktop_smoke" -run TestDesktopSmoke -timeout 60s
```

此检查使用隐藏窗口、临时配置与 WebView2 数据目录，验证真实调用桥接、规则保存与删除、CPU 全选、布局、100 条日志、托盘初始化和退出；自动监控保持关闭。

## 功能与兼容性

- 按进程名、首个实例或完整路径匹配；支持添加、编辑和删除规则。
- CPU 亲和性支持全部 CPU、最后一个 CPU、指定 CPU 和保持原设置；提供六档进程优先级。
- 后台自动扫描、立即应用、暂停与恢复；使用规则索引、PID 和启动时间缓存，失败后五秒重试，清理已退出进程的缓存。
- 托盘菜单提供显示窗口、立即应用、暂停、恢复、切换开机启动和退出；关闭窗口可隐藏到托盘，保存窗口尺寸。
- 添加页面保留亲和性与优先级的左右布局；运行日志和日志文件保留最近 100 条。

沿用原版配置 `%APPDATA%\ProcessAffinityManager\config\config.toml` 及日志 `%APPDATA%\ProcessAffinityManager\logs\app.log`，可直接读取现有 TOML。配置损坏时显示错误并使用默认值，保存成功后清除错误提示。两个版本共用文件，应避免同时运行。

开机启动使用 `HKCU\Software\Microsoft\Windows\CurrentVersion\Run` 中的 `ProcessAffinityManager`，开启时指向当前 exe，带空格的路径会加引号。多 Processor Group 或超过 64 个逻辑 CPU 时，请选择“不修改”亲和性，仅设置优先级。受保护进程可能拒绝访问，日志记录 Windows 错误码。

## 目录

`main.go` 和 `app.go` 负责桌面生命周期、调用绑定和托盘；`internal/manager/` 负责配置、日志、监控及 Windows API；`frontend/src/` 负责界面和调用适配；`build/windows/` 保存图标、版本资源和权限清单。测试使用临时配置及独立子进程，不修改现有进程规则或开机启动注册表。
