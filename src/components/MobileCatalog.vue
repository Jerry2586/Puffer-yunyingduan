<script setup lang="ts">
import {computed,ref,watch} from 'vue';
import type {Theme,Vod,Category,Section,AppPage} from '@/domain/types';
import {assetUrl} from '@/services/api';
import FilmModule from './FilmModule.vue';
import AppEmptyState from './AppEmptyState.vue';
const props=defineProps<{theme:Theme;page:AppPage;items:Vod[];discovery:Vod[];categories:Category[];categoryId:string;query:string;attempted:boolean;loading:boolean;error:string;sections:Section[];sectionItems:Record<string,Vod[]>;editing?:boolean;selected?:string}>();
const emit=defineEmits<{category:[id:string];search:[query:string];open:[film:Vod];play:[film:Vod];episode:[film:Vod,index:number];select:[id:string];more:[section:Section]}>();
const sort=ref('latest'),area=ref(''),year=ref('');
const history=ref<string[]>([]);
try {const saved=uni.getStorageSync('hetun-search-history');if(Array.isArray(saved))history.value=saved.filter(v=>typeof v==='string').slice(0,12);}catch{}
watch(()=>[props.attempted,props.query],()=>{if(!props.attempted||!props.query.trim())return;history.value=[props.query.trim(),...history.value.filter(v=>v!==props.query.trim())].slice(0,12);uni.setStorageSync('hetun-search-history',history.value);});
watch(()=>props.categoryId,()=>{area.value='';year.value='';});
const browsing=computed(()=>props.page==='search'&&!props.attempted?props.discovery.filter(v=>!props.categoryId||v.categoryId===props.categoryId):props.items);
const areas=computed(()=>[...new Set(browsing.value.map(v=>v.area).filter(Boolean))]);
const years=computed(()=>[...new Set(browsing.value.map(v=>v.year).filter(Boolean))].sort().reverse());
const list=computed(()=>{let rows=browsing.value.filter(v=>(!area.value||v.area===area.value)&&(!year.value||v.year===year.value));if(sort.value==='score'||props.page==='ranking')rows.sort((a,b)=>Number(b.score||0)-Number(a.score||0));if(sort.value==='year')rows.sort((a,b)=>b.year.localeCompare(a.year));return rows;});
const suggestions=computed(()=>props.discovery.slice(0,6));
const generic=computed<Section>(()=>({id:'mobile-catalog',title:'',subtitle:'',kind:'poster',style:props.page==='ranking'?'ranking':'grid3',source:'latest',categoryId:'',limit:list.value.length,columns:3,enabled:true,showMore:false,showScore:false,showNote:true,showYear:true,radius:3,gap:4,ratio:'3/4',orderBy:'latest'}));
function clearHistory(){history.value=[];uni.removeStorageSync('hetun-search-history');}
</script>
<template>
 <view class="mobile-catalog" :class="'catalog-'+page">
  <template v-if="page==='search'&&!attempted">
   <view v-if="history.length" class="discovery-block"><view class="discovery-heading"><text>历史搜索</text><text role="button" @click="clearHistory">清空</text></view><view class="word-chips"><text v-for="word in history" :key="word" @click="emit('search',word)">{{word}}</text></view></view>
   <view v-if="suggestions.length" class="discovery-block"><view class="discovery-heading"><text>搜索发现</text></view><view class="word-chips"><text v-for="v in suggestions" :key="v.id" @click="emit('search',v.title)">{{v.title}}</text></view></view>
   <view class="discovery-heading discovery-tabs"><text class="active">精选内容</text></view>
  </template>
  <template v-if="page!=='search'||!attempted">
   <scroll-view scroll-x class="filter-scroll"><view class="filter-row categories"><text :class="{active:!categoryId}" @click="emit('category','')">全部</text><text v-for="c in categories" :key="c.id" :class="{active:categoryId===c.id}" @click="emit('category',c.id)">{{c.name}}</text></view></scroll-view>
   <template v-if="page==='category'">
    <scroll-view v-if="areas.length>1" scroll-x class="filter-scroll"><view class="filter-row"><text :class="{active:!area}" @click="area=''">全部地区</text><text v-for="a in areas" :key="a" :class="{active:area===a}" @click="area=a">{{a}}</text></view></scroll-view>
    <scroll-view v-if="years.length>1" scroll-x class="filter-scroll"><view class="filter-row"><text :class="{active:!year}" @click="year=''">全部年份</text><text v-for="y in years" :key="y" :class="{active:year===y}" @click="year=y">{{y}}</text></view></scroll-view>
    <view class="filter-row"><text :class="{active:sort==='latest'}" @click="sort='latest'">默认排序</text><text :class="{active:sort==='score'}" @click="sort='score'">评分排序</text><text :class="{active:sort==='year'}" @click="sort='year'">年份排序</text></view>
    <text v-if="area||year" class="filter-hint">已筛选当前加载的影片</text>
   </template>
   <view v-if="page==='ranking'" class="filter-row"><text class="active">评分榜</text><text class="ranking-hint">按影片评分排序</text></view>
  </template>
  <view v-if="page==='search'&&attempted&&!loading&&!error" class="search-results">
   <view v-for="v in list" :key="v.id" class="result-card" @click="emit('open',v)">
    <view class="result-top"><image :src="assetUrl(v.poster)" mode="aspectFill"/><view class="result-copy"><text class="result-title">{{v.title}}</text><text class="result-meta">{{[v.score&&v.score+'分',v.year,v.area,v.category].filter(Boolean).join(' · ')}}</text><text v-if="v.actors" class="result-meta">{{v.actors}}</text><text class="result-description">{{v.description||v.note}}</text></view></view>
    <view v-if="v.sources[0]?.episodes.length" class="result-episodes"><text v-for="(episode,i) in v.sources[0].episodes.slice(0,4)" :key="i" @click.stop="emit('episode',v,i)">{{episode.name}}</text><text @click.stop="emit('open',v)">选集 ›</text></view>
    <button class="result-play" @click.stop="emit('play',v)">立即播放</button>
   </view>
   <AppEmptyState v-if="!list.length" title="没有找到相关影片" description="试试其他片名或关键词" icon="search"/>
  </view>
  <template v-else-if="!loading&&!error">
   <FilmModule v-for="s in sections" :key="s.id" :theme="theme" :section="s" :items="sectionItems[s.id]||list" :editing="editing" :selected="selected===s.id" @open="emit('open',$event)" @select="emit('select',$event)" @more="emit('more',$event)"/>
   <FilmModule v-if="!sections.length" :theme="theme" :section="generic" :items="list" @open="emit('open',$event)"/>
  </template>
 </view>
