<script setup lang="ts">
import {computed} from 'vue';
import type {CmsStatus} from '@/services/api';
const props=defineProps<{report:CmsStatus|null;title:string;checking?:boolean}>();
const label=computed(()=>props.checking?'正在检测…':({ready:'已接通',empty:'已接通 · 内容为空',error:'检测失败',unconfigured:'未配置'}[props.report?.status||'unconfigured']));
const time=computed(()=>props.report?.checkedAt?new Date(props.report.checkedAt).toLocaleString('zh-CN',{hour12:false}):'尚未检测');
</script>
<template>
  <view class="cms-report" :class="report?.status" aria-live="polite">
    <view class="cms-report-heading"><text>{{title}}</text><text class="cms-report-badge">{{label}}</text></view>
    <text v-if="report?.base" class="cms-report-address">{{report.base}}</text>
    <text class="cms-report-message">{{report?.message||'填写苹果 CMS 站点地址后检测连接。'}}</text>
    <view v-for="check in report?.checks||[]" :key="check.id" class="cms-check-row">
      <view><text class="cms-check-name">{{check.label}}</text><text class="cms-check-value" v-if="check.ok">{{check.id==='categories'?check.count+' 个分类':(check.total===null?'总数未返回':check.total+' 部影片')+' · 首批 '+check.count+' 部'}}</text><text class="cms-check-error" v-else>{{check.error}}</text></view>
      <text class="cms-check-result">{{check.ok?'通过':'失败'}} · {{check.latencyMs}} ms</text>
    </view>
    <text v-if="report" class="cms-check-time">最近检测：{{time}} · 耗时 {{report.latencyMs}} ms</text>
  </view>
</template>
<style scoped>
.cms-report{border:1px solid var(--studio-border);border-radius:12px;padding:16px;background:var(--studio-input);margin-top:16px}.cms-report-heading{display:flex;gap:12px;align-items:center;justify-content:space-between;font-size:13px;font-weight:600;color:var(--studio-text)}.cms-report-badge{font-size:11px;font-weight:400;color:var(--studio-muted);white-space:nowrap}.ready .cms-report-badge{color:var(--studio-link)}.empty .cms-report-badge{color:#b7933d}.error .cms-report-badge,.cms-check-error{color:#d46c61}.cms-report-address{display:block;margin-top:12px;font-size:11px;color:var(--studio-muted);overflow-wrap:anywhere;line-height:1.6}.cms-report-message{display:block;font-size:12px;line-height:1.7;margin:10px 0;color:var(--studio-muted)}.cms-check-row{display:flex;justify-content:space-between;align-items:flex-start;gap:10px;border-top:1px solid var(--studio-border);padding:12px 0;font-size:11px}.cms-check-name{display:block;color:var(--studio-text);font-weight:500}.cms-check-value,.cms-check-error{display:block;margin-top:5px;line-height:1.7}.cms-check-value,.cms-check-result{color:var(--studio-muted)}.cms-check-result{white-space:nowrap}.cms-check-time{display:block;color:var(--studio-muted);font-size:10px;line-height:1.7;border-top:1px solid var(--studio-border);padding-top:10px}
</style>
