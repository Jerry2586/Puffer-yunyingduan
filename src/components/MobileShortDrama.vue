<script setup lang="ts">
import {computed,getCurrentInstance,onUnmounted,ref,watch} from 'vue';
import type {AppPage,Vod} from '@/domain/types';
import {api,assetUrl} from '@/services/api';
import {remember} from '@/services/library';
import AppIcon from './AppIcon.vue';
import AppEmptyState from './AppEmptyState.vue';
const props=defineProps<{item?:Vod;bound:boolean;loading:boolean;favorite:boolean;ranking:boolean;editing?:boolean;demo?:boolean;blocked?:boolean}>();
const emit=defineEmits<{navigate:[page:AppPage];step:[by:number];favorite:[];select:[]}>();
const detail=ref<Vod|null>(null),source=ref(0),episode=ref(0),url=ref(''),pending=ref(false),error=ref(''),picker=ref(false),muted=ref(true),playing=ref(false);
const episodes=computed(()=>detail.value?.sources[source.value]?.episodes||[]);
const instance=getCurrentInstance()?.proxy;let generation=0,touchY=0;
const videoId='hetun-short-'+getCurrentInstance()?.uid;
watch(()=>props.item?.id,()=>{generation++;detail.value=null;source.value=0;episode.value=0;url.value='';pending.value=false;error.value='';picker.value=false;playing.value=false;});
watch(()=>props.blocked,value=>{if(value){generation++;url.value='';pending.value=false;playing.value=false;}});
onUnmounted(()=>{generation++;});
async function resolveDetail(){
 if(detail.value?.id===props.item?.id)return detail.value;
 if(!props.item||props.blocked)return null;
 if(props.editing){emit('select');return null;}
 if(props.demo){error.value='请连接内容服务后观看短剧。';return null;}
 const token=++generation,id=props.item.id;pending.value=true;error.value='';
 try{const response=await api.catalog(1,'','',id);if(token!==generation||props.item?.id!==id)return null;
  const result=response.items[0];if(!result)throw new Error('该短剧已下架或暂不可用。');detail.value=result;return result;
 }catch(e){if(token===generation)error.value=(e as Error).message;return null;}
 finally{if(token===generation)pending.value=false;}
}
async function playSelected(){
 if(props.blocked)return;
 const film=await resolveDetail();if(!film)return;
 if(!episodes.value[episode.value]){error.value='当前线路没有可播放集数。';return;}
 const token=++generation,id=film.id,sid=source.value,nid=episode.value;pending.value=true;error.value='';url.value='';playing.value=false;
 try{const response=await api.playback(id,sid+1,nid+1);if(token!==generation||props.item?.id!==id)return;
  if(!response.url.startsWith('https://')&&!response.url.startsWith('http://'))throw new Error('播放地址不受支持。');url.value=response.url;
 }catch(e){if(token===generation)error.value=(e as Error).message;}
 finally{if(token===generation)pending.value=false;}
}
async function chooseEpisodes(){if(await resolveDetail())picker.value=true;}
function chooseEpisode(index:number){episode.value=index;picker.value=false;playSelected();}
function chooseSource(index:number){source.value=index;episode.value=0;}
function playbackStarted(){playing.value=true;if(detail.value)remember(detail.value);}
function playbackError(){playing.value=false;error.value='视频加载失败，请重试或选择其他线路。';}
function togglePlayback(){if(!url.value){playSelected();return;}const context=uni.createVideoContext(videoId,instance);if(playing.value)context.pause();else context.play();}
function ended(){playing.value=false;if(episode.value+1<episodes.value.length){episode.value++;playSelected();}}
function touchStart(e:any){touchY=e.changedTouches?.[0]?.clientY||0;}
function touchEnd(e:any){const delta=(e.changedTouches?.[0]?.clientY||0)-touchY;if(Math.abs(delta)>55&&!picker.value)emit('step',delta>0?-1:1);}
</script>
<template>
 <view class="shortdrama" @touchstart="touchStart" @touchend="touchEnd">
  <image v-if="item&&!url" class="short-cover" :src="assetUrl(item.poster)" mode="aspectFill"/>
  <video v-if="url" :id="videoId" :key="url+'-'+source+'-'+episode" class="short-video" :src="url" :title="item?.title" :autoplay="true" :muted="muted" :controls="true" object-fit="contain" :show-fullscreen-btn="false" @play="playbackStarted" @pause="playing=false" @ended="ended" @error="playbackError"/>
  <view class="short-gradient"/>
  <view class="short-tabs"><text @click="emit('navigate','history')">看过</text><text class="active">短剧</text><text v-if="ranking" @click="emit('navigate','ranking')">排行榜</text><text @click="emit('navigate','category')">剧场</text><view role="button" aria-label="搜索短剧" @click="emit('navigate','search')"><AppIcon name="search" :size="23" color="#ffffff"/></view></view>
  <view v-if="item&&!url" class="short-center" role="button" aria-label="播放短剧" @click="playSelected"><AppIcon name="play" :size="48" color="#ffffff"/></view>
  <view v-if="item" class="short-actions">
   <view role="button" aria-label="收藏短剧" @click="emit('favorite')"><AppIcon name="favorite" :size="29" :color="favorite?'#ffe08c':'#ffffff'"/><text>{{favorite?'已收藏':'收藏'}}</text></view>
   <view v-if="url" role="button" aria-label="切换短剧声音" @click="muted=!muted"><text class="short-action-label">{{muted?'静音':'有声'}}</text></view>
   <view v-if="url" role="button" aria-label="切换短剧播放状态" @click="togglePlayback"><text class="short-action-label">{{playing?'暂停':'播放'}}</text></view>
   <view role="button" aria-label="下一部短剧" @click="emit('step',1)"><AppIcon name="chevron" :size="29" color="#ffffff"/><text>下一部</text></view>
  </view>
  <AppEmptyState v-if="!item&&!loading" class="short-empty" :title="bound?'暂无短剧内容':'尚未绑定短剧分类'" :description="bound?'暂时没有可观看的短剧，稍后再来看看':'请在页面编排中为短剧入口绑定内容分类。'"/>
  <view v-if="pending||loading" class="short-message">正在加载…</view>
  <view v-if="error" class="short-error"><text>{{error}}</text><text @click="playSelected">重试</text></view>
  <view v-if="item" class="short-overlay"><text class="short-author">{{item.title}}</text><text class="short-description">{{item.description||'查看分集，继续观看'}}</text><view class="short-episodes" role="button" aria-label="短剧选集" @click="chooseEpisodes"><text>▶ {{url?episodes[episode]?.name:item.note||'观看 / 选集'}}</text><text>选集 ›</text></view></view>
  <view v-if="picker" class="short-picker-mask" @click="picker=false"><view class="short-picker" role="dialog" aria-label="短剧选集面板" @click.stop><view class="picker-heading"><text>{{item?.title}}</text><text role="button" aria-label="关闭短剧选集" @click="picker=false">关闭</text></view><scroll-view scroll-y class="picker-scroll"><view class="short-source-chips"><text v-for="(line,index) in detail?.sources" :key="index" :class="{chosen:source===index}" @click="chooseSource(index)">{{line.name}}</text></view><view class="short-episode-chips"><text v-for="(row,index) in episodes" :key="index" role="button" :aria-label="'播放短剧'+row.name" :class="{chosen:episode===index}" @click="chooseEpisode(index)">{{row.name}}</text></view><text v-if="!episodes.length">当前线路没有可播放集数。</text></scroll-view></view></view>
 </view>
