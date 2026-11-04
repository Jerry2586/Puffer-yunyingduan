import {ref,onBeforeUnmount} from 'vue';
import type {Vod} from '@/domain/types';
import {remember,getHistory,getHistoryScope} from '@/services/library';
export function usePlaybackSession(stop:(message:string)=>void){
 const position=ref(0),initialTime=ref(0),expired=ref(false),trial=ref(0);
 let current:{film:Vod;episode:number;scope:string}|undefined,started=false;
 function flush(){if(current&&started)remember(current.film,current.episode,position.value,true,current.scope);}
 function reset(){flush();current=undefined;started=false;position.value=0;initialTime.value=0;trial.value=0;expired.value=false;}
 function start(film:Vod,episode:number,seconds=0){
  reset();trial.value=Number.isFinite(seconds)?Math.max(0,seconds):0;
  const old=getHistory().find(v=>v.id===film.id),saved=Number(old?.historyPosition);
  const resume=old?.historyEpisode===episode&&Number.isFinite(saved)?Math.max(0,saved):0;
  initialTime.value=trial.value&&resume>=trial.value?0:resume;position.value=initialTime.value;
  current={film,episode,scope:getHistoryScope()};
 }
 function played(){if(expired.value){stop('本线路试看已结束。');return;}if(current){started=true;remember(current.film,current.episode,position.value,true,current.scope);}}
 function timeupdate(event:any){
  if(!current||!started||expired.value)return;
  const value=Number(event?.detail?.currentTime);if(!Number.isFinite(value)||value<0)return;
  position.value=trial.value?Math.min(value,trial.value):value;
  if(trial.value&&value>=trial.value){expired.value=true;flush();stop('本线路试看已结束。');return;}
  remember(current.film,current.episode,position.value,false,current.scope);
 }
 function completed(){if(!current||!started||expired.value)return false;position.value=0;remember(current.film,current.episode,0,true,current.scope);started=false;return true;}
 onBeforeUnmount(flush);
 return {position,initialTime,expired,trial,start,reset,flush,played,timeupdate,completed};
}
