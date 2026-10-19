package mpsm

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParseOptionsDefaults(t *testing.T) {
	opt, err := parseOptions(nil)
	if err != nil {
		t.Fatalf("parseOptions() returned error: %v", err)
	}
	if opt.prefix != "smartmeter" {
		t.Errorf("prefix = %q, want %q", opt.prefix, "smartmeter")
	}
	if opt.logTarget != "auto" {
		t.Errorf("logTarget = %q, want %q", opt.logTarget, "auto")
	}
	if opt.verbosity != 1 {
		t.Errorf("verbosity = %d, want 1", opt.verbosity)
	}
	if opt.jsonMode || opt.scanMode || opt.dualStackSK {
		t.Errorf("boolean flags should default to false: %+v", opt)
	}
}

func TestParseOptionsFlagsBeatEnv(t *testing.T) {
	t.Setenv("SMARTMETER_DEVICE", "/dev/ttyUSB9")
	t.Setenv("SMARTMETER_ID", "env-id")
	t.Setenv("SMARTMETER_DSE", "true")
	t.Setenv("SMARTMETER_TIMEOUT", "45s")

	opt, err := parseOptions([]string{"-device", "/dev/ttyACM0", "-json"})
	if err != nil {
		t.Fatalf("parseOptions() returned error: %v", err)
	}
	if opt.serialPort != "/dev/ttyACM0" {
		t.Errorf("serialPort = %q, want the command line value", opt.serialPort)
	}
	if opt.bRouteID != "env-id" {
		t.Errorf("bRouteID = %q, want the environment value", opt.bRouteID)
	}
	if !opt.dualStackSK {
		t.Error("dualStackSK = false, want true from SMARTMETER_DSE")
	}
	if opt.timeout != 45*time.Second {
		t.Errorf("timeout = %v, want 45s", opt.timeout)
	}
	if !opt.jsonMode {
		t.Error("jsonMode = false, want true")
	}
}

func TestParseOptionsInvalidFlag(t *testing.T) {
	// ContinueOnErrorなのでos.Exitせずエラーが返ることを確認する。
	if _, err := parseOptions([]string{"-no-such-flag"}); err == nil {
		t.Fatal("parseOptions() returned nil error for an unknown flag")
	}
	if _, err := parseOptions([]string{"-h"}); err != flag.ErrHelp { //nolint:errorlint // flagパッケージは同一値を返す
		t.Fatalf("parseOptions(-h) error = %v, want flag.ErrHelp", err)
	}
}

func TestWriteFileAtomic(t *testing.T) {
	dir := t.TempDir()
	name := filepath.Join(dir, "smartmeter.json")

	if err := writeFileAtomic(name, []byte(`{"value":1}`), 0o644); err != nil {
		t.Fatalf("writeFileAtomic() returned error: %v", err)
	}
	// 2回目の書き込みで上書きできること。
	if err := writeFileAtomic(name, []byte(`{"value":2}`), 0o644); err != nil {
		t.Fatalf("writeFileAtomic() on existing file returned error: %v", err)
	}

	got, err := os.ReadFile(name)
	if err != nil {
		t.Fatalf("ReadFile() returned error: %v", err)
	}
	if string(got) != `{"value":2}` {
		t.Errorf("file content = %q, want the latest write", got)
	}

	fi, err := os.Stat(name)
	if err != nil {
		t.Fatalf("Stat() returned error: %v", err)
	}
	if perm := fi.Mode().Perm(); perm != 0o644 {
		t.Errorf("file mode = %o, want 644", perm)
	}

	// 一時ファイルが残っていないこと。
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir() returned error: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("directory holds %d entries, want only the target file", len(entries))
	}
}

func TestWriteFileAtomicMissingDirectory(t *testing.T) {
	name := filepath.Join(t.TempDir(), "missing", "smartmeter.json")
	if err := writeFileAtomic(name, []byte("{}"), 0o644); err == nil {
		t.Fatal("writeFileAtomic() returned nil error for a missing directory")
	}
}

