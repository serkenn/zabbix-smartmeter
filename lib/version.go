package mpsm

import (
	"runtime"
	"runtime/debug"
)

// Version はビルドされたバイナリのバージョン文字列を返す。
func Version() string {
	if version != "dev" {
		return version
	}
	// go install などで -ldflags が渡されなかった場合はVCS情報で補う。
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, s := range info.Settings {
			if s.Key == "vcs.revision" && s.Value != "" {
				if len(s.Value) > 12 {
					return "dev-" + s.Value[:12]
				}
				return "dev-" + s.Value
			}
		}
	}
	return version
}

// runtimeInfo はビルドに使われたGoのバージョンと対象プラットフォームを返す。
func runtimeInfo() string {
	return runtime.Version() + " " + runtime.GOOS + "/" + runtime.GOARCH
}
