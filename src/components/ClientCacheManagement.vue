<script setup lang="ts">
import {computed,onMounted,onUnmounted,ref} from 'vue';
import {clientCacheApi,type ClientCacheKind,type ClientCacheStatus} from '@/services/api';
import type {Platform} from '@/domain/types';
const props=defineProps<{platform:Platform}>();
const names:Record<Platform,string>={mobile:'手机客户端',tablet:'平板客户端',tv:'电视客户端',web:'电脑客户端'};
const name=computed(()=>names[props.platform]);
const cards:{kind:ClientCacheKind;title:string;icon:string;hint:string;scope:string}[]=[
 {kind:'config',title:'配置缓存',icon:'⚙',hint:'已发布的客户端运营配置',scope:'功能开关、页面内容、轮播、公告与版本配置。'},
 {kind:'users',title:'用户缓存',icon:'♙',hint:'会员资料的短时展示缓存',scope:'会员资料与会员名称；清理后重新读取，不退出登录。'},
 {kind:'catalog',title:'视频缓存',icon:'▷',hint:'内容源的目录查询缓存',scope:'分类、影片列表、详情与内容源搜索结果。'}
];
const status=ref<ClientCacheStatus|null>(null),loading=ref(false),action=ref(''),error=ref(''),message=ref('');
let alive=true,timer:ReturnType<typeof setTimeout>|undefined;
const search=computed(()=>status.value?.search);
const busy=computed(()=>loading.value||!!action.value||search.value?.phase==='running');
const indexLabel=computed(()=>search.value?.phase==='running'?'正在重建':search.value?.ready?'索引可用':search.value?.phase==='failed'?'重建失败':search.value?.builtAt?'索引已过期':'尚未建立');
function schedule(){if(timer)clearTimeout(timer);if(alive&&search.value?.phase==='running')timer=setTimeout(()=>void load(true),1600);}
async function load(silent=false){
 if(loading.value||action.value)return;loading.value=true;if(!silent)error.value='';
 try{const result=await clientCacheApi.status(props.platform);if(alive){status.value=result;error.value='';}}
 catch(e){if(alive){status.value=null;error.value=(e as Error).message;}}
 finally{if(alive){loading.value=false;schedule();}}
}
function count(kind:ClientCacheKind){const v=status.value?.kinds?.[kind];return status.value?.available&&v?String(v.entries)+(v.countComplete?'':'+'):'—';}
function ttl(kind:ClientCacheKind){return status.value?.available&&status.value.kinds?.[kind]?status.value.kinds[kind].ttlSeconds+' 秒':'—';}
async function clearCache(kind:ClientCacheKind){
 if(busy.value)return;action.value=kind;error.value='';message.value='';
 try{const result=await clientCacheApi.clear(props.platform,kind);if(!alive)return;
 message.value=cards.find(v=>v.kind===kind)!.title+'已清理 · 删除 '+result.deleted+' 条。'+(!result.cleanupComplete?'旧条目将在过期后回收。':'')+(!result.auditRecorded?'清理已生效，审计日志写入失败。':'');
 status.value=await clientCacheApi.status(props.platform);
 }catch(e){if(alive){status.value=null;error.value=(e as Error).message;}}
 finally{if(alive){action.value='';schedule();}}
}
function confirmClear(kind:ClientCacheKind){if(!status.value?.available||busy.value)return;uni.showModal({title:'清理'+cards.find(v=>v.kind===kind)!.title,content:'仅清理'+name.value+'的此类服务端缓存，下一次访问自动重新生成。不会删除业务数据。',success:answer=>{if(answer.confirm&&alive)void clearCache(kind);}});}
async function rebuild(){
 if(busy.value||!search.value?.sourceConfigured)return;action.value='search';error.value='';message.value='';
 try{await clientCacheApi.rebuild(props.platform);if(alive)status.value=await clientCacheApi.status(props.platform);}
 catch(e){if(alive)error.value=(e as Error).message;}
 finally{if(alive){action.value='';schedule();}}
}
function stamp(value?:string){return value?new Date(value).toLocaleString('zh-CN',{hour12:false}):'—';}
onMounted(()=>void load());onUnmounted(()=>{alive=false;if(timer)clearTimeout(timer);});
</script>
<template>
 <view class="cache-page" aria-label="客户端缓存管理">
  <view class="cache-heading"><view><text class="eyebrow">CACHE MANAGEMENT</text><text class="title">缓存管理</text><text class="subtitle">{{name}} <text class="separator">/</text> 独立管理 · 按需清理</text></view><button class="refresh" :disabled="loading||!!action" @click="load()">{{loading?'检测中…':'↻ 刷新状态'}}</button></view>
  <view v-if="error" class="notice error" role="alert">{{error}}</view><view v-if="message" class="notice success" role="status">{{message}}</view>
  <view class="status-bar"><view class="connection"><text class="dot" :class="{ready:status?.available}"/><text>{{loading&&!status?'正在检测缓存服务':status?.available?'Redis 连接正常':status?.configured?'Redis 连接异常':'Redis 未配置'}}</text><text class="status-divider">|</text><text>当前客户端独立清理</text></view><text>更新于 {{stamp(status?.checkedAt)}}</text></view>
  <view class="cache-grid"><view v-for="card in cards" :key="card.kind" class="cache-card" :class="card.kind">
   <view class="card-heading"><view class="icon">{{card.icon}}</view><view><text class="card-title">{{card.title}}</text><text class="card-meta">{{card.hint}}</text></view></view>
   <text class="description">{{card.scope}}</text>
   <view class="metrics"><view><text class="metric">{{count(card.kind)}}</text><text class="metric-label">有效条目</text></view><view><text class="ttl">{{ttl(card.kind)}}</text><text class="metric-label">自动过期</text></view></view>
   <button class="clear-button" :disabled="!status?.available||busy" @click="confirmClear(card.kind)">{{action===card.kind?'正在清理…':'清理'+card.title}}</button>
  </view></view>
  <view class="search-panel">
   <view class="search-heading"><view class="card-heading"><view class="icon search">⌕</view><view><text class="card-title">搜索索引</text><text class="card-meta">本客户端独立索引 · 有效期 15 分钟</text></view></view><text class="badge" :class="{ready:search?.ready,failed:search?.phase==='failed'}">{{indexLabel}}</text></view>
   <view class="search-body"><view class="search-details"><text class="description">重建后用于影片搜索；索引过期或服务重启后，自动回到内容源检索。</text><view class="search-stats"><text>已索引 <b>{{search?.indexedDocuments??0}}</b> 部影片</text><text>上次完成 {{stamp(search?.builtAt)}}</text></view><text v-if="search?.phase==='running'||search?.phase==='failed'" class="job-message" :class="{failed:search.phase==='failed'}" role="status">{{search.message}}</text><text v-else-if="!search?.sourceConfigured" class="job-message">请先连接内容源，即可重建索引。</text><text v-else-if="search?.builtAt" class="job-message">有效至 {{stamp(search.expiresAt)}}</text></view><button class="rebuild-button" :disabled="busy||!search?.sourceConfigured" @click="rebuild">{{search?.phase==='running'?'正在重建…':action==='search'?'正在提交…':'重建搜索索引'}}</button></view>
  </view>
  <view class="scope-note"><text>清理仅影响{{name}}的服务端缓存，业务数据完整保留。</text><text>设备下载文件、播放缓存不在此处清理。</text></view>
 </view>
