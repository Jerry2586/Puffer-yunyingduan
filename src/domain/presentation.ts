import type {Theme,Ad,AppPage,Section,Vod} from './types';
export const pageNames:Record<AppPage,string>={home:'首页',category:'分类',search:'搜索',shortdrama:'短剧',member:'会员',mine:'我的',ranking:'排行榜',schedule:'排期表',library:'收藏',history:'观看记录',settings:'设置',disclaimer:'免责声明',privacy:'隐私政策',about:'关于应用'};
export const carouselStyles=[['full','全幅大图'],['inset','内嵌圆角'],['peek','居中露边'],['poster','竖幅海报'],['split','图文分栏'],['video','视频轮播']] as const;
export const sectionStyles=[['grid2','双列海报'],['grid3','三列紧凑'],['scroll','横向滑动'],['list','横版列表'],['ranking','排名列表'],['mosaic','一大多小']] as const;
export function available(t:Theme,p:string){return !['member','shortdrama','schedule','ranking'].includes(p)||t.features[p as keyof Theme['features']]!==false;}
export function adsAt(t:Theme,slot:Ad['slot'],now=Date.now()){return t.ads.filter(a=>a.enabled&&a.asset&&a.slot===slot&&(!a.start||Date.parse(a.start)<=now)&&(!a.end||Date.parse(a.end)>now));}
export function moduleItems(s:Section,items:Vod[]){let a=s.source==='category'?items.filter(v=>v.categoryId===s.categoryId):[...items];if(s.orderBy==='manual'){const ids=(s.manualIds||'').split(',').map(x=>x.trim());a=ids.map(id=>items.find(v=>v.id===id)).filter((v):v is Vod=>!!v);}if(s.orderBy==='score')a.sort((x,y)=>Number(y.score)-Number(x.score));return a.slice(0,s.limit);}
