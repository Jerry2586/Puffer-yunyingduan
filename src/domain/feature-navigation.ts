import type {Theme,NavItem} from './types';
export function setFeatureEnabled(theme:Theme,key:keyof Theme['features'],enabled:boolean){
 theme.features[key]=enabled;
 if(!enabled||!['schedule','ranking'].includes(key))return;
 if(theme.navigation.some(n=>n.page===key))return;
 const used=new Set(theme.navigation.map(n=>n.id));let id:string=key;let suffix=1;while(used.has(id))id=key+'-'+suffix++;
 const item:NavItem={id,label:key==='schedule'?'排期表':'排行榜',page:key as 'schedule'|'ranking',categoryId:'',enabled:true,icon:key==='schedule'?'▤':'↗',activeIcon:key==='schedule'?'▤':'↗'};
 const mine=theme.navigation.findIndex(n=>n.page==='mine');theme.navigation.splice(mine<0?theme.navigation.length:mine,0,item);
}

export function bindNavigationCategory(nav:NavItem,categoryId:string){
 nav.categoryId=categoryId;
 if(categoryId&&!['shortdrama','ranking'].includes(nav.page))nav.page='category';
}
