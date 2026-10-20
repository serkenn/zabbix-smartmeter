"""同一定義から Zabbix テンプレートの YAML/XML を生成する。

テンプレートを変更するときはこのスクリプトを編集し、リポジトリのルートで

    python3 zabbix/generate_template.py

を実行して zabbix/zabbix_smartmeter_template.{yaml,xml} を再生成する。
手で片方だけ編集すると2つのファイルがずれるので注意。
"""
import re
import xml.etree.ElementTree as ET
from xml.dom import minidom

TEMPLATE = "Template Smartmeter"
MASTER_KEY = "vfs.file.contents[{$SMARTMETER.JSON_PATH}]"
POWER_GRAPH = "Smartmeter: Instantaneous power"
CURRENT_GRAPH = "Smartmeter: Current"
ENERGY_GRAPH = "Smartmeter: Cumulative energy"

def dep(uuid, name, key, jsonpath, value_type, units, desc, tag, extra_steps=None, triggers=None):
    item = {
        "uuid": uuid, "name": name, "type": "DEPENDENT", "key": key,
        "delay": "0", "history": "90d", "value_type": value_type,
        "units": units, "description": desc,
        "preprocessing": [{"type": "JSONPATH", "parameters": [jsonpath]}] + (extra_steps or []),
        "master_item": {"key": MASTER_KEY},
        "tags": [{"tag": "component", "value": tag}],
    }
    if triggers:
        item["triggers"] = triggers
    return item

HEARTBEAT = {"type": "DISCARD_UNCHANGED_HEARTBEAT", "parameters": ["1h"]}

items = [
    {
        "uuid": "50320482592446758661730044521463",
        "name": "Smartmeter raw data",
        "key": MASTER_KEY,
        "delay": "1m",
        "history": "0",
        "trends": "0",
        "value_type": "TEXT",
        "description": (
            "zabbix-smartmeter.timer が書き出した JSON をそのまま読み取るマスターアイテム。\n"
            "個々の値は依存アイテムとして取り出す。収集自体はエージェントの外で行うため、"
            "Bルートの応答が遅くても agent の Timeout に影響しない。"
        ),
        "tags": [{"tag": "component", "value": "smartmeter"}],
    },
    dep("96614458316248389642055621441864", "Instantaneous power", "smartmeter.power",
        "$.value", "FLOAT", "W", "瞬時電力計測値。太陽光の逆潮流時は負の値になる。", "power",
        triggers=[{
            "uuid": "39957ab4cd2c401e8a5e7ba4377f1a3a",
            "expression": "min(/%s/smartmeter.power,5m)>{$SMARTMETER.POWER.MAX}" % TEMPLATE,
            "name": "Smartmeter: 瞬時電力が 5 分間 {$SMARTMETER.POWER.MAX} W を超えています",
            "priority": "WARNING",
            "description": "契約容量に近い電力を使い続けている。ブレーカー遮断の恐れがある。",
            "tags": [{"tag": "scope", "value": "capacity"}],
        }]),
    dep("40866504229344168015509922003661", "Current R-phase", "smartmeter.current.r",
        "$.r", "FLOAT", "A", "R相の瞬時電流計測値。", "current"),
    dep("15016503080546118491626084661005", "Current T-phase", "smartmeter.current.t",
        "$.t", "FLOAT", "A", "T相の瞬時電流計測値。", "current"),
    dep("13d8d653a62c4d2eb9f1ae1ed12a6669", "Cumulative energy (bought)", "smartmeter.total.normal",
        "$.total_normal", "FLOAT", "kWh",
        "積算電力量（正方向・買電）。係数(0xD3)と積算電力量単位(0xE1)で換算済み。",
        "energy", extra_steps=[HEARTBEAT]),
    dep("9621ff6d1b8f415aa511e113045fc3c7", "Cumulative energy (sold)", "smartmeter.total.reverse",
        "$.total_reverse", "FLOAT", "kWh",
        "積算電力量（逆方向・売電）。係数(0xD3)と積算電力量単位(0xE1)で換算済み。",
        "energy", extra_steps=[HEARTBEAT]),
    {
        "uuid": "b5a56aca1a90437aaea3b062faddf223",
        "name": "Data collected at",
        "key": "vfs.file.time[{$SMARTMETER.JSON_PATH},modify]",
        "delay": "1m",
        "history": "7d",
        "trends": "0",
        "units": "unixtime",
        "description": "JSON ファイルの更新時刻。収集サービスが止まっていないかの監視に使う。",
        "tags": [{"tag": "component", "value": "smartmeter"}],
        "triggers": [{
            "uuid": "8d58a4ae9eb64b879684b2fce9f79eab",
            "expression": "fuzzytime(/%s/vfs.file.time[{$SMARTMETER.JSON_PATH},modify],{$SMARTMETER.STALE})=0" % TEMPLATE,
            "name": "Smartmeter: データが {$SMARTMETER.STALE} 以上更新されていません",
            "priority": "WARNING",
            "description": (
                "zabbix-smartmeter.timer が停止しているか、Wi-SUN モジュールと通信できていない。\n"
                "  systemctl status zabbix-smartmeter.timer\n"
                "  journalctl -u zabbix-smartmeter.service -n 50"
            ),
            "tags": [{"tag": "scope", "value": "availability"}],
        }],
    },
]

