# AlmaLinux / RHEL 系でのセットアップ

AlmaLinux 9・10（および RHEL 9/10、Rocky Linux、CentOS Stream といった互換ディストリビューション）での
導入手順です。Raspberry Pi OS や Debian / Ubuntu でも `.deb` を使えば同じ構成で動きます。

## 構成の考え方

Zabbix agent から直接スマートメーターを叩かず、**収集と参照を分離**しています。

```
systemd timer (1分ごと)
   └─ zabbix-smartmeter --json --output /var/lib/zabbix/smartmeter.json
            │  Wi-SUN / ECHONET Lite（数秒〜数十秒かかる）
            ▼
      /var/lib/zabbix/smartmeter.json   ← rename(2) で原子的に差し替え
            ▲
            │  vfs.file.contents[...]（即座に返る）
   Zabbix agent / agent2
```

分離している理由は 3 つあります。

- **agent の Timeout に引っかからない。** B ルートの応答は認証からやり直すと数十秒かかることがあり、
  Zabbix agent の `Timeout`（最大 30 秒）を超えて Item が Not supported になります。
- **SELinux と相性が良い。** `zabbix_agent_t` からシリアルポートを開くのは既定のポリシーで拒否されます。
  収集を別サービスに出せば、agent は `zabbix_var_lib_t` のファイルを読むだけになります。
- **B ルート認証情報を Zabbix に持たせずに済む。** UserParameter の引数に書くとプロセス一覧や
  Zabbix の設定画面に出てしまいます。

## 1. インストール

### RPM（推奨）

