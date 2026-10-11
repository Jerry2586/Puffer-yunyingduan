import type {Announcement,ClientSettings} from './types';
export type UpdateTarget='android'|'ios'|'web';
export function compareVersion(a:string,b:string){
 if(!/^\d+\.\d+\.\d+$/.test(a)||!/^\d+\.\d+\.\d+$/.test(b))return 0;
 const x=a.split('.').map(Number),y=b.split('.').map(Number);for(let i=0;i<3;i++)if(x[i]!==y[i])return x[i]>y[i]?1:-1;return 0;
}
export function updatePolicy(s:ClientSettings,current:string,target:UpdateTarget){
 const available=compareVersion(s.latestVersion,current)>0;
 const url=target==='ios'?s.iosUrl:target==='android'?s.androidUrl:'';
 return {available,url,forced:available&&s.forceUpdate&&target!=='web'&&!!url,automatic:available&&target!=='web'&&(s.notifyOnLaunch||s.forceUpdate)};
}
export function announcementKey(a:Announcement,scope:string){
 const content=JSON.stringify([scope,a.title,a.text,a.image,a.actionLabel,a.page,a.url]);let hash=2166136261;for(let i=0;i<content.length;i++){hash^=content.charCodeAt(i);hash=Math.imul(hash,16777619);}return 'hetun-announcement-'+(hash>>>0).toString(16);
}
export function shouldAnnounce(a:Announcement,seenLaunch:boolean,seenDate:string,today:string){return a.enabled&&a.style==='modal'&&!!a.text.trim()&&!seenLaunch&&(a.frequency==='launch'||(a.frequency==='daily'?seenDate!==today:!seenDate));}
