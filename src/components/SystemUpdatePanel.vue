<script setup lang="ts">
import {ref,computed,onMounted,onBeforeUnmount} from 'vue';
import {updateApi,type ServiceUpdate} from '@/services/api';
const state=ref<ServiceUpdate|null>(null),busy=ref(false),error=ref(''),confirm=ref(false),completed=ref(false),target=ref('');
let disposed=false,timer:ReturnType<typeof setTimeout>|undefined;
const active=computed(()=>!!state.value&&!['idle','checked','failed'].includes(state.value.phase));
const steps=[{id:'queued',name:'准备'},{id:'checking',name:'核对版本'},{id:'downloading',name:'下载安装包'},{id:'verifying',name:'完整性校验'},{id:'preflight',name:'兼容检查'},{id:'restarting',name:'重启服务'}];
const phaseIndex=computed(()=>steps.findIndex(s=>s.id===state.value?.phase));
const installable=computed(()=>state.value?.canInstall&&state.value.available&&!!state.value.latest?.asset&&!active.value);
function reload(){window.location.reload();}
const time=(v:string)=>v?new Date(v).toLocaleString():'尚未检查';
async function load(){try{state.value=await updateApi.status();if(active.value){target.value=state.value.latest?.version||'';poll();}}catch(e){error.value=(e as Error).message;}}
async function check(){if(busy.value||active.value)return;busy.value=true;confirm.value=false;error.value='';try{state.value=await updateApi.check();}catch(e){error.value=(e as Error).message;}finally{busy.value=false;}}
async function install(){if(!installable.value||busy.value)return;busy.value=true;error.value='';target.value=state.value!.latest!.version;try{state.value=await updateApi.install(target.value);confirm.value=false;poll();}catch(e){error.value=(e as Error).message;}finally{busy.value=false;}}
function poll(){let attempts=0;const tick=async()=>{if(disposed)return;attempts++;try{const health=await updateApi.health();if(health.status==='ok'&&health.version===target.value){completed.value=true;return;}state.value=await updateApi.status();if(state.value.phase==='failed'){error.value=state.value.message;return;}}catch{/* Restart temporarily closes connections and invalidates the login session. */}if(attempts>=90){error.value='未确认新版本恢复运行。请查看宝塔进程日志并重新打开后台；不要重复点击安装。';return;}timer=setTimeout(tick,2000);};timer=setTimeout(tick,2000);}
onMounted(load);onBeforeUnmount(()=>{disposed=true;if(timer)clearTimeout(timer);});
</script>
<template><view class="update-panel">
 <view v-if="error" role="alert" class="notice error">{{error}}</view>
 <view v-if="completed" role="status" class="notice success">已确认服务恢复运行，当前版本 {{target}}。<button class="primary" @click="reload">重新登录后台</button></view>
 <view class="config-card"><view class="card-head"><view><text class="head-title">运营服务更新</text></view><text class="badge live">正式发布通道</text></view>
 <view class="card-body"><template v-if="state">
 <view class="version-grid"><view class="version-tile"><text class="muted">当前运行</text><text class="version">v{{state.current}}</text><text class="muted">{{state.platform}}</text></view><view class="version-tile"><text class="muted">最新正式版</text><text class="version">{{state.latest?'v'+state.latest.version:'—'}}</text><text class="muted">{{state.latest?time(state.latest.publishedAt):'检查后显示真实版本'}}</text></view></view>
 <view v-if="state.phase!=='idle'" class="status-bar"><text role="status">{{state.message}}</text><text>{{time(state.checkedAt)}}</text></view>
 <view v-if="state.reason" class="notice">{{state.reason}}</view>
 <view v-if="active" class="update-steps" role="status"><view v-for="(step,index) in steps" :key="step.id" :class="['step',{current:index===phaseIndex,done:index<phaseIndex}]"><text class="step-number">{{index<phaseIndex?'✓':index+1}}</text><text>{{step.name}}</text></view></view>
 <view v-if="state.latest?.notes" class="release-notes"><text class="head-title">版本说明</text><text selectable>{{state.latest.notes}}</text></view>
 <view v-if="confirm" class="confirm-box"><text class="head-title">更新到 v{{state.latest?.version}}</text><text class="muted">程序将校验安装包并短暂重启，完成后需要重新登录。配置文件、素材及业务数据库保留。请在业务空闲时执行；更新前保留数据库和素材备份。</text><view class="actions"><button :disabled="busy" @click="confirm=false">暂不更新</button><button class="primary" :disabled="busy" @click="install">确认安装并重启</button></view></view>
 <view class="card-footer"><text class="muted">发布需附带对应架构的安装包和校验文件。只推送源码不会产生可安装的新版。</text><view class="actions"><button :disabled="busy||active||completed" @click="check">{{busy?'正在处理…':'检查更新'}}</button><button class="primary" :disabled="!installable||busy||completed" @click="confirm=true">安装更新</button></view></view>
 </template><text v-else class="muted">{{error?'暂时无法读取更新状态':'正在读取系统更新状态…'}}</text></view></view>
 <view class="section-note">此处更新的是运营后台服务。手机、平板及电视 App 安装包仍在“版本管理”中维护。</view>
</view></template>
<style scoped src="../styles/service-forms.css"></style>
<style scoped>
.version-grid{display:grid;grid-template-columns:1fr 1fr;gap:20px;margin-bottom:28px}.version-tile{display:flex;flex-direction:column;gap:10px;padding:22px;border:1px solid var(--studio-border);border-radius:9px;background:var(--studio-input)}.version{font-size:28px;font-weight:600;letter-spacing:-.6px;color:var(--studio-text)}.release-notes{padding:20px 0;border-top:1px solid var(--studio-border)}.release-notes>text{display:block}.release-notes>text:last-child{white-space:pre-wrap;overflow-wrap:anywhere;font-size:13px;line-height:1.9;color:var(--studio-muted);margin-top:12px;max-height:320px;overflow:auto}.update-steps{display:flex;flex-wrap:wrap;gap:14px;margin:24px 0}.step{display:flex;gap:8px;align-items:center;font-size:12px;color:var(--studio-muted)}.step-number{display:grid;place-items:center;width:24px;height:24px;background:var(--studio-input);border:1px solid var(--studio-border);border-radius:50%}.step.current,.step.done{color:#499c76}.step.current .step-number{border-color:#499c76}.confirm-box{display:flex;flex-direction:column;gap:16px;padding:20px;border:1px solid #499c7680;background:#499c760a;border-radius:9px}.confirm-box .actions{justify-content:flex-end}.notice.success button{display:inline-block;margin-left:16px}@media(max-width:540px){.version-grid{grid-template-columns:1fr}.version{font-size:24px}}
</style>
