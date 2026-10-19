// Package mpsm はWi-SUNモジュール経由でスマートメーター(Bルート)から
// 電力情報を取得し、Mackerelプラグイン形式またはJSON形式で出力する。
package mpsm

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"log/syslog"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"time"

	smartmeter "github.com/hnw/go-smartmeter"
	mp "github.com/mackerelio/go-mackerel-plugin"
)

// version はリリースビルド時に -ldflags "-X ...lib.version=v1.2.3" で埋め込む。
var version = "dev"

// SmartmeterPlugin mackerel plugin
type SmartmeterPlugin struct {
	Prefix   string
	dev      *smartmeter.Device
	ScanMode bool
	Timeout  time.Duration
}

// MetricKeyPrefix interface for PluginWithPrefix
func (p SmartmeterPlugin) MetricKeyPrefix() string {
	if p.Prefix == "" {
		p.Prefix = "smartmeter"
	}
	return p.Prefix
}

// GraphDefinition interface for mackerelplugin
func (p SmartmeterPlugin) GraphDefinition() map[string]mp.Graphs {
	return map[string]mp.Graphs{
		"power": {
			Label: "Electric power consumption [W]",
			Unit:  mp.UnitInteger,
			Metrics: []mp.Metrics{
				{Name: MetricPower, Label: "Electric power"},
			},
		},
		"current": {
			Label: "Electric current [A]",
			Unit:  mp.UnitFloat,
			Metrics: []mp.Metrics{
				{Name: MetricCurrentR, Label: "R-phase current", Stacked: true},
				{Name: MetricCurrentT, Label: "T-phase current", Stacked: true},
			},
		},
		"energy": {
			Label: "Cumulative electric energy [kWh]",
			Unit:  mp.UnitFloat,
			Metrics: []mp.Metrics{
				{Name: MetricTotalNormal, Label: "Normal direction (bought)"},
				{Name: MetricTotalReverse, Label: "Reverse direction (sold)"},
			},
		},
	}
}

// FetchMetrics interface for mackerelplugin
func (p SmartmeterPlugin) FetchMetrics() (map[string]float64, error) {
	// scan modeならスキャンして終了
	if p.ScanMode {
		if err := p.dev.Scan(smartmeter.Timeout(120*time.Second), smartmeter.Verbosity(3)); err != nil {
			return nil, fmt.Errorf("scan failed: %w", err)
		}
		return nil, nil
	}

	if p.dev.IPAddr == "" {
		ipAddr, err := p.dev.GetNeibourIP()
		if err == nil {
			p.dev.IPAddr = ipAddr
		}
	}

	opts := []smartmeter.Option{smartmeter.Retry(3)}
	if p.Timeout > 0 {
		opts = append(opts, smartmeter.Timeout(p.Timeout))
	}

	request := smartmeter.NewFrame(smartmeter.LvSmartElectricEnergyMeter, smartmeter.Get, requestProperties())

	// いきなり電力値取得を試みる
	response, err := p.dev.QueryEchonetLite(request, opts...)
	if err != nil {
		// 値が取得できなかったので、認証してから再度値を取る
		if authErr := p.dev.Authenticate(); authErr != nil {
			return nil, fmt.Errorf("query and authentication failed: %w", errors.Join(err, authErr))
		}
		response, err = p.dev.QueryEchonetLite(request, opts...)
		if err != nil {
			return nil, fmt.Errorf("query failed: %w", err)
		}
	}

	return ParseProperties(response.Properties)
}

type options struct {
	prefix         string
	tempfile       string
	bRouteID       string
	bRoutePassword string
	serialPort     string
	channel        string
	ipAddr         string
	output         string
	logTarget      string
	dualStackSK    bool
	scanMode       bool
	jsonMode       bool
	showVersion    bool
	verbosity      int
	timeout        time.Duration
}

