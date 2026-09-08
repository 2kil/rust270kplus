#![cfg_attr(windows, windows_subsystem = "windows")]

mod affinity;
mod config;
mod error;
mod logger;
mod monitor;
mod priority;
mod process;
mod startup;

use config::Config;
use monitor::{Command, Event, RuleSnapshot};
use serde::Serialize;
use std::{fs::OpenOptions, io::Write, sync::{mpsc, Mutex}};
use tauri::{Emitter, Manager, State, WindowEvent};

pub struct AppState {
    config: Mutex<Config>,
    monitor_tx: Mutex<mpsc::Sender<Command>>,
    runtime: Mutex<RuntimeStatus>,
}

#[derive(Debug, Clone, Serialize)]
pub struct RuntimeStatus {
    pub enabled: bool,
    pub paused: bool,
    pub logical_cpus: usize,
    pub processor_group_supported: bool,
    pub permission_denied: bool,
}

#[tauri::command]
fn get_config(state: State<'_, AppState>) -> Result<Config, String> { state.config.lock().map(|config| config.clone()).map_err(|e| e.to_string()) }

#[tauri::command]
fn save_config(config: Config, state: State<'_, AppState>) -> Result<(), String> {
    let cpus = state.runtime.lock().map_err(|e| e.to_string())?.logical_cpus;
    let errors = config::validate(&config, cpus);
    if !errors.is_empty() { return Err(errors.join("\n")); }
    config::save(&config).map_err(|e| e.to_string())?;
    *state.config.lock().map_err(|e| e.to_string())? = config.clone();
    state.monitor_tx.lock().map_err(|e| e.to_string())?.send(Command::SetConfig(config)).map_err(|e| e.to_string())?;
    Ok(())
}

#[tauri::command]
fn list_running_processes() -> Vec<process::ProcessInfo> { process::list_processes() }

#[tauri::command]
fn get_rule_snapshots(state: State<'_, AppState>) -> Result<Vec<RuleSnapshot>, String> {
    let config = state.config.lock().map_err(|e| e.to_string())?.clone();
    let processes = process::list_processes();
    Ok(config.processes.iter().filter(|rule| rule.enabled).map(|rule| {
        let matches = process::matching_from(&processes, rule);
        let pids = matches.iter().map(|item| item.pid).collect::<Vec<_>>();
        RuleSnapshot { name: rule.name.clone(), running: !pids.is_empty(), pids }
    }).collect())
}

#[tauri::command]
fn scan_now(state: State<'_, AppState>) -> Result<(), String> { state.monitor_tx.lock().map_err(|e| e.to_string())?.send(Command::Scan).map_err(|e| e.to_string()) }

#[tauri::command]
fn apply_all(state: State<'_, AppState>) -> Result<(), String> { scan_now(state) }

#[tauri::command]
fn set_monitor_paused(paused: bool, state: State<'_, AppState>) -> Result<(), String> {
    let command = if paused { Command::Pause } else { Command::Resume };
    state.monitor_tx.lock().map_err(|e| e.to_string())?.send(command).map_err(|e| e.to_string())?;
    state.runtime.lock().map_err(|e| e.to_string())?.paused = paused;
    Ok(())
}

#[tauri::command]
fn set_start_with_windows(enabled: bool, state: State<'_, AppState>) -> Result<(), String> {
    startup::set_enabled(enabled)?;
    let mut config = state.config.lock().map_err(|e| e.to_string())?;
    config.general.start_with_windows = enabled;
    config::save(&config).map_err(|e| e.to_string())
}

#[tauri::command]
fn get_runtime_status(state: State<'_, AppState>) -> Result<RuntimeStatus, String> { state.runtime.lock().map(|status| status.clone()).map_err(|e| e.to_string()) }

#[tauri::command]
fn get_logs() -> Vec<String> {
    std::fs::read_to_string(config::log_path())
        .map(|text| text.lines().map(|line| if has_timestamp(line) { line.to_owned() } else { timestamp_line(line) }).collect::<Vec<_>>())
        .unwrap_or_default()
        .into_iter()
        .rev()
        .take(1000)
        .collect::<Vec<_>>()
        .into_iter()
        .rev()
        .collect()
}

#[tauri::command]
fn clear_logs() -> Result<(), String> {
    let path = config::log_path();
    if path.exists() { std::fs::write(path, "").map_err(|error| error.to_string())?; }
    Ok(())
}

fn has_timestamp(line: &str) -> bool {
    let bytes = line.as_bytes();
    bytes.len() >= 10 && bytes[0] == b'[' && bytes[9] == b']'
        && bytes[1..9].iter().enumerate().all(|(index, byte)| if index == 2 || index == 5 { *byte == b':' } else { byte.is_ascii_digit() })
}

#[cfg(windows)]
fn current_time_text() -> String {
    #[repr(C)]
    struct SystemTime { year: u16, month: u16, day_of_week: u16, day: u16, hour: u16, minute: u16, second: u16, milliseconds: u16 }
    #[link(name = "kernel32")]
    unsafe extern "system" { fn GetLocalTime(system_time: *mut SystemTime); }
    let mut time = SystemTime { year: 0, month: 0, day_of_week: 0, day: 0, hour: 0, minute: 0, second: 0, milliseconds: 0 };
    unsafe { GetLocalTime(&mut time); }
    format!("{:02}:{:02}:{:02}", time.hour, time.minute, time.second)
}

#[cfg(not(windows))]
fn current_time_text() -> String { "00:00:00".into() }

fn timestamp_line(message: &str) -> String { format!("[{}] {}", current_time_text(), message) }

#[tauri::command]
fn delete_process_rule(index: usize, state: State<'_, AppState>) -> Result<(), String> {
    let mut config = state.config.lock().map_err(|e| e.to_string())?;
    if index >= config.processes.len() { return Err("进程规则不存在".into()); }
    config.processes.remove(index);
    config::save(&config).map_err(|e| e.to_string())?;
    state.monitor_tx.lock().map_err(|e| e.to_string())?.send(Command::SetConfig(config.clone())).map_err(|e| e.to_string())
}

fn append_log(message: &str) {
    let path = config::log_path();
    if let Some(parent) = path.parent() { let _ = std::fs::create_dir_all(parent); }
    if let Ok(mut file) = OpenOptions::new().create(true).append(true).open(path) { let _ = writeln!(file, "{}", timestamp_line(message)); }
}

fn create_tray(app: &mut tauri::App) -> tauri::Result<()> {
    use tauri::menu::{Menu, MenuItem};
    use tauri::tray::{TrayIconBuilder, TrayIconEvent};
    let show = MenuItem::with_id(app, "show", "显示主界面", true, None::<&str>)?;
    let apply = MenuItem::with_id(app, "apply", "立即应用", true, None::<&str>)?;
    let pause = MenuItem::with_id(app, "pause", "暂停监控", true, None::<&str>)?;
    let resume = MenuItem::with_id(app, "resume", "恢复监控", true, None::<&str>)?;
    let startup_item = MenuItem::with_id(app, "startup", "切换开机启动", true, None::<&str>)?;
    let quit = MenuItem::with_id(app, "quit", "退出", true, None::<&str>)?;
    let menu = Menu::with_items(app, &[&show, &apply, &pause, &resume, &startup_item, &quit])?;
    let icon = tauri::image::Image::from_bytes(include_bytes!("../icons/icon.ico"))?;
    TrayIconBuilder::new().icon(icon).menu(&menu).tooltip("进程亲和性管理器").on_tray_icon_event(|app, event| {
        if matches!(event, TrayIconEvent::DoubleClick { .. }) {
            if let Some(window) = app.app_handle().get_webview_window("main") { let _ = window.show(); let _ = window.set_focus(); }
        }
    }).on_menu_event(|app, event| {
        match event.id().as_ref() {
            "show" => { if let Some(window) = app.get_webview_window("main") { let _ = window.show(); let _ = window.set_focus(); } },
            "apply" => { if let Some(state) = app.try_state::<AppState>() { let _ = state.monitor_tx.lock().ok().and_then(|tx| tx.send(Command::Scan).ok()); } },
            "pause" => { if let Some(state) = app.try_state::<AppState>() { let _ = state.monitor_tx.lock().ok().and_then(|tx| tx.send(Command::Pause).ok()); } },
            "resume" => { if let Some(state) = app.try_state::<AppState>() { let _ = state.monitor_tx.lock().ok().and_then(|tx| tx.send(Command::Resume).ok()); } },
            "startup" => { if let Some(state) = app.try_state::<AppState>() { if let Ok(mut config) = state.config.lock() { let enabled = !config.general.start_with_windows; if startup::set_enabled(enabled).is_ok() { config.general.start_with_windows = enabled; let _ = config::save(&config); } } } },
            "quit" => { if let Some(state) = app.try_state::<AppState>() { let _ = state.monitor_tx.lock().ok().and_then(|tx| tx.send(Command::Stop).ok()); } app.exit(0); },
            _ => {}
        }
    }).build(app)?;
    Ok(())
}

pub fn run() {
    let _ = logger::init();
    if let Err(error) = process::enable_debug_privilege() { append_log(&format!("无法启用调试权限：{}", error)); }
    let config = config::load().unwrap_or_default();
    let logical_cpus = std::thread::available_parallelism().map(|n| n.get()).unwrap_or(1);
    let (command_tx, command_rx) = mpsc::channel();
    let (event_tx, event_rx) = mpsc::channel();
    monitor::spawn(config.clone(), logical_cpus, command_rx, event_tx);
    let state = AppState { config: Mutex::new(config.clone()), monitor_tx: Mutex::new(command_tx), runtime: Mutex::new(RuntimeStatus { enabled: config.general.enabled, paused: false, logical_cpus, processor_group_supported: logical_cpus <= 64, permission_denied: false }) };
    let builder = tauri::Builder::default().manage(state).invoke_handler(tauri::generate_handler![get_config, save_config, list_running_processes, get_rule_snapshots, get_logs, clear_logs, scan_now, apply_all, set_monitor_paused, set_start_with_windows, get_runtime_status, delete_process_rule]);
    builder.setup(move |app| {
        create_tray(app)?;
        if let Some(window) = app.get_webview_window("main") {
            if let Some(state) = app.try_state::<AppState>() {
                if let Ok(config) = state.config.lock() {
                    let (width, height) = config::clamp_window_size(config.general.window_width, config.general.window_height);
                    let _ = window.set_size(tauri::Size::Physical(tauri::PhysicalSize::new(width, height)));
                }
            }
            let window_handle = window.clone();
            let app_handle = app.handle().clone();
            window.on_window_event(move |event| {
                match event {
                    WindowEvent::Resized(size) => {
                        let (width, height) = config::clamp_window_size(size.width, size.height);
                        if let Some(state) = app_handle.try_state::<AppState>() {
                            if let Ok(mut config) = state.config.lock() {
                                if config.general.window_width != width || config.general.window_height != height {
                                    config.general.window_width = width;
                                    config.general.window_height = height;
                                    let _ = config::save(&config);
                                }
                            }
                        }
                    }
                    WindowEvent::CloseRequested { api, .. } => {
                        let minimize = app_handle.try_state::<AppState>().and_then(|state| state.config.lock().ok().map(|config| config.general.minimize_to_tray)).unwrap_or(true);
                        if minimize { api.prevent_close(); let _ = window_handle.hide(); }
                    }
                    _ => {}
                }
            });
        }
        let handle = app.handle().clone();
        std::thread::spawn(move || { while let Ok(event) = event_rx.recv() { if let Event::Applied { result, rule_name } = &event { let line = format!("{} PID={} {}", rule_name, result.pid, result.message); append_log(&line); if !result.ok && result.message.contains("错误码 5") { if let Some(state) = handle.try_state::<AppState>() { if let Ok(mut runtime) = state.runtime.lock() { runtime.permission_denied = true; } } } } if let Event::Status { message, paused } = &event { append_log(message); if let Some(state) = handle.try_state::<AppState>() { if let Ok(mut runtime) = state.runtime.lock() { runtime.paused = *paused; } } } let _ = handle.emit("monitor-event", event); } });
        Ok(())
    }).run(tauri::generate_context!()).expect("Tauri 运行失败");
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn log_timestamp_is_recognized() {
        assert!(has_timestamp("[12:34:56] 应用成功"));
        assert!(!has_timestamp("应用成功"));
        assert!(!has_timestamp("[1:34:56] 应用成功"));
    }

    #[test]
    fn timestamp_line_has_expected_shape() {
        let line = timestamp_line("应用成功");
        assert!(has_timestamp(&line));
        assert!(line.ends_with(" 应用成功"));
    }
}
