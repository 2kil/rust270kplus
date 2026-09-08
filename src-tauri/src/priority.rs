use serde::{Deserialize, Serialize};
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
#[serde(rename_all = "snake_case")]
pub enum Priority { Realtime, High, AboveNormal, Normal, BelowNormal, Idle }
impl Default for Priority { fn default()->Self{Self::Normal} }
impl Priority { pub fn label(&self)->&'static str{match self{Self::Realtime=>"实时",Self::High=>"高",Self::AboveNormal=>"高于正常",Self::Normal=>"正常",Self::BelowNormal=>"低于正常",Self::Idle=>"空闲"}} }
#[cfg(windows)] impl Priority { pub fn class(&self)->windows::Win32::System::Threading::PROCESS_CREATION_FLAGS{use windows::Win32::System::Threading::*;match self{Self::Realtime=>REALTIME_PRIORITY_CLASS,Self::High=>HIGH_PRIORITY_CLASS,Self::AboveNormal=>ABOVE_NORMAL_PRIORITY_CLASS,Self::Normal=>NORMAL_PRIORITY_CLASS,Self::BelowNormal=>BELOW_NORMAL_PRIORITY_CLASS,Self::Idle=>IDLE_PRIORITY_CLASS}} }
#[cfg(test)] mod tests{use super::*;#[test]fn default_normal(){assert_eq!(Priority::default(),Priority::Normal);}}
