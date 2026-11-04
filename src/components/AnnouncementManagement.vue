<script setup lang="ts">
import {computed,onMounted,onUnmounted,ref,watch} from 'vue';
import EditorChoice from './EditorChoice.vue';
import {announcementsApi,type AnnouncementEntry,type AnnouncementMutation,type AnnouncementState} from '@/services/announcements';
import type {Announcement} from '@/domain/types';
import {menuExternalUrl} from '@/domain/operation-pages';

const emit=defineEmits<{'dirty':[value:boolean]}>();
const empty=():Announcement=>({enabled:false,title:'',text:'',style:'strip',image:'',frequency:'launch',actionLabel:'查看',page:'home',url:''});
const state=ref<AnnouncementState>({version:0,updatedAt:'',items:[]});
const loading=ref(false),saving=ref(false),loaded=ref(false),editorOpen=ref(false),editingId=ref('');
const draft=ref<Announcement>(empty()),baseline=ref(JSON.stringify(empty()));
const error=ref(''),saveError=ref(''),notice=ref(''),conflict=ref(false),missing=ref(false);
const filter=ref(''),switchEpoch=ref(0);
const styles=[{id:'strip',label:'顶部公告条'},{id:'ticker',label:'滚动文字'},{id:'card',label:'公告卡片'},{id:'modal',label:'弹窗公告'}];
const frequencies=[{id:'launch',label:'每次启动'},{id:'daily',label:'每天一次'},{id:'once',label:'每条公告一次'}];
const pages=[{id:'home',label:'首页'},{id:'category',label:'分类'},{id:'search',label:'搜索'},{id:'shortdrama',label:'短剧'},{id:'member',label:'会员'},{id:'mine',label:'我的'},{id:'ranking',label:'排行榜'},{id:'schedule',label:'排期表'},{id:'library',label:'片库'},{id:'history',label:'历史记录'},{id:'settings',label:'设置'},{id:'disclaimer',label:'免责声明'},{id:'privacy',label:'隐私政策'},{id:'about',label:'关于'}];
const dirty=computed(()=>editorOpen.value&&JSON.stringify(draft.value)!==baseline.value);
const active=computed(()=>state.value.items.find(item=>item.config.enabled));
const entries=computed(()=>{const word=filter.value.trim().toLocaleLowerCase();return state.value.items.filter(item=>!word||(item.config.title+' '+item.config.text).toLocaleLowerCase().includes(word));});
const busy=computed(()=>loading.value||saving.value);
const editorEntry=computed(()=>state.value.items.find(item=>item.id===editingId.value));
const saveStatus=computed(()=>saveError.value?'保存失败，编辑已保留':saving.value?'正在保存…':dirty.value?editingId.value?'等待自动保存':'创建后自动保存':'已自动保存');
let alive=true,timer:ReturnType<typeof setTimeout>|undefined,inFlight:Promise<boolean>|null=null;
function describe(value:unknown){return value instanceof Error?value.message:'操作失败，请重试。';}
function copy(value:Announcement):Announcement{return JSON.parse(JSON.stringify(value));}
function stamp(value:string){const time=new Date(value);return value&&!Number.isNaN(time.getTime())?time.toLocaleString('zh-CN',{hour12:false}):'—';}
function styleName(value:string){return styles.find(item=>item.id===value)?.label||value;}
function stopTimer(){if(timer)clearTimeout(timer);timer=undefined;}
function localError(value:Announcement){if(!value.title.trim())return '请填写公告标题。';if(value.enabled&&!value.text.trim())return '请先填写公告内容再开启。';if(value.url&&!menuExternalUrl(value.url))return '跳转链接请填写完整的 http:// 或 https:// 地址。';return '';}
function schedule(){
  stopTimer();
  if(!editingId.value||!dirty.value||conflict.value||missing.value)return;
  timer=setTimeout(()=>{timer=undefined;void flush();},650);
}
watch(dirty,value=>emit('dirty',value),{flush:'sync'});
watch(draft,()=>{if(!conflict.value&&!missing.value)saveError.value='';schedule();},{deep:true});
async function load(){
  if(busy.value)return;
  loading.value=true;error.value='';
  try{
    const result=await announcementsApi.list();if(!alive)return;
    state.value=result;loaded.value=true;
    if(editorOpen.value&&editingId.value){
      const entry=result.items.find(item=>item.id===editingId.value);
      missing.value=!entry;
      if(entry&&(!dirty.value||JSON.stringify(entry.config)===JSON.stringify(draft.value))){draft.value=copy(entry.config);baseline.value=JSON.stringify(entry.config);saveError.value='';}
      else if(dirty.value)saveError.value=entry?'已加载最新列表，你的编辑已保留。核对后点击「重试保存」。':'这条公告已被删除，你的编辑仍保留在下方。';
    }
    conflict.value=false;
  }catch(e){if(alive)error.value=describe(e);}
  finally{if(alive)loading.value=false;}
}
async function saveExisting():Promise<boolean>{
  if(!editingId.value||!dirty.value)return true;
  if(conflict.value||missing.value)return false;
  const snapshot=copy(draft.value),id=editingId.value,validation=localError(snapshot);
  if(validation){saveError.value=validation;return false;}
  saving.value=true;saveError.value='';
  try{
    const result=await announcementsApi.change(state.value.version,{action:'update',id,config:snapshot});
    if(!alive)return true;
    state.value=result;baseline.value=JSON.stringify(snapshot);notice.value='公告已保存，四端下次同步时生效。';return true;
  }catch(e){if(alive){saveError.value=describe(e);conflict.value=(e as {status?:number})?.status===409;}return false;}
  finally{if(alive)saving.value=false;}
}
async function flush():Promise<boolean>{
  stopTimer();
  if(inFlight)return inFlight;
  if(!dirty.value||!editingId.value)return true;
  inFlight=(async()=>{while(alive&&editingId.value&&dirty.value){if(!await saveExisting())return false;}return true;})();
  try{return await inFlight;}finally{inFlight=null;}
}
async function confirmDiscard(){return new Promise<boolean>(resolve=>uni.showModal({title:'编辑尚未保存',content:'当前编辑还没有保存。离开将放弃这些修改，是否继续？',confirmText:'放弃修改',cancelText:'继续编辑',success:r=>resolve(!!r.confirm),fail:()=>resolve(false)}));}
async function prepareToLeave(){
  if(busy.value&&!inFlight)return false;
  if(editingId.value&&dirty.value&&!conflict.value&&!missing.value)await flush();
  if(dirty.value&&!await confirmDiscard())return false;
  stopTimer();return true;
}
async function closeEditor(){if(!await prepareToLeave())return;editorOpen.value=false;editingId.value='';draft.value=empty();baseline.value=JSON.stringify(draft.value);saveError.value='';conflict.value=false;missing.value=false;}
async function openEditor(entry?:AnnouncementEntry){
  if(busy.value)return;
  if(editorOpen.value&&!await prepareToLeave())return;
  editorOpen.value=false;editingId.value=entry?.id||'';draft.value=entry?copy(entry.config):empty();baseline.value=JSON.stringify(draft.value);saveError.value='';conflict.value=false;missing.value=false;editorOpen.value=true;notice.value='';
}
async function mutate(mutation:AnnouncementMutation){
  if(busy.value||!loaded.value||conflict.value)return;
  saving.value=true;error.value='';notice.value='';
  try{
    const result=await announcementsApi.change(state.value.version,mutation);if(!alive)return;
    state.value=result;
    if(mutation.action==='create'){
      const created=result.items.find(item=>!knownIds.has(item.id));
      if(created){editingId.value=created.id;draft.value=copy(created.config);baseline.value=JSON.stringify(created.config);}
      notice.value='公告已创建，后续编辑自动保存。';
    }else if(mutation.action==='delete')notice.value='公告已删除。';
    else if(mutation.action==='toggle')notice.value=mutation.enabled?'公告已开启，四端下次同步时生效。':'公告已关闭，四端下次同步时生效。';
  }catch(e){if(alive){error.value=describe(e);if((e as {status?:number})?.status===409){conflict.value=true;if(editorOpen.value)saveError.value=error.value;}}}
  finally{if(alive){saving.value=false;switchEpoch.value++;}}
}
let knownIds=new Set<string>();
async function create(){const validation=localError(draft.value);if(validation){saveError.value=validation;return;}knownIds=new Set(state.value.items.map(item=>item.id));await mutate({action:'create',config:copy(draft.value)});}
async function remove(entry:AnnouncementEntry){
  const approved=await new Promise<boolean>(resolve=>uni.showModal({title:'删除公告',content:'确认删除「'+entry.config.title+'」？'+(entry.config.enabled?'四端将在下次同步时停止显示。':''),confirmText:'删除',confirmColor:'#cf485c',success:r=>resolve(!!r.confirm),fail:()=>resolve(false)}));
  if(approved)await mutate({action:'delete',id:entry.id});
}
function toggle(entry:AnnouncementEntry,event:any){void mutate({action:'toggle',id:entry.id,enabled:!!event.detail.value});}
function editEnabled(event:any){draft.value.enabled=!!event.detail.value;}
function beforeUnload(event:BeforeUnloadEvent){if(dirty.value||saving.value){event.preventDefault();event.returnValue='';}}
onMounted(()=>{
  void load();
  // #ifdef H5
  window.addEventListener('beforeunload',beforeUnload);
  // #endif
});
onUnmounted(()=>{alive=false;stopTimer();
  // #ifdef H5
  window.removeEventListener('beforeunload',beforeUnload);
  // #endif
});
defineExpose({prepareToLeave});
</script>

