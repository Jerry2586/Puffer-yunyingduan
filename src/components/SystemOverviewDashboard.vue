<script setup lang="ts">
import {computed,ref} from 'vue';
import ConnectionDetails from './ConnectionDetails.vue';
import type {SystemOverview} from '@/services/api';
const props=defineProps<{overview:SystemOverview|null;loading:boolean}>();
defineEmits<{refresh:[]}>();
const expanded=ref(true);
const v=computed(()=>props.overview);
const memory=computed(()=>v.value?.memory);
const pool=computed(()=>v.value?.database.pool);
const good=computed(()=>Number(v.value?.database.ok)+Number(v.value?.cms.ok)+Number(v.value?.redis.ok));
const connections=computed(()=>2+Number(v.value?.redis.configured));
const redisUsed=computed(()=>Math.max(0,(v.value?.redis.pool?.total||0)-(v.value?.redis.pool?.idle||0)));
const redisPercent=computed(()=>v.value?.redis.pool?.max?Math.min(100,redisUsed.value/v.value.redis.pool.max*100):0);
const health=computed(()=>!v.value?'等待检测':!v.value.database.ok?'存储检测异常':v.value.redis.configured&&!v.value.redis.ok?'Redis 连接异常':!v.value.cms.configured?'等待连接内容源':!v.value.cms.ok?'内容源检测异常':'服务检查通过');
const tone=computed(()=>!v.value?'neutral':!v.value.database.ok||v.value.cms.status==='error'||v.value.redis.configured&&!v.value.redis.ok?'danger':v.value.cms.ok?'success':'warning');
const memoryPercent=computed(()=>memory.value?.available&&memory.value.totalBytes?Math.min(100,(memory.value.usedBytes||0)/memory.value.totalBytes*100):null);
const poolPercent=computed(()=>pool.value?.maxOpen?Math.min(100,pool.value.inUse/pool.value.maxOpen*100):0);
const date=(value:string)=>value?new Date(value).toLocaleString('zh-CN',{hour12:false}):'—';
const bytes=(n?:number)=>n===undefined?'未获取':n>=1024**3?(n/1024**3).toFixed(2)+' GB':(n/1024**2).toFixed(1)+' MB';
const duration=computed(()=>{const s=v.value?.uptimeSeconds||0;return s>=86400?`${Math.floor(s/86400)}天 ${Math.floor(s%86400/3600)}小时`:`${Math.floor(s/3600)}小时 ${Math.floor(s%3600/60)}分钟`;});
const requestSuccess=computed(()=>v.value?.requests.total?((v.value.requests.total-v.value.requests.failed)/v.value.requests.total*100).toFixed(1)+'%':'暂无请求');
</script>

