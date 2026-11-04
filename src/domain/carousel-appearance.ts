import type {Carousel,CarouselAppearance} from './types';

export const carouselStyles=[{id:'full',label:'通栏'},{id:'inset',label:'内嵌卡片'},{id:'peek',label:'两侧露出'},{id:'poster',label:'居中海报'},{id:'split',label:'图文分栏'}];
export const carouselNumbers=[
 {key:'widthPercent',label:'轮播宽度',min:40,max:100,unit:'%'},
 {key:'height',label:'轮播高度',min:120,max:600,unit:'px'},
 {key:'margin',label:'外边距',min:0,max:48,unit:'px'},
 {key:'radius',label:'圆角',min:0,max:300,unit:'px'},
] as const;
export type CarouselNumber=typeof carouselNumbers[number]['key'];
export type CarouselGeometry=Pick<Carousel,'style'|'height'|'radius'|'margin'|'fit'>&{widthPercent:number};

// Missing or invalid cached overrides inherit the theme, without mutating either source.
export function resolveCarouselAppearance(base:Carousel,appearance?:CarouselAppearance):CarouselGeometry {
 const result:CarouselGeometry={style:base.style,height:base.height,radius:base.radius,margin:base.margin,fit:base.fit,widthPercent:100};
 for(const field of carouselNumbers){const value=appearance?.[field.key];if(typeof value==='number'&&Number.isInteger(value)&&value>=field.min&&value<=field.max)result[field.key]=value;}
 if(carouselStyles.some(s=>s.id===appearance?.style))result.style=appearance!.style!;
 if(appearance?.fit==='cover'||appearance?.fit==='contain')result.fit=appearance.fit;
 return result;
}
export function carouselFrameStyle(geometry:CarouselGeometry){
 return {width:'calc('+geometry.widthPercent+'% - '+geometry.margin*2+'px)','--height':geometry.height+'px','--radius':geometry.radius+'px','--margin':geometry.margin+'px','--fit':geometry.fit};
}
