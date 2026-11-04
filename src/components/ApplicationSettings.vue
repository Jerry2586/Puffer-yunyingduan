<script setup lang="ts">
import {onBeforeUnmount,ref,computed,onMounted,watch} from 'vue';
import EditorChoice from './EditorChoice.vue';
import {applicationApi,type ApplicationSettings} from '@/services/application';
import {managementApi} from '@/services/management';
import {useSharedAutosave} from '@/composables/useSharedAutosave';
const emit=defineEmits<{(e:'dirty',value:boolean):void}>();
const section=ref('application'),groups=ref<{id:string;label:string}[]>([]);
const sections=[{id:'application',label:'应用与搜索'},{id:'registration',label:'注册与验证码'},{id:'history',label:'观看历史'},{id:'avatar',label:'头像设置'},{id:'share',label:'分享与联系'},{id:'membership',label:'会员积分'},{id:'comments',label:'评论设置'}];
const autosave=useSharedAutosave<ApplicationSettings>({get:applicationApi.get,save:v=>applicationApi.save(v.version,v.config),signature:v=>JSON.stringify(v.config),validate:v=>{
 const c=v.config;
 if(c.application.listLimit<1||c.application.listLimit>100||c.application.searchDelayMs<0||c.application.searchDelayMs>3000)return '列表上限为 1–100，搜索延迟为 0–3000ms';
 if(c.history.intervalSeconds<10||c.history.intervalSeconds>3600||c.history.limit<1||c.history.limit>1000)return '历史同步间隔为 10–3600 秒，保留条数为 1–1000';
 if(c.membership.managed&&(c.membership.groupId<3||!(['day','week','month','year'] as const).some(k=>c.membership[k]>0)))return '请先选择会员组并设置至少一个兑换周期';
 if((c.registration.managed||c.membership.managed)&&!v.adapterConfigured)return '请先在内容源与连接中配置 CMS 管理适配器';
 if(c.comments.pinnedEnabled&&!c.comments.pinnedContent.trim())return '请填写置顶评论内容';
 return '';
}});
const {draft,loading,saving,error,conflict,dirty,status,load,flush,reload,prepareToLeave}=autosave;
const c=computed(()=>draft.value?.config);
const avatars=computed({get:()=>c.value?.avatar.library.join('\n')||'',set:(v:string)=>{if(c.value)c.value.avatar.library=v.split(/\r?\n/).map(s=>s.trim()).filter(Boolean);}});
const checked=(e:any)=>!!e.detail.value;
const captchaFields=[{id:'login',label:'登录验证码'},{id:'register',label:'注册验证码'},{id:'comments',label:'评论验证码'},{id:'danmu',label:'弹幕验证码'}] as const;
const periods=[{id:'day',label:'一天'},{id:'week',label:'一周'},{id:'month',label:'一月'},{id:'year',label:'一年'}] as const;
watch(dirty,v=>emit('dirty',v),{immediate:true});
onMounted(async()=>{await load();try{const v=await managementApi.meta();groups.value=v.groups.filter(g=>g.id>=3).map(g=>({id:String(g.id),label:g.name}));}catch{/* Connection errors are shown on the settings that require the adapter. */}});
onBeforeUnmount(()=>emit('dirty',false));
defineExpose({prepareToLeave});
</script>
<template>
<view class="shared-settings"><view class="settings-shell">
 <view class="settings-heading"><view><text class="settings-title">共享应用设置</text><text class="settings-subtitle">手机、平板、电视、电脑共用业务配置，修改后自动保存。</text></view><view class="settings-heading-actions"><text class="settings-badge" :class="{pending:dirty||saving,failed:!!error}">{{status}}</text><button :disabled="loading||saving" @click="reload">重新读取</button></view></view>
 <view v-if="error" class="settings-feedback error" role="alert"><text>{{error}}</text><button v-if="!conflict" :disabled="saving" @click="flush">重试保存</button><button v-else @click="reload">读取最新配置</button></view>
 <view v-if="loading" class="settings-empty">正在读取应用设置…</view>
 <view v-else-if="c&&draft" class="settings-layout">
  <view class="settings-sections" role="tablist" aria-label="应用设置分组"><button v-for="item in sections" :key="item.id" role="tab" :aria-selected="section===item.id" :class="{active:section===item.id}" @click="section=item.id">{{item.label}}</button></view>
  <view>
   <view v-if="section==='application'" class="settings-card"><text class="settings-card-title">应用与搜索</text><text class="settings-card-description">统一列表、搜索交互与内容索引。</text>
    <view class="settings-row"><view><text class="settings-label">搜索输入延迟</text><text class="settings-help">停止输入后发起搜索，单位毫秒。</text></view><input class="settings-control narrow" v-model.number="c.application.searchDelayMs" type="number" min="0" max="3000" aria-label="搜索输入延迟"/></view>
    <view class="settings-row"><view><text class="settings-label">每页列表上限</text><text class="settings-help">每次最多读取 1–100 条影片。</text></view><input class="settings-control narrow" v-model.number="c.application.listLimit" type="number" min="1" max="100" aria-label="每页列表上限"/></view>
    <view class="settings-row"><view><text class="settings-label">搜索索引同步间隔</text><text class="settings-help">单位分钟，0 关闭；定时重建内容快照，支持 1–1440 分钟。</text></view><input class="settings-control narrow" v-model.number="c.search.intervalMinutes" type="number" min="0" max="1440" aria-label="搜索索引同步间隔"/></view>
    <view class="settings-row"><view><text class="settings-label">电视端下载链接</text><text class="settings-help">供个人中心显示下载入口。</text></view><input class="settings-control" v-model="c.application.tvDownloadUrl" placeholder="https://…" maxlength="2048" aria-label="电视端下载链接"/></view>
    <view class="settings-row"><view><text class="settings-label">VPN 检测提醒</text><text class="settings-help">Android 客户端检测到 VPN 连接时提醒用户，不阻止使用。默认关闭；网页预览不执行系统网络检测。</text></view><switch :checked="!!c.application.vpnDetection" @change="c.application.vpnDetection=checked($event)" aria-label="VPN 检测提醒"/></view>
    <text class="settings-help">电视端原生下载仍需对应客户端支持。</text>
   </view>
   <template v-if="section==='registration'">
   <view class="settings-card"><text class="settings-card-title">注册规则</text><text class="settings-card-description">会员资料保存到当前绑定的苹果 CMS。</text>
    <view class="settings-row"><view><text class="settings-label">允许注册</text><text class="settings-help">关闭后客户端隐藏注册入口，接口同时禁止注册。</text></view><switch :checked="c.registration.enabled" @change="c.registration.enabled=checked($event)" aria-label="允许注册"/></view>
    <view class="settings-row"><view><text class="settings-label">每日每个 IP 注册上限</text><text class="settings-help">0 表示不限，最多 1000 个账号。</text></view><input class="settings-control narrow" v-model.number="c.registration.dailyIpLimit" type="number" min="0" max="1000" aria-label="每日注册上限"/></view>
    <view class="settings-row"><view><text class="settings-label">由运营中心管理注册规则</text><text class="settings-help">{{draft.adapterConfigured?'管理适配器已连接，可同步初始状态与积分。':'请先连接 CMS 管理适配器。'}}</text></view><switch :checked="c.registration.managed" :disabled="!draft.adapterConfigured" @change="c.registration.managed=checked($event)" aria-label="管理注册规则"/></view>
    <template v-if="c.registration.managed"><view class="settings-row"><view><text class="settings-label">新账号直接启用</text><text class="settings-help">关闭后账号等待 CMS 审核。</text></view><switch :checked="c.registration.active" @change="c.registration.active=checked($event)" aria-label="新账号直接启用"/></view><view class="settings-grid"><label>注册赠送积分<input v-model.number="c.registration.points" type="number" min="0" max="1000000"/></label><label>有效邀请奖励积分<input v-model.number="c.registration.invitePoints" type="number" min="0" max="1000000"/></label></view></template>
   </view>
   <view class="settings-card"><text class="settings-card-title">验证码</text><text class="settings-card-description">服务端生成图片验证码，验证成功后一次使用。</text><view class="settings-row"><view><text class="settings-label">验证码绑定请求 IP</text><text class="settings-help">校验提交来源与领取验证码时一致。</text></view><switch :checked="c.captcha.bindIp" @change="c.captcha.bindIp=checked($event)" aria-label="验证码绑定IP"/></view><view v-for="field in captchaFields" :key="field.id" class="settings-row"><text class="settings-label">{{field.label}}</text><switch :checked="c.captcha[field.id]" @change="c.captcha[field.id]=checked($event)" :aria-label="field.label"/></view></view>
   </template>
   <view v-if="section==='history'" class="settings-card"><text class="settings-card-title">观看历史</text><text class="settings-card-description">登录后按账号同步影片、集数和播放进度。</text><view class="settings-row"><view><text class="settings-label">云端同步</text><text class="settings-help">关闭时客户端保留本地观看记录。</text></view><switch :checked="c.history.enabled" @change="c.history.enabled=checked($event)" aria-label="观看历史云端同步"/></view><view class="settings-grid"><label>同步间隔（秒）<input v-model.number="c.history.intervalSeconds" type="number" min="10" max="3600"/></label><label>每个账号保留条数<input v-model.number="c.history.limit" type="number" min="1" max="1000"/></label></view></view>
   <view v-if="section==='avatar'" class="settings-card"><text class="settings-card-title">头像设置</text><text class="settings-card-description">四端共用头像库，用户选择后更新 CMS 资料。</text><view class="settings-row"><view><text class="settings-label">允许上传头像</text><text class="settings-help">支持不超过 1MB、2048 像素的图片。</text></view><switch :checked="c.avatar.upload" @change="c.avatar.upload=checked($event)" aria-label="允许上传头像"/></view><label class="settings-field">头像库链接<textarea v-model="avatars" :maxlength="205000" placeholder="每行一个 HTTP(S) 图片链接或已上传的素材地址"/></label><text class="settings-help">已添加 {{c.avatar.library.length}} / 100 张。上传素材后也可以填写素材地址。</text></view>
   <view v-if="section==='share'" class="settings-card"><text class="settings-card-title">分享与联系</text><text class="settings-card-description">个人中心的分享文案、邀请码和联系入口。</text><label class="settings-field">分享文案<textarea v-model="c.share.text" :maxlength="1000" placeholder="填写分享应用时展示的文字"/></label><view class="settings-row"><view><text class="settings-label">启用邀请分享</text><text class="settings-help">登录用户获得专属邀请码，注册时可以填写。</text></view><switch :checked="c.share.invite" @change="c.share.invite=checked($event)" aria-label="启用邀请分享"/></view><view class="settings-grid"><label>联系入口形式<EditorChoice v-model="c.share.contactMode" :options="[{id:'copy',label:'复制联系信息'},{id:'link',label:'打开联系链接'}]"/></label><label v-if="c.share.contactMode==='link'">联系链接<input v-model="c.share.contactUrl" maxlength="2048" placeholder="https://…"/></label></view><label class="settings-field">联系信息<textarea v-model="c.share.contactText" :maxlength="1000" placeholder="客服说明或联系方式"/></label></view>
   <view v-if="section==='membership'" class="settings-card"><text class="settings-card-title">会员积分兑换</text><text class="settings-card-description">兑换操作由 CMS 校验余额、扣除积分并延长会员期限。</text><view class="settings-row"><view><text class="settings-label">使用运营中心兑换配置</text><text class="settings-help">{{draft.adapterConfigured?'连接正常，可配置付费会员组。':'需要先连接 CMS 管理适配器。'}}</text></view><switch :checked="c.membership.managed" :disabled="!draft.adapterConfigured" @change="c.membership.managed=checked($event)" aria-label="使用运营中心兑换配置"/></view><label class="settings-field">付费会员组<EditorChoice v-if="groups.length" :model-value="String(c.membership.groupId)" @update:model-value="c.membership.groupId=Number($event)" :options="[{id:'0',label:'请选择会员组'},...groups]"/><input v-else v-model.number="c.membership.groupId" type="number" placeholder="CMS 付费会员组 ID，至少为 3"/></label><view class="settings-grid"><label v-for="period in periods" :key="period.id">{{period.label}}所需积分<input v-model.number="c.membership[period.id]" type="number" min="0" max="100000000"/></label></view><text class="settings-help">0 表示关闭该兑换周期。关闭统一配置后沿用 CMS 原有兑换规则。</text></view>
   <view v-if="section==='comments'" class="settings-card"><text class="settings-card-title">评论设置</text><text class="settings-card-description">审核、举报和关键词规则在内容管理中统一处理。</text><view class="settings-row"><text class="settings-label">允许发送评论</text><switch :checked="c.comments.enabled" @change="c.comments.enabled=checked($event)" aria-label="允许发送评论"/></view><view class="settings-row"><view><text class="settings-label">展示统一置顶评论</text><text class="settings-help">显示在影片评论区顶部。</text></view><switch :checked="c.comments.pinnedEnabled" @change="c.comments.pinnedEnabled=checked($event)" aria-label="展示统一置顶评论"/></view><view class="settings-grid"><label>置顶昵称<input v-model="c.comments.pinnedNickname" maxlength="50" placeholder="运营团队"/></label><label>置顶头像地址<input v-model="c.comments.pinnedAvatar" maxlength="2048" placeholder="HTTP(S) 或素材地址"/></label></view><label class="settings-field">置顶内容<textarea v-model="c.comments.pinnedContent" :maxlength="2000" placeholder="请输入置顶评论"/></label></view>
   <text class="settings-note">当前内容源：{{draft.source||'尚未连接'}} · 业务规则四端共享。</text>
  </view>
 </view>
 <view v-else class="settings-empty"><text>暂时无法读取配置。</text><button @click="load">重新读取</button></view>
</view></view>
</template>
<style src="../styles/shared-settings.css"></style>
