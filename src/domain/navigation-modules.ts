import type {AppPage,Category,NavItem,Theme} from './types';
import defaults from './default-theme.json';
import {categoryContains,visibleCategoryShortcuts} from './operation-pages';
export const bottomRoutes:AppPage[]=['home','shortdrama','member','schedule','ranking','mine'];
export function bottomFeature(page:AppPage):keyof Theme['features']|undefined {return ['shortdrama','member','schedule','ranking'].includes(page)?page as keyof Theme['features']:undefined;}
// Fixed module identity never depends on the editable display name.
export function normalizeBottomNavigation(theme:Theme){
 const current=theme.navigation;
 theme.navigation=bottomRoutes.map(page=>{
  const original=current.find(n=>n.page===page&&!n.pageId)||current.find(n=>n.id===page);
  const fallback=defaults.navigation.find(n=>n.page===page)!;
  const entry={...fallback,...original,page} as NavItem;
  delete entry.pageId;
  if(page!=='shortdrama')entry.categoryId='';
  const feature=bottomFeature(page);
  if(feature){entry.enabled=entry.enabled&&theme.features[feature]!==false;theme.features[feature]=entry.enabled;}
  return entry;
 });
 // Keep the operator's order among known modules; new modules use the default order.
 theme.navigation.sort((a,b)=>{const ai=current.findIndex(n=>n.id===a.id),bi=current.findIndex(n=>n.id===b.id);return (ai<0?current.length+bottomRoutes.indexOf(a.page):ai)-(bi<0?current.length+bottomRoutes.indexOf(b.page):bi);});
}
export function setBottomEnabled(theme:Theme,entry:NavItem,enabled:boolean){entry.enabled=enabled;const key=bottomFeature(entry.page);if(key)theme.features[key]=enabled;}
export function scopedRows<T extends {categoryId?:string}>(rows:T[],categories:Category[],root:string):T[]{return root?rows.filter(row=>!!row.categoryId&&categoryContains(categories,root,row.categoryId)):rows;}
export function categoryRouteAllowed(theme:Theme,entryId:string,page:AppPage){return !!theme.categories.find(n=>n.id===entryId&&n.enabled)?.categoryId&&visibleCategoryShortcuts(theme,entryId).some(n=>(n.id==='all'?'category':n.id)===page);}
export function shortDramaSource(theme:Theme,categories:Category[]){return theme.navigation.find(n=>n.page==='shortdrama')?.categoryId||categories.find(c=>/^短剧(?:专区|频道|剧场)?$/.test(c.name.trim()))?.id||'';}
