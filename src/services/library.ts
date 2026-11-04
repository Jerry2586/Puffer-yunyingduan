import type {Vod} from '@/domain/types';
import {applicationRuntime} from './application-runtime';
import {applicationApi,type HistoryEntry} from './application';
import {getServiceBase} from './api';
import {historyScope,historyMember} from './member-session';
export function getLibrary():Vod[]{try{const v=uni.getStorageSync('hetun-favorites');return Array.isArray(v)?v:[];}catch{return [];}}
export function toggleFavorite(vod:Vod):boolean{const list=getLibrary(),index=list.findIndex(v=>v.id===vod.id);if(index>=0)list.splice(index,1);else list.unshift(vod);uni.setStorageSync('hetun-favorites',list);return index<0;}
export const getHistoryScope=()=>historyScope(getServiceBase());
const key=()=> 'hetun-history-v2:'+getHistoryScope();
export function getHistory():Vod[]{try{const v=uni.getStorageSync(key());return Array.isArray(v)?v:[];}catch{return [];}}
const localTimes=new Map<string,number>(),remoteTimes=new Map<string,number>();
const pending=new Map<string,{vod:Vod;scope:string}>();
let sending=false;
function stamp(map:Map<string,number>,id:string,value:number){map.delete(id);map.set(id,value);if(map.size>300)map.delete(map.keys().next().value!);}
export function remember(vod:Vod,episode=1,position=0,force=false,owner=getHistoryScope()){
 const identity=getHistoryScope();if(owner!==identity)return;
 const now=Date.now(),entryKey=identity+':'+vod.id,previous=localTimes.get(entryKey)||0;
 if(!force&&now-previous<2000)return;
 stamp(localTimes,entryKey,now);
 const row={...vod,historyEpisode:Number.isFinite(episode)?Math.max(1,Math.min(10000,Math.floor(episode))):1,historyPosition:Number.isFinite(position)?Math.max(0,Math.min(86400000,Math.floor(position))):0,historyUpdatedAt:new Date(now).toISOString()};
 uni.setStorageSync(key(),[row,...getHistory().filter(v=>v.id!==vod.id)].slice(0,applicationRuntime.history.enabled?applicationRuntime.history.limit:60));
 if(!applicationRuntime.history.enabled||!historyMember(getServiceBase()))return;
 if(!force&&now-(remoteTimes.get(entryKey)||0)<applicationRuntime.history.intervalSeconds*1000)return;
 // Keep the final write for each film when users switch faster than the network.
 pending.set(entryKey,{vod:row,scope:identity});if(pending.size>100)pending.delete(pending.keys().next().value!);
 if(!sending)void sendPending();
}
async function sendPending(){
 sending=true;
 try{while(pending.size){const [id,next]=pending.entries().next().value!;pending.delete(id);
  if(next.scope!==getHistoryScope()||!applicationRuntime.history.enabled||!historyMember(getServiceBase()))continue;
  try{await applicationApi.remember(next.vod.id,next.vod.historyEpisode||1,next.vod.historyPosition||0);stamp(remoteTimes,id,Date.now());}catch{/* The local copy remains available while offline. */}
 }}finally{sending=false;}
}
export function mergeHistory(local:Vod[],cloud:HistoryEntry[]):Vod[]{
 const rows=new Map(local.map(v=>[v.id,v]));
 for(const entry of cloud){const old=rows.get(entry.videoId);if(old&&(Date.parse(old.historyUpdatedAt||'')||0)>=(Date.parse(entry.updatedAt)||0))continue;rows.set(entry.videoId,{...(old||{id:entry.videoId,title:entry.title||'影片 '+entry.videoId,poster:entry.poster||'',categoryId:'',category:'',year:'',area:'',score:0,note:'',description:'',sources:[]}),historyEpisode:entry.episode,historyPosition:entry.position,historyUpdatedAt:entry.updatedAt} as Vod);}
 return [...rows.values()].sort((a,b)=>(Date.parse(b.historyUpdatedAt||'')||0)-(Date.parse(a.historyUpdatedAt||'')||0));
}
export async function syncHistory():Promise<Vod[]>{
 const identity=getHistoryScope();if(!applicationRuntime.history.enabled||!historyMember(getServiceBase()))return getHistory();
 const result=await applicationApi.history();if(identity!==getHistoryScope())return getHistory();
 const rows=mergeHistory(getHistory(),result.items).slice(0,applicationRuntime.history.limit);uni.setStorageSync(key(),rows);return rows;
}
