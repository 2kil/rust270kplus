#[cfg(windows)]
pub fn set_enabled(enabled: bool) -> Result<(), String> { use std::{env, os::windows::ffi::OsStrExt}; use windows::{Win32::{Foundation::WIN32_ERROR, System::Registry::*}}; let sub=windows::core::w!("Software\\Microsoft\\Windows\\CurrentVersion\\Run"); unsafe { let mut key=HKEY::default(); let status=RegOpenKeyExW(HKEY_CURRENT_USER, sub, 0, KEY_SET_VALUE, &mut key); if status != WIN32_ERROR(0) { return Err(format!("RegOpenKeyExW: {status:?}")); } let name=windows::core::w!("ProcessAffinityManager"); if enabled { let exe=env::current_exe().map_err(|e|e.to_string())?; let wide:Vec<u16>=exe.as_os_str().encode_wide().chain(std::iter::once(0)).collect(); let status=RegSetValueExW(key,name,0,REG_SZ,Some(std::slice::from_raw_parts(wide.as_ptr() as *const u8,wide.len()*2))); if status != WIN32_ERROR(0) { let _=RegCloseKey(key); return Err(format!("RegSetValueExW: {status:?}")); } } else { let _=RegDeleteValueW(key,name); } let _=RegCloseKey(key); } Ok(()) }
#[cfg(not(windows))] pub fn set_enabled(_enabled: bool) -> Result<(), String> { Err("仅支持 Windows".into()) }

#[cfg(windows)]
pub fn relaunch_as_admin() -> Result<(), String> {
    use std::{env, os::windows::ffi::OsStrExt};
    use windows::{core::PCWSTR, Win32::{Foundation::HWND, UI::{Shell::ShellExecuteW, WindowsAndMessaging::SW_SHOWNORMAL}}};
    let exe = env::current_exe().map_err(|e| e.to_string())?;
    let wide: Vec<u16> = exe.as_os_str().encode_wide().chain(std::iter::once(0)).collect();
    let result = unsafe { ShellExecuteW(HWND(std::ptr::null_mut()), windows::core::w!("runas"), PCWSTR(wide.as_ptr()), PCWSTR::null(), PCWSTR::null(), SW_SHOWNORMAL) };
    if (result.0 as usize) <= 32 { return Err(format!("无法请求管理员权限（ShellExecuteW 返回 {}）", result.0 as isize)); }
    Ok(())
}

#[cfg(not(windows))]
pub fn relaunch_as_admin() -> Result<(), String> { Err("仅支持 Windows".into()) }
