<?php
// Exercise the adapter against the older v10 hierarchy: All has label_user,
// while Base owns check_user_popedom. No CMS database or real users are used.
namespace app\common\controller {
 class All {
  public function __construct() {}
  protected function label_user() { $GLOBALS['label_calls']++; }
 }
}
namespace app\index\controller {
 class Base extends \app\common\controller\All {
  public function __construct() { parent::__construct(); $this->label_user(); }
  protected function check_user_popedom($type, $access, $param, $flag, $info, $trysee) {
   $GLOBALS['permission_calls'][] = [$type,$access,$param,$flag,$trysee];
   return $GLOBALS['permission'];
  }
 }
}
namespace {
 $label_calls=0; $permission_calls=[]; $params=[]; $sessions=[]; $user=[];
 $permission=['code'=>1];
 $vod=['code'=>1,'info'=>['vod_status'=>1,'vod_pwd_play'=>'','type_id'=>20,'vod_play_list'=>[1=>['urls'=>[1=>['url'=>'https://example.test/video.m3u8','name'=>'Episode 1']]]]]];
 function input($key,$default=null) { return $GLOBALS['params'][$key] ?? $default; }
 function json($value) { return $value; }
 function session($key,$value=null) { if (func_num_args()>1) $GLOBALS['sessions'][$key]=$value; return $GLOBALS['sessions'][$key] ?? null; }
 function model($name) {
  if ($name!=='Vod') throw new \RuntimeException('unexpected model');
  return new class {
   public function infoData($where,$fields,$details) {
    if ($where!==['vod_id'=>42,'vod_status'=>1] || $fields!=='*' || $details!==1) throw new \RuntimeException('catalog identity or publication filter lost');
    return $GLOBALS['vod'];
   }
  };
 }
 function expect($condition,$message) { if (!$condition) throw new \RuntimeException($message); }
 require __DIR__.'/../../go-server/cms/Hetun.php';
 $adapter=new \app\index\controller\Hetun();
 expect($label_calls===1,'Base must initialize the user exactly once');
 $params=['id'=>42,'sid'=>1,'nid'=>1];
 $result=$adapter->play();
 expect($result['code']===1 && $result['info']['url']==='https://example.test/video.m3u8','allowed playback failed');
 expect($permission_calls[0]===[20,3,['id'=>42,'sid'=>1,'nid'=>1],'play',0],'CMS playback authorization arguments changed');
 $permission=['code'=>3003,'msg'=>'<b>requires points</b>'];
 $result=$adapter->play();
 expect($result['code']===1403 && !isset($result['info']) && $result['msg']==='requires points','denied playback exposed a URL');
 $permission=['code'=>3002,'trysee'=>120];
 expect($adapter->play()['code']===1403,'preview-only permission exposed full URL');
 $permission=['code'=>1];
 $vod['info']['vod_pwd_play']='protected';
 expect($adapter->play()['code']===1403,'password-protected video was exposed');
 $vod['info']['vod_pwd_play']='';
 $vod['info']['vod_status']=0;
 expect($adapter->play()['code']===1004,'unpublished video was exposed');
 $vod['info']['vod_status']=1;
 $vod['info']['vod_play_list'][1]['urls'][1]['url']='javascript:alert(1)';
 expect($adapter->play()['code']===1404,'non-http URL was exposed');
 expect($adapter->token()['code']===1401,'guest received a member token');
 $user=['user_id'=>7];
 $result=$adapter->token();
 expect($result['code']===1 && preg_match('/^[a-f0-9]{32}$/',$result['info']['token']),'member token invalid');
 expect($adapter->token()['info']['token']===$result['info']['token'],'existing session token changed');
 echo "playback adapter regression: 11 checks passed\n";
}
