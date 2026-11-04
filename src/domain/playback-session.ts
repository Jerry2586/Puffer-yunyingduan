import type {Vod} from './types';
export interface PlayerDanmu {text:string;color:string;time:number}
export function playbackSourceId(film:Vod,index:number):number {
 const sid=Number(film.sources[index]?.sid);
 return Number.isSafeInteger(sid)&&sid>0?sid:index+1;
}
export function resumeEpisode(film:Vod,history:Vod[],source=0):number {
 const episode=Number(history.find(row=>row.id===film.id)?.historyEpisode);
 return Number.isSafeInteger(episode)&&episode>0&&episode<=(film.sources[source]?.episodes.length||0)?episode-1:0;
}
export function danmuMilliseconds(seconds:number):number {return Number.isFinite(seconds)?Math.round(Math.min(86400,Math.max(0,seconds))*1000):0;}
export function danmuSeconds(milliseconds:number):number {return Number.isFinite(milliseconds)?Math.max(0,milliseconds)/1000:0;}
// Native video components copy their initial list. Send later loads against the actual clock.
export function createDanmuTimeline(){
 let rows:PlayerDanmu[]=[],previous=0;
 return {
  set(value:PlayerDanmu[]){rows=value.filter(row=>row.text&&Number.isFinite(row.time)&&row.time>=0).slice().sort((a,b)=>a.time-b.time);},
  reset(position=0){previous=Math.max(0,position)-0.001;},
  advance(position:number):PlayerDanmu[]{
   if(!Number.isFinite(position)||position<0)return [];
   const from=previous;previous=position;
   // A seek must not replay every comment between the old and new positions.
   if(position<from||position-from>2)return [];
   return rows.filter(row=>row.time>from&&row.time<=position);
  }
 };
}
