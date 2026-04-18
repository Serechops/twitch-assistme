export namespace main {
	
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
	
	    static createFrom(source: any = {}) {
	        return new UserInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.login = source["login"];
	        this.displayName = source["displayName"];
	        this.profileImageUrl = source["profileImageUrl"];
	    }
	}

}

