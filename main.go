// Command zabbix-smartmeter はUSB接続のWi-SUNモジュール経由でスマートメーター
// （Bルート）から電力情報を取得し、Zabbix向けのJSONまたはMackerelプラグイン
// 形式で出力する。
package main

import mpsm "github.com/serkenn/zabbix-smartmeter/lib"

func main() {
	mpsm.Do()
}
