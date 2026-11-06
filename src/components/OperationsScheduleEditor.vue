<script setup lang="ts">
import {computed,onUnmounted,ref,watch} from 'vue';
import type {Theme,Category} from '@/domain/types';
import EditorChoice from './EditorChoice.vue';
type ScheduleItem=Theme['schedule'][number];
const props=defineProps<{theme:Theme;categories:Category[];save:()=>Promise<void>}>();
const emit=defineEmits<{dirty:[value:boolean]}>();
const form=ref<ScheduleItem|null>(null),original=ref(''),editing=ref(false),saving=ref(false),error=ref('');
const rows=computed(()=>[...props.theme.schedule].sort((a,b)=>a.date.localeCompare(b.date)||a.title.localeCompare(b.title)));
const options=computed(()=>[{id:'',label:'全部分类'},...props.categories.map(c=>({id:c.id,label:c.name})),...(form.value?.categoryId&&!props.categories.some(c=>c.id===form.value?.categoryId)?[{id:form.value.categoryId,label:'分类 '+form.value.categoryId+'（当前未读取）'}]:[])]);
const dirty=computed(()=>!!form.value&&JSON.stringify(form.value)!==original.value);
watch(dirty,value=>emit('dirty',value),{immediate:true});
function localDate(){const d=new Date();return [d.getFullYear(),String(d.getMonth()+1).padStart(2,'0'),String(d.getDate()).padStart(2,'0')].join('-');}
function categoryName(id?:string){return id?props.categories.find(c=>c.id===id)?.name||'分类 '+id:'全部分类';}
function discard(){form.value=null;original.value='';error.value='';}
async function prepareToLeave(){if(saving.value)return false;if(!dirty.value){discard();return true;}return new Promise<boolean>(resolve=>uni.showModal({title:'排期尚未保存',content:'放弃本次表单输入并离开？',confirmText:'放弃编辑',success:r=>{if(r.confirm)discard();resolve(r.confirm);},fail:()=>resolve(false)}));}
async function edit(item?:ScheduleItem){if(!await prepareToLeave())return;editing.value=!!item;form.value=item?{...item}:{id:'schedule-'+Date.now().toString(36)+'-'+Math.random().toString(36).slice(2,8),title:'',vodId:'',categoryId:'',date:localDate()};original.value=JSON.stringify(form.value);}
async function commit(){
 if(!form.value||saving.value)return;
 const item={...form.value,title:form.value.title.trim(),vodId:form.value.vodId.trim(),categoryId:form.value.categoryId||''};
 if(!item.title){error.value='请填写影片名称。';return;}
 if(!/^\d{4}-\d{2}-\d{2}$/.test(item.date)||!Number.isFinite(Date.parse(item.date+'T00:00:00Z'))||new Date(item.date+'T00:00:00Z').toISOString().slice(0,10)!==item.date){error.value='请选择有效的排期日期。';return;}
 if(item.vodId&&!/^[A-Za-z0-9_-]{1,64}$/.test(item.vodId)){error.value='影片 ID 只能包含字母、数字、下划线和短横线，最多64位。';return;}
 const index=props.theme.schedule.findIndex(row=>row.id===item.id);
 if(index<0&&props.theme.schedule.length>=100){error.value='每个客户端最多保存100条排期。';return;}
 const next=[...props.theme.schedule];if(index<0)next.push(item);else next[index]=item;
 props.theme.schedule=next;saving.value=true;error.value='';
 try{await props.save();discard();}catch(e){error.value=(e as Error).message||'保存失败，请重试。';}finally{saving.value=false;}
}
function remove(item:ScheduleItem){if(saving.value)return;uni.showModal({title:'删除排期',content:'删除“'+item.title+'”的排期？影片内容不会删除。',success:async r=>{if(!r.confirm||saving.value)return;saving.value=true;error.value='';props.theme.schedule=props.theme.schedule.filter(row=>row.id!==item.id);try{await props.save();}catch(e){error.value=(e as Error).message;}finally{saving.value=false;}}});}
onUnmounted(()=>emit('dirty',false));
defineExpose({prepareToLeave});
</script>
<template>
 <view class="schedule-editor">
  <view class="heading"><view><text class="title">排期表管理</text><text class="description">管理当前客户端的影片排期。保存后，已连接的主题骨架将在下次同步时显示。</text></view><button class="primary" :disabled="saving||rows.length>=100" @click="edit()">＋ 添加排期</button></view>
  <text class="description">按日历日期展示，最多100条；填写影片 ID 后可从排期进入影片详情。</text>
  <text v-if="!theme.features.schedule" class="notice">当前独立排期页已关闭，可在「功能开关」中开启；分类中的排期入口按分类导航设置显示。</text>
  <view v-if="form" class="form" :aria-busy="saving">
   <text class="form-title">{{editing?'编辑排期':'添加排期'}}</text>
   <label>影片名称<input v-model="form.title" aria-label="排期影片名称" maxlength="200" :disabled="saving" placeholder="填写影片名称"/></label>
   <view class="fields"><label>排期日期<picker mode="date" :value="form.date" :disabled="saving" @change="form.date=$event.detail.value"><view class="date-choice">{{form.date}}　⌄</view></picker></label><label>所属分类<EditorChoice v-model="form.categoryId" :options="options"/></label></view>
   <label>影片 ID（选填）<input v-model="form.vodId" aria-label="排期影片ID" maxlength="64" :disabled="saving" placeholder="填写苹果 CMS 的影片 ID；未上架可留空"/></label>
   <view class="actions"><button class="primary" :disabled="saving" @click="commit">{{saving?'保存中…':'保存排期'}}</button><button :disabled="saving" @click="discard">取消编辑</button></view>
  </view>
  <text v-if="error" class="error" role="alert">{{error}}</text>
  <view v-if="!rows.length" class="empty">暂无排期，点击「添加排期」录入第一条。</view>
  <view v-for="item in rows" :key="item.id" class="schedule-row"><view class="day">{{item.date}}</view><view class="content"><text class="name">{{item.title}}</text><text class="description">{{categoryName(item.categoryId)}} · {{item.vodId?'影片 '+item.vodId:'未绑定影片'}}</text></view><view class="actions"><button :disabled="saving" @click="edit(item)">编辑</button><button class="danger" :disabled="saving||!!form" @click="remove(item)">删除</button></view></view>
 </view>
