package openai

import "testing"

func TestIsCodexCLIRequest(t *testing.T) {
	tests := []struct {
		name string
		ua   string
		want bool
	}{
		{name: "codex_cli_rs 前缀", ua: "codex_cli_rs/0.1.0", want: true},
		{name: "codex_vscode 前缀", ua: "codex_vscode/1.2.3", want: true},
		{name: "大小写混合", ua: "Codex_CLI_Rs/0.1.0", want: true},
		{name: "复合 UA 包含 codex", ua: "Mozilla/5.0 codex_cli_rs/0.1.0", want: true},
		{name: "空白包裹", ua: "  codex_vscode/1.2.3  ", want: true},
		{name: "非 codex", ua: "curl/8.0.1", want: false},
		{name: "空字符串", ua: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsCodexCLIRequest(tt.ua)
			if got != tt.want {
				t.Fatalf("IsCodexCLIRequest(%q) = %v, want %v", tt.ua, got, tt.want)
			}
		})
	}
}

func TestIsCodexOfficialClientRequest(t *testing.T) {
	tests := []struct {
		name string
		ua   string
		want bool
	}{
		{name: "codex_cli_rs 前缀", ua: "codex_cli_rs/0.98.0", want: true},
		{name: "codex-tui 前缀", ua: "codex-tui/0.76.0", want: true},
		{name: "codex_vscode 前缀", ua: "codex_vscode/1.0.0", want: true},
		{name: "codex_vscode_copilot 前缀", ua: "codex_vscode_copilot/1.0.0", want: true},
		{name: "codex_app 前缀", ua: "codex_app/0.1.0", want: true},
		{name: "codex_chatgpt_desktop 前缀", ua: "codex_chatgpt_desktop/1.0.0", want: true},
		{name: "codex_atlas 前缀", ua: "codex_atlas/1.0.0", want: true},
		{name: "codex_exec 前缀", ua: "codex_exec/0.1.0", want: true},
		{name: "codex_sdk_ts 前缀", ua: "codex_sdk_ts/0.1.0", want: true},
		{name: "Codex 桌面 UA", ua: "Codex Desktop/1.2.3", want: true},
		{name: "Codex family trailing text", ua: "Codex Some Future Client/9.9", want: true},
		{name: "override UA trailer codex-tui", ua: "cccc/0.141.0 (Mac OS 26.5.0; arm64) Apple_Terminal/470.2 (codex-tui; 0.141.0)", want: true},
		{name: "override UA trailer copilot", ua: "cccc/0.141.0 (Mac OS 26.5.0; arm64) VSCode/1.0 (codex_vscode_copilot; 0.141.0)", want: true},
		{name: "复合 UA 包含 codex_app", ua: "Mozilla/5.0 codex_app/0.1.0", want: true},
		{name: "大小写混合", ua: "Codex_VSCode/1.2.3", want: true},
		{name: "trailer 非 codex 不命中", ua: "cccc/0.141.0 (Mac OS 26.5.0; arm64) Apple_Terminal/470.2 (evil-codex_cli; 0.141.0)", want: false},
		{name: "trailer malformed missing version", ua: "cccc/0.141.0 (Mac OS 26.5.0; arm64) Apple_Terminal/470.2 (codex-tui; )", want: false},
		{name: "非 codex", ua: "curl/8.0.1", want: false},
		{name: "空字符串", ua: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsCodexOfficialClientRequest(tt.ua)
			if got != tt.want {
				t.Fatalf("IsCodexOfficialClientRequest(%q) = %v, want %v", tt.ua, got, tt.want)
			}
		})
	}
}

