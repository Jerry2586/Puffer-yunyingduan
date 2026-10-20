<script setup lang="ts">
import {computed,ref} from 'vue';
import type {SystemOverview} from '@/services/api';
const props=defineProps<{overview:SystemOverview}>();
const expanded=ref(true);
const bytes=(n?:number)=>n===undefined?'未获取':(n/1024/1024).toFixed(2)+' MB';
const cards=computed(()=>{
 const v=props.overview,d=v.database,r=v.redis,h=v.http;
 return [
 {title:'河豚运营数据库',ok:d.ok,status:d.ok?'已连接':'连接异常',rows:d.mode==='mysql'?[
 ['版本',d.version||'未获取'],['地址',d.address||'未获取'],['数据库',d.name||'未获取'],['用户',d.username||'未获取'],['连接池',`最大打开 ${d.pool?.maxOpen??'—'}，最大空闲 ${d.maxIdle??'—'}，连接生存 ${Math.round((d.lifetimeSeconds||0)/60)} 分钟`]
 ]:[['存储方式','本机文件'],['检查结果',d.ok?'文件可读取':'读取失败'],['连接池','文件存储不使用数据库连接池']]},
 {title:'苹果 CMS 内容源',ok:v.cms.ok,status:v.cms.ok?'已连接':v.cms.configured?'检测异常':'未配置',rows:[['连接方式','HTTP 采集接口'],['接口地址',v.cms.base||'尚未配置'],['分类接口',v.cms.checks.find(x=>x.id==='categories')?.ok?'通过':'未通过 / 未检测'],['影片接口',v.cms.checks.find(x=>x.id==='catalog')?.ok?'通过':'未通过 / 未检测'],['数据库连接','当前通过 HTTP 对接，不直连 CMS 数据库']]},
 {title:'Redis',ok:r.ok,status:r.ok?'已连接':r.configured?'连接异常':'未配置',rows:[['版本',r.version||(r.ok?'INFO 信息未获取':'未获取')],['地址',r.address||'尚未配置'],['数据库 / 用户',r.configured?`${r.db} / ${r.username||'默认用户'}`:'—'],['已用 / 峰值内存',`${bytes(r.used_memory)} / ${bytes(r.used_memory_peak)}`],['连接池',r.pool?`连接上限 ${r.pool.max}，最小空闲 ${r.pool.minIdle}，最大空闲 ${r.pool.maxIdle}`:'—'],['目录缓存',r.configured?`有效期 ${r.ttlSeconds} 秒；仅缓存成功的公共影片目录`:'尚未启用']]},
 {title:'内容源 HTTP 客户端',ok:v.cms.ok,status:v.cms.ok?'检测通过':v.cms.configured?'检测异常':'等待内容源',rows:[['请求超时',`${h.timeoutSeconds} 秒`],['空闲连接保留',`${h.idleSeconds} 秒`],['全局空闲连接',String(h.maxIdle)],['单域名空闲连接',String(h.maxIdlePerHost)],['单域名最大连接',String(h.maxPerHost)],['监控采样',h.sampling]]}
 ];
});
</script>
<template>
 <view class="connection-details">
  <button class="toggle" :aria-expanded="expanded" @click="expanded=!expanded"><text>连接配置明细</text><text>{{expanded?'⌃':'⌄'}}</text></button>
  <view v-if="expanded" class="connections-grid">
   <view v-for="card in cards" :key="card.title" class="connection-card">
    <view class="card-top"><text>{{card.title}}</text><text class="status" :class="card.ok?'ready':'pending'">{{card.status}}</text></view>
    <view class="config-table"><view v-for="row in card.rows" :key="row[0]" class="config-row"><text class="config-label">{{row[0]}}</text><text class="config-value">{{row[1]}}</text></view></view>
   </view>
  </view>
 </view>
</template>
<style scoped>
.connection-details{margin-top:24px;background:var(--studio-sidebar);border:1px solid var(--studio-border);border-radius:13px;overflow:hidden}.toggle{display:flex;justify-content:space-between;align-items:center;width:100%;padding:20px 23px;margin:0;background:transparent;color:var(--studio-text);border:0;border-radius:0;font-size:16px;font-weight:650;text-align:left;line-height:1.6}.toggle::after{border:0}.connections-grid{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:20px;padding:15px 23px 24px}.connection-card{border:1px solid var(--studio-border);border-radius:11px;padding:22px;min-width:0}.card-top{display:flex;justify-content:space-between;gap:12px;align-items:center;font-size:16px;font-weight:650;margin-bottom:20px}.status{font-size:12px;padding:3px 9px;border-radius:20px;white-space:nowrap}.ready{color:#278151;background:rgba(76,175,80,.1)}.pending{color:#a7701e;background:rgba(222,159,56,.12)}.config-row{display:grid;grid-template-columns:120px minmax(0,1fr);font-size:13px;line-height:1.8;min-height:46px}.config-label{padding:12px 14px;background:var(--studio-input);font-weight:600;color:var(--studio-muted)}.config-value{padding:12px 16px;color:var(--studio-text);overflow-wrap:anywhere}@media(max-width:1050px){.connections-grid{grid-template-columns:1fr}}@media(max-width:600px){.connections-grid{padding:8px 12px 16px}.connection-card{padding:15px}.config-row{grid-template-columns:90px minmax(0,1fr)}.config-label,.config-value{padding:10px}.card-top{font-size:14px}}
</style>