<template>
<view class="dashboard" :aria-busy="loading">
  <view class="overview-banner surface">
    <view><view class="banner-title"><text class="heading">系统概览</text><text class="badge" :class="loading?'neutral':tone">{{loading?'正在检测…':health}}</text></view><text class="subtle">最后刷新：{{v?date(v.checkedAt):'尚未获取'}}<text v-if="v" class="version">河豚服务 v{{v.version}}</text></text></view>
    <button class="refresh" :disabled="loading" @click="$emit('refresh')"><text class="refresh-icon">↻</text>{{loading?'检测中…':'刷新'}}</button>
  </view>
  <template v-if="v">
    <view class="summary-grid">
      <view class="surface summary"><view class="summary-icon green">◷</view><view><text class="label">运行时间</text><text class="summary-value">{{duration}}</text><text class="subtle node">{{v.host||'当前节点'}} · PID {{v.pid}}</text></view></view>
      <view class="surface summary"><view class="summary-icon blue">▦</view><view><text class="label">服务器资源</text><text class="summary-value">{{v.cpuCores}} 核</text><text class="subtle">{{memoryPercent!==null?'内存 '+memoryPercent.toFixed(1)+'% · '+bytes(memory?.usedBytes):'系统内存暂未获取'}}</text></view></view>
      <view class="surface summary"><view class="summary-icon" :class="good===connections?'green':'amber'">⇄</view><view><text class="label">服务连接状态</text><text class="summary-value">{{good}} <text class="denominator">/ {{connections}}</text></text><text class="subtle">{{good===connections?'已配置服务检测通过':'按实际连接结果统计'}}</text></view></view>
    </view>

    <view class="section-heading"><text>服务与资源监控</text><text class="section-note">当前节点 · 按需检测</text></view>
    <view class="monitor-grid">
      <view class="surface monitor">
        <view class="card-heading"><text class="card-title">运营数据库<text class="kind">{{v.database.mode==='file'?'本地文件':'MySQL'}}</text></text><text class="badge" :class="v.database.ok?'success':'danger'">{{v.database.ok?'检测通过':'异常'}}</text></view>
        <template v-if="pool">
          <view class="big-reading">{{pool.inUse}} <text>/ {{pool.maxOpen||'未设上限'}}</text><text class="reading-label">使用中 / 连接上限</text></view>
          <view class="track"><view class="fill green-fill" :style="{width:poolPercent+'%'}"/></view>
          <view class="stats four"><view><text>打开连接</text><text>{{pool.open}}</text></view><view><text>空闲连接</text><text>{{pool.idle}}</text></view><view><text>累计等待</text><text>{{pool.waitCount}} 次</text></view><view><text>等待总时长</text><text>{{pool.waitDurationMs}} ms</text></view></view>
        </template>
        <template v-else><view class="status-reading"><view class="status-symbol" :class="v.database.ok?'success':'danger'">{{v.database.ok?'✓':'!'}}</view><view><text class="status-title">{{v.database.ok?'存储可读取':'存储读取失败'}}</text><text class="subtle">当前本机使用文件存储</text></view></view><view class="inline-notice">此模式没有数据库连接池；部署 MySQL 后显示真实连接明细。</view></template>
        <view class="card-footer"><text>本次检测 {{v.database.latencyMs}} ms</text><text>状态修订 {{v.revision}}</text></view>
      </view>

      <view class="surface monitor">
        <view class="card-heading"><text class="card-title">苹果 CMS 内容源<text class="kind">HTTP</text></text><text class="badge" :class="v.cms.ok?'success':v.cms.configured?'danger':'warning'">{{v.cms.ok?'已连接':v.cms.configured?'检测异常':'未配置'}}</text></view>
        <view class="cms-address">{{v.cms.base||'等待连接你的苹果 CMS'}}</view>
        <view v-if="v.cms.configured" class="probe-list"><view v-for="check in v.cms.checks" :key="check.id" class="probe"><view><text class="dot" :class="check.ok?'green-fill':'red-fill'"/><text>{{check.label}}</text></view><text :class="check.ok?'positive':'negative'">{{check.ok?'通过':'失败'}} · {{check.latencyMs}} ms</text></view></view>
        <view v-else class="empty-source"><text class="empty-icon">⇄</text><text>尚未连接内容服务</text><text class="subtle">前往「内容源与连接」自动寻找或填写地址</text></view>
        <text class="cms-message" :class="{'negative':v.cms.configured&&!v.cms.ok}">{{v.cms.message}}</text>
        <view class="card-footer"><text>{{v.cms.configured?'本次检测 '+v.cms.latencyMs+' ms':'未发起网络探测'}}</text><text>分类 / 影片接口</text></view>
      </view>

      <view class="surface monitor">
        <view class="card-heading"><text class="card-title">Redis 缓存</text><text class="badge" :class="v.redis.ok?'success':v.redis.configured?'danger':'warning'">{{v.redis.ok?'已连接':v.redis.configured?'连接异常':'未配置'}}</text></view>
        <view class="big-reading">{{redisUsed}} <text>/ {{v.redis.pool?.max??'—'}}</text><text class="reading-label">使用中 / 连接上限</text></view>
        <view class="track"><view class="fill green-fill" :style="{width:redisPercent+'%'}"/></view>
        <view class="stats four"><view><text>打开连接</text><text>{{v.redis.pool?.total??'—'}}</text></view><view><text>空闲连接</text><text>{{v.redis.pool?.idle??'—'}}</text></view><view><text>缓存命中 / 未命中</text><text>{{v.redis.cacheHits??0}} / {{v.redis.cacheMisses??0}}</text></view><view><text>缓存异常</text><text>{{v.redis.cacheErrors??0}}</text></view></view>
        <view class="card-footer"><text>{{v.redis.message}}</text><text>{{v.redis.configured?(v.redis.latencyMs+' ms'):'等待配置'}}</text></view>
      </view>
      <view class="surface monitor">
        <view class="card-heading"><text class="card-title">内容源 HTTP 客户端</text><text class="badge" :class="v.cms.ok?'success':'warning'">{{v.cms.ok?'检测通过':'等待内容源检测通过'}}</text></view>
        <view class="big-reading">{{v.http.busiestActive}} <text>/ {{v.http.maxPerHost}}</text><text class="reading-label">最繁忙域名请求 / 单域名连接上限</text></view>
        <view class="request-context"><text>当前请求 {{v.http.active}}</text><text>最繁忙域名 {{v.http.busiestHost}}</text></view>
        <view class="stats four"><view><text>累计请求</text><text>{{v.http.total}}</text></view><view><text>异常请求</text><text>{{v.http.failed}}</text></view><view><text>平均耗时</text><text>{{v.http.averageMs.toFixed(1)}} ms</text></view><view><text>连接复用率</text><text>{{v.http.reusePercent.toFixed(1)}}%</text></view></view>
        <view class="card-footer"><text>真实出站请求 · 重启归零</text><text>单域名上限 {{v.http.maxPerHost}}</text></view>
      </view>
    </view>
    <ConnectionDetails :overview="v"/>
    <view class="surface details">
      <button class="details-toggle" :aria-expanded="expanded" @click="expanded=!expanded"><text>服务器与运行明细</text><text>{{expanded?'⌃':'⌄'}}</text></button>
      <view v-if="expanded" class="detail-grid">
        <view><text>业务请求 / 成功率</text><text>{{v.requests.total}} / {{requestSuccess}}</text></view><view><text>物理内存 / Go 堆</text><text>{{bytes(memory?.usedBytes)}} / {{bytes(v.heapBytes)}}</text></view><view><text>操作系统</text><text>{{v.os}} / {{v.arch}}</text></view><view><text>当前节点</text><text>{{v.host||'未获取'}}</text></view>
        <view><text>运行版本</text><text>河豚 v{{v.version}} · {{v.goVersion}}</text></view><view><text>进程编号</text><text>{{v.pid}}</text></view>
        <view><text>启动时间</text><text>{{date(v.startedAt)}}</text></view><view><text>Go 运行时内存</text><text>{{bytes(v.runtimeBytes)}}</text></view>
        <view><text>CPU 核心数</text><text>{{v.cpuCores}} 核</text></view><view><text>存储模式</text><text>{{v.database.mode==='file'?'本地文件 · 本机查看环境':'MySQL'}}</text></view>
      </view>
      <text v-if="expanded" class="detail-note">CPU 使用率尚未采集。接口检测反映本次检查结果，业务请求统计自进程启动后累计。</text>
    </view>
  </template>
  <view v-else class="surface initial-state">{{loading?'正在读取服务器信息并检测服务连接…':'未获取概览数据，请点击刷新重试。'}}</view>
