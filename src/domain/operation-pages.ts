import type {AppPage, Category, CategoryShortcut, CategoryShortcutLayout, ContentPage, NavItem, Section, Theme, MenuTargetType} from './types';

export const pageKinds = [
 {id:'category',label:'分类内容页'}, {id:'campaign',label:'活动页'},
 {id:'blank',label:'空白页'}, {id:'custom',label:'自定义内容页'}
];
// Legacy menu entries keep their existing system or content-page destination.
export function menuTargetType(entry:NavItem):MenuTargetType {return entry.targetType||(entry.pageId?'page':'system');}
export function setMenuTargetType(theme:Theme,entry:NavItem,type:MenuTargetType){
 if(menuTargetType(entry)===type)return;
 delete entry.url;delete entry.pageId;entry.categoryId='';entry.page='home';entry.targetType=type;
 if(type==='external')entry.url='';
 if(type==='page'){const page=theme.contentPages?.[0];if(page)bindPage(theme,entry,page.id);}
}
export function menuExternalUrl(value?:string){
 if(!value||value.length>2048||/[\s\\]/.test(value))return '';
 try{const url=new URL(value);return /^https?:\/\//i.test(value)&&['http:','https:'].includes(url.protocol)&&url.hostname&&!url.username&&!url.password?url.href:'';}catch{return '';}
}
export function pageRoute(page:ContentPage):AppPage {return page.kind==='category'?'category':'home';}
export function findContentPage(theme:Theme,id?:string){return theme.contentPages?.find(p=>p.id===id);}
export function entryVisible(theme:Theme,entry:NavItem){return entry.enabled&&(!entry.pageId||findContentPage(theme,entry.pageId)?.enabled===true);}
export function entryActive(entry:NavItem,page:AppPage,pageId=''){return pageId?entry.pageId===pageId:!entry.pageId&&entry.page===page;}
export function pageSections(theme:Theme,page:AppPage,pageId=''):Section[]{
 if(pageId&&findContentPage(theme,pageId)?.kind==='blank')return [];
 return theme.sections.filter(s=>s.enabled&&(pageId?s.pageId===pageId:!s.pageId&&(s.page||'home')===page));
}
export function bindPage(theme:Theme,entry:NavItem,id:string){
 if(!id){delete entry.pageId;return;}
 const page=findContentPage(theme,id);if(!page)return;
 if(theme.serviceMenuItems?.includes(entry)){entry.targetType='page';delete entry.url;}
 entry.pageId=id;entry.page=pageRoute(page);
 // Primary category sources belong to the entry, independently of its destination page.
 if(!theme.categories.includes(entry))entry.categoryId=page.categoryId;
}
export function bindPageCategory(theme:Theme,page:ContentPage,categoryId:string){
 const old=page.categoryId;page.categoryId=categoryId;
 for(const entry of [...theme.navigation,...(theme.serviceMenuItems||[])])if(entry.pageId===page.id)entry.categoryId=categoryId;
 for(const section of theme.sections)if(section.pageId===page.id&&section.categoryId===old){section.categoryId=categoryId;section.source=categoryId||page.kind==='category'?'category':'latest';}
}
// Deleting a navigation entry or destination must never strand an invalid autosave.
export function reconcileCarouselReferences(theme:Theme,removedPageId?:string){
 const ids=new Set(theme.categories.map(e=>e.id));
 for(const group of theme.carouselGroups||[]){
  group.entryIds=group.entryIds.filter(id=>ids.has(id));
  for(const slide of group.slides)if(removedPageId&&slide.target.type==='page'&&slide.target.pageId===removedPageId){slide.target.pageId='';slide.enabled=false;}
  if(!group.entryIds.length)group.enabled=false;
 }
}

export function removeContentPage(theme:Theme,id:string){
 migrateCategoryShortcutOwners(theme);
 theme.navigation=theme.navigation.filter(n=>n.pageId!==id);
 theme.categories=theme.categories.filter(n=>n.pageId!==id);
 if(theme.categoryShortcuts)theme.categoryShortcuts=theme.categoryShortcuts.filter(g=>theme.categories.some(n=>n.id===g.entryId));
 if(theme.serviceMenuItems)theme.serviceMenuItems=theme.serviceMenuItems.filter(n=>n.pageId!==id);
 theme.sections=theme.sections.filter(s=>s.pageId!==id);
 theme.carousel.slides=theme.carousel.slides.filter(s=>s.pageId!==id);
 theme.contentPages=theme.contentPages?.filter(p=>p.id!==id);
 reconcileCarouselReferences(theme,id);
}

// Missing means legacy defaults; an explicit empty array deliberately leaves the drawer empty.
export function serviceMenuEntries(theme:Theme):NavItem[]{
 if(theme.serviceMenuItems!==undefined)return theme.serviceMenuItems;
 return [
  ['member','会员权益','member'],['library','我的收藏','favorite'],
  ['schedule','影片排期','schedule'],['settings','应用设置','settings'],
  ['disclaimer','免责声明','shield'],['privacy','隐私政策','shield'],['about','关于我们','info']
 ].map(([page,label,icon])=>({id:'service-'+page,page:page as AppPage,label,icon,activeIcon:'',categoryId:'',enabled:page==='member'?theme.features.memberCard:page==='schedule'?theme.features.schedule:true}));
}

// Resolve legacy ownership once before category bindings can change.
export function migrateCategoryShortcutOwners(theme:Theme){
 if(!theme.categoryShortcuts)return;
 const explicit=new Set(theme.categoryShortcuts.filter(g=>g.entryId).map(g=>g.entryId));
 theme.categoryShortcuts=theme.categoryShortcuts.flatMap(g=>{
  if(g.entryId)return [g];
  const owners=theme.categories.filter(n=>!!g.categoryId&&n.categoryId===g.categoryId);
  if(owners.length!==1||explicit.has(owners[0].id))return [];
  const {categoryId,...settings}=g;return [{...settings,entryId:owners[0].id}];
 });
}

// APP entry identity owns settings; the bound CMS category only scopes resources.
// Legacy CMS-keyed groups apply only when exactly one APP entry owns that source.
export function categoryShortcutGroup(theme:Theme,entryId:string){
 const entry=theme.categories.find(n=>n.id===entryId);if(!entry)return undefined;
 const own=theme.categoryShortcuts?.find(g=>g.entryId===entryId);if(own)return own;
 if(!entry.categoryId||theme.categories.filter(n=>n.categoryId===entry.categoryId).length!==1)return undefined;
 return theme.categoryShortcuts?.find(g=>!g.entryId&&g.categoryId===entry.categoryId);
}
export function categoryShortcuts(theme:Theme,entryId:string):CategoryShortcut[]{
 const configured=categoryShortcutGroup(theme,entryId);
 if(configured)return configured.items;
 return [{id:'all',label:'全部',enabled:false,display:'both',iconKey:'category'},
 {id:'ranking',label:'排行榜',enabled:false,display:'both',iconKey:'ranking'},
 {id:'schedule',label:'排期表',enabled:false,display:'both',iconKey:'schedule'},
 {id:'search',label:'找影片',enabled:false,display:'both',iconKey:'search'}];
}
// Missing group switches preserve legacy entry visibility. New groups start disabled.
export function categoryShortcutSettings(theme:Theme,entryId:string):{enabled:boolean;layout:CategoryShortcutLayout;columns:number}{
 const group=categoryShortcutGroup(theme,entryId),columns=group?.columns;
 return {enabled:group?.enabled??!!group?.items.some(n=>n.enabled),layout:group?.layout==='grid'?'grid':'scroll',columns:typeof columns==='number'&&Number.isInteger(columns)&&columns>=2&&columns<=6?columns:4};
}
export function visibleCategoryShortcuts(theme:Theme,entryId:string):CategoryShortcut[]{
 return categoryShortcutSettings(theme,entryId).enabled?categoryShortcuts(theme,entryId).filter(n=>n.enabled):[];
}
export function categoryContains(categories:Category[],root:string,id:string):boolean {
 if(!root)return true;
 const seen=new Set<string>();let current=id;
 while(current&&!seen.has(current)){if(current===root)return true;seen.add(current);current=categories.find(c=>c.id===current)?.parentId||'';}
 return false;
}
export function navigationDisplay(theme:Theme,entry:{display?:string},fallback:'text'|'icon'|'both'='both'){
 return entry.display||fallback;
}
export function navigationArtwork(theme:Theme,entry:{id:string;iconKey?:string;icon?:string;activeIcon?:string},active=false){
 const slots=[...theme.navigation,...theme.categories,...(theme.serviceMenuItems||[])];
 const slot=entry.iconKey?slots.find(n=>n.id===entry.iconKey):undefined;
 const source=slot||entry;
 return active?(source.activeIcon||source.icon||''):(source.icon||'');
}
export function navigationIcon(page:string){return ({library:'favorite',disclaimer:'shield',privacy:'shield',about:'info'} as Record<string,string>)[page]||(['home','category','search','ranking','schedule','shortdrama','mine','member','history','settings'].includes(page)?page:'category');}
