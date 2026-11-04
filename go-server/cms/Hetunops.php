<?php
// Install in application/index/controller/Hetunops.php (MacCMS V10 / ThinkPHP 5).
// application/extra/hetun_operations.php: <?php return ['key' => '64_HEX_CHARACTERS'];
namespace app\index\controller;

class Hetunops extends \app\common\controller\All
{
    private function opsResult($code, $data = [], $error = '') {
        return json(['code' => $code, 'data' => $data, 'error' => $error]);
    }
    public function gateway() {
        $raw = file_get_contents('php://input');
        if (request()->method() !== 'POST' || strlen($raw) > 32768) return $this->opsResult(0, [], 'invalid');
        $file = APP_PATH . 'extra/hetun_operations.php';
        $config = is_file($file) ? include $file : [];
        $key = isset($config['key']) ? $config['key'] : '';
        $stamp = isset($_SERVER['HTTP_X_HETUN_TIME']) ? $_SERVER['HTTP_X_HETUN_TIME'] : '';
        $nonce = isset($_SERVER['HTTP_X_HETUN_NONCE']) ? $_SERVER['HTTP_X_HETUN_NONCE'] : '';
        $sig = isset($_SERVER['HTTP_X_HETUN_SIGNATURE']) ? $_SERVER['HTTP_X_HETUN_SIGNATURE'] : '';
        if (!preg_match('/^[a-fA-F0-9]{64}$/D', $key) || !preg_match('/^[0-9]{10}$/D', $stamp) || abs(time() - intval($stamp)) > 90 || !preg_match('/^[a-f0-9]{64}$/D', $nonce) || !hash_equals(hash_hmac('sha256', $stamp . "\n" . $nonce . "\n" . $raw, $key), $sig)) return $this->opsResult(0, [], 'unauthorized');
        $dir = RUNTIME_PATH . 'hetun-ops-nonces/';
        if (!is_dir($dir) && !@mkdir($dir, 0700, true) && !is_dir($dir)) return $this->opsResult(0, [], 'storage');
        $handle = @fopen($dir . $nonce, 'x');
        if (!$handle) return $this->opsResult(0, [], 'unauthorized');
        fclose($handle);
        foreach (glob($dir . '*') as $old) { if (is_file($old) && filemtime($old) < time() - 190) @unlink($old); }
        $body = json_decode($raw, true);
        if (!is_array($body) || !isset($body['action']) || !isset($body['data']) || !is_array($body['data'])) return $this->opsResult(0, [], 'invalid');
        $d = $body['data']; $action = $body['action'];
        try {
            if ($action === 'meta') return $this->opsResult(1, $this->opsMeta());
            if ($action === 'playback.sources') return $this->opsResult(1, $this->opsSources());
            if (in_array($action, ['member.register', 'member.upgrade', 'member.avatar'], true)) return $this->opsMember($action, $d);
            $parts = explode('.', $action);
            if (count($parts) !== 2 || !in_array($parts[0], ['videos', 'users'], true)) return $this->opsResult(0, [], 'invalid');
            if ($parts[1] === 'list') return $this->opsResult(1, $this->opsListing($parts[0], $d));
            return $this->opsMutate($parts[0], $parts[1], $d);
        } catch (\InvalidArgumentException $e) { return $this->opsResult(0, [], 'invalid'); }
        catch (\Exception $e) { return $this->opsResult(0, [], 'database'); }
        catch (\Throwable $e) { return $this->opsResult(0, [], 'database'); }
    }
    private function opsInteger($v, $min, $max) {
        if (!is_scalar($v) || !preg_match('/^-?[0-9]+$/D', (string)$v)) throw new \InvalidArgumentException();
        $n = intval($v); if ($n < $min || $n > $max) throw new \InvalidArgumentException(); return $n;
    }
    private function opsText($v, $max, $required = false) {
        if (!is_string($v) || mb_strlen($v, 'UTF-8') > $max || ($required && trim($v) === '')) throw new \InvalidArgumentException();
        return trim($v);
    }
    private function opsSources() {
        $items = []; $players = config('vodplayer');
        if (!is_array($players)) $players = [];
        uasort($players, function($a, $b) { return intval(isset($a['sort'])?$a['sort']:0) - intval(isset($b['sort'])?$b['sort']:0); });
        foreach ($players as $key => $p) {
            $code = isset($p['from']) ? (string)$p['from'] : (string)$key;
            if (!preg_match('/^[a-zA-Z0-9_-]{1,64}$/D', $code)) continue;
            $items[] = ['code'=>$code, 'name'=>isset($p['show'])?(string)$p['show']:$code, 'enabled'=>isset($p['status']) && intval($p['status'])===1];
            if (count($items) >= 100) break;
        }
        return ['items'=>$items];
    }
    private function opsMember($action, $d) {
        $model = model('User');
        \think\Db::startTrans();
        try {
            if ($action === 'member.register') {
                $username = $this->opsText(isset($d['username'])?$d['username']:'', 50, true);
                $password = isset($d['password'])?$d['password']:'';
                $this->opsText($password, 128, true);
                if (!isset($d['active']) || !is_bool($d['active'])) throw new \InvalidArgumentException();
                $points = $this->opsInteger(isset($d['points'])?$d['points']:0,0,2147483647);
                $reward = $this->opsInteger(isset($d['invitePoints'])?$d['invitePoints']:0,0,2147483647);
                $inviter = $this->opsInteger(isset($d['inviter'])?$d['inviter']:0,0,2147483647);
                $ip = $this->opsText(isset($d['ip'])?$d['ip']:'',45,true);
                if (!filter_var($ip, FILTER_VALIDATE_IP)) throw new \InvalidArgumentException();
                $parent = null;
                if ($inviter > 0) {
                    $parent = $model->where(['user_id'=>$inviter,'user_status'=>1])->lock(true)->find();
                    if (!$parent || in_array('1', explode(',',(string)$parent['group_id']),true)) throw new \InvalidArgumentException();
                }
                $fields = ['user_name'=>$username,'user_pwd'=>$password,'group_id'=>'2','user_status'=>$d['active']?1:0,'user_points'=>$points,'user_reg_time'=>time(),'user_reg_ip'=>filter_var($ip, FILTER_VALIDATE_IP, FILTER_FLAG_IPV4) ? sprintf('%u', ip2long($ip)) : 0];
                if ($parent) $fields['user_pid'] = $inviter;
                // saveData owns validation and the installed MacCMS password hashing scheme.
                $saved = $model->saveData($fields);
                if (!isset($saved['code']) || intval($saved['code']) !== 1) throw new \InvalidArgumentException();
                $uid = intval($model->where(['user_name'=>$username])->value('user_id'));
                if ($uid < 1) throw new \RuntimeException();
                if ($parent && $d['active'] && $reward > 0) {
                    if (intval($parent['user_points']) > 2147483647 - $reward) throw new \InvalidArgumentException();
                    if ($model->where(['user_id'=>$inviter])->setInc('user_points',$reward) === false) throw new \RuntimeException();
                }
            } else {
                $uid = $this->opsInteger(isset($d['id'])?$d['id']:0,1,2147483647);
                $user = $model->where(['user_id'=>$uid,'user_status'=>1])->lock(true)->find();
                if (!$user || in_array('1',explode(',',(string)$user['group_id']),true)) throw new \InvalidArgumentException();
                if ($action === 'member.avatar') {
                    $avatar = $this->opsText(isset($d['avatar'])?$d['avatar']:'',2048,true);
                    if (!preg_match('#^(https?://|/api/assets/)#', $avatar) || preg_match('/[\r\n]/',$avatar)) throw new \InvalidArgumentException();
                    $update = ['user_portrait'=>$avatar];
                } else {
                    $gid = $this->opsInteger(isset($d['groupId'])?$d['groupId']:0,3,2147483647);
                    $cost = $this->opsInteger(isset($d['points'])?$d['points']:0,1,2147483647);
                    $days = ['day'=>1,'week'=>7,'month'=>30,'year'=>365]; $period = isset($d['period'])?$d['period']:'';
                    $group = model('Group')->where(['group_id'=>$gid,'group_status'=>1])->find();
                    if (!$group || !isset($days[$period]) || intval($user['user_points']) < $cost) throw new \InvalidArgumentException();
                    $until = max(time(),intval($user['user_end_time'])) + $days[$period] * 86400;
                    if ($until > 2147483647) throw new \InvalidArgumentException();
                    $update = ['group_id'=>(string)$gid,'user_points'=>intval($user['user_points'])-$cost,'user_end_time'=>$until];
                }
                if ($model->where(['user_id'=>$uid])->update($update) === false) throw new \RuntimeException();
            }
            \think\Db::commit(); return $this->opsResult(1,['ok'=>true,'affected'=>1,'userId'=>$uid]);
        } catch (\Exception $e) { \think\Db::rollback(); throw $e; }
        catch (\Throwable $e) { \think\Db::rollback(); throw $e; }
    }
    private function opsMeta() {
        $types = model('Type')->where(['type_mid'=>1])->field('type_id,type_name,type_pid')->order('type_sort asc,type_id asc')->select();
        $groups = model('Group')->field('group_id,group_name')->order('group_id asc')->select();
        $categories = []; $members = [];
        foreach ($types as $t) $categories[] = ['id' => intval($t['type_id']), 'name' => (string)$t['type_name'], 'parentId' => intval($t['type_pid'])];
        foreach ($groups as $g) $members[] = ['id' => intval($g['group_id']), 'name' => (string)$g['group_name']];
        return ['adapterVersion' => 1, 'categories' => $categories, 'groups' => $members];
    }
    private function opsListing($kind, $d) {
        $video = $kind === 'videos'; $prefix = $video ? 'vod_' : 'user_'; $idField = $prefix . 'id'; $where = [];
        $page = $this->opsInteger(isset($d['page']) ? $d['page'] : 1, 1, 1000000);
        $size = $this->opsInteger(isset($d['pageSize']) ? $d['pageSize'] : 20, 1, 100);
        $keyword = $this->opsText(isset($d['keyword']) ? $d['keyword'] : '', 200);
        if ($keyword !== '') $where[$video ? 'vod_name' : 'user_name|user_nick_name'] = ['like', '%' . addcslashes($keyword, '%_\\') . '%'];
        foreach (['status' => $prefix . 'status', 'id' => $idField, 'categoryId' => 'type_id'] as $key => $field) {
            if (!$video && $key === 'categoryId') continue;
            if (isset($d[$key]) && $d[$key] !== '') $where[$field] = $this->opsInteger($d[$key], $key === 'status' ? 0 : 1, $key === 'status' ? 1 : 2147483647);
        }
        if (!$video && isset($d['groupId']) && $d['groupId'] !== '') {
            $gid = (string)$this->opsInteger($d['groupId'],1,2147483647);
            $where['group_id'] = ['like', [$gid, $gid . ',%', '%,' . $gid . ',%', '%,' . $gid], 'OR'];
        }
        if ($video && isset($d['level']) && $d['level'] !== '') $where['vod_level'] = $this->opsInteger($d['level'], 0, 9);
        $sorts = $video ? ['id' => 'vod_id', 'level' => 'vod_level'] : ['id' => 'user_id', 'points' => 'user_points'];
        $sort = isset($d['sort']) && isset($sorts[$d['sort']]) ? $sorts[$d['sort']] : $idField;
        $order = isset($d['order']) && $d['order'] === 'asc' ? 'asc' : 'desc';
        $fields = $video ? 'vod_id,vod_name,vod_pic,type_id,vod_status,vod_level,vod_play_from,vod_year,vod_area' : 'user_id,user_name,user_nick_name,group_id,user_status,user_points,user_end_time,user_login_time,user_login_ip';
        $model = model($video ? 'Vod' : 'User');
        // Vod::listData preserves MacCMS recycle-bin filtering across supported versions.
        if ($video) {
            $result = $model->listData($where, $sort . ' ' . $order . ',' . $idField . ' desc', $page, $size, 0, $fields, 0, 1);
            if (!isset($result['code']) || intval($result['code']) !== 1) throw new \RuntimeException();
            $total = intval($result['total']);
            $lastPage = max(1, (int)ceil($total / $size));
            if ($page > $lastPage) {
                $page = $lastPage;
                $result = $model->listData($where, $sort . ' ' . $order . ',' . $idField . ' desc', $page, $size, 0, $fields, 0, 1);
                if (!isset($result['code']) || intval($result['code']) !== 1) throw new \RuntimeException();
                $total = intval($result['total']);
            }
            $rows = $result['list'];
        } else {
            $total = intval($model->where($where)->count());
            $page = min($page, max(1, (int)ceil($total / $size)));
            $rows = $model->where($where)->field($fields)->order($sort . ' ' . $order)->page($page, $size)->select();
        }
        $meta = $this->opsMeta(); $types = []; $groups = [];
        foreach ($meta['categories'] as $t) $types[$t['id']] = $t['name'];
        foreach ($meta['groups'] as $g) $groups[$g['id']] = $g['name'];
        $items = [];
        foreach ($rows as $r) {
            if ($video) $items[] = ['id'=>intval($r['vod_id']), 'name'=>(string)$r['vod_name'], 'cover'=>(string)$r['vod_pic'], 'categoryId'=>intval($r['type_id']), 'categoryName'=>isset($types[$r['type_id']])?$types[$r['type_id']]:'', 'status'=>intval($r['vod_status']), 'level'=>intval($r['vod_level']), 'sources'=>(string)$r['vod_play_from'], 'year'=>(string)$r['vod_year'], 'area'=>(string)$r['vod_area']];
            else {
                $groupNames = [];
                foreach (explode(',', (string)$r['group_id']) as $gid) { if (isset($groups[$gid])) $groupNames[] = $groups[$gid]; }
                $items[] = ['id'=>intval($r['user_id']), 'name'=>(string)$r['user_name'], 'nickname'=>(string)$r['user_nick_name'], 'groupId'=>(string)$r['group_id'], 'groupName'=>implode(' / ', $groupNames), 'status'=>intval($r['user_status']), 'points'=>intval($r['user_points']), 'expiresAt'=>intval($r['user_end_time']), 'lastLoginAt'=>intval($r['user_login_time']), 'lastLoginIP'=>is_numeric($r['user_login_ip']) ? (string)long2ip((int)$r['user_login_ip']) : (string)$r['user_login_ip']];
            }
        }
        return ['items'=>$items,'page'=>$page,'pageSize'=>$size,'total'=>intval($total),'pages'=>max(1,(int)ceil($total/$size))];
    }
    private function opsCategory($id) {
        $id = $this->opsInteger($id, 1, 2147483647); $row = model('Type')->where(['type_id'=>$id,'type_mid'=>1])->find();
        if (!$row) throw new \InvalidArgumentException();
        return ['type_id'=>$id,'type_id_1'=>intval($row['type_pid'])];
    }
    private function opsMutate($kind, $action, $d) {
        $video = $kind === 'videos'; $allowed = $video ? ['edit','status','category','level'] : ['edit','status','create','delete'];
        if (!in_array($action, $allowed, true)) throw new \InvalidArgumentException();
        $ids = []; if ($action !== 'create') {
            if (!isset($d['ids']) || !is_array($d['ids']) || count($d['ids']) < 1 || count($d['ids']) > 100) throw new \InvalidArgumentException();
            foreach ($d['ids'] as $id) $ids[] = $this->opsInteger($id, 1, 2147483647);
            $ids = array_values(array_unique($ids)); if ($action === 'edit' && count($ids) !== 1) throw new \InvalidArgumentException();
        }
        $model = model($video ? 'Vod' : 'User'); $idField = $video ? 'vod_id' : 'user_id'; $update = [];
        if ($action === 'status') $update[$video?'vod_status':'user_status'] = $this->opsInteger(isset($d['value'])?$d['value']:-1,0,1);
        if ($action === 'level') $update['vod_level'] = $this->opsInteger(isset($d['value'])?$d['value']:-1,0,9);
        if ($action === 'category') $update = $this->opsCategory(isset($d['value'])?$d['value']:0);
        if ($action === 'edit' || $action === 'create') {
            $f = isset($d['fields']) && is_array($d['fields']) ? $d['fields'] : [];
            if ($video) {
                $update = $this->opsCategory(isset($f['categoryId'])?$f['categoryId']:0);
                $update['vod_name'] = $this->opsText(isset($f['name'])?$f['name']:'',200,true);
                $update['vod_year'] = $this->opsText(isset($f['year'])?$f['year']:'',20);
                $update['vod_area'] = $this->opsText(isset($f['area'])?$f['area']:'',100);
                $update['vod_status'] = $this->opsInteger(isset($f['status'])?$f['status']:1,0,1);
                $update['vod_level'] = $this->opsInteger(isset($f['level'])?$f['level']:0,0,9);
            } else {
                $update['user_name'] = $this->opsText(isset($f['name'])?$f['name']:'',50,true);
                $update['user_nick_name'] = $this->opsText(isset($f['nickname'])?$f['nickname']:'',50);
                $rawGroups = isset($f['groupId']) ? $f['groupId'] : '2';
                if (!is_string($rawGroups) && !is_int($rawGroups)) throw new \InvalidArgumentException();
                $groupIds = [];
                foreach (explode(',', (string)$rawGroups) as $gid) $groupIds[] = $this->opsInteger($gid,2,2147483647);
                $groupIds = array_values(array_unique($groupIds));
                if (count($groupIds) > 20 || intval(model('Group')->where(['group_id'=>['in',$groupIds]])->count()) !== count($groupIds)) throw new \InvalidArgumentException();
                $update['group_id'] = implode(',', $groupIds);
                $update['user_status'] = $this->opsInteger(isset($f['status'])?$f['status']:1,0,1);
                $update['user_points'] = $this->opsInteger(isset($f['points'])?$f['points']:0,0,2147483647);
                $update['user_end_time'] = $this->opsInteger(isset($f['expiresAt'])?$f['expiresAt']:0,0,2147483647);
                if (isset($f['password']) && $f['password'] !== '') {
                    $this->opsText($f['password'],128,true);
                    $update['user_pwd'] = $f['password'];
                }
                if ($action === 'create' && !isset($update['user_pwd'])) throw new \InvalidArgumentException();
                if ($action === 'edit') $update['user_id'] = $ids[0];
            }
        }
        \think\Db::startTrans();
        try {
            if ($action !== 'create') {
                if ($video) {
                    $visible = $model->listData([$idField=>['in',$ids]],$idField . ' asc',1,100,0,$idField,0,1);
                    if (!isset($visible['code']) || intval($visible['code']) !== 1 || intval($visible['total']) !== count($ids)) throw new \InvalidArgumentException();
                } else {
                    $targets = $model->where([$idField=>['in',$ids]])->field('user_id,group_id')->lock(true)->select();
                    if (count($targets) !== count($ids)) throw new \InvalidArgumentException();
                    foreach ($targets as $target) {
                        if (in_array('1', explode(',', (string)$target['group_id']), true)) throw new \InvalidArgumentException();
                    }
                }
            }
            if (!$video && ($action === 'create' || $action === 'edit')) {
                // Use the CMS model's validation and version-specific password hashing.
                $res = $model->saveData($update);
                if (!isset($res['code']) || intval($res['code']) !== 1) throw new \InvalidArgumentException();
            } elseif ($action === 'delete') {
                $res = $model->delData([$idField=>['in',$ids]]);
                if (!isset($res['code']) || intval($res['code']) !== 1) throw new \RuntimeException();
            } else {
                $res = $model->where([$idField=>['in',$ids]])->update($update); if ($res === false) throw new \RuntimeException();
            }
            \think\Db::commit(); return $this->opsResult(1,['ok'=>true,'affected'=>$action==='create'?1:count($ids)]);
        } catch (\Exception $e) { \think\Db::rollback(); throw $e; }
        catch (\Throwable $e) { \think\Db::rollback(); throw $e; }
    }
}
