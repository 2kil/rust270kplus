# 进程亲和性管理器

Windows 10/11 下运行的 Rust + Tauri 进程性能管理工具。按进程名自动设置 CPU 亲和性和进程优先级，支持多个实例、后台监控、系统托盘和 TOML 配置。

## 构建环境

- Windows 10/11 x64
- Rust stable（MSVC 工具链）
- Node.js 22+ 和 npm
- Microsoft WebView2 Runtime

Node.js/npm 只用于构建前端，发布后的 exe 不需要 Node.js。

## 构建

```powershell
npm install
npm run build
cargo test --manifest-path src-tauri/Cargo.toml
npm run tauri:build
```

生成文件：

```text
target/release/process-affinity-manager.exe
target/release/bundle/nsis/进程亲和性管理器_0.2.0_x64-setup.exe
```

Release exe 会嵌入 Windows `requireAdministrator` manifest，启动时自动显示 UAC，并使用 Windows GUI 子系统，不显示控制台黑框。Debug 和测试构建不嵌入该 manifest，便于运行 `cargo test`。

## 使用

1. 启动程序并确认 UAC 提示。
2. 点击“添加进程”，输入例如 `mihomo.exe`，或从当前运行进程列表选择。
3. 选择匹配方式、CPU 亲和性和进程优先级。
4. 点击“保存规则”。程序会自动监控并在目标进程启动后应用设置。
5. “立即扫描”和“应用全部设置”可手动触发应用。

## 配置与日志

配置文件：

```text
%APPDATA%\ProcessAffinityManager\config\config.toml
```

日志文件：

```text
%APPDATA%\ProcessAffinityManager\logs\app.log
```

日志最多保留最近 1000 条内存记录，文件日志持续追加。

## CPU 亲和性

- **全部 CPU**：使用当前 Processor Group 的所有逻辑处理器。
- **最后一个 CPU**：使用编号最大的逻辑处理器，例如 8 个 CPU 时为 CPU 7。
- **指定 CPU**：按 CPU 编号构造 mask，程序会先校验编号范围。
- **不修改**：跳过 Affinity，只设置优先级。

当前版本支持单 Processor Group、最多 64 个逻辑 CPU。检测到多组或超过 64 个逻辑 CPU 时，程序不会构造错误 mask，并提示 Affinity 限制；优先级设置仍可用。

## 权限

Release 程序每次启动都会申请管理员权限，这是为了能够管理以管理员权限运行的目标进程。普通权限启动的目标也可以正常管理。系统服务或受保护进程仍可能拒绝访问，日志会显示 Windows 错误码。

“开机启动”使用当前用户的 `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`。由于主程序要求管理员权限，开机启动时可能显示 UAC。

## 优先级警告

“实时”优先级可能导致系统其他任务无法及时获得 CPU，不建议普通程序使用。

## 常见问题

### 显示“无法打开进程”

确认使用的是 `target/release/process-affinity-manager.exe` 或安装包版本，并允许 UAC。若目标是系统服务或受保护进程，即使管理员权限也可能无法修改。

### 进程重启后设置是否仍然有效

是。程序按进程名重新扫描，缓存同时包含 PID、进程名和启动时间，可以处理 PID 复用。

### 是否需要复制整个 release 文件夹

直接运行时只需 exe。使用安装包时运行 NSIS 安装程序即可。
