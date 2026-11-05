<?php
namespace app\index\controller;
use app\common\controller\All;
class Hetun extends All {
 public function __construct() {
  parent::__construct();
  $this->label_user();
 }
 public function token() {
  if (empty($GLOBALS['user']['user_id'])) return json(['code'=>1401,'msg'=>'请先登录']);
  $token = session('csrf_token');
  if (empty($token)) { $token=bin2hex(random_bytes(16)); session('csrf_token',$token); }
  return json(['code'=>1,'info'=>['token'=>$token]]);
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