func parseOptions(args []string) (*options, error) {
	opt := &options{}
	fs := flag.NewFlagSet(path.Base(os.Args[0]), flag.ContinueOnError)

	fs.StringVar(&opt.prefix, "metric-key-prefix", "smartmeter", "Metric key prefix")
	fs.StringVar(&opt.tempfile, "tempfile", "", "Temp file name")
	fs.StringVar(&opt.bRouteID, "id", envString("SMARTMETER_ID", ""), "B-route ID (env: SMARTMETER_ID)")
	fs.StringVar(&opt.bRoutePassword, "password", envString("SMARTMETER_PASSWORD", ""), "B-route password (env: SMARTMETER_PASSWORD)")
	fs.StringVar(&opt.serialPort, "device", envString("SMARTMETER_DEVICE", ""), "Path to serial port (env: SMARTMETER_DEVICE)")
	fs.StringVar(&opt.channel, "channel", envString("SMARTMETER_CHANNEL", ""), "Channel (env: SMARTMETER_CHANNEL)")
	fs.StringVar(&opt.ipAddr, "ipaddr", envString("SMARTMETER_IPADDR", ""), "IP address (env: SMARTMETER_IPADDR)")
	fs.StringVar(&opt.output, "output", envString("SMARTMETER_OUTPUT", ""), "Write the output to this file atomically instead of stdout (env: SMARTMETER_OUTPUT)")
	fs.StringVar(&opt.logTarget, "log", envString("SMARTMETER_LOG", "auto"), "Log destination: auto, syslog or stderr (env: SMARTMETER_LOG)")
	fs.BoolVar(&opt.dualStackSK, "dse", envBool("SMARTMETER_DSE", false), "Enable Dual Stack Edition (DSE) SK command (env: SMARTMETER_DSE)")
	fs.BoolVar(&opt.scanMode, "scan", false, "Scan mode")
	fs.BoolVar(&opt.jsonMode, "json", envBool("SMARTMETER_JSON", false), "Output in JSON format for Zabbix (env: SMARTMETER_JSON)")
	fs.BoolVar(&opt.showVersion, "version", false, "Show version and exit")
	fs.IntVar(&opt.verbosity, "verbosity", envInt("SMARTMETER_VERBOSITY", 1), "Verbosity (0:quiet, 3:debug) (env: SMARTMETER_VERBOSITY)")
	fs.DurationVar(&opt.timeout, "timeout", envDuration("SMARTMETER_TIMEOUT", 0), "Timeout for each ECHONET Lite query, e.g. 30s (0: library default) (env: SMARTMETER_TIMEOUT)")

	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	return opt, nil
}

// Do the plugin
func Do() {
	if err := Run(os.Args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		fmt.Fprintf(os.Stderr, "%s: %v\n", path.Base(os.Args[0]), err)
		os.Exit(1)
	}
}

// Run はコマンドライン引数を解釈して処理を実行する。
func Run(args []string) error {
	opt, err := parseOptions(args)
	if err != nil {
		return err
	}

	if opt.showVersion {
		fmt.Printf("%s %s (%s)\n", path.Base(os.Args[0]), Version(), runtimeInfo())
		return nil
	}

	// スキャンは SKSETRBID / SKSETPWD を必ず送るため、ID とパスワードが無いと必ず失敗する。
	// シリアルポートを開く前に、どこで設定するのかが分かる形で止める。
	if opt.scanMode && (opt.bRouteID == "" || opt.bRoutePassword == "") {
		return errors.New(
			"scan mode needs the B-route ID and password: pass --id and --password, " +
				"or set SMARTMETER_ID and SMARTMETER_PASSWORD " +
				"(on RPM installs they live in /etc/sysconfig/zabbix-smartmeter; " +
				`run: sudo bash -c 'set -a; . /etc/sysconfig/zabbix-smartmeter; set +a; zabbix-smartmeter --scan')`)
	}

	logWriter, err := newLogWriter(opt)
	if err != nil {
		return err
	}
	if closer, ok := logWriter.(io.Closer); ok {
		defer closer.Close() //nolint:errcheck // ログの後始末に失敗しても処理結果には影響しない
	}

	dev, err := smartmeter.Open(
		opt.serialPort,
		smartmeter.ID(opt.bRouteID),
		smartmeter.Password(opt.bRoutePassword),
		smartmeter.Channel(opt.channel),
		smartmeter.IPAddr(opt.ipAddr),
		smartmeter.DualStackSK(opt.dualStackSK),
		smartmeter.Verbosity(opt.verbosity),
		smartmeter.Logger(log.New(logWriter, "", 0)),
	)
	if err != nil {
		return fmt.Errorf("failed to open %q: %w", opt.serialPort, err)
	}

	p := SmartmeterPlugin{
		Prefix:   opt.prefix,
		dev:      dev,
		ScanMode: opt.scanMode,
		Timeout:  opt.timeout,
	}

	if opt.jsonMode {
		return runJSON(p, opt.output)
	}

	plugin := mp.NewMackerelPlugin(p)
	plugin.Tempfile = opt.tempfile
	plugin.Run()
	return nil
}

