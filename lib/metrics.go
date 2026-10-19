package mpsm

import (
	"encoding/binary"
	"errors"
	"math"

	smartmeter "github.com/hnw/go-smartmeter"
)

// メトリック名。JSON出力のキーとMackerelのメトリック名を兼ねる。
const (
	MetricPower        = "value"         // 瞬時電力計測値 [W]
	MetricCurrentR     = "r"             // 瞬時電流計測値（R相）[A]
	MetricCurrentT     = "t"             // 瞬時電流計測値（T相）[A]
	MetricTotalNormal  = "total_normal"  // 積算電力量（正方向・買電）[kWh]
	MetricTotalReverse = "total_reverse" // 積算電力量（逆方向・売電）[kWh]
)

// ErrNoMetrics はレスポンスから1つもメトリックを取り出せなかったことを表す。
var ErrNoMetrics = errors.New("smartmeter: no metric found in response")

// 積算電力量単位(0xE1)が応答に含まれなかった場合の既定値 [kWh/count]。
// Bルートで一般的な0.1kWh刻みを仮定する。
const defaultEnergyUnit = 0.1

// requestProperties はスマートメーターに問い合わせるプロパティの一覧を返す。
// 積算電力量(0xE0/0xE3)をkWhに換算するため、係数(0xD3)と単位(0xE1)も併せて取得する。
func requestProperties() []*smartmeter.Property {
	return []*smartmeter.Property{
		smartmeter.NewProperty(smartmeter.LvSmartElectricEnergyMeterInstantaneousElectricPower, nil),
		smartmeter.NewProperty(smartmeter.LvSmartElectricEnergyMeterInstantaneousCurrent, nil),
		smartmeter.NewProperty(smartmeter.LvSmartElectricEnergyMeterCoefficient, nil),
		smartmeter.NewProperty(smartmeter.LvSmartElectricEnergyMeterUnitForCumulativeAmountsOfElectricEnergy, nil),
		smartmeter.NewProperty(smartmeter.LvSmartElectricEnergyMeterNormalDirectionCumulativeElectricEnergy, nil),
		smartmeter.NewProperty(smartmeter.LvSmartElectricEnergyMeterReverseDirectionCumulativeElectricEnergy, nil),
	}
}

// ParseProperties はECHONET Liteのプロパティ列をメトリックに変換する。
// 壊れたプロパティは読み飛ばし、1つも解釈できなかった場合のみ ErrNoMetrics を返す。
func ParseProperties(props []*smartmeter.Property) (map[string]float64, error) {
	var (
		metrics           = make(map[string]float64, 5)
		coefficient       = 1.0
		unit              = defaultEnergyUnit
		rawNormal         uint32
		rawReverse        uint32
		hasNormal, hasRev bool
	)

	for _, p := range props {
		if p == nil {
			continue
		}
		switch p.EPC {
		case smartmeter.LvSmartElectricEnergyMeterInstantaneousElectricPower:
			if len(p.EDT) == 4 {
				metrics[MetricPower] = float64(int32(binary.BigEndian.Uint32(p.EDT)))
			}
		case smartmeter.LvSmartElectricEnergyMeterInstantaneousCurrent:
			if len(p.EDT) == 4 {
				metrics[MetricCurrentR] = float64(int16(binary.BigEndian.Uint16(p.EDT[:2]))) / 10.0
				metrics[MetricCurrentT] = float64(int16(binary.BigEndian.Uint16(p.EDT[2:]))) / 10.0
			}
		case smartmeter.LvSmartElectricEnergyMeterCoefficient:
			if len(p.EDT) == 4 {
				if c := binary.BigEndian.Uint32(p.EDT); c != 0 {
					coefficient = float64(c)
				}
			}
		case smartmeter.LvSmartElectricEnergyMeterUnitForCumulativeAmountsOfElectricEnergy:
			if len(p.EDT) == 1 {
				unit = energyUnit(p.EDT[0])
			}
		case smartmeter.LvSmartElectricEnergyMeterNormalDirectionCumulativeElectricEnergy:
			if len(p.EDT) == 4 {
				rawNormal, hasNormal = binary.BigEndian.Uint32(p.EDT), true
			}
		case smartmeter.LvSmartElectricEnergyMeterReverseDirectionCumulativeElectricEnergy:
			if len(p.EDT) == 4 {
				rawReverse, hasRev = binary.BigEndian.Uint32(p.EDT), true
			}
		}
	}

	// 係数と単位は積算電力量より後に現れることがあるため、ループを抜けてから換算する。
	if hasNormal {
		metrics[MetricTotalNormal] = float64(rawNormal) * coefficient * unit
	}
	if hasRev {
		metrics[MetricTotalReverse] = float64(rawReverse) * coefficient * unit
	}

	if len(metrics) == 0 {
		return nil, ErrNoMetrics
	}
	return metrics, nil
}

// energyUnit は積算電力量単位(0xE1)のEDTを 1カウントあたりのkWhに変換する。
// 規定外の値は1kWh/countとして扱う。
func energyUnit(edt byte) float64 {
	switch {
	case edt >= 0x01 && edt <= 0x04:
		return math.Pow(10, -float64(edt))
	case edt >= 0x0a && edt <= 0x0d:
		return math.Pow(10, float64(edt-0x09))
	default:
		return 1.0
	}
}
