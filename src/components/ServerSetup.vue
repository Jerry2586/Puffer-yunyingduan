<script setup lang="ts">
import {ref,onMounted} from 'vue';
import {api,type SetupStatus,type CmsDiscovery,type CmsStatus} from '@/services/api';
import CmsConnectionReport from './CmsConnectionReport.vue';
function savedAppearance() { try { return localStorage.getItem('hetun-install-theme')==='dark'?'dark':'light'; } catch { return 'light'; } }
const appearance=ref(savedAppearance());
function toggleAppearance(){appearance.value=appearance.value==='dark'?'light':'dark';try{localStorage.setItem('hetun-install-theme',appearance.value);}catch{}}
const props=defineProps<{status:SetupStatus}>();const emit=defineEmits<{complete:[];logout:[]}>();
const username=ref(props.status.username),password=ref(''),confirm=ref(''),base=ref(''),busy=ref(false),discovering=ref(false),error=ref(''),discovery=ref<CmsDiscovery|null>(null),report=ref<CmsStatus|null>(null);
async function discover(){discovering.value=true;error.value='';try{discovery.value=await api.discoverCms();const c=discovery.value.candidates.find(c=>c.id===discovery.value?.recommendedId);if(c){base.value=c.base;report.value=c.report;}}catch(e){error.value=(e as Error).message;}finally{discovering.value=false;}}
async function check(){busy.value=true;error.value='';try{report.value=await api.setupCheck(base.value);}catch(e){error.value=(e as Error).message;}finally{busy.value=false;}}
async function finish(){if(password.value!==confirm.value){error.value='两次输入的密码不一致。';return;}busy.value=true;error.value='';try{await api.completeSetup(username.value,password.value,base.value);password.value='';confirm.value='';emit('complete');}catch(e){error.value=(e as Error).message;}finally{busy.value=false;}}
onMounted(discover);

</script>
<template>
  <view class="setup-page" :class="'setup-'+appearance">
    <view class="setup-header">
      <view class="setup-brand"><image class="brand-logo" src="/static/hetun-mark.svg" mode="aspectFit" aria-label="河豚影视标识"/><view><text class="brand-name">河豚运营中心</text><text class="brand-caption">HETUN OPERATIONS CENTER</text></view></view>
      <button class="theme-toggle" :aria-label="appearance==='dark'?'切换到浅色模式':'切换到深色模式'" :aria-pressed="appearance==='dark'" @click="toggleAppearance"><text class="theme-icon">{{appearance==='dark'?'☾':'☀'}}</text><text>{{appearance==='dark'?'深色模式':'浅色模式'}}</text></button>
    </view>
    <view class="setup-shell">
      <view class="setup-intro">
        <image class="hero-logo" src="/static/hetun-mark.svg" mode="aspectFit" aria-label="河豚影视标识"/>
        <text class="eyebrow">开始使用河豚</text>
        <text class="setup-title">连接你的影视世界</text>
        <text class="setup-description">完成内容与账号设置，开启你的影视运营工作台。</text>
        <view class="setup-steps">
          <view class="setup-step"><text class="step-number">01</text><view><text class="step-title">独立部署</text><text class="muted">运营服务运行在你自己的服务器。</text></view></view>
          <view class="setup-step"><text class="step-number">02</text><view><text class="step-title">连接苹果 CMS</text><text class="muted">自动寻找站点，检查真实内容接口。</text></view></view>
          <view class="setup-step"><text class="step-number">03</text><view><text class="step-title">开始运营</text><text class="muted">管理内容、栏目与应用运营配置。</text></view></view>
        </view>
        <text class="aside-note">主题中心制作主题，本地打包器生成安装包，运营中心为 App 提供内容与业务服务。</text>
        <text v-if="status.localPreview" class="local-badge">本机测试环境</text>
      </view>
      <view class="setup-form">
        <view class="setup-heading"><view><text class="form-title">内容与账号初始化</text><text class="muted">连接内容来源，设置你的管理员账号。</text></view><text class="form-badge">首次设置</text></view>
        <view class="section-heading"><text class="section-number">01</text><text>内容连接</text></view>
        <view class="discovery-heading"><text class="muted">自动寻找同服务器上的苹果 CMS；多个可用站点选择一次。</text><button class="secondary" :disabled="busy||discovering" @click="discover">{{discovering?'检测中…':'重新发现 CMS'}}</button></view>
        <text v-if="discovery?.multiple" class="muted">检测到多个可用站点，请选择本应用连接的 CMS。</text>
        <button v-for="c in discovery?.candidates" :key="c.id" class="candidate" :class="{selected:base===c.base}" :disabled="busy||!c.base" @click="base=c.base;report=c.report"><view><text>{{c.name}}</text><text class="muted">{{c.base||'未识别域名，请在下面填写'}} · {{c.adapterInstalled?'已发现权限适配文件':'权限适配文件待安装'}}</text></view><text>{{c.report?.ok?'可连接':'待检查'}}</text></button>
        <view v-if="discovery&&!discovery.candidates.length" class="discovery-notice"><text>暂未发现苹果 CMS</text><text class="muted">请确认服务器已安装 CMS，或在下面填写已有站点地址。</text></view>
        <details v-if="discovery?.warnings.length" class="discovery-details"><summary>查看检测详情</summary><text v-for="warning in discovery.warnings" :key="warning" class="muted">{{warning}}</text></details>
        <label>苹果 CMS 站点地址</label>
        <view class="address-row"><input v-model="base" placeholder="https://你的CMS域名" :disabled="busy" @input="report=null"/><button class="secondary" :disabled="busy||discovering||!base" @click="check">检测连接</button></view>
        <CmsConnectionReport :report="report" :checking="busy" title="实际接口检测"/>
        <view class="section-heading account-heading"><text class="section-number">02</text><text>管理员账号</text></view>
        <view class="account-grid"><view><label>管理员账号</label><input v-model="username" maxlength="40" placeholder="3–40位字母、数字或下划线"/></view><view><label>新密码</label><input v-model="password" password :maxlength="-1" placeholder="密码由你自行设置"/></view><view><label>确认新密码</label><input v-model="confirm" password :maxlength="-1" placeholder="再次输入新密码"/></view></view>
        <text v-if="error" class="failure" role="alert">{{error}}</text>
        <button class="finish" :disabled="busy||discovering||!base||!username||!password" @click="finish">{{busy?'检测并保存中…':'完成安装，进入运营中心'}}<text v-if="!busy"> →</text></button>
        <text class="muted finish-note">保存前再次检测 CMS。完成后使用新账号密码登录。</text>
        <button class="exit" :disabled="busy" @click="emit('logout')">退出登录</button>
      </view>
    </view>
  </view>
