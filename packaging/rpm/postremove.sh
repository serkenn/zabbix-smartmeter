#!/bin/sh
set -e

if command -v systemctl >/dev/null 2>&1; then
    systemctl daemon-reload >/dev/null 2>&1 || :
fi

# アップグレード時（rpmなら$1=1、debならupgrade）は残骸を消さない。
case "$1" in
    1|upgrade) exit 0 ;;
esac

rm -f /var/lib/zabbix/smartmeter.json