</template>
<style scoped>
.schedule-editor{display:flex;flex-direction:column;gap:18px;padding:24px;color:var(--studio-text,#d8e3df)}.heading{display:flex;justify-content:space-between;align-items:center;gap:20px}.heading>view,.content{display:flex;flex-direction:column;gap:8px;min-width:0}.title{font-size:23px;font-weight:700}.description{font-size:13px;line-height:1.7;color:var(--studio-muted,#8b9b97)}button{margin:0;padding:7px 14px;font-size:13px;line-height:24px;color:var(--studio-text,#d8e3df);background:var(--studio-input,#172329);border:1px solid var(--studio-border,#ffffff18);border-radius:7px;white-space:nowrap}button::after{border:0}.primary{background:#75b99d;color:#10241c;border-color:transparent}.danger{color:#eb9393}button[disabled]{opacity:.5}.form{display:flex;flex-direction:column;gap:16px;padding:22px;border:1px solid var(--studio-border,#ffffff18);border-radius:12px;background:var(--studio-surface,#0f1b20)}.form-title{font-weight:700;font-size:17px}label{display:flex;flex-direction:column;gap:8px;font-size:13px;flex:1;min-width:0}input,.date-choice{box-sizing:border-box;min-height:42px;padding:10px 12px;font-size:13px;line-height:22px;color:var(--studio-text,#d8e3df);border:1px solid var(--studio-border,#ffffff18);border-radius:6px;background:var(--studio-input,#172329)}.fields,.actions{display:flex;gap:12px;align-items:center}.schedule-row{display:flex;gap:18px;align-items:center;padding:18px 20px;border:1px solid var(--studio-border,#ffffff18);border-radius:10px;background:var(--studio-surface,#0f1b20)}.day{font-variant-numeric:tabular-nums;font-size:14px;font-weight:600;white-space:nowrap}.content{flex:1}.name{font-size:16px;overflow-wrap:anywhere}.empty{padding:40px 20px;text-align:center;color:var(--studio-muted,#8b9b97);border:1px dashed var(--studio-border,#ffffff18);border-radius:10px}.error{font-size:13px;color:#ed9696}.notice{font-size:13px;line-height:1.7;padding:12px 16px;background:rgba(197,150,50,.1);border-radius:8px;color:var(--studio-muted,#b5c6be)}@media(max-width:720px){.schedule-editor{padding:16px}.heading{align-items:flex-start}.fields{align-items:stretch;flex-direction:column}.fields label{width:100%}.schedule-row{flex-wrap:wrap}.schedule-row .actions{width:100%;justify-content:flex-end}.title{font-size:20px}}
</style>