func TestRunJSONWritesParsableFile(t *testing.T) {
	// FetchMetricsはシリアルポートを必要とするため、出力部分だけを検証する。
	name := filepath.Join(t.TempDir(), "smartmeter.json")
	metrics := map[string]float64{MetricPower: 600, MetricCurrentR: 7.5}

	buf, err := json.Marshal(metrics)
	if err != nil {
		t.Fatalf("Marshal() returned error: %v", err)
	}
	if err := writeFileAtomic(name, append(buf, '\n'), 0o644); err != nil {
		t.Fatalf("writeFileAtomic() returned error: %v", err)
	}

	raw, err := os.ReadFile(name)
	if err != nil {
		t.Fatalf("ReadFile() returned error: %v", err)
	}
	var got map[string]float64
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("written file is not valid JSON: %v", err)
	}
	if got[MetricPower] != 600 {
		t.Errorf("%q = %v, want 600", MetricPower, got[MetricPower])
	}
}

func TestNewLogWriter(t *testing.T) {
	t.Run("scanモードは標準出力", func(t *testing.T) {
		w, err := newLogWriter(&options{scanMode: true, logTarget: "syslog"})
		if err != nil {
			t.Fatalf("newLogWriter() returned error: %v", err)
		}
		if w != os.Stdout {
			t.Errorf("newLogWriter() = %v, want os.Stdout", w)
		}
	})

	t.Run("stderr指定", func(t *testing.T) {
		w, err := newLogWriter(&options{logTarget: "stderr"})
		if err != nil {
			t.Fatalf("newLogWriter() returned error: %v", err)
		}
		if w != os.Stderr {
			t.Errorf("newLogWriter() = %v, want os.Stderr", w)
		}
	})

	t.Run("systemd配下のautoはstderr", func(t *testing.T) {
		t.Setenv("JOURNAL_STREAM", "8:123456")
		w, err := newLogWriter(&options{logTarget: "auto"})
		if err != nil {
			t.Fatalf("newLogWriter() returned error: %v", err)
		}
		if w != os.Stderr {
			t.Errorf("newLogWriter() = %v, want os.Stderr", w)
		}
	})

	t.Run("不正な値はエラー", func(t *testing.T) {
		if _, err := newLogWriter(&options{logTarget: "nowhere"}); err == nil {
			t.Fatal("newLogWriter() returned nil error for an invalid target")
		}
	})
}

func TestGraphDefinitionCoversAllMetrics(t *testing.T) {
	defined := map[string]bool{}
	for _, g := range (SmartmeterPlugin{}).GraphDefinition() {
		for _, m := range g.Metrics {
			defined[m.Name] = true
		}
	}
	for _, name := range []string{MetricPower, MetricCurrentR, MetricCurrentT, MetricTotalNormal, MetricTotalReverse} {
		if !defined[name] {
			t.Errorf("metric %q has no graph definition", name)
		}
	}
}

func TestMetricKeyPrefix(t *testing.T) {
	if got := (SmartmeterPlugin{}).MetricKeyPrefix(); got != "smartmeter" {
		t.Errorf("MetricKeyPrefix() = %q, want %q", got, "smartmeter")
	}
	if got := (SmartmeterPlugin{Prefix: "home"}).MetricKeyPrefix(); got != "home" {
		t.Errorf("MetricKeyPrefix() = %q, want %q", got, "home")
	}
}

func TestEnvHelpers(t *testing.T) {
	t.Setenv("ZSM_TEST_EMPTY", "")
	if got := envString("ZSM_TEST_EMPTY", "fallback"); got != "fallback" {
		t.Errorf("envString() = %q, want the fallback for an empty value", got)
	}
	t.Setenv("ZSM_TEST_BOOL", "not-a-bool")
	if got := envBool("ZSM_TEST_BOOL", true); !got {
		t.Error("envBool() = false, want the fallback for an unparsable value")
	}
	t.Setenv("ZSM_TEST_INT", "3")
	if got := envInt("ZSM_TEST_INT", 1); got != 3 {
		t.Errorf("envInt() = %d, want 3", got)
	}
	t.Setenv("ZSM_TEST_DURATION", "90s")
	if got := envDuration("ZSM_TEST_DURATION", 0); got != 90*time.Second {
		t.Errorf("envDuration() = %v, want 90s", got)
	}
}