</template>
<style scoped>
.shortdrama{flex:1;min-height:450px;height:100%;position:relative;overflow:hidden;background:#111;color:#fff}
.short-cover,.short-video,.short-gradient{position:absolute;inset:0;width:100%;height:100%}.short-gradient{background:linear-gradient(#0008,transparent 26%,transparent 54%,#000b);pointer-events:none}
.short-tabs{position:absolute;top:13px;left:8px;right:9px;display:flex;align-items:center;gap:13px;font-size:12px;color:#ffffffb3;z-index:2}.short-tabs>text,.short-tabs>view{cursor:pointer}.short-tabs .active{font-size:14px;color:#fff;font-weight:600}.short-tabs>view{margin-left:auto}
.short-center{position:absolute;left:50%;top:46%;transform:translate(-50%,-50%);cursor:pointer;opacity:.8}.short-actions{position:absolute;right:12px;bottom:125px;display:flex;flex-direction:column;gap:18px}.short-actions>view{display:flex;flex-direction:column;align-items:center;gap:5px;cursor:pointer}.short-actions text{font-size:9px}.short-actions .short-action-label{font-size:11px;padding:6px 4px;background:#0005;border-radius:3px}
.short-overlay{position:absolute;left:8px;right:48px;bottom:46px;display:flex;flex-direction:column;gap:7px;pointer-events:none}.short-author{font-size:12px;font-weight:600}.short-description{font-size:10px;line-height:16px;display:-webkit-box;-webkit-line-clamp:2;-webkit-box-orient:vertical;overflow:hidden}.short-episodes{display:flex;justify-content:space-between;padding:8px;background:#ffffff20;font-size:10px;border-radius:3px;pointer-events:auto;cursor:pointer}
.short-empty{position:absolute;left:10px;right:10px;top:30%}.short-message{position:absolute;top:45%;width:100%;text-align:center;font-size:12px;pointer-events:none}.short-error{position:absolute;top:52%;left:24px;right:24px;display:flex;flex-direction:column;align-items:center;gap:12px;font-size:12px;background:#000b;padding:14px;border-radius:3px}.short-error>text:last-child{color:#45d4ef;cursor:pointer}
.short-picker-mask{position:absolute;inset:0;background:#0008;z-index:5;display:flex;align-items:flex-end}.short-picker{width:100%;max-height:65%;display:flex;flex-direction:column;background:#191919;border-radius:10px 10px 0 0;padding:16px 12px;box-sizing:border-box}.picker-heading{display:flex;justify-content:space-between;font-size:13px;gap:12px;margin-bottom:15px}.picker-heading>text:last-child{color:#bbb;cursor:pointer}.picker-scroll{max-height:280px}.short-source-chips,.short-episode-chips{display:flex;flex-wrap:wrap;gap:8px;margin-bottom:16px}.short-source-chips>text,.short-episode-chips>text{background:#ffffff0d;font-size:12px;padding:10px 13px;border-radius:3px;cursor:pointer}.chosen{color:#46d7ec}
</style>