</template>
<style scoped>
.mobile-catalog{padding-bottom:14px}.filter-scroll{width:100%;white-space:nowrap}.filter-row{display:flex;align-items:center;gap:8px;padding:7px 8px;color:var(--muted);font-size:12px;white-space:nowrap}.filter-row>text{padding:4px 9px;border-radius:14px;flex-shrink:0;cursor:pointer}.filter-row .active{background:var(--surface);color:var(--text);font-weight:600}.categories{font-size:14px;padding:8px 7px}.categories .active{background:transparent;color:var(--text)}.filter-hint{display:block;margin:5px 14px;font-size:10px;color:var(--muted)}.filter-row .ranking-hint{font-size:10px;padding:0}.discovery-block{padding:12px 12px 5px}.discovery-heading{display:flex;align-items:center;justify-content:space-between;font-size:14px;font-weight:600;margin:0 0 13px}.discovery-heading>text+text{font-weight:400;font-size:10px;color:var(--muted);cursor:pointer}.word-chips{display:flex;gap:9px;flex-wrap:wrap}.word-chips>text{font-size:12px;padding:7px 12px;border-radius:18px;background:var(--surface);cursor:pointer}.discovery-tabs{padding:22px 12px 0;font-size:17px}.search-results{padding:0 12px}.result-card{padding:18px 0;border-bottom:1px solid var(--mobile-line);cursor:pointer}.result-top{display:flex;gap:12px}.result-top>image{width:91px;height:124px;flex-shrink:0;border-radius:4px;background:var(--surface)}.result-copy{flex:1;min-width:0}.result-title{display:block;font-size:17px;font-weight:600;margin:2px 0 9px;line-height:23px}.result-meta{display:block;font-size:10px;color:var(--muted);line-height:19px;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}.result-description{font-size:11px;line-height:18px;color:var(--muted);display:-webkit-box;-webkit-line-clamp:2;-webkit-box-orient:vertical;overflow:hidden;margin-top:7px}.result-episodes{display:flex;gap:7px;overflow:auto;margin:12px 0}.result-episodes>text{background:var(--surface);padding:8px 12px;border-radius:3px;font-size:11px;white-space:nowrap}.result-play{background:var(--accent);color:#052d35;border-radius:4px;font-size:13px;line-height:35px;margin:13px 0 0;padding:0;width:100%;font-weight:600}.result-play::after{border:0}.mobile-catalog :deep(.film-module){padding:10px 8px 0}.catalog-ranking :deep(.films){gap:16px}.catalog-ranking :deep(.poster){width:84px;aspect-ratio:3/4}.catalog-ranking :deep(.film-copy){flex:1}.catalog-ranking :deep(.film-title){font-size:15px;margin:0 0 9px;line-height:22px}.catalog-ranking :deep(.year){font-size:11px}.catalog-ranking :deep(.rank){background:none;left:3px;top:2px;padding:0;font-size:28px;line-height:32px;color:#fff;font-style:italic;text-shadow:1px 1px 0 #208bfa,2px 2px 0 #208bfa;font-weight:900}.catalog-ranking :deep(.description){font-size:11px;line-height:19px;margin-top:9px;-webkit-line-clamp:2}
</style>
