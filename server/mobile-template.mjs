// Consolidate the two original bundled phone themes once; uploaded templates remain independent.
export function createMobileTemplate(state, now = new Date().toISOString()) {
  return {mobileNavigationRevision:1,id:'hetun-kuying',name:'河豚酷影主题',platform:'mobile',renderer:'hetun-v2',draft:structuredClone(state.draft),published:structuredClone(state.published),version:state.version,publishedVersion:state.publishedVersion,installedAt:now};
}
function addIndependentNavigation(draft){
  for(const page of ['schedule','ranking'])if(!draft.navigation.some(n=>n.page===page)){
    let id=page,suffix=1;while(draft.navigation.some(n=>n.id===id))id=page+'-'+suffix++;
    const mine=draft.navigation.findIndex(n=>n.page==='mine');draft.navigation.splice(mine<0?draft.navigation.length:mine,0,{id,label:page==='schedule'?'排期表':'排行榜',page,categoryId:'',enabled:true,icon:page==='schedule'?'▤':'↗',activeIcon:page==='schedule'?'▤':'↗'});
  }
}
export function mergeBundledMobileTemplates(state) {
  const existing=state.templates?.find(t=>t.id==='hetun-kuying'&&t.name==='河豚酷影主题'&&t.platform==='mobile');
  // Existing installed templates belong to the operator; never insert navigation on restart.
  if(existing)return null;
  const legacy=state.templates?.find(t=>t.id==='legacy-mobile'&&t.name==='原有手机草稿'&&t.platform==='mobile'&&t.renderer==='hetun-v2');
  const modern=state.templates?.find(t=>t.id==='hetun-mobile'&&t.name==='河豚·光影手机'&&t.platform==='mobile'&&t.renderer==='hetun-v2');
  if(!legacy||!modern||state.templates.some(t=>t.id==='hetun-kuying'))return null;
  const next=structuredClone(state),old=next.templates.find(t=>t.id===legacy.id),fresh=next.templates.find(t=>t.id===modern.id);
  const draft=structuredClone(old.draft);
  // New visual treatment; existing brand, bindings, content, switches and business settings win.
  for(const key of ['colors','layout','pageStyles','carousel'])draft[key]=structuredClone(fresh.draft[key]);
  const visualKeys=['style','kind','radius','gap','ratio','columns'];
  for(const section of draft.sections){const incoming=fresh.draft.sections.find(s=>s.id===section.id);if(incoming)for(const key of visualKeys)if(incoming[key]!==undefined)section[key]=incoming[key];}
  for(const section of fresh.draft.sections)if(!draft.sections.some(s=>s.id===section.id))draft.sections.push(structuredClone(section));
  addIndependentNavigation(draft);
  const merged={...old,mobileNavigationRevision:1,id:'hetun-kuying',name:'河豚酷影主题',draft,version:Math.max(old.version,fresh.version)+1};
  next.templates=next.templates.flatMap(t=>t.id===old.id?[merged]:t.id===fresh.id?[]:[t]);
  for(const [platform,id] of Object.entries(next.activeTemplates||{}))if(id===old.id||id===fresh.id)next.activeTemplates[platform]=merged.id;
  if(next.selectedTemplate===old.id||next.selectedTemplate===fresh.id)next.selectedTemplate=merged.id;
  // Keep the active published theme unchanged until the operator publishes the merged draft.
  if(state.activeTemplates?.mobile===fresh.id&&fresh.published){merged.published=structuredClone(fresh.published);merged.publishedVersion=fresh.publishedVersion;}
  next.updatedAt=new Date().toISOString();
  return next;
}
