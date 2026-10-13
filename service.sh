#!/system/bin/sh
# UFI-Gate supervisor. Only touches files/chains owned by this plugin.
PATH=/system/bin:/system/xbin:/vendor/bin:/data/adb/magisk:$PATH
export PATH
DIR=/data/ufi-gate
BIN=$DIR/ufi-gate
BOOT=/sdcard/ufi_tools_boot.sh
MARK=' # ufi-gate-boot-v1'
alive() {
  [ -f "$1" ] || return 1
  PID=$(awk '{print $1; exit}' "$1")
  case "$PID" in ''|*[!0-9]*) return 1;; esac
  kill -0 "$PID" 2>/dev/null || return 1
  [ -r "/proc/$PID/cmdline" ] || return 1
  tr '\000' ' ' < "/proc/$PID/cmdline" | grep -q '/data/ufi-gate' || return 1
}
hook() {
  [ -f "$BOOT" ] || printf '#!/system/bin/sh\n' > "$BOOT"
  awk '!/ # ufi-gate-boot-v1$/' "$BOOT" > "$DIR/boot.tmp" || return 1
  printf '\nsh /data/ufi-gate/service.sh boot & # ufi-gate-boot-v1\n' >> "$DIR/boot.tmp"
  cp "$DIR/boot.tmp" "$BOOT" && rm -f "$DIR/boot.tmp"
}
unhook() {
  [ -f "$BOOT" ] || return 0
  awk '!/ # ufi-gate-boot-v1$/' "$BOOT" > "$DIR/boot.tmp" || return 1
  cp "$DIR/boot.tmp" "$BOOT" && rm -f "$DIR/boot.tmp"
}
stop_worker() {
  if alive "$DIR/worker.pid"; then
    kill "$PID" 2>/dev/null
    I=0
    while kill -0 "$PID" 2>/dev/null && [ "$I" -lt 8 ]; do sleep 1; I=$((I+1)); done
    kill -9 "$PID" 2>/dev/null
  fi
  rm -f "$DIR/worker.pid"
}
case "$1" in
 start|start-update)
  if [ "$1" = start ] && [ -f "$DIR/update-journal.json" ]; then
    "$DIR/update-runner" --dir "$DIR" --resume-update >> "$DIR/update.log" 2>&1
    exit $?
  fi
  rm -f "$DIR/stopped" "$DIR/suspended"
  if alive "$DIR/supervisor.pid"; then echo 'ok'; exit 0; fi
  rm -rf "$DIR/supervisor.lock"
  nohup sh "$0" supervise > "$DIR/supervisor.log" 2>&1 </dev/null &
  echo $! > "$DIR/supervisor.pid"
  hook || { echo '开机脚本写入失败'; exit 1; }
  echo 'ok'
  ;;
 boot)
  if [ -f "$DIR/update-journal.json" ]; then
    "$DIR/update-runner" --dir "$DIR" --resume-update >> "$DIR/update.log" 2>&1
    exit $?
  fi
  [ -f "$DIR/stopped" ] && exit 0
  [ -f "$DIR/suspended" ] && exit 0
  # Boot hook may run before WiFi/data services; delay the worker but don't block the host boot script.
  sleep 20
  sh "$0" start
  ;;
 supervise)
  mkdir "$DIR/supervisor.lock" 2>/dev/null || exit 0
  echo $$ > "$DIR/supervisor.pid"
  trap 'stop_worker; rmdir "$DIR/supervisor.lock" 2>/dev/null; exit 0' INT TERM
  FAIL=0
  while [ ! -f "$DIR/stopped" ] && [ ! -f "$DIR/suspended" ]; do
    "$BIN" --dir "$DIR" --cleanup-rules >> "$DIR/supervisor.log" 2>&1
    "$BIN" --dir "$DIR" >> "$DIR/service.log" 2>&1 &
    WORKER=$!
    echo "$WORKER" > "$DIR/worker.pid"
    wait "$WORKER"
    CODE=$?
    rm -f "$DIR/worker.pid"
    "$BIN" --dir "$DIR" --cleanup-rules >> "$DIR/supervisor.log" 2>&1
    [ -f "$DIR/stopped" ] && break
    [ -f "$DIR/suspended" ] && break
    FAIL=$((FAIL+1))
    if [ "$FAIL" -ge 5 ]; then
      "$BIN" --dir "$DIR" --recover >> "$DIR/supervisor.log" 2>&1
      printf '服务连续退出 5 次，已恢复网络；请检查 service.log\n' >> "$DIR/supervisor.log"
      break
    fi
    sleep 30
  done
  rmdir "$DIR/supervisor.lock" 2>/dev/null
  ;;
 stop|recover|stop-update)
  touch "$DIR/stopped"
  if alive "$DIR/supervisor.pid"; then kill "$PID" 2>/dev/null; fi
  stop_worker
  # Do not recover/load state until the old worker has actually released its lock.
  I=0
  while alive "$DIR/supervisor.pid" && [ "$I" -lt 10 ]; do sleep 1; I=$((I+1)); done
  if alive "$DIR/supervisor.pid"; then echo 'supervisor did not stop'; exit 1; fi
  if ! "$BIN" --dir "$DIR" --recover; then
    [ "$1" = stop-update ] && "$DIR/update-backup/ufi-gate" --dir "$DIR" --cleanup-rules || exit 1
  fi
  if [ "$1" = stop-update ]; then hook || exit 1; else unhook || exit 1; fi
  rm -f "$DIR/supervisor.pid"
  rm -rf "$DIR/supervisor.lock"
  echo 'ok'
  ;;
 status)
  if alive "$DIR/worker.pid"; then printf 'running %s\n' "$PID"; else echo 'stopped'; fi
  ;;
 uninstall)
  sh "$0" stop || exit 1
  # Keep credentials/statistics as a backup unless the administrator explicitly purges them.
  rm -f "$BIN" "$DIR/service.sh" "$DIR/bootstrap.json" "$DIR/setup.token"
  echo 'ok'
  ;;
 *) echo 'usage: service.sh start|stop|status|boot|recover|uninstall'; exit 1;;
esac