</view>
</template>

<style scoped>
.dashboard{--dash-success:#278151;--dash-success-bg:rgba(76,175,80,.1);--dash-warning:#a7701e;--dash-warning-bg:rgba(222,159,56,.12);--dash-danger:#cc5858;color:var(--studio-text);font-size:14px;font-variant-numeric:tabular-nums;line-height:1.5;padding-bottom:24px}
.surface{background:var(--studio-sidebar);border:1px solid var(--studio-border);border-radius:13px;box-shadow:0 7px 22px rgba(20,43,66,.035);min-width:0}
.overview-banner{padding:23px 26px;display:flex;align-items:center;justify-content:space-between;gap:18px;margin-bottom:20px}.banner-title{display:flex;align-items:center;gap:13px;flex-wrap:wrap;margin-bottom:7px}.heading{font-size:25px;font-weight:700;letter-spacing:-.5px}.subtle{display:block;color:var(--studio-muted);font-size:12px;line-height:1.7}.version{margin-left:18px}.badge{display:inline-flex;align-items:center;white-space:nowrap;align-self:flex-start;font-size:11px;font-weight:600;line-height:1.4;padding:4px 9px;border-radius:20px}.success{color:var(--dash-success);background:var(--dash-success-bg)}.warning{color:var(--dash-warning);background:var(--dash-warning-bg)}.danger{color:var(--dash-danger);background:rgba(204,88,88,.1)}.neutral{color:var(--studio-muted);background:var(--studio-input)}
.refresh{display:flex;align-items:center;gap:7px;border:0;border-radius:9px;background:#2f6be9;color:white;font-size:13px;line-height:1.5;font-weight:600;margin:0;padding:10px 18px;box-shadow:0 3px 7px rgba(47,107,233,.15)}.refresh::after,.details-toggle::after{border:none}.refresh[disabled]{opacity:.6}.refresh-icon{font-size:20px;line-height:1}.refresh:active{transform:translateY(1px)}
.summary-grid{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:20px;margin-bottom:26px}.summary{padding:25px 23px;display:flex;align-items:center;gap:17px;min-height:90px}.summary>view:last-child{min-width:0}.summary-icon{height:50px;width:50px;border-radius:12px;display:flex;align-items:center;justify-content:center;font-size:28px;color:white;flex-shrink:0}.green{background:linear-gradient(135deg,#55bf55,#36c58c)}.blue{background:linear-gradient(135deg,#368bf6,#68adff)}.amber{background:linear-gradient(135deg,#d5a04a,#e1b466)}.label{display:block;font-size:13px;color:var(--studio-muted);margin-bottom:5px}.summary-value{display:block;font-size:24px;font-weight:700;letter-spacing:-.5px;margin-bottom:5px}.denominator{font-size:23px}.node{overflow-wrap:anywhere}
.section-heading{display:flex;align-items:center;justify-content:space-between;gap:14px;font-size:16px;font-weight:650;margin:0 0 15px}.section-note{font-size:11px;font-weight:400;color:var(--studio-muted)}.monitor-grid{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:20px}.monitor{padding:22px 23px;display:flex;flex-direction:column;min-height:252px}.card-heading{display:flex;align-items:center;justify-content:space-between;gap:12px;margin-bottom:22px}.card-title{font-size:15px;font-weight:650}.kind{font-size:10px;font-weight:400;margin-left:9px;color:var(--studio-muted)}.big-reading{font-size:29px;font-weight:700;display:flex;align-items:baseline;gap:8px;flex-wrap:wrap;line-height:1.3;margin-bottom:17px}.big-reading>text:not(.reading-label){font-size:24px}.big-reading .reading-label{font-size:11px;font-weight:400;color:var(--studio-muted);margin-left:3px}.track{height:8px;border-radius:10px;background:var(--studio-input);overflow:hidden;margin-bottom:23px}.fill{height:100%;border-radius:10px}.green-fill{background:#49b979}.blue-fill{background:#5599f0}.amber-fill{background:#d8a34a}.red-fill{background:#d56f6f}.stats{display:grid;gap:12px;margin:0 0 22px}.four{grid-template-columns:repeat(4,minmax(0,1fr))}.stats>view>text{display:block;font-size:11px;color:var(--studio-muted);margin-bottom:9px}.stats>view>text:last-child{font-size:13px;color:var(--studio-text);font-weight:650;margin:0;overflow-wrap:anywhere}.stats>view>text.negative,.negative{color:var(--dash-danger)}.positive{color:var(--dash-success)}.card-footer{display:flex;justify-content:space-between;gap:12px;flex-wrap:wrap;border-top:1px solid var(--studio-border);padding-top:13px;margin-top:auto;color:var(--studio-muted);font-size:10px}.status-reading{display:flex;align-items:center;gap:14px;margin:0 0 20px}.status-symbol{width:43px;height:43px;border-radius:50%;display:flex;justify-content:center;align-items:center;font-size:23px}.status-title{display:block;font-size:18px;font-weight:650;margin-bottom:5px}.inline-notice{font-size:12px;line-height:1.8;color:var(--studio-muted);padding:11px 13px;border-radius:7px;background:var(--studio-input);margin-bottom:20px}.cms-address{font-size:12px;overflow-wrap:anywhere;color:var(--studio-muted);margin-bottom:14px}.probe-list{margin-bottom:12px}.probe{display:flex;align-items:center;justify-content:space-between;padding:10px 0;border-bottom:1px solid var(--studio-border);font-size:12px;gap:12px}.probe>view{display:flex;align-items:center;gap:8px}.dot{display:inline-block;width:7px;height:7px;border-radius:50%}.cms-message{font-size:11px;color:var(--studio-muted);margin-bottom:16px}.empty-source{display:flex;flex-direction:column;align-items:center;text-align:center;gap:4px;padding:7px 0 16px;font-size:13px}.empty-icon{font-size:27px;color:var(--studio-muted);line-height:1.3}.request-context{display:flex;justify-content:space-between;gap:14px;font-size:11px;color:var(--studio-muted);background:var(--studio-input);padding:8px 11px;border-radius:7px;margin-bottom:21px}
.details{margin-top:22px;overflow:hidden}.details-toggle{width:100%;display:flex;justify-content:space-between;align-items:center;margin:0;padding:19px 23px;color:inherit;background:transparent;font-size:14px;font-weight:650;border:0;border-radius:0;line-height:1.6;text-align:left}.detail-grid{display:grid;grid-template-columns:1fr 1fr;padding:0 23px 8px}.detail-grid>view{display:grid;grid-template-columns:135px minmax(0,1fr);font-size:12px;border-top:1px solid var(--studio-border);min-height:42px}.detail-grid>view>text{padding:12px 13px;overflow-wrap:anywhere}.detail-grid>view>text:first-child{background:var(--studio-input);color:var(--studio-muted)}.detail-note{display:block;font-size:10px;color:var(--studio-muted);padding:7px 23px 20px;line-height:1.8}.initial-state{padding:45px 25px;color:var(--studio-muted)}
@media(prefers-reduced-motion:reduce){.refresh:active{transform:none}}
@media(max-width:1200px){.summary{padding:20px 16px;gap:12px}.summary-icon{width:43px;height:43px}.summary-value{font-size:21px}.summary-grid{gap:14px}.monitor-grid{gap:16px}.monitor{padding:20px}.detail-grid>view{grid-template-columns:105px minmax(0,1fr)}}
@media(max-width:900px){.summary-grid{grid-template-columns:1fr}.summary{min-height:0}.summary>view:last-child{flex:1}.monitor-grid,.detail-grid{grid-template-columns:1fr}.version{display:block;margin-left:0}.heading{font-size:23px}.four{grid-template-columns:repeat(2,minmax(0,1fr))}.overview-banner{padding:19px}.monitor{min-height:240px}.section-note{display:none}}
</style>
