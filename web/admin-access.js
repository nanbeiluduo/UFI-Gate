(() => {
  'use strict';
  const $ = id => document.getElementById(id), forgot = $('forgot-password');
  forgot.onchange = () => {
    $('password-fields').hidden = forgot.checked; $('recovery-fields').hidden = !forgot.checked;
    for (const id of ['admin-user','admin-password']) {$(id).disabled=forgot.checked;$(id).required=!forgot.checked;}
    for (const id of ['admin-recovery-key','admin-new-password','admin-confirm-password']) {$(id).disabled=!forgot.checked;$(id).required=forgot.checked;}
    $('admin-access-submit').textContent = forgot.checked ? '验证密钥并生成新密钥' : '登录并添加此管理设备';
    $('admin-access-error').textContent = '';
  };
  $('admin-access-form').onsubmit = async event => {
    event.preventDefault(); const button=$('admin-access-submit'); button.disabled=true;
    try {
      if(forgot.checked && $('admin-new-password').value!==$('admin-confirm-password').value) throw Error('两次新密码不一致');
      const payload=forgot.checked ? {RecoveryKey:$('admin-recovery-key').value,NewPassword:$('admin-new-password').value} : {Username:$('admin-user').value,Password:$('admin-password').value};
      const response=await fetch('/api/admin-access/'+(forgot.checked?'reset':'login'),{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(payload)});
      let result=await response.json();
      if(!response.ok) throw Error(result.error || '验证失败');
      if(result.ticket) result=await window.ufiRecovery($('root'),result);
      location.assign(result.adminURL);
    } catch(e) {if($('admin-access-error')) $('admin-access-error').textContent=e.message;}
    finally {button.disabled=false;}
  };
})();
