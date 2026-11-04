<script setup lang="ts">
import {ref,computed,watch,onUnmounted} from 'vue';
import {onLoad} from '@dcloudio/uni-app';
import {api,assetUrl,captureMemberContext,getServiceBase,memberApi} from '@/services/api';
import {getLibrary,toggleFavorite,getHistory,getHistoryScope} from '@/services/library';
import {demoVods} from '@/domain/demo';
import type {Vod,Platform} from '@/domain/types';
import {playbackSourceId,resumeEpisode,type PlayerDanmu} from '@/domain/playback-session';
import {historyMember} from '@/services/member-session';
import {danmuRuntime} from '@/services/application-runtime';
import PlaybackVideo from '@/components/PlaybackVideo.vue';
import CommunityPanel from '@/components/CommunityPanel.vue';
const film=ref<Vod>(),error=ref(''),loading=ref(true),source=ref(0),episode=ref(0),playUrl=ref(''),favorite=ref(false),accent=ref('#ccf279');
const trialSeconds=ref(0),position=ref(0),playRevision=ref(0),playerDanmu=ref<PlayerDanmu[]>([]);
let id='',demo=false,demoReturn='/pages/studio/index',alive=true,generation=0;
let platform:Platform='web';
// #ifdef APP-PLUS
platform='mobile';
// #endif
const episodes=computed(()=>film.value?.sources[source.value]?.episodes||[]);
function clearPlayback(){generation++;playUrl.value='';position.value=0;playerDanmu.value=[];loading.value=false;}
async function load(){
 clearPlayback();const token=++generation;loading.value=true;error.value='';film.value=undefined;source.value=0;episode.value=0;
 try{
  const theme=await api.theme(platform);if(!alive||token!==generation)return;
  if(!demo&&uni.getStorageSync('hetun-member-token')&&!historyMember(getServiceBase())){try{await memberApi.profile();}catch{/* Public playback remains available when a session expires. */}}
  if(!alive||token!==generation)return;const current=captureMemberContext();
  const result=demo?{items:demoVods.filter(v=>v.id===id)}:await api.catalog(1,'','',id);
  if(!alive||token!==generation)return;current();
  const item=result.items.find(v=>v.id===id);if(!item)throw new Error(demo?'示例影片不存在。':'影片不存在或已下架。');
  film.value=item;accent.value=theme.theme.colors.accent;favorite.value=getLibrary().some(v=>v.id===id);episode.value=resumeEpisode(item,getHistory());
 }catch(e){if(alive&&token===generation)error.value=(e as Error).message;}finally{if(alive&&token===generation)loading.value=false;}
}
function back(){clearPlayback();if(demo){uni.redirectTo({url:demoReturn});return;}let fallback='/pages/index/index';
// #ifdef H5
fallback='/pages/studio/index';
// #endif
uni.navigateBack({fail:()=>uni.reLaunch({url:fallback})});}
function favoriteToggle(){if(film.value)favorite.value=toggleFavorite(film.value);}
function choose(index:number){if(!episodes.value[index])return;clearPlayback();episode.value=index;}
function chooseSource(index:number){if(!film.value?.sources[index])return;clearPlayback();source.value=index;episode.value=0;}
async function playFilm(){
 if(demo){error.value='原创设计示例。连接 CMS 后可播放真实影片。';return;}
 if(!film.value||!episodes.value[episode.value]){error.value='当前线路没有集数。';return;}
 clearPlayback();const token=++generation,current=captureMemberContext(),item=film.value,sid=source.value,nid=episode.value;loading.value=true;error.value='';
 try{const result=await api.playback(item.id,playbackSourceId(item,sid),nid+1);if(!alive||token!==generation||film.value?.id!==item.id)return;current();
  if(!/^https?:\/\//i.test(result.url))throw new Error('播放地址不受支持。');trialSeconds.value=result.trialSeconds||0;playRevision.value++;playUrl.value=result.url;
 }catch(e){if(alive&&token===generation)error.value=(e as Error).message;}finally{if(alive&&token===generation)loading.value=false;}
}
function nextEpisode(){if(episode.value+1<episodes.value.length){choose(episode.value+1);void playFilm();}}
function playerError(){error.value='视频加载失败，请检查播放线路或更换线路。';}
watch(()=>getHistoryScope(),()=>{clearPlayback();film.value=undefined;if(id)void load();},{flush:'sync'});
onUnmounted(()=>{alive=false;generation++;});
onLoad(options=>{id=String(options?.id||'');demo=options?.demo==='1';const target=String(options?.returnTo||'');if(['/pages/tv/index','/pages/demo/index?device=mobile','/pages/demo/index?device=tablet','/pages/demo/index?device=web'].includes(target))demoReturn=target;void load();});
</script>
<template><view class="detail" :style="{'--accent':accent}"><view class="detail-top"><text @click="back">← 返回</text><text>河豚 · 放映室</text></view><view v-if="loading" class="message">正在读取影片…</view><view v-if="film" class="detail-content"><PlaybackVideo v-if="playUrl" :key="playRevision" class="video-area" :film="film" :src="playUrl" :episode="episode+1" :trial-seconds="trialSeconds" :danmu-enabled="!demo&&danmuRuntime.enabled" :danmu="playerDanmu" @progress="position=$event" @ended="nextEpisode" @expired="error=$event" @error="playerError"/><view class="film-intro"><image v-if="film.poster" class="cover" :src="assetUrl(film.poster)" mode="aspectFill"/><view><text class="title">{{film.title}}</text><text class="meta">{{film.score}} · {{film.year}} · {{film.category}} · {{film.area}}</text><text class="note">{{film.note}}</text><view class="actions"><button class="watch" :disabled="loading" @click="playFilm">▶ {{playUrl?'正在播放':'立即观看'}}</button><button class="favorite" @click="favoriteToggle">{{favorite?'✓ 已加入片单':'+ 加入片单'}}</button></view><text class="actors" v-if="film.actors">主演：{{film.actors}}</text><text class="actors" v-if="film.director">导演：{{film.director}}</text></view></view><text class="heading">剧情简介</text><text class="description">{{film.description||'暂无简介。'}}</text><text class="heading">播放线路</text><view class="source-list"><button v-for="(s,index) in film.sources" :key="index" :class="{active:source===index}" @click="chooseSource(index)">{{s.name}}</button></view><text v-if="!film.sources.length" class="message">这部影片暂无播放资源。</text><view class="episode-list"><button v-for="(e,index) in episodes" :key="index" :class="{active:episode===index}" @click="choose(index)">{{e.name}}</button></view><CommunityPanel :film="film" :episode="episode+1" :position="position" :enabled="!demo" :demo="demo" :platform="platform" @danmu="playerDanmu=$event"/></view><view v-if="error" class="error"><text>{{error}}</text><button v-if="!film" @click="load">重新加载</button></view></view></template>
<style scoped>.detail{min-height:100vh;background:#0b1115;padding:0 6vw 70px;color:#eef2e9}.detail-top{height:84px;display:flex;align-items:center;justify-content:space-between;border-bottom:1px solid #ffffff12;font-size:13px;color:#9bab9d}.detail-top text:first-child{cursor:pointer}.detail-content{max-width:1000px;margin:35px auto}.film-intro{display:flex;gap:35px;align-items:center}.cover{width:190px;height:270px;border-radius:14px;flex-shrink:0}.title{font-size:35px;font-weight:700;display:block}.meta{font-size:12px;color:var(--accent);display:block;margin-top:20px}.note{display:block;color:#93a196;font-size:12px;margin-top:12px}.actions{display:flex;gap:14px;margin:23px 0}.actions button{padding:12px 22px;border-radius:7px;font-size:12px}.watch{background:var(--accent);color:#182518}.favorite{background:#1a2826;color:#ccd4ca;border:1px solid #ffffff14}.actors{display:block;font-size:11px;color:#84978a;line-height:1.9}.heading{display:block;font-size:20px;font-weight:700;margin:32px 0 18px}.description{font-size:13px;color:#9ea99f;line-height:2}.source-list,.episode-list{display:flex;flex-wrap:wrap;gap:12px}.source-list button,.episode-list button{padding:10px 17px;background:#182224;color:#9ead9f;border-radius:6px;font-size:12px;border:1px solid #ffffff10}.source-list button.active,.episode-list button.active{color:var(--accent);border-color:var(--accent)}.episode-list{margin-top:20px}.video-area{margin-bottom:30px;width:100%;height:480px;background:#000}.error{padding:20px;max-width:1000px;margin:30px auto;background:#c6615015;color:#eaa897;border-radius:10px;font-size:13px;line-height:1.9}.error button{margin-top:15px;display:inline-block;padding:8px 15px}.message{display:block;color:#8fa18f;padding:20px 0;font-size:13px}@media(max-width:600px){.detail{padding:0 20px 45px}.film-intro{gap:18px;align-items:flex-start}.cover{width:115px;height:175px}.title{font-size:23px}.actions{flex-direction:column;gap:8px;margin:14px 0}.actions button{padding:8px 10px;font-size:10px}.meta{font-size:10px;margin-top:13px}.video-area{height:240px}.actors{font-size:10px}}
</style>