<template>
  <view class="announcement-page">
    <view class="page-heading">
      <view><text class="eyebrow">CONTENT & COMMUNITY</text><text class="page-title">公告管理</text><text class="subtitle">统一管理手机、平板、电视和电脑端的公告内容</text></view>
      <text class="scope-tag">四端共享</text>
    </view>
    <view v-if="error" class="alert error" role="alert"><text>{{error}}</text><button :disabled="busy" @click="load">重新加载</button></view>
    <view v-if="notice" class="alert success" role="status">{{notice}}</view>
    <view class="summary-card"><view><text class="summary-label">当前公告</text><text class="summary-title">{{active?.config.title||'暂未开启公告'}}</text></view><text class="summary-note">四端共用一条启用公告 · 更改后自动保存</text></view>
    <view class="toolbar"><input v-model="filter" class="search-input" placeholder="搜索公告标题或内容" aria-label="搜索公告"/><text class="count">共 {{state.items.length}} 条公告</text><button class="btn" :disabled="busy||editorOpen" @click="load">刷新</button><button class="btn primary" :disabled="busy||!loaded||editorOpen||conflict" @click="openEditor()">＋ 新增公告</button></view>
    <view class="table-scroll">
      <view class="announcement-table" role="table" aria-label="共享公告列表">
        <view class="table-row table-head" role="row"><text role="columnheader">公告内容</text><text role="columnheader">展示方式</text><text role="columnheader">启用状态</text><text role="columnheader">更新时间</text><text role="columnheader">操作</text></view>
        <view v-for="entry in entries" :key="entry.id" class="table-row" :class="{selected:editingId===entry.id}" role="row">
          <view role="cell" class="content-cell"><text class="entry-title">{{entry.config.title||'未命名公告'}}</text><text class="excerpt">{{entry.config.text||'尚未填写内容'}}</text><text v-if="entry.sources?.length" class="legacy-source">历史来源：{{entry.sources?.join('、')}}</text></view>
          <text role="cell" class="style-label">{{styleName(entry.config.style)}}</text>
          <view role="cell" class="status-cell"><switch :key="entry.id+'-'+entry.config.enabled+'-'+state.version+'-'+switchEpoch" :checked="entry.config.enabled" color="#2b6ae8" :disabled="busy||editorOpen||conflict" :aria-label="entry.config.title+'启用开关'" @change="toggle(entry,$event)"/><text :class="{enabled:entry.config.enabled}">{{entry.config.enabled?'已开启':'已关闭'}}</text></view>
          <text role="cell" class="date-cell">{{stamp(entry.updatedAt)}}</text>
          <view role="cell" class="row-actions"><button class="text-button" :disabled="busy||editorOpen||conflict" @click="openEditor(entry)">编辑</button><button class="text-button danger" :disabled="busy||editorOpen||conflict" @click="remove(entry)">删除</button></view>
        </view>
        <view v-if="!entries.length" class="empty-state"><text class="empty-title">{{loading?'正在加载公告…':!loaded?'公告尚未加载':filter.trim()?'没有找到匹配的公告':'还没有公告'}}</text><text v-if="loaded&&!filter.trim()">点击「新增公告」，创建后四端共用。</text></view>
      </view>
    </view>
    <view v-if="editorOpen" class="editor-panel" :aria-busy="saving">
      <view class="editor-heading"><view><text class="editor-title">{{editingId?'编辑公告':'新增公告'}}</text><text class="save-status" :class="{failed:saveError}">{{editingId?saveStatus:'创建后，后续编辑会自动保存'}}</text></view><button class="btn" :disabled="busy" @click="closeEditor">收起编辑</button></view>
      <view v-if="saveError" class="alert error" role="alert"><text>{{saveError}}</text><button v-if="conflict||missing" :disabled="busy" @click="load">加载最新列表</button><button v-else-if="editingId" :disabled="busy" @click="flush">重试保存</button></view>
      <view class="form-grid" :inert="saving&&!editingId">
        <view class="field wide"><text>公告标题</text><input v-model="draft.title" maxlength="120" placeholder="请输入公告标题" :disabled="saving&&!editingId" aria-label="公告标题"/></view>
        <view class="field wide"><text>公告内容</text><textarea v-model="draft.text" maxlength="8000" placeholder="填写要展示给用户的公告内容" :disabled="saving&&!editingId" aria-label="公告内容"/><text class="field-help">开启的公告将同步到四端。开启另一条时，当前公告会自动关闭。</text></view>
        <view class="field"><text>展示方式</text><EditorChoice v-model="draft.style" :options="styles"/></view>
        <view class="field"><text>弹窗频率</text><EditorChoice v-model="draft.frequency" :options="frequencies"/><text class="field-help">仅在选择「弹窗公告」时使用。</text></view>
        <view class="field wide"><text>公告配图地址（可选）</text><input v-model="draft.image" maxlength="2048" placeholder="https://… 或已上传的素材地址" :disabled="saving&&!editingId" aria-label="公告配图地址"/></view>
        <view class="field"><text>按钮文字</text><input v-model="draft.actionLabel" maxlength="40" placeholder="例如：立即查看" :disabled="saving&&!editingId" aria-label="公告按钮文字"/></view>
        <view class="field"><text>App 内页面</text><EditorChoice v-model="draft.page" :options="pages"/></view>
        <view class="field wide"><text>第三方跳转链接（可选）</text><input v-model="draft.url" maxlength="2048" placeholder="https://…" :disabled="saving&&!editingId" aria-label="公告跳转链接"/><text class="field-help">填写链接后优先打开链接；留空时打开所选 App 内页面。</text></view>
      </view>
      <view v-if="editorEntry?.sources?.length" class="migration-note">从历史公告迁入：{{editorEntry.sources?.join('、')}}。现在统一按四端共享配置编辑。</view>
      <view class="editor-footer"><view class="enable-editor"><switch :key="editingId+'-'+draft.enabled" :checked="draft.enabled" color="#2b6ae8" :disabled="saving&&!editingId" aria-label="编辑公告启用状态" @change="editEnabled"/><text>{{draft.enabled?'开启公告':'暂不开启'}}</text></view><text v-if="editingId" class="field-help">编辑后自动保存，四端下次同步时生效</text><button v-else class="btn primary" :disabled="busy||!loaded||conflict" @click="create">{{saving?'正在创建…':'创建公告'}}</button></view>
    </view>
    <text class="sync-note">手机、平板、电视和电脑端共用此列表。客户端在线时约每 15 秒同步；离线端在恢复连接后获取最新公告。</text>
  </view>