macros = [
    {"macro": "{$SMARTMETER.JSON_PATH}", "value": "/var/lib/zabbix/smartmeter.json",
     "description": "zabbix-smartmeter が書き出す JSON ファイルのパス。"},
    {"macro": "{$SMARTMETER.POWER.MAX}", "value": "5000",
     "description": "瞬時電力の警告閾値 [W]。契約容量に合わせて調整する。"},
    {"macro": "{$SMARTMETER.STALE}", "value": "10m",
     "description": "データが更新されない状態を異常とみなすまでの時間。"},
]

dashboards = [{
    "uuid": "7e669d2e7ca34ed780604cee86db8caa",
    "name": "Smartmeter: Power and energy",
    "pages": [{
        "name": "Overview",
        "widgets": [
            {
                "type": "GRAPH_CLASSIC", "width": "24", "height": "5",
                "fields": [{
                    "type": "GRAPH", "name": "graphid",
                    "value": {"host": TEMPLATE, "name": POWER_GRAPH},
                }],
            },
            {
                "type": "GRAPH_CLASSIC", "y": "5", "width": "12", "height": "5",
                "fields": [{
                    "type": "GRAPH", "name": "graphid",
                    "value": {"host": TEMPLATE, "name": CURRENT_GRAPH},
                }],
            },
            {
                "type": "GRAPH_CLASSIC", "x": "12", "y": "5", "width": "12", "height": "5",
                "fields": [{
                    "type": "GRAPH", "name": "graphid",
                    "value": {"host": TEMPLATE, "name": ENERGY_GRAPH},
                }],
            },
        ],
    }],
}]

graphs = [
    {
        "uuid": "7c868f1824fb46b988c560ab76805125",
        "name": POWER_GRAPH,
        "show_work_period": "NO",
        "graph_items": [{
            "color": "E97659",
            "item": {"host": TEMPLATE, "key": "smartmeter.power"},
        }],
    },
    {
        "uuid": "3d25dda3ba2a492fa29e326b71372d94",
        "name": CURRENT_GRAPH,
        "show_work_period": "NO",
        "ymin_type_1": "FIXED",
        "graph_items": [
            {
                "color": "F63100",
                "item": {"host": TEMPLATE, "key": "smartmeter.current.r"},
            },
            {
                "sortorder": "1", "color": "2774A4",
                "item": {"host": TEMPLATE, "key": "smartmeter.current.t"},
            },
        ],
    },
    {
        "uuid": "35174609cbe341fe8e729b40014bcfb9",
        "name": ENERGY_GRAPH,
        "show_work_period": "NO",
        "graph_items": [
            {
                "color": "1976D2",
                "item": {"host": TEMPLATE, "key": "smartmeter.total.normal"},
            },
            {
                "sortorder": "1", "color": "4CAF50",
                "item": {"host": TEMPLATE, "key": "smartmeter.total.reverse"},
            },
        ],
    },
]

export = {
    "version": "6.0",
    "date": "2026-10-20T00:00:00Z",
    "groups": [{"uuid": "82531070505b4d729046028882756858", "name": "Templates/Smartmeter Project"}],
    "templates": [{
        "uuid": "0364d262145348879555138138981442",
        "template": TEMPLATE,
        "name": TEMPLATE,
        "description": (
            "USB 接続の Wi-SUN モジュールでスマートメーター（B ルート）から電力情報を取得する。\n"
            "https://github.com/serkenn/zabbix-smartmeter"
        ),
        "groups": [{"name": "Templates/Smartmeter Project"}],
        "items": items,
        "macros": macros,
        "dashboards": dashboards,
    }],
    "graphs": graphs,
}