func TestIsCodexOfficialClientRequestStrict(t *testing.T) {
	tests := []struct {
		name string
		ua   string
		want bool
	}{
		{name: "codex_cli_rs 前缀", ua: "codex_cli_rs/0.98.0", want: true},
		{name: "codex-tui 前缀", ua: "codex-tui/0.76.0", want: true},
		{name: "codex_vscode 前缀", ua: "codex_vscode/1.0.0", want: true},
		{name: "codex_vscode_copilot 前缀", ua: "codex_vscode_copilot/1.0.0", want: true},
		{name: "codex_app 前缀", ua: "codex_app/0.1.0", want: true},
		{name: "codex_chatgpt_desktop 前缀", ua: "codex_chatgpt_desktop/1.0.0", want: true},
		{name: "codex_atlas 前缀", ua: "codex_atlas/1.0.0", want: true},
		{name: "codex_exec 前缀", ua: "codex_exec/0.1.0", want: true},
		{name: "codex_sdk_ts 前缀", ua: "codex_sdk_ts/0.1.0", want: true},
		{name: "Codex 桌面 UA", ua: "Codex Desktop/1.2.3", want: true},
		{name: "Codex family trailing text", ua: "Codex Some Future Client/9.9", want: true},
		{name: "override UA trailer codex-tui", ua: "cccc/0.141.0 (Mac OS 26.5.0; arm64) Apple_Terminal/470.2 (codex-tui; 0.141.0)", want: true},
		{name: "override UA trailer copilot", ua: "cccc/0.141.0 (Mac OS 26.5.0; arm64) VSCode/1.0 (codex_vscode_copilot; 0.141.0)", want: true},
		{name: "大小写混合官方前缀", ua: "Codex_VSCode/1.2.3", want: true},
		{name: "复合 UA 不命中", ua: "Mozilla/5.0 codex_app/0.141.0", want: false},
		{name: "浏览器 UA trailer 不命中", ua: "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) (codex-tui; 0.141.0)", want: false},
		{name: "Codex 无尾随空格不命中", ua: "Codex", want: false},
		{name: "小写 codex desktop 不命中 literal Codex family", ua: "codex desktop/1.2.3", want: false},
		{name: "trailer 非 codex 不命中", ua: "cccc/0.141.0 (Mac OS 26.5.0; arm64) Apple_Terminal/470.2 (evil-codex_cli; 0.141.0)", want: false},
		{name: "trailer unknown codex 不命中", ua: "cccc/0.141.0 (Mac OS 26.5.0; arm64) Apple_Terminal/470.2 (codex_unknown; 0.141.0)", want: false},
		{name: "trailer malformed missing semicolon 不命中", ua: "cccc/0.141.0 (Mac OS 26.5.0; arm64) Apple_Terminal/470.2 (codex-tui 0.141.0)", want: false},
		{name: "非 codex", ua: "curl/8.0.1", want: false},
		{name: "空字符串", ua: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsCodexOfficialClientRequestStrict(tt.ua)
			if got != tt.want {
				t.Fatalf("IsCodexOfficialClientRequestStrict(%q) = %v, want %v", tt.ua, got, tt.want)
			}
		})
	}
}

func TestIsCodexOfficialClientOriginator(t *testing.T) {
	tests := []struct {
		name       string
		originator string
		want       bool
	}{
		{name: "codex_cli_rs", originator: "codex_cli_rs", want: true},
		{name: "codex-tui", originator: "codex-tui", want: true},
		{name: "codex_vscode", originator: "codex_vscode", want: true},
		{name: "codex_vscode_copilot", originator: "codex_vscode_copilot", want: true},
		{name: "codex_app", originator: "codex_app", want: true},
		{name: "codex_chatgpt_desktop", originator: "codex_chatgpt_desktop", want: true},
		{name: "codex_atlas", originator: "codex_atlas", want: true},
		{name: "codex_exec", originator: "codex_exec", want: true},
		{name: "codex_sdk_ts", originator: "codex_sdk_ts", want: true},
		{name: "Codex 前缀", originator: "Codex Desktop", want: true},
		{name: "Codex family trailing text", originator: "Codex Some Future Client", want: true},
		{name: "空白包裹", originator: "  codex_vscode  ", want: true},
		{name: "evil codex 不命中", originator: "evil-codex_cli", want: false},
		{name: "my codex 不命中", originator: "my_codex_thing", want: false},
		{name: "codex 下划线未知不命中", originator: "codex_unknown", want: false},
		{name: "Codex 无尾随空格不命中", originator: "Codex", want: false},
		{name: "非 codex", originator: "my_client", want: false},
		{name: "空字符串", originator: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsCodexOfficialClientOriginator(tt.originator)
			if got != tt.want {
				t.Fatalf("IsCodexOfficialClientOriginator(%q) = %v, want %v", tt.originator, got, tt.want)
			}
		})
	}
}

