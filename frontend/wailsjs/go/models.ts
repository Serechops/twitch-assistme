export namespace main {
	
	export class PollChoiceDTO {
	    id: string;
	    title: string;
	    votes: number;
	    channelPointsVotes: number;
	
	    static createFrom(source: any = {}) {
	        return new PollChoiceDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.title = source["title"];
	        this.votes = source["votes"];
	        this.channelPointsVotes = source["channelPointsVotes"];
	    }
	}
	export class ArchivedPollDTO {
	    id: number;
	    pollId: string;
	    title: string;
	    status: string;
	    duration: number;
	    choices: PollChoiceDTO[];
	    startedAt: string;
	    endedAt: string;
	    createdAt: number;
	
	    static createFrom(source: any = {}) {
	        return new ArchivedPollDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.pollId = source["pollId"];
	        this.title = source["title"];
	        this.status = source["status"];
	        this.duration = source["duration"];
	        this.choices = this.convertValues(source["choices"], PollChoiceDTO);
	        this.startedAt = source["startedAt"];
	        this.endedAt = source["endedAt"];
	        this.createdAt = source["createdAt"];
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
	
	export class PollDTO {
	    id: string;
	    title: string;
	    choices: PollChoiceDTO[];
	    status: string;
	    duration: number;
	    startedAt: string;
	    endedAt: string;
	
	    static createFrom(source: any = {}) {
	        return new PollDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.title = source["title"];
	        this.choices = this.convertValues(source["choices"], PollChoiceDTO);
	        this.status = source["status"];
	        this.duration = source["duration"];
	        this.startedAt = source["startedAt"];
	        this.endedAt = source["endedAt"];
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
	export class PollTemplateDTO {
	    id: number;
	    name: string;
	    title: string;
	    choices: string[];
	    duration: number;
	    createdAt: number;
	
	    static createFrom(source: any = {}) {
	        return new PollTemplateDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.title = source["title"];
	        this.choices = source["choices"];
	        this.duration = source["duration"];
	        this.createdAt = source["createdAt"];
	    }
	}
	export class SettingsDTO {
	    soundEnabled: boolean;
	    soundPath: string;
	    soundVolume: number;
	    ignoreOwn: boolean;
	    cooldownMs: number;
	
	    static createFrom(source: any = {}) {
	        return new SettingsDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.soundEnabled = source["soundEnabled"];
	        this.soundPath = source["soundPath"];
	        this.soundVolume = source["soundVolume"];
	        this.ignoreOwn = source["ignoreOwn"];
	        this.cooldownMs = source["cooldownMs"];
	    }
	}
	export class UserInfo {
	    id: string;
	    login: string;
	    displayName: string;
	    profileImageUrl: string;
	    offlineImageUrl: string;
	
	    static createFrom(source: any = {}) {
	        return new UserInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.login = source["login"];
	        this.displayName = source["displayName"];
	        this.profileImageUrl = source["profileImageUrl"];
	        this.offlineImageUrl = source["offlineImageUrl"];
	    }
	}

}

