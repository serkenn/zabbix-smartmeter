# zabbix-smartmeter

USB接続のWi-SUNモジュールを用いて、スマートメーターから電力情報を取得するCLIツールです。
従来の **Mackerel** プラグインとしての利用に加え、**Zabbix** エージェント（JSON出力）との連携にも対応しています。

![GitHub release (latest by date)](https://img.shields.io/github/v/release/serkenn/zabbix-smartmeter)
![Build Status](https://img.shields.io/github/actions/workflow/status/serkenn/zabbix-smartmeter/release.yml)

## 概要

Bルートサービス（スマートメーター）にアクセスし、以下のデータを取得します。

- **瞬時電力** [W]
- **瞬時電流**（R相・T相）[A]
- **積算電力量**（正方向：買電）[kWh] ※v0.0.6以降
- **積算電力量**（逆方向：売電）[kWh] ※v0.0.6以降

取得したデータは、Mackerelへの投稿用フォーマット、またはZabbix連携用のJSONフォーマットで出力可能です。

## 動作確認済みハードウェア

- **[UDG-1-WSNE](https://web116.jp/shop/netki/miruene_usb/miruene_usb_00.html)** (NTT東日本/西日本)
    - ※本モジュールを利用する場合は `--dse` オプションが必須です。

## インストール

[Releases](https://github.com/serkenn/zabbix-smartmeter/releases) ページから、環境に合わせたバイナリをダウンロードしてください。

**Raspberry Pi / Proxmox (LXC) への設置例:**

```bash
# ダウンロードと配置
wget [https://github.com/serkenn/zabbix-smartmeter/releases/download/v0.0.6/zabbix-smartmeter_linux-amd64.zip](https://github.com/serkenn/zabbix-smartmeter/releases/download/v0.0.6/zabbix-smartmeter_linux-amd64.zip)
unzip zabbix-smartmeter_linux-amd64.zip
sudo mv zabbix-smartmeter /usr/local/bin/
sudo chmod +x /usr/local/bin/zabbix-smartmeter

# シリアルポート権限の付与 (Zabbixユーザーの場合)
sudo usermod -aG dialout zabbix
