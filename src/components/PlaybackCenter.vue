<script setup lang="ts">
import {onBeforeUnmount,ref,computed,onMounted,watch} from 'vue';
import EditorChoice from './EditorChoice.vue';
import PlaybackResolvers from './PlaybackResolvers.vue';
import PlaybackEngineManagement from './PlaybackEngineManagement.vue';
import ContentManagement from './ContentManagement.vue';
import {playbackCenterApi,type PlaybackPreferences} from '@/services/application';
import {managementApi,type ModerationPolicy} from '@/services/management';
import {useSharedAutosave} from '@/composables/useSharedAutosave';
const emit=defineEmits<{(e:'dirty',value:boolean):void}>();
const tab=ref('sources'),visited=ref(['sources']);
const tabs=[{id:'sources',label:'播放源与解析'},{id:'engines',label:'播放内核'},{id:'library',label:'弹幕库'},{id:'danmu',label:'弹幕设置'}];
const resolverDirty=ref(false),engineDirty=ref(false),importing=ref(false),testing=ref(false),actionError=ref(''),notice=ref(''),testVideo=ref(''),testEpisode=ref(1),testResult=ref('');
const policy=ref<ModerationPolicy|null>(null),policyBusy=ref(false);
const autosave=useSharedAutosave<PlaybackPreferences>({get:playbackCenterApi.get,save:playbackCenterApi.save,signature:v=>JSON.stringify({source:v.source,sources:v.sources,danmu:v.danmu}),validate:v=>{
 if(v.sources.some(s=>!s.name.trim()||s.name.length>60||s.trialSeconds<0||s.trialSeconds>3600))return '播放源名称不能为空，试看时长为 0–3600 秒';
 if(v.danmu.thirdParty&&!v.danmu.endpoint.trim())return '请先填写第三方弹幕接口地址';
 if(v.danmu.tvDensity<1||v.danmu.tvDensity>100)return '电视端弹幕密度为 1–100';return '';
}});
const {draft,loading,saving,error,conflict,dirty,status,load,flush,reload}=autosave;
const checked=(e:any)=>!!e.detail.value;
const anyDirty=computed(()=>dirty.value||resolverDirty.value||engineDirty.value);
watch(anyDirty,v=>emit('dirty',v),{immediate:true});
function choose(id:string){tab.value=id;if(!visited.value.includes(id))visited.value.push(id);}
function move(i:number,delta:number){if(!draft.value)return;const rows=draft.value.sources,j=i+delta;if(j<0||j>=rows.length)return;[rows[i],rows[j]]=[rows[j],rows[i]];}
async function importSources(){if(!draft.value||importing.value)return;importing.value=true;actionError.value='';notice.value='';try{const v=await playbackCenterApi.sources();if(v.source&&v.source!==draft.value.source)throw new Error('CMS 内容源已更改，请重新读取配置。');let added=0;for(const s of v.items){if(draft.value.sources.some(row=>row.code===s.code))continue;draft.value.sources.push({code:s.code,name:s.name,enabled:s.enabled!==false,trialSeconds:0,mode:'auto'});added++;}notice.value=added?'已导入 '+added+' 个播放源。':'CMS 播放源已全部导入。';}catch(e){actionError.value=(e as Error).message;}finally{importing.value=false;}}
async function reviewDanmu(e:any){if(!policy.value||policyBusy.value)return;policyBusy.value=true;actionError.value='';try{policy.value=await managementApi.savePolicy({...policy.value,reviewDanmu:checked(e)});}catch(e){actionError.value=(e as Error).message;try{policy.value=await managementApi.policy();}catch{policy.value=null;}}finally{policyBusy.value=false;}}
async function test(){if(!draft.value)return;testing.value=true;actionError.value='';testResult.value='';try{const v=await playbackCenterApi.test(JSON.parse(JSON.stringify(draft.value)),testVideo.value.trim(),Number(testEpisode.value));testResult.value='读取成功，共 '+v.count+' 条弹幕。'+(v.items.length?'\n'+v.items.map(r=>(r.position/1000).toFixed(1)+' 秒 · '+r.content).join('\n'):'');}catch(e){actionError.value=(e as Error).message;}finally{testing.value=false;}}
async function prepareToLeave(){if(!await autosave.prepareToLeave())return false;if(!resolverDirty.value&&!engineDirty.value)return true;return new Promise<boolean>(resolve=>uni.showModal({title:'播放配置尚未保存',content:'解析调度或播放内核还有未保存的草稿，离开会放弃修改。',success:r=>resolve(!!r.confirm),fail:()=>resolve(false)}));}
onMounted(async()=>{await load();try{policy.value=await managementApi.policy();}catch{policy.value=null;}});
onBeforeUnmount(()=>emit('dirty',false));
defineExpose({prepareToLeave});
</script>
<template>
<view class="shared-settings"><view class="settings-shell">
 <view class="settings-heading"><view><text class="settings-title">播放中心</text><text class="settings-subtitle">统一播放源、解析调度、客户端内核与弹幕，四端共用播放规则。</text></view><view class="settings-heading-actions"><text class="settings-badge" :class="{pending:dirty||saving,failed:!!error}">{{status}}</text><button :disabled="loading||saving" @click="reload">重新读取</button></view></view>
 <view v-if="error" class="settings-feedback error" role="alert"><text>{{error}}</text><button v-if="!conflict" :disabled="saving" @click="flush">重试保存</button><button v-else @click="reload">读取最新配置</button></view>
 <view v-if="actionError" class="settings-feedback error" role="alert">{{actionError}}</view><view v-if="notice" class="settings-feedback" role="status">{{notice}}</view>
 <view class="settings-tabs" role="tablist" aria-label="播放中心分组"><button v-for="item in tabs" :key="item.id" role="tab" :aria-selected="tab===item.id" :class="{active:tab===item.id}" @click="choose(item.id)">{{item.label}}</button></view>
 <view v-if="loading" class="settings-empty">正在读取播放设置…</view>
 <template v-else-if="draft">
 <view v-show="tab==='sources'">
  <view class="settings-card"><view class="settings-heading" style="margin-bottom:8px"><view><text class="settings-card-title">CMS 播放源</text><text class="settings-card-description">控制来源名称、顺序、开关与试看；修改后自动保存。</text></view><button class="settings-primary" :disabled="importing" @click="importSources">{{importing?'正在读取…':'读取 CMS 播放源'}}</button></view>
   <view v-if="!draft.sources.length" class="settings-empty">连接 CMS 管理适配器后读取播放源。未配置时沿用影片的原始播放源。</view>
   <view v-for="(source,i) in draft.sources" :key="source.code" class="settings-source"><text class="order">{{String(i+1).padStart(2,'0')}}</text><label>显示名称<input v-model="source.name" maxlength="60" :aria-label="source.code+'显示名称'"/><text class="code">{{source.code}}</text></label><label>播放方式<EditorChoice v-model="source.mode" :options="[{id:'auto',label:'按解析规则'},{id:'direct',label:'直接播放'}]"/></label><label>试看秒数<input v-model.number="source.trialSeconds" type="number" min="0" max="3600" :aria-label="source.code+'试看秒数'"/></label><switch :checked="source.enabled" @change="source.enabled=checked($event)" :aria-label="'启用 '+source.name"/><view class="settings-inline"><button :disabled="i===0" @click="move(i,-1)">上移</button><button :disabled="i===draft.sources.length-1" @click="move(i,1)">下移</button></view></view>
   <text class="settings-help">试看为 0 时不限制；直链试看由客户端限制播放时长。付费内容访问仍由 CMS 权限控制。</text>
  </view>
  <view class="settings-embedded"><PlaybackResolvers @dirty="resolverDirty=$event"/></view>
 </view>
 <view v-if="visited.includes('engines')" v-show="tab==='engines'" class="settings-embedded"><PlaybackEngineManagement @dirty="engineDirty=$event"/></view>
 <view v-if="visited.includes('library')" v-show="tab==='library'"><ContentManagement kind="danmu"/></view>
 <view v-show="tab==='danmu'">
  <view class="settings-card"><text class="settings-card-title">弹幕显示与审核</text><text class="settings-card-description">弹幕按当前内容源、影片和集数分别保存。</text><view class="settings-row"><view><text class="settings-label">启用弹幕</text><text class="settings-help">同时控制客户端弹幕读取和发送。</text></view><switch :checked="draft.danmu.enabled" @change="draft.danmu.enabled=checked($event)" aria-label="启用弹幕"/></view><view class="settings-row"><view><text class="settings-label">发送后先审核</text><text class="settings-help">{{policy?'审核结果与弹幕库同步。':'请连接内容源后读取审核规则。'}}</text></view><switch :checked="policy?.reviewDanmu||false" :disabled="!policy||policyBusy" @change="reviewDanmu" aria-label="弹幕先审核"/></view><view class="settings-row"><view><text class="settings-label">电视端弹幕密度</text><text class="settings-help">1–100，控制电视端展示比例。</text></view><input class="settings-control narrow" v-model.number="draft.danmu.tvDensity" type="number" min="1" max="100" aria-label="电视端弹幕密度"/></view></view>
  <view class="settings-card"><text class="settings-card-title">第三方弹幕</text><text class="settings-card-description">读取外部弹幕后与本地已审核内容合并。</text><view class="settings-row"><view><text class="settings-label">启用第三方弹幕</text><text class="settings-help">先填写接口，再开启此功能。</text></view><switch :checked="draft.danmu.thirdParty" @change="draft.danmu.thirdParty=checked($event)" aria-label="启用第三方弹幕"/></view><label class="settings-field">弹幕接口地址<input v-model="draft.danmu.endpoint" maxlength="2048" placeholder="https://你的弹幕服务/api/danmu"/></label><view class="settings-grid"><label>影片匹配方式<EditorChoice v-model="draft.danmu.match" :options="[{id:'id',label:'CMS 影片 ID'},{id:'title',label:'影片标题'}]"/></label><label>优先域名<input v-model="draft.danmu.preferredDomain" maxlength="253" placeholder="留空，或填写接口对应的域名"/></label></view><text class="settings-help">接口接收 id 或 title 以及 episode，返回 items 数组，包含 content 和 position（毫秒）。</text><view class="settings-grid"><label>测试影片 ID<input v-model="testVideo" maxlength="20" placeholder="填写 CMS 中真实存在的影片 ID"/></label><label>集数<input v-model.number="testEpisode" type="number" min="1" max="10000"/></label></view><view class="settings-inline" style="margin-top:18px"><button :disabled="testing||!testVideo||!draft.danmu.endpoint" @click="test">{{testing?'正在测试…':'测试弹幕接口'}}</button><text class="settings-help">测试当前填写的配置。</text></view><view v-if="testResult" class="settings-test-result" role="status">{{testResult}}</view></view>
 </view>
 </template>
</view></view>
</template>
<style src="../styles/shared-settings.css"></style>