[Releases](https://github.com/serkenn/zabbix-smartmeter/releases) から対象アーキテクチャの RPM を取得します。
RPM のファイル名にはバージョンが入るため、`latest/download/` ではなくタグ付きの URL を使います。

```bash
# x86_64 の例。VER は Releases ページで確認したバージョンに置き換える
VER=0.1.0
curl -LO "https://github.com/serkenn/zabbix-smartmeter/releases/download/v${VER}/zabbix-smartmeter-${VER}-1.x86_64.rpm"
sudo dnf install "./zabbix-smartmeter-${VER}-1.x86_64.rpm"
```

`gh` コマンドが使えるなら、バージョンを調べずに取得できます。

```bash
gh release download --repo serkenn/zabbix-smartmeter --pattern '*.x86_64.rpm'
sudo dnf install ./zabbix-smartmeter-*.x86_64.rpm
```

> **RPM / deb は v0.0.6 より後のリリースから提供されます。**
> v0.0.6 以前は旧ワークフローで作られているため zip しかありません。
> その場合は下の「zip（RPM を使わない場合）」の手順を使ってください。

アーキテクチャ別のファイル名は次のとおりです。

| 環境 | RPM | deb |
| --- | --- | --- |
| x86_64 | `...x86_64.rpm` | `..._amd64.deb` |
| Raspberry Pi (64bit) | `...aarch64.rpm` | `..._arm64.deb` |
| Raspberry Pi (32bit) | `...armv6hl.rpm` | `..._armhf.deb` |

RPM は以下を配置します。

| パス | 内容 |
| --- | --- |
| `/usr/bin/zabbix-smartmeter` | 実行ファイル |
| `/etc/sysconfig/zabbix-smartmeter` | B ルート認証情報などの設定（0640） |
| `/usr/lib/systemd/system/zabbix-smartmeter.{service,timer}` | 定期収集 |
| `/etc/udev/rules.d/99-zabbix-smartmeter.rules` | デバイス名固定用のひな形（既定では無効） |
| `/usr/share/zabbix-smartmeter/` | Zabbix テンプレートと UserParameter サンプル |

### zip（RPM を使わない場合）

```bash
curl -LO https://github.com/serkenn/zabbix-smartmeter/releases/latest/download/zabbix-smartmeter_linux-amd64.zip
unzip zabbix-smartmeter_linux-amd64.zip
sudo install -m 0755 zabbix-smartmeter /usr/local/bin/
sudo install -m 0644 packaging/systemd/zabbix-smartmeter.service /etc/systemd/system/
sudo install -m 0644 packaging/systemd/zabbix-smartmeter.timer   /etc/systemd/system/
sudo install -m 0640 packaging/sysconfig/zabbix-smartmeter       /etc/sysconfig/
sudo mkdir -p /var/lib/zabbix && sudo restorecon -R /var/lib/zabbix
```

`/usr/local/bin` に置いた場合は unit の `ExecStart` のパスを合わせて修正してください。

## 2. シリアルポートの確認

```bash
# 接続されている USB シリアルを探す
ls -l /dev/ttyUSB* /dev/ttyACM*
# どのモジュールか確認する
udevadm info --name=/dev/ttyUSB0 | grep -E 'ID_VENDOR|ID_MODEL|ID_SERIAL'
```

USB ポートの抜き差しで番号が入れ替わる環境では、`/etc/udev/rules.d/99-zabbix-smartmeter.rules` の
コメントを外して `idVendor` / `idProduct` を自分のモジュールの値に書き換え、`/dev/wisun0` のような
固定名を使うと安定します。

```bash
sudo udevadm control --reload-rules && sudo udevadm trigger
```

## 3. 設定

```bash
sudo vi /etc/sysconfig/zabbix-smartmeter
```

最低限、以下の 3 つを設定します。

```sh
SMARTMETER_DEVICE=/dev/ttyUSB0
SMARTMETER_ID=00000000000000000000000000000000
SMARTMETER_PASSWORD=XXXXXXXXXXXX
```

UDG-1-WSNE（NTT 東日本／西日本の「見えるでんき」USB アダプタ）を使う場合は、
`SMARTMETER_DSE=true` が必須です。

## 4. 疎通確認

まずスキャンでスマートメーターが見つかるか確認します（数分かかります）。

設定ファイルを読み込んでから実行します。`/etc/sysconfig/zabbix-smartmeter` は root しか
読めない 0640 なので、`sudo` の内側で読み込むのがポイントです。認証情報がコマンドラインに
出ないので、`ps` や shell の履歴にも残りません。

```bash
sudo bash -c 'set -a; . /etc/sysconfig/zabbix-smartmeter; set +a; /usr/bin/zabbix-smartmeter --scan'
```

> 設定ファイルを読み込まずに `--id "$SMARTMETER_ID"` のようにシェル変数を渡すと、
> 変数が空のまま展開されて `scan failed: id not specified` になります。
> `sudo` は既定で環境変数を引き継がないため、`export` しただけでも同じです。

オプションを直接渡して試すこともできます。この場合はパスワードが履歴に残る点に注意してください。

```bash
sudo /usr/bin/zabbix-smartmeter --scan --dse \
  --device /dev/ttyUSB0 --id 'BルートID' --password 'Bルートパスワード'
```

成功すると次のような出力になります。

```
<< "EPANDESC"
<<   "  Channel:3B"            ← SMARTMETER_CHANNEL に設定する値
<<   "  Pan ID:04E9"
<<   "  Addr:60D51BFFFE26AF9E" ← MACアドレス。SMARTMETER_IPADDR ではない
>> "SKLL64 60D51BFFFE26AF9E"
<< "FE80:0000:0000:0000:62D5:1BFF:FE26:AF9E"   ← SMARTMETER_IPADDR に設定する値
```

この 2 つを `/etc/sysconfig/zabbix-smartmeter` に設定しておくと、毎回のスキャンが省けて収集が速くなります。

```sh
SMARTMETER_CHANNEL=3B
SMARTMETER_IPADDR=FE80:0000:0000:0000:62D5:1BFF:FE26:AF9E
```

> `Addr:` の値（MACアドレス）を `SMARTMETER_IPADDR` に入れると、`SKSENDTO` の宛先が不正になり
> `SK command response error: FAIL ER05` で失敗します。最後の `SKLL64` の応答にある
> `FE80:` で始まる値を使ってください。

続いて 1 回だけ収集してみます。

```bash
sudo systemctl start zabbix-smartmeter.service
sudo journalctl -u zabbix-smartmeter.service -n 50 --no-pager
cat /var/lib/zabbix/smartmeter.json
```

次のような JSON が出れば成功です。

```json
{"r":7.5,"t":3,"total_normal":12345.6,"total_reverse":0,"value":1050}
```

## 5. 定期収集を開始する

```bash
sudo systemctl enable --now zabbix-smartmeter.timer
systemctl list-timers zabbix-smartmeter.timer
```

間隔を変えたい場合は `systemctl edit zabbix-smartmeter.timer` で `OnUnitActiveSec` を上書きします。
B ルートは 1 分より短い間隔での連続取得に向きません。

## 6. SELinux

AlmaLinux は既定で SELinux が Enforcing です。**`zabbix_agentd` を使う場合は、
ポリシーモジュールの追加が 1 回だけ必要です。**

### なぜ必要か

`/var/lib/zabbix` に付く `zabbix_var_lib_t` は Zabbix **サーバー** 用の型で、
拘束された agent（`zabbix_agentd` は `zabbix_agent_t` で動く）には読み取りが
与えられていません。そのため `vfs.file.contents[]` が次の拒否で失敗します。

```
avc: denied { read } for comm="zabbix_agentd" name="smartmeter.json"
  scontext=system_u:system_r:zabbix_agent_t:s0
  tcontext=system_u:object_r:zabbix_var_lib_t:s0 tclass=file permissive=0
```

Zabbix フロントエンドには `Cannot open file: [13] Permission denied` と表示されます。
パーミッションは正しくても（ファイルは `0644 root:root`、ディレクトリは `0755 zabbix:zabbix`）
起きるので、`ls -l` だけでは原因が分かりません。

> `zabbix_agent2` は SELinux ポリシーを同梱しておらず `unconfined_service_t` で動くため
> （[ZBX-23058](https://support.zabbix.com/browse/ZBX-23058)）、この拒否は起きません。
> agent2 を使っている場合はこの節を飛ばせます。

### 対処 1: 実際の拒否からモジュールを作る（推奨）

自分の環境で起きた拒否だけを許可するので確実です。

```bash
sudo dnf install -y policycoreutils-python-utils

# 追加されるルールを先に確認する
sudo ausearch -m AVC -ts today -c zabbix_agentd | audit2allow

# 問題なければモジュール化して読み込む
sudo ausearch -m AVC -ts today -c zabbix_agentd | audit2allow -M zabbix-smartmeter-local
sudo semodule -i zabbix-smartmeter-local.pp
```

`-c zabbix_agentd` で絞るのは、agent が `system.users` などで起動する `who` の拒否を
一緒に許可してしまわないためです。

### 対処 2: 同梱のポリシーを使う

[`packaging/selinux/zabbix_smartmeter.te`](../packaging/selinux/zabbix_smartmeter.te)
に同じ内容を最小のモジュールとして置いています（RPM では
`/usr/share/zabbix-smartmeter/selinux/zabbix_smartmeter.te`）。

```bash
sudo dnf install -y checkpolicy policycoreutils-devel
checkmodule -M -m -o zabbix_smartmeter.mod zabbix_smartmeter.te
semodule_package -o zabbix_smartmeter.pp -m zabbix_smartmeter.mod
sudo semodule -i zabbix_smartmeter.pp
```

どちらの方法も、agent に `/var/lib/zabbix` 配下の読み取りを許可します。サーバーと agent を
同じホストで動かしている場合、agent からサーバーのデータファイルも読めるようになります。

### 専用の型を使う（より権限を絞る）

出力先だけを別の型にして、その型の読み取りだけを許可する方法もあります。手数は増えますが、
agent に `/var/lib/zabbix` 全体の読み取りを与えずに済みます。`selinux-policy-devel` の
refpolicy マクロで型を定義し、出力先のパスにその型を割り当ててください。

### ラベルの確認

出力先を `/var/lib/zabbix` 以外に変えた場合は、ラベルも合わせます。

```bash
ls -Z /var/lib/zabbix/smartmeter.json
sudo restorecon -Rv /var/lib/zabbix

# 別のパスに出力する場合
sudo semanage fcontext -a -t zabbix_var_lib_t '/opt/smartmeter(/.*)?'
sudo restorecon -Rv /opt/smartmeter
```

収集サービス側は systemd から起動され `unconfined_service_t` で動くため、
シリアルポートへのアクセスは追加のポリシーなしで通ります。

> `setenforce 0` で運用するのは避けてください。原因の切り分け時のみ一時的に使い、
> 判明したら上記の手順でポリシーを調整します。

## 7. Zabbix テンプレートの取り込み

Zabbix フロントエンドの **Data collection → Templates → Import** から読み込みます。

- Zabbix 6.0 以降: `/usr/share/zabbix-smartmeter/zabbix_smartmeter_template.yaml`
- XML を使いたい場合: `/usr/share/zabbix-smartmeter/zabbix_smartmeter_template.xml`

取り込んだら、Wi-SUN モジュールを挿しているホストに `Template Smartmeter` をリンクします。
ホスト側で調整できるマクロは次の 3 つです。

| マクロ | 既定値 | 用途 |
| --- | --- | --- |
| `{$SMARTMETER.JSON_PATH}` | `/var/lib/zabbix/smartmeter.json` | 読み取る JSON のパス |
| `{$SMARTMETER.POWER.MAX}` | `5000` | 瞬時電力の警告閾値 [W] |
| `{$SMARTMETER.STALE}` | `10m` | この時間更新がなければ障害とみなす |

Zabbix agent 側の設定変更は不要です（`vfs.file.contents` は組み込みアイテム）。
`EnableRemoteCommands` や UserParameter の追加もいりません。

UserParameter キーで取りたい場合のみ、`/usr/share/zabbix-smartmeter/userparameter_smartmeter.conf` を
`/etc/zabbix/zabbix_agent2.d/`（agent2）または `/etc/zabbix/zabbix_agentd.d/`（agent）に置いて
agent を再起動してください。

## 8. 非 root で動かす

既定ではシリアルポートへのアクセス権の問題を避けるため root で動かしていますが、
専用ユーザーに切り替えられます。

一番簡単なのは `zabbix` ユーザーで動かす方法です。出力先 `/var/lib/zabbix` の所有者と
一致するため、ディレクトリのパーミッション調整も `CAP_DAC_OVERRIDE` も要りません。

```bash
sudo systemctl edit zabbix-smartmeter.service
```

```ini
[Service]
User=zabbix
Group=zabbix
# シリアルポートを読むために必要
SupplementaryGroups=dialout
# 所有者が一致するので特権は不要
CapabilityBoundingSet=
```

`/etc/sysconfig/zabbix-smartmeter` は systemd が権限を落とす前に root として読むので、
0640 のままで問題ありません。

専用ユーザーを作る場合は、出力先に書き込める状態にしてください。

```bash
sudo useradd --system --no-create-home --shell /sbin/nologin smartmeter
sudo usermod -aG dialout smartmeter
sudo setfacl -m u:smartmeter:rwx /var/lib/zabbix
```

```ini
[Service]
User=smartmeter
Group=smartmeter
SupplementaryGroups=dialout
CapabilityBoundingSet=
```

## 9. トラブルシューティング

| 症状 | 確認すること |
| --- | --- |
| `Cannot open file: [13] Permission denied` | `zabbix_agentd` なら SELinux のポリシーモジュールが必要（上記 6 節）。`sudo ausearch -m AVC -ts today -c zabbix_agentd` で確認 |
| Item が Not supported | 上記の AVC を確認。`sudo -u zabbix cat ...` は呼び出し元のコンテキストで動くため、成功しても agent から読めるとは限らない |
| JSON が更新されない | `systemctl status zabbix-smartmeter.timer`、`journalctl -u zabbix-smartmeter.service` |
| `failed to open "/dev/ttyUSB0"` | デバイスの存在と権限。`DeviceAllow` を増やす必要がないか |
| `FAIL ER05` | `SMARTMETER_IPADDR` に MAC アドレスを設定していないか（上記 4 節） |
| `failed to create temp file in "/var/lib/zabbix": permission denied` | ユニットの `CapabilityBoundingSet` に `CAP_DAC_OVERRIDE` があるか。`/var/lib/zabbix` は `zabbix` 所有のため root でも必要 |
| `scan failed: id not specified` | B ルート ID が空。設定ファイルを読み込んでから実行する（上記 4 節） |
| スキャンでメーターが見つからない | B ルート ID / パスワードの誤り、`--dse` の付け忘れ、モジュールの設置場所 |
| `total_normal` が極端な値 | メーターが係数(0xD3)・単位(0xE1)を返しているか。`--verbosity 3` でログを確認 |

ログは既定で journald に出ます（systemd 以外から起動した場合は syslog）。
`--log stderr` で強制的に標準エラー出力に出せます。詳細は `--verbosity 3` を付けてください。
