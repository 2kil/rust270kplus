# 性能优化与验证（2026-10-08）

环境：Windows 11 专业版 10.0.26300（x64）、Intel Core Ultra 7 270K Plus、Go 1.25.6、Wails 2.10.1。

## 基准结果

同一台机器对修改前后各运行三次，以下为中位数。时间为每次操作的耗时，分配为 Go benchmark 的 B/op 和 allocs/op。

| 场景 | 修改前 | 修改后 | 变化 |
| --- | --- | --- | --- |
| 首个实例匹配：1,000 个同名进程 | 17,883 ns，89,760 B，11 次分配 | 5,581 ns，48 B，1 次分配 | 耗时减少约 69%，分配字节减少超过 99.9% |
| 缓存扫描：100 条规则、1,000 个进程 | 441,933 ns，44,720 B，228 次分配 | 101,314 ns，39,920 B，128 次分配 | 耗时减少约 77%，分配次数减少约 44% |

缓存扫描使用内存进程源，成功结果已经缓存，用于独立测量规则匹配、快照和缓存维护开销，不包含 Windows 枚举、实际应用、日志写盘和界面事件成本。

真实 Windows 枚举新增 Full、NoMatch、OneName、OnePath 四种基准。在优化后的同轮测量中，完整列表约 72.8 KB/次；单个名称筛选约 8.5 KB/次，减少约 88%。枚举耗时受系统负载、进程数量及 Toolhelp 快照影响较大：完整列表约 16.0–17.5 ms，单个名称筛选约 14.9–16.0 ms。修改前完整枚举为 13.1–14.4 ms，说明跨轮系统波动明显，不能据此声称整个应用 CPU 使用率或端到端扫描耗时降低 77%。OneName/OnePath 使用 explorer.exe；其他环境可以修改基准中的目标名称。

## 实现

- 首个实例使用线性最小值选择，保留启动时间优先、PID 次优先和未知启动时间的原有行为。
- 监控使用可复用的名称索引及匹配桶，配置更新时重建，仅为启用的规则保留进程信息。Unicode 名称索引保持 `strings.EqualFold` 的简单折叠语义。
- Windows 后台扫描仅为匹配启用规则名称的进程打开查询句柄，路径仅在同名的启用规则需要路径匹配时查询；界面完整列表继续提供原有元数据。
- 保留 PID/启动时间缓存、未知启动时间不缓存、五秒失败重试、退出进程清理及同名多规则独立应用。
- 修复桌面冒烟测试的线程及 COM 初始化：测试 goroutine 固定到系统线程，在该线程初始化并释放 STA COM，避免 WebView2 控制器创建超时。

## 已通过的验证

在 golang 目录执行：

```powershell
go test ./...
go test -race ./...
go vet ./...
go test ./internal/manager -run '^$' -bench 'Benchmark' -benchmem -count 3
pnpm --dir frontend build
go test -tags "desktop,production,desktop_smoke" -run TestDesktopSmoke -timeout 60s -v
wails build
```

真实 Windows API 测试在独立子进程上验证优先级、亲和性和过期启动时间保护；筛选测试验证当前测试进程、按需路径查询及禁用规则。新增测试覆盖名称 Unicode 折叠、首个实例边界、索引与原线性匹配等价、进程退出和配置更新后索引重建。race 检查无报错。

桌面冒烟测试使用临时配置和 WebView2 数据目录，自动监控关闭，验证调用桥接、规则保存删除、CPU 全选、页面布局、100 条实时及持久日志、托盘和退出，修复后约 1.7 秒通过。

仓库根目录的 `cargo test --manifest-path src-tauri/Cargo.toml`（11 个 Rust 测试）和 `npm run build` 也通过。

构建输出为 `build/bin/process-affinity-manager-go.exe`。测试没有修改现有用户规则、开机启动注册表或其他应用的进程设置。管理员权限、manifest、Tauri capabilities 和配置格式均未修改。
