export namespace manager {
	
	export class ProcessRule {
	    name: string;
	    enabled: boolean;
	    affinity: string;
	    cpus: number[];
	    priority: string;
	    match_mode: string;
	    path?: string;
	
	    static createFrom(source: any = {}) {
	        return new ProcessRule(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.enabled = source["enabled"];
	        this.affinity = source["affinity"];
	        this.cpus = source["cpus"];
	        this.priority = source["priority"];
	        this.match_mode = source["match_mode"];
	        this.path = source["path"];
	    }
	}
	export class General {
	    enabled: boolean;
	    interval_ms: number;
	    start_with_windows: boolean;
	    minimize_to_tray: boolean;
	    window_width: number;
	    window_height: number;
	
	    static createFrom(source: any = {}) {
	        return new General(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.interval_ms = source["interval_ms"];
	        this.start_with_windows = source["start_with_windows"];
	        this.minimize_to_tray = source["minimize_to_tray"];
	        this.window_width = source["window_width"];
	        this.window_height = source["window_height"];
	    }
	}
	export class Config {
	    general: General;
	    processes: ProcessRule[];
	
	    static createFrom(source: any = {}) {
	        return new Config(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.general = this.convertValues(source["general"], General);
	        this.processes = this.convertValues(source["processes"], ProcessRule);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class ProcessInfo {
	    pid: number;
	    name: string;
	    path?: string;
	    start_time: number;
	
	    static createFrom(source: any = {}) {
	        return new ProcessInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.pid = source["pid"];
	        this.name = source["name"];
	        this.path = source["path"];
	        this.start_time = source["start_time"];
	    }
	}
	
	export class RuleSnapshot {
	    index: number;
	    name: string;
	    running: boolean;
	    pids: number[];
	
	    static createFrom(source: any = {}) {
	        return new RuleSnapshot(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.index = source["index"];
	        this.name = source["name"];
	        this.running = source["running"];
	        this.pids = source["pids"];
	    }
	}
	export class RuntimeStatus {
	    enabled: boolean;
	    paused: boolean;
	    logical_cpus: number;
	    processor_group_supported: boolean;
	    permission_denied: boolean;
	    config_error?: string;
	
	    static createFrom(source: any = {}) {
	        return new RuntimeStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.paused = source["paused"];
	        this.logical_cpus = source["logical_cpus"];
	        this.processor_group_supported = source["processor_group_supported"];
	        this.permission_denied = source["permission_denied"];
	        this.config_error = source["config_error"];
	    }
	}

}

