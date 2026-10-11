import type {Vod} from '@/domain/types';
export function getLibrary():Vod[]{ try { const value=uni.getStorageSync('hetun-favorites'); return Array.isArray(value)?value:[]; }catch{return [];} }
export function toggleFavorite(vod:Vod):boolean {const list=getLibrary();const index=list.findIndex(v=>v.id===vod.id); if(index>=0) list.splice(index,1);else list.unshift(vod);uni.setStorageSync('hetun-favorites',list);return index<0;}
export function getHistory():Vod[]{const value=uni.getStorageSync('hetun-history');return Array.isArray(value)?value:[];}
export function remember(vod:Vod){uni.setStorageSync('hetun-history',[vod,...getHistory().filter(v=>v.id!==vod.id)].slice(0,60));}
