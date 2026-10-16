/* Shared mandatory key-download step for setup, migration and password reset. */
window.ufiRecovery = (root, pending) => new Promise(resolve => {
  root.innerHTML = '<main class="auth-shell"><section class="card auth-card"><div class="badge">UFI-GATE · ACCOUNT RECOVERY</div><h1>保存管理员恢复密钥</h1><p class="muted">请下载 TXT 并保存在密码管理器或安全位置。持有密钥的人可以重置管理员密码，请勿分享或上传。</p><p class="note">必须完成下载并确认保存，注册或密码重置才会生效。本步骤 10 分钟内有效；中途退出可重新开始，原凭据保持有效。</p><button type="button" class="primary" id="recovery-download">下载恢复密钥 TXT</button><div class="checks" style="margin-top:20px"><label><input type="checkbox" id="recovery-saved" disabled>我已确认 TXT 文件已保存，可在其他设备上找到</label></div><button type="button" id="recovery-finish" disabled>完成并进入后台</button><p class="error" id="recovery-error" role="alert"></p></section></main>';
  const download = root.querySelector('#recovery-download'), saved = root.querySelector('#recovery-saved'), finish = root.querySelector('#recovery-finish'), error = root.querySelector('#recovery-error');
  const request = (action, extra = {}) => fetch('/api/admin-access/' + action, {method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({Ticket:pending.ticket,...extra})});
  download.onclick = async () => {
    download.disabled = true; error.textContent = '';
    try {
      const response = await request('download');
      if (!response.ok) throw Error((await response.json()).error || '下载失败');
      const blob = await response.blob(), url = URL.createObjectURL(blob), link = document.createElement('a');
      link.href = url; link.download = 'UFI-Gate-recovery-key.txt'; document.body.appendChild(link); link.click(); link.remove();
      setTimeout(() => URL.revokeObjectURL(url), 60000);
      saved.disabled = false; download.textContent = '重新下载恢复密钥 TXT';
    } catch(e) {error.textContent = e.message;}
    finally {download.disabled = false;}
  };
  saved.onchange = () => {finish.disabled = !saved.checked;};
  finish.onclick = async () => {
    finish.disabled = true; download.disabled = true; saved.disabled = true; error.textContent = '';
    try {
      const response = await request('commit', {Saved:saved.checked}), result = await response.json();
      if (!response.ok) throw Error(result.error || '保存失败');
      resolve(result);
    } catch(e) {error.textContent=e.message;finish.disabled=!saved.checked;download.disabled=false;saved.disabled=false;}
  };
});
