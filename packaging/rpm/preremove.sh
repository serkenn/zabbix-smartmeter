#!/bin/sh
set -e

# アップグレード時（rpmなら$1=1、debならupgrade）は停止しない。
case "$1" in
    1|upgrade) exit 0 ;;
esac

if command -v systemctl >/dev/null 2>&1; then
    systemctl disable --now zabbix-smartmeter.timer >/dev/null 2>&1 || :
    systemctl stop zabbix-smartmeter.service >/dev/null 2>&1 || :
fi
