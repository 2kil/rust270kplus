use serde::{Deserialize, Serialize};

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
#[serde(rename_all = "snake_case")]
pub enum AffinityMode {
    All,
    LastCpu,
    CpuList,
    None,
}

impl Default for AffinityMode { fn default() -> Self { Self::All } }

pub fn all_mask(logical_cpus: usize) -> Option<u64> {
    if logical_cpus == 0 || logical_cpus > 64 { return None; }
    Some(if logical_cpus == 64 { u64::MAX } else { (1u64 << logical_cpus) - 1 })
}

pub fn last_cpu_mask(logical_cpus: usize) -> Option<u64> {
    if logical_cpus == 0 || logical_cpus > 64 { return None; }
    Some(1u64 << (logical_cpus - 1))
}

pub fn cpu_list_mask(cpus: &[usize], logical_cpus: usize) -> Result<u64, String> {
    if logical_cpus == 0 || logical_cpus > 64 { return Err("当前版本仅支持单组且最多 64 个逻辑 CPU".into()); }
    let mut mask = 0u64;
    for &cpu in cpus {
        if cpu >= logical_cpus { return Err(format!("CPU {cpu} 不存在")); }
        mask |= 1u64 << cpu;
    }
    if mask == 0 { return Err("CPU 列表不能为空".into()); }
    Ok(mask)
}

pub fn format_mask(mask: Option<u64>, logical_cpus: usize) -> String {
    match mask {
        None => "不支持（多 Processor Group）".into(),
        Some(value) => (0..logical_cpus).filter(|i| value & (1u64 << i) != 0)
            .map(|i| format!("CPU {i}")).collect::<Vec<_>>().join(", "),
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    #[test] fn last_cpu() { assert_eq!(last_cpu_mask(8), Some(128)); assert_eq!(last_cpu_mask(16), Some(1 << 15)); assert_eq!(last_cpu_mask(24), Some(1 << 23)); }
    #[test] fn list_mask() { assert_eq!(cpu_list_mask(&[0,2,4,6], 8).unwrap(), 0b01010101); }
    #[test] fn invalid_cpu() { assert!(cpu_list_mask(&[8], 8).is_err()); }
}
