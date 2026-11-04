<script setup lang="ts">
import {computed,onBeforeUnmount,ref,watch} from 'vue';
import type {Platform,Vod} from '@/domain/types';
import {communityApi,type ManagementRow} from '@/services/management';
import {applicationRuntime,danmuRuntime} from '@/services/application-runtime';
import type {CaptchaInput} from '@/services/application';
import CaptchaField from './CaptchaField.vue';
import {danmuMilliseconds,danmuSeconds} from '@/domain/playback-session';
type Kind='comments'|'danmu';
const props=defineProps<{film:Vod;episode:number;position:number;enabled?:boolean;demo?:boolean;compact?:boolean;platform?:Platform}>();
const emit=defineEmits<{danmu:[value:{text:string;color:string;time:number}[]]}>();
const tab=ref<Kind>('comments'),opened=ref(!props.compact),rows=ref<ManagementRow[]>([]),danmu=ref<ManagementRow[]>([]),third=ref<{content:string;position:number}[]>([]);
const paging=ref({comments:{page:1,pages:1,loading:false,error:''},danmu:{page:1,pages:1,loading:false,error:''}}),thirdError=ref('');
const busy=ref(false),text=ref(''),message=ref(''),captcha=ref<CaptchaInput>({}),captchaReady=ref(false),captchaRef=ref<InstanceType<typeof CaptchaField>>();
let generation=0;const listing={comments:0,danmu:0};
const allowed=computed(()=>!!props.enabled&&!props.demo),commentsOn=computed(()=>allowed.value&&applicationRuntime.comments.enabled),danmuOn=computed(()=>allowed.value&&danmuRuntime.enabled);
const activeRows=computed(()=>tab.value==='danmu'?danmu.value:rows.value),page=computed(()=>paging.value[tab.value].page),pages=computed(()=>paging.value[tab.value].pages),loading=computed(()=>paging.value[tab.value].loading);
const readError=computed(()=>paging.value[tab.value].error||(tab.value==='danmu'?thirdError.value:''));
const nativeDanmu=computed(()=>!danmuOn.value?[]:[...danmu.value,...third.value].filter((_,i)=>props.platform!=='tv'||(i*37%100)<danmuRuntime.density).map(r=>({text:r.content||'',color:'#ffffff',time:danmuSeconds(Number(r.position))})));
watch(nativeDanmu,v=>emit('danmu',v),{immediate:true});
async function loadKind(kind:Kind,reset=true){
 if(!allowed.value||(kind==='comments'?!commentsOn.value:!danmuOn.value))return;
 const seq=++listing[kind],owner=generation,id=props.film.id,episode=props.episode,state=paging.value[kind],target=reset?1:state.page+1;
 state.loading=true;state.error='';
 try{const result=await communityApi.list(kind,id,false,target,episode,kind==='danmu'?100:20);if(seq!==listing[kind]||owner!==generation)return;
  const existing=reset?[]:kind==='danmu'?danmu.value:rows.value;
  const items=[...new Map([...existing,...result.items].map(r=>[r.id,r])).values()];
  if(kind==='danmu')danmu.value=items;else rows.value=items;state.page=result.page;state.pages=result.pages;
 }catch(e){if(seq===listing[kind]&&owner===generation)state.error=(e as Error).message;}finally{if(seq===listing[kind]&&owner===generation)state.loading=false;}
}
function load(reset=true){return loadKind(tab.value,reset);}
async function bootstrap(){
 const owner=++generation;listing.comments++;listing.danmu++;
 rows.value=[];danmu.value=[];third.value=[];text.value='';message.value='';thirdError.value='';busy.value=false;
 paging.value={comments:{page:1,pages:1,loading:false,error:''},danmu:{page:1,pages:1,loading:false,error:''}};
 if(!commentsOn.value)tab.value='danmu';else if(!danmuOn.value)tab.value='comments';
 if(!allowed.value)return;
 if(commentsOn.value)void loadKind('comments');if(danmuOn.value)void loadKind('danmu');
 if(danmuOn.value&&danmuRuntime.thirdParty){try{const result=await communityApi.thirdParty(props.film.id,props.episode);if(owner===generation)third.value=result.items;}catch(e){if(owner===generation)thirdError.value='第三方弹幕：'+(e as Error).message;}}
}
watch(()=>[props.film.id,props.episode,allowed.value,commentsOn.value,danmuOn.value,danmuRuntime.thirdParty],()=>void bootstrap(),{immediate:true});
function selectTab(kind:Kind){if(!busy.value)tab.value=kind;}
watch(tab,()=>{text.value='';message.value='';});onBeforeUnmount(()=>{generation++;listing.comments++;listing.danmu++;});
async function submit(){
 if(!text.value.trim()||busy.value||!captchaReady.value)return;
 const owner=generation,kind=tab.value;busy.value=true;message.value='';
 try{const row=await communityApi.submit(kind,text.value.trim(),props.film.id,kind==='danmu'?danmuMilliseconds(props.position):0,props.episode,captcha.value);if(owner!==generation)return;
  text.value='';message.value=row.approved?'已发送。':'已提交，审核通过后显示。';await loadKind(kind);
 }catch(e){if(owner===generation)message.value=(e as Error).message;}finally{if(owner===generation){busy.value=false;void captchaRef.value?.refresh();}}
}
async function report(row:ManagementRow){if(!row.id||busy.value)return;const owner=generation,kind=tab.value;busy.value=true;try{await communityApi.report(kind,row.id);if(owner===generation)message.value='举报已提交。';}catch(e){if(owner===generation)message.value=(e as Error).message;}finally{if(owner===generation)busy.value=false;}}
</script>
<template><view v-if="commentsOn||danmuOn" class="community" :class="{'community-compact':compact}"><button v-if="compact" class="community-open" @click="opened=!opened">{{opened?'收起互动':'评论 / 弹幕'}}</button><view v-if="opened" class="community-body"><view class="community-tabs"><text v-if="commentsOn" role="button" :class="{active:tab==='comments'}" @click="selectTab('comments')">影片评论</text><text v-if="danmuOn" role="button" :class="{active:tab==='danmu'}" @click="selectTab('danmu')">第 {{episode}} 集弹幕</text></view><text v-if="readError" class="community-error" role="button" @click="bootstrap">{{readError}} · 重试</text><view class="community-list"><view v-for="row in activeRows" :key="row.id" class="community-row"><view class="community-meta"><text>{{row.pinned?'置顶 · ':''}}{{row.nickname||'用户'}}</text><text v-if="row.id" role="button" @click="report(row)">举报</text></view><text>{{row.content}}</text><text v-if="row.reply" class="community-reply">回复：{{row.reply}}</text></view><text v-if="!activeRows.length&&!loading">{{tab==='danmu'?'本集暂无站内弹幕':'暂无评论'}}</text></view><button v-if="page<pages" :disabled="loading" @click="load(false)">加载更多</button><text v-if="loading">读取中…</text><view class="community-compose"><input v-model="text" :maxlength="tab==='danmu'?100:1000" :placeholder="tab==='danmu'?'在当前进度发送弹幕':'说说你对这部影片的看法'" :disabled="busy"/><CaptchaField ref="captchaRef" v-model="captcha" :action="tab" :disabled="busy" @ready="captchaReady=$event"/><button :disabled="busy||!text.trim()||!captchaReady" @click="submit">{{busy?'发送中…':'发送'}}</button><text v-if="message">{{message}}</text></view></view></view></template>
<style scoped>.community{padding:18px 0;font-size:12px;line-height:1.6}.community-tabs{display:flex;gap:20px;border-bottom:1px solid #80808030;padding-bottom:12px;margin-bottom:14px;color:#9399a2}.community-tabs text,.community-meta text:last-child{cursor:pointer}.community-tabs .active{color:inherit;font-weight:700}.community-list{max-height:260px;overflow:auto;display:flex;flex-direction:column;gap:15px}.community-row{display:flex;flex-direction:column;gap:6px;padding-bottom:12px;border-bottom:1px solid #80808020;overflow-wrap:anywhere}.community-meta{display:flex;justify-content:space-between;color:#9399a2;font-size:11px;gap:14px}.community-reply{color:#9aa8c0}.community-compose{display:flex;flex-direction:column;gap:10px;margin-top:16px}.community-compose input{height:40px;padding:0 12px;border:1px solid #80808040;border-radius:6px;box-sizing:border-box}.community button{font-size:12px;background:#80808025;color:inherit;border-radius:6px;padding:7px 14px;margin:0}.community button[disabled]{opacity:.45}.community-error{color:#e99b91;cursor:pointer}.community-compact{padding:0}.community-compact .community-body{background:#191919f5;padding:16px;border-radius:8px;max-height:55vh;overflow:auto}.community-open{width:max-content;margin-left:auto!important}</style>
