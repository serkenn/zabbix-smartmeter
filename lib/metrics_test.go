package mpsm

import (
	"errors"
	"math"
	"testing"

	smartmeter "github.com/hnw/go-smartmeter"
)

func prop(epc smartmeter.PropertyCode, edt ...byte) *smartmeter.Property {
	return smartmeter.NewProperty(epc, edt)
}

const (
	epcPower       = smartmeter.LvSmartElectricEnergyMeterInstantaneousElectricPower
	epcCurrent     = smartmeter.LvSmartElectricEnergyMeterInstantaneousCurrent
	epcCoef        = smartmeter.LvSmartElectricEnergyMeterCoefficient
	epcUnit        = smartmeter.LvSmartElectricEnergyMeterUnitForCumulativeAmountsOfElectricEnergy
	epcTotalNorm   = smartmeter.LvSmartElectricEnergyMeterNormalDirectionCumulativeElectricEnergy
	epcTotalRev    = smartmeter.LvSmartElectricEnergyMeterReverseDirectionCumulativeElectricEnergy
	floatTolerance = 1e-9
)

func TestParseProperties(t *testing.T) {
	tests := []struct {
		name  string
		props []*smartmeter.Property
		want  map[string]float64
	}{
		{
			name:  "瞬時電力",
			props: []*smartmeter.Property{prop(epcPower, 0x00, 0x00, 0x02, 0x58)},
			want:  map[string]float64{MetricPower: 600},
		},
		{
			name:  "瞬時電力が負（売電中）",
			props: []*smartmeter.Property{prop(epcPower, 0xff, 0xff, 0xfd, 0xa8)},
			want:  map[string]float64{MetricPower: -600},
		},
		{
			name:  "瞬時電流はR相T相とも0.1A単位",
			props: []*smartmeter.Property{prop(epcCurrent, 0x00, 0x4b, 0xff, 0xf6)},
			want:  map[string]float64{MetricCurrentR: 7.5, MetricCurrentT: -1.0},
		},
		{
			name: "積算電力量は係数と単位で換算する",
			props: []*smartmeter.Property{
				prop(epcCoef, 0x00, 0x00, 0x00, 0x01),
				prop(epcUnit, 0x01),                        // 0.1kWh
				prop(epcTotalNorm, 0x00, 0x01, 0x86, 0xa0), // 100000 count
				prop(epcTotalRev, 0x00, 0x00, 0x03, 0xe8),  // 1000 count
			},
			want: map[string]float64{MetricTotalNormal: 10000, MetricTotalReverse: 100},
		},
		{
			name: "単位と係数が積算値より後に来ても換算できる",
			props: []*smartmeter.Property{
				prop(epcTotalNorm, 0x00, 0x00, 0x00, 0x64), // 100 count
				prop(epcUnit, 0x02),                        // 0.01kWh
				prop(epcCoef, 0x00, 0x00, 0x00, 0x0a),      // 係数10
			},
			want: map[string]float64{MetricTotalNormal: 10},
		},
		{
			name:  "単位が無ければ0.1kWh/countとみなす",
			props: []*smartmeter.Property{prop(epcTotalNorm, 0x00, 0x00, 0x00, 0x0a)},
			want:  map[string]float64{MetricTotalNormal: 1},
		},
		{
			name: "長さが不正なプロパティは読み飛ばす",
			props: []*smartmeter.Property{
				prop(epcPower, 0x01, 0x02),
				prop(epcCurrent, 0x00, 0x4b, 0x00, 0x4b),
			},
			want: map[string]float64{MetricCurrentR: 7.5, MetricCurrentT: 7.5},
		},
		{
			name: "全プロパティ",
			props: []*smartmeter.Property{
				prop(epcPower, 0x00, 0x00, 0x02, 0x58),
				prop(epcCurrent, 0x00, 0x4b, 0x00, 0x1e),
				prop(epcCoef, 0x00, 0x00, 0x00, 0x01),
				prop(epcUnit, 0x01),
				prop(epcTotalNorm, 0x00, 0x00, 0x27, 0x10),
				prop(epcTotalRev, 0x00, 0x00, 0x00, 0x00),
			},
			want: map[string]float64{
				MetricPower:        600,
				MetricCurrentR:     7.5,
				MetricCurrentT:     3.0,
				MetricTotalNormal:  1000,
				MetricTotalReverse: 0,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseProperties(tt.props)
			if err != nil {
				t.Fatalf("ParseProperties() returned error: %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("ParseProperties() = %v, want %v", got, tt.want)
			}
			for k, want := range tt.want {
				v, ok := got[k]
				if !ok {
					t.Errorf("metric %q is missing in %v", k, got)
					continue
				}
				if math.Abs(v-want) > floatTolerance {
					t.Errorf("metric %q = %v, want %v", k, v, want)
				}
			}
		})
	}
}

func TestParsePropertiesNoMetrics(t *testing.T) {
	tests := []struct {
		name  string
		props []*smartmeter.Property
	}{
		{name: "プロパティ無し", props: nil},
		{name: "空スライス", props: []*smartmeter.Property{}},
		{name: "nil要素のみ", props: []*smartmeter.Property{nil}},
		{name: "解釈できないEPCのみ", props: []*smartmeter.Property{prop(0x80, 0x30)}},
		{name: "係数と単位のみ", props: []*smartmeter.Property{prop(epcUnit, 0x01)}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseProperties(tt.props)
			if !errors.Is(err, ErrNoMetrics) {
				t.Fatalf("ParseProperties() error = %v, want ErrNoMetrics", err)
			}
			if got != nil {
				t.Errorf("ParseProperties() = %v, want nil", got)
			}
		})
	}
}

func TestEnergyUnit(t *testing.T) {
	tests := []struct {
		edt  byte
		want float64
	}{
		{0x00, 1},
		{0x01, 0.1},
		{0x02, 0.01},
		{0x03, 0.001},
		{0x04, 0.0001},
		{0x0a, 10},
		{0x0b, 100},
		{0x0c, 1000},
		{0x0d, 10000},
		{0x7f, 1}, // 規定外
	}
	for _, tt := range tests {
		if got := energyUnit(tt.edt); math.Abs(got-tt.want) > floatTolerance {
			t.Errorf("energyUnit(0x%02x) = %v, want %v", tt.edt, got, tt.want)
		}
	}
}

func TestRequestProperties(t *testing.T) {
	want := []smartmeter.PropertyCode{
		epcPower, epcCurrent, epcCoef, epcUnit, epcTotalNorm, epcTotalRev,
	}
	got := requestProperties()
	if len(got) != len(want) {
		t.Fatalf("requestProperties() returned %d properties, want %d", len(got), len(want))
	}
	for i, epc := range want {
		if got[i].EPC != epc {
			t.Errorf("requestProperties()[%d].EPC = 0x%02x, want 0x%02x", i, got[i].EPC, epc)
		}
		if got[i].EDT != nil {
			t.Errorf("requestProperties()[%d].EDT = %v, want nil", i, got[i].EDT)
		}
	}
}
