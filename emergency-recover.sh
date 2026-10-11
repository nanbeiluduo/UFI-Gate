#!/system/bin/sh
# Run through UFI-TOOLS Root Shell or ADB/su.
if [ -x /data/ufi-gate/ufi-gate ]; then
  sh /data/ufi-gate/service.sh recover
else
  echo '服务二进制不可用。仅删除 UFIG_ 跳转，保留其他插件规则。'
  for T in filter nat mangle; do
    iptables -t "$T" -S | awk '$1=="-A" && $2 !~ /^UFIG_/ && $0 ~ /-j UFIG_/ {sub(/^-A/,"-D"); print}' |
    while IFS= read -r R; do iptables -t "$T" $R; done
  done
  ip6tables -S | awk '$1=="-A" && $2 !~ /^UFIG_/ && $0 ~ /-j UFIG_/ {sub(/^-A/,"-D"); print}' |
  while IFS= read -r R; do ip6tables $R; done
  echo '已移除准入跳转。请按安装前记录恢复 tether_offload_disabled 设置。'
fi
