<script setup lang="ts">
import {computed,ref,watch} from 'vue';
import type {Vod} from '@/domain/types';
import AppIcon from './AppIcon.vue';
const props=defineProps<{film:Vod;source:number;episode:number;colors:{muted:string;text:string}}>();
const emit=defineEmits<{source:[index:number];episode:[index:number]}>();
const expanded=ref(false);
const episodes=computed(()=>props.film.sources[props.source]?.episodes||[]);
watch(()=>props.film.id,()=>{expanded.value=false;});
function chooseEpisode(index:number){expanded.value=false;emit('episode',index);}
</script>
<template>
 <view v-if="film.sources.length" class="episode-list">
  <view class="episode-heading"><text>选集</text><text class="episode-note">{{film.note}}</text><button class="all-episodes" aria-label="打开全部选集" @click="expanded=true">全部 <AppIcon name="chevron" :size="12" :color="colors.muted"/></button></view>
  <scroll-view scroll-x class="line-scroll" :show-scrollbar="false"><view class="line-row"><button v-for="(line,index) in film.sources" :key="index" :class="{chosen:source===index}" :aria-label="'切换播放线路'+line.name" @click="emit('source',index)">{{line.name}}</button></view></scroll-view>
  <scroll-view scroll-x class="episode-scroll" :show-scrollbar="false" :scroll-into-view="'detail-episode-'+episode"><view class="episode-row"><button v-for="(row,index) in episodes" :id="'detail-episode-'+index" :key="index" :class="{chosen:episode===index}" :aria-label="'播放'+row.name" @click="chooseEpisode(index)">{{row.name}}</button></view></scroll-view>
  <view v-if="!episodes.length" class="episode-empty">当前线路没有可播放集数。</view>
  <view v-if="expanded" class="episode-mask" @click="expanded=false"><view class="episode-sheet" role="dialog" aria-label="全部选集" @click.stop><view class="sheet-head"><text>{{film.title}}</text><button aria-label="关闭全部选集" @click="expanded=false"><AppIcon name="close" :size="18" :color="colors.text"/></button></view><scroll-view scroll-y class="sheet-scroll"><view class="line-row sheet-lines"><button v-for="(line,index) in film.sources" :key="index" :class="{chosen:source===index}" :aria-label="'切换播放线路'+line.name" @click="emit('source',index)">{{line.name}}</button></view><view class="episode-grid"><button v-for="(row,index) in episodes" :key="index" :class="{chosen:episode===index}" :aria-label="'播放'+row.name" @click="chooseEpisode(index)">{{row.name}}</button></view><text v-if="!episodes.length" class="episode-empty">当前线路没有可播放集数。</text></scroll-view></view></view>
 </view>
</template>
<style scoped>
.episode-heading{display:flex;align-items:center;gap:8px;font-size:14px;padding:15px 0 8px}.episode-note{flex:1;font-size:10px;color:var(--muted);white-space:nowrap;overflow:hidden;text-overflow:ellipsis}.all-episodes,.sheet-head button{padding:0;margin:0;background:transparent;color:var(--muted);font-size:10px;line-height:24px;display:flex;align-items:center;gap:2px;border:0;flex-shrink:0}.all-episodes::after,.sheet-head button::after,.line-row>button::after,.episode-row>button::after,.episode-grid>button::after{border:0}.line-scroll,.episode-scroll{width:100%;white-space:nowrap}.line-row,.episode-row{display:flex;align-items:stretch;gap:6px;padding:7px 0;min-width:max-content}.line-row>button,.episode-row>button,.episode-grid>button{margin:0;padding:0 11px;color:var(--muted);background:var(--surface);border:0;border-radius:3px;font-size:11px;line-height:34px;flex-shrink:0;white-space:nowrap}.line-row>button{background:transparent;padding:0 8px;line-height:27px}.episode-row>button{min-width:42px;line-height:34px;box-sizing:border-box}.line-row>.chosen,.episode-row>.chosen,.episode-grid>.chosen{color:var(--accent);background:var(--surface)}.episode-empty{padding:12px 0;font-size:11px;color:var(--muted)}.episode-mask{position:absolute;inset:0;z-index:75;background:#0009;display:flex;align-items:flex-end}.episode-sheet{width:100%;max-height:65%;display:flex;flex-direction:column;background:var(--bg);border-radius:12px 12px 0 0;padding:16px 12px;box-sizing:border-box}.sheet-head{display:flex;align-items:center;justify-content:space-between;gap:12px;font-size:14px;padding-bottom:12px}.sheet-head>text{overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.sheet-scroll{max-height:320px}.sheet-lines{min-width:0;flex-wrap:wrap}.episode-grid{display:grid;grid-template-columns:repeat(4,minmax(0,1fr));gap:8px;margin:10px 0}.episode-grid>button{width:100%;overflow:hidden;text-overflow:ellipsis;padding:0 4px}
</style>
