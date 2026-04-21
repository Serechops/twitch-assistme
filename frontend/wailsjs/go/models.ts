export namespace hotkey {
	
	export class Config {
	    type: string;
	    modifiers: string[];
	    key: string;
	    button: number;
	
	    static createFrom(source: any = {}) {
	        return new Config(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.type = source["type"];
	        this.modifiers = source["modifiers"];
	        this.key = source["key"];
	        this.button = source["button"];
	    }
	}

}

export namespace main {
	
	export class AICommandResultDTO {
	    transcript: string;
	    message: string;
	    actions: string[];
	
	    static createFrom(source: any = {}) {
	        return new AICommandResultDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.transcript = source["transcript"];
	        this.message = source["message"];
	        this.actions = source["actions"];
	    }
	}
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
	export class CategoryDTO {
	    id: string;
	    name: string;
	    boxArtURL: string;
	
	    static createFrom(source: any = {}) {
	        return new CategoryDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.boxArtURL = source["boxArtURL"];
	    }
	}
	export class ChannelInfoDTO {
	    title: string;
	    gameID: string;
	    gameName: string;
	    language: string;
	    tags: string[];
	
	    static createFrom(source: any = {}) {
	        return new ChannelInfoDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.title = source["title"];
	        this.gameID = source["gameID"];
	        this.gameName = source["gameName"];
	        this.language = source["language"];
	        this.tags = source["tags"];
	    }
	}
	export class ClipCreatedDTO {
	    id: string;
	    editUrl: string;
	
	    static createFrom(source: any = {}) {
	        return new ClipCreatedDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.editUrl = source["editUrl"];
	    }
	}
	export class ClipDTO {
	    id: string;
	    url: string;
	    editUrl: string;
	    title: string;
	    creatorName: string;
	    thumbnailUrl: string;
	    viewCount: number;
	    duration: number;
	    createdAt: string;
	    isFeatured: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ClipDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.url = source["url"];
	        this.editUrl = source["editUrl"];
	        this.title = source["title"];
	        this.creatorName = source["creatorName"];
	        this.thumbnailUrl = source["thumbnailUrl"];
	        this.viewCount = source["viewCount"];
	        this.duration = source["duration"];
	        this.createdAt = source["createdAt"];
	        this.isFeatured = source["isFeatured"];
	    }
	}
	export class CreateRewardInput {
	    title: string;
	    cost: number;
	    prompt: string;
	    isEnabled: boolean;
	    backgroundColor: string;
	    isUserInputRequired: boolean;
	    shouldRedemptionsSkipRequestQueue: boolean;
	    maxPerStreamEnabled: boolean;
	    maxPerStream: number;
	    maxPerUserEnabled: boolean;
	    maxPerUser: number;
	    cooldownEnabled: boolean;
	    cooldownSeconds: number;
	
	    static createFrom(source: any = {}) {
	        return new CreateRewardInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.title = source["title"];
	        this.cost = source["cost"];
	        this.prompt = source["prompt"];
	        this.isEnabled = source["isEnabled"];
	        this.backgroundColor = source["backgroundColor"];
	        this.isUserInputRequired = source["isUserInputRequired"];
	        this.shouldRedemptionsSkipRequestQueue = source["shouldRedemptionsSkipRequestQueue"];
	        this.maxPerStreamEnabled = source["maxPerStreamEnabled"];
	        this.maxPerStream = source["maxPerStream"];
	        this.maxPerUserEnabled = source["maxPerUserEnabled"];
	        this.maxPerUser = source["maxPerUser"];
	        this.cooldownEnabled = source["cooldownEnabled"];
	        this.cooldownSeconds = source["cooldownSeconds"];
	    }
	}
	export class CreatorGoalDTO {
	    id: string;
	    type: string;
	    description: string;
	    currentAmount: number;
	    targetAmount: number;
	    createdAt: string;
	
	    static createFrom(source: any = {}) {
	        return new CreatorGoalDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.type = source["type"];
	        this.description = source["description"];
	        this.currentAmount = source["currentAmount"];
	        this.targetAmount = source["targetAmount"];
	        this.createdAt = source["createdAt"];
	    }
	}
	export class CustomRewardDTO {
	    id: string;
	    title: string;
	    prompt: string;
	    cost: number;
	    backgroundColor: string;
	    isEnabled: boolean;
	    isPaused: boolean;
	    isInStock: boolean;
	    isUserInputRequired: boolean;
	    shouldRedemptionsSkipRequestQueue: boolean;
	    maxPerStreamEnabled: boolean;
	    maxPerStream: number;
	    maxPerUserEnabled: boolean;
	    maxPerUser: number;
	    cooldownEnabled: boolean;
	    cooldownSeconds: number;
	
	    static createFrom(source: any = {}) {
	        return new CustomRewardDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.title = source["title"];
	        this.prompt = source["prompt"];
	        this.cost = source["cost"];
	        this.backgroundColor = source["backgroundColor"];
	        this.isEnabled = source["isEnabled"];
	        this.isPaused = source["isPaused"];
	        this.isInStock = source["isInStock"];
	        this.isUserInputRequired = source["isUserInputRequired"];
	        this.shouldRedemptionsSkipRequestQueue = source["shouldRedemptionsSkipRequestQueue"];
	        this.maxPerStreamEnabled = source["maxPerStreamEnabled"];
	        this.maxPerStream = source["maxPerStream"];
	        this.maxPerUserEnabled = source["maxPerUserEnabled"];
	        this.maxPerUser = source["maxPerUser"];
	        this.cooldownEnabled = source["cooldownEnabled"];
	        this.cooldownSeconds = source["cooldownSeconds"];
	    }
	}
	export class GameSourceDTO {
	    title: string;
	    url: string;
	
	    static createFrom(source: any = {}) {
	        return new GameSourceDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.title = source["title"];
	        this.url = source["url"];
	    }
	}
	export class GameGuideResultDTO {
	    answer: string;
	    sources: GameSourceDTO[];
	
	    static createFrom(source: any = {}) {
	        return new GameGuideResultDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.answer = source["answer"];
	        this.sources = this.convertValues(source["sources"], GameSourceDTO);
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
	
	export class HypeTrainEventDTO {
	    level: number;
	    total: number;
	    goal: number;
	    startedAt: string;
	    expiresAt: string;
	
	    static createFrom(source: any = {}) {
	        return new HypeTrainEventDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.level = source["level"];
	        this.total = source["total"];
	        this.goal = source["goal"];
	        this.startedAt = source["startedAt"];
	        this.expiresAt = source["expiresAt"];
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
	export class PredictionOutcomeDTO {
	    id: string;
	    title: string;
	    color: string;
	    users: number;
	    channelPoints: number;
	
	    static createFrom(source: any = {}) {
	        return new PredictionOutcomeDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.title = source["title"];
	        this.color = source["color"];
	        this.users = source["users"];
	        this.channelPoints = source["channelPoints"];
	    }
	}
	export class PredictionDTO {
	    id: string;
	    title: string;
	    winningOutcomeId: string;
	    outcomes: PredictionOutcomeDTO[];
	    predictionWindow: number;
	    status: string;
	    createdAt: string;
	    endedAt: string;
	    lockedAt: string;
	
	    static createFrom(source: any = {}) {
	        return new PredictionDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.title = source["title"];
	        this.winningOutcomeId = source["winningOutcomeId"];
	        this.outcomes = this.convertValues(source["outcomes"], PredictionOutcomeDTO);
	        this.predictionWindow = source["predictionWindow"];
	        this.status = source["status"];
	        this.createdAt = source["createdAt"];
	        this.endedAt = source["endedAt"];
	        this.lockedAt = source["lockedAt"];
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
	
	export class RaidTargetDTO {
	    id: string;
	    login: string;
	    displayName: string;
	    gameName: string;
	    title: string;
	    viewerCount: number;
	    startedAt: string;
	    thumbnailURL: string;
	    avatarURL: string;
	    tags: string[];
	
	    static createFrom(source: any = {}) {
	        return new RaidTargetDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.login = source["login"];
	        this.displayName = source["displayName"];
	        this.gameName = source["gameName"];
	        this.title = source["title"];
	        this.viewerCount = source["viewerCount"];
	        this.startedAt = source["startedAt"];
	        this.thumbnailURL = source["thumbnailURL"];
	        this.avatarURL = source["avatarURL"];
	        this.tags = source["tags"];
	    }
	}
	export class RedemptionDTO {
	    id: string;
	    userId: string;
	    userLogin: string;
	    userName: string;
	    userInput: string;
	    status: string;
	    redeemedAt: string;
	    rewardId: string;
	    rewardTitle: string;
	    rewardCost: number;
	
	    static createFrom(source: any = {}) {
	        return new RedemptionDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.userId = source["userId"];
	        this.userLogin = source["userLogin"];
	        this.userName = source["userName"];
	        this.userInput = source["userInput"];
	        this.status = source["status"];
	        this.redeemedAt = source["redeemedAt"];
	        this.rewardId = source["rewardId"];
	        this.rewardTitle = source["rewardTitle"];
	        this.rewardCost = source["rewardCost"];
	    }
	}
	export class SettingsDTO {
	    soundEnabled: boolean;
	    soundPath: string;
	    soundVolume: number;
	    ignoreOwn: boolean;
	    cooldownMs: number;
	    openAIApiKey: string;
	    voiceFeedback: boolean;
	    hotkeyConfig?: hotkey.Config;
	
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
	        this.openAIApiKey = source["openAIApiKey"];
	        this.voiceFeedback = source["voiceFeedback"];
	        this.hotkeyConfig = this.convertValues(source["hotkeyConfig"], hotkey.Config);
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

