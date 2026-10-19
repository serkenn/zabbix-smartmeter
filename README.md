# zabbix-smartmeter

USB 接続の Wi-SUN モジュールを使って、スマートメーター（B ルートサービス）から電力情報を取得する CLI ツールです。
**Zabbix**（JSON 出力）と **Mackerel**（プラグイン形式）の両方に対応しています。

[![CI](https://github.com/serkenn/zabbix-smartmeter/actions/workflows/ci.yml/badge.svg)](https://github.com/serkenn/zabbix-smartmeter/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/serkenn/zabbix-smartmeter)](https://github.com/serkenn/zabbix-smartmeter/releases)
[![License](https://img.shields.io/github/license/serkenn/zabbix-smartmeter)](LICENSE)

## 取得できるメトリック

| JSON キー | 内容 | 単位 |
| --- | --- | --- |
| `value` | 瞬時電力計測値（逆潮流時は負） | W |
| `r` / `t` | 瞬時電流計測値（R 相 / T 相） | A |
| `total_normal` | 積算電力量（正方向・買電） | kWh |
| `total_reverse` | 積算電力量（逆方向・売電） | kWh |

積算電力量は、メーターが返す係数（EPC 0xD3）と積算電力量単位（EPC 0xE1）を使って kWh に換算しています。

```console
$ zabbix-smartmeter --json --device /dev/ttyUSB0 --dse --id "$ID" --password "$PW"
{"r":7.5,"t":3,"total_normal":12345.6,"total_reverse":0,"value":1050}
```

## 動作確認済みハードウェア

- **[UDG-1-WSNE](https://web116.jp/shop/netki/miruene_usb/miruene_usb_00.html)**（NTT 東日本 / 西日本）
  - 本モジュールを使う場合は `--dse` オプションが必須です。

## インストール

### AlmaLinux / RHEL 系（RPM）

```bash
gh release download --repo serkenn/zabbix-smartmeter --pattern '*.x86_64.rpm'
sudo dnf install ./zabbix-smartmeter-*.x86_64.rpm
sudo vi /etc/sysconfig/zabbix-smartmeter   # B ルート ID とパスワードを設定
sudo systemctl enable --now zabbix-smartmeter.timer
```

> RPM / deb は v0.0.6 より後のリリースから提供されます。v0.0.6 以前は zip のみです。

systemd タイマー、SELinux、Zabbix テンプレートの取り込みまで含めた手順は
**[docs/almalinux.md](docs/almalinux.md)** にまとめています。

### Debian / Ubuntu / Raspberry Pi OS

```bash
gh release download --repo serkenn/zabbix-smartmeter --pattern '*_arm64.deb'
sudo apt install ./zabbix-smartmeter_*_arm64.deb
```

### バイナリを直接置く

[Releases](https://github.com/serkenn/zabbix-smartmeter/releases) から
`zabbix-smartmeter_linux-<arch>.zip`（`386` / `amd64` / `armhf` / `arm64`）を取得します。

```bash
unzip zabbix-smartmeter_linux-amd64.zip
sudo install -m 0755 zabbix-smartmeter /usr/local/bin/
```

シリアルポートを非 root で読む場合は、実行ユーザーを `dialout` グループに入れてください。

```bash
sudo usermod -aG dialout zabbix
```

## Zabbix との連携

Zabbix agent から直接スマートメーターを叩くのではなく、**systemd タイマーで JSON を書き出し、
agent はそのファイルを読むだけ**という構成を推奨しています。

```
systemd timer ──> zabbix-smartmeter --json --output /var/lib/zabbix/smartmeter.json
                                                          │
                     Zabbix agent: vfs.file.contents[...] ┘
```

- B ルートの応答が遅くても Zabbix agent の `Timeout` に影響しません
- SELinux で `zabbix_agent_t` からシリアルポートを開く必要がありません
- B ルートのパスワードを Zabbix 側に持たせずに済みます

テンプレートは [`zabbix/`](zabbix/) にあります。

| ファイル | 用途 |
| --- | --- |
| `zabbix/zabbix_smartmeter_template.yaml` | Zabbix 6.0 以降向け（推奨） |
| `zabbix/zabbix_smartmeter_template.xml` | 同じ内容の XML 版 |
| `zabbix/userparameter_smartmeter.conf` | UserParameter で取りたい場合のサンプル |

テンプレートには瞬時電力・電流・積算電力量のアイテムに加えて、
データの更新が止まったことを検知するトリガーが含まれています。

> テンプレートの 2 ファイルは `zabbix/generate_template.py` から生成しています。
> 変更する場合はスクリプトを編集して `python3 zabbix/generate_template.py` を実行してください。

## Mackerel との連携

従来どおり [mackerel-agent](https://mackerel.io/ja/docs/entry/custom-metrics) のカスタムメトリックとして使えます。

```ini
[plugin.metrics.smartmeter]
command = ["/usr/bin/zabbix-smartmeter", "--device", "/dev/ttyUSB0", "--dse"]
```

## オプション

すべてのオプションは環境変数でも指定できます（コマンドラインの指定が優先）。

| オプション | 環境変数 | 既定値 | 内容 |
| --- | --- | --- | --- |
| `--device` | `SMARTMETER_DEVICE` | | シリアルポートのパス |
| `--id` | `SMARTMETER_ID` | | B ルート ID |
| `--password` | `SMARTMETER_PASSWORD` | | B ルートパスワード |
| `--dse` | `SMARTMETER_DSE` | `false` | Dual Stack Edition の SK コマンドを使う |
| `--channel` | `SMARTMETER_CHANNEL` | | チャンネル（指定するとスキャンを省略できる） |
| `--ipaddr` | `SMARTMETER_IPADDR` | | メーターの IPv6 アドレス |
| `--json` | `SMARTMETER_JSON` | `false` | Zabbix 向けに JSON で出力する |
| `--output` | `SMARTMETER_OUTPUT` | | 標準出力ではなくファイルへ原子的に書き出す |
| `--timeout` | `SMARTMETER_TIMEOUT` | ライブラリ既定 | ECHONET Lite クエリ 1 回のタイムアウト |
| `--log` | `SMARTMETER_LOG` | `auto` | ログの出力先（`auto` / `syslog` / `stderr`） |
| `--verbosity` | `SMARTMETER_VERBOSITY` | `1` | 0: quiet 〜 3: debug |
| `--scan` | | `false` | チャンネルとメーターを探索する |
| `--metric-key-prefix` | | `smartmeter` | Mackerel のメトリックキー接頭辞 |
| `--version` | | | バージョンを表示して終了 |

`--log auto` は、systemd から起動された場合（`JOURNAL_STREAM` が設定されている場合）は標準エラー出力、
それ以外は syslog を使います。Mackerel / JSON モードでは標準出力にログが混ざらないようにするための挙動です。

## 開発

```bash
go test ./...                      # テスト
go vet ./...
golangci-lint run ./...            # Lint（.golangci.yml, golangci-lint v2）
goreleaser release --snapshot --clean --skip=publish   # zip / rpm / deb をローカルで作る
```

- CI（[`.github/workflows/ci.yml`](.github/workflows/ci.yml)）でテスト・Lint・`govulncheck`・
  クロスコンパイル・パッケージングを実行します。
- 依存ライブラリと GitHub Actions は [Dependabot](.github/dependabot.yml) が毎週更新します。
- `v*` タグを push すると [GoReleaser](.goreleaser.yaml) が zip / rpm / deb を添えてリリースします。

## ライセンス

MIT License. [go-smartmeter](https://github.com/hnw/go-smartmeter) および
mackerel-plugin-smartmeter をベースにしています。
