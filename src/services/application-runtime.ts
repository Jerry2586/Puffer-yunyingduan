import {reactive} from 'vue';
export interface ApplicationConfig {
 application:{searchDelayMs:number;listLimit:number;tvDownloadUrl:string};
 registration:{enabled:boolean;dailyIpLimit:number;managed:boolean;active:boolean;points:number;invitePoints:number};
 captcha:{bindIp:boolean;login:boolean;register:boolean;comments:boolean;danmu:boolean};
 search:{intervalMinutes:number};history:{enabled:boolean;intervalSeconds:number;limit:number};
 avatar:{upload:boolean;library:string[]};share:{text:string;invite:boolean;contactText:string;contactUrl:string;contactMode:'copy'|'link'};
 membership:{managed:boolean;groupId:number;day:number;week:number;month:number;year:number};
 comments:{enabled:boolean;pinnedEnabled:boolean;pinnedNickname:string;pinnedAvatar:string;pinnedContent:string};
}
export interface PublicApplication {
 sourceKey:string;version:number;searchDelayMs:number;listLimit:number;tvDownloadUrl:string;registrationEnabled:boolean;
 captcha:ApplicationConfig['captcha'];history:ApplicationConfig['history'];avatar:ApplicationConfig['avatar'];share:ApplicationConfig['share'];comments:ApplicationConfig['comments'];membership:ApplicationConfig['membership'];
 capabilities:{vpnDetection:boolean;nativeTVDownload:boolean};
}
export const applicationRuntime=reactive<PublicApplication>({sourceKey:'',version:0,searchDelayMs:350,listLimit:100,tvDownloadUrl:'',registrationEnabled:true,captcha:{bindIp:true,login:false,register:false,comments:false,danmu:false},history:{enabled:false,intervalSeconds:60,limit:100},avatar:{upload:false,library:[]},share:{text:'',invite:false,contactText:'',contactUrl:'',contactMode:'copy'},comments:{enabled:true,pinnedEnabled:false,pinnedNickname:'',pinnedAvatar:'',pinnedContent:''},membership:{managed:false,groupId:0,day:0,week:0,month:0,year:0},capabilities:{vpnDetection:false,nativeTVDownload:false}});
export const danmuRuntime=reactive({enabled:true,thirdParty:false,density:100,review:false});
export function receiveApplication(value?:PublicApplication){if(value)Object.assign(applicationRuntime,value);}
export function receiveDanmu(value?:Partial<typeof danmuRuntime>){if(value)Object.assign(danmuRuntime,value);}
