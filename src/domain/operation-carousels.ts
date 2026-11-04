import type {CarouselTarget, OperationsCarouselGroup, OperationsCarouselSlide, Theme, Vod} from './types';
import {menuExternalUrl} from './operation-pages';

export function carouselDefaultName(theme:Theme,group:OperationsCarouselGroup):string{
 const label=theme.categories.find(e=>e.id===group.entryIds[0])?.label.trim();
 if(!label)return '新轮播组';
 let prefix='',bytes=0,count=0;
 for(const char of label){const size=new TextEncoder().encode(char).length;if(count===77||bytes+size>231)break;prefix+=char;bytes+=size;count++;}
 return prefix+'轮播组';
}
export function syncCarouselGroupName(theme:Theme,group:OperationsCarouselGroup){
 const automatic=carouselDefaultName(theme,group);
 // Old default names migrate automatically; existing custom names stay intact.
 group.nameMode??=group.name.trim()==='新轮播组'||group.name.trim()===automatic?'auto':'custom';
 if(group.nameMode==='auto')group.name=automatic;
}

export const carouselTargetOptions=[{id:'vod',label:'影片详情'},{id:'page',label:'内置内容页'},{id:'external',label:'第三方网页'},{id:'system',label:'APP 功能'}];
export function carouselImageValid(value:string){return /^\/api\/assets\/[a-f0-9]{64}\.(png|jpg|webp)$/.test(value)||!!menuExternalUrl(value);}
export function carouselSlideIssue(theme:Theme,slide:OperationsCarouselSlide):string{
 if(!carouselImageValid(slide.image))return '请先选择影片图片、上传图片或填写有效的 HTTP(S) 图片地址。';
 const t=slide.target;
 if(t.type==='vod'&&!/^\d{1,20}$/.test(t.vodId))return '请选择点击后打开的影片。';
 if(t.type==='page'&&!theme.contentPages?.some(p=>p.id===t.pageId))return '请选择已创建的内置内容页。';
 if(t.type==='external'&&!menuExternalUrl(t.url))return '请填写完整的 HTTP(S) 网页链接。';
 return '';
}
export function carouselGroupIssue(theme:Theme,group:OperationsCarouselGroup):string{
 if(group.entryIds.length!==1)return '请绑定一个 APP 一级分类，每组只能绑定一个分类。';
 const entryId=group.entryIds[0];
 if(!theme.categories.some(e=>e.id===entryId))return '绑定的一级分类已不存在，请重新选择。';
 if(theme.carouselGroups?.some(g=>g.id!==group.id&&g.entryIds.includes(entryId)))return '该一级分类已绑定其他轮播组，请先解除原绑定。';
 const active=group.slides.filter(s=>s.enabled);
 return active.map(s=>carouselSlideIssue(theme,s)).find(Boolean)||'';
}
export function resetCarouselTarget(slide:OperationsCarouselSlide,type:CarouselTarget['type']){
 slide.enabled=false;
 slide.target=type==='vod'?{type,vodId:''}:type==='page'?{type,pageId:''}:type==='external'?{type,url:''}:{type,page:'home'};
}

// Autosave replaces the normalized theme, so async editors must resolve stable IDs.
export function resolveCarouselRecord(theme:Theme,groupId:string,slideId:string){
 const group=theme.carouselGroups?.find(g=>g.id===groupId);
 const slide=group?.slides.find(s=>s.id===slideId);
 return group&&slide?{group,slide}:undefined;
}
export function carouselFilmImage(vod:Pick<Vod,'carouselImage'|'poster'>){
 return [vod.carouselImage,vod.poster].find((image):image is string=>!!image&&carouselImageValid(image))||'';
}
