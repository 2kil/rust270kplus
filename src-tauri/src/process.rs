use crate::{affinity::{self, AffinityMode}, config::{MatchMode, ProcessRule}};
use serde::Serialize;

#[derive(Debug, Clone, Serialize)]
pub struct ProcessInfo { pub pid: u32, pub name: String, pub path: Option<String>, pub start_time: u64 }

pub fn list_processes() -> Vec<ProcessInfo> { let mut system=sysinfo::System::new_all(); list_processes_with_system(&mut system) }
pub fn list_processes_with_system(system: &mut sysinfo::System) -> Vec<ProcessInfo> { system.refresh_processes(sysinfo::ProcessesToUpdate::All,true); system.processes().values().map(|p|ProcessInfo{pid:p.pid().as_u32(),name:p.name().to_string_lossy().into_owned(),path:p.exe().map(|x|x.to_string_lossy().into_owned()),start_time:p.start_time()}).collect() }
pub fn matching_from(processes: &[ProcessInfo], rule: &ProcessRule) -> Vec<ProcessInfo> { let mut items:Vec<_>=processes.iter().filter(|p|p.name.eq_ignore_ascii_case(&rule.name)).cloned().collect(); if matches!(rule.match_mode,MatchMode::SpecificPath){if let Some(path)=&rule.path{items.retain(|p|p.path.as_deref().map(|x|x.eq_ignore_ascii_case(path)).unwrap_or(false));}} if matches!(rule.match_mode,MatchMode::FirstInstance){items.sort_by_key(|p|(p.start_time,p.pid));items.truncate(1);} items }

#[cfg(windows)]
pub fn enable_debug_privilege() -> Result<(), String> {
    use windows::Win32::Foundation::{CloseHandle, GetLastError, HANDLE, LUID, ERROR_NOT_ALL_ASSIGNED};
    use windows::Win32::Security::{AdjustTokenPrivileges, LookupPrivilegeValueW, LUID_AND_ATTRIBUTES, SE_DEBUG_NAME, SE_PRIVILEGE_ENABLED, TOKEN_ADJUST_PRIVILEGES, TOKEN_PRIVILEGES, TOKEN_QUERY};
    use windows::Win32::System::Threading::{GetCurrentProcess, OpenProcessToken};

    unsafe {
        let mut token = HANDLE::default();
        OpenProcessToken(GetCurrentProcess(), TOKEN_ADJUST_PRIVILEGES | TOKEN_QUERY, &mut token).map_err(|error| format!("打开当前进程令牌失败：{error}"))?;
        let mut luid = LUID::default();
        if let Err(error) = LookupPrivilegeValueW(None, SE_DEBUG_NAME, &mut luid) {
            let _ = CloseHandle(token);
            return Err(format!("查询 SeDebugPrivilege 失败：{error}"));
        }
        let privileges = TOKEN_PRIVILEGES { PrivilegeCount: 1, Privileges: [LUID_AND_ATTRIBUTES { Luid: luid, Attributes: SE_PRIVILEGE_ENABLED }] };
        if let Err(error) = AdjustTokenPrivileges(token, false, Some(&privileges), 0, None, None) {
            let _ = CloseHandle(token);
            return Err(format!("启用 SeDebugPrivilege 失败：{error}"));
        }
        let not_assigned = GetLastError() == ERROR_NOT_ALL_ASSIGNED;
        let _ = CloseHandle(token);
        if not_assigned { return Err("当前令牌未分配 SeDebugPrivilege".into()); }
        Ok(())
    }
}

#[cfg(not(windows))]
pub fn enable_debug_privilege() -> Result<(), String> { Ok(()) }

#[derive(Debug, Clone, Serialize)]
pub struct ApplyResult { pub pid: u32, pub ok: bool, pub message: String }

#[cfg(windows)]
 pub fn apply(info:&ProcessInfo,rule:&ProcessRule,logical_cpus:usize)->ApplyResult { use windows::Win32::Foundation::CloseHandle; use windows::Win32::System::Threading::{OpenProcess,SetPriorityClass,SetProcessAffinityMask,PROCESS_SET_INFORMATION}; if info.pid == std::process::id() || info.name.eq_ignore_ascii_case("ProcessAffinityManager.exe") || info.name.eq_ignore_ascii_case("process-affinity-manager.exe"){return ApplyResult{pid:info.pid,ok:false,message:"拒绝修改工具自身".into()};} unsafe { let handle=match OpenProcess(PROCESS_SET_INFORMATION,false,info.pid){Ok(handle)=>handle,Err(error)=>{let code=(error.code().0 as u32)&0xffff; return ApplyResult{pid:info.pid,ok:false,message:format!("无法打开进程 {}：{}（错误码 {}）",info.pid,error.message(),code)}}}; let result=(||{if !matches!(rule.affinity,AffinityMode::None){let mask=match rule.affinity{AffinityMode::All=>affinity::all_mask(logical_cpus),AffinityMode::LastCpu=>affinity::last_cpu_mask(logical_cpus),AffinityMode::CpuList=>affinity::cpu_list_mask(&rule.cpus,logical_cpus).ok(),AffinityMode::None=>None}.ok_or_else(||"CPU 亲和性不支持或配置无效".to_string())?; SetProcessAffinityMask(handle,mask as usize).map_err(|e|format!("设置 CPU 亲和性失败：{e}"))?;} SetPriorityClass(handle,rule.priority.class()).map_err(|e|format!("设置进程优先级失败：{e}"))?; Ok::<_,String>(())})(); let _=CloseHandle(handle); match result{Ok(())=>ApplyResult{pid:info.pid,ok:true,message:"应用成功".into()},Err(e)=>ApplyResult{pid:info.pid,ok:false,message:e}} } }
#[cfg(not(windows))] pub fn apply(info:&ProcessInfo,_rule:&ProcessRule,_logical_cpus:usize)->ApplyResult{ApplyResult{pid:info.pid,ok:false,message:"仅支持 Windows API".into()}}
