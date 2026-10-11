<script setup lang="ts">
import {computed,ref,watch} from 'vue';import type {Theme,AppPage} from '@/domain/types';import {assetUrl} from '@/services/api';import {announcementKey,shouldAnnounce,updatePolicy} from '@/domain/client-notifications';import type {UpdateTarget} from '@/domain/client-notifications';
const props=defineProps<{theme:Theme;version:string;ready:boolean;studio?:boolean;scope:string;target:UpdateTarget;announcementPreview?:number;updatePreview?:number}>();
const emit=defineEmits<{navigate:[page:AppPage];status:[message:string];blocking:[value:boolean]}>();
const dialog=ref<''|'announcement'|'update'>(''),manualPreview=ref(false),actionError=ref('');const seenLaunch=new Set<string>(),dismissedUpdates=new Set<string>();
const policy=computed(()=>updatePolicy(props.theme.clientSettings,props.version,props.target));
const forced=computed(()=>dialog.value==='update'&&policy.value.forced);
watch(forced,v=>emit('blocking',v),{immediate:true});
const today=()=>{const d=new Date();return d.getFullYear()+'-'+(d.getMonth()+1)+'-'+d.getDate();};
function evaluate(){
 if(props.studio||!props.ready)return;
 if(dialog.value==='update'&&!policy.value.available){dialog.value='';manualPreview.value=false;}
 if(policy.value.automatic&&(policy.value.forced||!dismissedUpdates.has(props.theme.clientSettings.latestVersion))){dialog.value='update';actionError.value='';return;}
 if(dialog.value)return;
 const key=announcementKey(props.theme.announcement,props.scope);
 if(shouldAnnounce(props.theme.announcement,seenLaunch.has(key),String(uni.getStorageSync(key)||''),today())){dialog.value='announcement';actionError.value='';}
}
watch(()=>[props.ready,props.version,props.target,JSON.stringify(props.theme.clientSettings),JSON.stringify(props.theme.announcement)],evaluate,{immediate:true});
watch(()=>props.announcementPreview,()=>{manualPreview.value=true;actionError.value='';dialog.value='announcement';});
watch(()=>props.updatePreview,()=>checkUpdate(true));
function acknowledge(){
 if(!manualPreview.value){const key=announcementKey(props.theme.announcement,props.scope);seenLaunch.add(key);uni.setStorageSync(key,today());}
 dialog.value='';manualPreview.value=false;evaluate();
}
function closeUpdate(){if(forced.value)return;dismissedUpdates.add(props.theme.clientSettings.latestVersion);dialog.value='';manualPreview.value=false;evaluate();}
function checkUpdate(preview=false){
 manualPreview.value=preview;actionError.value='';
 if(policy.value.available)dialog.value='update';else emit('status',props.theme.clientSettings.latestVersion?'当前已是最新版本':'暂未发布更新信息，请先填写最新版本号。');
}
function openUrl(url:string){
 if(!/^https?:\/\//i.test(url)){actionError.value='请配置完整的 HTTPS 更新地址。';return;}
 // #ifdef H5
 window.open(url,'_blank','noopener,noreferrer');
 // #endif
 // #ifdef APP-PLUS
 plus.runtime.openURL(url);
 // #endif
}
function download(){if(!policy.value.url){actionError.value=props.target==='web'?'请在 Android 或 iOS 客户端更新应用。':'此平台的更新地址尚未配置。';return;}openUrl(policy.value.url);}
function noticeAction(){const a=props.theme.announcement;if(a.url&&!/^https?:\/\//i.test(a.url)){actionError.value='公告链接尚未正确配置。';return;}if(a.url)openUrl(a.url);acknowledge();if(!a.url&&a.page!=='home')emit('navigate',a.page);}
defineExpose({checkUpdate,blocking:forced});
</script>
<template><view v-if="dialog" class="client-prompt" @touchmove.stop.prevent @keydown.esc.stop.prevent="dialog==='announcement'?acknowledge():closeUpdate()"><view class="prompt-mask" @click="dialog==='announcement'?acknowledge():closeUpdate()"/><view class="prompt-card" :class="'prompt-'+dialog" role="dialog" aria-modal="true" :aria-label="dialog==='update'?'版本更新':theme.announcement.title"><template v-if="dialog==='update'"><view class="update-art"><text>↑</text><view/><view/></view><text class="prompt-kicker">{{forced?'必须更新后继续使用':'发现新版本'}}</text><text class="prompt-title">升级至 v{{theme.clientSettings.latestVersion}}</text><text class="prompt-version">当前版本 v{{version}} · {{target==='ios'?'iOS':target==='android'?'Android':'网页'}}</text><scroll-view scroll-y class="prompt-copy"><text>{{theme.clientSettings.updateNotes||'服务方暂未填写更新说明。'}}</text></scroll-view><text v-if="actionError" class="prompt-error">{{actionError}}</text><button class="prompt-primary" @click="download">{{target==='ios'?'前往 App Store 更新':'立即更新'}}</button><button v-if="!forced" class="prompt-secondary" @click="closeUpdate">稍后再说</button><text v-else class="force-hint">此版本需要更新，升级完成后重新打开应用。</text><button v-if="studio&&manualPreview" class="preview-close" @click="dialog='';manualPreview=false">结束预览</button></template><template v-else><image v-if="theme.announcement.image" class="notice-cover" :src="assetUrl(theme.announcement.image)" mode="aspectFill"/><text class="prompt-kicker">{{theme.brand.name}} · 公告</text><text class="prompt-title">{{theme.announcement.title||'公告'}}</text><scroll-view scroll-y class="prompt-copy"><text>{{theme.announcement.text||'请先填写公告内容。'}}</text></scroll-view><text v-if="actionError" class="prompt-error">{{actionError}}</text><button class="prompt-primary" @click="noticeAction">{{theme.announcement.actionLabel||'知道了'}}</button><button v-if="theme.announcement.url||theme.announcement.page!=='home'" class="prompt-secondary" @click="acknowledge">关闭</button></template></view></view></template>
<style scoped>.client-prompt{position:absolute;inset:0;z-index:220;display:flex;align-items:center;justify-content:center;padding:24px;box-sizing:border-box;isolation:isolate}.prompt-mask{position:absolute;inset:0;background:#000a;backdrop-filter:blur(4px)}.prompt-card{position:relative;width:100%;max-width:350px;max-height:92%;box-sizing:border-box;overflow:auto;background:var(--surface);color:var(--text);border:1px solid #80808030;border-radius:18px;padding:24px;box-shadow:0 22px 65px #0005}.prompt-kicker{display:block;font-size:11px;color:var(--accent);margin:6px 0 9px}.prompt-title{display:block;font-size:21px;font-weight:700;line-height:1.4}.prompt-version{display:block;font-size:11px;color:var(--muted);margin:9px 0}.prompt-copy{max-height:210px;margin:18px 0;line-height:1.9;font-size:13px;white-space:pre-wrap;word-break:break-word}.prompt-copy text{display:block}.prompt-primary,.prompt-secondary,.preview-close{width:100%;border:0;border-radius:8px;font-size:14px;line-height:42px;padding:0;margin:12px 0 0}.prompt-primary{background:var(--accent);color:#001a20;font-weight:600}.prompt-secondary,.preview-close{background:transparent;color:var(--muted)}.prompt-primary::after,.prompt-secondary::after,.preview-close::after{border:0}.force-hint{font-size:10px;color:var(--muted);display:block;text-align:center;line-height:1.7;margin-top:15px}.prompt-error{display:block;color:#e58d77;font-size:12px;line-height:1.7}.update-art{position:relative;height:82px;overflow:hidden;margin:-24px -24px 17px;background:linear-gradient(125deg,#136279,#27284b)}.update-art>text{position:absolute;left:28px;top:4px;font-size:76px;line-height:1;color:#95efff}.update-art>view{position:absolute;width:85px;height:85px;border:1px solid #ffffff25;border-radius:50%;right:24px;top:-12px}.update-art>view:last-child{right:-30px;top:20px}.notice-cover{width:calc(100% + 48px);height:145px;margin:-24px -24px 20px;display:block}.preview-close{font-size:11px;border:1px dashed #80808055;line-height:30px}</style>
