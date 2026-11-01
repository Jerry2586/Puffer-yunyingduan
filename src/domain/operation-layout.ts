import type {LayoutAppearance,Section,Theme} from './types';
export type LayoutKind='section'|'menu'|'middle';
export interface LayoutNumber {key:string;label:string;min:number;max:number;default:number;unit?:string}
export const layoutNumbers:Record<LayoutKind,LayoutNumber[]>={
 section:[{key:'widthPercent',label:'栏目宽度',min:40,max:100,default:100,unit:'%'},{key:'paddingX',label:'左右留白',min:0,max:40,default:14},{key:'paddingY',label:'上下留白',min:0,max:60,default:18},{key:'cardWidth',label:'横滑／列表封面宽度',min:72,max:320,default:120},{key:'headingSize',label:'栏目标题字号',min:12,max:32,default:18},{key:'textSize',label:'影片标题字号',min:10,max:24,default:12}],
 menu:[{key:'widthPercent',label:'菜单面板宽度',min:40,max:95,default:80,unit:'%'},{key:'rowHeight',label:'菜单行高',min:36,max:100,default:54},{key:'gap',label:'行间距',min:0,max:32,default:10},{key:'padding',label:'面板留白',min:8,max:32,default:18},{key:'iconSize',label:'图标大小',min:12,max:48,default:23},{key:'fontSize',label:'文字字号',min:10,max:28,default:14},{key:'radius',label:'菜单行圆角',min:0,max:32,default:10}],
 middle:[{key:'iconSize',label:'图标大小',min:16,max:64,default:36},{key:'fontSize',label:'文字字号',min:10,max:28,default:12},{key:'gap',label:'入口间距',min:0,max:32,default:8},{key:'contentGap',label:'图文间距',min:0,max:24,default:8},{key:'padding',label:'区域留白',min:0,max:32,default:12},{key:'itemHeight',label:'入口最小高度',min:32,max:120,default:64},{key:'itemWidth',label:'横滑入口宽度',min:48,max:200,default:72},{key:'radius',label:'入口圆角',min:0,max:40,default:8}]
};
export const layoutColors:Record<LayoutKind,{key:string;label:string}[]>={
 section:[{key:'headingColor',label:'栏目标题颜色'},{key:'textColor',label:'影片标题颜色'},{key:'background',label:'栏目背景'}],
 menu:[{key:'textColor',label:'菜单文字与图标'},{key:'background',label:'面板背景'},{key:'rowBackground',label:'菜单行背景'}],
 middle:[{key:'color',label:'文字与图标'},{key:'activeColor',label:'选中颜色'},{key:'background',label:'区域背景'},{key:'activeBackground',label:'选中背景'}]
};
export function layoutValue(kind:LayoutKind,appearance:LayoutAppearance|undefined,key:string):number {
 const rule=layoutNumbers[kind].find(n=>n.key===key)!;const value=appearance?.[key];
 return typeof value==='number'&&Number.isInteger(value)&&value>=rule.min&&value<=rule.max?value:rule.default;
}
export function layoutColor(appearance:LayoutAppearance|undefined,key:string,fallback:string){const value=appearance?.[key];return typeof value==='string'&&/^#[0-9a-fA-F]{6}$/.test(value)?value:fallback;}
export function sectionLayout(theme:Theme,section:Section){
 const a=section.appearance,n=(k:string)=>layoutValue('section',a,k);
 return {width:n('widthPercent')+'%',boxSizing:'border-box' as const,marginLeft:'auto',marginRight:'auto',padding:`${n('paddingY')}px ${n('paddingX')}px`,background:layoutColor(a,'background','transparent'),...(a?.cardWidth!==undefined?{'--card-width':n('cardWidth')+'px'}:{}),'--heading-size':n('headingSize')+'px','--text-size':n('textSize')+'px','--heading-color':layoutColor(a,'headingColor',theme.colors.text),'--title-color':layoutColor(a,'textColor',theme.colors.text)};
}
export function menuLayout(theme:Theme){
 const a=theme.serviceMenuStyle,n=(k:string)=>layoutValue('menu',a,k);
 return {'--menu-width':n('widthPercent')+'%','--menu-row-height':n('rowHeight')+'px','--menu-gap':n('gap')+'px','--menu-padding':n('padding')+'px','--menu-font':n('fontSize')+'px','--menu-radius':n('radius')+'px','--menu-color':layoutColor(a,'textColor',theme.colors.text),'--menu-bg':layoutColor(a,'background',theme.colors.background),'--menu-row-bg':layoutColor(a,'rowBackground',theme.colors.surface)};
}
