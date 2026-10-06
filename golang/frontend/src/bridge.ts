import * as backend from "../wailsjs/go/main/App";
import { manager } from "../wailsjs/go/models";
import { EventsOn } from "../wailsjs/runtime/runtime";

export async function invoke<T = void>(command: string, args?: Record<string, unknown>): Promise<T> {
  const commands: Record<string, () => unknown> = {
    get_config: () => backend.GetConfig(),
    save_config: () => backend.SaveConfig(args?.config as manager.Config),
    list_running_processes: () => backend.ListRunningProcesses(),
    get_rule_snapshots: () => backend.GetRuleSnapshots(),
    get_runtime_status: () => backend.GetRuntimeStatus(),
    get_logs: () => backend.GetLogs(),
    clear_logs: () => backend.ClearLogs(),
    scan_now: () => backend.ScanNow(),
    apply_all: () => backend.ApplyAll(),
    set_monitor_paused: () => backend.SetMonitorPaused(Boolean(args?.paused)),
    set_start_with_windows: () => backend.SetStartWithWindows(Boolean(args?.enabled)),
    delete_process_rule: () => backend.DeleteProcessRule(Number(args?.index)),
    save_window_size: () => backend.SaveWindowSize(),
  };
  const call = commands[command];
  if (!call) throw new Error(`未知命令：${command}`);
  return await call() as T;
}

export async function listen<T>(event: string, handler: (event: { payload: T }) => void): Promise<void> {
  EventsOn(event, (payload: T) => handler({ payload }));
}
