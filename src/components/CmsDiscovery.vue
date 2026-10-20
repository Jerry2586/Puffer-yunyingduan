<script setup lang="ts">
import {ref,onMounted} from 'vue';
import {api,type CmsDiscovery} from '@/services/api';
const emit=defineEmits<{connected:[base:string]}>();
const busy=ref(false),result=ref<CmsDiscovery|null>(null),error=ref('');
async function discover(){if(busy.value)return;busy.value=true;error.value='';try{result.value=await api.discoverCms();if(result.value.connectedBase)emit('connected',result.value.connectedBase);}catch(e){error.value=(e as Error).message;}finally{busy.value=false;}}
async function choose(base:string){busy.value=true;error.value='';try{await api.cms(base);emit('connected',base);if(result.value)result.value.connectedBase=base;}catch(e){error.value=(e as Error).message;}finally{busy.value=false;}}
onMounted(discover);
</script>
<template><view class="discovery"><button class="scan" :disabled="busy" @click="discover">{{busy?'正在服务器上寻找并检测…':'自动寻找并连接 CMS'}}</button><text class="hint">自动检查同服务器上的苹果 CMS；一个可用站点自动连接，多个可用站点选择一次。</text><text v-if="result?.connectedBase" class="hint success">已连接：{{result.connectedBase}}</text><view v-for="site in result?.candidates||[]" :key="site.id" class="site"><view><text>{{site.name}}</text><text class="hint">{{site.base||'站点未填写公开域名，请在 CMS 站点配置中补充'}}</text><text class="hint">{{site.report?.message||'暂无可检测地址'}}</text></view><button v-if="site.report?.ok&&result?.connectedBase!==site.base" :disabled="busy" @click="choose(site.base)">连接此站</button></view><text v-for="warning in result?.warnings||[]" :key="warning" class="hint">{{warning}}</text><text v-if="error" class="failure">{{error}}</text></view></template>
<style scoped>.discovery{padding:14px 0 22px}.scan{padding:12px;background:#8066d7;color:#fff;border-radius:9px;font-size:13px}.hint{display:block;font-size:12px;line-height:1.8;color:var(--studio-muted,#98a5b5);overflow-wrap:anywhere;margin-top:8px}.success{color:#65aa83}.site{display:flex;align-items:center;gap:12px;border-bottom:1px solid var(--studio-border,#35415a);padding:14px 0}.site>view{flex:1;min-width:0}.site button{font-size:12px;padding:8px;background:var(--studio-input,#1b2735);color:inherit}.failure{display:block;color:#d95a5a;margin-top:12px}</style>
