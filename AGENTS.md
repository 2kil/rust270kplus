# Repository Guidelines

## 项目结构与模块组织

这是一个 Windows 10/11 上运行的 Tauri 2 应用。前端入口位于 `frontend/main.ts`，样式位于 `frontend/styles.css`，根目录的 `index.html` 与 `cpu.svg` 提供页面和图形资源；Vite 将它们构建到 `dist/`。Rust 工作区位于根目录，后端代码集中在 `src-tauri/src/`：`config.rs` 处理配置，`monitor.rs` 负责后台扫描，`process.rs`、`affinity.rs` 和 `priority.rs` 处理进程控制。Tauri 配置、权限、图标和 Windows 清单分别位于 `src-tauri/tauri.conf.json`、`src-tauri/capabilities/`、`src-tauri/icons/` 和 `src-tauri/manifest.xml`。

## 构建、测试与开发命令

在仓库根目录执行：

- `npm install`：安装前端和 Tauri CLI 依赖。
- `npm run dev`：启动 Vite 开发服务器（`127.0.0.1`）。
- `npm run build`：运行 TypeScript 检查并生成前端生产包。
- `npm run tauri:dev`：启动带 Rust 后端的桌面开发程序。
- `npm run tauri:build`：构建 Release 可执行文件（不生成安装包）。
- `cargo test --manifest-path src-tauri/Cargo.toml`：运行 Rust 单元测试。
- `cargo fmt --all -- --check` 与 `cargo clippy --manifest-path src-tauri/Cargo.toml --all-targets`：检查格式和常见 Rust 问题。

开发环境需要 Node.js 22+、Rust stable MSVC、Windows WebView2；涉及进程权限的功能应在 Windows 上验证。

## 编码风格与命名约定

TypeScript 使用严格模式、双引号、分号和 2 空格缩进；类型、接口和枚举使用 PascalCase，函数和变量使用 camelCase，Tauri 命令名使用已有的 snake_case。Rust 代码使用 `rustfmt` 默认格式，类型使用 PascalCase，函数和模块使用 snake_case。CSS 选择器使用小写短横线命名。修改行为时同步更新用户可见文本和错误日志。

## 测试指南

Rust 单元测试与实现文件放在 `#[cfg(test)]` 模块中，测试函数使用行为描述命名，例如 `default_normal`。提交前至少运行 `cargo test --manifest-path src-tauri/Cargo.toml` 和 `npm run build`；新增进程匹配、亲和性或配置逻辑时，应补充对应的边界测试。当前项目没有单独的前端测试框架或覆盖率门槛。

## 提交与 Pull Request 指南

现有提交使用简短、直接的中文主题（如 `初次提交`）。沿用这一风格：主题使用动词或结果描述，控制在一行内，避免把无关重构混入同一提交。PR 描述应说明用户可见变化、Windows 版本和验证命令；涉及界面时附截图，涉及权限、启动项或配置格式时说明兼容性和迁移影响，并链接相关 issue。

## 配置与安全提示

不要提交 `%APPDATA%\ProcessAffinityManager\` 下的配置、日志或构建产物。修改管理员权限、注册表开机启动、Tauri capabilities 或 Windows manifest 前，先说明影响并在真实 Windows 环境测试；不要在日志、示例或提交中写入真实路径、账号或密钥。
