<script setup lang="ts">
import {getCurrentInstance,onBeforeUnmount,shallowRef,watch} from 'vue';
import type {Vod} from '@/domain/types';
import {createDanmuTimeline,type PlayerDanmu} from '@/domain/playback-session';
import {usePlaybackSession} from '@/composables/usePlaybackSession';
import {assetUrl} from '@/services/api';
const props=withDefaults(defineProps<{film:Vod;episode:number;src:string;trialSeconds?:number;autoplay?:boolean;muted?:boolean;showFullscreen?:boolean;danmuEnabled?:boolean;danmu?:PlayerDanmu[]}>(),{trialSeconds:0,autoplay:true,muted:false,showFullscreen:true,danmuEnabled:false,danmu:()=>[]});
const emit=defineEmits<{play:[];pause:[];progress:[seconds:number];ended:[];error:[event:any];expired:[message:string]}>();
const instance=getCurrentInstance(),videoId='hetun-player-'+instance?.uid;
const context=()=>uni.createVideoContext(videoId,instance?.proxy);
let revision=0,disposed=false,playing=false,wantsPlay=props.autoplay;
const timeline=createDanmuTimeline();
const session=usePlaybackSession(message=>{playing=false;wantsPlay=false;context().pause();emit('progress',session.position.value);emit('expired',message);});
const render=shallowRef<{key:number;initialTime:number;autoplay:boolean;danmu:boolean;events:Record<string,(event?:any)=>void>}>({key:0,initialTime:0,autoplay:false,danmu:false,events:{}});
function mount(initialTime:number,autoplay:boolean){
 const token=++revision;timeline.reset(initialTime);
 const current=()=>!disposed&&token===revision;
 render.value={key:token,initialTime,autoplay,danmu:props.danmuEnabled,events:{
  play:()=>{if(!current())return;session.played();if(session.expired.value)return;playing=true;wantsPlay=true;emit('play');},
  pause:()=>{if(!current())return;playing=false;wantsPlay=false;session.flush();emit('pause');},
  timeupdate:event=>{if(!current())return;session.timeupdate(event);emit('progress',session.position.value);if(!playing||session.expired.value||!props.danmuEnabled)return;for(const row of timeline.advance(session.position.value))context().sendDanmu({text:row.text,color:row.color});},
  ended:()=>{if(!current())return;playing=false;wantsPlay=false;if(session.completed()){emit('progress',0);emit('ended');}},
  error:event=>{if(!current())return;playing=false;wantsPlay=false;session.flush();emit('error',event);}
 }};
}
watch(()=>[props.film.id,props.episode,props.src,props.trialSeconds] as const,()=>{session.start(props.film,props.episode,props.trialSeconds);playing=false;wantsPlay=props.autoplay;mount(session.initialTime.value,wantsPlay);emit('progress',session.position.value);},{immediate:true,flush:'sync'});
watch(()=>props.danmu,rows=>timeline.set(rows),{immediate:true});
// uni video reads enableDanmu at mount. Keep the current clock and pause state when it changes.
watch(()=>props.danmuEnabled,()=>mount(session.position.value,playing||wantsPlay),{flush:'sync'});
onBeforeUnmount(()=>{disposed=true;revision++;});
defineExpose({play:()=>context().play(),pause:()=>context().pause(),flush:session.flush});
</script>
<template><view class="playback-video"><video v-if="render" :id="videoId" :key="render.key" class="playback-video-element" :src="src" :poster="assetUrl(film.poster)" :title="film.title" :initial-time="render.initialTime" :autoplay="render.autoplay" :muted="muted" controls object-fit="contain" :show-fullscreen-btn="showFullscreen" :enable-progress-gesture="true" :enable-danmu="render.danmu" :danmu-btn="false" :danmu-list="[]" v-on="render.events"/><view v-if="session.expired.value" class="trial-note" role="status">本线路试看已结束</view></view></template>
<style scoped>.playback-video{position:relative;background:#000;overflow:hidden}.playback-video-element{display:block;width:100%;height:100%}.trial-note{position:absolute;left:0;right:0;bottom:48px;padding:10px 16px;text-align:center;background:#000c;color:#fff;font-size:13px;pointer-events:none}</style>
