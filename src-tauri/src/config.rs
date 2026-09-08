use crate::{affinity::AffinityMode, error::AppError, priority::Priority};
use directories::ProjectDirs;
use serde::{Deserialize, Serialize};
use std::{fs, path::PathBuf};

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
#[serde(rename_all = "snake_case")]
pub enum MatchMode { AllInstances, FirstInstance, SpecificPath }
impl Default for MatchMode { fn default() -> Self { Self::AllInstances } }

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct ProcessRule {
    pub name: String,
    #[serde(default = "enabled_default")] pub enabled: bool,
    #[serde(default)] pub affinity: AffinityMode,
    #[serde(default)] pub cpus: Vec<usize>,
    #[serde(default)] pub priority: Priority,
    #[serde(default)] pub match_mode: MatchMode,
    #[serde(default)] pub path: Option<String>,
}
fn enabled_default() -> bool { true }

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct General {
    #[serde(default = "enabled_default")] pub enabled: bool,
    #[serde(default = "interval_default")] pub interval_ms: u64,
    #[serde(default)] pub start_with_windows: bool,
    #[serde(default = "enabled_default")] pub minimize_to_tray: bool,
    #[serde(default = "window_width_default")] pub window_width: u32,
    #[serde(default = "window_height_default")] pub window_height: u32,
}
fn interval_default() -> u64 { 1000 }
fn window_width_default() -> u32 { 900 }
fn window_height_default() -> u32 { 650 }
pub const MIN_WINDOW_WIDTH: u32 = 900;
pub const MIN_WINDOW_HEIGHT: u32 = 650;
pub const MAX_WINDOW_WIDTH: u32 = 3840;
pub const MAX_WINDOW_HEIGHT: u32 = 2160;

pub fn clamp_window_size(width: u32, height: u32) -> (u32, u32) {
    (width.clamp(MIN_WINDOW_WIDTH, MAX_WINDOW_WIDTH), height.clamp(MIN_WINDOW_HEIGHT, MAX_WINDOW_HEIGHT))
}

impl Default for General { fn default() -> Self { Self { enabled: true, interval_ms: 1000, start_with_windows: false, minimize_to_tray: true, window_width: window_width_default(), window_height: window_height_default() } } }

#[derive(Debug, Clone, Serialize, Deserialize, Default, PartialEq, Eq)]
pub struct Config { #[serde(default)] pub general: General, #[serde(default)] pub processes: Vec<ProcessRule> }

pub fn config_path() -> PathBuf {
    #[cfg(windows)]
    { if let Ok(app_data) = std::env::var("APPDATA") { return PathBuf::from(app_data).join("ProcessAffinityManager").join("config").join("config.toml"); } }
    ProjectDirs::from("com", "ProcessAffinityManager", "ProcessAffinityManager").map(|p| p.config_dir().join("config.toml")).unwrap_or_else(|| PathBuf::from("config.toml"))
}
pub fn log_path() -> PathBuf { config_path().parent().unwrap_or(std::path::Path::new(".")).join("..").join("logs").join("app.log") }
pub fn load() -> Result<Config, AppError> { let path=config_path(); if !path.exists(){return Ok(Config::default());} let text=fs::read_to_string(path)?; toml::from_str(&text).map_err(|e|AppError::Config(e.to_string())) }
pub fn save(config: &Config) -> Result<(), AppError> { let path=config_path(); if let Some(parent)=path.parent(){fs::create_dir_all(parent)?;} let text=toml::to_string_pretty(config).map_err(|e|AppError::Config(e.to_string()))?; fs::write(path,text)?; Ok(()) }
pub fn validate(config: &Config, cpus: usize) -> Vec<String> { let mut errors=Vec::new(); if ![250,500,1000,2000,5000].contains(&config.general.interval_ms){errors.push("监控间隔必须是 250、500、1000、2000 或 5000 毫秒".into());} for rule in &config.processes { if rule.name.trim().is_empty(){errors.push("进程名不能为空".into());} if matches!(rule.affinity,AffinityMode::CpuList){if rule.cpus.is_empty(){errors.push(format!("{}：CPU 列表不能为空",rule.name));} for &cpu in &rule.cpus {if cpu>=cpus{errors.push(format!("{}：CPU {} 不存在",rule.name,cpu));}}} if matches!(rule.match_mode,MatchMode::SpecificPath)&&rule.path.as_deref().unwrap_or("").trim().is_empty(){errors.push(format!("{}：指定路径不能为空",rule.name));} } errors }

#[cfg(test)] mod tests { use super::*; #[test] fn toml_roundtrip(){let c=Config{general:General::default(),processes:vec![ProcessRule{name:"x.exe".into(),enabled:true,affinity:AffinityMode::LastCpu,cpus:vec![],priority:Priority::Idle,match_mode:MatchMode::AllInstances,path:None}]}; let s=toml::to_string(&c).unwrap(); let d:Config=toml::from_str(&s).unwrap(); assert_eq!(d,c);} #[test] fn legacy_config_uses_window_defaults(){let c:Config=toml::from_str("[general]\nenabled = true\n").unwrap(); assert_eq!(c.general.window_width,900); assert_eq!(c.general.window_height,650);} #[test] fn window_size_is_clamped(){assert_eq!(clamp_window_size(1,1),(MIN_WINDOW_WIDTH,MIN_WINDOW_HEIGHT)); assert_eq!(clamp_window_size(5000,5000),(MAX_WINDOW_WIDTH,MAX_WINDOW_HEIGHT)); assert_eq!(clamp_window_size(900,650),(900,650));} }
