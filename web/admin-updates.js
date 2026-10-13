'use strict';
window.createUpdateUI = ({api, esc, stamp, toast, navigate}) => {
  let messages={items:[],kinds:{},push:{}}, update, filter='all';
  const $=id=>document.getElementById(id);
  const pushStates={off:'仅站内',pending:'等待推送',sent:'已推送',failed:'推送失败',expired:'申请已结束'};
  const phases={downloading:'下载中',verified:'校验通过',preparing:'备份中',stopping:'停服中',installing:'安装中',starting:'健康检查中',rolling_back:'恢复旧版本',success:'更新成功',failed:'更新失败',rolled_back:'已恢复旧版本',rollback_failed:'需要手动恢复'};
  const newer=(a,b)=>{if(!/^\d+\.\d+\.\d+$/.test(a||''))return false;const x=a.split('.').map(Number),y=b.split('.').map(Number);for(let i=0;i<3;i++)if(x[i]!==y[i])return x[i]>y[i];return false};
  async function perform(fn){try{await fn()}catch(e){toast(e.message)}}
  function mount(){
    $('root').insertAdjacentHTML('beforeend','<button id="message-float" class="message-float" aria-label="打开消息盒子">消息 <span id="message-count">0</span></button><dialog id="message-box" class="message-box"><div class="row between"><h2>消息盒子</h2><button id="message-close">关闭</button></div><div class="row"><select id="message-filter" aria-label="通知筛选"><option value="all">全部消息</option><option value="unread">未读消息</option><option value="updates">版本与更新</option><option value="alerts">设备与异常</option></select><button id="message-read-all">全部已读</button></div><div id="message-list"></div></dialog>');
    $('message-float').onclick=()=>perform(async()=>{await refresh();renderMessages();$('message-box').showModal()});
    $('message-close').onclick=()=>$('message-box').close();
    $('message-filter').onchange=e=>{filter=e.target.value;renderMessages()};
    $('message-read-all').onclick=()=>perform(async()=>{await api('notifications/read',{All:true});await refresh()});
  }
  function renderMessages(){
    if(!$('message-list'))return;
    const list=[...(messages.items||[])].reverse().filter(n=>filter==='all'||filter==='unread'&&!n.read||filter==='updates'&&n.kind.startsWith('update_')||filter==='alerts'&&!n.kind.startsWith('update_'));
    $('message-list').innerHTML=list.length?list.map(n=>'<article class="message-item '+(n.read?'':'unread')+'"><div class="row between"><strong>'+esc(messages.kinds[n.kind]||n.kind)+'</strong><span class="tiny">'+esc(stamp(n.time))+'</span></div><p>'+esc(n.message)+'</p><div class="row between"><span class="tiny">'+esc(pushStates[n.pushState]||n.pushState)+'</span><button data-message="'+esc(n.id)+'">'+(n.read?'查看':'查看并标记已读')+'</button></div></article>').join(''):'<p class="empty">暂无消息</p>';
    $('message-list').querySelectorAll('[data-message]').forEach(b=>b.onclick=()=>perform(async()=>{const n=messages.items.find(n=>n.id===b.dataset.message);await api('notifications/read',{ID:n.id});$('message-box').close();navigate(n.target);await refresh()}));
  }
  async function refresh(){
    const result=await api('notifications');if(!$('message-count'))return;messages=result;
    $('message-count').textContent=messages.unread>99?'99+':messages.unread;
    $('message-float').setAttribute('aria-label','消息盒子，'+messages.unread+' 条未读');
    if($('message-box').open)renderMessages();
    if($('update-panel')){update=await api('update');renderUpdateStatus()}
  }
  function renderUpdateStatus(){
    if(!$('update-status'))return;
    const r=update.release||{},s=update.status||{},active=['downloading','verified','preparing','stopping','installing','starting','rolling_back'].includes(s.phase);
    $('update-current').textContent=update.current+' · '+update.arch;
    $('update-latest').textContent=r.version||'尚未检查';
    $('update-checked').textContent=stamp(r.checked);
    $('update-status').textContent=(update.checking?'正在检查 GitHub…\n':'')+(s.phase?(phases[s.phase]||s.phase)+'：'+s.message+'\n':'')+(r.error?'检查提示：'+r.error:(!newer(r.version,update.current)&&r.version?'当前服务已是此发布渠道的最新版本。':''));
    $('update-notes').innerHTML=window.DOMPurify?DOMPurify.sanitize(marked.parse(r.notes||'检查更新后显示版本说明。')):marked.parse(r.notes||'检查更新后显示版本说明。');
    $('update-check').disabled=update.checking||active;
    $('update-install').disabled=active||update.checking||!update.signatureReady||!!r.error||!r.manifest||!newer(r.version,update.current);
    $('update-install').textContent=active?'更新进行中…':'安装已验证的新版本';
    const link=$('update-release');link.hidden=!r.url;if(r.url)link.href=r.url;
  }
  async function renderUpdates(){
    update=await api('update');
    $('view').innerHTML='<article class="card" id="update-panel"><h2>更新管理</h2><div class="update-facts"><p>当前服务 <strong id="update-current"></strong></p><p>最新正式版本 <strong id="update-latest"></strong></p><p>上次检查 <span id="update-checked"></span></p></div><div class="row"><button id="update-check" class="primary">检查更新</button><button id="update-install">安装已验证的新版本</button><a id="update-release" target="_blank" rel="noopener noreferrer" hidden>版本说明与下载</a></div><p class="note">更新会短暂停服并关闭认证，保留管理员、设备、配额、流量和自定义图片。安装后请重新登录，确认状态后手动开启认证。关闭浏览器不会取消已提交的更新。</p><pre id="update-status" role="status" aria-live="polite"></pre><div class="checks"><label><input id="update-auto" type="checkbox" '+(update.autoCheck?'checked':'')+'>每天自动检查并通知（安装仍需手动确认）</label></div><button id="update-save">保存检查设置</button><h3>版本说明</h3><div id="update-notes" class="update-notes"></div></article>';
    renderUpdateStatus();
    $('update-check').onclick=()=>perform(async()=>{ $('update-check').disabled=true;await api('update/check',{});await refresh()});
    $('update-save').onclick=()=>perform(async()=>{await api('update/settings',{AutoCheck:$('update-auto').checked});toast('检查设置已保存')});
    $('update-install').onclick=()=>perform(async()=>{const version=update.release.version;if(!confirm('确认从 '+update.current+' 更新到 '+version+'？\n将短暂停服、保留数据并关闭认证。失败会尝试恢复旧服务。'))return;$('update-install').disabled=true;await api('update/install',{Version:version});toast('更新任务已提交，服务重启后请重新登录');await refresh()});
  }
  async function renderPreferences(){
    const m=await api('notifications');if(!$('view')||!document.querySelector('[data-action="push-test"]'))return;
    $('view').insertAdjacentHTML('beforeend','<article class="card" id="notification-settings"><h2>通知推送范围</h2><p class="note">所有通知都会进入消息盒子。勾选的类型同时发送到上方配置的推送服务；推送失败只在站内记录。新设置对后续消息生效，关闭某类推送也会取消该类待发送消息。</p><div class="checks">'+Object.entries(m.kinds).filter(([k])=>k!=='push_failed').map(([k,v])=>'<label><input type="checkbox" data-push-kind="'+k+'" '+(m.push[k]?'checked':'')+'>'+esc(v)+'</label>').join('')+'</div><button id="notification-save" class="primary">保存通知推送范围</button></article>');
    $('notification-save').onclick=()=>perform(async()=>{const Push={};$('notification-settings').querySelectorAll('[data-push-kind]').forEach(e=>Push[e.dataset.pushKind]=e.checked);await api('notifications/settings',{Push});toast('通知推送范围已保存')});
  }
  return {mount,refresh,renderUpdates,renderPreferences};
};