</template>
<style scoped>
.setup-page{--studio-bg:#f5f6f3;--studio-card:#fff;--studio-text:#202c29;--studio-muted:#73807a;--studio-border:#e5eae6;--studio-input:#fafbf9;--studio-link:#28715b;--setup-accent:#28715b;--setup-hover:#edf3ee;--setup-button-text:#fff;--setup-shadow:0 16px 56px #243c2910;color-scheme:light;min-height:100vh;width:100%;background:var(--studio-bg);color:var(--studio-text);font-family:Inter,'Segoe UI','Microsoft YaHei',system-ui,sans-serif;padding:0 32px 40px;box-sizing:border-box}
.setup-dark{--studio-bg:#111816;--studio-card:#1a2420;--studio-text:#edf2ed;--studio-muted:#a0b0a6;--studio-border:#34443a;--studio-input:#141d18;--studio-link:#82c6a7;--setup-accent:#82c6a7;--setup-hover:#29392f;--setup-button-text:#14251b;--setup-shadow:0 16px 56px #0003;color-scheme:dark}
.setup-header{max-width:1152px;margin:auto;padding:28px 0;display:flex;align-items:center;justify-content:space-between;gap:16px}.setup-brand{display:flex;align-items:center;gap:12px}.brand-logo{width:42px;height:42px;flex-shrink:0}.brand-name{display:block;font-size:16px;font-weight:650;letter-spacing:1px}.brand-caption{display:block;margin-top:4px;font-size:10px;letter-spacing:1.8px;color:var(--studio-muted)}
.setup-page button{margin:0;line-height:1.5;box-sizing:border-box;cursor:pointer;font-family:inherit}.setup-page button::after{border:0}.theme-toggle{display:flex;align-items:center;gap:9px;border:1px solid var(--studio-border);border-radius:24px;padding:9px 14px;background:var(--studio-card);color:var(--studio-text);font-size:12px;flex-shrink:0}.theme-icon{font-size:18px;line-height:1}
.setup-shell{max-width:1076px;margin:30px auto 0;display:grid;grid-template-columns:.85fr 1.15fr;gap:80px;align-items:start}.setup-intro{padding-top:36px}.hero-logo{display:block;width:82px;height:82px;margin-bottom:28px}.eyebrow{display:block;font-size:12px;color:var(--setup-accent);font-weight:600}.setup-title{display:block;font-size:36px;line-height:1.3;letter-spacing:-1px;font-weight:650;margin:22px 0 14px}.setup-description{display:block;font-size:14px;line-height:1.9;max-width:320px;color:var(--studio-muted)}.muted{display:block;font-size:12px;line-height:1.8;color:var(--studio-muted);overflow-wrap:anywhere}
.setup-steps{margin:34px 0}.setup-step{position:relative;display:flex;gap:14px;padding-bottom:25px}.setup-step:not(:last-child)::after{content:'';position:absolute;left:15px;top:37px;bottom:5px;width:1px;background:var(--studio-border)}.step-number{display:flex;align-items:center;justify-content:center;width:32px;height:32px;flex-shrink:0;border:1px solid var(--studio-border);border-radius:50%;background:var(--studio-card);font-size:12px;font-weight:600;color:var(--setup-accent)}.step-title{display:block;font-size:14px;font-weight:600;margin:4px 0 6px}.aside-note{display:block;border-top:1px solid var(--studio-border);padding-top:20px;color:var(--studio-muted);font-size:12px;line-height:1.9;max-width:320px}.local-badge{display:inline-block;color:var(--setup-accent);background:var(--setup-hover);border-radius:6px;padding:5px 9px;font-size:10px;margin-top:18px}
.setup-form{min-width:0;background:var(--studio-card);border:1px solid var(--studio-border);border-radius:20px;padding:30px 32px;box-shadow:var(--setup-shadow)}.setup-heading{display:flex;align-items:start;justify-content:space-between;gap:12px;margin-bottom:25px}.form-title{display:block;font-size:21px;letter-spacing:-.5px;font-weight:650;margin-bottom:8px}.form-badge{font-size:10px;color:var(--setup-accent);background:var(--setup-hover);border-radius:6px;padding:5px 8px;white-space:nowrap}.section-heading{display:flex;align-items:center;gap:9px;font-size:12px;font-weight:600;margin:20px 0 14px}.section-heading::after{content:'';flex:1;height:1px;background:var(--studio-border);margin-left:5px}.section-number{font-size:10px;color:var(--setup-accent);background:var(--setup-hover);padding:3px 6px;border-radius:4px}.discovery-heading{display:flex;align-items:center;gap:14px}.discovery-heading>.muted{flex:1}.secondary{font-size:11px;white-space:nowrap;padding:10px 12px;background:var(--studio-card);color:var(--studio-text);border:1px solid var(--studio-border);border-radius:8px}.secondary:hover{background:var(--setup-hover)}.discovery-notice{margin-top:14px;padding:12px 14px;border:1px solid var(--studio-border);border-radius:9px;background:var(--studio-input);font-size:12px}.discovery-notice>.muted{margin-top:4px}.discovery-details{font-size:11px;color:var(--studio-muted);margin-top:10px}.discovery-details summary{cursor:pointer;margin-bottom:6px}.discovery-details .muted{font-size:11px}
label{display:block;font-size:12px;font-weight:500;margin:14px 0 7px}input{height:42px;padding:0 12px;border-radius:8px;background:var(--studio-input);color:var(--studio-text);border:1px solid var(--studio-border);font-size:12px;box-sizing:border-box;width:100%}input:focus-within{border-color:var(--setup-accent)}.address-row{display:flex;gap:10px}.address-row input{flex:1;min-width:0}.candidate{display:flex;align-items:center;justify-content:space-between;text-align:left;width:100%;padding:14px;margin-top:12px!important;font-size:12px;border:1px solid var(--studio-border);border-radius:10px;background:var(--studio-input);color:var(--studio-text)}.candidate.selected{border-color:var(--setup-accent)}.candidate>view{min-width:0;overflow-wrap:anywhere}.candidate>text{flex-shrink:0;color:var(--setup-accent);font-size:11px}.account-heading{margin-top:26px}.account-grid{display:grid;grid-template-columns:1fr 1fr;gap:0 12px}.account-grid>view{min-width:0}.account-grid>view:first-child{grid-column:1/-1}.setup-page .finish{width:100%;margin:24px 0 10px;background:var(--setup-accent);color:var(--setup-button-text);font-size:13px;font-weight:600;border-radius:9px;padding:13px;border:1px solid var(--setup-accent)}.failure{display:block;color:#c65348;font-size:12px;margin:16px 0}.finish-note{font-size:11px}.setup-page .exit{display:block;background:transparent;color:var(--studio-muted);font-size:12px;margin:14px auto 0;padding:8px}.setup-page button[disabled]{opacity:.5;cursor:not-allowed}.setup-page button:focus-visible{outline:2px solid var(--setup-accent);outline-offset:3px}
@media(max-width:900px){.setup-shell{gap:32px;grid-template-columns:.75fr 1.25fr}.setup-title{font-size:30px}.setup-form{padding:24px}}
@media(max-width:680px){.setup-page{padding:0 18px 24px}.setup-header{padding:20px 0}.brand-name{font-size:14px}.brand-caption{font-size:8px;letter-spacing:1px}.theme-toggle{padding:8px 10px;font-size:11px}.setup-shell{grid-template-columns:1fr;margin-top:12px;gap:24px}.setup-intro{padding-top:0}.hero-logo{display:none}.setup-title{font-size:28px;margin:12px 0}.setup-steps{display:none}.aside-note{display:none}.local-badge{margin-top:12px}.setup-form{padding:22px 18px}.account-grid{grid-template-columns:1fr}.discovery-heading{align-items:start}.form-title{font-size:19px}}
</style>