</template>
<style scoped>
.cache-page{padding:4px 0 28px;color:var(--studio-text,#172b4d)}.cache-heading,.card-heading,.status-bar,.connection,.search-heading,.search-body,.search-stats,.scope-note{display:flex;align-items:center}.cache-heading{justify-content:space-between;gap:16px;margin:20px 0}.eyebrow,.title,.subtitle,.card-title,.card-meta,.description,.metric,.metric-label,.ttl,.job-message{display:block}.eyebrow{font-size:10px;letter-spacing:2px;color:var(--studio-muted,#77849c);font-weight:700}.title{font-size:25px;font-weight:650;letter-spacing:-.6px;margin:6px 0 8px}.subtitle,.description{font-size:12px;color:var(--studio-muted,#748099);line-height:1.8}.separator{margin:0 8px;opacity:.45}.refresh,.clear-button,.rebuild-button{margin:0;font-size:12px;font-weight:600;border-radius:8px;line-height:40px;padding:0 18px;white-space:nowrap;transition:background-color .15s,color .15s}.refresh{background:var(--studio-panel,#fff);color:inherit;border:1px solid var(--studio-border,#e4eaf2)}button::after{border:0}.status-bar{justify-content:space-between;gap:12px;padding:12px 16px;border:1px solid var(--studio-border,#e4eaf2);background:var(--studio-panel,#fff);border-radius:9px;font-size:11px;color:var(--studio-muted,#748099);margin-bottom:18px;flex-wrap:wrap}.connection{gap:8px;flex-wrap:wrap}.dot{width:6px;height:6px;background:#94a3b8;border-radius:50%}.dot.ready{background:#19a779;box-shadow:0 0 0 3px #19a77916}.status-divider{opacity:.35;margin:0 4px}.cache-grid{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:18px}.cache-card{padding:22px;background:var(--studio-panel,#fff);border:1px solid var(--studio-border,#e4eaf2);border-radius:12px;display:flex;flex-direction:column;min-width:0}.card-heading{gap:10px}.icon{display:flex;align-items:center;justify-content:center;width:36px;height:36px;border-radius:9px;font-size:22px;flex-shrink:0;color:#3978e8;background:#3978e810}.users .icon{color:#9865c5;background:#9865c510}.catalog .icon{color:#159b7a;background:#159b7a10}.card-title{font-size:15px;font-weight:650;line-height:1.5}.card-meta{font-size:10px;color:var(--studio-muted,#748099);margin-top:3px}.cache-card .description{margin:17px 0 19px;min-height:44px}.metrics{display:grid;grid-template-columns:1fr 1fr;align-items:end;gap:12px;margin-bottom:20px}.metric{font-size:28px;line-height:1.2;font-weight:650;font-variant-numeric:tabular-nums;letter-spacing:-1px}.ttl{font-size:15px;font-weight:600;line-height:1.8}.metric-label{font-size:10px;color:var(--studio-muted,#748099);margin-top:6px}.clear-button{margin-top:auto;width:100%;border:1px solid var(--studio-border,#e4eaf2);background:var(--studio-panel,#fff);color:var(--studio-text,#243a5e)}.clear-button:not([disabled]):hover,.refresh:not([disabled]):hover{background:var(--studio-hover,#f1f5fb);border-color:#7299db}.clear-button[disabled],.rebuild-button[disabled]{background:var(--studio-hover,#94a3b80a);color:var(--studio-muted,#748099);opacity:.65}.search-panel{margin-top:18px;border:1px solid var(--studio-border,#e4eaf2);border-radius:12px;padding:22px;background:var(--studio-panel,#fff)}.search-heading{justify-content:space-between;gap:12px}.search-body{gap:24px;margin-top:18px;padding-top:18px;border-top:1px solid var(--studio-border,#e4eaf2)}.search-details{flex:1;min-width:0}.icon.search{color:#b4873b;background:#b4873b10}.search-stats{gap:20px;margin-top:12px;font-size:11px;color:var(--studio-muted,#748099);flex-wrap:wrap}.search-stats b{font-weight:650;color:var(--studio-text,#243a5e);padding:0 3px}.badge{font-size:10px;padding:4px 9px;line-height:1.6;border-radius:5px;background:var(--studio-hover,#94a3b814);color:var(--studio-muted,#748099);white-space:nowrap}.badge.ready{color:#159b7a;background:#159b7a12}.badge.failed,.job-message.failed{color:#c45555}.rebuild-button{background:#2864e9;color:#fff;min-width:140px}.rebuild-button:not([disabled]):hover{background:#1d56cc}.job-message{font-size:11px;color:var(--studio-muted,#748099);margin-top:10px;line-height:1.8;overflow-wrap:anywhere}.scope-note{justify-content:space-between;gap:12px;flex-wrap:wrap;font-size:10px;color:var(--studio-muted,#748099);margin-top:16px;line-height:1.8}.notice{padding:11px 15px;border-radius:8px;margin-bottom:14px;font-size:12px;line-height:1.8}.error{color:#c45555;background:#c4555510}.success{color:#159b7a;background:#159b7a10}button:focus-visible{outline:3px solid #3777eb70;outline-offset:3px}@media(max-width:1100px){.cache-grid{gap:12px}.cache-card{padding:18px}.card-meta{font-size:10px}}@media(max-width:860px){.cache-grid{grid-template-columns:1fr}.search-body{align-items:flex-start;flex-direction:column;gap:16px}.search-panel{padding:18px}.cache-heading{align-items:flex-start}.cache-card .description{min-height:0}.metrics{grid-template-columns:1fr 1fr}.scope-note{display:block}.scope-note text{display:block}}@media(prefers-reduced-motion:reduce){button{transition:none}}
</style>