# ---- YAML ----------------------------------------------------------------
def y(value, indent):
    pad = "  " * indent
    if isinstance(value, dict):
        out = []
        for k, v in value.items():
            if isinstance(v, (dict, list)):
                out.append("%s%s:" % (pad, scalar(k)))
                out.append(y(v, indent + 1))
            else:
                out.append("%s%s: %s" % (pad, scalar(k), scalar(v, indent + 1)))
        return "\n".join(out)
    if isinstance(value, list):
        out = []
        for v in value:
            if isinstance(v, dict):
                body = y(v, indent + 1).split("\n")
                out.append(pad + "- " + body[0].lstrip())
                out.extend(body[1:])
            else:
                out.append("%s- %s" % (pad, scalar(v, indent + 1)))
        return "\n".join(out)
    return pad + scalar(value, indent)

# YAMLが文字列以外（真偽値・数値・日時）として解釈してしまうキーや値のパターン。
# Zabbixは全フィールドを文字列として受け取るため、該当する文字列はクォートする。
# 例: date を裸で書くと datetime になり「文字列で指定してください」で弾かれる。
_YAML_TYPED = re.compile(
    r"""^(
        \d{4}-\d{2}-\d{2}([T ].*)?   # 日付・日時
        | \d{1,2}:\d{2}(:\d{2})?     # 時刻（60進数として解釈される）
        | [-+]?0[xXoObB][0-9a-fA-F_]+   # 16/8/2進数
        | [-+]?\.(inf|Inf|INF)         # 無限大
        | \.(nan|NaN|NAN)              # NaN
        | ~                             # null
    )$""",
    re.VERBOSE,
)


def scalar(v, indent=0):
    s = str(v)
    if "\n" in s:
        pad = "  " * (indent + 1)
        return "|\n" + "\n".join(pad + line if line else "" for line in s.split("\n"))
    needs_quote = (
        s == "" or s[0] in "{}[]&*#?|-<>=!%@`'\"" or ": " in s or s.endswith(":")
        or s.strip() != s or s.lower() in ("y", "n", "yes", "no", "true", "false", "null", "on", "off")
        or _YAML_TYPED.match(s) is not None
    )
    try:
        float(s)
        needs_quote = True
    except ValueError:
        pass
    if needs_quote:
        return "'" + s.replace("'", "''") + "'"
    return s

yaml_body = y({"zabbix_export": export}, 0)
# 複数行スカラーのインデント補正
with open("zabbix/zabbix_smartmeter_template.yaml", "w", encoding="utf-8") as f:
    f.write(yaml_body + "\n")

# ---- XML -----------------------------------------------------------------
SINGULAR = {
    "groups": "group", "templates": "template", "items": "item", "tags": "tag",
    "macros": "macro", "triggers": "trigger", "preprocessing": "step",
    "parameters": "parameter", "dashboards": "dashboard", "pages": "page",
    "widgets": "widget", "fields": "field", "graphs": "graph",
    "graph_items": "graph_item",
}

def build(parent, key, value):
    if isinstance(value, list):
        holder = ET.SubElement(parent, key)
        child_name = SINGULAR[key]
        for v in value:
            if isinstance(v, dict):
                el = ET.SubElement(holder, child_name)
                for k2, v2 in v.items():
                    build(el, k2, v2)
            else:
                ET.SubElement(holder, child_name).text = str(v)
    elif isinstance(value, dict):
        el = ET.SubElement(parent, key)
        for k2, v2 in value.items():
            build(el, k2, v2)
    else:
        ET.SubElement(parent, key).text = str(value)

root = ET.Element("zabbix_export")
for k, v in export.items():
    build(root, k, v)

pretty = minidom.parseString(ET.tostring(root, encoding="unicode")).toprettyxml(indent="    ", encoding=None)
pretty = pretty.replace('<?xml version="1.0" ?>', '<?xml version="1.0" encoding="UTF-8"?>', 1)
pretty = "\n".join(line for line in pretty.split("\n") if line.strip())
with open("zabbix/zabbix_smartmeter_template.xml", "w", encoding="utf-8") as f:
    f.write(pretty + "\n")
print("generated")