</template>

<style scoped>
.announcement-page{padding:30px 32px 40px;color:var(--studio-text,#263b58);font-size:13px;min-width:0}
.page-heading{display:flex;justify-content:space-between;align-items:center;gap:20px;margin-bottom:26px}
.eyebrow,.page-title,.subtitle{display:block}.eyebrow{font-size:10px;font-weight:650;letter-spacing:1.7px;color:var(--studio-muted,#8998af);margin-bottom:9px}.page-title{font-size:24px;font-weight:650;letter-spacing:.2px}.subtitle{font-size:12px;color:var(--studio-muted,#8998af);margin-top:9px;line-height:1.8}.scope-tag{font-size:11px;color:#3372cf;background:#3372cf0c;border:1px solid #3372cf20;border-radius:5px;padding:5px 11px;white-space:nowrap}
.summary-card{display:flex;align-items:center;justify-content:space-between;gap:20px;padding:19px 22px;border:1px solid var(--studio-border,#e1e9f4);background:var(--studio-card,#fff);border-radius:9px;margin-bottom:21px}.summary-label{font-size:11px;color:var(--studio-muted,#8998af);margin-right:17px}.summary-title{font-size:14px;font-weight:600}.summary-note{font-size:11px;color:var(--studio-muted,#8998af)}
.toolbar{display:flex;align-items:center;gap:11px;margin-bottom:18px}.search-input{height:38px;box-sizing:border-box;max-width:320px;min-width:160px;flex:1;padding:0 13px;border:1px solid var(--studio-border,#dfe7f2);border-radius:6px;background:var(--studio-card,#fff);color:inherit;font-size:12px}.count{margin-left:auto;color:var(--studio-muted,#8998af);font-size:11px;white-space:nowrap}.btn{display:inline-flex;align-items:center;justify-content:center;margin:0;min-height:37px;padding:0 15px;border:1px solid var(--studio-border,#dfe7f2);border-radius:6px;background:var(--studio-card,#fff);color:inherit;font-size:12px;line-height:35px;white-space:nowrap}.btn.primary{border-color:#2b6ae8;background:#2b6ae8;color:#fff}button:after{border:0}button[disabled]{opacity:.45;cursor:not-allowed}
.table-scroll{overflow-x:auto;border:1px solid var(--studio-border,#e1e9f4);border-radius:9px;background:var(--studio-card,#fff)}.announcement-table{min-width:820px}.table-row{display:grid;grid-template-columns:minmax(240px,1fr) 110px 145px 170px 100px;gap:15px;align-items:center;padding:20px 22px;border-bottom:1px solid var(--studio-border,#edf1f7);font-size:12px}.table-row:last-child{border-bottom:0}.table-head{padding-top:14px;padding-bottom:14px;background:var(--studio-input,#f8fafe);font-size:11px;font-weight:600;color:var(--studio-muted,#7e90aa)}.table-row.selected{background:#2b6ae806}.content-cell{min-width:0}.entry-title{display:block;font-weight:600;margin-bottom:7px;overflow-wrap:anywhere}.excerpt{display:block;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;color:var(--studio-muted,#8998af);font-size:11px}.legacy-source{display:block;margin-top:8px;font-size:10px;line-height:1.6;color:var(--studio-muted,#8998af);overflow-wrap:anywhere}.style-label,.date-cell{font-size:11px;color:var(--studio-muted,#7e90aa)}.date-cell{font-variant-numeric:tabular-nums;line-height:1.7}.status-cell{display:flex;align-items:center;gap:4px;color:var(--studio-muted,#7e90aa);font-size:11px}.status-cell switch,.enable-editor switch{transform:scale(.72);transform-origin:left center;width:43px}.enabled{color:#279673}.row-actions{display:flex;gap:14px}.text-button{background:transparent;margin:0;padding:5px 0;color:#2b6ae8;font-size:12px;line-height:1.5}.text-button.danger{color:#cd5364}.empty-state{display:flex;flex-direction:column;align-items:center;gap:10px;padding:65px 20px;color:var(--studio-muted,#8998af);font-size:12px}.empty-title{font-size:14px;color:var(--studio-text,#52647e)}
.editor-panel{margin-top:22px;border:1px solid var(--studio-border,#e1e9f4);border-radius:9px;background:var(--studio-card,#fff);padding:23px}.editor-heading{display:flex;align-items:center;justify-content:space-between;gap:16px;margin-bottom:22px}.editor-title{font-size:16px;font-weight:600}.save-status{margin-left:17px;font-size:11px;color:#279673}.save-status.failed{color:#cd5364}.form-grid{display:grid;grid-template-columns:1fr 1fr;gap:20px}.field{min-width:0}.field.wide{grid-column:1/-1}.field>text:first-child{display:block;margin-bottom:9px;color:var(--studio-muted,#7e90aa);font-size:12px}.field input,.field textarea{box-sizing:border-box;width:100%;border:1px solid var(--studio-border,#dfe7f2);border-radius:6px;background:var(--studio-input,#fbfcfe);color:inherit;font-size:12px}.field input{height:40px;padding:0 12px}.field textarea{height:140px;padding:12px;line-height:1.8}.field-help{display:block;font-size:11px;line-height:1.8;color:var(--studio-muted,#8998af);margin-top:7px}.editor-footer{display:flex;align-items:center;justify-content:space-between;gap:20px;border-top:1px solid var(--studio-border,#edf1f7);padding-top:20px;margin-top:24px}.enable-editor{display:flex;align-items:center;gap:7px;font-size:12px}.migration-note{margin-top:20px;font-size:11px;line-height:1.8;color:var(--studio-muted,#8998af)}
.alert{display:flex;align-items:center;justify-content:space-between;gap:15px;padding:12px 16px;border-radius:7px;font-size:12px;line-height:1.8;margin-bottom:17px}.alert.error{background:#cd53640b;border:1px solid #cd536430;color:#cd5364}.alert.success{background:#2796730b;border:1px solid #27967325;color:#279673}.alert button{flex-shrink:0;margin:0;padding:0 10px;background:transparent;color:inherit;border:1px solid currentColor;font-size:11px;line-height:28px}.sync-note{display:block;margin-top:23px;font-size:10px;color:var(--studio-muted,#8998af);line-height:1.9}
:deep(input:focus-visible),:deep(textarea:focus-visible),button:focus-visible{outline:2px solid #2b6ae8;outline-offset:2px}
@media(hover:hover) and (pointer:fine){.btn:not([disabled]):hover{border-color:#a5bce4}.btn.primary:not([disabled]):hover{background:#215bc9}.text-button:not([disabled]):hover{text-decoration:underline}.table-row:not(.table-head):hover{background:#2b6ae805}}
@media(max-width:900px){.announcement-page{padding:22px 18px}.summary-card{align-items:flex-start;flex-direction:column;gap:10px}.form-grid{grid-template-columns:1fr}.field{grid-column:1/-1}.toolbar{flex-wrap:wrap}.count{margin-left:0}.editor-heading>view{display:flex;flex-direction:column;gap:8px}.save-status{margin-left:0}.editor-footer{flex-wrap:wrap}}
</style>