func TestIsCodexOfficialClientOriginatorStrict(t *testing.T) {
	tests := []struct {
		name       string
		originator string
		want       bool
	}{
		{name: "codex_cli_rs", originator: "codex_cli_rs", want: true},
		{name: "codex-tui", originator: "codex-tui", want: true},
		{name: "codex_vscode", originator: "codex_vscode", want: true},
		{name: "codex_vscode_copilot", originator: "codex_vscode_copilot", want: true},
		{name: "codex_app", originator: "codex_app", want: true},
		{name: "codex_chatgpt_desktop", originator: "codex_chatgpt_desktop", want: true},
		{name: "codex_atlas", originator: "codex_atlas", want: true},
		{name: "codex_exec", originator: "codex_exec", want: true},
		{name: "codex_sdk_ts", originator: "codex_sdk_ts", want: true},
		{name: "Codex 前缀", originator: "Codex Desktop", want: true},
		{name: "Codex family trailing text", originator: "Codex Some Future Client", want: true},
		{name: "空白包裹", originator: "  codex_vscode  ", want: true},
		{name: "evil codex 不命中", originator: "evil-codex_cli", want: false},
		{name: "my codex 不命中", originator: "my_codex_thing", want: false},
		{name: "codex 下划线未知不命中", originator: "codex_unknown", want: false},
		{name: "Codex 无尾随空格不命中", originator: "Codex", want: false},
		{name: "非 codex", originator: "my_client", want: false},
		{name: "空字符串", originator: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsCodexOfficialClientOriginatorStrict(tt.originator)
			if got != tt.want {
				t.Fatalf("IsCodexOfficialClientOriginatorStrict(%q) = %v, want %v", tt.originator, got, tt.want)
			}
		})
	}
}

func TestIsCodexOfficialClientByHeaders(t *testing.T) {
	tests := []struct {
		name       string
		ua         string
		originator string
		want       bool
	}{
		{name: "仅 originator 命中 desktop", originator: "Codex Desktop", want: true},
		{name: "仅 originator 命中 vscode", originator: "codex_vscode", want: true},
		{name: "仅 originator 命中 copilot", originator: "codex_vscode_copilot", want: true},
		{name: "仅 ua 命中 desktop", ua: "Codex Desktop/1.2.3", want: true},
		{name: "仅 ua trailer 命中", ua: "cccc/0.141.0 (Mac OS 26.5.0; arm64) Apple_Terminal/470.2 (codex-tui; 0.141.0)", want: true},
		{name: "伪造 originator 不命中", originator: "evil-codex_cli", want: false},
		{name: "未知 codex originator 不命中", originator: "codex_unknown", want: false},
		{name: "ua 与 originator 都未命中", ua: "curl/8.0.1", originator: "my_client", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsCodexOfficialClientByHeaders(tt.ua, tt.originator)
			if got != tt.want {
				t.Fatalf("IsCodexOfficialClientByHeaders(%q, %q) = %v, want %v", tt.ua, tt.originator, got, tt.want)
			}
		})
	}
}

func TestIsCodexOfficialClientByHeadersStrict(t *testing.T) {
	tests := []struct {
		name       string
		ua         string
		originator string
		want       bool
	}{
		{name: "仅 originator 命中 desktop", originator: "Codex Desktop", want: true},
		{name: "仅 originator 命中 vscode", originator: "codex_vscode", want: true},
		{name: "仅 originator 命中 copilot", originator: "codex_vscode_copilot", want: true},
		{name: "仅 ua 命中 desktop", ua: "Codex Desktop/1.2.3", want: true},
		{name: "仅 ua trailer 命中", ua: "cccc/0.141.0 (Mac OS 26.5.0; arm64) Apple_Terminal/470.2 (codex-tui; 0.141.0)", want: true},
		{name: "复合 UA 不命中", ua: "Mozilla/5.0 codex_app/0.141.0", want: false},
		{name: "浏览器 UA trailer 不命中", ua: "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) (codex-tui; 0.141.0)", want: false},
		{name: "伪造 originator 不命中", originator: "evil-codex_cli", want: false},
		{name: "未知 codex originator 不命中", originator: "codex_unknown", want: false},
		{name: "ua 与 originator 都未命中", ua: "curl/8.0.1", originator: "my_client", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsCodexOfficialClientByHeadersStrict(tt.ua, tt.originator)
			if got != tt.want {
				t.Fatalf("IsCodexOfficialClientByHeadersStrict(%q, %q) = %v, want %v", tt.ua, tt.originator, got, tt.want)
			}
		})
	}
}
