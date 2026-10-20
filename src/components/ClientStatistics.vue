<script setup lang="ts">
import {computed,onMounted,onUnmounted,ref} from 'vue';
import type {Platform} from '@/domain/types';
import {clientStatisticsApi,type ClientStatisticsReport} from '@/services/api';
const props=defineProps<{platform:Platform}>();
const names:Record<Platform,string>={mobile:'手机客户端',tablet:'平板客户端',tv:'电视客户端',web:'电脑客户端'};
const name=computed(()=>names[props.platform]);
const report=ref<ClientStatisticsReport|null>(null),loading=ref(false),error=ref('');
let alive=true;
async function load(){if(loading.value)return;loading.value=true;error.value='';try{const data=await clientStatisticsApi.get(props.platform);if(alive)report.value=data;}catch(e){if(alive)error.value=(e as Error).message;}finally{if(alive)loading.value=false;}}
onMounted(load);onUnmounted(()=>{alive=false;});
const rate=computed(()=>report.value?.requests.total?((report.value.requests.successful/report.value.requests.total)*100).toFixed(1)+'%':'—');
const date=(value:string)=>value?new Date(value).toLocaleString():'尚未发布';
</script>
<template>
<view class="statistics">
 <view class="heading"><view><text class="eyebrow">CLIENT ANALYTICS</text><text class="title">{{name}} · 数据统计</text><text class="muted">独立统计当前客户端的真实接口请求与已保存业务数据。</text></view><button :disabled="loading" @click="load">{{loading?'读取中…':'刷新数据'}}</button></view>
 <text v-if="error" class="error" role="alert">{{error}}</text>
 <template v-if="report">
 <view class="cards"><view class="card"><text>接口请求</text><strong>{{report.requests.total}}</strong><text>本次服务启动以来</text></view><view class="card"><text>成功率</text><strong>{{rate}}</strong><text>失败 {{report.requests.failed}} 次 · HTTP ≥ 400</text></view><view class="card"><text>平均响应</text><strong>{{report.requests.averageMs.toFixed(1)}} <small>ms</small></strong><text>服务端处理耗时</text></view><view class="card"><text>运营发布版本</text><strong>v{{report.business.publishedVersion}}</strong><text>{{date(report.business.publishedAt)}}</text></view></view>
 <view class="panel"><text class="section-title">接口明细</text><view class="row table-head"><text>接口</text><text>请求</text><text>失败</text><text>平均耗时</text></view><view v-for="row in report.requests.endpoints" :key="row.endpoint" class="row"><text>{{row.endpoint}}</text><text>{{row.total}}</text><text>{{row.failed}}</text><text>{{row.averageMs.toFixed(1)}} ms</text></view><text v-if="!report.requests.endpoints.length" class="empty">尚无此端请求。客户端实际访问后显示统计。</text></view>
 <view class="panel"><text class="section-title">已保存业务数据</text><view class="business"><view><strong>{{report.business.releases}}</strong><text>安装包版本记录</text></view><view><strong>{{report.business.advertising.impressions}}</strong><text>广告展示回传</text></view><view><strong>{{report.business.advertising.clicks}}</strong><text>广告点击回传</text></view><view><strong>{{report.business.advertising.rewards}}</strong><text>激励发放记录</text></view></view></view>
 <text class="muted footnote">请求统计起点：{{date(report.startedAt)}}，服务重启后重新计数；业务数据从持久存储读取。广告汇总按现有保留日记录计算（通常最近 90 天）；展示、点击来自客户端事件回传，不代表平台结算收入。更新时间：{{date(report.checkedAt)}}。</text>
 </template><text v-else-if="loading" class="empty">正在读取真实统计…</text>
</view>
</template>
<style scoped>
.statistics{display:flex;flex-direction:column;gap:24px;padding:28px;max-width:1500px;margin:auto;color:inherit}.heading{display:flex;justify-content:space-between;align-items:center;gap:24px}.heading>view{display:flex;flex-direction:column;gap:10px}.eyebrow{font-size:11px;letter-spacing:2px;opacity:.55}.title{font-size:25px;font-weight:650}.muted{font-size:13px;opacity:.65;line-height:1.8}.heading button{margin:0;font-size:13px;border:1px solid #80808040;background:transparent;color:inherit;border-radius:10px}.cards{display:grid;grid-template-columns:repeat(4,minmax(0,1fr));gap:18px}.card,.panel{border:1px solid #80808030;border-radius:16px;background:#80808009;padding:24px}.card{display:flex;flex-direction:column;gap:14px}.card>text{font-size:12px;opacity:.65}.card strong{font-size:32px;font-weight:650}.card small{font-size:14px}.section-title{display:block;font-size:16px;font-weight:600;margin-bottom:20px}.row{display:grid;grid-template-columns:2fr 1fr 1fr 1fr;gap:12px;padding:14px 0;border-bottom:1px solid #80808022;font-size:13px}.table-head{opacity:.55;font-size:12px}.empty{display:block;padding:28px;text-align:center;opacity:.6}.business{display:grid;grid-template-columns:repeat(4,1fr);gap:20px}.business>view{display:flex;flex-direction:column;gap:12px}.business strong{font-size:24px}.business text{font-size:12px;opacity:.65}.error{color:#e87979}.footnote{font-size:12px}@media(max-width:1000px){.cards,.business{grid-template-columns:repeat(2,minmax(0,1fr))}.statistics{padding:16px}}
</style>
