#!/bin/sh
set -e

STATE_DIR=/var/lib/zabbix
OUTPUT=$STATE_DIR/smartmeter.json

# Zabbix agentが読み取れる場所に出力ディレクトリを用意する。
# zabbix-agent / zabbix-agent2 が未導入でもサービス単体で動くようにしておく。
if [ ! -d "$STATE_DIR" ]; then
    mkdir -p "$STATE_DIR"
    chmod 0755 "$STATE_DIR"
fi

# SELinuxが有効な環境（AlmaLinux / RHEL系の既定）では正しいラベルを付け直す。
# ラベルが var_lib_t のままだと zabbix_agent_t から読めずItemが Not supported になる。
if command -v restorecon >/dev/null 2>&1; then
    restorecon -R "$STATE_DIR" >/dev/null 2>&1 || :
fi

if command -v systemctl >/dev/null 2>&1; then
    systemctl daemon-reload >/dev/null 2>&1 || :
fi

cat <<'MSG'

zabbix-smartmeter をインストールしました。

  1. Bルート認証情報を設定する
       sudo vi /etc/sysconfig/zabbix-smartmeter
  2. 疎通を確認する
       sudo /usr/bin/zabbix-smartmeter --device /dev/ttyUSB0 --dse --scan
  3. 定期取得を開始する
       sudo systemctl enable --now zabbix-smartmeter.timer
  4. Zabbixにテンプレートを取り込む
       /usr/share/zabbix-smartmeter/zabbix_smartmeter_template.yaml

詳しい手順は /usr/share/doc/zabbix-smartmeter/almalinux.md を参照してください。

MSG
