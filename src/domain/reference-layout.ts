import type {Theme} from './types';
export function applyReferenceLayout(t:Theme){
 t.pageStyles={...t.pageStyles,design:'youku',header:'search',member:'violet',mine:'card',navigation:'text'};
 t.colors={background:'#000000',surface:'#151515',text:'#f4f4f4',muted:'#888888',accent:'#27c9e3'};
 const labels:Record<string,string>={home:'首页',shortdrama:'短剧',member:'会员',category:'淘好片',mine:'我的'};t.navigation.forEach(n=>{if(labels[n.page])n.label=labels[n.page];});if(!t.navigation.some(n=>n.page==='category')&&t.navigation.length<20){const item={id:'reference-discover',label:'淘好片',page:'category' as const,categoryId:'',enabled:true,icon:'◇',activeIcon:'◆'};let suffix=1;while(t.navigation.some(n=>n.id===item.id))item.id='reference-discover-'+suffix++;const index=t.navigation.findIndex(n=>n.page==='mine');t.navigation.splice(index<0?t.navigation.length:index,0,item);}const home=t.categories.find(n=>n.page==='home');if(home)home.label='首页';
 t.layout.posterRadius=3;t.carousel={...t.carousel,style:'full',height:210,radius:3,margin:8};
 t.sections.filter(s=>(s.page||'home')==='home').forEach(s=>{s.radius=3;s.gap=5;s.ratio='3/4';s.showScore=false;});
 const first=t.sections.find(s=>(s.page||'home')==='home');if(first)first.title='今日给您的专属推荐';
}
