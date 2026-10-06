use crate::{config::Config, process::{self, ApplyResult}};
use serde::Serialize;
use std::{collections::{HashMap, HashSet}, sync::mpsc::{Receiver, Sender}, thread, time::{Duration, Instant}};

#[derive(Debug)] pub enum Command { Scan, SetConfig(Config), Pause, Resume, Stop }

#[derive(Debug, Clone, PartialEq, Serialize)]
pub struct RuleSnapshot { pub index: usize, pub name: String, pub running: bool, pub pids: Vec<u32> }

#[derive(Debug, Clone, Serialize)]
#[serde(tag = "type", content = "data")]
pub enum Event {
    Applied { rule_name: String, result: ApplyResult },
    Status { message: String, paused: bool },
    Snapshot { rules: Vec<RuleSnapshot> },
}

pub fn spawn(mut config: Config, logical_cpus: usize, rx: Receiver<Command>, tx: Sender<Event>) -> thread::JoinHandle<()> {
    thread::spawn(move || {
        let mut paused = false;
        let mut cache = HashSet::<(usize, u32, String, u64)>::new();
        let mut retry_after = HashMap::<(usize, u32, String, u64), Instant>::new();
        let mut system = sysinfo::System::new();
        let mut last_snapshots: Option<Vec<RuleSnapshot>> = None;
        let mut scan = |config: &Config, cache: &mut HashSet<(usize, u32, String, u64)>, retry_after: &mut HashMap<(usize, u32, String, u64), Instant>| {
            let mut snapshots = Vec::new();
            let processes = if config.processes.iter().any(|rule| rule.enabled) { process::list_processes_with_system(&mut system) } else { Vec::new() };
            for (index, rule) in config.processes.iter().enumerate().filter(|(_, rule)| rule.enabled) {
                let matches = process::matching_from(&processes, rule);
                let pids = matches.iter().map(|item| item.pid).collect::<Vec<_>>();
                snapshots.push(RuleSnapshot { index, name: rule.name.clone(), running: !pids.is_empty(), pids });
                for item in matches {
                    let key = (index, item.pid, rule.name.clone(), item.start_time);
                    let retry_ready = retry_after.get(&key).map(|time| time.elapsed() >= Duration::from_secs(5)).unwrap_or(true);
                    if !cache.contains(&key) && retry_ready {
                        let result = process::apply(&item, rule, logical_cpus);
                        if result.ok { cache.insert(key.clone()); retry_after.remove(&key); } else { retry_after.insert(key.clone(), Instant::now()); }
                        let _ = tx.send(Event::Applied { rule_name: rule.name.clone(), result });
                    }
                }
            }
            if last_snapshots.as_ref() != Some(&snapshots) {
                last_snapshots = Some(snapshots.clone());
                let _ = tx.send(Event::Snapshot { rules: snapshots });
            }
        };
        if config.general.enabled { scan(&config, &mut cache, &mut retry_after); }
        loop {
            let interval = Duration::from_millis(config.general.interval_ms.clamp(250, 5000));
            match rx.recv_timeout(interval) {
                Ok(Command::Scan) => { cache.clear(); retry_after.clear(); if !paused && config.general.enabled { scan(&config, &mut cache, &mut retry_after); } },
                Ok(Command::SetConfig(next)) => { config = next; cache.clear(); retry_after.clear(); if !paused && config.general.enabled { scan(&config, &mut cache, &mut retry_after); } },
                Ok(Command::Pause) => { paused = true; let _ = tx.send(Event::Status { message: "监控已暂停".into(), paused }); },
                Ok(Command::Resume) => { paused = false; let _ = tx.send(Event::Status { message: "监控已恢复".into(), paused }); cache.clear(); retry_after.clear(); if config.general.enabled { scan(&config, &mut cache, &mut retry_after); } },
                Ok(Command::Stop) => break,
                Err(std::sync::mpsc::RecvTimeoutError::Timeout) => { if !paused && config.general.enabled { scan(&config, &mut cache, &mut retry_after); } },
                Err(std::sync::mpsc::RecvTimeoutError::Disconnected) => break,
            }
        }
    })
}