// runJSON はメトリックを取得してJSONとして出力する（Zabbix連携用）。
func runJSON(p SmartmeterPlugin, output string) error {
	metrics, err := p.FetchMetrics()
	if err != nil {
		return fmt.Errorf("failed to fetch metrics: %w", err)
	}
	if metrics == nil {
		metrics = map[string]float64{}
	}

	buf, err := json.Marshal(metrics)
	if err != nil {
		return fmt.Errorf("failed to marshal json: %w", err)
	}
	buf = append(buf, '\n')

	if output == "" {
		_, err = os.Stdout.Write(buf)
		return err
	}
	return writeFileAtomic(output, buf, 0o644)
}

// writeFileAtomic は同一ディレクトリの一時ファイル経由で書き込み、rename(2)で差し替える。
// Zabbix agentが読み取り中のファイルを中途半端な状態で読むことを防ぐ。
func writeFileAtomic(name string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(name)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(name)+".tmp")
	if err != nil {
		return fmt.Errorf("failed to create temp file in %q: %w", dir, err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) //nolint:errcheck // rename成功後は存在しないため失敗して構わない

	if _, err := tmp.Write(data); err != nil {
		tmp.Close() //nolint:errcheck,gosec // 書き込み失敗時のエラーを優先して返す
		return fmt.Errorf("failed to write %q: %w", tmpName, err)
	}
	if err := tmp.Chmod(perm); err != nil {
		tmp.Close() //nolint:errcheck,gosec // 同上
		return fmt.Errorf("failed to chmod %q: %w", tmpName, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("failed to close %q: %w", tmpName, err)
	}
	if err := os.Rename(tmpName, name); err != nil {
		return fmt.Errorf("failed to rename %q to %q: %w", tmpName, name, err)
	}
	return nil
}

// newLogWriter はログの出力先を決める。
// Mackerel/JSONモードでは標準出力にログを混ぜられないため、syslogか標準エラー出力に送る。
func newLogWriter(opt *options) (io.Writer, error) {
	if opt.scanMode {
		// scanモードはスキャン結果そのものをログとして標準出力に出す。
		return os.Stdout, nil
	}

	target := opt.logTarget
	if target == "auto" {
		// systemdから起動された場合は標準エラー出力がそのままjournaldに流れる。
		if os.Getenv("JOURNAL_STREAM") != "" {
			target = "stderr"
		} else {
			target = "syslog"
		}
	}

	switch target {
	case "stderr":
		return os.Stderr, nil
	case "syslog":
		w, err := syslog.New(syslog.LOG_NOTICE|syslog.LOG_USER, path.Base(os.Args[0]))
		if err != nil {
			return nil, fmt.Errorf("failed to connect to syslog: %w", err)
		}
		return w, nil
	default:
		return nil, fmt.Errorf("invalid -log value %q (want auto, syslog or stderr)", opt.logTarget)
	}
}

func envString(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return fallback
}

func envInt(key string, fallback int) int {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return fallback
}

func envDuration(key string, fallback time.Duration) time.Duration {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return fallback
}
