export namespace main {
	
	export class AppError {
	    code: string;
	    zh: string;
	    en: string;
	    detail?: string;
	
	    static createFrom(source: any = {}) {
	        return new AppError(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.code = source["code"];
	        this.zh = source["zh"];
	        this.en = source["en"];
	        this.detail = source["detail"];
	    }
	}
	export class Result {
	    ok: boolean;
	    error?: AppError;
	    paths?: string[];
	    path?: string;
	    taskId?: string;
	    total: number;
	    totalBytes: number;
	    algo?: string;
	
	    static createFrom(source: any = {}) {
	        return new Result(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ok = source["ok"];
	        this.error = this.convertValues(source["error"], AppError);
	        this.paths = source["paths"];
	        this.path = source["path"];
	        this.taskId = source["taskId"];
	        this.total = source["total"];
	        this.totalBytes = source["totalBytes"];
	        this.algo = source["algo"];
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

}

