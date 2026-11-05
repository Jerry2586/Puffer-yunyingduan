<?php
namespace app\index\controller;
// Base owns playback permission checks in older MacCMS v10 releases.
class Hetun extends Base {
 public function token() {
  if (empty($GLOBALS['user']['user_id'])) return json(['code'=>1401,'msg'=>'请先登录']);
  $token = session('csrf_token');
  if (empty($token)) { $token=bin2hex(random_bytes(16)); session('csrf_token',$token); }
  return json(['code'=>1,'info'=>['token'=>$token]]);
 }
 // Older v10 releases do not have api.php/user/get_info or ajax_upgrade_data.
 // Revalidate CMS cookies through its model; never trust a caller-supplied user id.
 private function memberData() {
  $result=model('User')->checkLogin();
  if (intval($result['code'] ?? 0)!==1 || empty($result['info']['user_id'])) return null;
  $info=$result['info'];
  $out=[];
  foreach (['user_id','user_name','user_nick_name','group_id','user_points','user_portrait','user_end_time'] as $key) $out[$key]=$info[$key] ?? '';
  // checkLogin may have downgraded an expired membership in its effective group.
  $out['group_id']=intval($info['group']['group_id'] ?? $info['group_id'] ?? 2);
  $out['member_name']=strip_tags($info['group']['group_name'] ?? '');
  $end=intval($info['user_end_time'] ?? 0);
  $out['expire_date']=$end>0 ? date('Y-m-d H:i:s',$end) : '';
  return $out;
 }
 public function member_info() {
  $info=$this->memberData();
  if ($info===null) return json(['code'=>1401,'msg'=>'请先登录']);
  return json(['code'=>1,'info'=>$info]);
 }
 public function member_plans() {
  $info=$this->memberData();
  $groups=[];
  foreach (model('Group')->getCache() as $group) {
   if (intval($group['group_id'] ?? 0)<3 || intval($group['group_status'] ?? 0)!==1) continue;
   $row=['group_id'=>intval($group['group_id']),'group_name'=>strip_tags($group['group_name'] ?? '')];
   foreach (['day','week','month','year'] as $period) {
    $row['group_points_'.$period]=max(0,intval($group['group_points_'.$period] ?? 0));
    // Legacy CMS offers points upgrades; it has no direct membership-order API.
    $row['price_'.$period]=0;
   }
   $groups[]=$row;
   if (count($groups)>=50) break;
  }
  return json(['code'=>1,'data'=>['is_login'=>$info!==null,'member_name'=>$info['member_name'] ?? '', 'expire_date'=>$info['expire_date'] ?? '', 'user_points'=>intval($info['user_points'] ?? 0),'groups'=>$groups]]);
 }
 public function play() {
  $param=['id'=>intval(input('id')), 'sid'=>max(1,intval(input('sid',1))), 'nid'=>max(1,intval(input('nid',1)))];
  // River client supplies canonical vod_id from the catalogue, independent of rewrite mode.
  $res=model('Vod')->infoData(['vod_id'=>$param['id'],'vod_status'=>1],'*',1);
  if (!isset($res['code']) || intval($res['code'])!==1) return json(['code'=>1004,'msg'=>'影片不存在']);
  $info=$res['info'];
  if (intval($info['vod_status'])!==1) return json(['code'=>1004,'msg'=>'影片已下架']);
  if (!empty($info['vod_pwd_play'])) return json(['code'=>1403,'msg'=>'该影片需要站点密码验证']);
  $permission=$this->check_user_popedom($info['type_id'],3,$param,'play',$info,0);
  if (!isset($permission['code']) || intval($permission['code'])!==1) return json(['code'=>1403,'msg'=>strip_tags($permission['msg'] ?? '请购买相应会员或积分权限')]);
  $row=$info['vod_play_list'][$param['sid']]['urls'][$param['nid']] ?? null;
  $url=$row['url'] ?? '';
  if (!preg_match('#^https?://#i',$url)) return json(['code'=>1404,'msg'=>'当前线路不支持直接播放，请配置直连视频线路']);
  return json(['code'=>1,'info'=>['url'=>$url,'name'=>$row['name'] ?? '']]);
 }
}
